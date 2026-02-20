package goxslt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/speedata/goxml"
)

const xslNS = "http://www.w3.org/1999/XSL/Transform"

// compileContext holds state during stylesheet compilation, including
// import precedence tracking, cycle detection, and base path for href resolution.
type compileContext struct {
	ss         *Stylesheet
	seq        int
	precedence int             // current import precedence level
	visited    map[string]bool // absolute paths → cycle detection
	basePath   string          // directory for resolving relative hrefs
}

// Compile parses an XSLT stylesheet (already parsed as a goxml document) and
// returns a Stylesheet ready for transformation.
// Import/include directives cannot be resolved without a file path; use
// CompileFile for stylesheets that use xsl:import or xsl:include.
func Compile(xsltDoc *goxml.XMLDocument) (*Stylesheet, error) {
	ss := &Stylesheet{
		DefaultMode:    NewMode("", &TextOnlyCopyRuleSet{}),
		Modes:          make(map[string]*Mode),
		Output:         OutputProperties{Method: "xml"},
		NamedTemplates: make(map[string]*TemplateBody),
		Functions:      make(map[string]*FunctionDef),
		Namespaces:     make(map[string]string),
	}
	ss.Modes[""] = ss.DefaultMode

	cc := &compileContext{
		ss:         ss,
		precedence: 1,
		visited:    make(map[string]bool),
	}

	if err := cc.compileStylesheet(xsltDoc); err != nil {
		return nil, err
	}

	ss.DefaultMode.ComputeRankings()
	for _, m := range ss.Modes {
		m.ComputeRankings()
	}

	return ss, nil
}

// CompileFile compiles an XSLT stylesheet from a file path, enabling
// xsl:import and xsl:include resolution via relative href attributes.
func CompileFile(xsltPath string) (*Stylesheet, error) {
	absPath, err := filepath.Abs(xsltPath)
	if err != nil {
		return nil, fmt.Errorf("XSLT: cannot resolve path %q: %w", xsltPath, err)
	}

	xsltDoc, err := parseXMLFile(absPath)
	if err != nil {
		return nil, err
	}

	ss := &Stylesheet{
		DefaultMode:    NewMode("", &TextOnlyCopyRuleSet{}),
		Modes:          make(map[string]*Mode),
		Output:         OutputProperties{Method: "xml"},
		NamedTemplates: make(map[string]*TemplateBody),
		Functions:      make(map[string]*FunctionDef),
		Namespaces:     make(map[string]string),
	}
	ss.Modes[""] = ss.DefaultMode

	cc := &compileContext{
		ss:         ss,
		precedence: 1,
		visited:    map[string]bool{absPath: true},
		basePath:   filepath.Dir(absPath),
	}

	if err := cc.compileStylesheet(xsltDoc); err != nil {
		return nil, err
	}

	ss.DefaultMode.ComputeRankings()
	for _, m := range ss.Modes {
		m.ComputeRankings()
	}

	return ss, nil
}

// parseXMLFile opens and parses an XML file.
func parseXMLFile(path string) (*goxml.XMLDocument, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("XSLT: %w", err)
	}
	defer f.Close()
	doc, err := goxml.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing %s: %w", path, err)
	}
	return doc, nil
}

// resolveHref resolves a relative href against the compile context's base path.
func (cc *compileContext) resolveHref(href string) (string, error) {
	if cc.basePath == "" {
		return "", fmt.Errorf("XSLT: xsl:import/include href=%q: no base path (use CompileFile instead of Compile)", href)
	}
	return filepath.Join(cc.basePath, href), nil
}

