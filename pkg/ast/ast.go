package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GioTld/gslc/pkg/diag"
)

// Node represents any node in the Abstract Syntax Tree.
type Node interface {
	Span() diag.Span
	String() string
}

// Expr represents an AST node that evaluates to a value.
type Expr interface {
	Node
	exprNode()
}

// Stmt represents a statement AST node.
type Stmt interface {
	Node
	stmtNode()
}

// Decl represents a top-level declaration AST node.
type Decl interface {
	Node
	declNode()
}

// Type represents a type annotation in GSL.
type Type interface {
	Node
	typeNode()
}

// --- Types ---

type NamedType struct {
	Name     string
	TypeSpan diag.Span
}

func (t *NamedType) Span() diag.Span { return t.TypeSpan }
func (t *NamedType) String() string  { return t.Name }
func (t *NamedType) typeNode()       {}

type GenericType struct {
	Base     string
	TypeArgs []Type
	TypeSpan diag.Span
}

func (t *GenericType) Span() diag.Span { return t.TypeSpan }
func (t *GenericType) String() string {
	args := make([]string, len(t.TypeArgs))
	for i, a := range t.TypeArgs {
		args[i] = a.String()
	}
	return fmt.Sprintf("%s[%s]", t.Base, strings.Join(args, ", "))
}
func (t *GenericType) typeNode() {}

type PointerType struct {
	IsMut    bool
	ElemType Type
	TypeSpan diag.Span
}

func (t *PointerType) Span() diag.Span { return t.TypeSpan }
func (t *PointerType) String() string {
	if t.IsMut {
		return "*" + t.ElemType.String()
	}
	return "*const " + t.ElemType.String()
}
func (t *PointerType) typeNode() {}

type SliceType struct {
	ElemType Type
	TypeSpan diag.Span
}

func (t *SliceType) Span() diag.Span { return t.TypeSpan }
func (t *SliceType) String() string  { return "[]" + t.ElemType.String() }
func (t *SliceType) typeNode()       {}

// --- Expressions ---

type IdentExpr struct {
	Name     string
	NodeSpan diag.Span
}

func (e *IdentExpr) Span() diag.Span { return e.NodeSpan }
func (e *IdentExpr) String() string  { return e.Name }
func (e *IdentExpr) exprNode()       {}

type IntLitExpr struct {
	Value    string
	NodeSpan diag.Span
}

func (e *IntLitExpr) Span() diag.Span { return e.NodeSpan }
func (e *IntLitExpr) String() string  { return e.Value }
func (e *IntLitExpr) exprNode()       {}

func (e *IntLitExpr) Uint64() (uint64, error) {
	base, start := 10, 0
	if len(e.Value) >= 2 && e.Value[0] == '0' {
		switch e.Value[1] {
		case 'x', 'X':
			base, start = 16, 2
		case 'b', 'B':
			base, start = 2, 2
		case 'o', 'O':
			base, start = 8, 2
		}
	}
	for i := start; i < len(e.Value); i++ {
		if e.Value[i] == '_' && (i == start || i+1 == len(e.Value) || e.Value[i-1] == '_' || e.Value[i+1] == '_') {
			return 0, fmt.Errorf("invalid integer separator in %q", e.Value)
		}
	}
	return strconv.ParseUint(strings.ReplaceAll(e.Value[start:], "_", ""), base, 64)
}

type FloatLitExpr struct {
	Value    string
	NodeSpan diag.Span
}

func (e *FloatLitExpr) Span() diag.Span { return e.NodeSpan }
func (e *FloatLitExpr) String() string  { return e.Value }
func (e *FloatLitExpr) exprNode()       {}

type StringLitExpr struct {
	Value    string
	NodeSpan diag.Span
}

func (e *StringLitExpr) Span() diag.Span { return e.NodeSpan }
func (e *StringLitExpr) String() string  { return e.Value }
func (e *StringLitExpr) exprNode()       {}

type BoolLitExpr struct {
	Value    bool
	NodeSpan diag.Span
}

func (e *BoolLitExpr) Span() diag.Span { return e.NodeSpan }
func (e *BoolLitExpr) String() string  { return fmt.Sprintf("%t", e.Value) }
func (e *BoolLitExpr) exprNode()       {}

type UnaryExpr struct {
	Op       string
	Right    Expr
	NodeSpan diag.Span
}

