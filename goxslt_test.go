package goxslt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/speedata/goxml"
)

const testXML = `<?xml version="1.0"?>
<catalog>
  <book id="1">
    <title>Go Programming</title>
    <author>Smith</author>
  </book>
  <book id="2">
    <title>XML Essentials</title>
    <author>Jones</author>
  </book>
</catalog>`

const testXSLT = `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <html>
      <body>
        <xsl:apply-templates select="catalog/book"/>
      </body>
    </html>
  </xsl:template>

  <xsl:template match="book">
    <div>
      <h2><xsl:value-of select="title"/></h2>
      <p><xsl:value-of select="author"/></p>
    </div>
  </xsl:template>
</xsl:stylesheet>`

func TestEndToEnd(t *testing.T) {
	// Parse source XML.
	sourceDoc, err := goxml.Parse(strings.NewReader(testXML))
	if err != nil {
		t.Fatal("parsing source XML:", err)
	}

	// Parse XSLT.
	xsltDoc, err := goxml.Parse(strings.NewReader(testXSLT))
	if err != nil {
		t.Fatal("parsing XSLT:", err)
	}

	// Compile stylesheet.
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal("compiling XSLT:", err)
	}

	// Transform.
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal("transforming:", err)
	}

	result := SerializeResult(resultDoc.Document)
	t.Logf("Result:\n%s", result)

	// Verify structure.
	if !strings.Contains(result, "<html>") {
		t.Error("result should contain <html>")
	}
	if !strings.Contains(result, "<h2>Go Programming</h2>") {
		t.Error("result should contain <h2>Go Programming</h2>")
	}
	if !strings.Contains(result, "<h2>XML Essentials</h2>") {
		t.Error("result should contain <h2>XML Essentials</h2>")
	}
	if !strings.Contains(result, "<p>Smith</p>") {
		t.Error("result should contain <p>Smith</p>")
	}
	if !strings.Contains(result, "<p>Jones</p>") {
		t.Error("result should contain <p>Jones</p>")
	}
}

func TestIdentityTransform(t *testing.T) {
	xml := `<root><a>hello</a><b>world</b></root>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:apply-templates/>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, err := goxml.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	xsltDoc, err := goxml.Parse(strings.NewReader(xslt))
	if err != nil {
		t.Fatal(err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	// With built-in text-only-copy rules, this should produce just the text.
	if !strings.Contains(result, "hello") {
		t.Error("result should contain 'hello'")
	}
	if !strings.Contains(result, "world") {
		t.Error("result should contain 'world'")
	}
}

func TestXSLIf(t *testing.T) {
	xml := `<items><item type="a">Alpha</item><item type="b">Beta</item></items>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <result>
      <xsl:apply-templates select="items/item"/>
    </result>
  </xsl:template>
  <xsl:template match="item">
    <xsl:if test="@type = 'a'">
      <found><xsl:value-of select="."/></found>
    </xsl:if>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xml))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if !strings.Contains(result, "<found>Alpha</found>") {
		t.Error("result should contain <found>Alpha</found>")
	}
	if strings.Contains(result, "Beta") {
		t.Error("result should NOT contain Beta")
	}
}

func TestForEach(t *testing.T) {
	xml := `<data><val>1</val><val>2</val><val>3</val></data>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <list>
      <xsl:for-each select="data/val">
        <item><xsl:value-of select="."/></item>
      </xsl:for-each>
    </list>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xml))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if !strings.Contains(result, "<item>1</item>") {
		t.Error("result should contain <item>1</item>")
	}
	if !strings.Contains(result, "<item>2</item>") {
		t.Error("result should contain <item>2</item>")
	}
	if !strings.Contains(result, "<item>3</item>") {
		t.Error("result should contain <item>3</item>")
	}
}

func TestChoose(t *testing.T) {
	xmlData := `<items>
  <item type="fruit">Apple</item>
  <item type="veggie">Carrot</item>
  <item type="fruit">Banana</item>
  <item>Unknown</item>
</items>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <result><xsl:apply-templates select="items/item"/></result>
  </xsl:template>
  <xsl:template match="item">
    <xsl:choose>
      <xsl:when test="@type = 'fruit'">
        <fruit><xsl:value-of select="."/></fruit>
      </xsl:when>
      <xsl:when test="@type = 'veggie'">
        <veggie><xsl:value-of select="."/></veggie>
      </xsl:when>
      <xsl:otherwise>
        <other><xsl:value-of select="."/></other>
      </xsl:otherwise>
    </xsl:choose>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if !strings.Contains(result, "<fruit>Apple</fruit>") {
		t.Error("expected <fruit>Apple</fruit>")
	}
	if !strings.Contains(result, "<fruit>Banana</fruit>") {
		t.Error("expected <fruit>Banana</fruit>")
	}
	if !strings.Contains(result, "<veggie>Carrot</veggie>") {
		t.Error("expected <veggie>Carrot</veggie>")
	}
	if !strings.Contains(result, "<other>Unknown</other>") {
		t.Error("expected <other>Unknown</other>")
	}
	if strings.Contains(result, "<veggie>Apple") {
		t.Error("Apple should not be in veggie")
	}
}

func TestVariable(t *testing.T) {
	xmlData := `<root><name>World</name></root>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="greeting" select="'Hello'"/>
    <xsl:variable name="who" select="root/name"/>
    <result><xsl:value-of select="$greeting"/>, <xsl:value-of select="$who"/>!</result>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if !strings.Contains(result, "Hello") {
		t.Error("expected 'Hello' in result")
	}
	if !strings.Contains(result, "World") {
		t.Error("expected 'World' in result")
	}
}

func TestVariableWithBody(t *testing.T) {
	xmlData := `<data><val>42</val></data>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="content">
      <inner><xsl:value-of select="data/val"/></inner>
    </xsl:variable>
    <result><xsl:copy-of select="$content"/></result>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if !strings.Contains(result, "<inner>42</inner>") {
		t.Error("expected <inner>42</inner> in result")
	}
}

func TestChooseWithVariable(t *testing.T) {
	xmlData := `<order><total>150</total></order>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="amount" select="order/total"/>
    <result>
      <xsl:choose>
        <xsl:when test="$amount &gt; 100">
          <status>premium</status>
        </xsl:when>
        <xsl:otherwise>
          <status>standard</status>
        </xsl:otherwise>
      </xsl:choose>
    </result>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if !strings.Contains(result, "<status>premium</status>") {
		t.Error("expected <status>premium</status>")
	}
	if strings.Contains(result, "standard") {
		t.Error("should not contain 'standard'")
	}
}

func TestOutputIndent(t *testing.T) {
	xmlData := `<data><item>A</item><item>B</item></data>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output indent="yes"/>
  <xsl:template match="/">
    <root>
      <xsl:for-each select="data/item">
        <entry><xsl:value-of select="."/></entry>
      </xsl:for-each>
    </root>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	if !ss.Output.Indent {
		t.Fatal("expected Output.Indent to be true")
	}

	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeIndent(resultDoc.Document, "  ")
	t.Logf("Result:\n%s", result)

	if !strings.Contains(result, "  <entry>A</entry>\n") {
		t.Error("expected indented <entry>A</entry>")
	}
	if !strings.Contains(result, "  <entry>B</entry>\n") {
		t.Error("expected indented <entry>B</entry>")
	}
	if !strings.HasPrefix(result, "<root>\n") {
		t.Error("expected <root> followed by newline")
	}
}

func TestOutputIndentNested(t *testing.T) {
	xmlData := `<catalog><book><title>Go</title><author>Smith</author></book></catalog>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output indent="yes"/>
  <xsl:template match="/">
    <html>
      <body>
        <xsl:apply-templates select="catalog/book"/>
      </body>
    </html>
  </xsl:template>
  <xsl:template match="book">
    <div>
      <h2><xsl:value-of select="title"/></h2>
      <p><xsl:value-of select="author"/></p>
    </div>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeIndent(resultDoc.Document, "  ")
	t.Logf("Result:\n%s", result)

	expected := `<html>
  <body>
    <div>
      <h2>Go</h2>
      <p>Smith</p>
    </div>
  </body>
</html>
`
	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestOutputNoIndent(t *testing.T) {
	xmlData := `<data><item>A</item></data>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <root><entry><xsl:value-of select="data/item"/></entry></root>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Output.Indent {
		t.Fatal("expected Output.Indent to be false")
	}

	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)

	if strings.Contains(result, "\n") {
		t.Error("expected no newlines without indent")
	}
	if result != "<root><entry>A</entry></root>" {
		t.Errorf("expected <root><entry>A</entry></root>, got %s", result)
	}
}

// ---------- helper ----------

