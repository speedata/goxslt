package goxslt

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
)

// TransformResult holds the primary and any secondary result documents.
type TransformResult struct {
	Document           *goxml.XMLDocument            // primary output
	SecondaryDocuments map[string]*goxml.XMLDocument // href → document
}

// TransformContext holds the state during XSLT transformation.
type TransformContext struct {
	MatchCtx           *MatchContext
	CurrentMode        *Mode
	Modes              map[string]*Mode
	DefaultMode        *Mode
	CurrentNode        goxml.XMLNode
	CurrentItem        goxpath.Item // non-nil when iterating over non-node items (arrays, maps, atomics)
	SourceDoc          *goxml.XMLDocument
	XPath              *goxpath.Parser
	OutputStack        []goxml.Appender // stack of output targets
	Stylesheet         *Stylesheet
	MessageHandler     func(text string, terminate bool) // optional callback for xsl:message
	msgWriter          io.Writer                         // output for xsl:message (default os.Stderr)
	MapBuilder         *goxpath.XPathMap                 // non-nil when inside xsl:map (xsl:map-entry appends here)
	SecondaryDocuments map[string]*goxml.XMLDocument     // href → secondary result documents
}

// TransformOptions configures optional behavior for the transformation.
type TransformOptions struct {
	MessageHandler func(text string, terminate bool) // callback for xsl:message
	MessageWriter  io.Writer                         // output for xsl:message (default os.Stderr)
	Parameters     map[string]goxpath.Sequence       // stylesheet parameters (overrides xsl:param defaults)
}

// Transform applies the compiled stylesheet to the source document and returns
// the result.
func Transform(ss *Stylesheet, sourceDoc *goxml.XMLDocument) (*TransformResult, error) {
	return TransformWithOptions(ss, sourceDoc, TransformOptions{})
}

// TransformWithOptions applies the stylesheet with configurable options.
func TransformWithOptions(ss *Stylesheet, sourceDoc *goxml.XMLDocument, opts TransformOptions) (*TransformResult, error) {
	resultDoc := &goxml.XMLDocument{}

	xp := &goxpath.Parser{Ctx: goxpath.NewContext(sourceDoc)}

	matchCtx := &MatchContext{Namespaces: make(map[string]string)}

	tc := &TransformContext{
		MatchCtx:           matchCtx,
		CurrentMode:        ss.DefaultMode,
		DefaultMode:        ss.DefaultMode,
		Modes:              ss.Modes,
		SourceDoc:          sourceDoc,
		XPath:              xp,
		OutputStack:        []goxml.Appender{resultDoc},
		Stylesheet:         ss,
		MessageHandler:     opts.MessageHandler,
		msgWriter:          opts.MessageWriter,
		SecondaryDocuments: make(map[string]*goxml.XMLDocument),
	}

	// Set up XPath evaluation for match pattern predicates.
	matchCtx.XPathEval = func(expr string, node goxml.XMLNode) (bool, error) {
		tc.XPath.Ctx.SetContextSequence(goxpath.Sequence{node})
		result, err := tc.XPath.Evaluate(expr)
		if err != nil {
			return false, err
		}
		return goxpath.BooleanValue(result)
	}

	// Propagate stylesheet namespace declarations to the XPath context
	// so that function prefixes can be resolved.
	for prefix, uri := range ss.Namespaces {
		xp.Ctx.Namespaces[prefix] = uri
	}

	// Process global stylesheet parameters.
	for _, p := range ss.GlobalParams {
		if opts.Parameters != nil {
			if val, ok := opts.Parameters[p.Name]; ok {
				if p.As != nil {
					var coerceErr error
					val, coerceErr = coerceSequence(p.As, val)
					if coerceErr != nil {
						return nil, fmt.Errorf("xsl:param name='%s': %w", p.Name, coerceErr)
					}
				}
				xp.SetVariable(p.Name, val)
				continue
			}
		}
		// Use default value.
		val, err := evalParamValue(tc, p.Select, p.Children)
		if err != nil {
			return nil, fmt.Errorf("xsl:param name='%s': %w", p.Name, err)
		}
		if p.As != nil {
			val, err = coerceSequence(p.As, val)
			if err != nil {
				return nil, fmt.Errorf("xsl:param name='%s': %w", p.Name, err)
			}
		}
		xp.SetVariable(p.Name, val)
	}

	// Process global stylesheet variables.
	for i := range ss.GlobalVars {
		if err := ss.GlobalVars[i].Execute(tc); err != nil {
			return nil, fmt.Errorf("xsl:variable name='%s': %w", ss.GlobalVars[i].Name, err)
		}
	}

	// Register stylesheet functions with the XPath evaluator.
	for _, fdef := range ss.Functions {
		fdef := fdef // capture for closure
		goxpath.RegisterFunction(&goxpath.Function{
			Name:      fdef.LocalName,
			Namespace: fdef.Namespace,
			MinArg:    len(fdef.Params),
			MaxArg:    len(fdef.Params),
			F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
				// Isolate variable scope.
				origCtx := tc.XPath.Ctx
				newCtx := goxpath.CopyContext(origCtx)
				tc.XPath.Ctx = newCtx

				// Bind positional parameters with optional type coercion.
				for i, p := range fdef.Params {
					arg := args[i]
					if p.As != nil {
						var coerceErr error
						arg, coerceErr = coerceSequence(p.As, arg)
						if coerceErr != nil {
							tc.XPath.Ctx = origCtx
							return nil, fmt.Errorf("xsl:function %s:%s param $%s: %w", fdef.Namespace, fdef.LocalName, p.Name, coerceErr)
						}
					}
					tc.XPath.SetVariable(p.Name, arg)
				}

				// Execute function body into a fragment document.
				fragDoc := &goxml.XMLDocument{}
				tc.pushOutput(fragDoc)
				err := tc.ExecuteTemplate(fdef.Body)
				tc.popOutput()

				// Restore original context.
				tc.XPath.Ctx = origCtx

				if err != nil {
					return nil, fmt.Errorf("xsl:function %s:%s: %w", fdef.Namespace, fdef.LocalName, err)
				}

				// Collect results.
				children := fragDoc.Children()
				var seq goxpath.Sequence
				for _, child := range children {
					seq = append(seq, child)
				}

				// Validate/coerce return type.
				if fdef.As != nil {
					seq, err = coerceSequence(fdef.As, seq)
					if err != nil {
						return nil, fmt.Errorf("xsl:function %s:%s return type: %w", fdef.Namespace, fdef.LocalName, err)
					}
				}

				return seq, nil
			},
		})
	}

	// Start transformation: apply-templates to the document node.
	if err := tc.ApplyTemplates(ss.DefaultMode, []goxml.XMLNode{sourceDoc}); err != nil {
		return nil, err
	}

	return &TransformResult{
		Document:           resultDoc,
		SecondaryDocuments: tc.SecondaryDocuments,
	}, nil
}

