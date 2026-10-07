package comptime

import (
	"fmt"
	"strconv"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/types"
)

type ValueKind int

const (
	ValInt ValueKind = iota
	ValBool
	ValString
	ValType
)

type Value interface {
	Kind() ValueKind
	Type() types.Type
	String() string
}

type IntValue struct {
	Val     int64
	ValType types.Type
}

func (v *IntValue) Kind() ValueKind     { return ValInt }
func (v *IntValue) Type() types.Type    { return v.ValType }
func (v *IntValue) String() string      { return strconv.FormatInt(v.Val, 10) }

type BoolValue struct {
	Val bool
}

func (v *BoolValue) Kind() ValueKind     { return ValBool }
func (v *BoolValue) Type() types.Type    { return types.Bool }
func (v *BoolValue) String() string      { return strconv.FormatBool(v.Val) }

type StringValue struct {
	Val string
}

func (v *StringValue) Kind() ValueKind     { return ValString }
func (v *StringValue) Type() types.Type    { return types.String }
func (v *StringValue) String() string      { return fmt.Sprintf("%q", v.Val) }

type TypeValue struct {
	TargetType types.Type
}

func (v *TypeValue) Kind() ValueKind     { return ValType }
func (v *TypeValue) Type() types.Type    { return types.Void }
func (v *TypeValue) String() string      { return v.TargetType.String() }

type Environment struct {
	parent *Environment
	values map[string]Value
	types  map[string]types.Type
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		parent: parent,
		values: make(map[string]Value),
		types:  make(map[string]types.Type),
	}
}

func (e *Environment) Define(name string, val Value) {
	e.values[name] = val
}

func (e *Environment) DefineType(name string, t types.Type) {
	e.types[name] = t
}

func (e *Environment) Lookup(name string) (Value, bool) {
	if v, ok := e.values[name]; ok {
		return v, true
	}
	if e.parent != nil {
		return e.parent.Lookup(name)
	}
	return nil, false
}

func (e *Environment) LookupType(name string) (types.Type, bool) {
	if t, ok := e.types[name]; ok {
		return t, true
	}
	if p := types.LookupPrimitive(name); p != nil {
		return p, true
	}
	if e.parent != nil {
		return e.parent.LookupType(name)
	}
	return nil, false
}

type Evaluator struct {
	Env *Environment
}

func NewEvaluator(env *Environment) *Evaluator {
	if env == nil {
		env = NewEnvironment(nil)
	}
	return &Evaluator{Env: env}
}

func (e *Evaluator) Eval(expr ast.Expr) (Value, error) {
	if expr == nil {
		return nil, fmt.Errorf("nil expression")
	}

	switch node := expr.(type) {
	case *ast.IntLitExpr:
		v, err := strconv.ParseInt(node.Value, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer literal %q: %w", node.Value, err)
		}
		return &IntValue{Val: v, ValType: types.UntypedInt}, nil

	case *ast.BoolLitExpr:
		return &BoolValue{Val: node.Value}, nil

	case *ast.StringLitExpr:
		unquoted := node.Value
		if s, err := strconv.Unquote(node.Value); err == nil {
			unquoted = s
		}
		return &StringValue{Val: unquoted}, nil

	case *ast.IdentExpr:
		if val, ok := e.Env.Lookup(node.Name); ok {
			return val, nil
		}
		if t, ok := e.Env.LookupType(node.Name); ok {
			return &TypeValue{TargetType: t}, nil
		}
		return nil, fmt.Errorf("undefined comptime identifier %q", node.Name)

	case *ast.ComptimeExpr:
		return e.Eval(node.Expr)

	case *ast.UnaryExpr:
		operand, err := e.Eval(node.Right)
		if err != nil {
			return nil, err
		}
		return evalUnary(node.Op, operand)

	case *ast.BinaryExpr:
		left, err := e.Eval(node.Left)
		if err != nil {
			return nil, err
		}
		right, err := e.Eval(node.Right)
		if err != nil {
			return nil, err
		}
		return evalBinary(node.Op, left, right)

	case *ast.BlockExpr:
		childEnv := NewEnvironment(e.Env)
		childEval := &Evaluator{Env: childEnv}
		var lastVal Value = &IntValue{Val: 0, ValType: types.Void}
		for _, stmt := range node.Stmts {
			switch s := stmt.(type) {
			case *ast.ConstDecl:
				val, err := childEval.Eval(s.Value)
				if err != nil {
					return nil, err
				}
				childEnv.Define(s.Name, val)
				lastVal = val
			case *ast.LetStmt:
				val, err := childEval.Eval(s.Value)
				if err != nil {
					return nil, err
				}
				childEnv.Define(s.Name, val)
				lastVal = val
			case *ast.ExprStmt:
				val, err := childEval.Eval(s.Expression)
				if err != nil {
					return nil, err
				}
				lastVal = val
			}
		}
		if node.Result != nil {
			return childEval.Eval(node.Result)
		}
		return lastVal, nil

	case *ast.CallExpr:
		return e.evalCall(node)

	case *ast.CastExpr:
		val, err := e.Eval(node.Target)
		if err != nil {
			return nil, err
		}
		if iv, ok := val.(*IntValue); ok {
			return &IntValue{Val: iv.Val, ValType: types.Usize}, nil
		}
		return val, nil

	default:
		return nil, fmt.Errorf("unsupported comptime expression type: %T", expr)
	}
}