func runXSLT(t *testing.T, xmlData, xslt string) string {
	t.Helper()
	sourceDoc, err := goxml.Parse(strings.NewReader(xmlData))
	if err != nil {
		t.Fatal("parsing XML:", err)
	}
	xsltDoc, err := goxml.Parse(strings.NewReader(xslt))
	if err != nil {
		t.Fatal("parsing XSLT:", err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal("compiling:", err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal("transforming:", err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)
	return result
}

// ========== xsl:copy ==========

func TestCopyElement(t *testing.T) {
	result := runXSLT(t,
		`<root><item id="1">Hello</item></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <xsl:copy><xsl:apply-templates/></xsl:copy>
  </xsl:template>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/item"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	// xsl:copy on element copies name but NOT attributes
	if !strings.Contains(result, "<item>Hello</item>") {
		t.Errorf("expected <item>Hello</item>, got %s", result)
	}
	if strings.Contains(result, "id=") {
		t.Error("xsl:copy should not copy attributes")
	}
}

func TestCopyText(t *testing.T) {
	result := runXSLT(t,
		`<root>Hello</root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="text()">
    <xsl:copy/>
  </xsl:template>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/text()"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>Hello</out>") {
		t.Errorf("expected <out>Hello</out>, got %s", result)
	}
}

func TestCopyWithBody(t *testing.T) {
	result := runXSLT(t,
		`<root><item>A</item><item>B</item></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="root">
    <xsl:copy>
      <xsl:apply-templates/>
    </xsl:copy>
  </xsl:template>
  <xsl:template match="item">
    <entry><xsl:value-of select="."/></entry>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<root>") {
		t.Error("expected <root>")
	}
	if !strings.Contains(result, "<entry>A</entry>") {
		t.Error("expected <entry>A</entry>")
	}
}

// ========== AVTs ==========

func TestAVTStatic(t *testing.T) {
	// Static attributes should still work (regression).
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <a href="http://example.com">link</a>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `href="http://example.com"`) {
		t.Errorf("expected static attribute, got %s", result)
	}
}

func TestAVTExpression(t *testing.T) {
	result := runXSLT(t,
		`<root><item url="http://go.dev"/></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:for-each select="root/item">
      <a href="{@url}">link</a>
    </xsl:for-each>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `href="http://go.dev"`) {
		t.Errorf("expected AVT with @url, got %s", result)
	}
}

func TestAVTMixed(t *testing.T) {
	result := runXSLT(t,
		`<root><item id="42"/></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:for-each select="root/item">
      <div class="item-{@id}">content</div>
    </xsl:for-each>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `class="item-42"`) {
		t.Errorf("expected class='item-42', got %s", result)
	}
}

func TestAVTEscapedBraces(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <code style="{{color: red}}">text</code>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `style="{color: red}"`) {
		t.Errorf("expected escaped braces, got %s", result)
	}
}

// ========== xsl:attribute ==========

func TestAttributeStatic(t *testing.T) {
	result := runXSLT(t,
		`<root><item>hello</item></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <div>
      <xsl:attribute name="class" select="'main'"/>
      content
    </div>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `class="main"`) {
		t.Errorf("expected class='main', got %s", result)
	}
}

func TestAttributeBodyContent(t *testing.T) {
	result := runXSLT(t,
		`<root><color>red</color></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <div>
      <xsl:attribute name="style">color: <xsl:value-of select="root/color"/></xsl:attribute>
      content
    </div>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `style="color: red"`) {
		t.Errorf("expected style='color: red', got %s", result)
	}
}

func TestAttributeInCopy(t *testing.T) {
	result := runXSLT(t,
		`<root><item>text</item></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <xsl:copy>
      <xsl:attribute name="added">yes</xsl:attribute>
      <xsl:apply-templates/>
    </xsl:copy>
  </xsl:template>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/item"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `added="yes"`) {
		t.Errorf("expected added='yes' on copied element, got %s", result)
	}
	if !strings.Contains(result, "<item") {
		t.Error("expected <item> element from copy")
	}
}

// ========== xsl:call-template ==========

func TestCallTemplateSimple(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:call-template name="greet"/></out>
  </xsl:template>
  <xsl:template name="greet">
    <msg>Hello</msg>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<msg>Hello</msg>") {
		t.Errorf("expected <msg>Hello</msg>, got %s", result)
	}
}

func TestCallTemplateWithParam(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:call-template name="greet">
        <xsl:with-param name="who" select="'World'"/>
      </xsl:call-template>
    </out>
  </xsl:template>
  <xsl:template name="greet">
    <xsl:param name="who" select="'Default'"/>
    <msg>Hello <xsl:value-of select="$who"/></msg>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "Hello World") {
		t.Errorf("expected 'Hello World', got %s", result)
	}
}

func TestCallTemplateDefaultParam(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:call-template name="greet"/></out>
  </xsl:template>
  <xsl:template name="greet">
    <xsl:param name="who" select="'Default'"/>
    <msg><xsl:value-of select="$who"/></msg>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<msg>Default</msg>") {
		t.Errorf("expected <msg>Default</msg>, got %s", result)
	}
}

func TestCallTemplateVariableScoping(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="x" select="'outer'"/>
    <xsl:call-template name="inner"/>
    <out><xsl:value-of select="$x"/></out>
  </xsl:template>
  <xsl:template name="inner">
    <xsl:variable name="x" select="'inner'"/>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>outer</out>") {
		t.Errorf("expected variable scoping to protect outer $x, got %s", result)
	}
}

func TestCallTemplateRecursive(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:call-template name="countdown">
      <xsl:with-param name="n" select="3"/>
    </xsl:call-template></out>
  </xsl:template>
  <xsl:template name="countdown">
    <xsl:param name="n" select="0"/>
    <xsl:if test="$n &gt; 0">
      <xsl:value-of select="$n"/>
      <xsl:call-template name="countdown">
        <xsl:with-param name="n" select="$n - 1"/>
      </xsl:call-template>
    </xsl:if>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "321") {
		t.Errorf("expected '321' from recursive countdown, got %s", result)
	}
}

// ========== xsl:sort ==========

func TestSortAscending(t *testing.T) {
	result := runXSLT(t,
		`<data><item>Banana</item><item>Apple</item><item>Cherry</item></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="data/item">
        <xsl:sort select="."/>
        <v><xsl:value-of select="."/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	expected := "<v>Apple</v><v>Banana</v><v>Cherry</v>"
	if !strings.Contains(result, expected) {
		t.Errorf("expected sorted ascending: %s\ngot: %s", expected, result)
	}
}

func TestSortDescending(t *testing.T) {
	result := runXSLT(t,
		`<data><item>Banana</item><item>Apple</item><item>Cherry</item></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="data/item">
        <xsl:sort select="." order="descending"/>
        <v><xsl:value-of select="."/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	expected := "<v>Cherry</v><v>Banana</v><v>Apple</v>"
	if !strings.Contains(result, expected) {
		t.Errorf("expected sorted descending: %s\ngot: %s", expected, result)
	}
}

func TestSortNumeric(t *testing.T) {
	result := runXSLT(t,
		`<data><val>10</val><val>2</val><val>100</val></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="data/val">
        <xsl:sort select="." data-type="number"/>
        <v><xsl:value-of select="."/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	expected := "<v>2</v><v>10</v><v>100</v>"
	if !strings.Contains(result, expected) {
		t.Errorf("expected numeric sort: %s\ngot: %s", expected, result)
	}
}

func TestSortByAttribute(t *testing.T) {
	result := runXSLT(t,
		`<data><item name="C"/><item name="A"/><item name="B"/></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="data/item">
        <xsl:sort select="@name"/>
        <v><xsl:value-of select="@name"/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	expected := "<v>A</v><v>B</v><v>C</v>"
	if !strings.Contains(result, expected) {
		t.Errorf("expected sort by attr: %s\ngot: %s", expected, result)
	}
}

func TestSortInApplyTemplates(t *testing.T) {
	result := runXSLT(t,
		`<data><item>Z</item><item>A</item><item>M</item></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:apply-templates select="data/item">
        <xsl:sort select="."/>
      </xsl:apply-templates>
    </out>
  </xsl:template>
  <xsl:template match="item">
    <v><xsl:value-of select="."/></v>
  </xsl:template>
</xsl:stylesheet>`)
	expected := "<v>A</v><v>M</v><v>Z</v>"
	if !strings.Contains(result, expected) {
		t.Errorf("expected sorted apply-templates: %s\ngot: %s", expected, result)
	}
}

func TestSortNoSort(t *testing.T) {
	// Without sort, document order should be preserved (regression).
	result := runXSLT(t,
		`<data><val>3</val><val>1</val><val>2</val></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="data/val">
        <v><xsl:value-of select="."/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	expected := "<v>3</v><v>1</v><v>2</v>"
	if !strings.Contains(result, expected) {
		t.Errorf("expected document order preserved: %s\ngot: %s", expected, result)
	}
}

func TestSortMultipleKeys(t *testing.T) {
	result := runXSLT(t,
		`<data>
  <item cat="B" name="Zulu"/>
  <item cat="A" name="Beta"/>
  <item cat="A" name="Alpha"/>
  <item cat="B" name="Yankee"/>
</data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="data/item">
        <xsl:sort select="@cat"/>
        <xsl:sort select="@name"/>
        <v><xsl:value-of select="@cat"/>-<xsl:value-of select="@name"/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	// Should be: A-Alpha, A-Beta, B-Yankee, B-Zulu
	if !strings.Contains(result, "<v>A-Alpha</v><v>A-Beta</v><v>B-Yankee</v><v>B-Zulu</v>") {
		t.Errorf("expected multi-key sort, got %s", result)
	}
}

// ========== xsl:function + xsl:sequence ==========

func TestFunctionSimple(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:greet">
    <xsl:param name="name"/>
    <xsl:sequence select="concat('Hello ', $name)"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:greet('World')"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>Hello World</result>") {
		t.Errorf("expected <result>Hello World</result>, got %s", result)
	}
}

func TestFunctionDouble(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:double">
    <xsl:param name="n"/>
    <xsl:sequence select="$n * 2"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:double(21)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>42</result>") {
		t.Errorf("expected <result>42</result>, got %s", result)
	}
}

func TestFunctionMultipleParams(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:add">
    <xsl:param name="a"/>
    <xsl:param name="b"/>
    <xsl:sequence select="$a + $b"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:add(17, 25)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>42</result>") {
		t.Errorf("expected <result>42</result>, got %s", result)
	}
}

