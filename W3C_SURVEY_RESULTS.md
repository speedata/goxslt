# W3C XSLT 3.0 Test Survey — 2026-04-10

## Gesamtergebnis

| Metrik | Wert |
|--------|------|
| Total Tests | 14714 |
| Pass | 4985 (33.9%) |
| Fail | 8459 |
| Panic/Crash | **7** |
| Skip | 1263 |

**Veränderung zum letzten Survey (2026-03-27):**

| Metrik | Vorher | Jetzt | Δ |
|--------|--------|-------|---|
| Pass | 3214 (21.8%) | 4985 (33.9%) | **+1771 (+12.1 pp)** |
| Panic/Crash | 2849 | **7** | **−2842** |
| Fail | 7388 | 8459 | +1071 |

> Die Crashes sind fast komplett verschwunden. Die meisten waren keine echten Bugs in goxslt, sondern entweder (a) Tests die korrekt fehlschlagen aber vorher in einen Subprocess-Timeout liefen, oder (b) der `misc/unicode-90`-Set, der pro Test einen Regex 63454-mal frisch kompilierte.

## Top Fehler-Kategorien (nach Häufigkeit)

| # | Fehler | Anzahl |
|---|--------|--------|
| 1 | **assertion failed** (XPath-Assertion schlägt fehl) | 2852 |
| 2 | **xml mismatch** (falsche Ausgabe) | 1806 |
| 3 | **expected error but succeeded** (fehlende Error-Erkennung) | 1246 |
| 4 | **serialization mismatch** (Serialisierung weicht ab) | 219 |
| 5 | **unsupported assertion type** (Test-Harness-Lücke) | 173 |
| 6 | **xsl:evaluate** (dynamische XPath-Auswertung nicht implementiert) | 165 |
| 7 | **any-of: no match** | 109 |
| 8 | **string value mismatch** | 102 |
| 9 | **`element(*, type)`-Syntax** (Sequence-Type Parser) | ~70 |
| 10 | **`xsl:merge`** (nicht implementiert) | ~60 |

## Top 25 Test-Sets nach bestehendem Pass-Count

| Test-Set | Pass | Fail | Total | Pass% |
|----------|------|------|-------|-------|
| misc/regex-syntax-xslt20 | 834 | 153 | 987 | **84%** |
| misc/error | 200 | 379 | 579 | 35% |
| fn/position | 150 | 61 | 211 | 71% |
| expr/math | 138 | 20 | 159 | **87%** |
| type/string | 127 | 9 | 136 | **93%** |
| misc/regex-classes | 120 | 0 | 120 | **100%** |
| attr/select | 116 | 42 | 158 | 73% |
| type/boolean | 110 | 2 | 112 | **98%** |
| expr/axes | 106 | 96 | 202 | 52% |
| expr/expression | 92 | 13 | 105 | 88% |
| fn/core-function | 89 | 1 | 90 | **99%** |
| attr/match | 84 | 210 | 294 | 29% |
| attr/as | 80 | 124 | 204 | 39% |
| decl/variable | 77 | 31 | 108 | 71% |
| fn/key | 76 | 23 | 99 | 77% |
| insn/number | 71 | 200 | 271 | 26% |
| insn/sort | 67 | 13 | 80 | **84%** |
| attr/streamable | 61 | 78 | 139 | 44% |
| attr/mode | 58 | 93 | 169 | 34% |
| insn/result-document | 56 | 98 | 154 | 36% |
| insn/sequence | 54 | 37 | 92 | 59% |
| type/date | 53 | 85 | 138 | 38% |
| type/namespace | 51 | 173 | 224 | 23% |
| insn/copy | 50 | 97 | 148 | 34% |
| decl/function | 45 | 64 | 110 | 41% |

## Was hat sich seit dem letzten Survey verbessert?

