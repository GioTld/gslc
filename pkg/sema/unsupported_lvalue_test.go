package sema

import (
	"strings"
	"testing"

	"github.com/GioTld/gslc/pkg/lexer"
	"github.com/GioTld/gslc/pkg/parser"
)

func TestUnsupportedIndexAndPointerOperationsAreRejected(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{"index read", "func main(items: []u8) u8 { return items[0 as usize] }", "slice indexing is not supported by the LLVM backend"},
		{"index write", "func main(items: []u8) { items[0 as usize] = 1 as u8 }", "assignment target is not supported by the LLVM backend"},
		{"pointer write", "func main(ptr: *u8) { unsafe { *ptr = 1 as u8 } }", "assignment target is not supported by the LLVM backend"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := parser.New(lexer.New("test.gsl", test.source))
			program := p.ParseProgram()
			if diagnostics := p.Diagnostics(); len(diagnostics) != 0 {
				t.Fatalf("unexpected parser diagnostics: %v", diagnostics)
			}
			analyzer := New()
			analyzer.AnalyzeProgram(program)
			for _, diagnostic := range analyzer.Diagnostics() {
				if strings.Contains(diagnostic.Message, test.message) {
					return
				}
			}
			t.Fatalf("expected %q, got %v", test.message, analyzer.Diagnostics())
		})
	}
}

func TestCharacterLiteralIsRejectedBeforeCodegen(t *testing.T) {
	analyzer := parseAndAnalyze("func main() u8 { return 'A' as u8 }")
	for _, diagnostic := range analyzer.Diagnostics() {
		if strings.Contains(diagnostic.Message, "character literals are not supported") {
			return
		}
	}
	t.Fatalf("expected character literal diagnostic, got %v", analyzer.Diagnostics())
}