func TestFunctionProducingNodes(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn">
  <xsl:function name="f:wrap">
    <xsl:param name="text"/>
    <span><xsl:value-of select="$text"/></span>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:copy-of select="f:wrap('hello')"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<span>hello</span>") {
		t.Errorf("expected <span>hello</span>, got %s", result)
	}
}

func TestSequenceStandalone(t *testing.T) {
	result := runXSLT(t,
		`<root><val>42</val></root>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <result><xsl:sequence select="root/val"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<val>42</val>") {
		t.Errorf("expected <val>42</val> from xsl:sequence, got %s", result)
	}
}

// ========== as attribute (sequence types) ==========

func TestFunctionAsInteger(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:double" as="xs:integer">
    <xsl:param name="n" as="xs:integer"/>
    <xsl:sequence select="$n * 2"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:double(21)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>42</result>") {
		t.Errorf("expected <result>42</result>, got %s", result)
	}
}

func TestFunctionAsString(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:to-string" as="xs:string">
    <xsl:param name="v" as="xs:string"/>
    <xsl:sequence select="$v"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:to-string(42)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>42</result>") {
		t.Errorf("expected <result>42</result>, got %s", result)
	}
}

func TestFunctionAsBoolean(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:is-positive" as="xs:boolean">
    <xsl:param name="n"/>
    <xsl:sequence select="$n &gt; 0"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:is-positive(5)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>true</result>") {
		t.Errorf("expected <result>true</result>, got %s", result)
	}
}

func TestFunctionAsDouble(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn"
		  exclude-result-prefixes="f">
  <xsl:function name="f:half" as="xs:double">
    <xsl:param name="n" as="xs:double"/>
    <xsl:sequence select="$n div 2"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:half(7)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>3.5</result>") {
		t.Errorf("expected <result>3.5</result>, got %s", result)
	}
}

func TestFunctionAsNodeStar(t *testing.T) {
	result := runXSLT(t,
		`<root><a>1</a><b>2</b></root>`,
		`<xsl:stylesheet version="2.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:f="http://example.com/fn">
  <xsl:function name="f:children" as="node()*">
    <xsl:param name="ctx" as="node()"/>
    <xsl:sequence select="$ctx/*"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:copy-of select="f:children(root)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<a>1</a>") || !strings.Contains(result, "<b>2</b>") {
		t.Errorf("expected <a>1</a><b>2</b>, got %s", result)
	}
}

func TestFunctionCardinalityError(t *testing.T) {
	// as="xs:integer" (exactly 1) but empty sequence from body.
	xmlData := `<root/>`
	xslt := `<xsl:stylesheet version="2.0"
	  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
	  xmlns:f="http://example.com/fn">
  <xsl:function name="f:empty" as="xs:integer">
    <xsl:sequence select="()"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:empty()"/></result>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Transform(ss, sourceDoc)
	if err == nil {
		t.Fatal("expected cardinality error, got nil")
	}
	if !strings.Contains(err.Error(), "expected exactly 1 item") {
		t.Errorf("expected cardinality error message, got: %s", err.Error())
	}
}

func TestFunctionTypeError(t *testing.T) {
	// as="xs:integer" on param but passed a non-numeric string.
	xmlData := `<root/>`
	xslt := `<xsl:stylesheet version="2.0"
	  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
	  xmlns:f="http://example.com/fn">
  <xsl:function name="f:inc" as="xs:integer">
    <xsl:param name="n" as="xs:integer"/>
    <xsl:sequence select="$n + 1"/>
  </xsl:function>
  <xsl:template match="/">
    <result><xsl:value-of select="f:inc('abc')"/></result>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Transform(ss, sourceDoc)
	if err == nil {
		t.Fatal("expected type error, got nil")
	}
	if !strings.Contains(err.Error(), "xs:integer") || !strings.Contains(err.Error(), "param") {
		t.Errorf("expected type error mentioning xs:integer and param, got: %s", err.Error())
	}
}

// ========== xsl:element ==========

func TestElementSimple(t *testing.T) {
	result := runXSLT(t,
		`<root><tag>div</tag></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:element name="section">
      <xsl:element name="{root/tag}">content</xsl:element>
    </xsl:element>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<section>") {
		t.Errorf("expected <section>, got %s", result)
	}
	if !strings.Contains(result, "<div>content</div>") {
		t.Errorf("expected <div>content</div>, got %s", result)
	}
}

func TestElementWithAttribute(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:element name="a">
      <xsl:attribute name="href">http://example.com</xsl:attribute>
      link
    </xsl:element>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `<a href="http://example.com">`) {
		t.Errorf("expected <a href=...>, got %s", result)
	}
}

func TestElementWithChildren(t *testing.T) {
	result := runXSLT(t,
		`<data><item>A</item><item>B</item></data>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:element name="list">
      <xsl:for-each select="data/item">
        <xsl:element name="entry"><xsl:value-of select="."/></xsl:element>
      </xsl:for-each>
    </xsl:element>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<list>") || !strings.Contains(result, "<entry>A</entry>") || !strings.Contains(result, "<entry>B</entry>") {
		t.Errorf("expected <list><entry>A</entry><entry>B</entry></list>, got %s", result)
	}
}

// ========== xsl:comment ==========

func TestCommentStatic(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:comment>this is a comment</xsl:comment></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<!--this is a comment-->") {
		t.Errorf("expected <!--this is a comment-->, got %s", result)
	}
}

func TestCommentDynamic(t *testing.T) {
	result := runXSLT(t,
		`<root><msg>generated</msg></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:comment>comment: <xsl:value-of select="root/msg"/></xsl:comment></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<!--comment: generated-->") {
		t.Errorf("expected <!--comment: generated-->, got %s", result)
	}
}

// ========== xsl:processing-instruction ==========

func TestProcessingInstruction(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:processing-instruction name="xml-stylesheet">type="text/xsl" href="style.xsl"</xsl:processing-instruction>
    <out/>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `<?xml-stylesheet type="text/xsl" href="style.xsl"?>`) {
		t.Errorf("expected <?xml-stylesheet ...?>, got %s", result)
	}
}

func TestProcessingInstructionDynamic(t *testing.T) {
	result := runXSLT(t,
		`<root><target>my-app</target></root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:processing-instruction name="{root/target}">version="1.0"</xsl:processing-instruction>
    <out/>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `<?my-app version="1.0"?>`) {
		t.Errorf("expected <?my-app ...?>, got %s", result)
	}
}

// ========== xsl:message ==========

func TestMessageNoTerminate(t *testing.T) {
	xmlData := `<root/>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:message>debug info</xsl:message>
    <out>ok</out>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}

	// Capture message output.
	var msgBuf strings.Builder
	resultDoc, err := TransformWithOptions(ss, sourceDoc, TransformOptions{
		MessageWriter: &msgBuf,
	})
	if err != nil {
		t.Fatal("should not error without terminate:", err)
	}
	result := SerializeResult(resultDoc.Document)
	if !strings.Contains(result, "<out>ok</out>") {
		t.Errorf("expected <out>ok</out>, got %s", result)
	}
	if !strings.Contains(msgBuf.String(), "debug info") {
		t.Errorf("expected message 'debug info', got %q", msgBuf.String())
	}
}

func TestMessageTerminate(t *testing.T) {
	xmlData := `<root/>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:message terminate="yes">fatal error</xsl:message>
    <out>should not appear</out>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Transform(ss, sourceDoc)
	if err == nil {
		t.Fatal("expected error from terminate='yes'")
	}
	if !strings.Contains(err.Error(), "fatal error") {
		t.Errorf("expected 'fatal error' in message, got: %s", err.Error())
	}
}

// ========== xsl:number ==========

func TestNumberValue(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="3" format="1"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>3</out>") {
		t.Errorf("expected <out>3</out>, got %s", result)
	}
}

func TestNumberValueAlpha(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="4" format="a"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>d</out>") {
		t.Errorf("expected <out>d</out>, got %s", result)
	}
}