func (e *UnaryExpr) Span() diag.Span { return e.NodeSpan }
func (e *UnaryExpr) String() string  { return fmt.Sprintf("(%s%s)", e.Op, e.Right) }
func (e *UnaryExpr) exprNode()       {}

type BinaryExpr struct {
	Left     Expr
	Op       string
	Right    Expr
	NodeSpan diag.Span
}

func (e *BinaryExpr) Span() diag.Span { return e.NodeSpan }
func (e *BinaryExpr) String() string  { return fmt.Sprintf("(%s %s %s)", e.Left, e.Op, e.Right) }
func (e *BinaryExpr) exprNode()       {}

type CallExpr struct {
	Callee   Expr
	Args     []Expr
	NodeSpan diag.Span
}

func (e *CallExpr) Span() diag.Span { return e.NodeSpan }
func (e *CallExpr) String() string {
	args := make([]string, len(e.Args))
	for i, a := range e.Args {
		args[i] = a.String()
	}
	return fmt.Sprintf("%s(%s)", e.Callee, strings.Join(args, ", "))
}
func (e *CallExpr) exprNode() {}

type MemberExpr struct {
	Target   Expr
	Field    string
	NodeSpan diag.Span
}

func (e *MemberExpr) Span() diag.Span { return e.NodeSpan }
func (e *MemberExpr) String() string  { return fmt.Sprintf("%s.%s", e.Target, e.Field) }
func (e *MemberExpr) exprNode()       {}

type IndexExpr struct {
	Target   Expr
	Index    Expr
	NodeSpan diag.Span
}

func (e *IndexExpr) Span() diag.Span { return e.NodeSpan }
func (e *IndexExpr) String() string  { return fmt.Sprintf("%s[%s]", e.Target, e.Index) }
func (e *IndexExpr) exprNode()       {}

type CastExpr struct {
	Target   Expr
	ToType   Type
	NodeSpan diag.Span
}

func (e *CastExpr) Span() diag.Span { return e.NodeSpan }
func (e *CastExpr) String() string  { return fmt.Sprintf("(%s as %s)", e.Target, e.ToType) }
func (e *CastExpr) exprNode()       {}

type QuestionExpr struct {
	Target   Expr
	NodeSpan diag.Span
}

type MatchArm struct {
	Pattern Expr
	Value   Expr
}

type MatchExpr struct {
	Target   Expr
	Arms     []MatchArm
	NodeSpan diag.Span
}

func (e *MatchExpr) Span() diag.Span { return e.NodeSpan }
func (e *MatchExpr) exprNode()       {}
func (e *MatchExpr) String() string {
	var arms []string
	for _, arm := range e.Arms {
		arms = append(arms, arm.Pattern.String()+" => "+arm.Value.String())
	}
	return "match " + e.Target.String() + " { " + strings.Join(arms, ", ") + " }"
}

func (e *QuestionExpr) Span() diag.Span { return e.NodeSpan }
func (e *QuestionExpr) String() string  { return e.Target.String() + "?" }
func (e *QuestionExpr) exprNode()       {}

type BlockExpr struct {
	Stmts    []Stmt
	Result   Expr // optional trailing expression
	NodeSpan diag.Span
}

func (e *BlockExpr) Span() diag.Span { return e.NodeSpan }
func (e *BlockExpr) String() string {
	var sb strings.Builder
	sb.WriteString("{\n")
	for _, s := range e.Stmts {
		sb.WriteString("  " + s.String() + "\n")
	}
	if e.Result != nil {
		sb.WriteString("  " + e.Result.String() + "\n")
	}
	sb.WriteString("}")
	return sb.String()
}
func (e *BlockExpr) exprNode() {}
func (e *BlockExpr) stmtNode() {}

type ComptimeExpr struct {
	Expr     Expr
	NodeSpan diag.Span
}

func (e *ComptimeExpr) Span() diag.Span { return e.NodeSpan }
func (e *ComptimeExpr) String() string  { return fmt.Sprintf("comptime %s", e.Expr) }
func (e *ComptimeExpr) exprNode()       {}

type AsmExpr struct {
	Instruction string
	NodeSpan    diag.Span
}

func (e *AsmExpr) Span() diag.Span { return e.NodeSpan }
func (e *AsmExpr) String() string  { return fmt.Sprintf("asm(%q)", e.Instruction) }
func (e *AsmExpr) exprNode()       {}