// compileStylesheet compiles a single stylesheet document into cc.ss.
func (cc *compileContext) compileStylesheet(xsltDoc *goxml.XMLDocument) error {
	root, err := xsltDoc.Root()
	if err != nil {
		return fmt.Errorf("XSLT: %w", err)
	}

	// Verify root is xsl:stylesheet or xsl:transform
	rootNS := root.Namespaces[root.Prefix]
	if rootNS != xslNS {
		return fmt.Errorf("XSLT: root element is not xsl:stylesheet (namespace: %s)", rootNS)
	}
	if root.Name != "stylesheet" && root.Name != "transform" {
		return fmt.Errorf("XSLT: root element is %s, expected stylesheet or transform", root.Name)
	}

	// Copy namespace declarations from the root element so that
	// functions and XPath expressions can resolve prefixes.
	for prefix, uri := range root.Namespaces {
		cc.ss.Namespaces[prefix] = uri
	}

	// Build result namespaces: all stylesheet namespaces except XSL namespace
	// and those listed in exclude-result-prefixes.
	excluded := make(map[string]bool)
	if erp := attrValue(root, "exclude-result-prefixes"); erp != "" {
		if erp == "#all" {
			for prefix := range root.Namespaces {
				excluded[prefix] = true
			}
		} else {
			for _, p := range strings.Fields(erp) {
				if p == "#default" {
					excluded[""] = true
				} else {
					excluded[p] = true
				}
			}
		}
	}
	cc.ss.ResultNamespaces = make(map[string]string)
	for prefix, uri := range root.Namespaces {
		if uri == xslNS {
			continue
		}
		if excluded[prefix] {
			continue
		}
		cc.ss.ResultNamespaces[prefix] = uri
	}

	children := root.Children()

	// Phase 1: process xsl:import elements (must come before all other top-level elements).
	for _, child := range children {
		elt, ok := child.(*goxml.Element)
		if !ok {
			continue
		}
		eltNS := elt.Namespaces[elt.Prefix]
		if eltNS != xslNS {
			break // non-XSL element → imports section is over
		}
		if elt.Name != "import" {
			break // first non-import XSL element → imports section is over
		}
		if err := cc.processImport(elt); err != nil {
			return err
		}
	}

	// Phase 2: process all other top-level elements.
	for _, child := range children {
		elt, ok := child.(*goxml.Element)
		if !ok {
			continue
		}
		eltNS := elt.Namespaces[elt.Prefix]
		if eltNS != xslNS {
			continue
		}

		switch elt.Name {
		case "import":
			// Already processed in phase 1.
			continue
		case "include":
			if err := cc.processInclude(elt); err != nil {
				return err
			}
		case "template":
			if err := cc.compileTemplate(elt); err != nil {
				return err
			}
		case "output":
			if v := attrValue(elt, "method"); v != "" {
				cc.ss.Output.Method = v
			}
			if v := attrValue(elt, "indent"); v == "yes" {
				cc.ss.Output.Indent = true
			}
			if v := attrValue(elt, "version"); v != "" {
				cc.ss.Output.Version = v
			}
		case "function":
			if err := cc.compileFunction(elt); err != nil {
				return err
			}
		case "param":
			name := attrValue(elt, "name")
			if name == "" {
				return fmt.Errorf("XSLT: xsl:param missing name attribute (line %d)", elt.Line)
			}
			sel := attrValue(elt, "select")
			var children []Instruction
			if sel == "" {
				var err error
				children, err = compileChildren(elt, cc.ss.Namespaces)
				if err != nil {
					return err
				}
			}
			var asType *SequenceType
			if asStr := attrValue(elt, "as"); asStr != "" {
				var err error
				asType, err = parseSequenceType(asStr)
				if err != nil {
					return fmt.Errorf("XSLT: xsl:param name='%s' as='%s': %w", name, asStr, err)
				}
			}
			cc.ss.GlobalParams = append(cc.ss.GlobalParams, TemplateParam{Name: name, Select: sel, Children: children, As: asType})
		case "variable":
			v, err := compileVariable(elt)
			if err != nil {
				return err
			}
			cc.ss.GlobalVars = append(cc.ss.GlobalVars, *v)
		}
	}

	return nil
}

// processImport handles an xsl:import element. The imported stylesheet gets
// a lower precedence than the importing one.
func (cc *compileContext) processImport(elt *goxml.Element) error {
	href := attrValue(elt, "href")
	if href == "" {
		return fmt.Errorf("XSLT: xsl:import missing href attribute (line %d)", elt.Line)
	}

	absPath, err := cc.resolveHref(href)
	if err != nil {
		return err
	}

	if cc.visited[absPath] {
		return fmt.Errorf("XSLT: circular import/include detected: %s", absPath)
	}
	cc.visited[absPath] = true

	doc, err := parseXMLFile(absPath)
	if err != nil {
		return err
	}

	// Save current state.
	savedPrecedence := cc.precedence
	savedBasePath := cc.basePath

	// Imported stylesheet gets the current (lower) precedence.
	cc.basePath = filepath.Dir(absPath)
	if err := cc.compileStylesheet(doc); err != nil {
		return fmt.Errorf("XSLT: in imported %s: %w", href, err)
	}

	// After import: bump precedence so the importing stylesheet gets a higher one.
	cc.precedence = savedPrecedence + 1
	cc.basePath = savedBasePath
	return nil
}

// processInclude handles an xsl:include element. The included stylesheet gets
// the same precedence as the including one.
func (cc *compileContext) processInclude(elt *goxml.Element) error {
	href := attrValue(elt, "href")
	if href == "" {
		return fmt.Errorf("XSLT: xsl:include missing href attribute (line %d)", elt.Line)
	}

	absPath, err := cc.resolveHref(href)
	if err != nil {
		return err
	}

	if cc.visited[absPath] {
		return fmt.Errorf("XSLT: circular import/include detected: %s", absPath)
	}
	cc.visited[absPath] = true

	doc, err := parseXMLFile(absPath)
	if err != nil {
		return err
	}

	// Save and restore basePath; precedence stays the same.
	savedBasePath := cc.basePath
	cc.basePath = filepath.Dir(absPath)
	if err := cc.compileStylesheet(doc); err != nil {
		return fmt.Errorf("XSLT: in included %s: %w", href, err)
	}
	cc.basePath = savedBasePath
	return nil
}

// OutputProperties holds xsl:output settings.
type OutputProperties struct {
	Method  string // "xml", "html", "text"
	Indent  bool
	Version string
}

// FunctionDef holds a compiled xsl:function definition.
type FunctionDef struct {
	Namespace string          // resolved namespace URI
	LocalName string          // function name without prefix
	Params    []TemplateParam // positional parameters
	Body      *TemplateBody   // compiled body
	As        *SequenceType   // optional return type declaration
}