func (e *Evaluator) evalCall(c *ast.CallExpr) (Value, error) {
	fnName := ""
	if id, ok := c.Callee.(*ast.IdentExpr); ok {
		fnName = id.Name
	}

	switch fnName {
	case "size_of":
		if len(c.Args) != 1 {
			return nil, fmt.Errorf("size_of expects exactly 1 argument, got %d", len(c.Args))
		}
		t, err := e.resolveTypeArg(c.Args[0])
		if err != nil {
			return nil, err
		}
		return &IntValue{Val: int64(t.Size()), ValType: types.Usize}, nil

	case "align_of":
		if len(c.Args) != 1 {
			return nil, fmt.Errorf("align_of expects exactly 1 argument, got %d", len(c.Args))
		}
		t, err := e.resolveTypeArg(c.Args[0])
		if err != nil {
			return nil, err
		}
		return &IntValue{Val: int64(t.Align()), ValType: types.Usize}, nil

	default:
		return nil, fmt.Errorf("unsupported comptime call %q", fnName)
	}
}

func (e *Evaluator) resolveTypeArg(expr ast.Expr) (types.Type, error) {
	if id, ok := expr.(*ast.IdentExpr); ok {
		if t, ok := e.Env.LookupType(id.Name); ok {
			return t, nil
		}
	}
	val, err := e.Eval(expr)
	if err != nil {
		return nil, err
	}
	if tv, ok := val.(*TypeValue); ok {
		return tv.TargetType, nil
	}
	return nil, fmt.Errorf("expected type in comptime query, got %v", val)
}

func evalUnary(op string, operand Value) (Value, error) {
	switch op {
	case "-":
		if iv, ok := operand.(*IntValue); ok {
			return &IntValue{Val: -iv.Val, ValType: iv.ValType}, nil
		}
		return nil, fmt.Errorf("unary '-' operator requires integer operand")
	case "!":
		if bv, ok := operand.(*BoolValue); ok {
			return &BoolValue{Val: !bv.Val}, nil
		}
		return nil, fmt.Errorf("unary '!' operator requires bool operand")
	case "~":
		if iv, ok := operand.(*IntValue); ok {
			return &IntValue{Val: ^iv.Val, ValType: iv.ValType}, nil
		}
		return nil, fmt.Errorf("unary '~' operator requires integer operand")
	default:
		return nil, fmt.Errorf("unsupported unary operator %q", op)
	}
}

func evalBinary(op string, left, right Value) (Value, error) {
	li, isLeftInt := left.(*IntValue)
	ri, isRightInt := right.(*IntValue)

	if isLeftInt && isRightInt {
		switch op {
		case "+":
			return &IntValue{Val: li.Val + ri.Val, ValType: li.ValType}, nil
		case "-":
			return &IntValue{Val: li.Val - ri.Val, ValType: li.ValType}, nil
		case "*":
			return &IntValue{Val: li.Val * ri.Val, ValType: li.ValType}, nil
		case "/":
			if ri.Val == 0 {
				return nil, fmt.Errorf("compile-time division by zero")
			}
			return &IntValue{Val: li.Val / ri.Val, ValType: li.ValType}, nil
		case "%":
			if ri.Val == 0 {
				return nil, fmt.Errorf("compile-time modulo by zero")
			}
			return &IntValue{Val: li.Val % ri.Val, ValType: li.ValType}, nil
		case "&":
			return &IntValue{Val: li.Val & ri.Val, ValType: li.ValType}, nil
		case "|":
			return &IntValue{Val: li.Val | ri.Val, ValType: li.ValType}, nil
		case "^":
			return &IntValue{Val: li.Val ^ ri.Val, ValType: li.ValType}, nil
		case "<<":
			if ri.Val < 0 || ri.Val >= 64 {
				return nil, fmt.Errorf("compile-time invalid shift count %d", ri.Val)
			}
			return &IntValue{Val: li.Val << ri.Val, ValType: li.ValType}, nil
		case ">>":
			if ri.Val < 0 || ri.Val >= 64 {
				return nil, fmt.Errorf("compile-time invalid shift count %d", ri.Val)
			}
			return &IntValue{Val: li.Val >> ri.Val, ValType: li.ValType}, nil
		case "==":
			return &BoolValue{Val: li.Val == ri.Val}, nil
		case "!=":
			return &BoolValue{Val: li.Val != ri.Val}, nil
		case "<":
			return &BoolValue{Val: li.Val < ri.Val}, nil
		case "<=":
			return &BoolValue{Val: li.Val <= ri.Val}, nil
		case ">":
			return &BoolValue{Val: li.Val > ri.Val}, nil
		case ">=":
			return &BoolValue{Val: li.Val >= ri.Val}, nil
		}
	}

	lb, isLeftBool := left.(*BoolValue)
	rb, isRightBool := right.(*BoolValue)
	if isLeftBool && isRightBool {
		switch op {
		case "&&":
			return &BoolValue{Val: lb.Val && rb.Val}, nil
		case "||":
			return &BoolValue{Val: lb.Val || rb.Val}, nil
		case "==":
			return &BoolValue{Val: lb.Val == rb.Val}, nil
		case "!=":
			return &BoolValue{Val: lb.Val != rb.Val}, nil
		}
	}

	return nil, fmt.Errorf("mismatched or unsupported binary operand types: %s %s %s", left, op, right)
}
