package goxslt

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
)

// --------------------------------------------------------------------------
// Counting algorithms for xsl:number
// --------------------------------------------------------------------------

// numberValue evaluates the value expression and returns a single number.
func numberValue(tc *TransformContext, expr string) ([]int, error) {
	result, err := tc.evalXPath(expr)
	if err != nil {
		return nil, fmt.Errorf("xsl:number value='%s': %w", expr, err)
	}
	f, err := goxpath.NumberValue(result)
	if err != nil {
		return nil, fmt.Errorf("xsl:number value='%s': %w", expr, err)
	}
	n := int(math.Round(f))
	if n < 1 {
		n = 0
	}
	return []int{n}, nil
}

// defaultCountPattern creates a pattern matching nodes with the same name and
// type as the given node.
func defaultCountPattern(node goxml.XMLNode) Pattern {
	switch n := node.(type) {
	case *goxml.Element:
		return &NameTest{LocalName: n.Name, Kind: NodeElement}
	case goxml.CharData:
		return &NodeKindTest{Kind: NodeText}
	case goxml.Comment:
		return &NodeKindTest{Kind: NodeComment}
	case goxml.ProcInst:
		return &NodeKindTest{Kind: NodeProcessingInstruction}
	default:
		return &AnyNodeTest{}
	}
}

// numberSingle implements level="single": find the nearest ancestor-or-self
// matching count, then count preceding siblings matching count.
func numberSingle(node goxml.XMLNode, count, from Pattern, mc *MatchContext) []int {
	target := findAncestorOrSelf(node, count, from, mc)
	if target == nil {
		return nil
	}
	return []int{countPrecedingSiblings(target, count, mc) + 1}
}

// numberMultiple implements level="multiple": collect all ancestors-or-self
// matching count (stopping at from), then count preceding siblings for each.
func numberMultiple(node goxml.XMLNode, count, from Pattern, mc *MatchContext) []int {
	var ancestors []goxml.XMLNode
	cur := node
	for cur != nil {
		if from != nil && from.Matches(cur, mc) {
			break
		}
		if count.Matches(cur, mc) {
			ancestors = append(ancestors, cur)
		}
		cur = elementParent(cur)
	}
	if len(ancestors) == 0 {
		return nil
	}
	// Reverse to get outermost first.
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}
	numbers := make([]int, len(ancestors))
	for i, anc := range ancestors {
		numbers[i] = countPrecedingSiblings(anc, count, mc) + 1
	}
	return numbers
}

// numberAny implements level="any": count all nodes matching count that
// precede the current node in document order (stopping at from).
func numberAny(node goxml.XMLNode, count, from Pattern, mc *MatchContext) []int {
	n := countPrecedingAny(node, count, from, mc)
	if n == 0 {
		return nil
	}
	return []int{n}
}

// findAncestorOrSelf walks ancestor-or-self to find the first node matching
// count. Stops if from is matched (ancestor above the match is ignored).
func findAncestorOrSelf(node goxml.XMLNode, count, from Pattern, mc *MatchContext) goxml.XMLNode {
	cur := node
	for cur != nil {
		if count.Matches(cur, mc) {
			return cur
		}
		if from != nil && from.Matches(cur, mc) {
			return nil
		}
		cur = elementParent(cur)
	}
	return nil
}

// countPrecedingSiblings counts how many preceding siblings match count.
func countPrecedingSiblings(node goxml.XMLNode, count Pattern, mc *MatchContext) int {
	parent := elementParent(node)
	if parent == nil {
		return 0
	}
	siblings := parent.Children()
	n := 0
	for _, sib := range siblings {
		if nodeID(sib) == nodeID(node) {
			break
		}
		if count.Matches(sib, mc) {
			n++
		}
	}
	return n
}

// countPrecedingAny counts all nodes matching count that precede node in
// document order. Stops at from (if set).
func countPrecedingAny(node goxml.XMLNode, count, from Pattern, mc *MatchContext) int {
	// Collect all nodes in document order up to and including node.
	var all []goxml.XMLNode
	targetID := nodeID(node)
	found := false
	collectDocOrder(documentRoot(node), &all, targetID, &found)

	// Count backwards from the node, stopping at from.
	n := 0
	// Include the target node itself if it matches.
	for i := len(all) - 1; i >= 0; i-- {
		nd := all[i]
		if from != nil && from.Matches(nd, mc) && nodeID(nd) != targetID {
			break
		}
		if count.Matches(nd, mc) {
			n++
		}
	}
	return n
}

// collectDocOrder collects nodes in document order, stopping after targetID.
func collectDocOrder(node goxml.XMLNode, result *[]goxml.XMLNode, targetID int, found *bool) {
	if *found {
		return
	}
	*result = append(*result, node)
	if nodeID(node) == targetID {
		*found = true
		return
	}
	for _, child := range node.Children() {
		collectDocOrder(child, result, targetID, found)
		if *found {
			return
		}
	}
}

