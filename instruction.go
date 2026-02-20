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
	Name  string
	Value AVT
}

// LiteralText writes text to the result tree.
type LiteralText struct {
	Text string
}

// XSLValueOf evaluates an XPath expression and writes the string value to the
// result tree. Corresponds to <xsl:value-of select="..."/>.
type XSLValueOf struct {
	Select string // XPath expression
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
type XSLText struct {
	Text string
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

// XSLSequence evaluates an XPath expression and adds items to the current
// sequence constructor. Corresponds to <xsl:sequence select="..."/>.
type XSLSequence struct {
	Select string
}

// SortKey defines a sort criterion for xsl:sort.
type SortKey struct {
	Select   string // XPath expression; default "."
	Order    string // "ascending" (default) or "descending"
	DataType string // "text" (default) or "number"
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
	Select   string        // XPath expression selecting the population
	GroupBy  string        // XPath expression for grouping key
	Sorts    []SortKey     // optional sort keys
	Children []Instruction // body to execute per group
}

// XSLResultDocument redirects output to a secondary result document.
// Corresponds to <xsl:result-document href="...">.
type XSLResultDocument struct {
	Href     AVT
	Children []Instruction
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