// MessageWriter returns the writer for xsl:message output (defaults to os.Stderr).
func (tc *TransformContext) MessageWriter() io.Writer {
	if tc.msgWriter != nil {
		return tc.msgWriter
	}
	return os.Stderr
}

// output returns the current output target.
func (tc *TransformContext) output() goxml.Appender {
	return tc.OutputStack[len(tc.OutputStack)-1]
}

// pushOutput pushes a new output target onto the stack.
func (tc *TransformContext) pushOutput(a goxml.Appender) {
	tc.OutputStack = append(tc.OutputStack, a)
}

// popOutput removes the top output target.
func (tc *TransformContext) popOutput() {
	tc.OutputStack = tc.OutputStack[:len(tc.OutputStack)-1]
}

// ApplyTemplates selects the best template for each node and executes it.
func (tc *TransformContext) ApplyTemplates(mode *Mode, nodes []goxml.XMLNode) error {
	return tc.ApplyTemplatesWithParams(mode, nodes, nil)
}

func (tc *TransformContext) ApplyTemplatesWithParams(mode *Mode, nodes []goxml.XMLNode, paramValues map[string]goxpath.Sequence) error {
	for _, node := range nodes {
		rule, err := mode.GetRule(node, tc.MatchCtx)
		if err != nil {
			return err
		}

		prevNode := tc.CurrentNode
		tc.CurrentNode = node

		if rule == nil {
			if mode.BuiltInRules != nil {
				if err := mode.BuiltInRules.Process(node, tc); err != nil {
					return err
				}
			}
		} else {
			// Create isolated variable scope and set parameters.
			origCtx := tc.XPath.Ctx
			newCtx := goxpath.CopyContext(origCtx)
			tc.XPath.Ctx = newCtx

			for _, p := range rule.Template.Params {
				if val, ok := paramValues[p.Name]; ok {
					tc.XPath.SetVariable(p.Name, val)
				} else {
					val, err := evalParamValue(tc, p.Select, p.Children)
					if err != nil {
						tc.XPath.Ctx = origCtx
						return fmt.Errorf("xsl:param name='%s' default: %w", p.Name, err)
					}
					tc.XPath.SetVariable(p.Name, val)
				}
			}

			if err := tc.ExecuteTemplate(rule.Template); err != nil {
				tc.XPath.Ctx = origCtx
				return err
			}
			tc.XPath.Ctx = origCtx
		}

		tc.CurrentNode = prevNode
	}
	return nil
}

