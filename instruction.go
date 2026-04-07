package goxslt

// Instruction is a compiled XSLT instruction that can be executed during
// transformation.
type Instruction interface {
	Execute(ctx *TransformContext) error
}

// LiteralElement creates an element in the result tree, then executes its
// child instructions inside it.
type LiteralElement struct {
	Name       string
	Namespace  string
	Prefix     string
	Attributes []LiteralAttribute
	Children   []Instruction
}

// AVTPart represents one piece of an Attribute Value Template.
// Exactly one of Text or Expr is non-empty.
type AVTPart struct {
	Text string // static text
	Expr string // XPath expression (from {...})
}

// AVT is a parsed Attribute Value Template.
type AVT struct {
	Parts []AVTPart
}

// LiteralAttribute is an attribute on a literal result element.
type LiteralAttribute struct {
	Name      string
	Namespace string
	Value     AVT
}

// LiteralText writes text to the result tree.
// When TVT is non-nil, the text is evaluated as a Text Value Template.
type LiteralText struct {
	Text string
	TVT  *AVT // non-nil when expand-text is active
}

// XSLValueOf evaluates an XPath expression or its child sequence constructor
// and writes the string value to the result tree.
type XSLValueOf struct {
	Select    string        // XPath expression (optional if Children is set)
	Children  []Instruction // sequence constructor (used when Select is empty)
	Separator string        // item separator (default: single space in XSLT 2.0+)
}

// XSLApplyTemplates selects nodes via an XPath expression and applies
// matching templates. Corresponds to <xsl:apply-templates select="..."/>.
type XSLApplyTemplates struct {
	Select     string         // XPath expression; empty means "child::node()"
	Mode       string         // mode name; empty means default mode
	Sorts      []SortKey      // optional sort keys
	WithParams []XSLWithParam // xsl:with-param children
}

// XSLForEach iterates over a node set and executes child instructions.
// Corresponds to <xsl:for-each select="...">.
type XSLForEach struct {
	Select   string
	Sorts    []SortKey
	Children []Instruction
}

// XSLIf conditionally executes child instructions.
// Corresponds to <xsl:if test="...">.
type XSLIf struct {
	Test     string // XPath expression returning boolean
	Children []Instruction
}

// XSLCopyOf copies nodes selected by an XPath expression to the result tree.
// Corresponds to <xsl:copy-of select="..."/>.
type XSLCopyOf struct {
	Select string
}

// XSLText writes literal text to the result tree.
// Corresponds to <xsl:text>...</xsl:text>.
// When TVT is non-nil, the text is evaluated as a Text Value Template.
type XSLText struct {
	Text string
	TVT  *AVT // non-nil when expand-text is active
}

// XSLChoose implements conditional processing with multiple branches.
// Corresponds to <xsl:choose>.
type XSLChoose struct {
	When      []XSLWhen     // one or more xsl:when branches
	Otherwise []Instruction // optional xsl:otherwise body (nil if absent)
}

// XSLWhen is a single branch inside xsl:choose.
type XSLWhen struct {
	Test     string        // XPath boolean expression
	Children []Instruction // body to execute if test is true
}

// XSLVariable binds a variable in the current scope.
// Corresponds to <xsl:variable name="..." select="..."> or with a body.
type XSLVariable struct {
	Name     string        // variable name (without $)
	Select   string        // XPath expression (used if Children is empty)
	Children []Instruction // body (used if Select is empty, produces an RTF)
	As       *SequenceType // optional type declaration (as attribute)
}

// XSLCopy performs a shallow copy of the current node.
// Corresponds to <xsl:copy>...</xsl:copy>.
type XSLCopy struct {
	Children []Instruction
}

// XSLAttribute creates an attribute on the current result element.
// Corresponds to <xsl:attribute name="..." namespace="..." select="...">.
type XSLAttribute struct {
	Name      AVT
	Namespace AVT
	Select    string
	Children  []Instruction
}

// XSLCallTemplate calls a named template.
// Corresponds to <xsl:call-template name="...">.
type XSLCallTemplate struct {
	Name       string
	WithParams []XSLWithParam
}

// XSLWithParam passes a parameter to a called template.
type XSLWithParam struct {
	Name     string
	Select   string
	Children []Instruction
}

// XSLNextMatch finds the next matching template rule (lower priority/precedence)
// and executes it. Corresponds to <xsl:next-match/>.
type XSLNextMatch struct {
	WithParams []XSLWithParam // xsl:with-param children
}

// XSLApplyImports applies imported template rules. Corresponds to <xsl:apply-imports/>.
type XSLApplyImports struct {
	WithParams []XSLWithParam // xsl:with-param children
}

// XSLSequence evaluates an XPath expression and adds items to the current
// sequence constructor. Corresponds to <xsl:sequence select="..."/>.
type XSLSequence struct {
	Select string
}

// SortKey defines a sort criterion for xsl:sort.
type SortKey struct {
	Select          string // XPath expression; default "."
	Order           AVT    // "ascending" (default) or "descending"
	DataType        AVT    // "text" (default) or "number"
	DataTypeExplicit bool  // true if data-type was explicitly set in the stylesheet
	Lang            string // language tag for collation (e.g. "en", "de")
	CaseOrder       AVT    // "upper-first" or "lower-first"
	Stable          string // "yes" or "no"
	Collation       string // collation URI
}