// Stylesheet holds the compiled XSLT stylesheet.
type Stylesheet struct {
	DefaultMode      *Mode
	Modes            map[string]*Mode
	Output           OutputProperties
	NamedTemplates   map[string]*TemplateBody
	Functions        map[string]*FunctionDef // key = "namespace localname"
	Namespaces       map[string]string       // prefix → URI from root element
	ResultNamespaces map[string]string       // prefix → URI to propagate to output elements
	GlobalParams     []TemplateParam         // top-level xsl:param declarations
	GlobalVars       []XSLVariable           // top-level xsl:variable declarations
}

func (cc *compileContext) compileTemplate(elt *goxml.Element) error {
	matchAttr := attrValue(elt, "match")
	nameAttr := attrValue(elt, "name")
	modeAttr := attrValue(elt, "mode")

	if matchAttr == "" && nameAttr == "" {
		return fmt.Errorf("XSLT: xsl:template has neither match nor name attribute (line %d)", elt.Line)
	}

	body, err := compileTemplateBody(elt, cc.ss.Namespaces)
	if err != nil {
		return err
	}
	body.Name = nameAttr

	// Register named template (higher precedence wins).
	if nameAttr != "" {
		cc.ss.NamedTemplates[nameAttr] = body
	}

	if matchAttr != "" {
		pattern, err := parseMatchPattern(matchAttr, cc.ss.Namespaces)
		if err != nil {
			return fmt.Errorf("XSLT: error parsing match='%s': %w", matchAttr, err)
		}

		priority := pattern.DefaultPriority()
		if p := attrValue(elt, "priority"); p != "" {
			if _, err := fmt.Sscanf(p, "%f", &priority); err != nil {
				return fmt.Errorf("XSLT: invalid priority '%s'", p)
			}
		}

		mode := cc.ss.DefaultMode
		if modeAttr != "" {
			var ok bool
			mode, ok = cc.ss.Modes[modeAttr]
			if !ok {
				mode = NewMode(modeAttr, &TextOnlyCopyRuleSet{})
				cc.ss.Modes[modeAttr] = mode
			}
		}

		// Handle union patterns: split "a | b" into separate rules.
		if up, ok := pattern.(*UnionPattern); ok {
			for i, sub := range up.Patterns {
				mode.AddRule(sub, body, cc.precedence, sub.DefaultPriority(), cc.seq, i)
			}
		} else {
			mode.AddRule(pattern, body, cc.precedence, priority, cc.seq, 0)
		}
		cc.seq++
	}

	return nil
}

// compileChildren compiles the children of an XSLT element into instructions.
func compileChildren(parent *goxml.Element, namespaces ...map[string]string) ([]Instruction, error) {
	var ns map[string]string
	if len(namespaces) > 0 {
		ns = namespaces[0]
	}
	var instructions []Instruction

	for _, child := range parent.Children() {
		switch n := child.(type) {
		case goxml.CharData:
			text := n.Contents
			// Skip whitespace-only text nodes between XSLT instructions.
			if strings.TrimSpace(text) == "" {
				continue
			}
			instructions = append(instructions, &LiteralText{Text: text})

		case *goxml.Element:
			childNS := n.Namespaces[n.Prefix]
			if childNS == xslNS {
				instr, err := compileXSLInstruction(n, ns)
				if err != nil {
					return nil, err
				}
				if instr != nil {
					instructions = append(instructions, instr)
				}
			} else {
				// Literal result element.
				instr, err := compileLiteralElement(n, ns)
				if err != nil {
					return nil, err
				}
				instructions = append(instructions, instr)
			}
		}
	}

	return instructions, nil
}

