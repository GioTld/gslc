package llvm

import (
	"strings"
	"testing"
)

func TestGenerateLLVMComptimeAndConstants(t *testing.T) {
	input := `
const BUFFER_CAP = comptime 1024 * 8

func get_capacity() usize {
    return BUFFER_CAP
}
`
	ll := generateLLVM(input)
	if !strings.Contains(ll, "8192") {
		t.Errorf("expected folded constant 8192 in LLVM IR, got:\n%s", ll)
	}
}

func TestGenerateLLVMYield(t *testing.T) {
	input := `
func spin() {
    yield()
}
`
	ll := generateLLVM(input)
	if !strings.Contains(ll, "pause") {
		t.Errorf("expected pause asm in LLVM IR, got:\n%s", ll)
	}
}

func TestGenerateLLVMChannel(t *testing.T) {
	input := `
func test_channel() {
    let ch = Channel.new(16)
    let ok = ch.send(99 as u32)
    let res = ch.recv()
}
`
	ll := generateLLVM(input)
	if !strings.Contains(ll, "%struct.Channel") {
		t.Errorf("expected %%struct.Channel in LLVM IR, got:\n%s", ll)
	}
	if !strings.Contains(ll, "load atomic") || !strings.Contains(ll, "store atomic") {
		t.Errorf("expected atomic load/store for channel operations in LLVM IR, got:\n%s", ll)
	}
}
