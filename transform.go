package goxslt

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/ianaindex"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
)

const nsFN = "http://www.w3.org/2005/xpath-functions"

func init() {
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "regex-group",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    1,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			n, err := goxpath.NumberValue(args[0])
			if err != nil {
				return nil, err
			}
			idx := int(n)
			if ctx.Store == nil {
				return goxpath.Sequence{""}, nil
			}
			groups, ok := ctx.Store["regex-groups"].([]string)
			if !ok || idx < 0 || idx >= len(groups) {
				return goxpath.Sequence{""}, nil
			}
			return goxpath.Sequence{groups[idx]}, nil
		},
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "current",
		Namespace: nsFN,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			if item := ctx.CurrentItem(); item != nil {
				return goxpath.Sequence{item}, nil
			}
			return goxpath.Sequence{}, nil
		},
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "current-group",
		Namespace: nsFN,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			if ctx.Store == nil {
				return goxpath.Sequence{}, nil
			}
			if group, ok := ctx.Store["current-group"]; ok {
				if seq, ok := group.(goxpath.Sequence); ok {
					return seq, nil
				}
			}
			return goxpath.Sequence{}, nil
		},
		DynamicCallError: "XTDE1061",
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "current-grouping-key",
		Namespace: nsFN,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			if ctx.Store == nil {
				return goxpath.Sequence{""}, nil
			}
			if key, ok := ctx.Store["current-grouping-key"]; ok {
				if s, ok := key.(string); ok {
					return goxpath.Sequence{s}, nil
				}
			}
			return goxpath.Sequence{""}, nil
		},
		DynamicCallError: "XTDE1071",
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "generate-id",
		Namespace: nsFN,
		MaxArg:    1,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			var node any
			if len(args) == 0 || len(args[0]) == 0 {
				seq := ctx.GetContextSequence()
				if len(seq) == 0 {
					return goxpath.Sequence{""}, nil
				}
				node = seq[0]
			} else {
				node = args[0][0]
			}
			type hasID interface {
				GetID() int
			}
			if n, ok := node.(hasID); ok {
				return goxpath.Sequence{fmt.Sprintf("d%d", n.GetID())}, nil
			}
			return goxpath.Sequence{""}, nil
		},
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "system-property",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    1,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			name, err := goxpath.StringValue(args[0])
			if err != nil {
				return nil, err
			}
			switch name {
			case "xsl:version":
				return goxpath.Sequence{"3.0"}, nil
			case "xsl:vendor":
				return goxpath.Sequence{"speedata"}, nil
			case "xsl:vendor-url":
				return goxpath.Sequence{"https://github.com/speedata/goxslt"}, nil
			case "xsl:product-name":
				return goxpath.Sequence{"goxslt"}, nil
			case "xsl:product-version":
				return goxpath.Sequence{"1.0"}, nil
			case "xsl:is-schema-aware":
				return goxpath.Sequence{"no"}, nil
			case "xsl:supports-serialization":
				return goxpath.Sequence{"yes"}, nil
			case "xsl:supports-backwards-compatibility":
				return goxpath.Sequence{"no"}, nil
			case "xsl:supports-namespace-axis":
				return goxpath.Sequence{"no"}, nil
			case "xsl:supports-streaming":
				return goxpath.Sequence{"no"}, nil
			case "xsl:supports-dynamic-evaluation":
				return goxpath.Sequence{"no"}, nil
			case "xsl:xpath-version":
				return goxpath.Sequence{"3.1"}, nil
			case "xsl:xsd-version":
				return goxpath.Sequence{"1.1"}, nil
			default:
				return goxpath.Sequence{""}, nil
			}
		},
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "element-available",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    1,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			name, err := goxpath.StringValue(args[0])
			if err != nil {
				return nil, err
			}
			// Resolve QName and check if it's a known XSLT instruction.
			local := name
			ns := ""
			if before, after, ok := strings.Cut(name, ":"); ok {
				prefix := before
				local = after
				if uri, ok := ctx.Namespaces[prefix]; ok {
					ns = uri
				}
			}
			if ns != "http://www.w3.org/1999/XSL/Transform" {
				return goxpath.Sequence{false}, nil
			}
			switch local {
			case "apply-templates", "attribute", "call-template", "choose",
				"copy", "copy-of", "element", "fallback", "for-each",
				"for-each-group", "if", "message", "namespace",
				"number", "otherwise", "param", "processing-instruction",
				"result-document", "sequence", "sort", "template", "text",
				"value-of", "variable", "when", "with-param",
				"analyze-string", "matching-substring", "non-matching-substring",
				"comment", "document", "function", "import", "include",
				"key", "mode", "output", "source-document", "strip-space",
				"preserve-space", "stylesheet", "transform", "try", "catch",
				"fork", "where-populated", "on-empty", "on-non-empty", "map",
				"map-entry", "array":
				return goxpath.Sequence{true}, nil
			default:
				return goxpath.Sequence{false}, nil
			}
		},
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "function-available",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    2,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			name, err := goxpath.StringValue(args[0])
			if err != nil {
				return nil, err
			}
			// Resolve QName: split prefix:local and look up namespace.
			ns := nsFN
			local := name
			if before, after, ok := strings.Cut(name, ":"); ok {
				prefix := before
				local = after
				if uri, ok := ctx.Namespaces[prefix]; ok {
					ns = uri
				}
			}
			available := goxpath.FunctionExists(ns, local)
			return goxpath.Sequence{available}, nil
		},
	})
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "serialize",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    2,
		F: func(ctx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			// fn:serialize($arg as item()*, $params as element(output:serialization-parameters)?)
			// Simplified: serialize nodes to XML string.
			var sb strings.Builder
			for _, item := range args[0] {
				switch n := item.(type) {
				case *goxml.XMLDocument:
					sb.WriteString(n.ToXML())
				case *goxml.Element:
					sb.WriteString(n.ToXML())
				case goxml.CharData:
					sb.WriteString(n.Contents)
				case goxml.Comment:
					sb.WriteString("<!--")
					sb.WriteString(n.Contents)
					sb.WriteString("-->")
				case goxml.ProcInst:
					sb.WriteString("<?")
					sb.WriteString(n.Target)
					if len(n.Inst) > 0 {
						sb.WriteByte(' ')
						sb.Write(n.Inst)
					}
					sb.WriteString("?>")
				default:
					sb.WriteString(goxpath.ItemStringvalue(item))
				}
			}
			return goxpath.Sequence{sb.String()}, nil
		},
	})
}

// TransformResult holds the primary and any secondary result documents.
type TransformResult struct {
	Document           *goxml.XMLDocument            // primary output
	SecondaryDocuments map[string]*goxml.XMLDocument // href → document
	Output             OutputProperties              // xsl:output settings
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
	keyIndexCache      map[string]*keyIndex              // keyName → built index (lazy)
	documentCache      map[string]*goxml.XMLDocument     // absolute path → loaded document (for document())
	recursionDepth     int                               // current template recursion depth
	buildingKeyIndex   map[string]bool                   // guards against recursive key index building
	CurrentRule        *Rule                             // currently executing template rule (for xsl:next-match)
}

// keyIndex holds a pre-built index mapping string key values to matching nodes
// in document order.
type keyIndex struct {
	entries map[string][]goxml.XMLNode
}