func compileXSLInstruction(elt *goxml.Element, namespaces map[string]string) (Instruction, error) {
	switch elt.Name {
	case "apply-templates":
		sel := attrValue(elt, "select")
		mode := attrValue(elt, "mode")
		sorts, err := extractSorts(elt)
		if err != nil {
			return nil, err
		}
		var withParams []XSLWithParam
		for _, child := range elt.Children() {
			childElt, ok := child.(*goxml.Element)
			if !ok {
				continue
			}
			childNS := childElt.Namespaces[childElt.Prefix]
			if childNS != xslNS || childElt.Name != "with-param" {
				continue
			}
			pname := attrValue(childElt, "name")
			if pname == "" {
				return nil, fmt.Errorf("XSLT: xsl:with-param missing name attribute (line %d)", childElt.Line)
			}
			wpsel := attrValue(childElt, "select")
			var children []Instruction
			if wpsel == "" {
				var err error
				children, err = compileChildren(childElt, namespaces)
				if err != nil {
					return nil, err
				}
			}
			withParams = append(withParams, XSLWithParam{Name: pname, Select: wpsel, Children: children})
		}
		return &XSLApplyTemplates{Select: sel, Mode: mode, Sorts: sorts, WithParams: withParams}, nil

	case "value-of":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:value-of missing select attribute (line %d)", elt.Line)
		}
		return &XSLValueOf{Select: sel}, nil

	case "for-each":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:for-each missing select attribute (line %d)", elt.Line)
		}
		sorts, err := extractSorts(elt)
		if err != nil {
			return nil, err
		}
		children, err := compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLForEach{Select: sel, Sorts: sorts, Children: children}, nil

	case "if":
		test := attrValue(elt, "test")
		if test == "" {
			return nil, fmt.Errorf("XSLT: xsl:if missing test attribute (line %d)", elt.Line)
		}
		children, err := compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLIf{Test: test, Children: children}, nil

	case "text":
		var sb strings.Builder
		for _, child := range elt.Children() {
			if cd, ok := child.(goxml.CharData); ok {
				sb.WriteString(cd.Contents)
			}
		}
		return &XSLText{Text: sb.String()}, nil

	case "copy-of":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:copy-of missing select attribute (line %d)", elt.Line)
		}
		return &XSLCopyOf{Select: sel}, nil

	case "choose":
		return compileChoose(elt, namespaces)

	case "variable", "param":
		return compileVariable(elt)

	case "copy":
		children, err := compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLCopy{Children: children}, nil

	case "attribute":
		return compileAttribute(elt, namespaces)

	case "call-template":
		return compileCallTemplate(elt, namespaces)

	case "sequence":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:sequence missing select attribute (line %d)", elt.Line)
		}
		return &XSLSequence{Select: sel}, nil

	case "element":
		return compileElement(elt, namespaces)

	case "comment":
		return compileComment(elt, namespaces)

	case "processing-instruction":
		return compileProcessingInstruction(elt, namespaces)

	case "message":
		return compileMessage(elt, namespaces)

	case "number":
		return compileNumber(elt, namespaces)

	case "map":
		children, err := compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLMap{Children: children}, nil

	case "map-entry":
		key := attrValue(elt, "key")
		if key == "" {
			return nil, fmt.Errorf("XSLT: xsl:map-entry missing key attribute (line %d)", elt.Line)
		}
		sel := attrValue(elt, "select")
		var children []Instruction
		if sel == "" {
			var err error
			children, err = compileChildren(elt, namespaces)
			if err != nil {
				return nil, err
			}
		}
		return &XSLMapEntry{Key: key, Select: sel, Children: children}, nil

	case "for-each-group":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:for-each-group missing select attribute (line %d)", elt.Line)
		}
		groupBy := attrValue(elt, "group-by")
		if groupBy == "" {
			return nil, fmt.Errorf("XSLT: xsl:for-each-group missing group-by attribute (line %d)", elt.Line)
		}
		sorts, err := extractSorts(elt)
		if err != nil {
			return nil, err
		}
		children, err := compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLForEachGroup{Select: sel, GroupBy: groupBy, Sorts: sorts, Children: children}, nil

	case "result-document":
		return compileResultDocument(elt, namespaces)

	case "sort":
		// xsl:sort is handled by the parent (for-each / apply-templates).
		return nil, nil

	case "with-param":
		// xsl:with-param is handled by the parent (call-template).
		return nil, nil

	default:
		return nil, fmt.Errorf("XSLT: unsupported instruction xsl:%s (line %d)", elt.Name, elt.Line)
	}
}

func compileChoose(elt *goxml.Element, namespaces map[string]string) (*XSLChoose, error) {
	choose := &XSLChoose{}
	for _, child := range elt.Children() {
		childElt, ok := child.(*goxml.Element)
		if !ok {
			continue
		}
		childNS := childElt.Namespaces[childElt.Prefix]
		if childNS != xslNS {
			continue
		}
		switch childElt.Name {
		case "when":
			test := attrValue(childElt, "test")
			if test == "" {
				return nil, fmt.Errorf("XSLT: xsl:when missing test attribute (line %d)", childElt.Line)
			}
			children, err := compileChildren(childElt, namespaces)
			if err != nil {
				return nil, err
			}
			choose.When = append(choose.When, XSLWhen{Test: test, Children: children})
		case "otherwise":
			children, err := compileChildren(childElt, namespaces)
			if err != nil {
				return nil, err
			}
			choose.Otherwise = children
		}
	}
	if len(choose.When) == 0 {
		return nil, fmt.Errorf("XSLT: xsl:choose must have at least one xsl:when (line %d)", elt.Line)
	}
	return choose, nil
}

func compileVariable(elt *goxml.Element) (*XSLVariable, error) {
	name := attrValue(elt, "name")
	if name == "" {
		return nil, fmt.Errorf("XSLT: xsl:variable missing name attribute (line %d)", elt.Line)
	}
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		var err error
		children, err = compileChildren(elt)
		if err != nil {
			return nil, err
		}
	}
	var asType *SequenceType
	if asStr := attrValue(elt, "as"); asStr != "" {
		var err error
		asType, err = parseSequenceType(asStr)
		if err != nil {
			return nil, fmt.Errorf("XSLT: xsl:variable name='%s' as='%s': %w", name, asStr, err)
		}
	}
	return &XSLVariable{Name: name, Select: sel, Children: children, As: asType}, nil
}

