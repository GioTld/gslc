package llvm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestPartialIntegerAndArenaOperationsTrap(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("trap signal assertions require Linux x86_64")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("clang is required for partial-operation tests in CI: %v", err)
		}
		t.Skip("clang is required for partial-operation tests")
	}

	tests := []struct {
		name   string
		source string
	}{
		{"unsigned division by zero", "func main() u32 { let divisor: u32 = 0; return 4 / divisor }"},
		{"unsigned remainder by zero", "func main() u32 { let divisor: u32 = 0; return 4 % divisor }"},
		{"signed division overflow", "func main() u32 { let low: i32 = 0x80000000 as i32; let minus_one: i32 = (0 as i32) - (1 as i32); return (low / minus_one) as u32 }"},
		{"signed remainder overflow", "func main() u32 { let low: i32 = 0x80000000 as i32; let minus_one: i32 = (0 as i32) - (1 as i32); return (low % minus_one) as u32 }"},
		{"left shift outside width", "func main() u32 { let count: u32 = 32; return 1 << count }"},
		{"right shift outside width", "func main() u32 { let count: u32 = 32; return 1 >> count }"},
		{"arena exhaustion", "func main() u32 { arena a(8); let ptr = a.alloc(16 as usize); return 0 }"},
		{"dynamic arena exhaustion", "func main() u32 { arena a(16); var count: usize = 12; let first = a.alloc(count); let second = a.alloc(8 as usize); return 0 }"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ll := generateLLVM(test.source)
			if !strings.Contains(ll, "call void @llvm.trap()") {
				t.Fatalf("missing guarded trap in LLVM IR:\n%s", ll)
			}
			binary := compilePartialOperationFixture(t, ll)
			output, err := exec.Command(binary).CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("expected trap, got %v: %s\n%s", err, output, ll)
			}
			status, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGILL {
				t.Fatalf("expected SIGILL from llvm.trap, got %v: %s\n%s", exit, output, ll)
			}
		})
	}
}

func TestArenaDynamicAllocationWithinCapacity(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang is required for arena allocation execution")
	}
	ll := generateLLVM("func main() u32 { arena a(16); var count: usize = 8; let first = a.alloc(count); let second = a.alloc(count); return 0 }")
	binary := compilePartialOperationFixture(t, ll)
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("valid arena allocation failed: %v: %s", err, output)
	}
}

func TestIntegerOverflowWraps(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang is required for integer overflow execution")
	}
	tests := []struct {
		name   string
		source string
	}{
		{"unsigned addition", "func main() u32 { let high: u8 = 255 as u8; if high + (1 as u8) == 0 as u8 { return 0 }; return 1 }"},
		{"signed addition", "func main() u32 { let high: i8 = 127 as i8; if high + (1 as i8) == 0x80 as i8 { return 0 }; return 1 }"},
		{"unsigned subtraction", "func main() u32 { let low: u8 = 0 as u8; if low - (1 as u8) == 255 as u8 { return 0 }; return 1 }"},
		{"unsigned multiplication", "func main() u32 { let high: u8 = 128 as u8; if high * (2 as u8) == 0 as u8 { return 0 }; return 1 }"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binary := compilePartialOperationFixture(t, generateLLVM(test.source))
			if output, err := exec.Command(binary).CombinedOutput(); err != nil {
				t.Fatalf("wrapping arithmetic failed: %v: %s", err, output)
			}
		})
	}
}

func compilePartialOperationFixture(t *testing.T, ll string) string {
	t.Helper()
	dir := t.TempDir()
	irPath := filepath.Join(dir, "program.ll")
	binary := filepath.Join(dir, "program")
	if err := os.WriteFile(irPath, []byte(ll), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("clang", "-O2", irPath, "-o", binary).CombinedOutput(); err != nil {
		t.Fatalf("clang failed: %v: %s\n%s", err, output, ll)
	}
	return binary
}
