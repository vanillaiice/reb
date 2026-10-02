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
rebcompiler/      .reb source -> field schema (JSON) + Go html/template
rebrender/        executes a compiled template against data (math, formatNumber, formatMoney, formatDate ...)
cmd/rebc/         the JSON CLI: rebc compile | rebc render
cmd/wasm/         WebAssembly entry point for browsers (Studio, editor previews)
assets/           tailwindcss.js, the Tailwind browser build that <reb-tailwind> loads
docs/             the .reb specification
```

## rebc

Input always arrives as one JSON object on stdin, never as arguments.

```
rebc compile   stdin  {"reb": "<.reb source>"}
               stdout {"schema": [...], "html": "<compiled Go template>"}

rebc render    stdin  {"html": "...", "system": {...}, "answers": {...}, "assets": {"blob:<id>": "<file name>"}}
               stdout {"html": "<rendered HTML>"}

errors         exit status 1, stdout {"error": "..."}
```

`render` replaces asset references in the answers (also inside table rows) with the given
file names, flattens the answers into the template root without overriding system keys,
and sanitizes plain strings with bluemonday's UGC policy before executing the template.

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
implemented), so CI tries it on every push without blocking.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE). Copyright (C) 2026 hblabs.
