# Changelog

Every change a consumer of reb can see: the `rebc` and WebAssembly JSON, the schemas, error and
warning codes, the `.reb` language, and what templates compile and render to. The rules are in
[specification section 8.4](docs/specification.md#84-compatibility); `testdata/golden` shows each
change to the output.

## v0.8.0

### Fixed

- **A top-level field named like a table column is no longer dropped.** `Normalize` used to remove
  every non-table field whose name any table's options used as a column, so `<reb-number name="qty">`
  next to `<reb-table options="qty:number">` lost the field: the form never asked for it and
  `prepare` discarded its answer. Only a tag inside a `<reb-table>` named after one of its columns
  now declares the column; the raw schema marks it with `table` (the table's name), and both
  declarations are kept, so the raw schema can list one name twice (once with `table`). A raw
  schema from an earlier release has no marks, so `normalize` keeps all its entries at the top
  level; recompile such templates.
- `prepare` drops the errors of hidden fields with their answers: a hidden number holding `"bad"`,
  a hidden table with a bad cell or too many rows no longer blocks the form.
- `{{formatNumber 123 2}}` prints `123.00`: with two integer arguments the value comes first, as
  documented (it used to print `2.00000000000000000000`). An integer pipeline is written
  `{{formatNumber (len .rows) 0}}`. `formatMoney` no longer depends on that rule: `{{formatMoney 123
  "QAR" 2}}` prints `QAR 123.00`.
- A formula's precision is held to 0..10 wherever it comes from (a normalized schema passed in used
  to reach the formatter unchecked).

### Added

- `invalid_schema` (`detail`): a schema passed in as `fields` (or raw `schema`) is checked before
  `prepare`, `render` or `pdf-answers` use it: `schemaVersion` 2, known field and column kinds,
  precision 0..10, and size budgets (specification section 8.1). In rebc and the WebAssembly build
  alike.
- `output_too_large` (`count`): a render that would print more than 64 MiB stops with this error
  instead of growing without bound (`{{range 100000000}}`).
- The warning `column_shadows_field` (`field`, `table`): a table column named like a top-level field.
- `testdata/visibility_cases.json`: whole-form show-if visibility (chains, negation, cycles), for
  every evaluator to run; specification section 3.3 now spells out the passes.

### Changed

- `rebc render` prints its HTML without escaping `<`, `>` and `&` in the JSON (the same JSON value).
- rebc sets a soft 1 GiB memory limit unless `GOMEMLIMIT` is set.
- Built with Go 1.27.2 (`toolchain` in go.mod), which fixes two `html/template` escaping advisories
  (GO-2026-6599, GO-2026-6600); `golang.org/x/net` v0.61.0. CI runs govulncheck.
- `duplicate_field` no longer counts a table's column declaration against a top-level field.

## v0.7.0

### Added

- Fillable PDF fields (experimental, specification section 4.5): `fillable` on `<reb-checkbox>`.
  In fillable mode it prints a framed square (`reb-field:NAME;checkbox`, class
  `reb-fillable-checkbox`) that `rebc fillable` turns into a PDF check box, ticked when the answer
  is; `rebc pdf-answers` reads it back as `true` or `false`.

## v0.6.0

### Added

- **Fillable PDF fields** (specification section 4.5), **experimental**: outside the compatibility
  promise, so a minor release may change or remove them. The `fillable` attribute on `<reb-text>`,
  `<reb-number>`, `<reb-date>` and `<reb-textarea>` lets the answer be typed into the PDF.
  `render` takes `"fillable": true` to print those fields as empty boxes (others keep their
  answers); `rebc fillable` turns the boxes of the printed PDF into PDF text fields, and
  `rebc pdf-answers` reads the template's fillable fields back from a filled PDF. In browsers they
  are `__rebFillable` and `__rebPdfAnswers`, in a second build, `rebpdf.wasm` (`cmd/wasmpdf`).
- `{{.Fillable}}`, a system value: true when the document is rendered fillable.
- `fillable` in both schemas, and the warning `fillable_ignored` (`field`, `tag`) for the attribute
  on a tag that cannot be filled.
- `testdata/golden/fillable`: the golden harness renders a case in both modes when its input says
  `"fillable": true` (`NAME.fillable.html`).

## v0.5.0

The contract the engine will freeze as v1.0.0.

### Changed

- **Answers reach the template as plain text.** They used to arrive as HTML sanitized with
  bluemonday, so a comparison like `{{if eq .dept "R&D"}}` (or `"O'Brien"`) never held: the answer
  had become `R&amp;D`. Now the template escapes answers where it prints them, and HTML typed into a
  text field prints as text instead of rendering. Text areas still render as paragraphs.
  `{{safeHTML .name}}` prints an answer as sanitized HTML.
- The Go packages moved to `internal/` (`rebcompiler`, `rebrender`, `rebdoc`): the contract is the
  JSON interface, not a Go API. No consumer imported them.
- `invalid_pattern` messages quote the pattern as written, without the anchors the engine adds.

- **Table formulas handle negative values.** The evaluator had no unary minus, so `qty*rate` with a
  rate of -4 and an empty quantity gave -4.00, `2*-4` gave -4, `5--3` gave -8 and `10/-2` gave -2.
  Formulas now follow the grammar in specification section 4.3; an expression that does not parse
  (unbalanced parentheses, a stray character such as `^`, two values in a row) gives 0 rather than
  whatever part of it could be read. Values no longer turn into text inside the expression, so a
  tiny value such as 0.0000001 stays a number. `testdata/formula_cases.json` has the new cases; the
  web form's and the mobile app's evaluators must move with this release.

### Added

- Errors on a table cell (`not_a_number`, `not_an_option`, `too_long`, ...) carry `row` and `column`
  params, so a form can point at the cell.

- `testdata/golden`: templates with their expected schema, warnings, prepared answers, compiled
  template and rendered document, run by `go test ./cmd/rebc`.
- `__rebPrepare` and `__rebRender` accept the raw `schema` as `rebc` does, besides `fields`.
- Specification section 8.4: what the version number promises.

### Fixed

- `{{.Answers.name}}` counts as using `name` (no `unused_field` warning) and is checked for
  `unknown_binding`.
- Previews print `{{formatNumber .Number 2}}` as `1.00` (was `2.0`).
- `<body` inside an attribute no longer makes the compiled template a whole HTML document.
- `golang.org/x/net` v0.59.0; `go.mod` asks for Go 1.26.0 rather than 1.26.2.

## v0.4.1

- Quotes in a template action written over several lines are kept.

## v0.4.0

- Lint warnings: `unknown_binding`, `unused_field`, `duplicate_field`.

## v0.3.1

- Previews show sample images instead of broken ones.

## v0.3.0

- `rebdoc`, the rules consumers used to copy: normalized schema, `prepare` (validation, formulas,
  show-if), the render context, sample answers; `rebc prepare` and `rebc normalize`.

## v0.2.0

- `assets/paged.polyfill.js` for browser pagination.

## v0.1.0

- reb as a standalone repository: the compiler, the renderer, `rebc` and the WebAssembly build.
