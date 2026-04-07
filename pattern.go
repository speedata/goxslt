package goxslt

import (
	"strconv"
	"strings"

	"github.com/speedata/goxml"
)

// --------------------------------------------------------------------------
// NameTest — matches element or attribute by local name + optional namespace
// --------------------------------------------------------------------------

// NameTest matches an element or attribute by local name (and optionally
// namespace URI). Corresponds to patterns like match="book" or match="svg:rect".
type NameTest struct {
	LocalName    string
	NamespaceURI string // empty = no namespace constraint
	Kind         NodeKind
}

func (p *NameTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	switch p.Kind {
	case NodeElement:
		elt, ok := node.(*goxml.Element)
		if !ok {
			return false
		}
		if elt.Name != p.LocalName {
			return false
		}
		if p.NamespaceURI != "" {
			return elt.Namespaces[elt.Prefix] == p.NamespaceURI
		}
		return true
	case NodeAttribute:
		attr, ok := node.(*goxml.Attribute)
		if !ok {
			return false
		}
		if attr.Name != p.LocalName {
			return false
		}
		if p.NamespaceURI != "" {
			return attr.Namespace == p.NamespaceURI
		}
		return true
	}
	return false
}

func (p *NameTest) Fingerprint() string       { return p.LocalName }
func (p *NameTest) MatchesNodeKind() NodeKind { return p.Kind }
func (p *NameTest) DefaultPriority() float64  { return 0.0 }

// --------------------------------------------------------------------------
// NodeKindTest — matches any node of a given kind (text(), comment(), etc.)
// --------------------------------------------------------------------------

type NodeKindTest struct {
	Kind NodeKind
}

func (p *NodeKindTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	return NodeKindOf(node) == p.Kind
}

func (p *NodeKindTest) Fingerprint() string       { return "" }
func (p *NodeKindTest) MatchesNodeKind() NodeKind { return p.Kind }
func (p *NodeKindTest) DefaultPriority() float64  { return -0.5 }

// --------------------------------------------------------------------------
// AnyNodeTest — matches any node (match="node()")
// --------------------------------------------------------------------------

type AnyNodeTest struct{}

func (p *AnyNodeTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	// node() in a match pattern matches element, text, comment, PI — but NOT
	// attributes, namespace nodes, or the document node.
	switch node.(type) {
	case *goxml.Attribute, *goxml.XMLDocument:
		return false
	default:
		return true
	}
}
func (p *AnyNodeTest) Fingerprint() string       { return "" }
func (p *AnyNodeTest) MatchesNodeKind() NodeKind { return NodeGeneric }
func (p *AnyNodeTest) DefaultPriority() float64  { return -0.5 }

// --------------------------------------------------------------------------
// WildcardTest — matches any element or any attribute (match="*")
// --------------------------------------------------------------------------

type WildcardTest struct {
	Kind NodeKind // NodeElement or NodeAttribute
}

func (p *WildcardTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	return NodeKindOf(node) == p.Kind
}

func (p *WildcardTest) Fingerprint() string       { return "" }
func (p *WildcardTest) MatchesNodeKind() NodeKind { return p.Kind }
func (p *WildcardTest) DefaultPriority() float64  { return -0.5 }

// --------------------------------------------------------------------------
// NamespaceWildcardTest — matches elements/attributes in a namespace (ns:*)
// --------------------------------------------------------------------------

type NamespaceWildcardTest struct {
	NamespaceURI string
	Kind         NodeKind
}

func (p *NamespaceWildcardTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	switch p.Kind {
	case NodeElement:
		elt, ok := node.(*goxml.Element)
		if !ok {
			return false
		}
		return elt.Namespaces[elt.Prefix] == p.NamespaceURI
	case NodeAttribute:
		attr, ok := node.(*goxml.Attribute)
		if !ok {
			return false
		}
		return attr.Namespace == p.NamespaceURI
	}
	return false
}

func (p *NamespaceWildcardTest) Fingerprint() string       { return "" }
func (p *NamespaceWildcardTest) MatchesNodeKind() NodeKind { return p.Kind }
func (p *NamespaceWildcardTest) DefaultPriority() float64  { return -0.25 }

// --------------------------------------------------------------------------
// AncestorQualifiedPattern — path patterns like match="book/title" or
// match="chapter//para"
// --------------------------------------------------------------------------

type AncestorQualifiedPattern struct {
	BasePattern  Pattern // for the node itself (e.g. "title")
	UpperPattern Pattern // for the parent/ancestor (e.g. "book")
	UseAncestor  bool    // false = parent only (/), true = any ancestor (//)
}

func (p *AncestorQualifiedPattern) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	if !p.BasePattern.Matches(node, ctx) {
		return false
	}
	parent := parentOf(node)
	if parent == nil {
		return false
	}
	if !p.UseAncestor {
		return p.UpperPattern.Matches(parent, ctx)
	}
	for anc := parent; anc != nil; anc = parentOf(anc) {
		if p.UpperPattern.Matches(anc, ctx) {
			return true
		}
	}
	return false
}

func (p *AncestorQualifiedPattern) Fingerprint() string       { return p.BasePattern.Fingerprint() }
func (p *AncestorQualifiedPattern) MatchesNodeKind() NodeKind { return p.BasePattern.MatchesNodeKind() }
func (p *AncestorQualifiedPattern) DefaultPriority() float64  { return 0.5 }

// --------------------------------------------------------------------------
// PredicatePattern — base pattern + predicate filter (match="book[@lang='en']")
// --------------------------------------------------------------------------

