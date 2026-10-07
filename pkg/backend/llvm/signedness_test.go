package llvm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegerSignednessExecution(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("clang is required for integer signedness tests in CI: %v", err)
		}
		t.Skip("clang is required for integer signedness tests")
	}

	tests := []struct {
		name   string
		source string
		opcode string
	}{
		{"unsigned division", "func main() u32 { let high: u32 = 0x80000000; if high / 2 == 0x40000000 { return 0 }; return 1 }", "udiv i32"},
		{"unsigned remainder", "func main() u32 { let high: u32 = 0x80000000; if high % 3 == 2 { return 0 }; return 1 }", "urem i32"},
		{"unsigned right shift", "func main() u32 { let high: u32 = 0x80000000; if high >> 1 == 0x40000000 { return 0 }; return 1 }", "lshr i32"},
		{"unsigned less than", "func main() u32 { let high: u32 = 0x80000000; if 1 < high { return 0 }; return 1 }", "icmp ult i32"},
		{"unsigned less equal", "func main() u32 { let high: u32 = 0x80000000; if 1 <= high { return 0 }; return 1 }", "icmp ule i32"},
		{"unsigned greater than", "func main() u32 { let high: u32 = 0x80000000; if high > 1 { return 0 }; return 1 }", "icmp ugt i32"},
		{"unsigned greater equal", "func main() u32 { let high: u32 = 0x80000000; if high >= 1 { return 0 }; return 1 }", "icmp uge i32"},
		{"signed comparison", "func main() u32 { let low: i32 = 0x80000000 as i32; if low < 0 as i32 { return 0 }; return 1 }", "icmp slt i32"},
		{"unsigned compound division", "func main() u32 { var high: u32 = 0x80000000; high /= 2; if high == 0x40000000 { return 0 }; return 1 }", "udiv i32"},
		{"unsigned compound remainder", "func main() u32 { var high: u32 = 0x80000000; high %= 3; if high == 2 { return 0 }; return 1 }", "urem i32"},
		{"unsigned compound shift", "func main() u32 { var high: u32 = 0x80000000; high >>= 1; if high == 0x40000000 { return 0 }; return 1 }", "lshr i32"},
		{"unsigned widening cast", "func main() u32 { let high: u32 = 0x80000000; let wide: u64 = high as u64; if wide == 0x80000000 as u64 { return 0 }; return 1 }", "zext i32"},
		{"signed widening cast", "func main() u32 { let low: i32 = 0x80000000 as i32; let wide: i64 = low as i64; if wide < 0 as i64 { return 0 }; return 1 }", "sext i32"},
		{"unsigned 64-bit division", "func main() u32 { let high: u64 = (1 as u64) << (63 as u64); if high / (2 as u64) == (1 as u64) << (62 as u64) { return 0 }; return 1 }", "udiv i64"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ll := generateLLVM(test.source)
			if !strings.Contains(ll, test.opcode) {
				t.Fatalf("missing %q in generated LLVM IR:\n%s", test.opcode, ll)
			}
			dir := t.TempDir()
			irPath := filepath.Join(dir, "program.ll")
			binPath := filepath.Join(dir, "program")
			if err := os.WriteFile(irPath, []byte(ll), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command("clang", irPath, "-o", binPath).CombinedOutput(); err != nil {
				t.Fatalf("clang failed: %v: %s\n%s", err, output, ll)
			}
			if output, err := exec.Command(binPath).CombinedOutput(); err != nil {
				var exit *exec.ExitError
				if errors.As(err, &exit) {
					t.Fatalf("program exited %d: %s", exit.ExitCode(), output)
				}
				t.Fatalf("program failed: %v: %s", err, output)
			}
		})
	}
}