func compileLiteralElement(elt *goxml.Element, namespaces ...map[string]string) (*LiteralElement, error) {
	var ns map[string]string
	if len(namespaces) > 0 {
		ns = namespaces[0]
	}
	var attrs []LiteralAttribute
	for _, attr := range elt.Attributes() {
		// Skip xmlns declarations.
		if attr.Name == "xmlns" || strings.HasPrefix(attr.Name, "xmlns:") {
			continue
		}
		avt, err := parseAVT(attr.Value)
		if err != nil {
			return nil, fmt.Errorf("XSLT: error parsing AVT in attribute %s: %w", attr.Name, err)
		}
		attrs = append(attrs, LiteralAttribute{Name: attr.Name, Value: avt})
	}

	children, err := compileChildren(elt, ns)
	if err != nil {
		return nil, err
	}

	return &LiteralElement{
		Name:       elt.Name,
		Namespace:  elt.Namespaces[elt.Prefix],
		Prefix:     elt.Prefix,
		Attributes: attrs,
		Children:   children,
	}, nil
}

// compileTemplateBody separates leading xsl:param elements from
// instructions and returns a TemplateBody.
func compileTemplateBody(elt *goxml.Element, namespaces ...map[string]string) (*TemplateBody, error) {
	var ns map[string]string
	if len(namespaces) > 0 {
		ns = namespaces[0]
	}
	var params []TemplateParam
	var instructions []Instruction

	pastParams := false
	for _, child := range elt.Children() {
		switch n := child.(type) {
		case goxml.CharData:
			text := n.Contents
			if strings.TrimSpace(text) == "" {
				continue
			}
			pastParams = true
			instructions = append(instructions, &LiteralText{Text: text})
		case *goxml.Element:
			childNS := n.Namespaces[n.Prefix]
			if childNS == xslNS && n.Name == "param" && !pastParams {
				name := attrValue(n, "name")
				if name == "" {
					return nil, fmt.Errorf("XSLT: xsl:param missing name attribute (line %d)", n.Line)
				}
				sel := attrValue(n, "select")
				var children []Instruction
				if sel == "" {
					var err error
					children, err = compileChildren(n, ns)
					if err != nil {
						return nil, err
					}
				}
				var asType *SequenceType
				if asStr := attrValue(n, "as"); asStr != "" {
					var asErr error
					asType, asErr = parseSequenceType(asStr)
					if asErr != nil {
						return nil, fmt.Errorf("XSLT: xsl:param name='%s' as='%s': %w", name, asStr, asErr)
					}
				}
				params = append(params, TemplateParam{Name: name, Select: sel, Children: children, As: asType})
			} else {
				pastParams = true
				if childNS == xslNS {
					instr, err := compileXSLInstruction(n, ns)
					if err != nil {
						return nil, err
					}
					if instr != nil {
						instructions = append(instructions, instr)
					}
				} else {
					instr, err := compileLiteralElement(n, ns)
					if err != nil {
						return nil, err
					}
					instructions = append(instructions, instr)
				}
			}
		}
	}

	return &TemplateBody{
		Params:       params,
		Instructions: instructions,
	}, nil
}

// compileAttribute compiles an xsl:attribute element.
func compileAttribute(elt *goxml.Element, namespaces map[string]string) (*XSLAttribute, error) {
	nameStr := attrValue(elt, "name")
	if nameStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:attribute missing name attribute (line %d)", elt.Line)
	}
	nameAVT, err := parseAVT(nameStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing name AVT in xsl:attribute: %w", err)
	}

	var nsAVT AVT
	if nsStr := attrValue(elt, "namespace"); nsStr != "" {
		nsAVT, err = parseAVT(nsStr)
		if err != nil {
			return nil, fmt.Errorf("XSLT: error parsing namespace AVT in xsl:attribute: %w", err)
		}
	}

	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		children, err = compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLAttribute{Name: nameAVT, Namespace: nsAVT, Select: sel, Children: children}, nil
}

// compileCallTemplate compiles an xsl:call-template element.
func compileCallTemplate(elt *goxml.Element, namespaces map[string]string) (*XSLCallTemplate, error) {
	name := attrValue(elt, "name")
	if name == "" {
		return nil, fmt.Errorf("XSLT: xsl:call-template missing name attribute (line %d)", elt.Line)
	}

	var withParams []XSLWithParam
	for _, child := range elt.Children() {
		childElt, ok := child.(*goxml.Element)
		if !ok {
			continue
		}
		childNS := childElt.Namespaces[childElt.Prefix]
		if childNS != xslNS || childElt.Name != "with-param" {
			continue
		}
		pname := attrValue(childElt, "name")
		if pname == "" {
			return nil, fmt.Errorf("XSLT: xsl:with-param missing name attribute (line %d)", childElt.Line)
		}
		sel := attrValue(childElt, "select")
		var children []Instruction
		if sel == "" {
			var err error
			children, err = compileChildren(childElt, namespaces)
			if err != nil {
				return nil, err
			}
		}
		withParams = append(withParams, XSLWithParam{Name: pname, Select: sel, Children: children})
	}

	return &XSLCallTemplate{Name: name, WithParams: withParams}, nil
}

