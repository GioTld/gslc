package sema

import (
	"strings"
	"testing"
)

func TestExhaustiveBoolMatch(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{"complete", `func main(flag: bool) u32 { return match flag { true => 1, false => 2 } }`, ""},
		{"missing arm", `func main(flag: bool) u32 { return match flag { true => 1 } }`, "non-exhaustive bool match"},
		{"default", `func main(flag: bool) u32 { return match flag { true => 1, _ => 2 } }`, ""},
		{"duplicate", `func main(flag: bool) u32 { return match flag { true => 1, true => 2, false => 3 } }`, "duplicate match pattern"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analyzer := parseAndAnalyze(test.source)
			if test.wantError == "" {
				if len(analyzer.Diagnostics()) != 0 {
					t.Fatalf("unexpected diagnostics: %v", analyzer.Diagnostics())
				}
				return
			}
			for _, diagnostic := range analyzer.Diagnostics() {
				if strings.Contains(diagnostic.Message, test.wantError) {
					return
				}
			}
			t.Fatalf("expected %q, got %v", test.wantError, analyzer.Diagnostics())
		})
	}
}

func TestExhaustiveAlgebraicMatch(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{"option complete", `func get(value: Option[u32]) u32 { return match value { Option.Some(n) => n, Option.None() => 0 } }`, ""},
		{"option missing none", `func get(value: Option[u32]) u32 { return match value { Option.Some(n) => n } }`, "non-exhaustive Option match"},
		{"result complete", `func get(value: Result[u32, u32]) u32 { return match value { Result.Ok(n) => n, Result.Err(e) => e } }`, ""},
		{"result missing err", `func get(value: Result[u32, u32]) u32 { return match value { Result.Ok(n) => n } }`, "non-exhaustive Result match"},
		{"duplicate variant", `func get(value: Option[u32]) u32 { return match value { Option.Some(a) => a, Option.Some(b) => b, Option.None() => 0 } }`, "duplicate match pattern"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analyzer := parseAndAnalyze(test.source)
			if test.wantError == "" {
				if len(analyzer.Diagnostics()) != 0 {
					t.Fatalf("unexpected diagnostics: %v", analyzer.Diagnostics())
				}
				return
			}
			for _, diagnostic := range analyzer.Diagnostics() {
				if strings.Contains(diagnostic.Message, test.wantError) {
					return
				}
			}
			t.Fatalf("expected %q, got %v", test.wantError, analyzer.Diagnostics())
		})
	}
}
