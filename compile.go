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
	expandText bool            // current expand-text setting (XSLT 3.0 TVT)
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

	ss.BasePath = filepath.Dir(absPath)
	ss.StylesheetDoc = xsltDoc

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

	// Read expand-text from root element (XSLT 3.0 Text Value Templates).
	if attrValue(root, "expand-text") == "yes" {
		cc.expandText = true
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
			if v := attrValue(elt, "omit-xml-declaration"); v != "" {
				cc.ss.Output.OmitXMLDeclaration = (v == "yes")
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
				children, err = compileChildren(elt, cc.expandText, cc.ss.Namespaces)
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
			p := TemplateParam{Name: name, Select: sel, Children: children, As: asType}
			cc.ss.GlobalParams = append(cc.ss.GlobalParams, p)
			cc.ss.GlobalDecls = append(cc.ss.GlobalDecls, GlobalDecl{IsParam: true, Param: &cc.ss.GlobalParams[len(cc.ss.GlobalParams)-1]})
		case "variable":
			v, err := compileVariable(elt)
			if err != nil {
				return err
			}
			cc.ss.GlobalVars = append(cc.ss.GlobalVars, *v)
			cc.ss.GlobalDecls = append(cc.ss.GlobalDecls, GlobalDecl{IsParam: false, Var: &cc.ss.GlobalVars[len(cc.ss.GlobalVars)-1]})
		case "key":
			name := attrValue(elt, "name")
			if name == "" {
				return fmt.Errorf("XSLT: xsl:key missing name attribute (line %d)", elt.Line)
			}
			matchStr := attrValue(elt, "match")
			if matchStr == "" {
				return fmt.Errorf("XSLT: xsl:key missing match attribute (line %d)", elt.Line)
			}
			use := attrValue(elt, "use")
			if use == "" {
				return fmt.Errorf("XSLT: xsl:key missing use attribute (line %d)", elt.Line)
			}
			// XTSE0010: xsl:key must not contain child elements when use is specified.
			for _, child := range elt.Children() {
				if _, ok := child.(*goxml.Element); ok {
					return fmt.Errorf("XTSE0010: xsl:key must not contain child elements (line %d)", elt.Line)
				}
			}
			pat, err := parseMatchPattern(matchStr, cc.ss.Namespaces)
			if err != nil {
				return fmt.Errorf("XSLT: xsl:key match='%s': %w", matchStr, err)
			}
			composite := attrValue(elt, "composite") == "yes"
			expandedName := expandQName(name, elt.Namespaces)
			// XTSE1222: all xsl:key declarations with the same name must have
			// the same effective value for the composite attribute.
			for _, existing := range cc.ss.Keys {
				if existing.Name == expandedName && existing.Composite != composite {
					return fmt.Errorf("XTSE1222: xsl:key declarations with name '%s' have inconsistent composite attribute (line %d)", name, elt.Line)
				}
			}
			cc.ss.Keys = append(cc.ss.Keys, KeyDefinition{Name: expandedName, Match: pat, Use: use, Composite: composite})
		case "mode":
			name := attrValue(elt, "name")
			onNoMatch := attrValue(elt, "on-no-match")
			// Determine built-in rule set.
			var builtIn BuiltInRuleSet
			switch onNoMatch {
			case "shallow-copy":
				builtIn = &ShallowCopyRuleSet{}
			default:
				builtIn = &TextOnlyCopyRuleSet{}
			}
			// Get or create the mode.
			if name == "" || name == "#default" || name == "#unnamed" {
				cc.ss.DefaultMode.BuiltInRules = builtIn
			} else {
				if m, ok := cc.ss.Modes[name]; ok {
					m.BuiltInRules = builtIn
				} else {
					m := NewMode(name, builtIn)
					cc.ss.Modes[name] = m
				}
			}
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
	Method             string // "xml", "html", "text"
	Indent             bool
	Version            string
	OmitXMLDeclaration bool // omit-xml-declaration="yes" → true
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
	Keys             []KeyDefinition         // xsl:key declarations
	Namespaces       map[string]string       // prefix → URI from root element
	ResultNamespaces map[string]string       // prefix → URI to propagate to output elements
	GlobalParams     []TemplateParam         // top-level xsl:param declarations
	GlobalVars       []XSLVariable           // top-level xsl:variable declarations
	GlobalDecls      []GlobalDecl            // params and variables in declaration order
	BasePath         string                  // directory for resolving document() URIs
	StylesheetDoc    *goxml.XMLDocument      // parsed stylesheet document (for document(''))
}

// GlobalDecl represents a top-level parameter or variable in declaration order.
type GlobalDecl struct {
	IsParam bool
	Param   *TemplateParam
	Var     *XSLVariable
}

func (cc *compileContext) compileTemplate(elt *goxml.Element) error {
	matchAttr := attrValue(elt, "match")
	nameAttr := attrValue(elt, "name")
	modeAttr := attrValue(elt, "mode")

	if matchAttr == "" && nameAttr == "" {
		return fmt.Errorf("XSLT: xsl:template has neither match nor name attribute (line %d)", elt.Line)
	}

	body, err := compileTemplateBody(elt, cc.expandText, cc.ss.Namespaces)
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
// expandText controls whether text nodes are parsed as Text Value Templates.
func compileChildren(parent *goxml.Element, expandText bool, namespaces ...map[string]string) ([]Instruction, error) {
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
			lt := &LiteralText{Text: text}
			if expandText {
				avt, err := parseAVT(text)
				if err != nil {
					return nil, fmt.Errorf("XSLT: error in text value template: %w", err)
				}
				lt.TVT = &avt
			}
			instructions = append(instructions, lt)

		case *goxml.Element:
			childNS := n.Namespaces[n.Prefix]
			if childNS == xslNS {
				instr, err := compileXSLInstruction(n, expandText, ns)
				if err != nil {
					return nil, err
				}
				if instr != nil {
					instructions = append(instructions, instr)
				}
			} else {
				// Literal result element.
				instr, err := compileLiteralElement(n, expandText, ns)
				if err != nil {
					return nil, err
				}
				instructions = append(instructions, instr)
			}
		}
	}

	return instructions, nil
}

