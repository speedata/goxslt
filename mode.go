// Package goxslt implements XSLT template matching and apply-templates logic.
// It uses goxml's node model and is designed to integrate with goxpath for
// pattern evaluation.
package goxslt

import (
	"fmt"

	"github.com/speedata/goxml"
)

// NodeKind identifies the type of an XML node.
type NodeKind int

const (
	NodeDocument              NodeKind = iota // document-node()
	NodeElement                               // element()
	NodeAttribute                             // attribute()
	NodeText                                  // text()
	NodeComment                               // comment()
	NodeProcessingInstruction                 // processing-instruction()
	NodeGeneric               NodeKind = -1   // matches any node kind
)

// NodeKindOf returns the NodeKind for a goxml.XMLNode.
func NodeKindOf(node goxml.XMLNode) NodeKind {
	switch node.(type) {
	case *goxml.XMLDocument:
		return NodeDocument
	case *goxml.Element:
		return NodeElement
	case *goxml.Attribute:
		return NodeAttribute
	case goxml.CharData:
		return NodeText
	case goxml.Comment:
		return NodeComment
	case goxml.ProcInst:
		return NodeProcessingInstruction
	default:
		return NodeGeneric
	}
}

// Pattern is an interface for XSLT match patterns. Implementations test
// whether a given node matches the pattern.
type Pattern interface {
	// Matches returns true if the node matches this pattern.
	Matches(node goxml.XMLNode, ctx *MatchContext) bool
	// Fingerprint returns a unique element/attribute name for bucketing,
	// or "" if the pattern can match multiple names.
	Fingerprint() string
	// MatchesNodeKind returns the NodeKind this pattern can match, or
	// NodeGeneric if it can match multiple kinds.
	MatchesNodeKind() NodeKind
	// DefaultPriority returns the default priority per the XSLT spec.
	DefaultPriority() float64
}

// MatchContext provides the context needed during pattern matching (namespace
// resolution etc.). Can be extended to wrap a goxpath.Context.
type MatchContext struct {
	Namespaces map[string]string
	XPathEval  func(expr string, node goxml.XMLNode) (bool, error)
	KeyLookup  func(keyName, valueExpr string, ns map[string]string) []goxml.XMLNode
}

// --------------------------------------------------------------------------
// Rule
// --------------------------------------------------------------------------

// Rule represents a single template rule (an xsl:template with a match
// pattern).
type Rule struct {
	pattern       Pattern
	Template      *TemplateBody
	Precedence    int     // import precedence (higher = imported later)
	Priority      float64 // explicit or default priority
	Sequence      int     // declaration order in the stylesheet
	Part          int     // part number when a union pattern is split
	rank          int     // precomputed rank for fast comparison
	alwaysMatches bool    // skip pattern test (bucketing guarantees match)
	next          *Rule   // linked list within a ruleChain
}

// Matches tests whether this rule's pattern matches the given node.
func (r *Rule) Matches(node goxml.XMLNode, ctx *MatchContext) bool {
	if r.alwaysMatches {
		return true
	}
	return r.pattern.Matches(node, ctx)
}

func (r *Rule) compareRank(other *Rule) int {
	return r.rank - other.rank
}

func (r *Rule) compareComputedRank(other *Rule) int {
	if r.Precedence == other.Precedence {
		if r.Priority < other.Priority {
			return -1
		} else if r.Priority > other.Priority {
			return 1
		}
		return 0
	}
	if r.Precedence < other.Precedence {
		return -1
	}
	return 1
}

// --------------------------------------------------------------------------
// TemplateBody — placeholder for your template execution logic
// --------------------------------------------------------------------------

// TemplateParam is a declared parameter on a named template.
type TemplateParam struct {
	Name     string
	Select   string
	Children []Instruction
	As       *SequenceType // optional type declaration (as attribute)
}

// TemplateBody represents the body of an xsl:template.
type TemplateBody struct {
	Name         string          // optional template name
	Params       []TemplateParam // declared xsl:param elements
	Instructions []Instruction   // compiled instruction tree (after params)
}

// --------------------------------------------------------------------------
// ruleChain — sorted linked list of rules
// --------------------------------------------------------------------------

type ruleChain struct {
	head *Rule
}

// --------------------------------------------------------------------------
// BuiltInRuleSet
// --------------------------------------------------------------------------

// BuiltInRuleSet defines the fallback behavior when no user template matches.
type BuiltInRuleSet interface {
	Process(node goxml.XMLNode, ctx *TransformContext) error
}

// RecoveryPolicy controls how ambiguous template matches are handled.
type RecoveryPolicy int

