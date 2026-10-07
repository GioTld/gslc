package llvm

import (
	"os"
	"strings"
	"testing"

	"github.com/GioTld/gslc/pkg/ir"
	"github.com/GioTld/gslc/pkg/lexer"
	"github.com/GioTld/gslc/pkg/parser"
	"github.com/GioTld/gslc/pkg/sema"
)

func generateLLVM(input string) string {
	l := lexer.New("test.gsl", input)
	p := parser.New(l)
	prog := p.ParseProgram()
	sem := sema.New()
	sem.AnalyzeProgram(prog)
	b := ir.NewBuilder(sem)
	mod := b.Build(prog)
	gen := NewGenerator(mod)
	return gen.Generate()
}

func TestGenerateLLVMSimple(t *testing.T) {
	input := `
func add(a: u32, b: u32) u32 {
    return a + b
}
`
	ll := generateLLVM(input)

	if !strings.Contains(ll, "target triple = \"x86_64-unknown-linux-gnu\"") {
		t.Errorf("expected target triple, got:\n%s", ll)
	}
	if !strings.Contains(ll, "define i32 @add(i32 %t1, i32 %t2)") {
		t.Errorf("expected function definition, got:\n%s", ll)
	}
	if !strings.Contains(ll, "add i32") {
		t.Errorf("expected add instruction, got:\n%s", ll)
	}
	if !strings.Contains(ll, "ret i32") {
		t.Errorf("expected ret instruction, got:\n%s", ll)
	}
}