// TransformOptions configures optional behavior for the transformation.
type TransformOptions struct {
	MessageHandler  func(text string, terminate bool) // callback for xsl:message
	MessageWriter   io.Writer                         // output for xsl:message (default os.Stderr)
	Parameters      map[string]goxpath.Sequence       // stylesheet parameters (overrides xsl:param defaults)
	InitialTemplate string                            // if set, call this named template instead of apply-templates on the document root
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

	// Set baseURI so that doc() can resolve relative paths.
	if ss.BasePath != "" {
		if xp.Ctx.Store == nil {
			xp.Ctx.Store = make(map[any]any)
		}
		xp.Ctx.Store["baseURI"] = filepath.Join(ss.BasePath, "stylesheet.xsl")
	}

	matchCtx := &MatchContext{Namespaces: make(map[string]string)}

	tc := &TransformContext{
		MatchCtx:           matchCtx,
		CurrentMode:        ss.DefaultMode,
		DefaultMode:        ss.DefaultMode,
		Modes:              ss.Modes,
		CurrentNode:        sourceDoc,
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
		// current() in pattern predicates returns the node being matched.
		tc.XPath.Ctx.SetCurrentItem(node)
		result, err := tc.XPath.Evaluate(expr)
		if err != nil {
			return false, err
		}
		return goxpath.BooleanValue(result)
	}
	matchCtx.XPathEvalSequence = func(expr string, node goxml.XMLNode) ([]any, error) {
		tc.XPath.Ctx.SetContextSequence(goxpath.Sequence{node})
		tc.XPath.Ctx.SetCurrentItem(node)
		result, err := tc.XPath.Evaluate(expr)
		if err != nil {
			return nil, err
		}
		items := make([]any, len(result))
		for i, v := range result {
			items[i] = v
		}
		return items, nil
	}

	// Propagate stylesheet namespace declarations to the XPath context
	// so that function prefixes can be resolved.
	maps.Copy(xp.Ctx.Namespaces, ss.Namespaces)

	// Register key() and document() functions BEFORE processing global
	// variables, because a global variable's select expression may call
	// these functions (e.g. document('') in key-066).
	tc.keyIndexCache = make(map[string]*keyIndex)
	tc.documentCache = make(map[string]*goxml.XMLDocument)
	tc.registerKeyFunction(ss)
	tc.registerDocumentFunction()
	tc.registerIDFunction()
	tc.registerUnparsedTextFunction()

	// Process global params and variables in declaration order so that
	// a param default can reference a preceding variable and vice versa.
	for _, decl := range ss.GlobalDecls {
		if decl.IsParam {
			p := decl.Param
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
		} else {
			v := decl.Var
			if err := v.Execute(tc); err != nil {
				return nil, fmt.Errorf("xsl:variable name='%s': %w", v.Name, err)
			}
		}
	}

	// Set up key lookup for match pattern key() support.
	if len(ss.Keys) > 0 {
		tc.keyIndexCache = make(map[string]*keyIndex)

		matchCtx.KeyLookup = func(keyName, valueExpr string, ns map[string]string) []goxml.XMLNode {
			idx, ok := tc.keyIndexCache[keyName]
			if !ok {
				idx = tc.buildKeyIndex(keyName)
				tc.keyIndexCache[keyName] = idx
			}
			// Evaluate valueExpr: string literal or XPath expression.
			trimmed := strings.TrimSpace(valueExpr)
			if len(trimmed) >= 2 && ((trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') || (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"')) {
				return idx.entries[trimmed[1:len(trimmed)-1]]
			}
			// XPath expression (e.g., $var) — evaluate it; may return a sequence.
			origNS := tc.XPath.Ctx.Namespaces
			if ns != nil {
				maps.Copy(tc.XPath.Ctx.Namespaces, ns)
			}
			result, err := tc.XPath.Evaluate(trimmed)
			tc.XPath.Ctx.Namespaces = origNS
			if err != nil {
				return nil
			}
			// Look up each value in the sequence separately.
			if len(result) <= 1 {
				return idx.entries[result.Stringvalue()]
			}
			seen := make(map[int]bool)
			var nodes []goxml.XMLNode
			for _, item := range result {
				for _, n := range idx.entries[goxpath.ItemStringvalue(item)] {
					if nid := n.GetID(); !seen[nid] {
						seen[nid] = true
						nodes = append(nodes, n)
					}
				}
			}
			return nodes
		}
	}

	// (key() and document() are registered earlier, before global variable processing)

	// Register stylesheet functions with the XPath evaluator.
	for _, fdef := range ss.Functions {
		goxpath.RegisterFunction(&goxpath.Function{
			Name:      fdef.LocalName,
			Namespace: fdef.Namespace,
			MinArg:    len(fdef.Params),
			MaxArg:    len(fdef.Params),
			F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
				tc.recursionDepth++
				if tc.recursionDepth > maxRecursionDepth {
					tc.recursionDepth--
					return nil, fmt.Errorf("XTDE0560: function recursion depth exceeds %d", maxRecursionDepth)
				}
				defer func() { tc.recursionDepth-- }()
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

	// Start transformation.
	if opts.InitialTemplate != "" {
		// Call the named template directly.
		tmpl, ok := ss.NamedTemplates[opts.InitialTemplate]
		if !ok {
			return nil, fmt.Errorf("initial-template: no template named '%s'", opts.InitialTemplate)
		}
		if err := tc.ExecuteTemplate(tmpl); err != nil {
			return nil, err
		}
	} else {
		// Default: apply-templates to the document node.
		if err := tc.ApplyTemplates(ss.DefaultMode, []goxml.XMLNode{sourceDoc}); err != nil {
			return nil, err
		}
	}

	return &TransformResult{
		Document:           resultDoc,
		SecondaryDocuments: tc.SecondaryDocuments,
		Output:             ss.Output,
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

const maxRecursionDepth = 10000

func (tc *TransformContext) ApplyTemplatesWithParams(mode *Mode, nodes []goxml.XMLNode, paramValues map[string]goxpath.Sequence) error {
	tc.recursionDepth++
	if tc.recursionDepth > maxRecursionDepth {
		tc.recursionDepth--
		return fmt.Errorf("XTDE0560: template recursion depth exceeds %d", maxRecursionDepth)
	}
	defer func() { tc.recursionDepth-- }()
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

			prevRule := tc.CurrentRule
			tc.CurrentRule = rule
			if err := tc.ExecuteTemplate(rule.Template); err != nil {
				tc.CurrentRule = prevRule
				tc.XPath.Ctx = origCtx
				return err
			}
			tc.CurrentRule = prevRule
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
		maps.Copy(elt.Namespaces, ctx.Stylesheet.ResultNamespaces)
	}
	for _, attr := range instr.Attributes {
		val, err := ctx.evalAVT(attr.Value)
		if err != nil {
			return fmt.Errorf("literal element %s attribute %s: %w", instr.Name, attr.Name, err)
		}
		attrName := xml.Name{Local: attr.Name, Space: attr.Namespace}
		elt.SetAttribute(xml.Attr{
			Name:  attrName,
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
	text := instr.Text
	if instr.TVT != nil {
		var err error
		text, err = ctx.evalAVT(*instr.TVT)
		if err != nil {
			return fmt.Errorf("text value template: %w", err)
		}
	}
	ctx.output().Append(goxml.CharData{Contents: text})
	return nil
}

func (instr *XSLText) Execute(ctx *TransformContext) error {
	text := instr.Text
	if instr.TVT != nil {
		var err error
		text, err = ctx.evalAVT(*instr.TVT)
		if err != nil {
			return fmt.Errorf("xsl:text value template: %w", err)
		}
	}
	ctx.output().Append(goxml.CharData{Contents: text})
	return nil
}

func (instr *XSLValueOf) Execute(ctx *TransformContext) error {
	var text string
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:value-of select='%s': %w", instr.Select, err)
		}
		text = result.StringvalueJoin(instr.Separator)
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
		if ctx.CurrentNode == nil {
			return nil
		}
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

func (instr *XSLNextMatch) Execute(ctx *TransformContext) error {
	if ctx.CurrentRule == nil || ctx.CurrentMode == nil {
		return nil
	}
	node := ctx.CurrentNode
	nextRule, err := ctx.CurrentMode.GetNextRule(node, ctx.MatchCtx, ctx.CurrentRule)
	if err != nil {
		return err
	}
	if nextRule == nil {
		// No next matching rule — apply built-in rules.
		if ctx.CurrentMode.BuiltInRules != nil {
			return ctx.CurrentMode.BuiltInRules.Process(node, ctx)
		}
		return nil
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

	// Create isolated variable scope.
	origCtx := ctx.XPath.Ctx
	newCtx := goxpath.CopyContext(origCtx)
	ctx.XPath.Ctx = newCtx

	for _, p := range nextRule.Template.Params {
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

	prevRule := ctx.CurrentRule
	ctx.CurrentRule = nextRule
	ctx.recursionDepth++
	if ctx.recursionDepth > maxRecursionDepth {
		ctx.recursionDepth--
		ctx.CurrentRule = prevRule
		ctx.XPath.Ctx = origCtx
		return fmt.Errorf("XTDE0560: template recursion depth exceeds %d", maxRecursionDepth)
	}
	err = ctx.ExecuteTemplate(nextRule.Template)
	ctx.recursionDepth--
	ctx.CurrentRule = prevRule
	ctx.XPath.Ctx = origCtx
	return err
}

func (instr *XSLApplyImports) Execute(ctx *TransformContext) error {
	if ctx.CurrentRule == nil || ctx.CurrentMode == nil {
		return nil
	}
	// apply-imports looks for rules with lower import precedence only.
	// For now we use the same logic as next-match.
	node := ctx.CurrentNode
	nextRule, err := ctx.CurrentMode.GetNextRule(node, ctx.MatchCtx, ctx.CurrentRule)
	if err != nil {
		return err
	}
	if nextRule == nil {
		if ctx.CurrentMode.BuiltInRules != nil {
			return ctx.CurrentMode.BuiltInRules.Process(node, ctx)
		}
		return nil
	}

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

	origCtx := ctx.XPath.Ctx
	newCtx := goxpath.CopyContext(origCtx)
	ctx.XPath.Ctx = newCtx

	for _, p := range nextRule.Template.Params {
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

	prevRule := ctx.CurrentRule
	ctx.CurrentRule = nextRule
	ctx.recursionDepth++
	if ctx.recursionDepth > maxRecursionDepth {
		ctx.recursionDepth--
		ctx.CurrentRule = prevRule
		ctx.XPath.Ctx = origCtx
		return fmt.Errorf("XTDE0560: template recursion depth exceeds %d", maxRecursionDepth)
	}
	err = ctx.ExecuteTemplate(nextRule.Template)
	ctx.recursionDepth--
	ctx.CurrentRule = prevRule
	ctx.XPath.Ctx = origCtx
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

	prevSize := ctx.XPath.Ctx.Size()
	prevPos := ctx.XPath.Ctx.Pos
	seqLen := len(result)

	for i, item := range result {
		prevNode := ctx.CurrentNode
		prevItem := ctx.CurrentItem

		if node, ok := item.(goxml.XMLNode); ok {
			ctx.CurrentNode = node
			ctx.CurrentItem = nil
		} else {
			ctx.CurrentItem = item
		}

		for _, child := range instr.Children {
			// Reset position/size before each child instruction,
			// because XPath evaluation may change ctx.size as a side effect.
			ctx.XPath.Ctx.Pos = i + 1
			ctx.XPath.Ctx.SetSize(seqLen)
			if err := child.Execute(ctx); err != nil {
				return err
			}
		}

		ctx.CurrentNode = prevNode
		ctx.CurrentItem = prevItem
	}
	ctx.XPath.Ctx.Pos = prevPos
	ctx.XPath.Ctx.SetSize(prevSize)
	return nil
}

// errIterateBreak is a sentinel error used by xsl:break to exit xsl:iterate.
var errIterateBreak = errors.New("xsl:break")

// errNextIteration is a sentinel used by xsl:next-iteration to signal
// that iteration should continue with updated parameters.
var errNextIteration = errors.New("xsl:next-iteration")

func (instr *XSLIterate) Execute(ctx *TransformContext) error {
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:iterate select='%s': %w", instr.Select, err)
	}

	// Create an isolated variable scope for the iteration parameters.
	origCtx := ctx.XPath.Ctx
	newCtx := goxpath.CopyContext(origCtx)
	ctx.XPath.Ctx = newCtx

	// Evaluate initial parameter values.
	for _, p := range instr.Params {
		val, evalErr := evalParamValue(ctx, p.Select, p.Children)
		if evalErr != nil {
			ctx.XPath.Ctx = origCtx
			return fmt.Errorf("xsl:iterate xsl:param name='%s': %w", p.Name, evalErr)
		}
		if p.As != nil {
			val, evalErr = coerceSequence(p.As, val)
			if evalErr != nil {
				ctx.XPath.Ctx = origCtx
				return fmt.Errorf("xsl:iterate xsl:param name='%s': %w", p.Name, evalErr)
			}
		}
		ctx.XPath.SetVariable(p.Name, val)
	}

	prevSize := origCtx.Size()
	prevPos := origCtx.Pos

	broken := false
	for i, item := range result {
		prevNode := ctx.CurrentNode
		prevItem := ctx.CurrentItem

		if node, ok := item.(goxml.XMLNode); ok {
			ctx.CurrentNode = node
			ctx.CurrentItem = nil
		} else {
			ctx.CurrentItem = item
		}
		ctx.XPath.Ctx.Pos = i + 1
		ctx.XPath.Ctx.SetSize(len(result))

		// Store the current iteration's next-iteration params here.
		if ctx.XPath.Ctx.Store == nil {
			ctx.XPath.Ctx.Store = make(map[any]any)
		}
		ctx.XPath.Ctx.Store["xsl:iterate:next-params"] = nil

		for _, child := range instr.Children {
			if childErr := child.Execute(ctx); childErr != nil {
				if errors.Is(childErr, errIterateBreak) {
					broken = true
					break
				}
				if errors.Is(childErr, errNextIteration) {
					break
				}
				ctx.CurrentNode = prevNode
				ctx.CurrentItem = prevItem
				ctx.XPath.Ctx = origCtx
				return childErr
			}
		}

		ctx.CurrentNode = prevNode
		ctx.CurrentItem = prevItem

		if broken {
			break
		}

		// Apply next-iteration parameter updates.
		if nextParams, ok := ctx.XPath.Ctx.Store["xsl:iterate:next-params"]; ok && nextParams != nil {
			paramMap := nextParams.(map[string]goxpath.Sequence)
			for name, val := range paramMap {
				ctx.XPath.SetVariable(name, val)
			}
		}
	}

	// If not broken, execute on-completion.
	// Per spec, there is no context item in xsl:on-completion.
	if !broken && len(instr.OnCompletion) > 0 {
		prevNode := ctx.CurrentNode
		ctx.CurrentNode = nil
		for _, child := range instr.OnCompletion {
			if childErr := child.Execute(ctx); childErr != nil {
				ctx.CurrentNode = prevNode
				ctx.XPath.Ctx = origCtx
				return childErr
			}
		}
		ctx.CurrentNode = prevNode
	}

	ctx.XPath.Ctx = origCtx
	origCtx.Pos = prevPos
	origCtx.SetSize(prevSize)
	return nil
}

func (instr *XSLNextIteration) Execute(ctx *TransformContext) error {
	// Evaluate all with-param values in the current context.
	paramMap := make(map[string]goxpath.Sequence)
	for _, wp := range instr.WithParams {
		val, err := evalParamValue(ctx, wp.Select, wp.Children)
		if err != nil {
			return fmt.Errorf("xsl:next-iteration xsl:with-param name='%s': %w", wp.Name, err)
		}
		paramMap[wp.Name] = val
	}
	ctx.XPath.Ctx.Store["xsl:iterate:next-params"] = paramMap
	return errNextIteration
}

func (instr *XSLBreak) Execute(ctx *TransformContext) error {
	// Execute any content children before breaking.
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return errIterateBreak
}

func (instr *XSLPerformSort) Execute(ctx *TransformContext) error {
	var nodes []goxml.XMLNode
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:perform-sort select='%s': %w", instr.Select, err)
		}
		nodes = sequenceToNodes(result)
	} else {
		// Sequence constructor: execute children, capture output.
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
		}
		ctx.popOutput()
		nodes = fragDoc.Children()
	}
	if err := ctx.sortNodes(nodes, instr.Sorts); err != nil {
		return err
	}
	for _, n := range nodes {
		ctx.output().Append(n)
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

	if instr.GroupStartingPat != nil {
		// group-starting-with: start a new group each time a node matches the pattern.
		for _, node := range nodes {
			if instr.GroupStartingPat.Matches(node, ctx.MatchCtx) || len(groups) == 0 {
				groups = append(groups, group{nodes: []goxml.XMLNode{node}})
			} else {
				groups[len(groups)-1].nodes = append(groups[len(groups)-1].nodes, node)
			}
		}
	} else if instr.GroupEndingPat != nil {
		// group-ending-with: current node ends the current group when it matches.
		for _, node := range nodes {
			if len(groups) == 0 {
				groups = append(groups, group{nodes: []goxml.XMLNode{node}})
			} else {
				groups[len(groups)-1].nodes = append(groups[len(groups)-1].nodes, node)
			}
			if instr.GroupEndingPat.Matches(node, ctx.MatchCtx) {
				// Start a new group after this node.
				groups = append(groups, group{})
			}
		}
		// Remove trailing empty group.
		if len(groups) > 0 && len(groups[len(groups)-1].nodes) == 0 {
			groups = groups[:len(groups)-1]
		}
	} else if instr.GroupAdjacent != "" {
		// group-adjacent: group consecutive nodes with the same key.
		var prevKey string
		for _, node := range nodes {
			prevNode := ctx.CurrentNode
			prevItem := ctx.CurrentItem
			ctx.CurrentNode = node
			ctx.CurrentItem = nil
			keyResult, err := ctx.evalXPath(instr.GroupAdjacent)
			ctx.CurrentNode = prevNode
			ctx.CurrentItem = prevItem
			if err != nil {
				return fmt.Errorf("xsl:for-each-group group-adjacent='%s': %w", instr.GroupAdjacent, err)
			}
			key := keyResult.Stringvalue()

			if len(groups) == 0 || key != prevKey {
				groups = append(groups, group{key: key, nodes: []goxml.XMLNode{node}})
			} else {
				groups[len(groups)-1].nodes = append(groups[len(groups)-1].nodes, node)
			}
			prevKey = key
		}
	} else {
		// group-by: group by evaluated key expression.
		keyIdx := make(map[string]int) // key -> index in groups
		for _, node := range nodes {
			prevNode := ctx.CurrentNode
			prevItem := ctx.CurrentItem
			ctx.CurrentNode = node
			ctx.CurrentItem = nil
			keyResult, err := ctx.evalXPath(instr.GroupBy)
			ctx.CurrentNode = prevNode
			ctx.CurrentItem = prevItem
			if err != nil {
				return fmt.Errorf("xsl:for-each-group group-by='%s': %w", instr.GroupBy, err)
			}
			key := keyResult.Stringvalue()

			if idx, ok := keyIdx[key]; ok {
				groups[idx].nodes = append(groups[idx].nodes, node)
			} else {
				keyIdx[key] = len(groups)
				groups = append(groups, group{key: key, nodes: []goxml.XMLNode{node}})
			}
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
			ctx.XPath.Ctx.Store = make(map[any]any)
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
			fragDoc := &goxml.XMLDocument{}
			ctx.pushOutput(fragDoc)
			if err := instr.Children[0].Execute(ctx); err != nil {
				ctx.popOutput()
				return err
			}
			ctx.popOutput()
			if instr.As != nil {
				// With "as" type constraint: return the children as a sequence
				// to be coerced, not the document wrapper (XSLT 2.0+ §9.4).
				for _, child := range fragDoc.Children() {
					seq = append(seq, child)
				}
			} else {
				// Without "as": value is the document node (temporary tree).
				seq = goxpath.Sequence{fragDoc}
			}
		}
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
		if instr.As != nil {
			// With "as" type constraint: return the children as a sequence.
			for _, child := range fragDoc.Children() {
				seq = append(seq, child)
			}
		} else {
			// Without "as": value is the document node (temporary tree).
			seq = goxpath.Sequence{fragDoc}
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
		case *goxml.XMLDocument:
			// copy-of a document node copies its children, not the document node itself.
			for _, child := range n.Children() {
				ctx.output().Append(child)
			}
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
		case *goxml.XMLDocument:
			for _, child := range n.Children() {
				ctx.output().Append(child)
			}
		case goxml.XMLNode:
			ctx.output().Append(n)
		default:
			ctx.output().Append(goxml.CharData{Contents: goxpath.ItemStringvalue(n)})
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
	if before, after, ok := strings.Cut(name, ":"); ok {
		elt.Prefix = before
		elt.Name = after
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
			if !instr.Terminate {
				// Per XSLT 3.0 spec, dynamic errors in non-terminating
				// xsl:message must not cause the transformation to fail.
				return nil
			}
			return fmt.Errorf("xsl:message select='%s': %w", instr.Select, err)
		}
		text = result.StringvalueJoin(" ")
	} else if len(instr.Children) > 0 {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				ctx.popOutput()
				if !instr.Terminate {
					return nil
				}
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
// XSLAnalyzeString
// --------------------------------------------------------------------------

func (instr *XSLAnalyzeString) Execute(ctx *TransformContext) error {
	// 1. Evaluate select → input string.
	result, err := ctx.evalXPath(instr.Select)
	if err != nil {
		return fmt.Errorf("xsl:analyze-string select='%s': %w", instr.Select, err)
	}
	input := result.Stringvalue()

	// 2. Evaluate regex AVT → pattern string.
	pattern, err := ctx.evalAVT(instr.Regex)
	if err != nil {
		return fmt.Errorf("xsl:analyze-string regex: %w", err)
	}

	// 3. Evaluate flags AVT → Go regex prefix.
	if len(instr.Flags.Parts) > 0 {
		flags, err := ctx.evalAVT(instr.Flags)
		if err != nil {
			return fmt.Errorf("xsl:analyze-string flags: %w", err)
		}
		var prefix strings.Builder
		for _, ch := range flags {
			switch ch {
			case 'i', 's', 'm':
				prefix.WriteString("(?")
				prefix.WriteRune(ch)
				prefix.WriteByte(')')
			case 'x':
				// Go's regexp doesn't support x flag; ignore.
			}
		}
		if prefix.Len() > 0 {
			pattern = prefix.String() + pattern
		}
	}

	// 4. Compile regex.
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("xsl:analyze-string regex='%s': %w", pattern, err)
	}

	// 5. Find all matches with subgroup positions.
	matches := re.FindAllStringSubmatchIndex(input, -1)

	// Save/restore Store for regex-groups.
	if ctx.XPath.Ctx.Store == nil {
		ctx.XPath.Ctx.Store = make(map[any]any)
	}
	prevGroups := ctx.XPath.Ctx.Store["regex-groups"]

	// 6. Build the sequence of segments (alternating non-matching and matching)
	// so we can assign correct position()/last() values.
	type segment struct {
		text     string
		matching bool
		match    []int // submatch indices (only for matching segments)
	}
	var segments []segment
	pos := 0
	for _, match := range matches {
		matchStart := match[0]
		matchEnd := match[1]
		if pos < matchStart {
			segments = append(segments, segment{text: input[pos:matchStart], matching: false})
		}
		segments = append(segments, segment{text: input[matchStart:matchEnd], matching: true, match: match})
		pos = matchEnd
	}
	if pos < len(input) {
		segments = append(segments, segment{text: input[pos:], matching: false})
	}

	prevSize := ctx.XPath.Ctx.Size()
	prevPos := ctx.XPath.Ctx.Pos
	totalSegments := len(segments)

	for segIdx, seg := range segments {
		ctx.XPath.Ctx.Pos = segIdx + 1
		ctx.XPath.Ctx.SetSize(totalSegments)

		if !seg.matching && len(instr.NonMatching) > 0 {
			prevNode := ctx.CurrentNode
			prevItem := ctx.CurrentItem
			ctx.CurrentItem = seg.text
			for _, child := range instr.NonMatching {
				if err := child.Execute(ctx); err != nil {
					ctx.CurrentNode = prevNode
					ctx.CurrentItem = prevItem
					ctx.XPath.Ctx.Store["regex-groups"] = prevGroups
					ctx.XPath.Ctx.Pos = prevPos
					ctx.XPath.Ctx.SetSize(prevSize)
					return err
				}
			}
			ctx.CurrentNode = prevNode
			ctx.CurrentItem = prevItem
		}

		if seg.matching && len(instr.Matching) > 0 {
			numGroups := len(seg.match)/2 - 1
			groups := make([]string, numGroups+1)
			groups[0] = seg.text
			for g := 1; g <= numGroups; g++ {
				start := seg.match[2*g]
				end := seg.match[2*g+1]
				if start >= 0 && end >= 0 {
					groups[g] = input[start:end]
				}
			}
			ctx.XPath.Ctx.Store["regex-groups"] = groups

			prevNode := ctx.CurrentNode
			prevItem := ctx.CurrentItem
			ctx.CurrentItem = seg.text
			for _, child := range instr.Matching {
				if err := child.Execute(ctx); err != nil {
					ctx.CurrentNode = prevNode
					ctx.CurrentItem = prevItem
					ctx.XPath.Ctx.Store["regex-groups"] = prevGroups
					ctx.XPath.Ctx.Pos = prevPos
					ctx.XPath.Ctx.SetSize(prevSize)
					return err
				}
			}
			ctx.CurrentNode = prevNode
			ctx.CurrentItem = prevItem
		}
	}

	ctx.XPath.Ctx.Pos = prevPos
	ctx.XPath.Ctx.SetSize(prevSize)

	// Restore previous regex-groups.
	ctx.XPath.Ctx.Store["regex-groups"] = prevGroups

	return nil
}

// --------------------------------------------------------------------------
// XSLNumber
// --------------------------------------------------------------------------

func (instr *XSLNumber) Execute(ctx *TransformContext) error {
	// Determine the source node for counting.
	sourceNode := ctx.CurrentNode
	if instr.Select == "" && sourceNode == nil {
		return fmt.Errorf("XTTE0990: xsl:number requires a context node")
	}
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

	// If href is empty (no href attribute), write to the primary output.
	if href == "" {
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				return err
			}
		}
		return nil
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

// Execute implements xsl:source-document by loading the document referenced
// by href and executing the children with that document as context node.
func (instr *XSLSourceDocument) Execute(ctx *TransformContext) error {
	href, err := ctx.evalAVT(instr.Href)
	if err != nil {
		return fmt.Errorf("xsl:source-document href: %w", err)
	}
	if href == "" {
		return fmt.Errorf("xsl:source-document: href is empty")
	}

	doc, err := ctx.loadDocument(href)
	if err != nil {
		return fmt.Errorf("xsl:source-document href='%s': %w", href, err)
	}

	// Save current context.
	prevNode := ctx.CurrentNode
	prevItem := ctx.CurrentItem
	prevPos := ctx.XPath.Ctx.Pos
	prevSize := ctx.XPath.Ctx.Size()
	prevSeq := ctx.XPath.Ctx.GetContextSequence()

	// Set loaded document as context.
	ctx.CurrentNode = doc
	ctx.CurrentItem = nil
	ctx.XPath.Ctx.SetContextSequence(goxpath.Sequence{doc})
	ctx.XPath.Ctx.Pos = 1
	ctx.XPath.Ctx.SetSize(1)

	// Execute children.
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			ctx.CurrentNode = prevNode
			ctx.CurrentItem = prevItem
			ctx.XPath.Ctx.SetContextSequence(prevSeq)
			ctx.XPath.Ctx.Pos = prevPos
			ctx.XPath.Ctx.SetSize(prevSize)
			return err
		}
	}

	// Restore context.
	ctx.CurrentNode = prevNode
	ctx.CurrentItem = prevItem
	ctx.XPath.Ctx.SetContextSequence(prevSeq)
	ctx.XPath.Ctx.Pos = prevPos
	ctx.XPath.Ctx.SetSize(prevSize)
	return nil
}

// Execute implements xsl:try by executing the try body and catching errors.
func (instr *XSLTry) Execute(ctx *TransformContext) error {
	var tryErr error
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			tryErr = err
		} else {
			text := result.Stringvalue()
			if text != "" {
				ctx.output().Append(goxml.CharData{Contents: text})
			}
		}
	} else {
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				tryErr = err
				break
			}
		}
	}

	if tryErr == nil {
		return nil
	}

	// Find matching catch clause.
	for _, catch := range instr.Catches {
		if catch.Errors == "*" || catchMatchesError(catch.Errors, tryErr) {
			return catch.execute(ctx)
		}
	}

	// No matching catch — propagate the error.
	return tryErr
}

func catchMatchesError(errors string, err error) bool {
	errMsg := err.Error()
	for code := range strings.FieldsSeq(errors) {
		if code == "*" || strings.Contains(errMsg, code) {
			return true
		}
	}
	return false
}

func (catch *XSLCatch) execute(ctx *TransformContext) error {
	if catch.Select != "" {
		result, err := ctx.evalXPath(catch.Select)
		if err != nil {
			return fmt.Errorf("xsl:catch select='%s': %w", catch.Select, err)
		}
		text := result.Stringvalue()
		if text != "" {
			ctx.output().Append(goxml.CharData{Contents: text})
		}
		return nil
	}
	for _, child := range catch.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Execute implements xsl:where-populated: execute children into a temporary
// document, include output only if non-empty.
func (instr *XSLWherePopulated) Execute(ctx *TransformContext) error {
	fragDoc := &goxml.XMLDocument{}
	ctx.pushOutput(fragDoc)
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			ctx.popOutput()
			return err
		}
	}
	ctx.popOutput()

	// Only append children if there is non-empty content.
	children := fragDoc.Children()
	if len(children) == 0 {
		return nil
	}
	for _, child := range children {
		ctx.output().Append(child)
	}
	return nil
}

// Execute implements xsl:on-empty. For now, this is a simplified version
// that just executes the children (proper empty-detection requires parent context).
func (instr *XSLOnEmpty) Execute(ctx *TransformContext) error {
	// Simplified: always execute. Full implementation would check
	// if the parent's output is empty.
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Execute implements xsl:on-non-empty. For now, this is a simplified version.
func (instr *XSLOnNonEmpty) Execute(ctx *TransformContext) error {
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Execute implements a generic sequence constructor.
func (instr *XSLSequenceConstructor) Execute(ctx *TransformContext) error {
	for _, child := range instr.Children {
		if err := child.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Execute implements xsl:namespace by adding a namespace declaration to the
// current output element.
func (instr *XSLNamespace) Execute(ctx *TransformContext) error {
	prefix, err := ctx.evalAVT(instr.Name)
	if err != nil {
		return fmt.Errorf("xsl:namespace name: %w", err)
	}

	var uri string
	if instr.Select != "" {
		result, err := ctx.evalXPath(instr.Select)
		if err != nil {
			return fmt.Errorf("xsl:namespace select='%s': %w", instr.Select, err)
		}
		uri = result.Stringvalue()
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
		uri = sb.String()
	}

	// Add namespace to current output element, or create a namespace node
	// if the output target is not an element (e.g. inside xsl:variable body).
	if elt, ok := ctx.output().(*goxml.Element); ok {
		if elt.Namespaces == nil {
			elt.Namespaces = make(map[string]string)
		}
		elt.Namespaces[prefix] = uri
	} else {
		ctx.output().Append(goxml.NamespaceNode{
			ID:     goxml.NewID(),
			Prefix: prefix,
			URI:    uri,
		})
	}
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
			// XSLT spec: sequence items in AVTs are space-separated.
			sb.WriteString(result.StringvalueJoin(" "))
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
	if ctx.CurrentNode == nil {
		return nil
	}
	switch n := ctx.CurrentNode.(type) {
	case *goxml.Element:
		elt := goxml.NewElement()
		elt.Name = n.Name
		elt.Prefix = n.Prefix
		maps.Copy(elt.Namespaces, n.Namespaces)
		ctx.output().Append(elt)
		ctx.pushOutput(elt)
		for _, child := range instr.Children {
			if err := child.Execute(ctx); err != nil {
				return err
			}
		}
		ctx.popOutput()
	case goxml.Attribute:
		if elt, ok := ctx.output().(*goxml.Element); ok {
			elt.SetAttribute(xml.Attr{
				Name:  xml.Name{Local: n.Name, Space: n.Namespace},
				Value: n.Value,
			})
		}
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

	output := ctx.output()
	switch o := output.(type) {
	case *goxml.Element:
		o.SetAttribute(xml.Attr{Name: xml.Name{Local: name}, Value: value})
	case *goxml.XMLDocument:
		// If the output is a document, try to find the last element child.
		for i := len(o.Children()) - 1; i >= 0; i-- {
			if elt, ok := o.Children()[i].(*goxml.Element); ok {
				elt.SetAttribute(xml.Attr{Name: xml.Name{Local: name}, Value: value})
				return nil
			}
		}
		// No element child: store the attribute as a standalone attribute node
		// in the fragment document. This supports xsl:variable as="attribute()".
		nsURI, _ := ctx.evalAVT(instr.Namespace)
		attr := &goxml.Attribute{Name: name, Value: value, Namespace: nsURI}
		o.Append(attr)
		return nil
	default:
		return fmt.Errorf("xsl:attribute '%s': output is not an element (%T)", name, output)
	}
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
	ctx.recursionDepth++
	if ctx.recursionDepth > maxRecursionDepth {
		ctx.recursionDepth--
		return fmt.Errorf("XTDE0560: call-template recursion depth exceeds %d", maxRecursionDepth)
	}
	defer func() { ctx.recursionDepth-- }()
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
			if p.As != nil {
				var coerceErr error
				val, coerceErr = coerceSequence(p.As, val)
				if coerceErr != nil {
					ctx.XPath.Ctx = origCtx
					return fmt.Errorf("xsl:param name='%s': %w", p.Name, coerceErr)
				}
			}
			ctx.XPath.SetVariable(p.Name, val)
		} else {
			val, err := evalParamValue(ctx, p.Select, p.Children)
			if err != nil {
				ctx.XPath.Ctx = origCtx
				return fmt.Errorf("xsl:param name='%s' default: %w", p.Name, err)
			}
			if p.As != nil {
				val, err = coerceSequence(p.As, val)
				if err != nil {
					ctx.XPath.Ctx = origCtx
					return fmt.Errorf("xsl:param name='%s': %w", p.Name, err)
				}
			}
			ctx.XPath.SetVariable(p.Name, val)
		}
	}

	// If the template has a declared return type (as attribute), capture
	// output in a fragment and validate the result type (XTTE0505).
	if tmpl.As != nil {
		fragDoc := &goxml.XMLDocument{}
		ctx.pushOutput(fragDoc)
		err := ctx.ExecuteTemplate(tmpl)
		ctx.popOutput()
		ctx.XPath.Ctx = origCtx
		if err != nil {
			return err
		}
		var seq goxpath.Sequence
		for _, child := range fragDoc.Children() {
			seq = append(seq, child)
		}
		seq, err = coerceSequence(tmpl.As, seq)
		if err != nil {
			return fmt.Errorf("XTTE0505: xsl:call-template name='%s': return type %w", instr.Name, err)
		}
		// Write the coerced result back to the actual output.
		for _, item := range seq {
			if node, ok := item.(goxml.XMLNode); ok {
				ctx.output().Append(node)
			} else {
				ctx.output().Append(goxml.CharData{Contents: fmt.Sprintf("%v", item)})
			}
		}
		return nil
	}

	err := ctx.ExecuteTemplate(tmpl)

	// Restore original context.
	ctx.XPath.Ctx = origCtx
	return err
}

// compareStrings performs a collation-aware string comparison.
// caseOrder can be "upper-first", "lower-first", or "" (default: Unicode codepoint order).
func compareStrings(a, b, caseOrder string) int {
	if caseOrder == "" {
		return strings.Compare(a, b)
	}

	// Compare case-insensitively first, then break ties by case.
	la := strings.ToLower(a)
	lb := strings.ToLower(b)
	cmp := strings.Compare(la, lb)
	if cmp != 0 {
		return cmp
	}

	// Same letters, different case: compare rune-by-rune for case ordering.
	ra := []rune(a)
	rb := []rune(b)
	minLen := min(len(rb), len(ra))
	for i := range minLen {
		if ra[i] == rb[i] {
			continue
		}
		aUpper := ra[i] >= 'A' && ra[i] <= 'Z'
		bUpper := rb[i] >= 'A' && rb[i] <= 'Z'
		if aUpper != bUpper {
			if caseOrder == "upper-first" {
				if aUpper {
					return -1
				}
				return 1
			}
			// lower-first
			if aUpper {
				return 1
			}
			return -1
		}
		// Both same case, compare normally.
		if ra[i] < rb[i] {
			return -1
		}
		return 1
	}
	return len(ra) - len(rb)
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

	// Resolve AVT values for sort keys (order, data-type, case-order).
	type resolvedSort struct {
		Select      string
		Order       string
		DataType    string
		CaseOrder   string
		AutoNumeric bool // true if data-type was not explicitly set (auto-detect from result type)
	}
	resolved := make([]resolvedSort, len(sorts))
	for j, sk := range sorts {
		order, err := tc.evalAVT(sk.Order)
		if err != nil {
			return fmt.Errorf("xsl:sort order: %w", err)
		}
		if order == "" {
			order = "ascending"
		}
		dataType, err := tc.evalAVT(sk.DataType)
		if err != nil {
			return fmt.Errorf("xsl:sort data-type: %w", err)
		}
		caseOrder, _ := tc.evalAVT(sk.CaseOrder)
		resolved[j] = resolvedSort{
			Select: sk.Select, Order: order, DataType: dataType,
			CaseOrder: caseOrder, AutoNumeric: !sk.DataTypeExplicit,
		}
	}

	// Pre-compute sort values for each node and each sort key.
	type sortVal struct {
		str       string
		num       float64
		isNumeric bool
	}
	vals := make([][]sortVal, len(nodes))
	origXPCtx := tc.XPath.Ctx
	seqLen := len(nodes)
	for i, node := range nodes {
		vals[i] = make([]sortVal, len(sorts))
		prevNode := tc.CurrentNode
		tc.CurrentNode = node
		for j, rs := range resolved {
			// Use a fresh XPath context for each evaluation to avoid state leaks.
			tc.XPath.Ctx = goxpath.CopyContext(origXPCtx)
			// Set position/size so position() and last() work in sort-key expressions.
			tc.XPath.Ctx.Pos = i + 1
			tc.XPath.Ctx.SetSize(seqLen)
			result, err := tc.evalXPath(rs.Select)
			if err != nil {
				tc.CurrentNode = prevNode
				tc.XPath.Ctx = origXPCtx
				return fmt.Errorf("xsl:sort select='%s': %w", rs.Select, err)
			}
			// Detect numeric values: if data-type is explicitly "number" or if
			// it was not set and the result is a numeric type, use numeric sorting.
			isNumeric := rs.DataType == "number"
			if !isNumeric && rs.AutoNumeric && len(result) == 1 {
				switch result[0].(type) {
				case float64, int, goxpath.XSDouble, goxpath.XSFloat, goxpath.XSInteger:
					isNumeric = true
				}
			}
			s := result.Stringvalue()
			vals[i][j].str = s
			vals[i][j].isNumeric = isNumeric
			if isNumeric {
				f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil {
					// Try direct numeric extraction from result.
					nv, nerr := goxpath.NumberValue(result)
					if nerr != nil {
						f = math.NaN()
					} else {
						f = nv
					}
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
		for j, rs := range resolved {
			va := vals[ia][j]
			vb := vals[ib][j]
			var cmp int
			if va.isNumeric || vb.isNumeric {
				aNaN := math.IsNaN(va.num)
				bNaN := math.IsNaN(vb.num)
				switch {
				case aNaN && bNaN:
					cmp = 0
				case aNaN:
					cmp = -1 // NaN sorts first (before all numbers)
				case bNaN:
					cmp = 1
				case va.num < vb.num:
					cmp = -1
				case va.num > vb.num:
					cmp = 1
				default:
					cmp = 0
				}
			} else {
				cmp = compareStrings(va.str, vb.str, rs.CaseOrder)
			}
			if rs.Order == "descending" {
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
		tc.XPath.Ctx.SetCurrentItem(tc.CurrentItem)
	} else {
		tc.XPath.Ctx.SetContextSequence(goxpath.Sequence{tc.CurrentNode})
		tc.XPath.Ctx.SetCurrentItem(tc.CurrentNode)
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
			nodes = append(nodes, goxml.CharData{Contents: goxpath.ItemStringvalue(v)})
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
	case goxml.Comment, goxml.ProcInst, goxml.NamespaceNode:
		// do nothing
	}
	return nil
}

// --------------------------------------------------------------------------
// ShallowCopyRuleSet — built-in rules for on-no-match="shallow-copy"
// --------------------------------------------------------------------------

type ShallowCopyRuleSet struct{}

func (b *ShallowCopyRuleSet) Process(node goxml.XMLNode, ctx *TransformContext) error {
	switch n := node.(type) {
	case *goxml.XMLDocument:
		return ctx.ApplyTemplates(ctx.CurrentMode, n.Children())
	case *goxml.Element:
		elt := goxml.NewElement()
		elt.Name = n.Name
		elt.Prefix = n.Prefix
		maps.Copy(elt.Namespaces, n.Namespaces)
		ctx.output().Append(elt)
		ctx.pushOutput(elt)
		// Apply templates to attributes and children.
		attrs := n.Attributes()
		children := n.Children()
		nodes := make([]goxml.XMLNode, 0, len(attrs)+len(children))
		for _, a := range attrs {
			nodes = append(nodes, a)
		}
		nodes = append(nodes, children...)
		err := ctx.ApplyTemplates(ctx.CurrentMode, nodes)
		ctx.popOutput()
		return err
	case goxml.CharData:
		ctx.output().Append(goxml.CharData{Contents: n.Contents})
	case *goxml.Attribute:
		if elt, ok := ctx.output().(*goxml.Element); ok {
			elt.SetAttribute(xml.Attr{
				Name:  xml.Name{Local: n.Name, Space: n.Namespace},
				Value: n.Value,
			})
		}
	case goxml.Comment:
		ctx.output().Append(goxml.Comment{Contents: n.Contents})
	case goxml.ProcInst:
		ctx.output().Append(goxml.ProcInst{Target: n.Target, Inst: n.Inst})
	case goxml.NamespaceNode:
		// Shallow-copy a namespace node: add the namespace declaration to the output element.
		if elt, ok := ctx.output().(*goxml.Element); ok {
			if elt.Namespaces == nil {
				elt.Namespaces = make(map[string]string)
			}
			elt.Namespaces[n.Prefix] = n.URI
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Key index helpers
// --------------------------------------------------------------------------

// buildKeyIndex builds the index for the named key by traversing the source
// document and evaluating the use expression for each matching node.
func (tc *TransformContext) buildKeyIndex(keyName string) *keyIndex {
	return tc.buildKeyIndexFrom(keyName, tc.SourceDoc)
}

// buildKeyIndexFrom builds the index for the named key by traversing the tree
// rooted at root and evaluating the use expression for each matching node.
func (tc *TransformContext) buildKeyIndexFrom(keyName string, root goxml.XMLNode) *keyIndex {
	// Guard against recursive key index building (e.g. key() in a match pattern
	// of an xsl:key triggers building the same index).
	if tc.buildingKeyIndex == nil {
		tc.buildingKeyIndex = make(map[string]bool)
	}
	if tc.buildingKeyIndex[keyName] {
		return &keyIndex{entries: make(map[string][]goxml.XMLNode)}
	}
	tc.buildingKeyIndex[keyName] = true
	defer func() { delete(tc.buildingKeyIndex, keyName) }()

	idx := &keyIndex{entries: make(map[string][]goxml.XMLNode)}
	// Track which (key-value, node-id) pairs have been added to avoid duplicates
	// when multiple xsl:key definitions with the same name match the same node.
	seen := make(map[string]map[int]bool)
	for _, kd := range tc.Stylesheet.Keys {
		if kd.Name != keyName {
			continue
		}
		walkNodes(root, func(node goxml.XMLNode) {
			if !kd.Match.Matches(node, tc.MatchCtx) {
				return
			}
			// Evaluate the use expression in the context of the matched node.
			prevNode := tc.CurrentNode
			tc.CurrentNode = node
			result, err := tc.evalXPath(kd.Use)
			tc.CurrentNode = prevNode
			if err != nil {
				return
			}
			// Each item in the result creates a separate index entry,
			// so a single node can be indexed under multiple keys.
			nid := node.GetID()
			for _, item := range result {
				key := goxpath.ItemStringvalue(item)
				if seen[key] == nil {
					seen[key] = make(map[int]bool)
				}
				if !seen[key][nid] {
					seen[key][nid] = true
					idx.entries[key] = append(idx.entries[key], node)
				}
			}
		})
	}
	return idx
}

// loadDocument loads an external XML document by URI, resolving relative paths
// against the stylesheet's base path. document(”) returns the stylesheet itself.
// Documents are cached so that repeated calls return the same instance.
func (tc *TransformContext) loadDocument(uri string) (*goxml.XMLDocument, error) {
	if uri == "" {
		// document('') returns the stylesheet document itself.
		if tc.Stylesheet.StylesheetDoc == nil {
			return nil, fmt.Errorf("document(''): stylesheet document not available")
		}
		return tc.Stylesheet.StylesheetDoc, nil
	}

	// Resolve relative URI against the stylesheet's base path.
	absPath := uri
	if !filepath.IsAbs(uri) && tc.Stylesheet.BasePath != "" {
		absPath = filepath.Join(tc.Stylesheet.BasePath, uri)
	}
	absPath, _ = filepath.Abs(absPath)

	// Return cached document if already loaded.
	if doc, ok := tc.documentCache[absPath]; ok {
		return doc, nil
	}

	// Parse the document.
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	doc, err := goxml.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", uri, err)
	}

	tc.documentCache[absPath] = doc
	return doc, nil
}

// registerKeyFunction registers the key() XPath function for this transformation.
func (tc *TransformContext) registerKeyFunction(ss *Stylesheet) {
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "key",
		Namespace: "http://www.w3.org/2005/xpath-functions",
		MinArg:    2,
		MaxArg:    3,
		F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			keyName := expandQName(args[0].Stringvalue(), xpCtx.Namespaces)

			// XTDE1260: check that a key with this name has been defined.
			keyExists := false
			for _, kd := range ss.Keys {
				if kd.Name == keyName {
					keyExists = true
					break
				}
			}
			if !keyExists {
				return nil, fmt.Errorf("XTDE1260: no key named '%s' has been defined", keyName)
			}

			var idx *keyIndex
			if len(args) >= 3 && len(args[2]) > 0 {
				// 3-arg form: build index from each subtree root in arg3.
				idx = &keyIndex{entries: make(map[string][]goxml.XMLNode)}
				for _, item := range args[2] {
					if node, ok := item.(goxml.XMLNode); ok {
						subIdx := tc.buildKeyIndexFrom(keyName, node)
						for k, nodes := range subIdx.entries {
							idx.entries[k] = append(idx.entries[k], nodes...)
						}
					}
				}
			} else {
				// 2-arg form: determine document root from context node.
				var docRoot *goxml.XMLDocument
				ctxSeq := xpCtx.GetContextSequence()
				if len(ctxSeq) > 0 {
					if node, ok := ctxSeq[0].(goxml.XMLNode); ok {
						if dr, ok := documentRoot(node).(*goxml.XMLDocument); ok {
							docRoot = dr
						}
					}
				}
				if docRoot == nil || docRoot == tc.SourceDoc {
					// Source document: use cache.
					var ok bool
					idx, ok = tc.keyIndexCache[keyName]
					if !ok {
						idx = tc.buildKeyIndex(keyName)
						tc.keyIndexCache[keyName] = idx
					}
				} else {
					// RTF or other document: build fresh index.
					idx = tc.buildKeyIndexFrom(keyName, docRoot)
				}
			}

			// Atomize: look up each item's string value separately,
			// collecting unique nodes in document order.
			seen := make(map[int]bool)
			var seq goxpath.Sequence
			for _, item := range args[1] {
				sv := goxpath.ItemStringvalue(item)
				for _, n := range idx.entries[sv] {
					nid := n.GetID()
					if !seen[nid] {
						seen[nid] = true
						seq = append(seq, n)
					}
				}
			}
			return seq, nil
		},
	})
}

// registerDocumentFunction registers the document() XPath function.
func (tc *TransformContext) registerDocumentFunction() {
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "document",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    1,
		F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			var seq goxpath.Sequence
			for _, item := range args[0] {
				uri := goxpath.ItemStringvalue(item)
				doc, err := tc.loadDocument(uri)
				if err != nil {
					return nil, fmt.Errorf("document('%s'): %w", uri, err)
				}
				seq = append(seq, doc)
			}
			if len(args[0]) == 0 {
				return goxpath.Sequence{}, nil
			}
			return seq, nil
		},
	})
}

// registerIDFunction registers the id() XPath function.
// id(value) returns elements with a matching xml:id attribute in the context document.
// id(value, node) returns elements in the document containing node.
func (tc *TransformContext) registerIDFunction() {
	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "id",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    2,
		F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			// Determine the document to search.
			var root goxml.XMLNode
			if len(args) >= 2 && len(args[1]) > 0 {
				root = args[1][0].(goxml.XMLNode)
			} else {
				// Use the context node's document.
				root = tc.SourceDoc
			}
			// Walk up to document root.
			root = documentRoot(root)
			// Collect IDREFS (space-separated list of IDs).
			var ids []string
			for _, item := range args[0] {
				sv := goxpath.ItemStringvalue(item)
				for id := range strings.FieldsSeq(sv) {
					ids = append(ids, id)
				}
			}
			// Search for elements with matching xml:id.
			var seq goxpath.Sequence
			seen := make(map[int]bool)
			walkNodes(root, func(node goxml.XMLNode) {
				elt, ok := node.(*goxml.Element)
				if !ok {
					return
				}
				for _, attr := range elt.Attributes() {
					if (attr.Name == "id" || attr.Name == "xml:id") && attr.Namespace == "http://www.w3.org/XML/1998/namespace" {
						if slices.Contains(ids, strings.TrimSpace(attr.Value)) {
							nid := elt.GetID()
							if !seen[nid] {
								seen[nid] = true
								seq = append(seq, elt)
							}
							return
						}
					}
				}
			})
			return seq, nil
		},
	})
}

// walkNodes calls fn for every node in the document tree (depth-first).
func walkNodes(node goxml.XMLNode, fn func(goxml.XMLNode)) {
	fn(node)
	// Also visit attributes so that match="@foo" patterns work in xsl:key.
	if elt, ok := node.(*goxml.Element); ok {
		for _, attr := range elt.Attributes() {
			fn(attr)
		}
	}
	for _, child := range node.Children() {
		walkNodes(child, fn)
	}
}

// registerUnparsedTextFunction registers the XSLT-only unparsed-text() and
// unparsed-text-available() functions. These resolve relative URIs against
// the stylesheet base path.
func (tc *TransformContext) registerUnparsedTextFunction() {
	basePath := tc.Stylesheet.BasePath

	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "unparsed-text",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    2,
		F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			href, err := goxpath.StringValue(args[0])
			if err != nil {
				return nil, fmt.Errorf("unparsed-text: %w", err)
			}
			if href == "" {
				return goxpath.Sequence{""}, nil
			}
			resolvedPath := href
			if !filepath.IsAbs(href) && basePath != "" {
				resolvedPath = filepath.Join(basePath, href)
			}
			data, err := os.ReadFile(resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("unparsed-text: %w", err)
			}
			// Decode from the specified encoding (default: UTF-8).
			if len(args) >= 2 {
				encName, _ := goxpath.StringValue(args[1])
				if encName != "" && !strings.EqualFold(encName, "utf-8") && !strings.EqualFold(encName, "utf8") {
					enc, encErr := ianaindex.IANA.Encoding(encName)
					if encErr != nil || enc == nil {
						return nil, fmt.Errorf("unparsed-text: unsupported encoding %q", encName)
					}
					decoded, decErr := enc.NewDecoder().Bytes(data)
					if decErr != nil {
						return nil, fmt.Errorf("unparsed-text: decoding %q: %w", encName, decErr)
					}
					data = decoded
				}
			}
			return goxpath.Sequence{string(data)}, nil
		},
	})

	goxpath.RegisterFunction(&goxpath.Function{
		Name:      "unparsed-text-available",
		Namespace: nsFN,
		MinArg:    1,
		MaxArg:    2,
		F: func(xpCtx *goxpath.Context, args []goxpath.Sequence) (goxpath.Sequence, error) {
			href, err := goxpath.StringValue(args[0])
			if err != nil {
				return goxpath.Sequence{false}, nil
			}
			if href == "" {
				return goxpath.Sequence{false}, nil
			}
			resolvedPath := href
			if !filepath.IsAbs(href) && basePath != "" {
				resolvedPath = filepath.Join(basePath, href)
			}
			_, err = os.Stat(resolvedPath)
			return goxpath.Sequence{err == nil}, nil
		},
	})
}

// --------------------------------------------------------------------------
// Result serialization
// --------------------------------------------------------------------------

// SerializeResult converts a result document to an XML string.
func SerializeResult(doc *goxml.XMLDocument) string {
	return doc.ToXML()
}

// SerializeWithOutput converts a result document to a string using the given output properties.
func SerializeWithOutput(doc *goxml.XMLDocument, output OutputProperties) string {
	var sb strings.Builder
	if output.Method != "text" && !output.OmitXMLDeclaration {
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
		if output.Indent {
			sb.WriteByte('\n')
		}
	}
	if output.Indent {
		nsPrinted := make(map[string]bool)
		for _, child := range doc.Children() {
			serializeIndentNode(&sb, child, 0, "  ", nsPrinted)
		}
	} else {
		sb.WriteString(doc.ToXML())
	}
	return sb.String()
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
	for range depth {
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
