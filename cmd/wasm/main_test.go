//go:build js && wasm

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Run under Node (CI does):
//
//	GOOS=js GOARCH=wasm go test -exec="bash $(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./cmd/wasm

func decodeResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCompileRendersThePreviewWithSampleAnswers(t *testing.T) {
	out := decodeResult(t, compile(`<p>{{.Name}}</p><reb-textarea name="notes" label="Notes"></reb-textarea>
		<reb-table name="items" label="Items" options="qty:number,rate:number,total:formula[qty*rate|2]">
		<table><tr reb-row><td>{{.total}}</td></tr></table></reb-table>`))

	preview, _ := out["previewHtml"].(string)
	for _, want := range []string{"Sample document", "<p>Sample paragraph text rendered for preview.</p>", "<td>50.00</td>", "<td>200.00</td>"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview lacks %q:\n%s", want, preview)
		}
	}
	if out["engineVersion"] == "" || out["fields"] == nil || out["schema"] == nil {
		t.Errorf("fields, raw schema and engine version expected: %v", out)
	}
}

func TestCompileReportsCodedErrors(t *testing.T) {
	out := decodeResult(t, compile(`<reb-text name="bad-name" label="x"></reb-text>`))
	if out["code"] != "invalid_field_name" || !strings.Contains(out["error"].(string), "bad-name") {
		t.Errorf("out = %v", out)
	}
}

func TestPrepareAndRender(t *testing.T) {
	compiled := decodeResult(t, compile(`<reb-table name="items" label="Items" options="qty:number,total:formula[qty*2|0]"></reb-table>`))
	fields, _ := json.Marshal(compiled["fields"])

	prepared := decodeResult(t, prepare(`{"fields": `+string(fields)+`, "answers": {"items": [{"qty": "4", "total": "1"}]}}`))
	rows := prepared["answers"].(map[string]any)["items"].([]any)
	if total := rows[0].(map[string]any)["total"]; total != "8" {
		t.Errorf("total = %v", total)
	}

	rendered := decodeResult(t, render(`{"html": "{{.site}} {{.Name}}", "system": {"Name": "R1"}, "answers": {"site": "<b>North</b><script>x</script>"}}`))
	if rendered["html"] != "&lt;b&gt;North&lt;/b&gt;&lt;script&gt;x&lt;/script&gt; R1" {
		t.Errorf("html = %v", rendered["html"])
	}

	// Like rebc, prepare also takes the raw schema.
	prepared = decodeResult(t, prepare(`{"schema": [{"key": "n", "type": "number"}], "answers": {"n": "x"}}`))
	if errors := prepared["errors"].([]any); len(errors) != 1 || errors[0].(map[string]any)["code"] != "not_a_number" {
		t.Errorf("prepare with a raw schema: %v", prepared)
	}
}
