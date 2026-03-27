# goxslt

An XSLT 3.0 processor written in Go, built on [goxml](https://github.com/speedata/goxml) and [goxpath](https://github.com/speedata/goxpath).

## Status

goxslt covers most commonly used XSLT features: template matching with priorities and modes, `xsl:for-each` / `xsl:for-each-group` (group-by, group-adjacent, group-starting-with, group-ending-with), `xsl:choose`, variables and parameters with type declarations, `xsl:copy` / `xsl:copy-of`, `xsl:number`, `xsl:analyze-string`, `xsl:function` (with EQName support), `xsl:key`, `xsl:import` / `xsl:include`, `xsl:result-document`, `xsl:source-document`, `xsl:try`/`xsl:catch`, `xsl:fork`, `xsl:namespace`, `xsl:where-populated`, `xsl:mode` with `on-no-match="shallow-copy"`, Text Value Templates (`expand-text`), and JSON processing via `json-to-xml()` / `xml-to-json()`.

Over 3200 tests from the [W3C XSLT 3.0 conformance test suite](https://github.com/w3c/xslt30-test) pass (21.8% of the ~14700 tests across ~260 test sets). Not yet implemented are streaming, schema-aware processing, and full package support.

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
sourceDoc, _ := goxml.Parse(os.Stdin)
ss, _ := goxslt.CompileFile("style.xsl")
result, _ := goxslt.Transform(ss, sourceDoc)
fmt.Print(goxslt.SerializeWithOutput(result.Document, result.Output))
```

See [pkg.go.dev](https://pkg.go.dev/github.com/speedata/goxslt) for the full Go API.

## Documentation

The full reference for supported XSLT instructions, functions, modes, and features is at:

**https://doc.speedata.de/goxml/**

## W3C XSLT 3.0 Test Suite

```
git clone https://github.com/w3c/xslt30-test.git testdata/w3c
go test -v -run TestW3C ./...
```

## License

MIT — see [LICENSE](LICENSE).
