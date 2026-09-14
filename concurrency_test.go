package goxslt

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/speedata/goxml"
)

// transformWith compiles a stylesheet, runs it against a source document and
// returns the serialized result.
func transformWith(xslt, xml string) (string, error) {
	sourceDoc, err := goxml.Parse(strings.NewReader(xml))
	if err != nil {
		return "", fmt.Errorf("parsing source: %w", err)
	}
	xsltDoc, err := goxml.Parse(strings.NewReader(xslt))
	if err != nil {
		return "", fmt.Errorf("parsing stylesheet: %w", err)
	}
	ss, err := Compile(xsltDoc)
	if err != nil {
		return "", fmt.Errorf("compiling: %w", err)
	}
	result, err := Transform(ss, sourceDoc)
	if err != nil {
		return "", fmt.Errorf("transforming: %w", err)
	}
	return SerializeResult(result.Document), nil
}

// labelStylesheet builds a stylesheet whose my:label() function returns label.
func labelStylesheet(label string) string {
	return `<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:my="urn:test">
  <xsl:function name="my:label">
    <xsl:sequence select="'` + label + `'"/>
  </xsl:function>
  <xsl:template match="/">
    <out><xsl:value-of select="my:label()"/></out>
  </xsl:template>
</xsl:stylesheet>`
}

// TestConcurrentTransform verifies that transformations can run in parallel.
// Registering the stylesheet functions globally instead aborts the process with
// "fatal error: concurrent map read and map write".
func TestConcurrentTransform(t *testing.T) {
	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			label := fmt.Sprintf("label-%d", id)
			xslt := labelStylesheet(label)
			for range 20 {
				got, err := transformWith(xslt, `<root/>`)
				if err != nil {
					t.Error(err)
					return
				}
				// Only the value matters here, not the namespace
				// declaration the literal result element carries.
				if want := ">" + label + "<"; !strings.Contains(got, want) {
					t.Errorf("goroutine %d: got %s, want it to contain %s", id, got, want)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestStylesheetFunctionDoesNotLeak verifies that a function defined by one
// stylesheet is gone once the transformation is over, instead of staying visible
// to an unrelated stylesheet that calls it without defining it.
func TestStylesheetFunctionDoesNotLeak(t *testing.T) {
	if _, err := transformWith(labelStylesheet("first"), `<root/>`); err != nil {
		t.Fatal(err)
	}
	caller := `<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:my="urn:test">
  <xsl:template match="/">
    <out><xsl:value-of select="my:label()"/></out>
  </xsl:template>
</xsl:stylesheet>`
	got, err := transformWith(caller, `<root/>`)
	if err == nil {
		t.Errorf("my:label() resolved to the previous stylesheet's function: %s", got)
	}
}