func TestNumberValueAlphaUpper(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="4" format="A"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>D</out>") {
		t.Errorf("expected <out>D</out>, got %s", result)
	}
}

func TestNumberValueRomanLower(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="9" format="i"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>ix</out>") {
		t.Errorf("expected <out>ix</out>, got %s", result)
	}
}

func TestNumberValueRomanUpper(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="14" format="I"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>XIV</out>") {
		t.Errorf("expected <out>XIV</out>, got %s", result)
	}
}

func TestNumberValueZeroPadded(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="3" format="01"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>03</out>") {
		t.Errorf("expected <out>03</out>, got %s", result)
	}
}

func TestNumberSingle(t *testing.T) {
	result := runXSLT(t,
		`<list><item>A</item><item>B</item><item>C</item></list>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="list/item"/></out>
  </xsl:template>
  <xsl:template match="item">
    <n><xsl:number/><xsl:text>:</xsl:text><xsl:value-of select="."/></n>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<n>1:A</n>") {
		t.Errorf("expected <n>1:A</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>2:B</n>") {
		t.Errorf("expected <n>2:B</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>3:C</n>") {
		t.Errorf("expected <n>3:C</n>, got %s", result)
	}
}

func TestNumberSingleCount(t *testing.T) {
	result := runXSLT(t,
		`<list><item>A</item><other>X</other><item>B</item><item>C</item></list>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="list/item"/></out>
  </xsl:template>
  <xsl:template match="item">
    <n><xsl:number count="item"/><xsl:text>:</xsl:text><xsl:value-of select="."/></n>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<n>1:A</n>") {
		t.Errorf("expected <n>1:A</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>2:B</n>") {
		t.Errorf("expected <n>2:B</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>3:C</n>") {
		t.Errorf("expected <n>3:C</n>, got %s", result)
	}
}

func TestNumberMultiple(t *testing.T) {
	result := runXSLT(t,
		`<doc>
  <chapter><title>Ch1</title>
    <section><title>S1</title></section>
    <section><title>S2</title></section>
  </chapter>
  <chapter><title>Ch2</title>
    <section><title>S1</title></section>
  </chapter>
</doc>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="//section"/></out>
  </xsl:template>
  <xsl:template match="section">
    <n><xsl:number level="multiple" count="chapter|section" format="1.1"/><xsl:text>:</xsl:text><xsl:value-of select="title"/></n>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<n>1.1:S1</n>") {
		t.Errorf("expected 1.1:S1, got %s", result)
	}
	if !strings.Contains(result, "<n>1.2:S2</n>") {
		t.Errorf("expected 1.2:S2, got %s", result)
	}
	if !strings.Contains(result, "<n>2.1:S1</n>") {
		t.Errorf("expected 2.1:S1, got %s", result)
	}
}

func TestNumberAny(t *testing.T) {
	result := runXSLT(t,
		`<doc>
  <chapter><item>A</item><item>B</item></chapter>
  <chapter><item>C</item></chapter>
</doc>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="//item"/></out>
  </xsl:template>
  <xsl:template match="item">
    <n><xsl:number level="any" count="item"/><xsl:text>:</xsl:text><xsl:value-of select="."/></n>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<n>1:A</n>") {
		t.Errorf("expected <n>1:A</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>2:B</n>") {
		t.Errorf("expected <n>2:B</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>3:C</n>") {
		t.Errorf("expected <n>3:C</n>, got %s", result)
	}
}

func TestNumberAnyFrom(t *testing.T) {
	result := runXSLT(t,
		`<doc>
  <chapter><item>A</item><item>B</item></chapter>
  <chapter><item>C</item><item>D</item></chapter>
</doc>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="//item"/></out>
  </xsl:template>
  <xsl:template match="item">
    <n><xsl:number level="any" count="item" from="chapter"/><xsl:text>:</xsl:text><xsl:value-of select="."/></n>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<n>1:A</n>") {
		t.Errorf("expected <n>1:A</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>2:B</n>") {
		t.Errorf("expected <n>2:B</n>, got %s", result)
	}
	if !strings.Contains(result, "<n>1:C</n>") {
		t.Errorf("expected <n>1:C</n> (restart after chapter), got %s", result)
	}
	if !strings.Contains(result, "<n>2:D</n>") {
		t.Errorf("expected <n>2:D</n>, got %s", result)
	}
}

func TestNumberFormatPunctuation(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:number value="3" format="1."/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>3.</out>") {
		t.Errorf("expected <out>3.</out>, got %s", result)
	}
}

func TestNumberSelect(t *testing.T) {
	result := runXSLT(t,
		`<doc><a><item>X</item></a><b><item>Y</item><item>Z</item></b></doc>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:for-each select="//item">
        <n><xsl:number select="." count="item"/></n>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	// a/item → 1, b/item[1] → 1, b/item[2] → 2
	if !strings.Contains(result, "<n>1</n><n>1</n><n>2</n>") {
		t.Errorf("expected 1,1,2 with select, got %s", result)
	}
}

// ========== xsl:include ==========

func TestIncludeBasic(t *testing.T) {
	dir := t.TempDir()
	// helpers.xsl defines a template for "item"
	writeFile(t, dir, "helpers.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <entry><xsl:value-of select="."/></entry>
  </xsl:template>
</xsl:stylesheet>`)

	// main.xsl includes helpers.xsl and defines a template for "/"
	writeFile(t, dir, "main.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:include href="helpers.xsl"/>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/item"/></out>
  </xsl:template>
</xsl:stylesheet>`)

	ss, err := CompileFile(filepath.Join(dir, "main.xsl"))
	if err != nil {
		t.Fatal(err)
	}
	sourceDoc, _ := goxml.Parse(strings.NewReader(`<root><item>A</item><item>B</item></root>`))
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)
	if !strings.Contains(result, "<entry>A</entry>") || !strings.Contains(result, "<entry>B</entry>") {
		t.Errorf("expected included template to produce <entry> elements, got %s", result)
	}
}

// ========== xsl:import ==========

func TestImportOverride(t *testing.T) {
	dir := t.TempDir()
	// base.xsl defines a template for "item" that produces <base>
	writeFile(t, dir, "base.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <base><xsl:value-of select="."/></base>
  </xsl:template>
</xsl:stylesheet>`)

	// main.xsl imports base.xsl and overrides the "item" template
	writeFile(t, dir, "main.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:import href="base.xsl"/>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/item"/></out>
  </xsl:template>
  <xsl:template match="item">
    <override><xsl:value-of select="."/></override>
  </xsl:template>
</xsl:stylesheet>`)

	ss, err := CompileFile(filepath.Join(dir, "main.xsl"))
	if err != nil {
		t.Fatal(err)
	}
	sourceDoc, _ := goxml.Parse(strings.NewReader(`<root><item>X</item></root>`))
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)
	if !strings.Contains(result, "<override>X</override>") {
		t.Errorf("expected main template to override imported one, got %s", result)
	}
	if strings.Contains(result, "<base>") {
		t.Error("imported template should be overridden, but <base> appeared")
	}
}

func TestImportPrecedence(t *testing.T) {
	dir := t.TempDir()
	// base.xsl: lower precedence template for "item"
	writeFile(t, dir, "base.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <from-base><xsl:value-of select="."/></from-base>
  </xsl:template>
</xsl:stylesheet>`)

	// utils.xsl: imported second, so higher precedence than base but lower than main
	writeFile(t, dir, "utils.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <from-utils><xsl:value-of select="."/></from-utils>
  </xsl:template>
</xsl:stylesheet>`)

	// main.xsl: imports both but does NOT define its own "item" template
	writeFile(t, dir, "main.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:import href="base.xsl"/>
  <xsl:import href="utils.xsl"/>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/item"/></out>
  </xsl:template>
</xsl:stylesheet>`)

	ss, err := CompileFile(filepath.Join(dir, "main.xsl"))
	if err != nil {
		t.Fatal(err)
	}
	sourceDoc, _ := goxml.Parse(strings.NewReader(`<root><item>X</item></root>`))
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)
	// utils.xsl was imported second → higher precedence than base.xsl
	if !strings.Contains(result, "<from-utils>X</from-utils>") {
		t.Errorf("expected utils.xsl (higher import precedence) to win, got %s", result)
	}
}

func TestImportFallback(t *testing.T) {
	dir := t.TempDir()
	// base.xsl defines a template for "item"
	writeFile(t, dir, "base.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="item">
    <from-base><xsl:value-of select="."/></from-base>
  </xsl:template>
</xsl:stylesheet>`)

	// main.xsl imports base.xsl but does NOT override "item"
	writeFile(t, dir, "main.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:import href="base.xsl"/>
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/item"/></out>
  </xsl:template>
</xsl:stylesheet>`)

	ss, err := CompileFile(filepath.Join(dir, "main.xsl"))
	if err != nil {
		t.Fatal(err)
	}
	sourceDoc, _ := goxml.Parse(strings.NewReader(`<root><item>Y</item></root>`))
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	t.Logf("Result: %s", result)
	if !strings.Contains(result, "<from-base>Y</from-base>") {
		t.Errorf("expected imported template to be used as fallback, got %s", result)
	}
}

// ========== cycle detection ==========

func TestImportCycleDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:import href="b.xsl"/>
  <xsl:template match="/"><out/></xsl:template>
</xsl:stylesheet>`)
	writeFile(t, dir, "b.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:import href="a.xsl"/>
</xsl:stylesheet>`)

	_, err := CompileFile(filepath.Join(dir, "a.xsl"))
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected 'circular' in error, got: %s", err.Error())
	}
}

func TestIncludeCycleDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:include href="b.xsl"/>
  <xsl:template match="/"><out/></xsl:template>
</xsl:stylesheet>`)
	writeFile(t, dir, "b.xsl", `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:include href="a.xsl"/>
</xsl:stylesheet>`)

	_, err := CompileFile(filepath.Join(dir, "a.xsl"))
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected 'circular' in error, got: %s", err.Error())
	}
}

// ========== Compile() without path ==========

func TestCompileImportWithoutPath(t *testing.T) {
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:import href="other.xsl"/>
  <xsl:template match="/"><out/></xsl:template>
</xsl:stylesheet>`
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	_, err := Compile(xsltDoc)
	if err == nil {
		t.Fatal("expected error when using import with Compile()")
	}
	if !strings.Contains(err.Error(), "CompileFile") {
		t.Errorf("expected error to mention CompileFile, got: %s", err.Error())
	}
}

// ========== helpers ==========

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// ========== match pattern predicates ==========

func TestPredicateAttributeValue(t *testing.T) {
	result := runXSLT(t,
		`<catalog>
  <book lang="en">English Book</book>
  <book lang="de">German Book</book>
</catalog>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="catalog/book"/></out>
  </xsl:template>
  <xsl:template match="book[@lang='en']">
    <en><xsl:value-of select="."/></en>
  </xsl:template>
  <xsl:template match="book[@lang='de']">
    <de><xsl:value-of select="."/></de>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<en>English Book</en>") {
		t.Errorf("expected <en>English Book</en>, got %s", result)
	}
	if !strings.Contains(result, "<de>German Book</de>") {
		t.Errorf("expected <de>German Book</de>, got %s", result)
	}
}

func TestPredicateFilter(t *testing.T) {
	result := runXSLT(t,
		`<items>
  <item type="a">Alpha</item>
  <item type="b">Beta</item>
  <item type="a">Again</item>
</items>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="items/item"/></out>
  </xsl:template>
  <xsl:template match="item[@type='a']">
    <found><xsl:value-of select="."/></found>
  </xsl:template>
  <xsl:template match="item">
    <skip/>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<found>Alpha</found>") {
		t.Errorf("expected <found>Alpha</found>, got %s", result)
	}
	if !strings.Contains(result, "<found>Again</found>") {
		t.Errorf("expected <found>Again</found>, got %s", result)
	}
	if strings.Contains(result, "Beta") {
		t.Errorf("should not contain Beta, got %s", result)
	}
}

func TestPredicateWildcardWithAttribute(t *testing.T) {
	result := runXSLT(t,
		`<root>
  <a id="1">A</a>
  <b>B</b>
  <c id="2">C</c>
</root>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="root/*"/></out>
  </xsl:template>
  <xsl:template match="*[@id]">
    <has-id><xsl:value-of select="."/></has-id>
  </xsl:template>
  <xsl:template match="*">
    <no-id><xsl:value-of select="."/></no-id>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<has-id>A</has-id>") {
		t.Errorf("expected <has-id>A</has-id>, got %s", result)
	}
	if !strings.Contains(result, "<has-id>C</has-id>") {
		t.Errorf("expected <has-id>C</has-id>, got %s", result)
	}
	if !strings.Contains(result, "<no-id>B</no-id>") {
		t.Errorf("expected <no-id>B</no-id>, got %s", result)
	}
}

func TestPredicateMultiple(t *testing.T) {
	result := runXSLT(t,
		`<items>
  <item type="a" status="active">Match</item>
  <item type="a" status="inactive">NoMatch1</item>
  <item type="b" status="active">NoMatch2</item>
</items>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="items/item"/></out>
  </xsl:template>
  <xsl:template match="item[@type='a'][@status='active']">
    <both><xsl:value-of select="."/></both>
  </xsl:template>
  <xsl:template match="item">
    <other><xsl:value-of select="."/></other>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<both>Match</both>") {
		t.Errorf("expected <both>Match</both>, got %s", result)
	}
	if !strings.Contains(result, "<other>NoMatch1</other>") {
		t.Errorf("expected <other>NoMatch1</other>, got %s", result)
	}
	if !strings.Contains(result, "<other>NoMatch2</other>") {
		t.Errorf("expected <other>NoMatch2</other>, got %s", result)
	}
}

func TestPredicateWithPath(t *testing.T) {
	result := runXSLT(t,
		`<catalog>
  <book lang="en"><title>English</title></book>
  <book lang="de"><title>German</title></book>
</catalog>`,
		`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out><xsl:apply-templates select="catalog/book/title"/></out>
  </xsl:template>
  <xsl:template match="book[@lang='en']/title">
    <en-title><xsl:value-of select="."/></en-title>
  </xsl:template>
  <xsl:template match="title">
    <other-title><xsl:value-of select="."/></other-title>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<en-title>English</en-title>") {
		t.Errorf("expected <en-title>English</en-title>, got %s", result)
	}
	if !strings.Contains(result, "<other-title>German</other-title>") {
		t.Errorf("expected <other-title>German</other-title>, got %s", result)
	}
}

func TestMessageHandler(t *testing.T) {
	xmlData := `<root/>`
	xslt := `<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:message>hello callback</xsl:message>
    <out>ok</out>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, _ := goxml.Parse(strings.NewReader(xmlData))
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}

	var captured string
	resultDoc, err := TransformWithOptions(ss, sourceDoc, TransformOptions{
		MessageHandler: func(text string, terminate bool) {
			captured = text
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := SerializeResult(resultDoc.Document)
	if !strings.Contains(result, "<out>ok</out>") {
		t.Errorf("expected <out>ok</out>, got %s", result)
	}
	if captured != "hello callback" {
		t.Errorf("expected captured='hello callback', got %q", captured)
	}
}

// ========== xsl:map + xsl:map-entry ==========

func TestMapBasic(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="3.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:map="http://www.w3.org/2005/xpath-functions/map"
		  exclude-result-prefixes="map">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map>
        <xsl:map-entry key="'greeting'" select="'Hello'"/>
        <xsl:map-entry key="'name'" select="'World'"/>
      </xsl:map>
    </xsl:variable>
    <result>
      <xsl:value-of select="map:get($m, 'greeting')"/>
      <xsl:text> </xsl:text>
      <xsl:value-of select="map:get($m, 'name')"/>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>Hello World</result>") {
		t.Errorf("expected <result>Hello World</result>, got %s", result)
	}
}

func TestMapSize(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="3.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:map="http://www.w3.org/2005/xpath-functions/map"
		  exclude-result-prefixes="map">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map>
        <xsl:map-entry key="'a'" select="1"/>
        <xsl:map-entry key="'b'" select="2"/>
        <xsl:map-entry key="'c'" select="3"/>
      </xsl:map>
    </xsl:variable>
    <result><xsl:value-of select="map:size($m)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>3</result>") {
		t.Errorf("expected <result>3</result>, got %s", result)
	}
}

func TestMapContains(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="3.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:map="http://www.w3.org/2005/xpath-functions/map"
		  exclude-result-prefixes="map">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map>
        <xsl:map-entry key="'x'" select="42"/>
      </xsl:map>
    </xsl:variable>
    <result>
      <has-x><xsl:value-of select="map:contains($m, 'x')"/></has-x>
      <has-y><xsl:value-of select="map:contains($m, 'y')"/></has-y>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<has-x>true</has-x>") {
		t.Errorf("expected <has-x>true</has-x>, got %s", result)
	}
	if !strings.Contains(result, "<has-y>false</has-y>") {
		t.Errorf("expected <has-y>false</has-y>, got %s", result)
	}
}

func TestMapEntryWithBody(t *testing.T) {
	result := runXSLT(t,
		`<root><val>42</val></root>`,
		`<xsl:stylesheet version="3.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:map="http://www.w3.org/2005/xpath-functions/map"
		  exclude-result-prefixes="map">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map>
        <xsl:map-entry key="'content'">
          <item><xsl:value-of select="root/val"/></item>
        </xsl:map-entry>
      </xsl:map>
    </xsl:variable>
    <result><xsl:copy-of select="map:get($m, 'content')"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<item>42</item>") {
		t.Errorf("expected <item>42</item>, got %s", result)
	}
}

