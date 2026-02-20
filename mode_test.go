package goxslt

import (
	"strings"
	"testing"

	"github.com/speedata/goxml"
)

// Helper: parse XML from string.
func parseXML(t *testing.T, s string) *goxml.XMLDocument {
	t.Helper()
	doc, err := goxml.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestNameTestMatchesElement(t *testing.T) {
	doc := parseXML(t, `<catalog><book><title>Go</title></book></catalog>`)
	root, _ := doc.Root()

	pat := &NameTest{LocalName: "catalog", Kind: NodeElement}
	ctx := &MatchContext{}
	if !pat.Matches(root, ctx) {
		t.Error("NameTest should match <catalog>")
	}
	if pat.Matches(root.Children()[0], ctx) {
		t.Error("NameTest 'catalog' should not match <book>")
	}
}

func TestAncestorQualifiedPattern(t *testing.T) {
	// <catalog><book><title>Go</title></book></catalog>
	doc := parseXML(t, `<catalog><book><title>Go</title></book></catalog>`)
	root, _ := doc.Root()
	book := root.Children()[0].(*goxml.Element)
	title := book.Children()[0].(*goxml.Element)

	// Pattern: book/title (direct parent)
	pat := &AncestorQualifiedPattern{
		BasePattern:  &NameTest{LocalName: "title", Kind: NodeElement},
		UpperPattern: &NameTest{LocalName: "book", Kind: NodeElement},
		UseAncestor:  false,
	}
	ctx := &MatchContext{}

	if !pat.Matches(title, ctx) {
		t.Error("book/title should match <title> under <book>")
	}
	if pat.Matches(book, ctx) {
		t.Error("book/title should not match <book>")
	}

	// Pattern: catalog//title (any ancestor)
	pat2 := &AncestorQualifiedPattern{
		BasePattern:  &NameTest{LocalName: "title", Kind: NodeElement},
		UpperPattern: &NameTest{LocalName: "catalog", Kind: NodeElement},
		UseAncestor:  true,
	}
	if !pat2.Matches(title, ctx) {
		t.Error("catalog//title should match <title> under <catalog>/<book>")
	}
}

func TestModeGetRulePrecedence(t *testing.T) {
	mode := NewMode("", nil)

	// Rule 1: match="*" priority -0.5 (wildcard)
	mode.AddRule(
		&WildcardTest{Kind: NodeElement},
		&TemplateBody{Name: "wildcard"},
		1, -0.5, 0, 0,
	)

	// Rule 2: match="book" priority 0 (specific name)
	mode.AddRule(
		&NameTest{LocalName: "book", Kind: NodeElement},
		&TemplateBody{Name: "book-template"},
		1, 0.0, 1, 0,
	)

	// Rule 3: match="book" priority 1 (high priority override)
	mode.AddRule(
		&NameTest{LocalName: "book", Kind: NodeElement},
		&TemplateBody{Name: "book-high-prio"},
		1, 1.0, 2, 0,
	)

	mode.ComputeRankings()

	doc := parseXML(t, `<catalog><book>test</book><cd>music</cd></catalog>`)
	root, _ := doc.Root()
	book := root.Children()[0].(*goxml.Element)
	cd := root.Children()[1].(*goxml.Element)
	ctx := &MatchContext{}

	// <book> should match the highest priority rule
	rule, err := mode.GetRule(book, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected a rule for <book>")
	}
	if rule.Template.Name != "book-high-prio" {
		t.Errorf("expected book-high-prio, got %s", rule.Template.Name)
	}

	// <cd> should match the wildcard
	rule, err = mode.GetRule(cd, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected a rule for <cd>")
	}
	if rule.Template.Name != "wildcard" {
		t.Errorf("expected wildcard, got %s", rule.Template.Name)
	}
}

func TestModeImportPrecedence(t *testing.T) {
	mode := NewMode("", nil)

	// Imported stylesheet: match="book" precedence=1
	mode.AddRule(
		&NameTest{LocalName: "book", Kind: NodeElement},
		&TemplateBody{Name: "imported"},
		1, 0.0, 0, 0,
	)

	// Importing stylesheet: match="book" precedence=2 (overrides)
	mode.AddRule(
		&NameTest{LocalName: "book", Kind: NodeElement},
		&TemplateBody{Name: "importing"},
		2, 0.0, 1, 0,
	)

	mode.ComputeRankings()

	doc := parseXML(t, `<book/>`)
	root, _ := doc.Root()
	ctx := &MatchContext{}

	rule, err := mode.GetRule(root, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Template.Name != "importing" {
		t.Errorf("expected importing (higher precedence), got %s", rule.Template.Name)
	}
}

func TestModeTextNode(t *testing.T) {
	mode := NewMode("", nil)

	mode.AddRule(
		&NodeKindTest{Kind: NodeText},
		&TemplateBody{Name: "text-handler"},
		1, -0.5, 0, 0,
	)

	mode.ComputeRankings()

	doc := parseXML(t, `<root>hello</root>`)
	root, _ := doc.Root()
	textNode := root.Children()[0] // CharData
	ctx := &MatchContext{}

	rule, err := mode.GetRule(textNode, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected a rule for text node")
	}
	if rule.Template.Name != "text-handler" {
		t.Errorf("expected text-handler, got %s", rule.Template.Name)
	}
}

func TestBuiltInRulesFallback(t *testing.T) {
	builtIn := &TextOnlyCopyRuleSet{}
	mode := NewMode("", builtIn)

	// Only a rule for <book>, nothing for <cd>
	mode.AddRule(
		&NameTest{LocalName: "book", Kind: NodeElement},
		&TemplateBody{Name: "book-template"},
		1, 0.0, 0, 0,
	)

	mode.ComputeRankings()

	doc := parseXML(t, `<root><cd>music</cd></root>`)
	root, _ := doc.Root()
	cd := root.Children()[0].(*goxml.Element)
	ctx := &MatchContext{}

	rule, err := mode.GetRule(cd, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Error("expected nil rule for <cd> (should fall through to built-in)")
	}
}

func TestPredicatePattern(t *testing.T) {
	doc := parseXML(t, `<root><book lang="en">English</book><book lang="de">Deutsch</book></root>`)
	root, _ := doc.Root()
	bookEN := root.Children()[0].(*goxml.Element)
	bookDE := root.Children()[1].(*goxml.Element)

	// match="book[@lang='en']"
	pat := &PredicatePattern{
		BasePattern: &NameTest{LocalName: "book", Kind: NodeElement},
		PredicateFunc: func(node goxml.XMLNode, ctx *MatchContext) bool {
			if elt, ok := node.(*goxml.Element); ok {
				for _, attr := range elt.Attributes() {
					if attr.Name == "lang" && attr.Value == "en" {
						return true
					}
				}
			}
			return false
		},
	}
	ctx := &MatchContext{}

	if !pat.Matches(bookEN, ctx) {
		t.Error("should match <book lang='en'>")
	}
	if pat.Matches(bookDE, ctx) {
		t.Error("should not match <book lang='de'>")
	}
}
