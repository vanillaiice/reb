package rebrender

import (
	"html/template"
	"testing"
)

func render(t *testing.T, src string, data map[string]any) string {
	t.Helper()
	out, err := CompileHTML(src, data)
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return out
}

func TestMathOnSanitizedAnswers(t *testing.T) {
	// The document export passes top-level answers as template.HTML; math on
	// them used to evaluate to 0.
	data := map[string]any{"qty": template.HTML("3"), "rate": template.HTML(" 2.5 ")}
	if got := render(t, `{{multiply .qty .rate}} {{add .qty .rate}} {{subtract .qty .rate}} {{divide .qty .rate}}`, data); got != "7.5 5.5 0.5 1.2" {
		t.Errorf("got %q", got)
	}
}

func TestFormatNumberBothArgumentOrders(t *testing.T) {
	data := map[string]any{"val": 3.14159, "rows": []any{map[string]any{"amount": "1.5"}, map[string]any{"amount": 2.0}}}
	cases := map[string]string{
		`{{formatNumber .val 2}}`:                       "3.14", // form documented in the .reb spec
		`{{formatNumber 2 .val}}`:                       "3.14",
		`{{sumColumn .rows "amount" | formatNumber 2}}`: "3.50",
		`{{multiply .val 2 | formatNumber 0}}`:          "6",
	}
	for src, want := range cases {
		if got := render(t, src, data); got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

func TestSafeHTMLMissingValue(t *testing.T) {
	if got := render(t, `[{{.missing | safeHTML}}][{{.nil | safeHTML}}]`, map[string]any{"nil": nil}); got != "[][]" {
		t.Errorf("got %q", got)
	}
}