func TestMapDynamicKeys(t *testing.T) {
	result := runXSLT(t,
		`<data><item key="color" value="red"/><item key="size" value="large"/></data>`,
		`<xsl:stylesheet version="3.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:map="http://www.w3.org/2005/xpath-functions/map"
		  exclude-result-prefixes="map">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map>
        <xsl:for-each select="data/item">
          <xsl:map-entry key="@key" select="@value"/>
        </xsl:for-each>
      </xsl:map>
    </xsl:variable>
    <result>
      <color><xsl:value-of select="map:get($m, 'color')"/></color>
      <size><xsl:value-of select="map:get($m, 'size')"/></size>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<color>red</color>") {
		t.Errorf("expected <color>red</color>, got %s", result)
	}
	if !strings.Contains(result, "<size>large</size>") {
		t.Errorf("expected <size>large</size>, got %s", result)
	}
}

func TestMapEntryMissingKey(t *testing.T) {
	xslt := `<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map>
        <xsl:map-entry select="'val'"/>
      </xsl:map>
    </xsl:variable>
  </xsl:template>
</xsl:stylesheet>`
	xsltDoc, _ := goxml.Parse(strings.NewReader(xslt))
	_, err := Compile(xsltDoc)
	if err == nil {
		t.Fatal("expected compilation error for missing key attribute")
	}
	if !strings.Contains(err.Error(), "missing key") {
		t.Errorf("expected 'missing key' in error, got: %s", err.Error())
	}
}

func TestMapEmpty(t *testing.T) {
	result := runXSLT(t,
		`<root/>`,
		`<xsl:stylesheet version="3.0"
		  xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:map="http://www.w3.org/2005/xpath-functions/map"
		  exclude-result-prefixes="map">
  <xsl:template match="/">
    <xsl:variable name="m">
      <xsl:map/>
    </xsl:variable>
    <result><xsl:value-of select="map:size($m)"/></result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result>0</result>") {
		t.Errorf("expected <result>0</result>, got %s", result)
	}
}

