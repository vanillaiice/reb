# reb

The `.reb` template engine: one Go implementation of the `.reb` form-template format, used
everywhere a template is compiled or rendered.

| Consumer | How it runs reb |
|---|---|
| Rebar (Rails) | `rebc`, a JSON-in, JSON-out command the server calls for compile and render; then Gotenberg makes the PDF |
| Rebar Studio (desktop and PWA) | `cmd/wasm`, the same packages compiled to WebAssembly |
| Rebar's template editor preview | the same WebAssembly build |

A new `.reb` feature lands here first, with its specification text and tests, and is released
as a version. Consumers then move to that version. The format is described in
[docs/specification.md](docs/specification.md).

## Layout

```
cmd/rebc/              the JSON CLI
cmd/wasm/              WebAssembly entry point for browsers (Studio, editor previews)
internal/rebcompiler/  .reb source -> raw field schema + Go html/template
internal/rebrender/    executes a compiled template against data (math, formatNumber, formatMoney, formatDate ...)
internal/rebdoc/       what consumers need around it: Compile (normalized schema, coded errors, warnings),
                       Prepare (answer validation, formulas, required/min/max/pattern, show-if),
                       BuildContext (system values, file names, text-area paragraphs), sample answers
assets/                tailwindcss.js (the Tailwind browser build <reb-tailwind> loads) and paged.polyfill.js
testdata/              cases every consumer can run: formula_cases.json, show_if_cases.json, and golden/
                       (templates with their expected schema, compiled template and rendered document)
docs/                  the .reb specification
```

The Go packages are internal: consumers run `rebc` or the WebAssembly build, never import them.

## Compatibility

Releases follow semantic versioning over the contract in
[specification section 8.4](docs/specification.md#84-compatibility): the `rebc` and WebAssembly
JSON, both schemas, the error and warning codes, the `.reb` language, and what the templates in
`testdata/golden` compile and render to. Patch releases fix, minor releases add (consumers ignore
output fields and warning codes they do not know), anything else is a major release.
[CHANGELOG.md](CHANGELOG.md) lists every change consumers can see.

To move a consumer to a new release, read its changelog entry and the diff of `testdata/golden`
between the two tags (`git diff v0.4.1 v0.5.0 -- testdata/golden`).

## rebc

Input always arrives as one JSON object on stdin, never as arguments.

```
rebc compile    {"reb"}                                  -> {"schema", "fields", "html", "engineVersion", "warnings"}
rebc prepare    {"fields" | "schema", "answers"}         -> {"answers", "errors"}
rebc render     {"html", "system", "answers", "assets", "fields"? | "schema"?}  -> {"html"}
rebc normalize  {"schema"}                               -> {"schemaVersion", "fields"}
rebc version                                             -> {"version"}

errors          exit status 1, stdout {"error", "code"?, "params"?}
```

`schema` is the raw schema the tags declared, `fields` the normalized one (specification section 8).
`prepare` cleans a document's answers, computes formula and row-number cells, drops fields hidden by
`show-if` and returns coded errors. `render` replaces file references in the answers (also inside
table rows) with the given file names, turns text areas into paragraphs when a schema is given, and
flattens the answers into the template root without overriding system values. Answers stay plain
text, escaped where the template prints them; `safeHTML` prints one as HTML sanitized with
bluemonday's UGC policy.

The WebAssembly build takes the same input for `__rebPrepare` and `__rebRender`.

## Build and test

Requires Go (see `go.mod`).

```bash
go vet ./... && go test ./...

# after a change to what templates compile or render to: rewrite testdata/golden, review the diff
go test ./cmd/rebc -update

# the WebAssembly build's tests run under Node
GOOS=js GOARCH=wasm go test -exec="bash $(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./cmd/wasm

CGO_ENABLED=0 go build -trimpath -o rebc ./cmd/rebc
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o rebcompiler.wasm ./cmd/wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .   # the loader the .wasm needs
```

The WebAssembly build is about 8 MB (2.1 MB gzipped). TinyGo would make it about 4 times
smaller, but TinyGo 0.42 cannot run `html/template` (`reflect.Type.NumOut` is not
implemented).

## Browser pagination

PDFs come from Chromium (Gotenberg on a server, Electron in Studio), which applies `@page` rules
and repeats `<reb-footer>` itself. A plain browser does not, so consumers that show or print
rendered HTML paginate it with `assets/paged.polyfill.js`: hoist the template's `<style>` blocks
into `<head>`, turn the `rebar-pdf-footer-extract` element into a paged.js running element placed
first in the body (`position: running(rebFooter)` with `@page { @bottom-center { content:
element(rebFooter) } }`), let Tailwind finish, then call `PagedPolyfill.preview()` and fill each
page's `.pageNumber` / `.totalPages`. paged.js reads page breaks from stylesheets only, while
`<reb-pagebreak>` compiles to an inline `page-break-after: always` (and templates write inline breaks
too), so mark elements whose inline style breaks with classes and add `break-before` / `break-after:
page` rules for them. Shipping paged.js here keeps every consumer on one version.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE). Copyright (C) 2026 hblabs.

Bundled: `assets/tailwindcss.js` (Tailwind CSS, MIT) and `assets/paged.polyfill.js` (paged.js, MIT,
see `assets/paged.polyfill.LICENSE.md`).