// --- Statements ---

type ExprStmt struct {
	Expression Expr
	NodeSpan   diag.Span
}

func (s *ExprStmt) Span() diag.Span { return s.NodeSpan }
func (s *ExprStmt) String() string  { return s.Expression.String() }
func (s *ExprStmt) stmtNode()       {}

type LetStmt struct {
	Name     string
	Type     Type // optional
	Value    Expr
	NodeSpan diag.Span
}

func (s *LetStmt) Span() diag.Span { return s.NodeSpan }
func (s *LetStmt) String() string {
	if s.Type != nil {
		return fmt.Sprintf("let %s: %s = %s", s.Name, s.Type, s.Value)
	}
	return fmt.Sprintf("let %s = %s", s.Name, s.Value)
}
func (s *LetStmt) stmtNode() {}

type VarStmt struct {
	Name     string
	Type     Type // optional
	Value    Expr
	NodeSpan diag.Span
}

func (s *VarStmt) Span() diag.Span { return s.NodeSpan }
func (s *VarStmt) String() string {
	if s.Type != nil {
		return fmt.Sprintf("var %s: %s = %s", s.Name, s.Type, s.Value)
	}
	return fmt.Sprintf("var %s = %s", s.Name, s.Value)
}
func (s *VarStmt) stmtNode() {}

type AssignStmt struct {
	Target   Expr
	Op       string // "=", "+=", "-=", etc.
	Value    Expr
	NodeSpan diag.Span
}

func (s *AssignStmt) Span() diag.Span { return s.NodeSpan }
func (s *AssignStmt) String() string  { return fmt.Sprintf("%s %s %s", s.Target, s.Op, s.Value) }
func (s *AssignStmt) stmtNode()       {}

type ReturnStmt struct {
	Value    Expr // optional
	NodeSpan diag.Span
}

func (s *ReturnStmt) Span() diag.Span { return s.NodeSpan }
func (s *ReturnStmt) String() string {
	if s.Value != nil {
		return "return " + s.Value.String()
	}
	return "return"
}
func (s *ReturnStmt) stmtNode() {}

type BreakStmt struct {
	NodeSpan diag.Span
}

func (s *BreakStmt) Span() diag.Span { return s.NodeSpan }
func (s *BreakStmt) String() string  { return "break" }
func (s *BreakStmt) stmtNode()       {}

type ContinueStmt struct {
	NodeSpan diag.Span
}

func (s *ContinueStmt) Span() diag.Span { return s.NodeSpan }
func (s *ContinueStmt) String() string  { return "continue" }
func (s *ContinueStmt) stmtNode()       {}

type IfStmt struct {
	Condition  Expr
	ThenBlock  *BlockExpr
	ElseBranch Stmt // *BlockExpr or *IfStmt
	NodeSpan   diag.Span
}

func (s *IfStmt) Span() diag.Span { return s.NodeSpan }
func (s *IfStmt) String() string {
	if s.ElseBranch != nil {
		return fmt.Sprintf("if %s %s else %s", s.Condition, s.ThenBlock, s.ElseBranch)
	}
	return fmt.Sprintf("if %s %s", s.Condition, s.ThenBlock)
}
func (s *IfStmt) stmtNode() {}

type ForStmt struct {
	Item       string
	Collection Expr
	Body       *BlockExpr
	NodeSpan   diag.Span
}

func (s *ForStmt) Span() diag.Span { return s.NodeSpan }
func (s *ForStmt) String() string {
	return fmt.Sprintf("for %s in %s %s", s.Item, s.Collection, s.Body)
}
func (s *ForStmt) stmtNode() {}

type WhileStmt struct {
	Condition Expr
	Body      *BlockExpr
	NodeSpan  diag.Span
}

func (s *WhileStmt) Span() diag.Span { return s.NodeSpan }
func (s *WhileStmt) String() string  { return fmt.Sprintf("while %s %s", s.Condition, s.Body) }
func (s *WhileStmt) stmtNode()       {}

type UnsafeBlockStmt struct {
	Body     *BlockExpr
	NodeSpan diag.Span
}

func (s *UnsafeBlockStmt) Span() diag.Span { return s.NodeSpan }
func (s *UnsafeBlockStmt) String() string  { return "unsafe " + s.Body.String() }
func (s *UnsafeBlockStmt) stmtNode()       {}

