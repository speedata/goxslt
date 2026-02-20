package goxslt

import (
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

func (p *AnyNodeTest) Matches(node goxml.XMLNode, ctx *MatchContext) bool { return true }
func (p *AnyNodeTest) Fingerprint() string                                { return "" }
func (p *AnyNodeTest) MatchesNodeKind() NodeKind                          { return NodeGeneric }
func (p *AnyNodeTest) DefaultPriority() float64                           { return -0.5 }

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
	if p.PredicateExpr != "" && ctx != nil && ctx.XPathEval != nil {
		result, err := ctx.XPathEval(p.PredicateExpr, node)
		if err != nil {
			return false
		}
		return result
	}
	if p.PredicateFunc != nil {
		return p.PredicateFunc(node, ctx)
	}
	return true
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
// Helper
// --------------------------------------------------------------------------

func parentOf(node goxml.XMLNode) goxml.XMLNode {
	if elt, ok := node.(*goxml.Element); ok {
		return elt.Parent
	}
	return nil
}
