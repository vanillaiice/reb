// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"encoding/json"
	"errors"
	"html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/vanillaiice/reb/rebcompiler"
	"github.com/vanillaiice/reb/rebrender"
)

func raw(t *testing.T, text string) []rebcompiler.RebFieldSchema {
	t.Helper()
	var entries []rebcompiler.RebFieldSchema
	if err := json.Unmarshal([]byte(text), &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func keysAndKinds(schema Schema) ([]string, []string) {
	var keys, kinds []string
	for _, field := range schema.Fields {
		keys = append(keys, field.Key)
		kinds = append(kinds, field.Kind)
	}
	return keys, kinds
}

func TestNormalizeMapsTypesAndAliases(t *testing.T) {
	schema := Normalize(raw(t, `[{"key":"a","type":"string"},{"key":"b","type":"radio","options":["x","y"]},{"key":"c","type":"image"},
		{"key":"d","type":"attachments"},{"key":"e","type":"section","label":"Part B"},{"key":"f","type":"mystery"},
		{"key":"g","type":"checkbox"},{"type":"text"}]`))

	keys, kinds := keysAndKinds(schema)
	if !reflect.DeepEqual(keys, []string{"a", "b", "c", "d", "e", "f", "g"}) {
		t.Errorf("keys = %v", keys)
	}
	if !reflect.DeepEqual(kinds, []string{"text", "select", "images", "images", "section", "text", "checkbox"}) {
		t.Errorf("kinds = %v", kinds)
	}
	if schema.Version != 2 || schema.Fields[1].Type != "radio" || schema.Fields[0].MaxLength != TextLimit {
		t.Errorf("version, radio type or text limit wrong: %+v", schema)
	}
}

func TestParseColumnsAsTheMobileAppDoes(t *testing.T) {
	columns := ParseColumns([]string{"no:autoincrement", "item", "unit_price:number", "total:formula[qty*unit_price|3]", "ok:checkbox",
		"grade:select[A| B |C]", "pic:photo", "img:image", "sign:signature", "odd:weird", "plain:formula[a+b]", " "})

	var keys, kinds []string
	byKey := map[string]Column{}
	for _, column := range columns {
		keys = append(keys, column.Key)
		kinds = append(kinds, column.Kind)
		byKey[column.Key] = column
	}
	if !reflect.DeepEqual(keys, []string{"no", "item", "unit_price", "total", "ok", "grade", "pic", "img", "sign", "odd", "plain"}) {
		t.Errorf("keys = %v", keys)
	}
	if !reflect.DeepEqual(kinds, []string{"autoincrement", "text", "number", "formula", "checkbox", "select", "image", "image", "signature", "text", "formula"}) {
		t.Errorf("kinds = %v", kinds)
	}
	total := byKey["total"]
	if total.Expression != "qty*unit_price" || *total.Precision != 3 || total.Label != "Total" {
		t.Errorf("total = %+v", total)
	}
	if *byKey["plain"].Precision != 2 || !reflect.DeepEqual(byKey["grade"].Options, []string{"A", "B", "C"}) || byKey["unit_price"].Label != "Unit Price" {
		t.Errorf("plain, grade or unit_price wrong")
	}
}

func TestColumnsDeclaredInsideATableAreAnsweredPerRow(t *testing.T) {
	keys, _ := keysAndKinds(Normalize(raw(t, `[{"key":"rows","type":"table","options":["qty:number"]},{"key":"qty","type":"number"},{"key":"note","type":"text"}]`)))
	if !reflect.DeepEqual(keys, []string{"rows", "note"}) {
		t.Errorf("keys = %v", keys)
	}
}

func schemaFrom(t *testing.T, source string) Schema {
	t.Helper()
	compiled, err := Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	return compiled.Fields
}

func errorCodes(errs []FieldError) []string {
	var codes []string
	for _, e := range errs {
		codes = append(codes, e.Key+":"+e.Code)
	}
	return codes
}

func TestPrepareCleansAnswersAndComputesCells(t *testing.T) {
	schema := schemaFrom(t, `
		<reb-text name="site" label="Site"></reb-text>
		<reb-number name="crew" label="Crew"></reb-number>
		<reb-date name="day" label="Day"></reb-date>
		<reb-select name="shift" label="Shift" options="Day, Night"></reb-select>
		<reb-declare type="checkbox" name="safe" label="Safe"></reb-declare>
		<reb-declare type="section" name="part" label="Part"></reb-declare>
		<reb-photogrid name="photos" label="Photos"></reb-photogrid>
		<reb-signature name="sign" label="Sign"></reb-signature>
		<reb-table name="items" label="Items" options="no:autoincrement,item:text,qty:number,rate:number,total:formula[qty*rate|2],pic:photo"></reb-table>`)

	prepared := Prepare(schema, map[string]any{
		"site": "North", "crew": "12", "day": "20261002", "shift": "Night", "safe": "on", "part": "x", "unknown": "y",
		"photos": []any{" blob:1 ", "", "upload:2"}, "sign": "blob:3",
		"items": map[string]any{"1": map[string]any{"item": "B", "qty": "2", "rate": "1.5", "total": "999"}, "0": map[string]any{"item": "A", "qty": "3", "rate": "2", "pic": "blob:4"}},
	})

	if len(prepared.Errors) != 0 {
		t.Fatalf("errors = %v", prepared.Errors)
	}
	want := map[string]any{
		"site": "North", "crew": "12", "day": "2026-10-02", "shift": "Night", "safe": true,
		"photos": []any{"blob:1", "upload:2"}, "sign": "blob:3",
		"items": []any{
			map[string]any{"no": "1", "item": "A", "qty": "3", "rate": "2", "total": "6.00", "pic": "blob:4"},
			map[string]any{"no": "2", "item": "B", "qty": "2", "rate": "1.5", "total": "3.00", "pic": ""},
		},
	}
	if !reflect.DeepEqual(prepared.Answers, want) {
		t.Errorf("answers =\n%v\nwant\n%v", prepared.Answers, want)
	}
}

func TestPrepareRefusesBadAnswers(t *testing.T) {
	schema := schemaFrom(t, `
		<reb-text name="site" label="Site"></reb-text>
		<reb-number name="crew" label="Crew"></reb-number>
		<reb-date name="day" label="Day"></reb-date>
		<reb-select name="shift" label="Shift" options="Day, Night"></reb-select>
		<reb-table name="items" label="Items" options="qty:number"></reb-table>`)

	prepared := Prepare(schema, map[string]any{"site": []any{"x"}, "crew": "12 men", "day": "02/10/2026", "shift": "Evening", "items": "rows"})

	if got, want := errorCodes(prepared.Errors), []string{"site:invalid", "crew:not_a_number", "day:invalid_date", "shift:not_an_option", "items:invalid"}; !reflect.DeepEqual(got, want) {
		t.Errorf("errors = %v, want %v", got, want)
	}
	long := Prepare(schema, map[string]any{"site": strings.Repeat("é", TextLimit+1)})
	if errorCodes(long.Errors)[0] != "site:too_long" || len([]rune(long.Answers["site"].(string))) != TextLimit {
		t.Errorf("a long text is cut at the limit with an error: %v", long.Errors)
	}
}

func TestPrepareAppliesRequiredBoundsPatternAndShowIf(t *testing.T) {
	schema := schemaFrom(t, `
		<reb-select name="kind" label="Kind" options="Hot work, Cold work" required></reb-select>
		<reb-text name="permit" label="Permit" required show-if="kind == 'Hot work'" pattern="P-[0-9]+"></reb-text>
		<reb-text name="watch" label="Fire watch" show-if="permit"></reb-text>
		<reb-number name="crew" label="Crew" min="1" max="20"></reb-number>
		<reb-date name="day" label="Day" min="2026-01-01"></reb-date>
		<reb-declare type="checkbox" name="agree" label="Agree" required></reb-declare>`)

	prepared := Prepare(schema, map[string]any{"kind": "", "permit": "P-1", "watch": "Ali", "crew": "21", "day": "2025-12-31", "agree": "0"})
	if got, want := errorCodes(prepared.Errors), []string{"kind:blank", "crew:too_large", "day:too_small", "agree:blank"}; !reflect.DeepEqual(got, want) {
		t.Errorf("errors = %v, want %v", got, want)
	}
	if _, kept := prepared.Answers["permit"]; kept {
		t.Error("a hidden field's answer is dropped")
	}
	if _, kept := prepared.Answers["watch"]; kept {
		t.Error("a field shown only by a hidden field is hidden too")
	}

	prepared = Prepare(schema, map[string]any{"kind": "Hot work", "permit": "X-1", "crew": "0", "agree": true})
	if got, want := errorCodes(prepared.Errors), []string{"permit:invalid", "crew:too_small"}; !reflect.DeepEqual(got, want) {
		t.Errorf("errors = %v, want %v", got, want)
	}
	prepared = Prepare(schema, map[string]any{"kind": "Hot work", "agree": true})
	if got := errorCodes(prepared.Errors); !reflect.DeepEqual(got, []string{"permit:blank"}) {
		t.Errorf("a visible required field must be answered: %v", got)
	}
}

func TestParagraphs(t *testing.T) {
	// Spaces opening a later paragraph stay, as Rebar's Ruby conversion kept them.
	if got := Paragraphs("  Line one\nLine <two>\r\n\r\n  Next  "); got != "<p>Line one<br>Line &lt;two&gt;</p><p>  Next</p>" {
		t.Errorf("Paragraphs = %q", got)
	}
	if got := Paragraphs(" \n "); got != "" {
		t.Errorf("blank text gives nothing, got %q", got)
	}
}

func TestBuildContextTurnsTextAreasIntoParagraphs(t *testing.T) {
	schema := schemaFrom(t, `<reb-textarea name="notes" label="Notes"></reb-textarea><reb-text name="site" label="Site"></reb-text>`)
	context := BuildContext(map[string]any{"Name": "R1"}, map[string]any{"notes": "a\nb\n\nc", "site": "x\ny"}, nil, &schema)

	if got := context["notes"].(template.HTML); got != "<p>a<br>b</p><p>c</p>" {
		t.Errorf("notes = %q", got)
	}
	if got := context["site"].(template.HTML); got != "x\ny" {
		t.Errorf("a text field stays as typed: %q", got)
	}
}

func TestCompileErrorsAndWarnings(t *testing.T) {
	for source, code := range map[string]string{
		`<reb-text name="site-name" label="x"></reb-text>`:              "invalid_field_name",
		`<reb-text name="a" label="A" show-if="kind =="></reb-text>`:    "invalid_show_if",
		`<reb-text name="a" label="A" pattern="[a-"></reb-text>`:        "invalid_pattern",
		`<reb-text name="a" label="A"></reb-text>{{if .a}}open`:         "syntax",
		`<reb-text name="a" label="A"></reb-text>{{nosuchfunction .a}}`: "syntax",
	} {
		_, err := Compile(source)
		var coded *Error
		if !errors.As(err, &coded) || coded.Code != code {
			t.Errorf("Compile(%q) = %v, want code %s", source, err, code)
		}
	}

	compiled, err := Compile(`<reb-text name="a" show-if="ghost"></reb-text><reb-header>Head</reb-header>`)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, warning := range compiled.Warnings {
		codes = append(codes, warning.Code)
	}
	if !reflect.DeepEqual(codes, []string{"show_if_unknown_field", "missing_label"}) {
		t.Errorf("warnings = %v", codes)
	}
	if !strings.Contains(compiled.HTML, `<rebar-pdf-header-extract style="display:none;">Head</rebar-pdf-header-extract>`) {
		t.Errorf("the header is hidden for the PDF client to extract: %s", compiled.HTML)
	}
	if compiled.EngineVersion == "" || compiled.Fields.Fields[0].ShowIf != "ghost" {
		t.Errorf("engine version and attributes expected: %+v", compiled)
	}
}

func TestSampleAnswersComputeFormulasAndUseDefaults(t *testing.T) {
	schema := schemaFrom(t, `<reb-text name="site" label="Site" default="North"></reb-text>
		<reb-table name="items" label="Items" options="qty:number,rate:number,total:formula[qty*rate|1]"></reb-table>`)
	answers := SampleAnswers(schema)

	if answers["site"] != "North" {
		t.Errorf("site = %v", answers["site"])
	}
	rows := answers["items"].([]any)
	if first := rows[0].(map[string]any); first["qty"] != "5" || first["rate"] != "10" || first["total"] != "50.0" {
		t.Errorf("first row = %v", first)
	}
}

func TestSampleImagesRenderInSrcAttributes(t *testing.T) {
	compiled, err := Compile(`<reb-signature name="sig" label="Signature"></reb-signature>
		<reb-photogrid name="photos" label="Photos"></reb-photogrid>
		<reb-table name="rows" label="Rows" options="photo:photo"><table><tr reb-row><td><img src="{{.photo}}"></td></tr></table></reb-table>
		<img src="{{.OrganizationLogo}}">`)
	if err != nil {
		t.Fatal(err)
	}
	html, err := rebrender.CompileHTML(compiled.HTML, BuildContext(SampleSystem(), SampleAnswers(compiled.Fields), nil, &compiled.Fields))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "ZgotmplZ") {
		t.Errorf("a sample image was filtered out:\n%s", html)
	}
	if got := strings.Count(html, `src="data:image/svg`); got != 6 { // signature, two photos, two row photos, logo
		t.Errorf("%d sample images, want 6:\n%s", got, html)
	}
	var decoded string
	encoded, _ := json.Marshal(SampleAnswers(compiled.Fields)["sig"])
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded != PlaceholderImage {
		t.Errorf("sample signature encodes as %s", encoded)
	}
}

func warningCodes(t *testing.T, source string) []string {
	t.Helper()
	compiled, err := Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, w := range compiled.Warnings {
		codes = append(codes, w.Code+":"+w.Params["field"]+w.Params["name"]+"@"+w.Params["table"])
	}
	return codes
}

func TestLintReportsUnknownBindingsUnusedAndConflictingFields(t *testing.T) {
	got := warningCodes(t, `<p>{{.Reference}} {{.clinet}}</p>
		<reb-declare name="kept" label="Kept"></reb-declare>
		<reb-declare name="unused" label="Unused"></reb-declare>{{if .kept}}x{{end}}
		<reb-text name="dup" label="Dup"></reb-text><reb-number name="dup" label="Dup"></reb-number>
		<reb-table name="rows" label="Rows" options="qty:number"><table><tr reb-row><td>{{.qty}} {{.qtty}}</td></tr></table></reb-table>`)
	want := []string{"unknown_binding:clinet@", "unknown_binding:qtty@rows", "unused_field:unused@", "duplicate_field:dup@"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warnings = %v, want %v", got, want)
	}
}

func TestLintAcceptsWhatTemplatesDo(t *testing.T) {
	got := warningCodes(t, `<reb-tailwind></reb-tailwind>
		<p>{{.Name}} {{now | formatDate "02/01/2006"}} {{formatDate "2006" .CreatedAt}} {{.Answers.site}}</p>
		<reb-text name="site" label="Site"></reb-text><reb-text name="site" label="Site again"></reb-text>
		<reb-photogrid name="photos" label="Photos"></reb-photogrid>
		<reb-signature name="sig" label="Signature"></reb-signature>
		<reb-declare name="gate" label="Gate" type="checkbox"></reb-declare>
		<reb-declare name="detail" label="Detail" show-if="gate"></reb-declare>{{.detail}}
		<reb-table name="items" label="Items" options="qty:number,rate:number,amount:formula[qty*rate|2]">
		<table><tr reb-row><td>{{.qty}} {{.amount}} {{$.site}}</td></tr></table>
		<p>{{sumColumn .items "amount" | formatMoney "QAR" 2}}</p></reb-table>
		{{with .site}}{{.anything}}{{end}}{{range .photos}}<img src="{{.}}">{{end}}`)
	if len(got) != 0 {
		t.Errorf("warnings = %v, want none", got)
	}
}