func TestGenerateLLVMVolatile(t *testing.T) {
	input := `
func write_mmio(addr: usize, val: u8) {
    unsafe {
        let ptr = addr as *u8
        volatile_write(ptr, val)
    }
}
`
	ll := generateLLVM(input)

	if !strings.Contains(ll, "store volatile") {
		t.Errorf("expected store volatile in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "inttoptr") {
		t.Errorf("expected inttoptr cast in LLVM IR, got:\n%s", ll)
	}
}

func TestGenerateLLVMControlFlow(t *testing.T) {
	input := `
func max(a: u32, b: u32) u32 {
    if a > b {
        return a
    } else {
        return b
    }
}
`
	ll := generateLLVM(input)

	if !strings.Contains(ll, "icmp ugt i32") {
		t.Errorf("expected icmp ugt for u32 operands in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "br i1") {
		t.Errorf("expected conditional branch in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "then_") || !strings.Contains(ll, "else_") {
		t.Errorf("expected then/else basic blocks, got:\n%s", ll)
	}
}

func TestGenerateLLVMSampleFile(t *testing.T) {
	content, err := os.ReadFile("../../../test/fixtures/sample.gsl")
	if err != nil {
		t.Fatalf("failed to read sample fixture: %v", err)
	}

	ll := generateLLVM(string(content))

	if !strings.Contains(ll, "define void @uart_write_byte") {
		t.Errorf("expected define void @uart_write_byte, got:\n%s", ll)
	}
	if !strings.Contains(ll, "define { i64, [4 x i8] } @process_packet") {
		t.Errorf("expected tagged Result return from process_packet, got:\n%s", ll)
	}
	if !strings.Contains(ll, "define void @process_event") {
		t.Errorf("expected define void @process_event, got:\n%s", ll)
	}
}

func TestGenerateLLVMScopedArena(t *testing.T) {
	input := `
struct Packet {
    id: u32,
    size: u32
}

func handle_packet() u32 {
    arena temp(4096)
    let p = temp.alloc(Packet)
    return 0
}
`
	ll := generateLLVM(input)

	if !strings.Contains(ll, "%struct.Arena = type { ptr, i64, i64 }") {
		t.Errorf("expected Arena struct definition in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "alloca [4096 x i8]") {
		t.Errorf("expected buffer alloca for 4096 bytes, got:\n%s", ll)
	}
	if !strings.Contains(ll, "alloca %struct.Arena") {
		t.Errorf("expected Arena struct alloca, got:\n%s", ll)
	}
	if !strings.Contains(ll, "getelementptr inbounds i8, ptr") {
		t.Errorf("expected buffer allocation GEP, got:\n%s", ll)
	}
	if !strings.Contains(ll, "store i64 0, ptr") {
		t.Errorf("expected arena reset before return, got:\n%s", ll)
	}
}

func TestGenerateLLVMAtomics(t *testing.T) {
	input := `
func test_atomics() {
    unsafe {
        let addr: usize = 0x1000
        let ptr = addr as *u64
        atomic_store(ptr, 1 as u64)
        let v = atomic_load(ptr)
        let ok = atomic_cas(ptr, 1 as u64, 2 as u64)
    }
}
`
	ll := generateLLVM(input)

	if !strings.Contains(ll, "store atomic i64") {
		t.Errorf("expected 'store atomic i64' in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "load atomic i64") {
		t.Errorf("expected 'load atomic i64' in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "cmpxchg ptr") {
		t.Errorf("expected 'cmpxchg ptr' in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "seq_cst") {
		t.Errorf("expected 'seq_cst' ordering in LLVM IR, got:\n%s", ll)
	}
}

func TestGenerateLLVMHelloFixture(t *testing.T) {
	content, err := os.ReadFile("../../../test/fixtures/hello.gsl")
	if err != nil {
		t.Fatalf("failed to read hello fixture: %v", err)
	}

	ll := generateLLVM(string(content))

	if !strings.Contains(ll, "@main") {
		t.Errorf("expected @main function definition, got:\n%s", ll)
	}
	if !strings.Contains(ll, "@.str") {
		t.Errorf("expected string constant in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "syscall") {
		t.Errorf("expected syscall inline asm in LLVM IR, got:\n%s", ll)
	}
}

func TestGenerateLLVMKernelPrimitivesAndPackedStruct(t *testing.T) {
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

func kernel_cpu_ops() {
    unsafe {
        asm("cli");
        outb(0x3F8 as u16, 0x41 as u8);
        let b = inb(0x3F8 as u16);
        outw(0x3F8 as u16, 0x1234 as u16);
        let w = inw(0x3F8 as u16);
        cli();
        sti();
        hlt();
    }
}
`
	ll := generateLLVM(input)

	// Check packed struct LLVM syntax: %struct.IDTEntry = type <{ ... }>
	if !strings.Contains(ll, "%struct.IDTEntry = type <{ i16, i16, i8, i8, i16, i32, i32 }>") {
		t.Errorf("expected packed struct definition with <{ ... }>, got:\n%s", ll)
	}

	// Check inline assembly instructions
	if !strings.Contains(ll, `call void asm sideeffect "cli"`) {
		t.Errorf("expected cli asm in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, `call void asm sideeffect "sti"`) {
		t.Errorf("expected sti asm in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, `call void asm sideeffect "hlt"`) {
		t.Errorf("expected hlt asm in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, `call void asm sideeffect "outb %al, %dx"`) {
		t.Errorf("expected outb asm in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, `call i8 asm sideeffect "inb %dx, %al"`) {
		t.Errorf("expected inb asm in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, `call void asm sideeffect "outw %ax, %dx"`) {
		t.Errorf("expected outw asm in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, `call i16 asm sideeffect "inw %dx, %ax"`) {
		t.Errorf("expected inw asm in LLVM IR, got:\n%s", ll)
	}
}

func TestGenerateLLVMTargetTriple(t *testing.T) {
	input := `func f() {}`
	l := lexer.New("test.gsl", input)
	p := parser.New(l)
	prog := p.ParseProgram()
	sem := sema.New()
	sem.AnalyzeProgram(prog)
	b := ir.NewBuilder(sem)
	mod := b.Build(prog)
	gen := NewGenerator(mod)
	gen.TargetTriple = "x86_64-unknown-none-elf"
	ll := gen.Generate()

	if !strings.Contains(ll, `target triple = "x86_64-unknown-none-elf"`) {
		t.Errorf("expected target triple in LLVM IR, got:\n%s", ll)
	}
}
