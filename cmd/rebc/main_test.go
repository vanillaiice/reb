// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func runJSON(t *testing.T, command string, input any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := run(command, raw)
	if err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not JSON: %s", out)
	}
	return result
}

func TestCompileReturnsSchemaAndHTML(t *testing.T) {
	result := runJSON(t, "compile", map[string]string{"reb": `<reb-text name="site" label="Site" /><reb-photogrid name="photos" label="Photos" />`})

	schema := result["schema"].([]any)
	if len(schema) != 2 {
		t.Fatalf("expected 2 fields, got %v", schema)
	}
	if html := result["html"].(string); !strings.Contains(html, "{{.site}}") || !strings.Contains(html, "{{range .photos}}") {
		t.Errorf("unexpected html %q", html)
	}
}

func TestCompileRejectsInvalidFieldName(t *testing.T) {
	if _, err := run("compile", []byte(`{"reb": "<reb-text name=\"site-name\" label=\"x\"></reb-text>"}`)); err == nil {
		t.Fatal("expected an error for an invalid field name")
	}
}

func TestRenderReplacesAssetsAndSanitizes(t *testing.T) {
	compiled := runJSON(t, "compile", map[string]string{"reb": `<reb-textarea name="notes" label="Notes"></reb-textarea>` +
		`<reb-signature name="sig" label="Signature"></reb-signature>` +
		`<reb-photogrid name="photos" label="Photos"></reb-photogrid>` +
		`<reb-table name="rows" label="Rows" options="photo:photo"><table><tbody><tr reb-row><td><img src="{{.photo}}"></td></tr></tbody></table></reb-table>` +
		`<p>{{.ID}}</p>`})

	result := runJSON(t, "render", map[string]any{
		"html":   compiled["html"],
		"system": map[string]any{"ID": "doc-1", "notes": "system keys win"},
		"answers": map[string]any{
			"notes":  "<p>ok</p><script>alert(1)</script>",
			"sig":    "blob:a",
			"photos": []any{"blob:b"},
			"rows":   []any{map[string]any{"photo": "blob:c"}},
		},
		"assets": map[string]string{"blob:a": "asset-a.svg", "blob:b": "asset-b.jpg", "blob:c": "asset-c.png"},
	})

	html := result["html"].(string)
	for _, want := range []string{`src="asset-a.svg"`, `src="asset-b.jpg"`, `src="asset-c.png"`, "<p>doc-1</p>", "system keys win"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered html is missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("script survived sanitizing:\n%s", html)
	}
}

func TestRenderSanitizesAnswersWhenFlattened(t *testing.T) {
	result := runJSON(t, "render", map[string]any{
		"html":    `{{.notes}}|{{multiply .qty .rate | formatNumber 2}}`,
		"answers": map[string]any{"notes": `<b>bold</b><img src=x onerror=alert(1)>`, "qty": "3", "rate": 2.5},
	})

	html := result["html"].(string)
	if !strings.Contains(html, "<b>bold</b>") || strings.Contains(html, "onerror") {
		t.Errorf("unexpected sanitizing result %q", html)
	}
	if !strings.HasSuffix(html, "|7.50") {
		t.Errorf("math on a sanitized answer: got %q", html)
	}
}

func TestUnknownCommand(t *testing.T) {
	if _, err := run("explode", []byte(`{}`)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCompileReturnsNormalizedFieldsAndEngineVersion(t *testing.T) {
	out := runJSON(t, "compile", map[string]string{"reb": `<reb-text name="site" label="Site" required help="Gate code"></reb-text>`})

	fields := out["fields"].(map[string]any)
	first := fields["fields"].([]any)[0].(map[string]any)
	if fields["schemaVersion"] != 2.0 || first["kind"] != "text" || first["required"] != true || first["help"] != "Gate code" {
		t.Errorf("fields = %v", fields)
	}
	if out["engineVersion"] == "" || len(out["warnings"].([]any)) != 0 {
		t.Errorf("engine version and no warnings expected: %v", out)
	}
}

func TestCodedErrorsCarryCodeAndParams(t *testing.T) {
	_, err := run("compile", []byte(`{"reb": "<reb-text name=\"site-name\" label=\"x\"></reb-text>"}`))
	var coded interface{ Error() string }
	if coded = err; coded == nil {
		t.Fatal("expected an error")
	}
	out, _ := json.Marshal(err)
	if !strings.Contains(string(out), `"code":"invalid_field_name"`) || !strings.Contains(string(out), `"name":"site-name"`) {
		t.Errorf("error JSON = %s", out)
	}
}

func TestPrepareFromRawOrNormalizedSchema(t *testing.T) {
	raw := []map[string]any{{"key": "items", "type": "table", "options": []string{"qty:number", "total:formula[qty*3|0]"}}, {"key": "crew", "type": "number"}}
	for _, input := range []map[string]any{
		{"schema": raw, "answers": map[string]any{"items": []any{map[string]any{"qty": "2"}}, "crew": "x"}},
		{"fields": runJSON(t, "normalize", map[string]any{"schema": raw}), "answers": map[string]any{"items": []any{map[string]any{"qty": "2"}}, "crew": "x"}},
	} {
		out := runJSON(t, "prepare", input)
		row := out["answers"].(map[string]any)["items"].([]any)[0].(map[string]any)
		errs := out["errors"].([]any)
		if row["total"] != "6" || len(errs) != 1 || errs[0].(map[string]any)["code"] != "not_a_number" {
			t.Errorf("prepare = %v", out)
		}
	}
}

func TestRenderWithFieldsTurnsTextAreasIntoParagraphs(t *testing.T) {
	fields := runJSON(t, "normalize", map[string]any{"schema": []map[string]any{{"key": "notes", "type": "textarea"}}})
	out := runJSON(t, "render", map[string]any{"html": "{{.notes}}", "answers": map[string]any{"notes": "a\n\nb <c>"}, "fields": fields})
	if out["html"] != "<p>a</p><p>b &lt;c&gt;</p>" {
		t.Errorf("html = %v", out["html"])
	}
}

func TestVersion(t *testing.T) {
	if out := runJSON(t, "version", map[string]any{}); !strings.HasPrefix(out["version"].(string), "v") {
		t.Errorf("version = %v", out)
	}
}
