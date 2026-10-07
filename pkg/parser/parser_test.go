package parser

import (
	"testing"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/lexer"
)

func TestParsePrecedence(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1 + 2 * 3", "(1 + (2 * 3))"},
		{"a && b || c", "((a && b) || c)"},
		{"a == b && c < d", "((a == b) && (c < d))"},
		{"-a * b", "((-a) * b)"},
		{"!flag && true", "((!flag) && true)"},
	}

	for _, tt := range tests {
		l := lexer.New("test.gsl", tt.input)
		p := New(l)
		expr := p.parseExpr(precLowest)
		if len(p.Diagnostics()) > 0 {
			t.Fatalf("parser diagnostics on %q: %v", tt.input, p.Diagnostics())
		}
		if expr == nil {
			t.Fatalf("expected expr for %q, got nil", tt.input)
		}
		// Compare expression representation
		actual := expr.String()
		if actual != tt.expected {
			t.Errorf("for input %q: expected %q, got %q", tt.input, tt.expected, actual)
		}
	}
}

func TestParseFuncDecl(t *testing.T) {
	input := `
func add(a: i32, b: i32) i32 {
    return a + b
}
`
	l := lexer.New("test.gsl", input)
	p := New(l)
	prog := p.ParseProgram()

	if len(p.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics, got %v", p.Diagnostics())
	}

	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(prog.Decls))
	}

	fn, ok := prog.Decls[0].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("expected *ast.FuncDecl, got %T", prog.Decls[0])
	}

	if fn.Name != "add" {
		t.Errorf("expected func name 'add', got %s", fn.Name)
	}

	if len(fn.Params) != 2 {
		t.Errorf("expected 2 parameters, got %d", len(fn.Params))
	}

	if fn.ReturnType == nil || fn.ReturnType.String() != "i32" {
		t.Errorf("expected return type 'i32', got %v", fn.ReturnType)
	}
}

func TestParseStructAndArena(t *testing.T) {
	input := `
arena global_pool(1024 * 1024)

struct Node {
    id: u64,
    next: *Node,
}
`
	l := lexer.New("test.gsl", input)
	p := New(l)
	prog := p.ParseProgram()

	if len(p.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics, got %v", p.Diagnostics())
	}

	if len(prog.Decls) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(prog.Decls))
	}

	arenaDecl, ok := prog.Decls[0].(*ast.ArenaDecl)
	if !ok {
		t.Fatalf("expected *ast.ArenaDecl, got %T", prog.Decls[0])
	}
	if arenaDecl.Name != "global_pool" {
		t.Errorf("expected arena name 'global_pool', got %s", arenaDecl.Name)
	}

	structDecl, ok := prog.Decls[1].(*ast.StructDecl)
	if !ok {
		t.Fatalf("expected *ast.StructDecl, got %T", prog.Decls[1])
	}
	if structDecl.Name != "Node" {
		t.Errorf("expected struct name 'Node', got %s", structDecl.Name)
	}
	if len(structDecl.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(structDecl.Fields))
	}
}

func TestParseSampleFile(t *testing.T) {
	input := `
arena memory_pool(64 * 1024)

struct UartPort {
    base_address: usize,
}

func uart_write_byte(port: UartPort, byte: u8) {
    unsafe {
        let reg = port.base_address as *u8
        volatile_write(reg, byte)
    }
}

func process_packet(raw_data: []u8) Result[u32, Error] {
    var counter: u32 = 0
    for b in raw_data {
        if b == 0 {
            break
        }
        counter += 1
    }
    return Result.Ok(counter)
}

func process_event(event: Event) {
    switch event {
    case Event.Data(payload):
        handle_data(payload)
    case Event.Disconnect:
        cleanup()
    default:
        log.warn("ignored event")
    }
}
`
	l := lexer.New("sample.gsl", input)
	p := New(l)
	prog := p.ParseProgram()

	if len(p.Diagnostics()) > 0 {
		for _, d := range p.Diagnostics() {
			t.Log(d.Format(input))
		}
		t.Fatalf("expected 0 diagnostics parsing sample file, got %d", len(p.Diagnostics()))
	}

	if len(prog.Decls) != 5 {
		t.Fatalf("expected 5 declarations (arena, struct, 3 funcs), got %d", len(prog.Decls))
	}
}

func TestParsePackedStructAndAsm(t *testing.T) {
	input := `
packed struct IDTEntry {
    offset_low: u16,
    selector: u16,
    ist: u8,
    type_attr: u8,
    offset_mid: u16,
    offset_high: u32,
    zero: u32,
}

func cpu_disable_interrupts() {
    unsafe {
        asm("cli");
    }
}
`
	l := lexer.New("test.gsl", input)
	p := New(l)
	prog := p.ParseProgram()

	if len(p.Diagnostics()) > 0 {
		for _, d := range p.Diagnostics() {
			t.Log(d.Format(input))
		}
		t.Fatalf("expected 0 diagnostics, got %d", len(p.Diagnostics()))
	}

	if len(prog.Decls) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(prog.Decls))
	}

	st, ok := prog.Decls[0].(*ast.StructDecl)
	if !ok {
		t.Fatalf("expected *ast.StructDecl, got %T", prog.Decls[0])
	}
	if !st.IsPacked {
		t.Errorf("expected IsPacked=true for packed struct")
	}
	if st.Name != "IDTEntry" {
		t.Errorf("expected struct name IDTEntry, got %s", st.Name)
	}
	if len(st.Fields) != 7 {
		t.Errorf("expected 7 fields, got %d", len(st.Fields))
	}

	fn, ok := prog.Decls[1].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("expected *ast.FuncDecl, got %T", prog.Decls[1])
	}
	if fn.Name != "cpu_disable_interrupts" {
		t.Errorf("expected func name cpu_disable_interrupts, got %s", fn.Name)
	}
	unsafeStmt, ok := fn.Body.Stmts[0].(*ast.UnsafeBlockStmt)
	if !ok {
		t.Fatalf("expected *ast.UnsafeBlockStmt, got %T", fn.Body.Stmts[0])
	}
	exprStmt, ok := unsafeStmt.Body.Stmts[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected *ast.ExprStmt, got %T", unsafeStmt.Body.Stmts[0])
	}
	asmExpr, ok := exprStmt.Expression.(*ast.AsmExpr)
	if !ok {
		t.Fatalf("expected *ast.AsmExpr, got %T", exprStmt.Expression)
	}
	if asmExpr.Instruction != "cli" {
		t.Errorf("expected asm instruction 'cli', got %q", asmExpr.Instruction)
	}
}