func (cc *compileContext) compileFunction(elt *goxml.Element) error {
	name := attrValue(elt, "name")
	if name == "" {
		return fmt.Errorf("XSLT: xsl:function missing name attribute (line %d)", elt.Line)
	}

	// Split prefix:localname
	var prefix, localName string
	if idx := strings.IndexByte(name, ':'); idx >= 0 {
		prefix = name[:idx]
		localName = name[idx+1:]
	} else {
		return fmt.Errorf("XSLT: xsl:function name '%s' must be in a namespace (line %d)", name, elt.Line)
	}

	// Resolve prefix to namespace URI (check element namespaces first, then stylesheet)
	ns, ok := elt.Namespaces[prefix]
	if !ok {
		ns, ok = cc.ss.Namespaces[prefix]
	}
	if !ok {
		return fmt.Errorf("XSLT: xsl:function name '%s': cannot resolve prefix '%s' (line %d)", name, prefix, elt.Line)
	}

	var asType *SequenceType
	if asStr := attrValue(elt, "as"); asStr != "" {
		var parseErr error
		asType, parseErr = parseSequenceType(asStr)
		if parseErr != nil {
			return fmt.Errorf("XSLT: xsl:function name='%s' as='%s': %w", name, asStr, parseErr)
		}
	}

	body, err := compileTemplateBody(elt, cc.ss.Namespaces)
	if err != nil {
		return err
	}

	cc.ss.Functions[ns+" "+localName] = &FunctionDef{
		Namespace: ns,
		LocalName: localName,
		Params:    body.Params,
		Body:      body,
		As:        asType,
	}
	return nil
}

// parseAVT parses an Attribute Value Template string into an AVT.
// For example: "Hello {name}, welcome to {city}!" → parts with mixed text/expr.
func parseAVT(s string) (AVT, error) {
	var parts []AVTPart
	i := 0
	for i < len(s) {
		switch s[i] {
		case '{':
			if i+1 < len(s) && s[i+1] == '{' {
				// Escaped {{ → literal {
				parts = appendTextPart(parts, "{")
				i += 2
			} else {
				// Find matching }
				depth := 0
				inString := false
				stringChar := byte(0)
				j := i + 1
				for j < len(s) {
					ch := s[j]
					if inString {
						if ch == stringChar {
							inString = false
						}
					} else {
						switch ch {
						case '\'', '"':
							inString = true
							stringChar = ch
						case '{':
							depth++
						case '}':
							if depth == 0 {
								goto found
							}
							depth--
						}
					}
					j++
				}
				return AVT{}, fmt.Errorf("unterminated '{' in AVT: %s", s)
			found:
				expr := strings.TrimSpace(s[i+1 : j])
				if expr == "" {
					return AVT{}, fmt.Errorf("empty expression in AVT: %s", s)
				}
				parts = append(parts, AVTPart{Expr: expr})
				i = j + 1
			}
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				// Escaped }} → literal }
				parts = appendTextPart(parts, "}")
				i += 2
			} else {
				return AVT{}, fmt.Errorf("unmatched '}' in AVT: %s", s)
			}
		default:
			// Collect static text
			j := i + 1
			for j < len(s) && s[j] != '{' && s[j] != '}' {
				j++
			}
			parts = appendTextPart(parts, s[i:j])
			i = j
		}
	}

	// Optimize: if no parts, return a single empty text part.
	if len(parts) == 0 {
		parts = []AVTPart{{Text: ""}}
	}
	return AVT{Parts: parts}, nil
}

// appendTextPart appends text to the last text part or creates a new one.
func appendTextPart(parts []AVTPart, text string) []AVTPart {
	if len(parts) > 0 && parts[len(parts)-1].Expr == "" {
		parts[len(parts)-1].Text += text
		return parts
	}
	return append(parts, AVTPart{Text: text})
}

// extractSorts extracts xsl:sort children from the element and returns SortKeys.
// The xsl:sort children are consumed and will be skipped by compileChildren.
func extractSorts(elt *goxml.Element) ([]SortKey, error) {
	var sorts []SortKey
	for _, child := range elt.Children() {
		childElt, ok := child.(*goxml.Element)
		if !ok {
			continue
		}
		childNS := childElt.Namespaces[childElt.Prefix]
		if childNS != xslNS || childElt.Name != "sort" {
			continue
		}
		sel := attrValue(childElt, "select")
		if sel == "" {
			sel = "."
		}
		order := attrValue(childElt, "order")
		if order == "" {
			order = "ascending"
		}
		dataType := attrValue(childElt, "data-type")
		if dataType == "" {
			dataType = "text"
		}
		sorts = append(sorts, SortKey{Select: sel, Order: order, DataType: dataType})
	}
	return sorts, nil
}

// --------------------------------------------------------------------------
// Match pattern parser (MVP: handles /, name, name/name, *, text(), node(),
// comment(), a|b, and simple patterns with //)
// --------------------------------------------------------------------------

func parseMatchPattern(s string, namespaces map[string]string) (Pattern, error) {
	s = strings.TrimSpace(s)

	// Union pattern: "a | b" (predicate-aware split)
	if parts := splitTopLevelOn(s, '|'); len(parts) > 1 {
		var patterns []Pattern
		for _, p := range parts {
			pat, err := parseMatchPattern(p, namespaces)
			if err != nil {
				return nil, err
			}
			patterns = append(patterns, pat)
		}
		return &UnionPattern{Patterns: patterns}, nil
	}

	// Document root: "/"
	if s == "/" {
		return &NodeKindTest{Kind: NodeDocument}, nil
	}

	// Path pattern: "a/b" or "a//b" (predicate-aware check)
	if containsTopLevel(s, '/') {
		return parsePathPattern(s, namespaces)
	}

	// Simple pattern
	return parseSimplePattern(s, namespaces)
}