- **`xsl:iterate`** (mit `xsl:break`, `xsl:next-iteration`, `xsl:on-completion`) implementiert
- **Regex-Cache in `fn:matches`/`fn:replace`/`fn:tokenize`** — Pattern wird pro `(pattern, flags)` einmal kompiliert statt pro Aufruf. ~5,5x schneller bei Loops mit Regex
- **Subprocess-Timeout im Survey dynamisch** nach Set-Größe — vorher wurden große Sets fälschlicherweise als "Crash" markiert wenn der 30s-Timeout zuschlug
- **`misc/regex-syntax-xslt20`**: 369 → 834 (+465)
- **`misc/regex-classes`**: 36 → 120 (**komplett bestanden**)
- **`misc/error`**: 0 → 200
- **`expr/math`**: 0 → 138 — `math:*`-Funktionen ergänzt
- **`decl/function`**: 0 → 45
- **`fn/key`**, **`type/boolean`**, **`type/string`** — XPath-Bugfixes
- Diverse XSLT-3.0-Conformance-Fixes: `xsl:number` ohne Kontextknoten (XTTE0990), `xsl:message` mit dynamischen Fehlern, `position()`/`last()` in `xsl:analyze-string`, `div`-by-zero (FOAR0001)
- **`fn:unparsed-text`**: Encoding-Argument unterstützt (`iso-8859-1` etc.)

## Empfohlene Prioritäten für Implementierung

### Prio 1 — Höchster Impact (viele Tests, schnell lösbar)

1. **Sequence-Type Parser-Erweiterung** (`element(*, type)` Syntax)
   - ~70 Tests blockiert durch "expect close paren, got comma"
   - Betrifft `attr/as`, `type/*`, mehrere `decl/*`-Sets

2. **`xsl:evaluate`** (dynamische XPath-Auswertung)
   - 165 Tests blockiert
   - `fn/system-property-gen` (166 Tests, alle blockiert)

3. **`xsl:merge`** (~60 Tests)
   - `strm/si-merge` und mehrere `insn/*`-Sets

4. **`copy-of()` / `outermost()` / `innermost()` / `snapshot()` als Funktionen**
   - ~50 Tests in Streaming-Sets

5. **`accumulator-before()` / `accumulator-after()`**
   - ~30 Tests in `decl/accumulator`

### Prio 2 — Mittlerer Impact

6. **`misc/unicode-90`** (1460 Tests, 0 Pass)
   - Tests prüfen Unicode-9.0-Konformität von Regex-Klassen
   - Go's `regexp` interpretiert XPath-Regex-Klassen wie `\d`, `\w`, `\p{...}` nicht XPath-konform
   - Eigentlich kein einzelner Bug — bräuchte eine eigene XPath-Regex-Engine oder einen Wrapper, der die Klassen umschreibt
   - Aufwand-Nutzen-Verhältnis vermutlich schlecht

7. **`insn/number`** (200 Fail von 271)
   - Word-Formatierung (`Ww`, `w`, `W`), Sprach-Lokalisierung

8. **`attr/match`** (210 Fail von 294, 29%)
9. **`type/namespace`** (173 Fail von 224, 23%)
10. **`insn/copy`** (97 Fail von 148, 34%)

### Prio 3 — Qualitätsverbesserung bestehender Features

| Set | Pass% | Fehlende Tests |
|-----|-------|----------------|
| `fn/core-function` | 99% | 1 |
| `type/boolean` | 98% | 2 |
| `type/string` | 93% | 9 |
| `expr/math` | 87% | 20 |
| `misc/regex-syntax-xslt20` | 84% | 153 |
| `insn/sort` | 84% | 13 |
| `expr/expression` | 88% | 13 |

### Sets die komplett übersprungen werden (kein Stylesheet / Packages)

- `sandp/*` (alle ~900 Tests): Streaming-and-Patterns — alles Skip
- `decl/accept` (50): Package-Level — alles Skip
- `decl/expose` (42): Package-Level — alles Skip
- `decl/override` (102): Package-Level
- `decl/package` (57): Package-Level

## Schnellster Weg zu mehr bestandenen Tests

| Aktion | Geschätzte zusätzliche Passes |
|--------|-------------------------------|
| `element(*, type)` Sequence-Type Syntax | +70-100 |
| `xsl:evaluate` implementieren | +150-200 |
| `xsl:merge` implementieren | +50-80 |
| `copy-of()` / `snapshot()` / `outermost()` als Fn | +50-100 |
| `accumulator-before/after` | +30-50 |
| `insn/number` Word-Formatierung | +50-100 |