const (
	RecoverSilently    RecoveryPolicy = iota // first match wins, no warning
	RecoverWithWarning                       // warn, use last in declaration order
	DoNotRecover                             // error on ambiguity
)

// --------------------------------------------------------------------------
// Mode
// --------------------------------------------------------------------------

// Mode is a collection of template rules, equivalent to an XSLT named or
// unnamed mode. It uses bucketing by node kind and element/attribute name
// for efficient lookup.
type Mode struct {
	Name string

	// Per node kind:
	genericChain  *ruleChain
	documentChain *ruleChain
	textChain     *ruleChain
	commentChain  *ruleChain
	piChain       *ruleChain

	// Elements:
	namedElementChains  map[string]*ruleChain // key = local name
	unnamedElementChain *ruleChain            // match="*" etc.

	// Attributes:
	namedAttributeChains  map[string]*ruleChain
	unnamedAttributeChain *ruleChain

	BuiltInRules   BuiltInRuleSet
	RecoveryPolicy RecoveryPolicy
	rankComputed   bool
}

// NewMode creates a new empty Mode.
func NewMode(name string, builtIn BuiltInRuleSet) *Mode {
	return &Mode{
		Name:                  name,
		genericChain:          &ruleChain{},
		documentChain:         &ruleChain{},
		textChain:             &ruleChain{},
		commentChain:          &ruleChain{},
		piChain:               &ruleChain{},
		namedElementChains:    make(map[string]*ruleChain),
		unnamedElementChain:   &ruleChain{},
		namedAttributeChains:  make(map[string]*ruleChain),
		unnamedAttributeChain: &ruleChain{},
		BuiltInRules:          builtIn,
		RecoveryPolicy:        RecoverSilently,
	}
}

// AddRule registers a new template rule in this mode.
func (m *Mode) AddRule(pattern Pattern, tmpl *TemplateBody, precedence int, priority float64, sequence int, part int) {
	r := &Rule{
		pattern:    pattern,
		Template:   tmpl,
		Precedence: precedence,
		Priority:   priority,
		Sequence:   sequence,
		Part:       part,
	}

	fp := pattern.Fingerprint()
	kind := pattern.MatchesNodeKind()
	guaranteed := !patternNeedsMatch(pattern)

	switch kind {
	case NodeElement:
		if fp != "" {
			r.alwaysMatches = guaranteed
			chain := m.namedElementChains[fp]
			if chain == nil {
				chain = &ruleChain{head: r}
				m.namedElementChains[fp] = chain
			} else {
				insertRule(r, chain)
			}
		} else {
			insertRule(r, m.unnamedElementChain)
		}
	case NodeAttribute:
		if fp != "" {
			r.alwaysMatches = guaranteed
			chain := m.namedAttributeChains[fp]
			if chain == nil {
				chain = &ruleChain{head: r}
				m.namedAttributeChains[fp] = chain
			} else {
				insertRule(r, chain)
			}
		} else {
			insertRule(r, m.unnamedAttributeChain)
		}
	case NodeDocument:
		insertRule(r, m.documentChain)
	case NodeText:
		insertRule(r, m.textChain)
	case NodeComment:
		insertRule(r, m.commentChain)
	case NodeProcessingInstruction:
		insertRule(r, m.piChain)
	default:
		insertRule(r, m.genericChain)
	}

	m.rankComputed = false
}

// ComputeRankings assigns integer ranks to all rules so that runtime
// comparison is a single integer subtraction. Call after all rules have been
// added.
func (m *Mode) ComputeRankings() {
	var all []*Rule
	collect := func(chain *ruleChain) {
		for r := chain.head; r != nil; r = r.next {
			all = append(all, r)
		}
	}

	collect(m.genericChain)
	collect(m.documentChain)
	collect(m.textChain)
	collect(m.commentChain)
	collect(m.piChain)
	collect(m.unnamedElementChain)
	collect(m.unnamedAttributeChain)
	for _, chain := range m.namedElementChains {
		collect(chain)
	}
	for _, chain := range m.namedAttributeChains {
		collect(chain)
	}

	if len(all) == 0 {
		m.rankComputed = true
		return
	}

	sortRulesByComputedRank(all)

	rank := 0
	all[0].rank = 0
	for i := 1; i < len(all); i++ {
		if all[i].compareComputedRank(all[i-1]) != 0 {
			rank++
		}
		all[i].rank = rank
	}

	m.rankComputed = true
}