// ExecuteTemplate executes a compiled template body.
func (tc *TransformContext) ExecuteTemplate(tmpl *TemplateBody) error {
	for _, instr := range tmpl.Instructions {
		if err := instr.Execute(tc); err != nil {
			return err
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Instruction Execute implementations
// --------------------------------------------------------------------------

func (instr *LiteralElement) Execute(ctx *TransformContext) error {
	elt := goxml.NewElement()
	elt.Name = instr.Name
	elt.Prefix = instr.Prefix
	if instr.Namespace != "" && instr.Prefix != "" {
		elt.Namespaces[instr.Prefix] = instr.Namespace
	}
	// Propagate result namespaces to root-level output elements only.
	// Child elements inherit namespace declarations in XML.
	if _, isDoc := ctx.output().(*goxml.XMLDocument); isDoc {
		for prefix, uri := range ctx.Stylesheet.ResultNamespaces {
			elt.Namespaces[prefix] = uri
		}
	}
	for _, attr := range instr.Attributes {
		val, err := ctx.evalAVT(attr.Value)
		if err != nil {
			return fmt.Errorf("literal element %s attribute %s: %w", instr.Name, attr.Name, err)
		}
		elt.SetAttribute(xml.Attr{
			Name:  xml.Name{Local: attr.Name},
			Value: val,
		})
	}

	ctx.output().Append(elt)

	ctx.pushOutput(elt)
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	ctx.popOutput()

	return nil
}

func (instr *LiteralText) Execute(ctx *TransformContext) error {
	ctx.output().Append(goxml.CharData{Contents: instr.Text})
	return nil
}

func (instr *XSLText) Execute(ctx *TransformContext) error {
	ctx.output().Append(goxml.CharData{Contents: instr.Text})
	return nil
}

func (instr *XSLValueOf) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:value-of select='%s': %w", instr.Select, err)
	}
	text := result.Stringvalue()
	if text != "" {
		ctx.output().Append(goxml.CharData{Contents: text})
	}
	return nil
}

func (instr *XSLApplyTemplates) Execute(ctx *TransformContext) error {
	mode := ctx.CurrentMode
	if instr.Mode != "" {
		if m, ok := ctx.Modes[instr.Mode]; ok {
			mode = m
		}
	}

	var nodes []goxml.XMLNode

	if instr.Select == "" {
		// Default: select child nodes of current node.
		nodes = ctx.CurrentNode.Children()
	} else {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:apply-templates select='%s': %w", instr.Select, err)
		}
		nodes = sequenceToNodes(result)
	}

	if len(instr.Sorts) > 0 {
		if err := ctx.sortNodes(nodes, instr.Sorts); err != nil {
			return err
		}
	}

	// Evaluate with-param values in the caller's context.
	var paramValues map[string]goxpath.Sequence
	if len(instr.WithParams) > 0 {
		paramValues = make(map[string]goxpath.Sequence)
		for _, wp := range instr.WithParams {
			val, err := evalParamValue(ctx, wp.Select, wp.Children)
			if err != nil {
				return fmt.Errorf("xsl:with-param name='%s': %w", wp.Name, err)
			}
			paramValues[wp.Name] = val
		}
	}

	prevMode := ctx.CurrentMode
	ctx.CurrentMode = mode
	err := ctx.ApplyTemplatesWithParams(mode, nodes, paramValues)
	ctx.CurrentMode = prevMode
	return err
}

func (instr *XSLForEach) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:for-each select='%s': %w", instr.Select, err)
	}

	if len(instr.Sorts) > 0 {
		nodes := sequenceToNodes(result)
		if err := ctx.sortNodes(nodes, instr.Sorts); err != nil {
			return err
		}
		// Rebuild result from sorted nodes.
		result = make(goxpath.Sequence, len(nodes))
		for i, n := range nodes {
			result[i] = n
		}
	}

	for i, item := range result {
		prevNode := ctx.CurrentNode
		prevItem := ctx.CurrentItem
		prevPos := ctx.XPath.Ctx.Pos

		ctx.XPath.Ctx.Pos = i + 1

		if node, ok := item.(goxml.XMLNode); ok {
			ctx.CurrentNode = node
			ctx.CurrentItem = nil
		} else {
			ctx.CurrentItem = item
		}

		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				return err
			}
		}

		ctx.CurrentNode = prevNode
		ctx.CurrentItem = prevItem
		ctx.XPath.Ctx.Pos = prevPos
	}
	return nil
}

func (instr *XSLForEachGroup) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:for-each-group select='%s': %w", instr.Select, err)
	}

	nodes := sequenceToNodes(result)
	if len(nodes) == 0 {
		return nil
	}

	// Group by key, preserving order of first occurrence.
	type group struct {
		key   string
		nodes []goxml.XMLNode
	}
	var groups []group
	keyIndex := make(map[string]int) // key -> index in groups

	for _, node := range nodes {
		prevNode := ctx.CurrentNode
		ctx.CurrentNode = node
		keyResult, err := ctx.evalXPath(instr.GroupBy)
		ctx.CurrentNode = prevNode
		if err != nil {
			return fmt.Errorf("xsl:for-each-group group-by='%s': %w", instr.GroupBy, err)
		}
		key := keyResult.Stringvalue()

		if idx, ok := keyIndex[key]; ok {
			groups[idx].nodes = append(groups[idx].nodes, node)
		} else {
			keyIndex[key] = len(groups)
			groups = append(groups, group{key: key, nodes: []goxml.XMLNode{node}})
		}
	}

	// Optional: sort groups by sort keys (applying to first item of each group).
	if len(instr.Sorts) > 0 {
		firstItems := make([]goxml.XMLNode, len(groups))
		for i, g := range groups {
			firstItems[i] = g.nodes[0]
		}
		if err := ctx.sortNodes(firstItems, instr.Sorts); err != nil {
			return err
		}
		// Reorder groups to match sorted first items.
		sortedGroups := make([]group, len(groups))
		for i, node := range firstItems {
			for _, g := range groups {
				if g.nodes[0] == node {
					sortedGroups[i] = g
					break
				}
			}
		}
		groups = sortedGroups
	}

	// Execute body for each group.
	for _, g := range groups {
		prevNode := ctx.CurrentNode
		ctx.CurrentNode = g.nodes[0]

		// Set current-grouping-key() and current-group().
		if ctx.XPath.Ctx.Store == nil {
			ctx.XPath.Ctx.Store = make(map[interface{}]interface{})
		}
		prevKey := ctx.XPath.Ctx.Store["current-grouping-key"]
		prevGroup := ctx.XPath.Ctx.Store["current-group"]

		ctx.XPath.Ctx.Store["current-grouping-key"] = g.key
		groupSeq := make(goxpath.Sequence, len(g.nodes))
		for i, n := range g.nodes {
			groupSeq[i] = n
		}
		ctx.XPath.Ctx.Store["current-group"] = groupSeq

		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.XPath.Ctx.Store["current-grouping-key"] = prevKey
				ctx.XPath.Ctx.Store["current-group"] = prevGroup
				ctx.CurrentNode = prevNode
				return err
			}
		}

		ctx.XPath.Ctx.Store["current-grouping-key"] = prevKey
		ctx.XPath.Ctx.Store["current-group"] = prevGroup
		ctx.CurrentNode = prevNode
	}

	return nil
}

