package goxslt

import (
	"strings"
	"testing"

	"github.com/speedata/goxml"
)

// TestSerializeCompactMatchesToXML ensures that the compact mode of the
// unified serializer produces the same bytes as goxml's ToXML.
func TestSerializeCompactMatchesToXML(t *testing.T) {
	testdata := []string{
		`<data><name>AO "Banana" &amp; Co &lt;3</name></data>`,
		`<root xmlns:ns="http://example.com/ns"><ns:item id="1" ns:type="a">x</ns:item><item /></root>`,
		`<a><!--a comment--><?target some data?><b>text</b> tail </a>`,
		`<a xmlns="http://example.com/default"><b attr="&quot;quoted&quot;" /></a>`,
		`<a>
	<b>keep whitespace</b>
</a>`,
	}
	for _, td := range testdata {
		doc, err := goxml.Parse(strings.NewReader(td))
		if err != nil {
			t.Fatal(err)
		}
		want := doc.ToXML()
		if got := SerializeResult(doc); got != want {
			t.Errorf("SerializeResult(%s):\n  got  %s\n  want %s", td, got, want)
		}
		if got := SerializeWithOutput(doc, OutputProperties{OmitXMLDeclaration: true}); got != want {
			t.Errorf("SerializeWithOutput(%s):\n  got  %s\n  want %s", td, got, want)
		}
	}
}

// TestSerializeEscapingConsistent ensures that the compact and the indented
// output use the same escaping policy: quotes stay literal in text nodes and
// are escaped in attribute values (issue goxml#3).
func TestSerializeEscapingConsistent(t *testing.T) {
	input := `<data attr="AO &quot;Banana&quot;"><name>AO "Banana" &amp; 1 &lt; 2</name></data>`
	doc, err := goxml.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	compact := SerializeResult(doc)
	indented := SerializeIndent(doc, "  ")
	for mode, got := range map[string]string{"compact": compact, "indent": indented} {
		if !strings.Contains(got, `<name>AO "Banana" &amp; 1 &lt; 2</name>`) {
			t.Errorf("%s: unexpected text escaping in %s", mode, got)
		}
		if !strings.Contains(got, `attr="AO &quot;Banana&quot;"`) {
			t.Errorf("%s: unexpected attribute escaping in %s", mode, got)
		}
	}
}