// GetRule finds the best matching template rule for the given node.
func (m *Mode) GetRule(node goxml.XMLNode, ctx *MatchContext) (*Rule, error) {
	if !m.rankComputed {
		m.ComputeRankings()
	}

	var bestRule *Rule
	var err error

	switch n := node.(type) {
	case *goxml.XMLDocument:
		bestRule, err = m.searchChain(node, ctx, nil, m.documentChain)
		if err != nil {
			return nil, err
		}

	case *goxml.Element:
		if chain, ok := m.namedElementChains[n.Name]; ok {
			bestRule, err = m.searchChain(node, ctx, nil, chain)
			if err != nil {
				return nil, err
			}
		}
		bestRule, err = m.searchChain(node, ctx, bestRule, m.unnamedElementChain)
		if err != nil {
			return nil, err
		}

	case *goxml.Attribute:
		if chain, ok := m.namedAttributeChains[n.Name]; ok {
			bestRule, err = m.searchChain(node, ctx, nil, chain)
			if err != nil {
				return nil, err
			}
		}
		bestRule, err = m.searchChain(node, ctx, bestRule, m.unnamedAttributeChain)
		if err != nil {
			return nil, err
		}

	case goxml.CharData:
		bestRule, err = m.searchChain(node, ctx, nil, m.textChain)
		if err != nil {
			return nil, err
		}

	case goxml.Comment:
		bestRule, err = m.searchChain(node, ctx, nil, m.commentChain)
		if err != nil {
			return nil, err
		}

	case goxml.ProcInst:
		bestRule, err = m.searchChain(node, ctx, nil, m.piChain)
		if err != nil {
			return nil, err
		}
	}

	// Always also search the generic chain (match="node()").
	bestRule, err = m.searchChain(node, ctx, bestRule, m.genericChain)
	if err != nil {
		return nil, err
	}

	return bestRule, nil
}

// searchChain walks a sorted rule chain to find the best matching rule.
// This is the core algorithm from Saxon's SimpleMode.searchRuleChain().
func (m *Mode) searchChain(node goxml.XMLNode, ctx *MatchContext, bestRule *Rule, chain *ruleChain) (*Rule, error) {
	head := chain.head
	for head != nil {
		if bestRule != nil {
			cmp := head.compareRank(bestRule)
			if cmp < 0 {
				// Rest of chain has lower rank → done.
				break
			} else if cmp == 0 {
				// Same rank → ambiguity check.
				if head.Matches(node, ctx) {
					if head.Sequence != bestRule.Sequence {
						if err := m.reportAmbiguity(bestRule, head); err != nil {
							return nil, err
						}
					}
					if bestRule.Sequence > head.Sequence {
						// bestRule wins (later declaration)
					} else if bestRule.Sequence < head.Sequence {
						bestRule = head
					} else {
						// Same sequence (split union pattern) → higher part wins.
						if head.Part > bestRule.Part {
							bestRule = head
						}
					}
					break
				}
			} else {
				// Higher rank → try to match.
				if head.Matches(node, ctx) {
					bestRule = head
				}
			}
		} else if head.Matches(node, ctx) {
			bestRule = head
			if m.RecoveryPolicy == RecoverSilently {
				break
			}
		}
		head = head.next
	}
	return bestRule, nil
}

func (m *Mode) reportAmbiguity(rule1, rule2 *Rule) error {
	switch m.RecoveryPolicy {
	case DoNotRecover:
		return fmt.Errorf("XTDE0540: ambiguous template match (rules at sequence %d and %d)", rule1.Sequence, rule2.Sequence)
	case RecoverWithWarning:
		// TODO: log warning
	}
	return nil
}

// --------------------------------------------------------------------------
// insertRule: sorted insert into a chain (descending by precedence/priority)
// --------------------------------------------------------------------------

func insertRule(r *Rule, chain *ruleChain) {
	if chain.head == nil {
		chain.head = r
		return
	}

	head := chain.head
	var prev *Rule

	for head != nil {
		if head.Precedence < r.Precedence ||
			(head.Precedence == r.Precedence && head.Priority <= r.Priority) {
			r.next = head
			if prev == nil {
				chain.head = r
			} else {
				prev.next = r
			}
			return
		}
		prev = head
		head = head.next
	}

	prev.next = r
}

// patternNeedsMatch returns true for patterns that require actual matching
// even when the fingerprint matches (predicates, ancestor checks).
func patternNeedsMatch(p Pattern) bool {
	switch p.(type) {
	case *PredicatePattern, *AncestorQualifiedPattern:
		return true
	}
	return false
}

func sortRulesByComputedRank(rules []*Rule) {
	for i := 1; i < len(rules); i++ {
		key := rules[i]
		j := i - 1
		for j >= 0 && rules[j].compareComputedRank(key) > 0 {
			rules[j+1] = rules[j]
			j--
		}
		rules[j+1] = key
	}
}