func TestForEachGroupBasic(t *testing.T) {
	result := runXSLT(t,
		`<books>
  <book category="fiction"><title>A</title></book>
  <book category="tech"><title>B</title></book>
  <book category="fiction"><title>C</title></book>
</books>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <result>
      <xsl:for-each-group select="books/book" group-by="@category">
        <group key="{current-grouping-key()}">
          <xsl:value-of select="count(current-group())"/>
        </group>
      </xsl:for-each-group>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `<group key="fiction">2</group>`) {
		t.Errorf("expected <group key=\"fiction\">2</group>, got %s", result)
	}
	if !strings.Contains(result, `<group key="tech">1</group>`) {
		t.Errorf("expected <group key=\"tech\">1</group>, got %s", result)
	}
	// Verify order: fiction comes first (first occurrence).
	fictionIdx := strings.Index(result, `key="fiction"`)
	techIdx := strings.Index(result, `key="tech"`)
	if fictionIdx < 0 || techIdx < 0 || fictionIdx > techIdx {
		t.Errorf("expected fiction group before tech group, got %s", result)
	}
}

func TestForEachGroupCurrentGroup(t *testing.T) {
	result := runXSLT(t,
		`<books>
  <book category="fiction"><title>A</title></book>
  <book category="tech"><title>B</title></book>
  <book category="fiction"><title>C</title></book>
</books>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <result>
      <xsl:for-each-group select="books/book" group-by="@category">
        <group key="{current-grouping-key()}">
          <xsl:for-each select="current-group()">
            <item><xsl:value-of select="title"/></item>
          </xsl:for-each>
        </group>
      </xsl:for-each-group>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `<item>A</item>`) {
		t.Errorf("expected <item>A</item>, got %s", result)
	}
	if !strings.Contains(result, `<item>B</item>`) {
		t.Errorf("expected <item>B</item>, got %s", result)
	}
	if !strings.Contains(result, `<item>C</item>`) {
		t.Errorf("expected <item>C</item>, got %s", result)
	}
	// Fiction group should contain A and C.
	fictionStart := strings.Index(result, `key="fiction"`)
	techStart := strings.Index(result, `key="tech"`)
	fictionSection := result[fictionStart:techStart]
	if !strings.Contains(fictionSection, "<item>A</item>") || !strings.Contains(fictionSection, "<item>C</item>") {
		t.Errorf("fiction group should contain A and C, got %s", fictionSection)
	}
	if strings.Contains(fictionSection, "<item>B</item>") {
		t.Errorf("fiction group should NOT contain B, got %s", fictionSection)
	}
}

