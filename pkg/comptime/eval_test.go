package comptime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/lexer"
	"github.com/GioTld/gslc/pkg/parser"
	"github.com/GioTld/gslc/pkg/types"
)

func evalExpr(input string, env *Environment) (Value, error) {
	full := "const RESULT = " + input + ";"
	l := lexer.New("test.gsl", full)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Diagnostics()) > 0 {
		return nil, fmt.Errorf("%s", p.Diagnostics()[0].Message)
	}
	cd, ok := prog.Decls[0].(*ast.ConstDecl)
	if !ok {
		t := prog.Decls[0]
		_ = t
		panic("expected ConstDecl")
	}
	eval := NewEvaluator(env)
	return eval.Eval(cd.Value)
}

func TestEvalArithmetic(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"10 + 20", 30},
		{"100 - 45", 55},
		{"8 * 1024", 8192},
		{"1024 / 4", 256},
		{"15 % 4", 3},
		{"1 << 10", 1024},
		{"1024 >> 2", 256},
		{"0xFF & 0x0F", 0x0F},
		{"0xF0 | 0x0F", 0xFF},
		{"0xAA ^ 0x55", 0xFF},
		{"-42", -42},
		{"~0", -1},
		{"(10 + 2) * 5", 60},
	}

	for _, tt := range tests {
		val, err := evalExpr(tt.input, nil)
		if err != nil {
			t.Fatalf("eval error on %q: %v", tt.input, err)
		}
		iv, ok := val.(*IntValue)
		if !ok || iv.Val != tt.expected {
			t.Errorf("input %q: expected %d, got %v", tt.input, tt.expected, val)
		}
	}
}

func TestEvalDivisionByZero(t *testing.T) {
	_, err := evalExpr("100 / 0", nil)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division by zero error, got: %v", err)
	}

	_, err = evalExpr("100 % 0", nil)
	if err == nil || !strings.Contains(err.Error(), "modulo by zero") {
		t.Fatalf("expected modulo by zero error, got: %v", err)
	}
}

func TestEvalBooleansAndLogic(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true && false", false},
		{"true || false", true},
		{"!false", true},
		{"10 == 10", true},
		{"10 != 20", true},
		{"5 < 10", true},
		{"15 <= 15", true},
		{"20 > 5", true},
		{"10 >= 10", true},
	}

	for _, tt := range tests {
		val, err := evalExpr(tt.input, nil)
		if err != nil {
			t.Fatalf("eval error on %q: %v", tt.input, err)
		}
		bv, ok := val.(*BoolValue)
		if !ok || bv.Val != tt.expected {
			t.Errorf("input %q: expected %t, got %v", tt.input, tt.expected, val)
		}
	}
}

func TestEvalSizeOfAndAlignOf(t *testing.T) {
	env := NewEnvironment(nil)
	structFields := []types.StructField{
		{Name: "id", Type: types.U32},
		{Name: "data", Type: types.U64},
	}
	st := types.NewStructType("Packet", structFields)
	env.DefineType("Packet", st)

	val, err := evalExpr("size_of(u32)", env)
	if err != nil || val.(*IntValue).Val != 4 {
		t.Errorf("expected size_of(u32)=4, got %v, err: %v", val, err)
	}

	val, err = evalExpr("size_of(u64)", env)
	if err != nil || val.(*IntValue).Val != 8 {
		t.Errorf("expected size_of(u64)=8, got %v, err: %v", val, err)
	}

	val, err = evalExpr("size_of(Packet)", env)
	if err != nil || val.(*IntValue).Val != 16 {
		t.Errorf("expected size_of(Packet)=16, got %v, err: %v", val, err)
	}

	val, err = evalExpr("align_of(Packet)", env)
	if err != nil || val.(*IntValue).Val != 8 {
		t.Errorf("expected align_of(Packet)=8, got %v, err: %v", val, err)
	}
}

func TestEvalComptimeBlock(t *testing.T) {
	input := "comptime { const A = 512; const B = 2; A * B }"
	val, err := evalExpr(input, nil)
	if err != nil {
		t.Fatalf("comptime block eval error: %v", err)
	}
	iv, ok := val.(*IntValue)
	if !ok || iv.Val != 1024 {
		t.Fatalf("expected 1024, got %v", val)
	}
}
