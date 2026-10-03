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
rebcompiler/      .reb source -> raw field schema (JSON) + Go html/template
rebrender/        executes a compiled template against data (math, formatNumber, formatMoney, formatDate ...)
rebdoc/           what consumers need around it: Compile (normalized schema, coded errors, warnings),
                  Prepare (answer validation, formulas, required/min/max/pattern, show-if),
                  BuildContext (system values, file names, text-area paragraphs), sample answers
cmd/rebc/         the JSON CLI
cmd/wasm/         WebAssembly entry point for browsers (Studio, editor previews)
assets/           tailwindcss.js (the Tailwind browser build <reb-tailwind> loads) and paged.polyfill.js
testdata/         golden cases every consumer runs: formula_cases.json, show_if_cases.json
docs/             the .reb specification
```

## rebc

Input always arrives as one JSON object on stdin, never as arguments.

```
rebc compile    {"reb"}                                  -> {"schema", "fields", "html", "engineVersion", "warnings"}
rebc prepare    {"fields" | "schema", "answers"}         -> {"answers", "errors"}
rebc render     {"html", "system", "answers", "assets", "fields"?}  -> {"html"}
rebc normalize  {"schema"}                               -> {"schemaVersion", "fields"}
rebc version                                             -> {"version"}

errors          exit status 1, stdout {"error", "code"?, "params"?}
```

`schema` is the raw schema the tags declared, `fields` the normalized one (specification section 8).
`prepare` cleans a document's answers, computes formula and row-number cells, drops fields hidden by
`show-if` and returns coded errors. `render` replaces file references in the answers (also inside
table rows) with the given file names, turns text areas into paragraphs when `fields` is given,
flattens the answers into the template root without overriding system values, and sanitizes plain
strings with bluemonday's UGC policy before executing the template.

## Build and test

Requires Go (see `go.mod`).

```bash
go vet ./... && go test ./...

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