func parsePathPattern(s string, namespaces map[string]string) (Pattern, error) {
	// Handle "//" (ancestor) vs "/" (parent)
	// Split from the right to build bottom-up, predicate-aware.
	useAncestor := false
	var upperStr, baseStr string

	idx, isDouble := lastTopLevelSlash(s)
	if idx >= 0 {
		if isDouble {
			upperStr = strings.TrimSpace(s[:idx])
			baseStr = strings.TrimSpace(s[idx+2:])
			useAncestor = true
		} else {
			upperStr = strings.TrimSpace(s[:idx])
			baseStr = strings.TrimSpace(s[idx+1:])
			useAncestor = false
		}
	}

	if baseStr == "" {
		return nil, fmt.Errorf("empty base in path pattern: %s", s)
	}

	basePat, err := parseSimplePattern(baseStr, namespaces)
	if err != nil {
		return nil, err
	}

	// If upper is empty (e.g. "/book"), it means child of document.
	if upperStr == "" {
		return &AncestorQualifiedPattern{
			BasePattern:  basePat,
			UpperPattern: &NodeKindTest{Kind: NodeDocument},
			UseAncestor:  false,
		}, nil
	}

	// Upper may itself be a path.
	upperPat, err := parseMatchPattern(upperStr, namespaces)
	if err != nil {
		return nil, err
	}

	return &AncestorQualifiedPattern{
		BasePattern:  basePat,
		UpperPattern: upperPat,
		UseAncestor:  useAncestor,
	}, nil
}

func parseSimplePattern(s string, namespaces map[string]string) (Pattern, error) {
	s = strings.TrimSpace(s)

	// Split off predicates: "book[@lang='en']" → base="book", preds=["@lang='en'"]
	base, predicates := splitPredicates(s)

	pat, err := parseBasePattern(base, namespaces)
	if err != nil {
		return nil, err
	}

	// Wrap in PredicatePattern for each predicate (innermost first).
	for _, pred := range predicates {
		pat = &PredicatePattern{
			BasePattern:   pat,
			PredicateExpr: pred,
		}
	}

	return pat, nil
}

func parseBasePattern(s string, namespaces map[string]string) (Pattern, error) {
	s = strings.TrimSpace(s)

	switch s {
	case "*":
		return &WildcardTest{Kind: NodeElement}, nil
	case "node()":
		return &AnyNodeTest{}, nil
	case "text()":
		return &NodeKindTest{Kind: NodeText}, nil
	case "comment()":
		return &NodeKindTest{Kind: NodeComment}, nil
	case "processing-instruction()":
		return &NodeKindTest{Kind: NodeProcessingInstruction}, nil
	case ".":
		return &AnyNodeTest{}, nil
	}

	// Attribute pattern: @name
	if strings.HasPrefix(s, "@") {
		name := s[1:]
		return &NameTest{LocalName: name, Kind: NodeAttribute}, nil
	}

	// Element name test (possibly with namespace prefix)
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		nsURI := namespaces[parts[0]]
		// ns:* wildcard
		if parts[1] == "*" {
			return &NamespaceWildcardTest{
				NamespaceURI: nsURI,
				Kind:         NodeElement,
			}, nil
		}
		return &NameTest{
			LocalName:    parts[1],
			NamespaceURI: nsURI,
			Kind:         NodeElement,
		}, nil
	}

	// Simple element name.
	if isValidName(s) {
		return &NameTest{LocalName: s, Kind: NodeElement}, nil
	}

	return nil, fmt.Errorf("unsupported match pattern: %s", s)
}

func isValidName(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		if i == 0 {
			if !isNameStartChar(ch) {
				return false
			}
		} else {
			if !isNameChar(ch) {
				return false
			}
		}
	}
	return true
}

func isNameStartChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isNameChar(ch rune) bool {
	return isNameStartChar(ch) || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.'
}

// attrValue returns the value of a named attribute, or "" if not present.
func attrValue(elt *goxml.Element, name string) string {
	for _, attr := range elt.Attributes() {
		if attr.Name == name {
			return attr.Value
		}
	}
	return ""
}

// compileElement compiles an xsl:element instruction.
func compileElement(elt *goxml.Element, namespaces map[string]string) (*XSLElement, error) {
	nameStr := attrValue(elt, "name")
	if nameStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:element missing name attribute (line %d)", elt.Line)
	}
	nameAVT, err := parseAVT(nameStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing name AVT in xsl:element: %w", err)
	}
	var nsAVT AVT
	if nsStr := attrValue(elt, "namespace"); nsStr != "" {
		nsAVT, err = parseAVT(nsStr)
		if err != nil {
			return nil, fmt.Errorf("XSLT: error parsing namespace AVT in xsl:element: %w", err)
		}
	}
	children, err := compileChildren(elt, namespaces)
	if err != nil {
		return nil, err
	}
	return &XSLElement{Name: nameAVT, Namespace: nsAVT, Children: children}, nil
}

// compileComment compiles an xsl:comment instruction.
func compileComment(elt *goxml.Element, namespaces map[string]string) (*XSLComment, error) {
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		var err error
		children, err = compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLComment{Select: sel, Children: children}, nil
}