type CaseClause struct {
	Pattern  Expr
	Body     []Stmt
	NodeSpan diag.Span
}

type SwitchStmt struct {
	Expr       Expr
	Cases      []CaseClause
	Default    []Stmt
	HasDefault bool
	NodeSpan   diag.Span
}

func (s *SwitchStmt) Span() diag.Span { return s.NodeSpan }
func (s *SwitchStmt) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("switch %s {\n", s.Expr))
	for _, c := range s.Cases {
		sb.WriteString(fmt.Sprintf("case %s:\n", c.Pattern))
		for _, st := range c.Body {
			sb.WriteString("  " + st.String() + "\n")
		}
	}
	if s.HasDefault {
		sb.WriteString("default:\n")
		for _, st := range s.Default {
			sb.WriteString("  " + st.String() + "\n")
		}
	}
	sb.WriteString("}")
	return sb.String()
}
func (s *SwitchStmt) stmtNode() {}

// --- Declarations ---

type Param struct {
	Name string
	Type Type
}

type FuncDecl struct {
	Name       string
	Params     []Param
	ReturnType Type // optional
	Body       *BlockExpr
	NodeSpan   diag.Span
}

func (d *FuncDecl) Span() diag.Span { return d.NodeSpan }
func (d *FuncDecl) String() string {
	params := make([]string, len(d.Params))
	for i, p := range d.Params {
		params[i] = fmt.Sprintf("%s: %s", p.Name, p.Type)
	}
	ret := ""
	if d.ReturnType != nil {
		ret = " " + d.ReturnType.String()
	}
	return fmt.Sprintf("func %s(%s)%s %s", d.Name, strings.Join(params, ", "), ret, d.Body)
}
func (d *FuncDecl) declNode() {}

type Field struct {
	Name string
	Type Type
}

type StructDecl struct {
	Name     string
	Fields   []Field
	IsPacked bool
	IsCopy   bool
	NodeSpan diag.Span
}

func (d *StructDecl) Span() diag.Span { return d.NodeSpan }
func (d *StructDecl) String() string {
	fields := make([]string, len(d.Fields))
	for i, f := range d.Fields {
		fields[i] = fmt.Sprintf("  %s: %s", f.Name, f.Type)
	}
	prefix := "struct"
	if d.IsPacked {
		prefix = "packed struct"
	}
	if d.IsCopy {
		prefix = "copy " + prefix
	}
	return fmt.Sprintf("%s %s {\n%s\n}", prefix, d.Name, strings.Join(fields, ",\n"))
}
func (d *StructDecl) declNode() {}

type ArenaDecl struct {
	Name     string
	SizeExpr Expr
	NodeSpan diag.Span
}

func (d *ArenaDecl) Span() diag.Span { return d.NodeSpan }
func (d *ArenaDecl) String() string  { return fmt.Sprintf("arena %s(%s)", d.Name, d.SizeExpr) }
func (d *ArenaDecl) declNode()       {}
func (d *ArenaDecl) stmtNode()       {}

type ConstDecl struct {
	Name     string
	Type     Type // optional
	Value    Expr
	NodeSpan diag.Span
}

func (d *ConstDecl) Span() diag.Span { return d.NodeSpan }
func (d *ConstDecl) String() string {
	if d.Type != nil {
		return fmt.Sprintf("const %s: %s = %s", d.Name, d.Type, d.Value)
	}
	return fmt.Sprintf("const %s = %s", d.Name, d.Value)
}
func (d *ConstDecl) declNode() {}
func (d *ConstDecl) stmtNode() {}

type ImportDecl struct {
	Path     string
	NodeSpan diag.Span
}

func (d *ImportDecl) Span() diag.Span { return d.NodeSpan }
func (d *ImportDecl) String() string  { return fmt.Sprintf("import %q", d.Path) }
func (d *ImportDecl) declNode()       {}

// Program is the root AST node representing a compiled GSL file.
type Program struct {
	Decls    []Decl
	NodeSpan diag.Span
}

func (p *Program) Span() diag.Span { return p.NodeSpan }
func (p *Program) String() string {
	var sb strings.Builder
	for _, d := range p.Decls {
		sb.WriteString(d.String() + "\n\n")
	}
	return sb.String()
}