func TestForEachGroupEmpty(t *testing.T) {
	result := runXSLT(t,
		`<books/>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <result>
      <xsl:for-each-group select="books/book" group-by="@category">
        <group key="{current-grouping-key()}"/>
      </xsl:for-each-group>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<result />") && !strings.Contains(result, "<result/>") {
		t.Errorf("expected empty result, got %s", result)
	}
	if strings.Contains(result, "<group") {
		t.Errorf("expected no groups for empty input, got %s", result)
	}
}

func TestForEachGroupCurrentGroupChildPredicate(t *testing.T) {
	result := runXSLT(t,
		`<MeDaPro>
  <Row>
    <Artikelklasse_ID><![CDATA[ID1]]></Artikelklasse_ID>
    <InvariantAttributeName1>Attr1</InvariantAttributeName1>
    <InvariantAttributeValue1>Val1</InvariantAttributeValue1>
    <InvariantAttributeUnit1>mm</InvariantAttributeUnit1>
    <Pikto1 Name="Pikto1" />
    <Pikto2 Name="Pikto2"><![CDATA[\_img\VDE.png]]></Pikto2>
    <Pikto3 Name="Pikto3"><![CDATA[\_img\EAC.png]]></Pikto3>
    <Pikto4 Name="Pikto4" />
  </Row>
  <Row>
    <Artikelklasse_ID><![CDATA[ID1]]></Artikelklasse_ID>
    <InvariantAttributeName1>Attr1</InvariantAttributeName1>
    <InvariantAttributeValue1>Val2</InvariantAttributeValue1>
    <InvariantAttributeUnit1>mm</InvariantAttributeUnit1>
    <Pikto1 Name="Pikto1" />
    <Pikto2 Name="Pikto2"><![CDATA[\_img\VDE.png]]></Pikto2>
    <Pikto3 Name="Pikto3" />
    <Pikto4 Name="Pikto4" />
  </Row>
</MeDaPro>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="*[starts-with(local-name(), 'Pikto')]">
    <piktogramm datei="{.}"/>
  </xsl:template>
  <xsl:template match="MeDaPro">
    <result>
      <xsl:for-each-group select="Row" group-by="Artikelklasse_ID">
        <xsl:variable name="row" select="."/>
        <xsl:variable name="l" select="string-length('InvariantAttributeName') + 1"/>
        <group key="{current-grouping-key()}">
          <!-- Loop like the real stylesheet: for $i in *[starts-with(...)]/local-name() -->
          <tab>
            <xsl:for-each select="
                for $i in *[starts-with(local-name(), 'InvariantAttributeName')]/local-name()
                return replace($i, '[a-zA-Z]', '')">
              <xsl:variable name="c" select="."/>
              <xsl:variable name="eltname" select="concat('InvariantAttributeName', $c)"/>
              <xsl:variable name="eltvalue" select="concat('InvariantAttributeValue', $c)"/>
              <xsl:variable name="name" select="$row/*[local-name() = $eltname]"/>
              <xsl:variable name="value" select="$row/*[local-name() = $eltvalue]"/>
              <xsl:if test="string-length($name) > 0">
                <attr name="{$name}" value="{$value}"/>
              </xsl:if>
            </xsl:for-each>
          </tab>
          <!-- Now the piktogramme variable -->
          <xsl:variable name="piktos">
            <xsl:apply-templates select="current-group()/*[starts-with(local-name(), 'Pikto') and string-length(.) > 0]"/>
          </xsl:variable>
          <piktogramme><xsl:copy-of select="$piktos"/></piktogramme>
        </group>
      </xsl:for-each-group>
    </result>
  </xsl:template>
</xsl:stylesheet>`)
	t.Logf("Result: %s", result)
	if !strings.Contains(result, `datei="\_img\VDE.png"`) {
		t.Errorf("expected piktogramm with VDE.png, got %s", result)
	}
	if !strings.Contains(result, `datei="\_img\EAC.png"`) {
		t.Errorf("expected piktogramm with EAC.png, got %s", result)
	}
}

func TestRealFlexaStylesheet(t *testing.T) {
	xmlFile := "/Users/patrick/work/projekte/2025/2025-03/Arbeitsverzeichnis/rohdaten.xml"
	xsltFile := "/Users/patrick/work/projekte/2025/2025-03/Arbeitsverzeichnis/flexa2data.xslt"

	// Change to the working directory so relative doc() calls work
	origDir, _ := os.Getwd()
	os.Chdir("/Users/patrick/work/projekte/2025/2025-03/Arbeitsverzeichnis")
	defer os.Chdir(origDir)

	xmlReader, err := os.Open(xmlFile)
	if err != nil {
		t.Skip("real data not available:", err)
	}
	defer xmlReader.Close()

	xsltReader, err := os.Open(xsltFile)
	if err != nil {
		t.Skip("real stylesheet not available:", err)
	}
	defer xsltReader.Close()

	sourceDoc, err := goxml.Parse(xmlReader)
	if err != nil {
		t.Fatal("parsing XML:", err)
	}
	xsltDoc, err := goxml.Parse(xsltReader)
	if err != nil {
		t.Fatal("parsing XSLT:", err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal("compiling:", err)
	}

	var messages []string
	opts := TransformOptions{
		MessageHandler: func(text string, terminate bool) {
			messages = append(messages, text)
		},
	}
	result, err := TransformWithOptions(ss, sourceDoc, opts)
	if err != nil {
		t.Fatal("transforming:", err)
	}
	for _, msg := range messages {
		t.Logf("XSLT message: %s", msg)
	}
	// Check the secondary document (output.xml from result-document).
	for href, doc := range result.SecondaryDocuments {
		output := doc.ToXML()
		t.Logf("Secondary %s (len=%d): %s", href, len(output), output[:min(len(output), 500)])
		// Dump the eigenschaften section for debugging.
		if idx := strings.Index(output, "<eigenschaften>"); idx >= 0 {
			end := strings.Index(output[idx:], "</eigenschaften>")
			if end > 0 {
				t.Logf("eigenschaften section: %s", output[idx:idx+end+len("</eigenschaften>")])
			}
		}
		if strings.Contains(output, "<eigenschaften />") || strings.Contains(output, "<eigenschaften/>") {
			t.Log("eigenschaften is EMPTY")
		}
		if href == "output.xml" && strings.Contains(output, "<piktogramme />") {
			t.Error("piktogramme is empty — $variable/child path not working")
		}
		if href == "output.xml" && strings.Contains(output, "<piktogramme>") {
			t.Log("piktogramme has content — fix works!")
		}
	}
}

// ========== Variable with body content is a document node ==========

func TestVariableBodyIsDocumentNode(t *testing.T) {
	xmlData := `<root><a>1</a><b>2</b><c>3</c></root>`
	xslt := `<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="items">
      <xsl:for-each select="root/*">
        <item val="{.}"/>
      </xsl:for-each>
    </xsl:variable>
    <out count="{count($items/item)}">
      <xsl:for-each select="$items/item">
        <v><xsl:value-of select="@val"/></v>
      </xsl:for-each>
    </out>
  </xsl:template>
</xsl:stylesheet>`
	result := runXSLT(t, xmlData, xslt)
	t.Log("Result:", result)
	if !strings.Contains(result, `count="3"`) {
		t.Error("expected count=3, got:", result)
	}
	if !strings.Contains(result, "<v>1</v><v>2</v><v>3</v>") {
		t.Error("expected <v>1</v><v>2</v><v>3</v>, got:", result)
	}
}

func TestVariableBodyStringValue(t *testing.T) {
	xmlData := `<root/>`
	xslt := `<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="msg">hello world</xsl:variable>
    <out><xsl:value-of select="$msg"/></out>
  </xsl:template>
</xsl:stylesheet>`
	result := runXSLT(t, xmlData, xslt)
	t.Log("Result:", result)
	if !strings.Contains(result, "hello world") {
		t.Error("expected 'hello world', got:", result)
	}
}

func TestVariableBodyDocNodePathAndSome(t *testing.T) {
	xmlData := `<root><a val="x"/><b val=""/><c val="y"/></root>`
	xslt := `<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <xsl:variable name="data">
      <container>
        <xsl:for-each select="root/*">
          <item attr="{@val}"/>
        </xsl:for-each>
      </container>
    </xsl:variable>
    <xsl:variable name="hasNonEmpty" select="
        some $i in $data/container/item/@attr
          satisfies normalize-space($i) != ''"/>
    <xsl:variable name="count" select="count($data/container/item/@attr)"/>
    <out has="{$hasNonEmpty}" count="{$count}"/>
  </xsl:template>
</xsl:stylesheet>`
	result := runXSLT(t, xmlData, xslt)
	t.Log("Result:", result)
	if !strings.Contains(result, `has="true"`) {
		t.Error("expected has=true, got:", result)
	}
	if !strings.Contains(result, `count="3"`) {
		t.Error("expected count=3, got:", result)
	}
}

// ========== xsl:result-document ==========

func TestResultDocumentBasic(t *testing.T) {
	xmlData := `<root><item>Hello</item></root>`
	xslt := `<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <main>primary</main>
    <xsl:result-document href="secondary.xml">
      <secondary><xsl:value-of select="root/item"/></secondary>
    </xsl:result-document>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, err := goxml.Parse(strings.NewReader(xmlData))
	if err != nil {
		t.Fatal(err)
	}
	xsltDoc, err := goxml.Parse(strings.NewReader(xslt))
	if err != nil {
		t.Fatal(err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}

	// Primary document should contain <main>.
	primary := SerializeResult(resultDoc.Document)
	t.Logf("Primary: %s", primary)
	if !strings.Contains(primary, "<main>primary</main>") {
		t.Errorf("expected <main>primary</main> in primary doc, got %s", primary)
	}

	// Secondary document should exist and contain <secondary>.
	secDoc, ok := resultDoc.SecondaryDocuments["secondary.xml"]
	if !ok {
		t.Fatal("expected secondary document 'secondary.xml'")
	}
	secondary := SerializeResult(secDoc)
	t.Logf("Secondary: %s", secondary)
	if !strings.Contains(secondary, "<secondary>Hello</secondary>") {
		t.Errorf("expected <secondary>Hello</secondary> in secondary doc, got %s", secondary)
	}
}

func TestResultDocumentMultiple(t *testing.T) {
	xmlData := `<catalog><book id="1"><title>Go</title></book><book id="2"><title>XML</title></book></catalog>`
	xslt := `<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <index>
      <xsl:for-each select="catalog/book">
        <ref href="{@id}.xml"/>
        <xsl:result-document href="{@id}.xml">
          <book-detail><xsl:value-of select="title"/></book-detail>
        </xsl:result-document>
      </xsl:for-each>
    </index>
  </xsl:template>
</xsl:stylesheet>`

	sourceDoc, err := goxml.Parse(strings.NewReader(xmlData))
	if err != nil {
		t.Fatal(err)
	}
	xsltDoc, err := goxml.Parse(strings.NewReader(xslt))
	if err != nil {
		t.Fatal(err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal(err)
	}
	resultDoc, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal(err)
	}

	// Primary document should contain the index.
	primary := SerializeResult(resultDoc.Document)
	t.Logf("Primary: %s", primary)
	if !strings.Contains(primary, "<index>") {
		t.Errorf("expected <index> in primary doc, got %s", primary)
	}

	// Should have two secondary documents.
	if len(resultDoc.SecondaryDocuments) != 2 {
		t.Fatalf("expected 2 secondary documents, got %d", len(resultDoc.SecondaryDocuments))
	}

	sec1, ok := resultDoc.SecondaryDocuments["1.xml"]
	if !ok {
		t.Fatal("expected secondary document '1.xml'")
	}
	s1 := SerializeResult(sec1)
	t.Logf("1.xml: %s", s1)
	if !strings.Contains(s1, "<book-detail>Go</book-detail>") {
		t.Errorf("expected <book-detail>Go</book-detail> in 1.xml, got %s", s1)
	}

	sec2, ok := resultDoc.SecondaryDocuments["2.xml"]
	if !ok {
		t.Fatal("expected secondary document '2.xml'")
	}
	s2 := SerializeResult(sec2)
	t.Logf("2.xml: %s", s2)
	if !strings.Contains(s2, "<book-detail>XML</book-detail>") {
		t.Errorf("expected <book-detail>XML</book-detail> in 2.xml, got %s", s2)
	}
}

// ========== xsl:analyze-string ==========

func TestAnalyzeStringBasic(t *testing.T) {
	result := runXSLT(t,
		`<root>one 42 two 99 three</root>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:analyze-string select="root" regex="\d+">
        <xsl:matching-substring>
          <m><xsl:value-of select="."/></m>
        </xsl:matching-substring>
        <xsl:non-matching-substring>
          <n><xsl:value-of select="."/></n>
        </xsl:non-matching-substring>
      </xsl:analyze-string>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<m>42</m>") {
		t.Errorf("expected <m>42</m>, got %s", result)
	}
	if !strings.Contains(result, "<m>99</m>") {
		t.Errorf("expected <m>99</m>, got %s", result)
	}
	if !strings.Contains(result, "<n>one </n>") {
		t.Errorf("expected <n>one </n>, got %s", result)
	}
	if !strings.Contains(result, "<n> two </n>") {
		t.Errorf("expected <n> two </n>, got %s", result)
	}
	if !strings.Contains(result, "<n> three</n>") {
		t.Errorf("expected <n> three</n>, got %s", result)
	}
}

func TestAnalyzeStringRegexGroup(t *testing.T) {
	result := runXSLT(t,
		`<root>2024-01-15 and 2025-12-31</root>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:analyze-string select="root" regex="(\d{{4}})-(\d{{2}})-(\d{{2}})">
        <xsl:matching-substring>
          <date y="{regex-group(1)}" m="{regex-group(2)}" d="{regex-group(3)}"/>
        </xsl:matching-substring>
      </xsl:analyze-string>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, `y="2024"`) {
		t.Errorf("expected y='2024', got %s", result)
	}
	if !strings.Contains(result, `m="01"`) {
		t.Errorf("expected m='01', got %s", result)
	}
	if !strings.Contains(result, `d="15"`) {
		t.Errorf("expected d='15', got %s", result)
	}
	if !strings.Contains(result, `y="2025"`) {
		t.Errorf("expected y='2025', got %s", result)
	}
	if !strings.Contains(result, `m="12"`) {
		t.Errorf("expected m='12', got %s", result)
	}
	if !strings.Contains(result, `d="31"`) {
		t.Errorf("expected d='31', got %s", result)
	}
}

func TestAnalyzeStringFullMatch(t *testing.T) {
	result := runXSLT(t,
		`<root>12345</root>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:analyze-string select="root" regex="\d+">
        <xsl:matching-substring>
          <m><xsl:value-of select="."/></m>
        </xsl:matching-substring>
        <xsl:non-matching-substring>
          <n><xsl:value-of select="."/></n>
        </xsl:non-matching-substring>
      </xsl:analyze-string>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<m>12345</m>") {
		t.Errorf("expected <m>12345</m>, got %s", result)
	}
	if strings.Contains(result, "<n>") {
		t.Errorf("expected no <n> elements for full match, got %s", result)
	}
}

func TestAnalyzeStringNoMatch(t *testing.T) {
	result := runXSLT(t,
		`<root>hello world</root>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <out>
      <xsl:analyze-string select="root" regex="\d+">
        <xsl:matching-substring>
          <m><xsl:value-of select="."/></m>
        </xsl:matching-substring>
        <xsl:non-matching-substring>
          <n><xsl:value-of select="."/></n>
        </xsl:non-matching-substring>
      </xsl:analyze-string>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	if strings.Contains(result, "<m>") {
		t.Errorf("expected no <m> elements when nothing matches, got %s", result)
	}
	if !strings.Contains(result, "<n>hello world</n>") {
		t.Errorf("expected <n>hello world</n>, got %s", result)
	}
}

// ========== xsl:key and key() ==========

func TestKeyBasic(t *testing.T) {
	result := runXSLT(t,
		`<data><item id="x">Alpha</item><item id="y">Beta</item></data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="idx" match="item" use="@id"/>
  <xsl:template match="/">
    <out><xsl:value-of select="key('idx','x')"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>Alpha</out>") {
		t.Errorf("expected <out>Alpha</out>, got %s", result)
	}
}

func TestKeyMultipleMatches(t *testing.T) {
	result := runXSLT(t,
		`<data><item cat="a">One</item><item cat="b">Two</item><item cat="a">Three</item></data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="byCat" match="item" use="@cat"/>
  <xsl:template match="/">
    <out><xsl:for-each select="key('byCat','a')"><v><xsl:value-of select="."/></v></xsl:for-each></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<v>One</v>") {
		t.Errorf("expected <v>One</v>, got %s", result)
	}
	if !strings.Contains(result, "<v>Three</v>") {
		t.Errorf("expected <v>Three</v>, got %s", result)
	}
}

func TestKeyNoMatch(t *testing.T) {
	result := runXSLT(t,
		`<data><item id="x">Alpha</item></data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="idx" match="item" use="@id"/>
  <xsl:template match="/">
    <out><xsl:value-of select="count(key('idx','missing'))"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>0</out>") {
		t.Errorf("expected <out>0</out>, got %s", result)
	}
}

func TestKeyInForEach(t *testing.T) {
	result := runXSLT(t,
		`<data>
  <item id="1" ref="a"/>
  <item id="2" ref="b"/>
  <lookup code="a" val="Apple"/>
  <lookup code="b" val="Banana"/>
</data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="lu" match="lookup" use="@code"/>
  <xsl:template match="/">
    <out><xsl:for-each select="data/item"><r><xsl:value-of select="key('lu',@ref)/@val"/></r></xsl:for-each></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<r>Apple</r>") {
		t.Errorf("expected <r>Apple</r>, got %s", result)
	}
	if !strings.Contains(result, "<r>Banana</r>") {
		t.Errorf("expected <r>Banana</r>, got %s", result)
	}
}

func TestKeyMultipleKeys(t *testing.T) {
	result := runXSLT(t,
		`<data><item id="x" cat="a">Alpha</item><item id="y" cat="b">Beta</item></data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="byId" match="item" use="@id"/>
  <xsl:key name="byCat" match="item" use="@cat"/>
  <xsl:template match="/">
    <out>
      <id><xsl:value-of select="key('byId','y')"/></id>
      <cat><xsl:value-of select="key('byCat','a')"/></cat>
    </out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<id>Beta</id>") {
		t.Errorf("expected <id>Beta</id>, got %s", result)
	}
	if !strings.Contains(result, "<cat>Alpha</cat>") {
		t.Errorf("expected <cat>Alpha</cat>, got %s", result)
	}
}

func TestKeyWithPath(t *testing.T) {
	result := runXSLT(t,
		`<data><item><name>Foo</name><val>100</val></item><item><name>Bar</name><val>200</val></item></data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="byName" match="item" use="name"/>
  <xsl:template match="/">
    <out><xsl:value-of select="key('byName','Bar')/val"/></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>200</out>") {
		t.Errorf("expected <out>200</out>, got %s", result)
	}
}

func TestKeyUsedInPredicate(t *testing.T) {
	result := runXSLT(t,
		`<data>
  <item id="1"><title>First</title></item>
  <item id="2"><title>Second</title></item>
  <ref target="2"/>
</data>`,
		`<xsl:stylesheet version="2.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:key name="items" match="item" use="@id"/>
  <xsl:template match="/">
    <out><xsl:for-each select="data/ref"><xsl:value-of select="key('items',@target)/title"/></xsl:for-each></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "<out>Second</out>") {
		t.Errorf("expected <out>Second</out>, got %s", result)
	}
}

// transformHelper compiles an XSLT stylesheet and transforms the source XML,
// returning the serialized result.
func transformHelper(t *testing.T, sourceXML, xsltStr string) string {
	t.Helper()
	sourceDoc, err := goxml.Parse(strings.NewReader(sourceXML))
	if err != nil {
		t.Fatal("parsing source:", err)
	}
	xsltDoc, err := goxml.Parse(strings.NewReader(xsltStr))
	if err != nil {
		t.Fatal("parsing XSLT:", err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		t.Fatal("compiling XSLT:", err)
	}
	result, err := Transform(ss, sourceDoc)
	if err != nil {
		t.Fatal("transforming:", err)
	}
	return SerializeResult(result.Document)
}

// ========== expand-text / Text Value Templates ==========

func TestExpandTextBasic(t *testing.T) {
	result := transformHelper(t,
		`<root><item name="Alice"/><item name="Bob"/></root>`,
		`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  expand-text="yes">
  <xsl:template match="root">
    <out><xsl:apply-templates/></out>
  </xsl:template>
  <xsl:template match="item">Hello {./@name}! </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "Hello Alice!") {
		t.Errorf("expected 'Hello Alice!', got %s", result)
	}
	if !strings.Contains(result, "Hello Bob!") {
		t.Errorf("expected 'Hello Bob!', got %s", result)
	}
}

func TestExpandTextLiteralElement(t *testing.T) {
	result := transformHelper(t,
		`<data><val>42</val></data>`,
		`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  expand-text="yes">
  <xsl:template match="data">
    <out>The value is {val}.</out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "The value is 42.") {
		t.Errorf("expected 'The value is 42.', got %s", result)
	}
}

func TestExpandTextXslText(t *testing.T) {
	result := transformHelper(t,
		`<data><x>7</x></data>`,
		`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  expand-text="yes">
  <xsl:template match="data">
    <out><xsl:text>x={x}</xsl:text></out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "x=7") {
		t.Errorf("expected 'x=7', got %s", result)
	}
}

func TestExpandTextEscapedBraces(t *testing.T) {
	result := transformHelper(t,
		`<data/>`,
		`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  expand-text="yes">
  <xsl:template match="data">
    <out>A {{literal}} brace</out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "A {literal} brace") {
		t.Errorf("expected 'A {literal} brace', got %s", result)
	}
}

func TestExpandTextDisabledByDefault(t *testing.T) {
	result := transformHelper(t,
		`<data><x>7</x></data>`,
		`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="data">
    <out>no expansion {x} here</out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "no expansion {x} here") {
		t.Errorf("expected literal braces preserved, got %s", result)
	}
}

func TestExpandTextLocalOverride(t *testing.T) {
	result := transformHelper(t,
		`<data><x>7</x></data>`,
		`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
		  xmlns:xsle="http://www.w3.org/1999/XSL/Transform"
		  expand-text="yes">
  <xsl:template match="data">
    <out xsle:expand-text="no">not expanded {x} here</out>
  </xsl:template>
</xsl:stylesheet>`)
	if !strings.Contains(result, "not expanded {x} here") {
		t.Errorf("expected expand-text=no to disable TVT, got %s", result)
	}
}