func (instr *XSLIf) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Test)
	if err != nil {
		return fmt.Errorf("xsl:if test='%s': %w", instr.Test, err)
	}
	boolVal, err := goxpath.BooleanValue(result)
	if err != nil {
		return err
	}
	if !boolVal {
		return nil
	}
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (instr *XSLChoose) Execute(ctx *TransformContext) error {
	for _, when := range instr.When {
		result, err := ctx.evalXPath(when.Test)
		if err != nil {
			return fmt.Errorf("xsl:when test='%s': %w", when.Test, err)
		}
		boolVal, err := goxpath.BooleanValue(result)
		if err != nil {
			return err
		}
		if boolVal {
			for _, child := range when.Children {
				if err := child.Execute(ctx); err != nil {
					return err
				}
			}
			return nil
		}
	}
	// No xsl:when matched → execute xsl:otherwise if present.
	for _, child := range instr.Otherwise {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (instr *XSLVariable) Execute(ctx *TransformContext) error {
	var seq goxpath.Sequence
	if instr.Select != "" {
		// Variable bound via select expression.
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:variable name='%s' select='%s': %w", instr.Name, instr.Select, err)
		}
		seq = result
	} else if len(instr.Children) == 1 {
		if mapInstr, ok := instr.Children[0].(*XSLMap); ok {
			// Special case: xsl:map produces a map value, not XML nodes.
			m, err := mapInstr.BuildMap(ctx)
			if err != nil {
				return fmt.Errorf("xsl:variable name='%s': %w", instr.Name, err)
			}
			seq = goxpath.Sequence{m}
		} else {
			// Single non-map child → build a result tree fragment.
			fragDoc := &goxml.XMLDocument{}
			ctx.pushOutput(fragDoc)
			if err := instr.Children[0].Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
			ctx.popOutput()
			for _, child := range fragDoc.Children() {
				seq = append(seq, child)
			}
		}
	} else if len(instr.Children) > 0 {
		// Variable bound via body content → build a result tree fragment.
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		// Set the variable to the children of the fragment.
		for _, child := range fragDoc.Children() {
			seq = append(seq, child)
		}
	} else {
		// Empty variable → empty string.
		seq = goxpath.Sequence{""}
	}
	if instr.As != nil {
		var err error
		seq, err = coerceSequence(instr.As, seq)
		if err != nil {
			return fmt.Errorf("xsl:variable name='%s': %w", instr.Name, err)
		}
	}
	ctx.XPath.SetVariable(instr.Name, seq)
	return nil
}

func (instr *XSLCopyOf) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:copy-of select='%s': %w", instr.Select, err)
	}
	for _, item := range result {
		switch n := item.(type) {
		case goxml.XMLNode:
			ctx.output().Append(n)
		case string:
			ctx.output().Append(goxml.CharData{Contents: n})
		}
	}
	return nil
}

func (instr *XSLSequence) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:sequence select='%s': %w", instr.Select, err)
	}
	for _, item := range result {
		switch n := item.(type) {
		case goxml.XMLNode:
			ctx.output().Append(n)
		default:
			ctx.output().Append(goxml.CharData{Contents: fmt.Sprintf("%v", n)})
		}
	}
	return nil
}

func (instr *XSLElement) Execute(ctx *TransformContext) error {
	name, err := ctx.evalAVT(instr.Name)
	if err != nil {
		return fmt.Errorf("xsl:element name: %w", err)
	}

	elt := goxml.NewElement()

	// Handle prefix:localname.
	if idx := strings.IndexByte(name, ':'); idx >= 0 {
		elt.Prefix = name[:idx]
		elt.Name = name[idx+1:]
	} else {
		elt.Name = name
	}

	// Set namespace if provided.
	if len(instr.Namespace.Parts) > 0 {
		ns, err := ctx.evalAVT(instr.Namespace)
		if err != nil {
			return fmt.Errorf("xsl:element namespace: %w", err)
		}
		if ns != "" && elt.Prefix != "" {
			elt.Namespaces[elt.Prefix] = ns
		}
	}

	ctx.output().Append(elt)

	ctx.pushOutput(elt)
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	ctx.popOutput()

	return nil
}

func (instr *XSLComment) Execute(ctx *TransformContext) error {
	var text string
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:comment select='%s': %w", instr.Select, err)
		}
		text = result.Stringvalue()
	} else if len(instr.Children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		var sb strings.Builder
		for _, child := range fragDoc.Children() {
			sb.WriteString(nodeStringValue(child))
		}
		text = sb.String()
	}
	ctx.output().Append(goxml.Comment{Contents: text})
	return nil
}