// XSLPerformSort sorts a sequence and writes it to the result tree.
// Corresponds to <xsl:perform-sort select="...">.
type XSLPerformSort struct {
	Select   string        // XPath expression for the input sequence (optional)
	Sorts    []SortKey     // sort keys
	Children []Instruction // sequence constructor (used when Select is empty)
}

// XSLElement creates an element with a computed name.
// Corresponds to <xsl:element name="..." namespace="...">.
type XSLElement struct {
	Name      AVT
	Namespace AVT
	Children  []Instruction
}

// XSLComment creates a comment node in the result tree.
// Corresponds to <xsl:comment>.
type XSLComment struct {
	Select   string        // optional select expression
	Children []Instruction // body (used if Select is empty)
}

// XSLProcessingInstruction creates a processing instruction in the result tree.
// Corresponds to <xsl:processing-instruction name="...">.
type XSLProcessingInstruction struct {
	Name     AVT
	Select   string
	Children []Instruction
}

// XSLMessage outputs a message during transformation.
// Corresponds to <xsl:message terminate="yes|no">.
type XSLMessage struct {
	Terminate bool          // if true, halt transformation after message
	Select    string        // optional select expression
	Children  []Instruction // body (used if Select is empty)
}

// XSLMap creates an XPath map from xsl:map-entry children.
// Corresponds to <xsl:map>.
type XSLMap struct {
	Children []Instruction
}

// XSLMapEntry creates a single map entry within xsl:map.
// Corresponds to <xsl:map-entry key="..." select="...">.
type XSLMapEntry struct {
	Key      string        // XPath expression for the key
	Select   string        // XPath expression for the value (optional)
	Children []Instruction // body for value (used if Select is empty)
}

// XSLForEachGroup groups items and iterates over groups.
// Corresponds to <xsl:for-each-group select="..." group-by="...">.
type XSLForEachGroup struct {
	Select           string        // XPath expression selecting the population
	GroupBy          string        // XPath expression for grouping key
	GroupAdjacent    string        // XPath expression for adjacent grouping key
	GroupStartingPat Pattern       // compiled pattern for group-starting-with
	GroupEndingPat   Pattern       // compiled pattern for group-ending-with
	Sorts            []SortKey     // optional sort keys
	Children         []Instruction // body to execute per group
}

// XSLResultDocument redirects output to a secondary result document.
// Corresponds to <xsl:result-document href="...">.
type XSLResultDocument struct {
	Href     AVT
	Children []Instruction
}

// XSLSourceDocument loads an external document and executes its body with
// that document as the context node.
// Corresponds to <xsl:source-document href="..." streamable="yes|no">.
type XSLSourceDocument struct {
	Href     AVT
	Children []Instruction
}

// XSLTry executes a sequence constructor and catches dynamic errors.
type XSLTry struct {
	Children []Instruction // try body
	Catches  []XSLCatch    // xsl:catch clauses
	Select   string        // optional select expression (alternative to Children)
}

// XSLCatch handles errors caught by xsl:try.
type XSLCatch struct {
	Errors   string        // space-separated error QNames or "*" (default: "*")
	Children []Instruction // catch body
	Select   string        // optional select expression
}

// XSLWherePopulated executes children and includes output only if non-empty.
type XSLWherePopulated struct {
	Children []Instruction
}

// XSLOnEmpty provides fallback content when the parent sequence is empty.
type XSLOnEmpty struct {
	Children []Instruction
}

// XSLOnNonEmpty provides content only when the parent sequence is non-empty.
type XSLOnNonEmpty struct {
	Children []Instruction
}

// XSLSequenceConstructor is a generic wrapper for a sequence of instructions.
type XSLSequenceConstructor struct {
	Children []Instruction
}

// XSLNamespace creates a namespace node on the current output element.
type XSLNamespace struct {
	Name     AVT
	Select   string
	Children []Instruction
}

// XSLAnalyzeString splits a string by a regular expression and processes
// matching and non-matching substrings separately.
// Corresponds to <xsl:analyze-string select="..." regex="...">.
type XSLAnalyzeString struct {
	Select      string        // XPath expression → input string
	Regex       AVT           // AVT → regex pattern
	Flags       AVT           // AVT → regex flags (optional)
	Matching    []Instruction // body of xsl:matching-substring
	NonMatching []Instruction // body of xsl:non-matching-substring
}

// KeyDefinition holds a compiled xsl:key declaration.
type KeyDefinition struct {
	Name      string  // key name (QName)
	Match     Pattern // pattern for nodes to index
	Use       string  // XPath expression for the key value
	Composite bool    // composite="yes" means the key value is a sequence
}

// XSLNumber generates a formatted number in the result tree.
// Corresponds to <xsl:number>.
type XSLNumber struct {
	Select   string  // XPath expression selecting the node to count from (default: context node)
	Value    string  // XPath expression → direct number (bypasses count/from/level)
	Count    string  // match pattern string: which nodes to count
	From     string  // match pattern string: where to start counting
	Level    string  // "single" (default), "multiple", "any"
	Format   string  // format picture string (default "1")
	CountPat Pattern // compiled count pattern (nil means use default)
	FromPat  Pattern // compiled from pattern (nil means no from)
}
