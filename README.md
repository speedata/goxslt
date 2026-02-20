# goxslt

A proof-of-concept XSLT processor written in Go.

> **Note:** This is an experimental implementation and not intended for production use. It supports only a subset of the XSLT specification.

## Supported Features

### Instructions

- `xsl:template` (match and named templates with priority)
- `xsl:apply-templates` with `xsl:sort` and `xsl:with-param`
- `xsl:call-template` with `xsl:with-param`
- `xsl:value-of`
- `xsl:for-each` with `xsl:sort`
- `xsl:for-each-group` (group-by, `current-group()`, `current-grouping-key()`)
- `xsl:if`, `xsl:choose`/`xsl:when`/`xsl:otherwise`
- `xsl:variable`, `xsl:param` (with `as` type declarations)
- `xsl:copy`, `xsl:copy-of`
- `xsl:element`, `xsl:attribute`
- `xsl:text`, `xsl:sequence`
- `xsl:comment`, `xsl:processing-instruction`
- `xsl:number` (single/multiple/any level, format patterns)
- `xsl:message` (with terminate, custom message handler)
- `xsl:result-document` (multiple output documents)
- `xsl:map`, `xsl:map-entry` (XPath maps and arrays supported)
- `xsl:function` (stylesheet functions callable from XPath)

### Stylesheet Structure

- `xsl:output` (method, indent, version)
- `xsl:import` and `xsl:include` (with precedence and cycle detection)
- Stylesheet parameters (`xsl:param` at top level, passable via CLI or API)
- Attribute Value Templates (`class="item-{@id}"`)
- Literal result elements
- Match patterns with predicates (`book[@lang='en']`), path patterns, union patterns

## Installation

```
go install github.com/speedata/goxslt/cmd/goxslt@latest
```

## Usage

```
goxslt -s source.xml -t stylesheet.xsl [-o output.xml] [param=value ...]
```

## Library Usage

```go
package main

import (
    "fmt"
    "os"

    "github.com/speedata/goxml"
    "github.com/speedata/goxslt"
)

func main() {
    sourceDoc, _ := goxml.Parse(os.Stdin)
    xsltFile, _ := os.Open("style.xsl")
    xsltDoc, _ := goxml.Parse(xsltFile)

    ss, _ := goxslt.Compile(xsltDoc)
    result, _ := goxslt.Transform(ss, sourceDoc)

    fmt.Print(goxslt.SerializeIndent(result.Document, "  "))

    // Secondary documents produced by xsl:result-document
    for href, doc := range result.SecondaryDocuments {
        os.WriteFile(href, []byte(goxslt.SerializeResult(doc)), 0644)
    }
}
```

## License

MIT — see [LICENSE](LICENSE).