func (instr *XSLProcessingInstruction) Execute(ctx *TransformContext) error {
	name, err := ctx.evalAVT(instr.Name)
	if err != nil {
		return fmt.Errorf("xsl:processing-instruction name: %w", err)
	}

	var text string
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:processing-instruction select='%s': %w", instr.Select, err)
		}
		text = result.Stringvalue()
	} else if len(instr.Children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		var sb strings.Builder
		for _, child := range fragDoc.Children() {
			sb.WriteString(nodeStringValue(child))
		}
		text = sb.String()
	}

	ctx.output().Append(goxml.ProcInst{Target: name, Inst: []byte(text)})
	return nil
}

func (instr *XSLMessage) Execute(ctx *TransformContext) error {
	var text string
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:message select='%s': %w", instr.Select, err)
		}
		text = result.StringvalueJoin(" ")
	} else if len(instr.Children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		var sb strings.Builder
		for _, child := range fragDoc.Children() {
			sb.WriteString(nodeStringValue(child))
		}
		text = sb.String()
	}

	if ctx.MessageHandler != nil {
		ctx.MessageHandler(text, instr.Terminate)
	} else {
		fmt.Fprintf(ctx.MessageWriter(), "XSLT message: %s\n", text)
	}

	if instr.Terminate {
		return fmt.Errorf("XTMM9000: xsl:message terminate='yes': %s", text)
	}
	return nil
}

// --------------------------------------------------------------------------
// XSLNumber
// --------------------------------------------------------------------------

func (instr *XSLNumber) Execute(ctx *TransformContext) error {
	// Determine the source node for counting.
	sourceNode := ctx.CurrentNode
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:number select='%s': %w", instr.Select, err)
		}
		nodes := sequenceToNodes(result)
		if len(nodes) == 0 {
			return nil
		}
		sourceNode = nodes[0]
	}

	var numbers []int

	if instr.Value != "" {
		// Direct value expression.
		prevNode := ctx.CurrentNode
		ctx.CurrentNode = sourceNode
		nums, err := numberValue(ctx, instr.Value)
		ctx.CurrentNode = prevNode
		if err != nil {
			return err
		}
		numbers = nums
	} else {
		// Count-based numbering: use pre-compiled patterns from compile time.
		countPat := instr.CountPat
		if countPat == nil {
			countPat = defaultCountPattern(sourceNode)
		}

		fromPat := instr.FromPat

		mc := ctx.MatchCtx

		switch instr.Level {
		case "single":
			numbers = numberSingle(sourceNode, countPat, fromPat, mc)
		case "multiple":
			numbers = numberMultiple(sourceNode, countPat, fromPat, mc)
		case "any":
			numbers = numberAny(sourceNode, countPat, fromPat, mc)
		default:
			return fmt.Errorf("xsl:number: unsupported level='%s'", instr.Level)
		}
	}

	if len(numbers) == 0 {
		return nil
	}

	text := formatNumbers(numbers, instr.Format)
	if text != "" {
		ctx.output().Append(goxml.CharData{Contents: text})
	}
	return nil
}

// --------------------------------------------------------------------------
// XSLMap + XSLMapEntry
// --------------------------------------------------------------------------

// BuildMap evaluates all children (including xsl:map-entry and any wrapping
// instructions like xsl:for-each) and collects the resulting map entries.
// It sets ctx.MapBuilder so that nested XSLMapEntry.Execute calls append to
// the map being built.
func (instr *XSLMap) BuildMap(ctx *TransformContext) (*goxpath.XPathMap, error) {
	m := &goxpath.XPathMap{}
	prevBuilder := ctx.MapBuilder
	ctx.MapBuilder = m
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			ctx.MapBuilder = prevBuilder
			return nil, err
		}
	}
	ctx.MapBuilder = prevBuilder
	return m, nil
}

func (instr *XSLMap) Execute(ctx *TransformContext) error {
	// When used directly in output (not inside xsl:variable), build the map
	// and serialize its entries as text.
	m, err := instr.BuildMap(ctx)
	if err != nil {
		return err
	}
	// Maps used outside xsl:variable are stringified.
	ctx.output().Append(goxml.CharData{Contents: fmt.Sprintf("%v", m)})
	return nil
}

func (instr *XSLMapEntry) Execute(ctx *TransformContext) error {
	if ctx.MapBuilder == nil {
		return fmt.Errorf("xsl:map-entry used outside xsl:map")
	}
	// Evaluate key.
	keyResult, err := ctx.evalXPath(instr.Key)
	if err != nil {
		return fmt.Errorf("xsl:map-entry key='%s': %w", instr.Key, err)
	}
	var key goxpath.Item
	if len(keyResult) > 0 {
		key = keyResult[0]
	}
	// Evaluate value.
	var value goxpath.Sequence
	if instr.Select != "" {
		value, err = ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:map-entry select='%s': %w", instr.Select, err)
		}
	} else if len(instr.Children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, c := range instr.Children {
			if err := c.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		for _, c := range fragDoc.Children() {
			value = append(value, c)
		}
	}
	ctx.MapBuilder.Entries = append(ctx.MapBuilder.Entries, goxpath.MapEntry{Key: key, Value: value})
	return nil
}