// compileProcessingInstruction compiles an xsl:processing-instruction instruction.
func compileProcessingInstruction(elt *goxml.Element, namespaces map[string]string) (*XSLProcessingInstruction, error) {
	nameStr := attrValue(elt, "name")
	if nameStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:processing-instruction missing name attribute (line %d)", elt.Line)
	}
	nameAVT, err := parseAVT(nameStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing name AVT in xsl:processing-instruction: %w", err)
	}
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		children, err = compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLProcessingInstruction{Name: nameAVT, Select: sel, Children: children}, nil
}

// compileMessage compiles an xsl:message instruction.
func compileMessage(elt *goxml.Element, namespaces map[string]string) (*XSLMessage, error) {
	terminate := attrValue(elt, "terminate") == "yes"
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		var err error
		children, err = compileChildren(elt, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLMessage{Terminate: terminate, Select: sel, Children: children}, nil
}

// compileNumber compiles an xsl:number instruction.
func compileNumber(elt *goxml.Element, namespaces map[string]string) (*XSLNumber, error) {
	level := attrValue(elt, "level")
	if level == "" {
		level = "single"
	}
	format := attrValue(elt, "format")
	if format == "" {
		format = "1"
	}
	n := &XSLNumber{
		Select: attrValue(elt, "select"),
		Value:  attrValue(elt, "value"),
		Count:  attrValue(elt, "count"),
		From:   attrValue(elt, "from"),
		Level:  level,
		Format: format,
	}
	if n.Count != "" {
		pat, err := parseMatchPattern(n.Count, namespaces)
		if err != nil {
			return nil, fmt.Errorf("XSLT: xsl:number count='%s': %w", n.Count, err)
		}
		n.CountPat = pat
	}
	if n.From != "" {
		pat, err := parseMatchPattern(n.From, namespaces)
		if err != nil {
			return nil, fmt.Errorf("XSLT: xsl:number from='%s': %w", n.From, err)
		}
		n.FromPat = pat
	}
	return n, nil
}

// compileResultDocument compiles an xsl:result-document instruction.
func compileResultDocument(elt *goxml.Element, namespaces map[string]string) (*XSLResultDocument, error) {
	hrefStr := attrValue(elt, "href")
	if hrefStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:result-document missing href attribute (line %d)", elt.Line)
	}
	hrefAVT, err := parseAVT(hrefStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing href AVT in xsl:result-document: %w", err)
	}
	children, err := compileChildren(elt, namespaces)
	if err != nil {
		return nil, err
	}
	return &XSLResultDocument{Href: hrefAVT, Children: children}, nil
}

// --------------------------------------------------------------------------
// Predicate-aware helpers for pattern parsing
// --------------------------------------------------------------------------

// splitPredicates splits "name[p1][p2]" into ("name", ["p1", "p2"]).
// String literals inside predicates are handled correctly.
func splitPredicates(s string) (string, []string) {
	// Find first '[' not inside a string literal.
	firstBracket := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\'' || ch == '"' {
			i++
			for i < len(s) && s[i] != ch {
				i++
			}
			continue
		}
		if ch == '[' {
			firstBracket = i
			break
		}
	}
	if firstBracket < 0 {
		return s, nil
	}

	base := s[:firstBracket]
	rest := s[firstBracket:]

	var preds []string
	for len(rest) > 0 {
		if rest[0] != '[' {
			break
		}
		end := findMatchingBracket(rest)
		if end < 0 {
			return s, nil // unmatched bracket
		}
		preds = append(preds, rest[1:end])
		rest = rest[end+1:]
	}

	return strings.TrimSpace(base), preds
}

// findMatchingBracket returns the index of the ']' matching the '[' at position 0.
// Returns -1 if not found.
func findMatchingBracket(s string) int {
	if len(s) == 0 || s[0] != '[' {
		return -1
	}
	depth := 0
	inStr := false
	var strCh byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if ch == strCh {
				inStr = false
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inStr = true
			strCh = ch
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevelOn splits s on the given separator byte at the top level
// (not inside [...] brackets or string literals).
func splitTopLevelOn(s string, sep byte) []string {
	var parts []string
	depth := 0
	inStr := false
	var strCh byte
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if ch == strCh {
				inStr = false
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inStr = true
			strCh = ch
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		default:
			if ch == sep && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// containsTopLevel checks if s contains ch at the top level.
func containsTopLevel(s string, ch byte) bool {
	depth := 0
	inStr := false
	var strCh byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == strCh {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = true
			strCh = c
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		default:
			if c == ch && depth == 0 {
				return true
			}
		}
	}
	return false
}

// lastTopLevelSlash finds the last '/' or '//' at the top level.
// Returns (index, isDouble). index is -1 if not found.
func lastTopLevelSlash(s string) (int, bool) {
	depth := 0
	inStr := false
	var strCh byte
	lastIdx := -1
	lastDouble := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if ch == strCh {
				inStr = false
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inStr = true
			strCh = ch
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '/':
			if depth == 0 {
				if i+1 < len(s) && s[i+1] == '/' {
					lastIdx = i
					lastDouble = true
					i++ // skip second '/'
				} else {
					lastIdx = i
					lastDouble = false
				}
			}
		}
	}
	return lastIdx, lastDouble
}
