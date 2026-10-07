package rebcompiler

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompile(t *testing.T) {
	rawHTML := `
		<div class="grid">
			<h1>Test Form</h1>
			<p>Project: <reb-text name="project" label="Project Name" class="font-bold" /></p>
			<reb-photogrid name="photos" label="Site Photos" />
		</div>
	`

	schema, finalHTML, err := Compile(rawHTML)
	if err != nil {
		t.Fatalf("failed to compile: %v", err)
	}

	if len(schema) != 2 {
		t.Fatalf("expected 2 schema items, got %d", len(schema))
	}

	if schema[0].Key != "project" || schema[0].Type != "text" {
		t.Errorf("unexpected first schema item: %+v", schema[0])
	}
	if schema[1].Key != "photos" || schema[1].Type != "photogrid" {
		t.Errorf("unexpected second schema item: %+v", schema[1])
	}

	// Output shouldn't have <reb-text> anymore
	if strings.Contains(finalHTML, "<reb-text") {
		t.Errorf("expected <reb-text> to be stripped, got: %s", finalHTML)
	}

	// Should contain the go template replacement
	if !strings.Contains(finalHTML, `{{.project}}`) {
		t.Errorf("expected go template for project, got: %s", finalHTML)
	}
	if !strings.Contains(finalHTML, `{{range .photos}}`) {
		t.Errorf("expected go template loop for photos, got: %s", finalHTML)
	}
}

func TestCompileSelfClosingSiblings(t *testing.T) {
	// HTML5 ignores "/>" on custom elements; the first tag used to swallow the
	// second, dropping field b from the schema and the output.
	schema, out, err := Compile(`<div><reb-text name="a" label="A" /> and <reb-text name="b" label="B" /></div>`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(schema) != 2 || schema[1].Key != "b" {
		t.Fatalf("expected fields a and b, got %+v", schema)
	}
	if want := `<div><span>{{.a}}</span> and <span>{{.b}}</span></div>`; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestCompileKeepsLeadingStyle(t *testing.T) {
	// A leading <style> is moved into <head> by the parser and used to be
	// dropped when the snippet wrapper was stripped.
	_, out, err := Compile("<style>.x{color:red}</style>\n<div class=\"x\"><reb-text name=\"a\" label=\"A\"></reb-text></div>")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.HasPrefix(out, "<style>.x{color:red}</style>") || !strings.Contains(out, `<div class="x"><span>{{.a}}</span></div>`) {
		t.Errorf("style or body lost: %q", out)
	}
}

func TestCompileTrimsOptions(t *testing.T) {
	schema, _, err := Compile(`<reb-select name="sev" label="Severity" options="High, Medium , Low,"></reb-select>`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := strings.Join(schema[0].Options, "|"); got != "High|Medium|Low" {
		t.Errorf("options = %q", got)
	}
}

func TestCompileRejectsInvalidFieldNames(t *testing.T) {
	for _, name := range []string{"site-name", "1st", "a b", "x}}{{.y"} {
		if _, _, err := Compile(`<reb-text name="` + name + `" label="L"></reb-text>`); err == nil {
			t.Errorf("expected an error for name %q", name)
		}
	}
	if _, _, err := Compile(`<reb-text name="siteName_2" label="L"></reb-text>`); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
}

func TestCompileRebRowWithAttributes(t *testing.T) {
	_, out, err := Compile(`<reb-table name="t" label="T" options="a:text"><table><tbody><reb-row class="r"><td>{{.a}}</td></reb-row></tbody></table></reb-table>`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if want := `{{range .t}}<tr class="r"><td>{{.a}}</td></tr>{{end}}`; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}

func TestQuotesInActionsOverSeveralLines(t *testing.T) {
	_, html, err := Compile("<reb-table name=\"items\" label=\"Items\" options=\"amount:number\">\n<p>Total: {{sumColumn .items \"amount\"\n  | formatMoney \"QAR\" 2}}</p>\n</reb-table>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "{{sumColumn .items \"amount\"\n  | formatMoney \"QAR\" 2}}") {
		t.Errorf("quotes stay escaped in a multi-line action:\n%s", html)
	}
}

func TestCompileWithoutFieldsHasAnEmptySchema(t *testing.T) {
	schema, _, err := Compile(`<p>no fields</p>`)
	if err != nil || schema == nil || len(schema) != 0 {
		t.Errorf("schema = %#v, err = %v", schema, err)
	}
}

func TestBodyInAnAttributeIsNotABodyTag(t *testing.T) {
	_, out, err := Compile(`<p title="<body>">x</p>`)
	if err != nil {
		t.Fatal(err)
	}
	if out != `<p title="&lt;body&gt;">x</p>` {
		t.Errorf("got %q", out)
	}
}

// pdf-forms:boxes
func TestCompileFillableFields(t *testing.T) {
	schema, out, err := Compile(`<p><reb-text name="client" label="Client" class="w-64" fillable /></p>
		<reb-textarea name="notes" label="Notes" fillable></reb-textarea>
		<reb-number name="qty" label="Qty" fillable="false" />
		<reb-select name="grade" label="Grade" options="A,B" fillable></reb-select>
		<reb-checkbox name="ok" label="OK" fillable></reb-checkbox>`)
	if err != nil {
		t.Fatal(err)
	}
	fillable := map[string]bool{}
	for _, field := range schema {
		fillable[field.Key] = field.Fillable
	}
	if want := map[string]bool{"client": true, "notes": true, "qty": false, "grade": false, "ok": true}; !reflect.DeepEqual(fillable, want) {
		t.Errorf("fillable = %v, want %v", fillable, want)
	}
	for _, want := range []string{
		`{{if $.Fillable}}<a href="reb-field:client" class="reb-fillable w-64"></a>{{else}}<span class="w-64">{{.client}}</span>{{end}}`,
		`{{if $.Fillable}}<a href="reb-field:notes;multiline" class="reb-fillable reb-fillable-multiline"></a>{{else}}<div `,
		`{{if $.Fillable}}<a href="reb-field:ok;checkbox" class="reb-fillable reb-fillable-checkbox"></a>{{else}}<span>{{.ok}}</span>{{end}}`,
		`<span>{{.qty}}</span>`,
		`<span>{{.grade}}</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compiled template lacks %s:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "<style>"); n != 1 {
		t.Errorf("the box style appears %d times, want once:\n%s", n, out)
	}
}
