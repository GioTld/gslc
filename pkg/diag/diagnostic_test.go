package diag

import (
	"strings"
	"testing"
)

func TestSpanMerge(t *testing.T) {
	p1 := Position{Filename: "test.gsl", Line: 1, Column: 1, Offset: 0}
	p2 := Position{Filename: "test.gsl", Line: 1, Column: 5, Offset: 4}
	p3 := Position{Filename: "test.gsl", Line: 1, Column: 10, Offset: 9}

	s1 := NewSpan(p1, p2)
	s2 := NewSpan(p2, p3)

	merged := s1.Merge(s2)
	if merged.Start != p1 || merged.End != p3 {
		t.Errorf("expected merged span %v to be from %v to %v", merged, p1, p3)
	}
}

func TestDiagnosticFormat(t *testing.T) {
	source := "let val = 42;\nlet err = !@#;"
	span := NewSpan(
		Position{Filename: "main.gsl", Line: 2, Column: 11, Offset: 25},
		Position{Filename: "main.gsl", Line: 2, Column: 14, Offset: 28},
	)

	d := NewError(span, "unexpected token sequence").WithHint("remove illegal characters")
	formatted := d.Format(source)

	if !strings.Contains(formatted, "error: unexpected token sequence") {
		t.Errorf("expected formatted output to contain error message, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "--> main.gsl:2:11") {
		t.Errorf("expected location indicator, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "^^^") {
		t.Errorf("expected carets under error, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "= hint: remove illegal characters") {
		t.Errorf("expected hint, got:\n%s", formatted)
	}
}