// --------------------------------------------------------------------------
// XSLResultDocument — redirect output to a secondary document
// --------------------------------------------------------------------------

func (instr *XSLResultDocument) Execute(ctx *TransformContext) error {
	href, err := ctx.evalAVT(instr.Href)
	if err != nil {
		return fmt.Errorf("xsl:result-document href: %w", err)
	}
	if href == "" {
		return fmt.Errorf("xsl:result-document: href is empty")
	}

	secDoc := &goxml.XMLDocument{}
	ctx.pushOutput(secDoc)
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			ctx.popOutput()
			return err
		}
	}
	ctx.popOutput()

	ctx.SecondaryDocuments[href] = secDoc
	return nil
}

// --------------------------------------------------------------------------
// evalAVT evaluates an Attribute Value Template.
// --------------------------------------------------------------------------

func (tc *TransformContext) evalAVT(avt AVT) (string, error) {
	// Fast path: single static part.
	if len(avt.Parts) == 1 && avt.Parts[0].Expr == "" {
		return avt.Parts[0].Text, nil
	}
	var sb strings.Builder
	for _, part := range avt.Parts {
		if part.Expr != "" {
			result, err := tc.evalXPath(part.Expr)
			if err != nil {
				return "", fmt.Errorf("AVT expression {%s}: %w", part.Expr, err)
			}
			sb.WriteString(result.Stringvalue())
		} else {
			sb.WriteString(part.Text)
		}
	}
	return sb.String(), nil
}

// --------------------------------------------------------------------------
// XSLCopy — shallow copy
// --------------------------------------------------------------------------

func (instr *XSLCopy) Execute(ctx *TransformContext) error {
	switch n := ctx.CurrentNode.(type) {
	case *goxml.Element:
		elt := goxml.NewElement()
		elt.Name = n.Name
		elt.Prefix = n.Prefix
		for k, v := range n.Namespaces {
			elt.Namespaces[k] = v
		}
		ctx.output().Append(elt)
		ctx.pushOutput(elt)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				return err
			}
		}
		ctx.popOutput()
	case goxml.CharData:
		ctx.output().Append(goxml.CharData{Contents: n.Contents})
	case goxml.Comment:
		ctx.output().Append(goxml.Comment{Contents: n.Contents})
	case goxml.ProcInst:
		ctx.output().Append(goxml.ProcInst{Target: n.Target, Inst: n.Inst})
	case *goxml.XMLDocument:
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// XSLAttribute
// --------------------------------------------------------------------------

func (instr *XSLAttribute) Execute(ctx *TransformContext) error {
	name, err := ctx.evalAVT(instr.Name)
	if err != nil {
		return fmt.Errorf("xsl:attribute name: %w", err)
	}

	var value string
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:attribute select='%s': %w", instr.Select, err)
		}
		value = result.Stringvalue()
	} else if len(instr.Children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		var sb strings.Builder
		for _, child := range fragDoc.Children() {
			sb.WriteString(nodeStringValue(child))
		}
		value = sb.String()
	}

	elt, ok := ctx.output().(*goxml.Element)
	if !ok {
		return fmt.Errorf("xsl:attribute '%s': output is not an element", name)
	}
	elt.SetAttribute(xml.Attr{Name: xml.Name{Local: name}, Value: value})
	return nil
}

// nodeStringValue returns the string value of a node (for RTF text extraction).
func nodeStringValue(n goxml.XMLNode) string {
	switch t := n.(type) {
	case goxml.CharData:
		return t.Contents
	case *goxml.Element:
		return t.Stringvalue()
	default:
		return ""
	}
}

// --------------------------------------------------------------------------
// XSLCallTemplate
// --------------------------------------------------------------------------

func (instr *XSLCallTemplate) Execute(ctx *TransformContext) error {
	tmpl, ok := ctx.Stylesheet.NamedTemplates[instr.Name]
	if !ok {
		return fmt.Errorf("xsl:call-template: no template named '%s'", instr.Name)
	}

	// Evaluate with-param values in the caller's context.
	paramValues := make(map[string]goxpath.Sequence)
	for _, wp := range instr.WithParams {
		val, err := evalParamValue(ctx, wp.Select, wp.Children)
		if err != nil {
			return fmt.Errorf("xsl:with-param name='%s': %w", wp.Name, err)
		}
		paramValues[wp.Name] = val
	}

	// Create isolated variable scope.
	origCtx := ctx.XPath.Ctx
	newCtx := goxpath.CopyContext(origCtx)
	ctx.XPath.Ctx = newCtx

	// Set template parameters: with-param overrides, then defaults.
	for _, p := range tmpl.Params {
		if val, ok := paramValues[p.Name]; ok {
			ctx.XPath.SetVariable(p.Name, val)
		} else {
			val, err := evalParamValue(ctx, p.Select, p.Children)
			if err != nil {
				ctx.XPath.Ctx = origCtx
				return fmt.Errorf("xsl:param name='%s' default: %w", p.Name, err)
			}
			ctx.XPath.SetVariable(p.Name, val)
		}
	}

	err := ctx.ExecuteTemplate(tmpl)

	// Restore original context.
	ctx.XPath.Ctx = origCtx
	return err
}

