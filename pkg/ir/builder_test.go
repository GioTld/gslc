package ir

import (
	"strings"
	"testing"

	"github.com/GioTld/gslc/pkg/lexer"
	"github.com/GioTld/gslc/pkg/parser"
	"github.com/GioTld/gslc/pkg/sema"
)

func buildIR(input string) *Module {
	l := lexer.New("test.gsl", input)
	p := parser.New(l)
	prog := p.ParseProgram()
	sem := sema.New()
	sem.AnalyzeProgram(prog)
	b := NewBuilder(sem)
	return b.Build(prog)
}

func TestBuildSimpleFunction(t *testing.T) {
	input := `
func add(a: u32, b: u32) u32 {
    return a + b
}
`
	mod := buildIR(input)
	if len(mod.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(mod.Functions))
	}

	fn := mod.Functions[0]
	if fn.Name != "add" {
		t.Errorf("expected func name 'add', got %s", fn.Name)
	}

	irStr := fn.String()
	if !strings.Contains(irStr, "alloca") {
		t.Errorf("expected stack allocas for params, got:\n%s", irStr)
	}
	if !strings.Contains(irStr, "add") {
		t.Errorf("expected add binary op, got:\n%s", irStr)
	}
	if !strings.Contains(irStr, "ret") {
		t.Errorf("expected ret instruction, got:\n%s", irStr)
	}
}

func TestBuildVolatileMMIO(t *testing.T) {
	input := `
func write_mmio(addr: usize, val: u8) {
    unsafe {
        let ptr = addr as *u8
        volatile_write(ptr, val)
    }
}
`
	mod := buildIR(input)
	irStr := mod.String()

	if !strings.Contains(irStr, "store volatile") {
		t.Errorf("expected 'store volatile' instruction in IR, got:\n%s", irStr)
	}
}

func TestBuildIfElseBranches(t *testing.T) {
	input := `
func check(val: u32) u32 {
    if val == 0 {
        return 1
    } else {
        return 2
    }
}
`
	mod := buildIR(input)
	irStr := mod.String()

	if !strings.Contains(irStr, "br") || !strings.Contains(irStr, "then") || !strings.Contains(irStr, "else") {
		t.Errorf("expected branch instructions and then/else labels, got:\n%s", irStr)
	}
}
