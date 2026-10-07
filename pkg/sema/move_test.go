package sema

import (
	"strings"
	"testing"
)

func TestMoveSemantics(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{"use after move", `func main() { let first = Channel.new(2); let second = first; first.recv() }`, true},
		{"branch move", `func main(flag: bool) { let first = Channel.new(2); if flag { let second = first }; first.recv() }`, true},
		{"reinitialize", `func main() { var first = Channel.new(2); let second = first; first = Channel.new(3); first.recv() }`, false},
		{"copy value", `func main() { let first: u32 = 2; let second = first; let third = first }`, false},
		{"borrow value", `func main() { let first = Channel.new(2); let view = &first; first.recv() }`, false},
		{"move in loop", `func main() { let first = Channel.new(2); while true { let second = first } }`, true},
		{"function argument", `func take(value: Channel[u32]) {} func main() { let first = Channel.new(2); take(first); first.recv() }`, true},
		{"return value", `func forward(value: Channel[u32]) Channel[u32] { return value }`, false},
		{"move field", `struct Holder { ch: Channel[u32] } func main(holder: Holder) { let second = holder.ch; holder.ch.recv() }`, true},
		{"plain struct moves", `struct Token { kind: u32 } func main(value: Token) { let next = value; let again = value }`, true},
		{"copy struct", `copy struct Token { kind: u32 } func main(value: Token) { let next = value; let again = value }`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analyzer := parseAndAnalyze(test.source)
			found := false
			for _, diagnostic := range analyzer.Diagnostics() {
				if strings.Contains(diagnostic.Message, "use of moved value") || strings.Contains(diagnostic.Message, "moving a value from an outer scope") {
					found = true
				}
			}
			if found != test.wantError {
				t.Fatalf("use-after-move diagnostic = %t, want %t: %v", found, test.wantError, analyzer.Diagnostics())
			}
		})
	}
}

func TestCopyStructRejectsOwnedField(t *testing.T) {
	source := `copy struct Holder { channel: Channel[u32] }`
	analyzer := parseAndAnalyze(source)
	for _, diagnostic := range analyzer.Diagnostics() {
		if strings.Contains(diagnostic.Message, "contains non-copy field") {
			return
		}
	}
	t.Fatalf("expected non-copy field error, got %v", analyzer.Diagnostics())
}
