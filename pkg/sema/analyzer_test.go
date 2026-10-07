package sema

import (
	"strings"
	"testing"

	"github.com/GioTld/gslc/pkg/lexer"
	"github.com/GioTld/gslc/pkg/parser"
	"github.com/GioTld/gslc/pkg/types"
)

func parseAndAnalyze(input string) *Analyzer {
	l := lexer.New("test.gsl", input)
	p := parser.New(l)
	prog := p.ParseProgram()
	sem := New()
	sem.AnalyzeProgram(prog)
	return sem
}

func TestDescriptorPrimitivesRequireUnsafeAndValidSymbols(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{"unsafe", `func main() { lgdt(0 as usize) }`, "requires an 'unsafe' block"},
		{"literal", `func main() { unsafe { symbol_address("kernel_gdt; hlt") } }`, "invalid symbol name"},
		{"pointer", `func main() { unsafe { lidt(1 as u32) } }`, "requires a usize descriptor pointer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sem := parseAndAnalyze(test.source)
			for _, diagnostic := range sem.Diagnostics() {
				if strings.Contains(diagnostic.Message, test.message) {
					return
				}
			}
			t.Fatalf("expected diagnostic %q, got %v", test.message, sem.Diagnostics())
		})
	}
}

func TestSemaSampleFile(t *testing.T) {
	input := `
arena memory_pool(64 * 1024)

struct UartPort {
    base_address: usize
}

func uart_write_byte(port: UartPort, byte: u8) {
    unsafe {
        let reg = port.base_address as *u8
        volatile_write(reg, byte)
    }
}

func process_packet(raw_data: []u8) Result[u32, u32] {
    var counter: u32 = 0
    for b in raw_data {
        if b == 0 {
            break
        }
        counter += 1
    }
    return Result.Ok(counter)
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		for _, d := range sem.Diagnostics() {
			t.Log(d.Format(input))
		}
		t.Fatalf("expected 0 diagnostics, got %d", len(sem.Diagnostics()))
	}
}

func TestSemaImmutabilityEnforcement(t *testing.T) {
	input := `
func test_mut() {
    let x: u32 = 10
    x = 20
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) == 0 {
		t.Fatalf("expected error reassigning immutable variable 'let x', got 0")
	}

	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "cannot reassign immutable binding") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected immutable binding diagnostic, got: %v", sem.Diagnostics())
	}
}

func TestSemaZeroImplicitCasting(t *testing.T) {
	input := `
func test_cast() {
    var a: u32 = 10
    var b: u64 = 20
    var c = a + b
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) == 0 {
		t.Fatalf("expected error for implicit conversion between u32 and u64, got 0")
	}

	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "mismatched types in binary expression") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected mismatched types error, got: %v", sem.Diagnostics())
	}
}

func TestSemaUnsafeEnforcement(t *testing.T) {
	input := `
func test_unsafe(addr: usize) {
    let reg = addr as *u8
    volatile_write(reg, 0)
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) < 2 {
		t.Fatalf("expected errors for pointer cast and volatile call outside unsafe, got %d", len(sem.Diagnostics()))
	}

	hasCastErr := false
	hasVolatileErr := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "raw pointer cast requires an 'unsafe' block") {
			hasCastErr = true
		}
		if strings.Contains(d.Message, "volatile MMIO operations require an 'unsafe' block") || strings.Contains(d.Message, "call to hardware primitive \"volatile_write\" requires an 'unsafe' block") {
			hasVolatileErr = true
		}
	}

	if !hasCastErr || !hasVolatileErr {
		t.Fatalf("expected both raw pointer and volatile unsafe errors, got: %v", sem.Diagnostics())
	}
}

func TestSemaOptionAndResult(t *testing.T) {
	input := `
func get_status(ok: bool) Result[u32, u32] {
    if ok {
        return Result.Ok(200)
    }
    return Result.Err(500)
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics for valid Result return, got: %v", sem.Diagnostics())
	}
}

func TestSemaKernelPrimitivesUnsafeEnforcement(t *testing.T) {
	inputSafe := `
func safe_code() {
    asm("cli");
    outb(0x3F8 as u16, 0x41 as u8);
    let val = inb(0x3F8 as u16);
    hlt();
}
`
	semSafe := parseAndAnalyze(inputSafe)
	if len(semSafe.Diagnostics()) < 4 {
		t.Fatalf("expected at least 4 unsafe diagnostics, got %d: %v", len(semSafe.Diagnostics()), semSafe.Diagnostics())
	}

	inputUnsafe := `
packed struct GDTR {
    limit: u16,
    base: u64,
}

func kernel_entry() {
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
	semUnsafe := parseAndAnalyze(inputUnsafe)
	if len(semUnsafe.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics in unsafe block, got %d: %v", len(semUnsafe.Diagnostics()), semUnsafe.Diagnostics())
	}

	// Verify GDTR is packed in global scope
	sym := semUnsafe.globalScope.Lookup("GDTR")
	if sym == nil {
		t.Fatal("expected GDTR in global scope")
	}
	st, ok := sym.Type.(*types.StructType)
	if !ok {
		t.Fatalf("expected *types.StructType, got %T", sym.Type)
	}
	if !st.IsPacked {
		t.Errorf("expected GDTR.IsPacked == true")
	}
	if st.Size() != 10 { // 2 + 8, no padding
		t.Errorf("expected GDTR size 10, got %d", st.Size())
	}
	if st.Align() != 1 {
		t.Errorf("expected GDTR align 1, got %d", st.Align())
	}
}

func TestStructFieldResolution(t *testing.T) {
	forward := parseAndAnalyze(`struct A { child: B } struct B { value: u32 }`)
	if len(forward.Diagnostics()) != 0 {
		t.Fatalf("forward struct reference failed: %v", forward.Diagnostics())
	}
	if st := forward.StructType("A"); st == nil || st.Size() != 4 {
		t.Fatalf("forward struct layout is incorrect: %v", st)
	}

	for _, test := range []struct {
		source string
		want   string
	}{
		{`struct Missing { value: Unknown }`, `unknown type "Unknown"`},
		{`struct Cycle { value: Cycle }`, "contains itself by value"},
	} {
		analyzer := parseAndAnalyze(test.source)
		found := false
		for _, diagnostic := range analyzer.Diagnostics() {
			found = found || strings.Contains(diagnostic.Message, test.want)
		}
		if !found {
			t.Errorf("expected %q for %q, got %v", test.want, test.source, analyzer.Diagnostics())
		}
	}
}
