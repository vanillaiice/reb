# Changelog

Every change a consumer of reb can see: the `rebc` and WebAssembly JSON, the schemas, error and
warning codes, the `.reb` language, and what templates compile and render to. The rules are in
[specification section 8.4](docs/specification.md#84-compatibility); `testdata/golden` shows each
change to the output.

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