// evalParamValue evaluates a param/with-param value via select or children body.
func evalParamValue(ctx *TransformContext, sel string, children []Instruction) (goxpath.Sequence, error) {
	if sel != "" {
		return ctx.evalXPath(sel)
	}
	if len(children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return nil, err
			}
		}
		ctx.popOutput()
		var seq goxpath.Sequence
		for _, child := range fragDoc.Children() {
			seq = append(seq, child)
		}
		return seq, nil
	}
	return goxpath.Sequence{""}, nil
}

// --------------------------------------------------------------------------
// sortNodes sorts a slice of nodes in-place using sort keys.
// --------------------------------------------------------------------------

func (tc *TransformContext) sortNodes(nodes []goxml.XMLNode, sorts []SortKey) error {
	if len(nodes) == 0 || len(sorts) == 0 {
		return nil
	}

	// Pre-compute sort values for each node and each sort key.
	type sortVal struct {
		str string
		num float64
	}
	vals := make([][]sortVal, len(nodes))
	origXPCtx := tc.XPath.Ctx
	for i, node := range nodes {
		vals[i] = make([]sortVal, len(sorts))
		prevNode := tc.CurrentNode
		tc.CurrentNode = node
		for j, sk := range sorts {
			// Use a fresh XPath context for each evaluation to avoid state leaks.
			tc.XPath.Ctx = goxpath.CopyContext(origXPCtx)
			result, err := tc.evalXPath(sk.Select)
			if err != nil {
				tc.CurrentNode = prevNode
				tc.XPath.Ctx = origXPCtx
				return fmt.Errorf("xsl:sort select='%s': %w", sk.Select, err)
			}
			s := result.Stringvalue()
			vals[i][j].str = s
			if sk.DataType == "number" {
				f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil {
					f = 0
				}
				vals[i][j].num = f
			}
		}
		tc.CurrentNode = prevNode
	}
	tc.XPath.Ctx = origXPCtx

	// Build indices and sort them (avoids the vals/nodes index mismatch when
	// sort.SliceStable swaps nodes in place).
	indices := make([]int, len(nodes))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		ia, ib := indices[a], indices[b]
		for j, sk := range sorts {
			va := vals[ia][j]
			vb := vals[ib][j]
			var cmp int
			if sk.DataType == "number" {
				switch {
				case va.num < vb.num:
					cmp = -1
				case va.num > vb.num:
					cmp = 1
				default:
					cmp = 0
				}
			} else {
				cmp = strings.Compare(va.str, vb.str)
			}
			if sk.Order == "descending" {
				cmp = -cmp
			}
			if cmp != 0 {
				return cmp < 0
			}
		}
		return false
	})
	// Rearrange nodes according to sorted indices.
	sorted := make([]goxml.XMLNode, len(nodes))
	for i, idx := range indices {
		sorted[i] = nodes[idx]
	}
	copy(nodes, sorted)
	return nil
}

// --------------------------------------------------------------------------
// XPath evaluation helper
// --------------------------------------------------------------------------

func (tc *TransformContext) evalXPath(expr string) (goxpath.Sequence, error) {
	if tc.CurrentItem != nil {
		tc.XPath.Ctx.SetContextSequence(goxpath.Sequence{tc.CurrentItem})
	} else {
		tc.XPath.Ctx.SetContextSequence(goxpath.Sequence{tc.CurrentNode})
	}
	return tc.XPath.Evaluate(expr)
}

// sequenceToNodes extracts goxml.XMLNodes from a goxpath.Sequence.
// String items (produced by goxpath for text nodes) are wrapped in CharData.
func sequenceToNodes(seq goxpath.Sequence) []goxml.XMLNode {
	var nodes []goxml.XMLNode
	for _, item := range seq {
		switch v := item.(type) {
		case goxml.XMLNode:
			nodes = append(nodes, v)
		case string:
			nodes = append(nodes, goxml.CharData{Contents: v})
		case int:
			nodes = append(nodes, goxml.CharData{Contents: strconv.Itoa(v)})
		case float64:
			nodes = append(nodes, goxml.CharData{Contents: strconv.FormatFloat(v, 'f', -1, 64)})
		case bool:
			if v {
				nodes = append(nodes, goxml.CharData{Contents: "true"})
			} else {
				nodes = append(nodes, goxml.CharData{Contents: "false"})
			}
		default:
			nodes = append(nodes, goxml.CharData{Contents: fmt.Sprintf("%v", v)})
		}
	}
	return nodes
}

// --------------------------------------------------------------------------
// TextOnlyCopyRuleSet — default built-in rules
// --------------------------------------------------------------------------

type TextOnlyCopyRuleSet struct{}

func (b *TextOnlyCopyRuleSet) Process(node goxml.XMLNode, ctx *TransformContext) error {
	switch n := node.(type) {
	case *goxml.XMLDocument:
		return ctx.ApplyTemplates(ctx.CurrentMode, n.Children())
	case *goxml.Element:
		return ctx.ApplyTemplates(ctx.CurrentMode, n.Children())
	case goxml.CharData:
		ctx.output().Append(goxml.CharData{Contents: n.Contents})
	case *goxml.Attribute:
		ctx.output().Append(goxml.CharData{Contents: n.Value})
	case goxml.Comment, goxml.ProcInst:
		// do nothing
	}
	return nil
}

// --------------------------------------------------------------------------
// Result serialization
// --------------------------------------------------------------------------