type PredicatePattern struct {
	BasePattern   Pattern
	PredicateExpr string // XPath expression from the match pattern
	PredicateFunc func(node goxml.XMLNode, ctx *MatchContext) bool
}

func (p *PredicatePattern) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	if !p.BasePattern.Matches(node, ctx) {
		return false
	}
	if p.PredicateExpr != "" {
		// Handle numeric positional predicates: [N] means position()=N
		// among siblings that match the base pattern.
		if pos, err := strconv.Atoi(strings.TrimSpace(p.PredicateExpr)); err == nil {
			return positionAmongSiblings(node, p.BasePattern, ctx) == pos
		}
		if ctx != nil && ctx.XPathEval != nil {
			result, err := ctx.XPathEval(p.PredicateExpr, node)
			if err != nil {
				return false
			}
			return result
		}
	}
	if p.PredicateFunc != nil {
		return p.PredicateFunc(node, ctx)
	}
	return true
}

// positionAmongSiblings returns the 1-based position of node among its
// parent's children that match the given pattern.
func positionAmongSiblings(node goxml.XMLNode, base Pattern, ctx *MatchContext) int {
	parent := parentOf(node)
	if parent == nil {
		return 1
	}
	nodeID := node.GetID()
	pos := 0
	for _, sibling := range parent.Children() {
		if base.Matches(sibling, ctx) {
			pos++
			if sibling.GetID() == nodeID {
				return pos
			}
		}
	}
	return 0
}

func (p *PredicatePattern) Fingerprint() string       { return p.BasePattern.Fingerprint() }
func (p *PredicatePattern) MatchesNodeKind() NodeKind { return p.BasePattern.MatchesNodeKind() }
func (p *PredicatePattern) DefaultPriority() float64  { return 0.5 }

// --------------------------------------------------------------------------
// UnionPattern — match="a | b"
// --------------------------------------------------------------------------

// UnionPattern matches if any sub-pattern matches. In XSLT, union patterns
// are typically split into separate rules during compilation (each branch gets
// its own Rule with the same template but different Part numbers). This type
// is provided for cases where you want to keep them together.
type UnionPattern struct {
	Patterns []Pattern
}

func (p *UnionPattern) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	for _, sub := range p.Patterns {
		if sub.Matches(node, ctx) {
			return true
		}
	}
	return false
}

func (p *UnionPattern) Fingerprint() string { return "" }

func (p *UnionPattern) MatchesNodeKind() NodeKind {
	if len(p.Patterns) == 0 {
		return NodeGeneric
	}
	kind := p.Patterns[0].MatchesNodeKind()
	for _, sub := range p.Patterns[1:] {
		if sub.MatchesNodeKind() != kind {
			return NodeGeneric
		}
	}
	return kind
}

func (p *UnionPattern) DefaultPriority() float64 { return -0.5 }

// --------------------------------------------------------------------------
// KeyPattern — matches nodes that are in the result of a key() call
// --------------------------------------------------------------------------

// KeyPattern matches a node if it appears in the result of
// key(KeyName, ValueExpr). Used for match="key('k', 'val')" patterns.
type KeyPattern struct {
	KeyName    string            // expanded key name
	ValueExpr  string            // second argument (string literal or XPath expression)
	Namespaces map[string]string // for XPath evaluation of ValueExpr
}

func (p *KeyPattern) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	if ctx == nil || ctx.KeyLookup == nil {
		return false
	}
	nodes := ctx.KeyLookup(p.KeyName, p.ValueExpr, p.Namespaces)
	nodeID := node.GetID()
	for _, n := range nodes {
		if n.GetID() == nodeID {
			return true
		}
	}
	return false
}

func (p *KeyPattern) Fingerprint() string       { return "" }
func (p *KeyPattern) MatchesNodeKind() NodeKind { return NodeGeneric }
func (p *KeyPattern) DefaultPriority() float64  { return 0.5 }

// --------------------------------------------------------------------------
// XPathPattern — matches by evaluating an XPath expression (e.g. id(), doc())
// --------------------------------------------------------------------------

type XPathPattern struct {
	Expr       string
	Namespaces map[string]string
}

func (p *XPathPattern) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	if ctx == nil || ctx.XPathEvalSequence == nil {
		return false
	}
	result, err := ctx.XPathEvalSequence(p.Expr, node)
	if err != nil {
		return false
	}
	nodeID := node.GetID()
	for _, item := range result {
		if n, ok := item.(goxml.XMLNode); ok && n.GetID() == nodeID {
			return true
		}
	}
	return false
}

func (p *XPathPattern) Fingerprint() string       { return "" }
func (p *XPathPattern) MatchesNodeKind() NodeKind { return NodeGeneric }
func (p *XPathPattern) DefaultPriority() float64  { return 0.5 }

// --------------------------------------------------------------------------
// PINameTest — matches processing-instruction by target name
// --------------------------------------------------------------------------

type PINameTest struct {
	Name string // target name to match
}

func (p *PINameTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	pi, ok := node.(goxml.ProcInst)
	if !ok {
		return false
	}
	return pi.Target == p.Name
}

func (p *PINameTest) Fingerprint() string       { return p.Name }
func (p *PINameTest) MatchesNodeKind() NodeKind { return NodeProcessingInstruction }
func (p *PINameTest) DefaultPriority() float64  { return 0.0 }

// --------------------------------------------------------------------------
// Helper
// --------------------------------------------------------------------------

func parentOf(node goxml.XMLNode) goxml.XMLNode {
	if elt, ok := node.(*goxml.Element); ok {
		return elt.Parent
	}
	return nil
}
