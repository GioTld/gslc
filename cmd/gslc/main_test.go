package main

import (
	"strings"
	"testing"
)

func TestParseArgsWithOutput(t *testing.T) {
	src, out := parseArgsWithOutput([]string{"-o", "bin/foo", "src/main.gsl"})
	if src != "src/main.gsl" {
		t.Errorf("expected src 'src/main.gsl', got %q", src)
	}
	if out != "bin/foo" {
		t.Errorf("expected out 'bin/foo', got %q", out)
	}

	src2, out2 := parseArgsWithOutput([]string{"test.gsl"})
	if src2 != "test.gsl" {
		t.Errorf("expected src 'test.gsl', got %q", src2)
	}
	if out2 != "" {
		t.Errorf("expected empty out, got %q", out2)
	}
}

func TestParseBuildArgsFreestanding(t *testing.T) {
	cfg := parseBuildArgs([]string{"-freestanding", "-o", "bin/kernel", "src/main.gsl"})
	if cfg.src != "src/main.gsl" {
		t.Errorf("expected src 'src/main.gsl', got %q", cfg.src)
	}
	if cfg.out != "bin/kernel" {
		t.Errorf("expected out 'bin/kernel', got %q", cfg.out)
	}
	if !cfg.freestanding {
		t.Errorf("expected freestanding=true, got false")
	}
}

func TestParseBuildArgsTargetAndLinkerScript(t *testing.T) {
	cfg := parseBuildArgs([]string{
		"-freestanding",
		"-T", "kernel/linker.ld",
		"--target=x86_64-unknown-none-elf",
		"-o", "bin/kernel.elf",
		"src/main.gsl",
	})
	if cfg.src != "src/main.gsl" {
		t.Errorf("expected src 'src/main.gsl', got %q", cfg.src)
	}
	if cfg.out != "bin/kernel.elf" {
		t.Errorf("expected out 'bin/kernel.elf', got %q", cfg.out)
	}
	if !cfg.freestanding {
		t.Errorf("expected freestanding=true")
	}
	if cfg.linkerScript != "kernel/linker.ld" {
		t.Errorf("expected linkerScript 'kernel/linker.ld', got %q", cfg.linkerScript)
	}
	if cfg.target != "x86_64-unknown-none-elf" {
		t.Errorf("expected target 'x86_64-unknown-none-elf', got %q", cfg.target)
	}
}

func TestCompileToLLVME2E(t *testing.T) {
	prog, mod, ll := compileToLLVM("../../test/fixtures/sample.gsl")
	if prog == nil || len(prog.Decls) == 0 {
		t.Fatal("expected parsed declarations")
	}
	if mod == nil || len(mod.Functions) == 0 {
		t.Fatal("expected lowered IR module with functions")
	}
	if !strings.Contains(ll, "target triple = \"x86_64-unknown-linux-gnu\"") {
		t.Errorf("expected target triple in LLVM output, got:\n%s", ll)
	}
	if !strings.Contains(ll, "define void @uart_write_byte") {
		t.Errorf("expected define void @uart_write_byte in LLVM output")
	}
}

func TestCompileToLLVMCoreImports(t *testing.T) {
	prog, mod, ll := compileToLLVM("../../test/fixtures/core_test.gsl")
	if prog == nil || len(prog.Decls) == 0 {
		t.Fatal("expected parsed declarations with imports resolved")
	}
	if mod == nil || len(mod.Functions) == 0 {
		t.Fatal("expected lowered IR module with imported functions")
	}
	expectedFuncs := []string{"@memcpy", "@memset", "@spinlock_acquire", "@spinlock_release", "@print_raw", "@main"}
	for _, fn := range expectedFuncs {
		if !strings.Contains(ll, fn) {
			t.Errorf("expected %s in generated LLVM IR", fn)
		}
	}
}

func TestCompileToLLVMStdImports(t *testing.T) {
	prog, mod, ll := compileToLLVM("../../test/fixtures/std_test.gsl")
	if prog == nil || len(prog.Decls) == 0 {
		t.Fatal("expected parsed declarations with std imports resolved")
	}
	if mod == nil || len(mod.Functions) == 0 {
		t.Fatal("expected lowered IR module with std functions")
	}
	expectedFuncs := []string{"@open", "@write_string", "@close", "@println", "@main"}
	for _, fn := range expectedFuncs {
		if !strings.Contains(ll, fn) {
			t.Errorf("expected %s in generated LLVM IR", fn)
		}
	}
}