// SerializeResult converts a result document to an XML string.
func SerializeResult(doc *goxml.XMLDocument) string {
	return doc.ToXML()
}

// SerializeIndent converts a result document to an indented XML string.
func SerializeIndent(doc *goxml.XMLDocument, indentStr string) string {
	var sb strings.Builder
	nsPrinted := make(map[string]bool)
	for _, child := range doc.Children() {
		serializeIndentNode(&sb, child, 0, indentStr, nsPrinted)
	}
	return sb.String()
}

func serializeIndentNode(sb *strings.Builder, node goxml.XMLNode, depth int, indent string, nsPrinted map[string]bool) {
	switch n := node.(type) {
	case *goxml.Element:
		serializeIndentElement(sb, n, depth, indent, nsPrinted)
	case goxml.CharData:
		sb.WriteString(escapeText(n.Contents))
	case goxml.Comment:
		writeIndent(sb, depth, indent)
		sb.WriteString("<!--")
		sb.WriteString(n.Contents)
		sb.WriteString("-->\n")
	case goxml.ProcInst:
		writeIndent(sb, depth, indent)
		sb.WriteString("<?")
		sb.WriteString(n.Target)
		if len(n.Inst) > 0 {
			sb.WriteByte(' ')
			sb.Write(n.Inst)
		}
		sb.WriteString("?>\n")
	}
}

func serializeIndentElement(sb *strings.Builder, elt *goxml.Element, depth int, indent string, nsPrinted map[string]bool) {
	children := elt.Children()

	// Check if this element has only a single text child (inline it).
	textOnly := len(children) == 1 && isCharData(children[0])

	writeIndent(sb, depth, indent)
	sb.WriteByte('<')
	name := elt.Name
	if elt.Prefix != "" {
		name = elt.Prefix + ":" + name
	}
	sb.WriteString(name)

	// Collect namespace declarations that haven't been printed yet (sorted).
	type nsDecl struct{ attr, uri string }
	var nsDecls []nsDecl
	nsPrefixes := make([]string, 0, len(elt.Namespaces))
	for prefix := range elt.Namespaces {
		nsPrefixes = append(nsPrefixes, prefix)
	}
	sort.Strings(nsPrefixes)
	for _, prefix := range nsPrefixes {
		uri := elt.Namespaces[prefix]
		if _, ok := nsPrinted[uri]; !ok {
			nsPrinted[uri] = true
			nsAttr := "xmlns"
			if prefix != "" {
				nsAttr = "xmlns:" + prefix
			}
			nsDecls = append(nsDecls, nsDecl{nsAttr, uri})
		}
	}

	// Calculate total attribute length to decide single-line vs multi-line.
	attrs := elt.Attributes()
	totalLen := 0
	for _, ns := range nsDecls {
		totalLen += 1 + len(ns.attr) + 2 + len(ns.uri) + 1 // ' name="uri"'
	}
	for _, attr := range attrs {
		totalLen += 1 + len(attr.Name) + 2 + len(escapeAttr(attr.Value)) + 1
	}

	// If element tag (name + attrs) exceeds 80 chars, put each attr on its own line.
	const lineLimit = 80
	multiLine := totalLen > 0 && (len(name)+1+totalLen) > lineLimit
	attrIndent := ""
	if multiLine {
		// Indent to align under the first attribute (after "<name ").
		attrIndent = "\n" + strings.Repeat(" ", depth*len(indent)+1+len(name)+1)
	}

	first := true
	for _, ns := range nsDecls {
		if first || !multiLine {
			sb.WriteByte(' ')
			first = false
		} else {
			sb.WriteString(attrIndent)
		}
		sb.WriteString(ns.attr)
		sb.WriteString("=\"")
		sb.WriteString(escapeAttr(ns.uri))
		sb.WriteByte('"')
	}
	for _, attr := range attrs {
		if first || !multiLine {
			sb.WriteByte(' ')
			first = false
		} else {
			sb.WriteString(attrIndent)
		}
		sb.WriteString(attr.Name)
		sb.WriteString("=\"")
		sb.WriteString(escapeAttr(attr.Value))
		sb.WriteByte('"')
	}

	if len(children) == 0 {
		sb.WriteString(" />\n")
		return
	}
	sb.WriteByte('>')

	if textOnly {
		sb.WriteString(escapeText(children[0].(goxml.CharData).Contents))
		sb.WriteString("</")
		sb.WriteString(name)
		sb.WriteString(">\n")
		return
	}

	sb.WriteByte('\n')
	for _, child := range children {
		if cd, ok := child.(goxml.CharData); ok {
			if strings.TrimSpace(cd.Contents) == "" {
				continue // skip whitespace-only text between elements
			}
		}
		serializeIndentNode(sb, child, depth+1, indent, nsPrinted)
	}
	writeIndent(sb, depth, indent)
	sb.WriteString("</")
	sb.WriteString(name)
	sb.WriteString(">\n")
}

func isCharData(n goxml.XMLNode) bool {
	_, ok := n.(goxml.CharData)
	return ok
}

func writeIndent(sb *strings.Builder, depth int, indent string) {
	for i := 0; i < depth; i++ {
		sb.WriteString(indent)
	}
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}

func escapeAttr(s string) string {
	s = escapeText(s)
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