func compileXSLInstruction(elt *goxml.Element, expandText bool, namespaces map[string]string) (Instruction, error) {
	// Any XSL element can override expand-text for its subtree.
	if et := attrValue(elt, "expand-text"); et != "" {
		expandText = et == "yes"
	}
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
				children, err = compileChildren(childElt, expandText, namespaces)
				if err != nil {
					return nil, err
				}
			}
			withParams = append(withParams, XSLWithParam{Name: pname, Select: wpsel, Children: children})
		}
		return &XSLApplyTemplates{Select: sel, Mode: mode, Sorts: sorts, WithParams: withParams}, nil

	case "value-of":
		sel := attrValue(elt, "select")
		var children []Instruction
		if sel == "" {
			var err error
			children, err = compileChildren(elt, expandText, namespaces)
			if err != nil {
				return nil, err
			}
		}
		sep := " " // XSLT 2.0+ default separator
		for _, attr := range elt.Attributes() {
			if attr.Name == "separator" {
				sep = attr.Value
				break
			}
		}
		return &XSLValueOf{Select: sel, Children: children, Separator: sep}, nil

	case "for-each":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:for-each missing select attribute (line %d)", elt.Line)
		}
		sorts, err := extractSorts(elt)
		if err != nil {
			return nil, err
		}
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLForEach{Select: sel, Sorts: sorts, Children: children}, nil

	case "if":
		test := attrValue(elt, "test")
		if test == "" {
			return nil, fmt.Errorf("XSLT: xsl:if missing test attribute (line %d)", elt.Line)
		}
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLIf{Test: test, Children: children}, nil

	case "text":
		// xsl:text can have its own expand-text attribute
		localExpand := expandText
		if et := attrValue(elt, "expand-text"); et != "" {
			localExpand = et == "yes"
		}
		var sb strings.Builder
		for _, child := range elt.Children() {
			if cd, ok := child.(goxml.CharData); ok {
				sb.WriteString(cd.Contents)
			}
		}
		text := sb.String()
		xslText := &XSLText{Text: text}
		if localExpand {
			avt, err := parseAVT(text)
			if err != nil {
				return nil, fmt.Errorf("XSLT: error in xsl:text value template: %w", err)
			}
			xslText.TVT = &avt
		}
		return xslText, nil

	case "copy-of":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:copy-of missing select attribute (line %d)", elt.Line)
		}
		return &XSLCopyOf{Select: sel}, nil

	case "choose":
		return compileChoose(elt, expandText, namespaces)

	case "variable", "param":
		return compileVariable(elt, expandText)

	case "copy":
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLCopy{Children: children}, nil

	case "attribute":
		return compileAttribute(elt, expandText, namespaces)

	case "call-template":
		return compileCallTemplate(elt, expandText, namespaces)

	case "sequence":
		sel := attrValue(elt, "select")
		if sel == "" {
			return nil, fmt.Errorf("XSLT: xsl:sequence missing select attribute (line %d)", elt.Line)
		}
		return &XSLSequence{Select: sel}, nil

	case "element":
		return compileElement(elt, expandText, namespaces)

	case "comment":
		return compileComment(elt, expandText, namespaces)

	case "processing-instruction":
		return compileProcessingInstruction(elt, expandText, namespaces)

	case "message":
		return compileMessage(elt, expandText, namespaces)

	case "number":
		return compileNumber(elt, namespaces)

	case "map":
		children, err := compileChildren(elt, expandText, namespaces)
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
			children, err = compileChildren(elt, expandText, namespaces)
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
		groupAdjacent := attrValue(elt, "group-adjacent")
		groupStartingWith := attrValue(elt, "group-starting-with")
		groupEndingWith := attrValue(elt, "group-ending-with")
		if groupBy == "" && groupAdjacent == "" && groupStartingWith == "" && groupEndingWith == "" {
			return nil, fmt.Errorf("XSLT: xsl:for-each-group missing grouping attribute (line %d)", elt.Line)
		}
		var groupStartingPat, groupEndingPat Pattern
		if groupStartingWith != "" {
			var patErr error
			groupStartingPat, patErr = parseMatchPattern(groupStartingWith, namespaces)
			if patErr != nil {
				return nil, fmt.Errorf("XSLT: xsl:for-each-group group-starting-with (line %d): %w", elt.Line, patErr)
			}
		}
		if groupEndingWith != "" {
			var patErr error
			groupEndingPat, patErr = parseMatchPattern(groupEndingWith, namespaces)
			if patErr != nil {
				return nil, fmt.Errorf("XSLT: xsl:for-each-group group-ending-with (line %d): %w", elt.Line, patErr)
			}
		}
		sorts, err := extractSorts(elt)
		if err != nil {
			return nil, err
		}
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLForEachGroup{Select: sel, GroupBy: groupBy, GroupAdjacent: groupAdjacent, GroupStartingPat: groupStartingPat, GroupEndingPat: groupEndingPat, Sorts: sorts, Children: children}, nil

	case "result-document":
		return compileResultDocument(elt, expandText, namespaces)

	case "source-document":
		return compileSourceDocument(elt, expandText, namespaces)

	case "analyze-string":
		return compileAnalyzeString(elt, expandText, namespaces)

	case "matching-substring", "non-matching-substring":
		// Handled by the parent (analyze-string).
		return nil, nil

	case "sort":
		// xsl:sort is handled by the parent (for-each / apply-templates).
		return nil, nil

	case "with-param":
		// xsl:with-param is handled by the parent (call-template).
		return nil, nil

	case "try":
		return compileTry(elt, expandText, namespaces)

	case "catch":
		// Handled by the parent (try).
		return nil, nil

	case "fork":
		// xsl:fork executes children sequentially (non-streaming).
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLSequenceConstructor{Children: children}, nil

	case "where-populated":
		// Execute children; if the result is empty, discard it.
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLWherePopulated{Children: children}, nil

	case "on-empty":
		// Fallback content when parent produces empty output.
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLOnEmpty{Children: children}, nil

	case "on-non-empty":
		// Additional content when parent produces non-empty output.
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLOnNonEmpty{Children: children}, nil

	case "fallback":
		// xsl:fallback provides fallback for unsupported instructions.
		// Since we're executing in a non-streaming context, just compile children.
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLSequenceConstructor{Children: children}, nil

	case "namespace":
		return compileNamespace(elt, expandText, namespaces)

	case "context-item":
		// Declaration, not executable. Metadata for template validation.
		return nil, nil

	case "document":
		// xsl:document creates a document node. Similar to xsl:result-document without href.
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		return &XSLSequenceConstructor{Children: children}, nil

	default:
		return nil, fmt.Errorf("XSLT: unsupported instruction xsl:%s (line %d)", elt.Name, elt.Line)
	}
}

