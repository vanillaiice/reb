// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestShowIfCases(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/show_if_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Expression string         `json:"expression"`
			Answers    map[string]any `json:"answers"`
			Expected   bool           `json:"expected"`
		} `json:"cases"`
		Invalid []string `json:"invalid"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	for _, c := range file.Cases {
		condition, err := ParseShowIf(c.Expression)
		if err != nil {
			t.Errorf("ParseShowIf(%q): %v", c.Expression, err)
			continue
		}
		if got := condition.Holds(c.Answers); got != c.Expected {
			t.Errorf("%q with %v = %v, want %v", c.Expression, c.Answers, got, c.Expected)
		}
	}
	for _, expression := range file.Invalid {
		if _, err := ParseShowIf(expression); err == nil {
			t.Errorf("ParseShowIf(%q) accepted an invalid condition", expression)
		}
	}
}

func TestShowIfFields(t *testing.T) {
	condition, err := ParseShowIf("kind == 'x' and (crew or not safe)")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"kind", "crew", "safe"}; !reflect.DeepEqual(condition.Fields, want) {
		t.Errorf("Fields = %v, want %v", condition.Fields, want)
	}
}

func TestVisibilityCases(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/visibility_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name    string         `json:"name"`
			Fields  []Field        `json:"fields"`
			Answers map[string]any `json:"answers"`
			Visible []string       `json:"visible"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	for _, c := range file.Cases {
		visible := visibility(Schema{Version: SchemaVersion, Fields: c.Fields}, c.Answers)
		got := []string{}
		for _, field := range c.Fields {
			if field.Stored() && visible[field.Key] {
				got = append(got, field.Key)
			}
		}
		if !reflect.DeepEqual(got, c.Visible) {
			t.Errorf("%s: visible = %v, want %v", c.Name, got, c.Visible)
		}
	}
}
