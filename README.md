# goxslt

An XSLT 3.0 processor written in Go, built on [goxml](https://github.com/speedata/goxml) and [goxpath](https://github.com/speedata/goxpath).

## Status

**Beta** — goxslt covers most commonly used XSLT features: template matching with priorities and modes, `xsl:apply-templates`, `xsl:call-template`, `xsl:next-match`, `xsl:apply-imports`, `xsl:for-each` / `xsl:for-each-group` (group-by, group-adjacent, group-starting-with, group-ending-with), `xsl:choose`, variables and parameters with type declarations, `xsl:copy` / `xsl:copy-of`, `xsl:number`, `xsl:perform-sort`, `xsl:analyze-string`, `xsl:function`, `xsl:key`, `xsl:import` / `xsl:include`, `xsl:result-document`, `xsl:source-document`, `xsl:try`/`xsl:catch`, `xsl:fork`, `xsl:namespace`, `xsl:fallback`, `xsl:where-populated`, `xsl:mode` with `on-no-match="shallow-copy"`, Text Value Templates (`expand-text`), `fn:serialize()`, and JSON processing via `json-to-xml()` / `xml-to-json()`.

Over 5000 tests from the [W3C XSLT 3.0 conformance test suite](https://github.com/w3c/xslt30-test) pass (~34% of the ~14700 tests across ~260 test sets).

### Not yet implemented

- **Streaming** (`xsl:stream`, streamable modes)
- **Schema-aware processing** (schema validation, typed values)
- **Full package support** (`xsl:use-package`, `xsl:accept`, `xsl:expose`)
- **`xsl:iterate`**, **`xsl:merge`**, **`xsl:evaluate`**
- **`namespace::` axis**
- **Unicode Collation Algorithm** (sort/compare uses codepoint order)
- **Date/duration arithmetic** (partial)

See the [full documentation](https://doc.speedata.de/goxml/) for details on supported features.

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

## Testing

```bash
go test ./...                  # unit tests + whitelisted W3C tests (~1s)
```

### W3C XSLT 3.0 conformance suite

```bash
git clone https://github.com/w3c/xslt30-test.git testdata/w3c
go test -v -run TestW3C ./...  # run whitelisted tests (~1s)

# full survey across all ~14700 tests (~90s)
W3C_SURVEY=1 go test -v -run TestW3CSurvey -timeout 3600s ./...
```

## License

BSD-3-Clause — see [LICENSE](LICENSE).