func compileChoose(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLChoose, error) {
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
			children, err := compileChildren(childElt, expandText, namespaces)
			if err != nil {
				return nil, err
			}
			choose.When = append(choose.When, XSLWhen{Test: test, Children: children})
		case "otherwise":
			children, err := compileChildren(childElt, expandText, namespaces)
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

func compileVariable(elt *goxml.Element, expandTexts ...bool) (*XSLVariable, error) {
	et := false
	if len(expandTexts) > 0 {
		et = expandTexts[0]
	}
	name := attrValue(elt, "name")
	if name == "" {
		return nil, fmt.Errorf("XSLT: xsl:variable missing name attribute (line %d)", elt.Line)
	}
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		var err error
		children, err = compileChildren(elt, et)
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

func compileLiteralElement(elt *goxml.Element, expandText bool, namespaces ...map[string]string) (*LiteralElement, error) {
	var ns map[string]string
	if len(namespaces) > 0 {
		ns = namespaces[0]
	}

	// Check for xsl:expand-text attribute on literal elements
	for _, attr := range elt.Attributes() {
		if attr.Name == "expand-text" && attr.Namespace == xslNS {
			expandText = attr.Value == "yes"
			break
		}
	}

	var attrs []LiteralAttribute
	for _, attr := range elt.Attributes() {
		// Skip xmlns declarations and xsl:expand-text.
		if attr.Name == "xmlns" || strings.HasPrefix(attr.Name, "xmlns:") {
			continue
		}
		if attr.Name == "expand-text" && attr.Namespace == xslNS {
			continue
		}
		avt, err := parseAVT(attr.Value)
		if err != nil {
			return nil, fmt.Errorf("XSLT: error parsing AVT in attribute %s: %w", attr.Name, err)
		}
		attrName := attr.Name
		if attr.Prefix != "" {
			attrName = attr.Prefix + ":" + attr.Name
		}
		attrs = append(attrs, LiteralAttribute{Name: attrName, Namespace: attr.Namespace, Value: avt})
	}

	children, err := compileChildren(elt, expandText, ns)
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
func compileTemplateBody(elt *goxml.Element, expandText bool, namespaces ...map[string]string) (*TemplateBody, error) {
	var ns map[string]string
	if len(namespaces) > 0 {
		ns = namespaces[0]
	}

	// xsl:template can have its own expand-text attribute
	if et := attrValue(elt, "expand-text"); et != "" {
		expandText = et == "yes"
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
			lt := &LiteralText{Text: text}
			if expandText {
				avt, err := parseAVT(text)
				if err != nil {
					return nil, fmt.Errorf("XSLT: error in text value template: %w", err)
				}
				lt.TVT = &avt
			}
			instructions = append(instructions, lt)
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
					children, err = compileChildren(n, expandText, ns)
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
					instr, err := compileXSLInstruction(n, expandText, ns)
					if err != nil {
						return nil, err
					}
					if instr != nil {
						instructions = append(instructions, instr)
					}
				} else {
					instr, err := compileLiteralElement(n, expandText, ns)
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
func compileAttribute(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLAttribute, error) {
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
		children, err = compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLAttribute{Name: nameAVT, Namespace: nsAVT, Select: sel, Children: children}, nil
}

// compileCallTemplate compiles an xsl:call-template element.
func compileCallTemplate(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLCallTemplate, error) {
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
			children, err = compileChildren(childElt, expandText, namespaces)
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

	// Parse function name: either Q{uri}local (EQName) or prefix:local (QName).
	var ns, localName string
	if strings.HasPrefix(name, "Q{") {
		closeBrace := strings.Index(name, "}")
		if closeBrace < 0 {
			return fmt.Errorf("XSLT: xsl:function name '%s': malformed EQName (line %d)", name, elt.Line)
		}
		ns = name[2:closeBrace]
		localName = name[closeBrace+1:]
	} else if idx := strings.IndexByte(name, ':'); idx >= 0 {
		prefix := name[:idx]
		localName = name[idx+1:]
		var ok bool
		ns, ok = elt.Namespaces[prefix]
		if !ok {
			ns, ok = cc.ss.Namespaces[prefix]
		}
		if !ok {
			return fmt.Errorf("XSLT: xsl:function name '%s': cannot resolve prefix '%s' (line %d)", name, prefix, elt.Line)
		}
	} else {
		return fmt.Errorf("XSLT: xsl:function name '%s' must be in a namespace (line %d)", name, elt.Line)
	}

	var asType *SequenceType
	if asStr := attrValue(elt, "as"); asStr != "" {
		var parseErr error
		asType, parseErr = parseSequenceType(asStr)
		if parseErr != nil {
			return fmt.Errorf("XSLT: xsl:function name='%s' as='%s': %w", name, asStr, parseErr)
		}
	}

	body, err := compileTemplateBody(elt, cc.expandText, cc.ss.Namespaces)
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

	// key() pattern: key('name', expr)
	if strings.HasPrefix(s, "key(") {
		return parseKeyPattern(s, namespaces)
	}

	// element() / element(*) / element(name) pattern
	if strings.HasPrefix(s, "element(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[8 : len(s)-1])
		if inner == "" || inner == "*" {
			return &WildcardTest{Kind: NodeElement}, nil
		}
		// element(name) or element(ns:name) — ignore optional type annotation after comma
		if idx := strings.Index(inner, ","); idx >= 0 {
			inner = strings.TrimSpace(inner[:idx])
		}
		if strings.Contains(inner, ":") {
			parts := strings.SplitN(inner, ":", 2)
			nsURI := namespaces[parts[0]]
			if parts[1] == "*" {
				return &NamespaceWildcardTest{NamespaceURI: nsURI, Kind: NodeElement}, nil
			}
			return &NameTest{LocalName: parts[1], NamespaceURI: nsURI, Kind: NodeElement}, nil
		}
		if inner == "*" || inner == "" {
			return &WildcardTest{Kind: NodeElement}, nil
		}
		return &NameTest{LocalName: inner, Kind: NodeElement}, nil
	}

	// attribute() / attribute(*) / attribute(name) pattern
	if strings.HasPrefix(s, "attribute(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[10 : len(s)-1])
		if inner == "" || inner == "*" {
			return &WildcardTest{Kind: NodeAttribute}, nil
		}
		if idx := strings.Index(inner, ","); idx >= 0 {
			inner = strings.TrimSpace(inner[:idx])
		}
		if strings.Contains(inner, ":") {
			parts := strings.SplitN(inner, ":", 2)
			nsURI := namespaces[parts[0]]
			if parts[1] == "*" {
				return &NamespaceWildcardTest{NamespaceURI: nsURI, Kind: NodeAttribute}, nil
			}
			return &NameTest{LocalName: parts[1], NamespaceURI: nsURI, Kind: NodeAttribute}, nil
		}
		return &NameTest{LocalName: inner, Kind: NodeAttribute}, nil
	}

	// document-node() pattern
	if s == "document-node()" {
		return &AnyNodeTest{}, nil
	}

	// schema-element(name) — treat leniently as element name test
	if strings.HasPrefix(s, "schema-element(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[15 : len(s)-1])
		if inner == "" || inner == "*" {
			return &WildcardTest{Kind: NodeElement}, nil
		}
		return &NameTest{LocalName: inner, Kind: NodeElement}, nil
	}

	// Attribute pattern: @name or @*
	if strings.HasPrefix(s, "@") {
		name := s[1:]
		if name == "*" {
			return &WildcardTest{Kind: NodeAttribute}, nil
		}
		if strings.Contains(name, ":") {
			parts := strings.SplitN(name, ":", 2)
			nsURI := namespaces[parts[0]]
			if parts[1] == "*" {
				return &NamespaceWildcardTest{NamespaceURI: nsURI, Kind: NodeAttribute}, nil
			}
			return &NameTest{LocalName: parts[1], NamespaceURI: nsURI, Kind: NodeAttribute}, nil
		}
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

// expandQName expands a prefixed QName to Clark notation {uri}local using the
// given namespace mapping. Unprefixed names are returned unchanged.
func expandQName(name string, namespaces map[string]string) string {
	if idx := strings.IndexByte(name, ':'); idx >= 0 {
		prefix := name[:idx]
		local := name[idx+1:]
		if ns, ok := namespaces[prefix]; ok {
			return "{" + ns + "}" + local
		}
	}
	return name
}

// compileElement compiles an xsl:element instruction.
func compileElement(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLElement, error) {
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
	children, err := compileChildren(elt, expandText, namespaces)
	if err != nil {
		return nil, err
	}
	return &XSLElement{Name: nameAVT, Namespace: nsAVT, Children: children}, nil
}

// compileNamespace compiles an xsl:namespace instruction.
func compileNamespace(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLNamespace, error) {
	nameStr := attrValue(elt, "name")
	if nameStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:namespace missing name attribute (line %d)", elt.Line)
	}
	nameAVT, err := parseAVT(nameStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing name AVT in xsl:namespace: %w", err)
	}
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		children, err = compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLNamespace{Name: nameAVT, Select: sel, Children: children}, nil
}

// compileComment compiles an xsl:comment instruction.
func compileComment(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLComment, error) {
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		var err error
		children, err = compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLComment{Select: sel, Children: children}, nil
}

// compileProcessingInstruction compiles an xsl:processing-instruction instruction.
func compileProcessingInstruction(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLProcessingInstruction, error) {
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
		children, err = compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
	}
	return &XSLProcessingInstruction{Name: nameAVT, Select: sel, Children: children}, nil
}

// compileMessage compiles an xsl:message instruction.
func compileMessage(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLMessage, error) {
	terminate := attrValue(elt, "terminate") == "yes"
	sel := attrValue(elt, "select")
	var children []Instruction
	if sel == "" {
		var err error
		children, err = compileChildren(elt, expandText, namespaces)
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
func compileResultDocument(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLResultDocument, error) {
	hrefStr := attrValue(elt, "href")
	var hrefAVT AVT
	if hrefStr != "" {
		var err error
		hrefAVT, err = parseAVT(hrefStr)
		if err != nil {
			return nil, fmt.Errorf("XSLT: error parsing href AVT in xsl:result-document: %w", err)
		}
	}
	children, err := compileChildren(elt, expandText, namespaces)
	if err != nil {
		return nil, err
	}
	return &XSLResultDocument{Href: hrefAVT, Children: children}, nil
}

// compileSourceDocument compiles an xsl:source-document instruction.
func compileSourceDocument(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLSourceDocument, error) {
	hrefStr := attrValue(elt, "href")
	if hrefStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:source-document missing href attribute (line %d)", elt.Line)
	}
	hrefAVT, err := parseAVT(hrefStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing href AVT in xsl:source-document: %w", err)
	}
	children, err := compileChildren(elt, expandText, namespaces)
	if err != nil {
		return nil, err
	}
	return &XSLSourceDocument{Href: hrefAVT, Children: children}, nil
}

// compileTry compiles an xsl:try instruction with its xsl:catch children.
func compileTry(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLTry, error) {
	tryInstr := &XSLTry{
		Select: attrValue(elt, "select"),
	}

	// Build a virtual element containing only the non-catch children for the try body.
	// Then compile catch clauses separately.
	for _, child := range elt.Children() {
		childElt, ok := child.(*goxml.Element)
		if !ok {
			continue
		}
		childNS := childElt.Namespaces[childElt.Prefix]
		if childNS == xslNS && childElt.Name == "catch" {
			errors := attrValue(childElt, "errors")
			if errors == "" {
				errors = "*"
			}
			catchChildren, err := compileChildren(childElt, expandText, namespaces)
			if err != nil {
				return nil, err
			}
			tryInstr.Catches = append(tryInstr.Catches, XSLCatch{
				Errors:   errors,
				Children: catchChildren,
				Select:   attrValue(childElt, "select"),
			})
		}
	}

	// Compile the try body: all children except xsl:catch and xsl:fallback.
	// We use compileChildren which handles text nodes, literal elements, and XSL instructions.
	// Since compileChildren processes all children and xsl:catch returns nil from
	// compileXSLInstruction, we can just use compileChildren directly.
	if tryInstr.Select == "" {
		children, err := compileChildren(elt, expandText, namespaces)
		if err != nil {
			return nil, err
		}
		tryInstr.Children = children
	}

	return tryInstr, nil
}

// compileAnalyzeString compiles an xsl:analyze-string instruction.
func compileAnalyzeString(elt *goxml.Element, expandText bool, namespaces map[string]string) (*XSLAnalyzeString, error) {
	sel := attrValue(elt, "select")
	if sel == "" {
		return nil, fmt.Errorf("XSLT: xsl:analyze-string missing select attribute (line %d)", elt.Line)
	}
	regexStr := attrValue(elt, "regex")
	if regexStr == "" {
		return nil, fmt.Errorf("XSLT: xsl:analyze-string missing regex attribute (line %d)", elt.Line)
	}
	regexAVT, err := parseAVT(regexStr)
	if err != nil {
		return nil, fmt.Errorf("XSLT: error parsing regex AVT in xsl:analyze-string: %w", err)
	}
	var flagsAVT AVT
	if flagsStr := attrValue(elt, "flags"); flagsStr != "" {
		flagsAVT, err = parseAVT(flagsStr)
		if err != nil {
			return nil, fmt.Errorf("XSLT: error parsing flags AVT in xsl:analyze-string: %w", err)
		}
	}

	instr := &XSLAnalyzeString{
		Select: sel,
		Regex:  regexAVT,
		Flags:  flagsAVT,
	}

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
		case "matching-substring":
			children, err := compileChildren(childElt, expandText, namespaces)
			if err != nil {
				return nil, err
			}
			instr.Matching = children
		case "non-matching-substring":
			children, err := compileChildren(childElt, expandText, namespaces)
			if err != nil {
				return nil, err
			}
			instr.NonMatching = children
		}
	}

	return instr, nil
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

// findMatchingParen returns the index of the ')' matching the '(' at position 0.
// Returns -1 if not found.
func findMatchingParen(s string) int {
	if len(s) == 0 || s[0] != '(' {
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
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseKeyPattern parses "key('name', expr)" into a KeyPattern.
func parseKeyPattern(s string, namespaces map[string]string) (*KeyPattern, error) {
	// s starts with "key(" — find matching ")"
	inner := s[3:] // starts with "("
	end := findMatchingParen(inner)
	if end < 0 {
		return nil, fmt.Errorf("unmatched '(' in key pattern: %s", s)
	}
	if end+3 != len(s)-1 {
		// Trailing content after ) — shouldn't happen for a base pattern
		// (path continuations are handled by parsePathPattern)
		trail := strings.TrimSpace(s[end+4:])
		if trail != "" {
			return nil, fmt.Errorf("unexpected content after key() pattern: %s", trail)
		}
	}
	content := inner[1:end] // content between ( and )

	// Split on top-level comma to get two arguments
	args := splitTopLevelOn(content, ',')
	if len(args) != 2 {
		return nil, fmt.Errorf("key() pattern requires exactly 2 arguments, got %d: %s", len(args), s)
	}

	// First argument: key name (string literal)
	keyNameRaw := strings.TrimSpace(args[0])
	keyName, err := stripStringLiteral(keyNameRaw)
	if err != nil {
		return nil, fmt.Errorf("key() first argument must be a string literal: %s", keyNameRaw)
	}
	expandedName := expandQName(keyName, namespaces)

	// Second argument: value expression (kept as-is for runtime evaluation)
	valueExpr := strings.TrimSpace(args[1])

	return &KeyPattern{
		KeyName:    expandedName,
		ValueExpr:  valueExpr,
		Namespaces: namespaces,
	}, nil
}

// stripStringLiteral removes surrounding quotes from a string literal ('...' or "...").
func stripStringLiteral(s string) (string, error) {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1], nil
		}
	}
	return "", fmt.Errorf("not a string literal: %s", s)
}

// splitTopLevelOn splits s on the given separator byte at the top level
// (not inside [...] brackets, (...) parentheses, or string literals).
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
		case '[', '(':
			depth++
		case ']', ')':
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
		case '[', '(':
			depth++
		case ']', ')':
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
		case '[', '(':
			depth++
		case ']', ')':
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