// elementParent returns the parent of a node (only works for *Element).
func elementParent(node goxml.XMLNode) goxml.XMLNode {
	if elt, ok := node.(*goxml.Element); ok {
		return elt.Parent
	}
	return nil
}

// documentRoot walks up to the root document node.
func documentRoot(node goxml.XMLNode) goxml.XMLNode {
	for {
		p := elementParent(node)
		if p == nil {
			return node
		}
		node = p
	}
}

// nodeID returns the ID of a node for identity comparison.
func nodeID(node goxml.XMLNode) int {
	switch n := node.(type) {
	case *goxml.XMLDocument:
		return n.ID
	case *goxml.Element:
		return n.ID
	case goxml.CharData:
		return n.ID
	case goxml.Comment:
		return n.ID
	case goxml.ProcInst:
		return n.ID
	default:
		return -1
	}
}

// --------------------------------------------------------------------------
// Format tokenizer and number formatting
// --------------------------------------------------------------------------

// tokenizeFormat splits a format string into alternating punctuation and
// format tokens. Alphanumeric runs are tokens, everything else is punctuation.
// Returns punctuation (len = tokens+1) and tokens.
func tokenizeFormat(format string) (punctuation []string, tokens []string) {
	runes := []rune(format)
	i := 0
	var punc strings.Builder

	// Leading punctuation.
	for i < len(runes) && !isFormatToken(runes[i]) {
		punc.WriteRune(runes[i])
		i++
	}
	punctuation = append(punctuation, punc.String())

	for i < len(runes) {
		// Token.
		var tok strings.Builder
		for i < len(runes) && isFormatToken(runes[i]) {
			tok.WriteRune(runes[i])
			i++
		}
		tokens = append(tokens, tok.String())

		// Separator.
		punc.Reset()
		for i < len(runes) && !isFormatToken(runes[i]) {
			punc.WriteRune(runes[i])
			i++
		}
		punctuation = append(punctuation, punc.String())
	}
	return
}

// isFormatToken returns true for characters that form format tokens.
func isFormatToken(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// formatNumber formats a single integer according to a format token.
func formatNumber(n int, token string) string {
	if n < 1 {
		return "0"
	}
	switch {
	case token == "1":
		return fmt.Sprintf("%d", n)
	case len(token) > 1 && allDigits(token):
		// Zero-padded: "01" means width 2, "001" means width 3.
		return fmt.Sprintf("%0*d", len(token), n)
	case token == "a":
		return alphaNumber(n, 'a')
	case token == "A":
		return alphaNumber(n, 'A')
	case token == "i":
		return toRomanLower(n)
	case token == "I":
		return toRomanUpper(n)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// alphaNumber converts 1→a, 2→b, ..., 26→z, 27→aa, etc.
func alphaNumber(n int, base rune) string {
	if n < 1 {
		return string(base)
	}
	var result []rune
	for n > 0 {
		n--
		result = append([]rune{rune(int(base) + n%26)}, result...)
		n /= 26
	}
	return string(result)
}

// toRomanUpper converts n to uppercase Roman numerals.
func toRomanUpper(n int) string {
	if n <= 0 || n >= 4000 {
		return fmt.Sprintf("%d", n)
	}
	values := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var sb strings.Builder
	for i, v := range values {
		for n >= v {
			sb.WriteString(symbols[i])
			n -= v
		}
	}
	return sb.String()
}

// toRomanLower converts n to lowercase Roman numerals.
func toRomanLower(n int) string {
	return strings.ToLower(toRomanUpper(n))
}

// formatNumbers assembles numbers with a format string.
func formatNumbers(numbers []int, format string) string {
	if len(numbers) == 0 {
		return ""
	}

	punctuation, tokens := tokenizeFormat(format)

	// If no tokens found, use "1" as default.
	if len(tokens) == 0 {
		tokens = []string{"1"}
		punctuation = []string{"", ""}
	}

	var sb strings.Builder
	// Leading punctuation.
	sb.WriteString(punctuation[0])

	for i, n := range numbers {
		if i > 0 {
			// Separator between numbers.
			sepIdx := i
			if sepIdx >= len(punctuation) {
				sepIdx = len(punctuation) - 2 // reuse last separator
			}
			if sepIdx < 1 {
				sepIdx = 1
			}
			if sepIdx < len(punctuation) {
				sb.WriteString(punctuation[sepIdx])
			} else {
				sb.WriteString(".")
			}
		}

		// Pick token: use last available if index exceeds tokens.
		tokIdx := i
		if tokIdx >= len(tokens) {
			tokIdx = len(tokens) - 1
		}
		sb.WriteString(formatNumber(n, tokens[tokIdx]))
	}

	// Trailing punctuation.
	if len(punctuation) > 0 {
		sb.WriteString(punctuation[len(punctuation)-1])
	}

	return sb.String()
}
