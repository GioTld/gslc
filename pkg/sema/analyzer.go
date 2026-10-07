package sema

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/diag"
	"github.com/GioTld/gslc/pkg/types"
)

// Analyzer performs type checking and semantic validation on the AST.
type Analyzer struct {
	globalScope    *Scope
	curScope       *Scope
	curFuncScope   *Scope
	curFunc        *ast.FuncDecl
	curFuncType    *types.FuncType
	lastAllocArena string
	loopDepth      int
	exprTypes      map[ast.Expr]types.Type
	structDecls    map[string]*ast.StructDecl
	structState    map[string]uint8
	diagnostics    []diag.Diagnostic
}

// New creates and initializes a semantic Analyzer.
func New() *Analyzer {
	global := NewScope(nil)
	initBuiltins(global)

	return &Analyzer{
		globalScope: global,
		curScope:    global,
		exprTypes:   make(map[ast.Expr]types.Type),
		structDecls: make(map[string]*ast.StructDecl),
		structState: make(map[string]uint8),
	}
}

func initBuiltins(scope *Scope) {
	// Register primitives in global scope
	prims := []types.Type{
		types.Void, types.Bool,
		types.U8, types.U16, types.U32, types.U64, types.Usize,
		types.I8, types.I16, types.I32, types.I64, types.Isize,
		types.F32, types.F64, types.String,
	}
	for _, p := range prims {
		_ = scope.Define(&Symbol{
			Name: p.String(),
			Kind: SymbolType,
			Type: p,
		})
	}

	// Builtin Result and Option namespace types
	_ = scope.Define(&Symbol{Name: "Result", Kind: SymbolType, Type: types.Void})
	_ = scope.Define(&Symbol{Name: "Option", Kind: SymbolType, Type: types.Void})

	// Builtin volatile primitives
	_ = scope.Define(&Symbol{
		Name: "volatile_write",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{&types.PointerType{Elem: types.U8, IsMut: true}, types.U8},
			ReturnType: types.Void,
		},
	})
	_ = scope.Define(&Symbol{
		Name: "volatile_read",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{&types.PointerType{Elem: types.U8, IsMut: false}},
			ReturnType: types.U8,
		},
	})

	// Builtin atomic primitives (require unsafe block)
	_ = scope.Define(&Symbol{
		Name: "atomic_load",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{&types.PointerType{Elem: types.U64, IsMut: false}},
			ReturnType: types.U64,
		},
	})
	_ = scope.Define(&Symbol{
		Name: "atomic_store",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{&types.PointerType{Elem: types.U64, IsMut: true}, types.U64},
			ReturnType: types.Void,
		},
	})
	_ = scope.Define(&Symbol{
		Name: "atomic_cas",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{&types.PointerType{Elem: types.U64, IsMut: true}, types.U64, types.U64},
			ReturnType: types.Bool,
		},
	})

	// Builtin syscall_write: fd, buf addr, len — freestanding write, args as raw integers
	_ = scope.Define(&Symbol{
		Name: "syscall_write",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{types.U64, types.U64, types.U64},
			ReturnType: types.I64,
		},
	})

	// Builtin syscall: num, a1, a2, a3 — direct raw Linux syscall
	_ = scope.Define(&Symbol{
		Name: "syscall",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{types.U64, types.U64, types.U64, types.U64},
			ReturnType: types.I64,
		},
	})

	// Builtin log namespace symbol
	_ = scope.Define(&Symbol{
		Name: "log",
		Kind: SymbolVar,
		Type: types.Void,
	})

	// Builtin Channel type
	_ = scope.Define(&Symbol{Name: "Channel", Kind: SymbolType, Type: types.Void})

	// Builtin cooperative yield
	_ = scope.Define(&Symbol{
		Name: "yield",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     nil,
			ReturnType: types.Void,
		},
	})

	// Builtin comptime intrinsics
	_ = scope.Define(&Symbol{
		Name: "size_of",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{types.Void},
			ReturnType: types.Usize,
		},
	})
	_ = scope.Define(&Symbol{
		Name: "align_of",
		Kind: SymbolFunc,
		Type: &types.FuncType{
			Params:     []types.Type{types.Void},
			ReturnType: types.Usize,
		},
	})

	// CPU port I/O primitives (require unsafe block)
	_ = scope.Define(&Symbol{
		Name: "inb",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: []types.Type{types.U16}, ReturnType: types.U8},
	})
	_ = scope.Define(&Symbol{
		Name: "outb",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: []types.Type{types.U16, types.U8}, ReturnType: types.Void},
	})
	_ = scope.Define(&Symbol{
		Name: "inw",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: []types.Type{types.U16}, ReturnType: types.U16},
	})
	_ = scope.Define(&Symbol{
		Name: "outw",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: []types.Type{types.U16, types.U16}, ReturnType: types.Void},
	})
	_ = scope.Define(&Symbol{
		Name: "hlt",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: nil, ReturnType: types.Void},
	})
	_ = scope.Define(&Symbol{
		Name: "cli",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: nil, ReturnType: types.Void},
	})
	_ = scope.Define(&Symbol{
		Name: "sti",
		Kind: SymbolFunc,
		Type: &types.FuncType{Params: nil, ReturnType: types.Void},
	})
	for _, name := range []string{"lgdt", "lidt"} {
		_ = scope.Define(&Symbol{Name: name, Kind: SymbolFunc,
			Type: &types.FuncType{Params: []types.Type{types.Usize}, ReturnType: types.Void}})
	}
	_ = scope.Define(&Symbol{Name: "symbol_address", Kind: SymbolFunc,
		Type: &types.FuncType{Params: []types.Type{types.String}, ReturnType: types.Usize}})
}

// Diagnostics returns all semantic diagnostics encountered.
func (a *Analyzer) Diagnostics() []diag.Diagnostic {
	return a.diagnostics
}

// ExprType returns the checked type of an expression.
func (a *Analyzer) ExprType(e ast.Expr) types.Type {
	return a.exprTypes[e]
}

func (a *Analyzer) StructType(name string) *types.StructType {
	symbol := a.globalScope.Lookup(name)
	if symbol == nil {
		return nil
	}
	st, _ := symbol.Type.(*types.StructType)
	return st
}

func (a *Analyzer) error(span diag.Span, msg string) {
	a.diagnostics = append(a.diagnostics, diag.NewError(span, msg))
}

func (a *Analyzer) errorCode(span diag.Span, code, msg string) {
	a.diagnostics = append(a.diagnostics, diag.NewError(span, msg).WithCode(code))
}

func (a *Analyzer) errorWithHint(span diag.Span, msg, hint string) {
	d := diag.NewError(span, msg).WithHint(hint)
	a.diagnostics = append(a.diagnostics, d)
}

// AnalyzeProgram runs full semantic checks on a GSL Program.
func (a *Analyzer) AnalyzeProgram(prog *ast.Program) {
	// Pass 1: Declare top-level structs, arenas, and constants
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *ast.StructDecl:
			a.structDecls[d.Name] = d
			a.declareStruct(d)
		case *ast.ArenaDecl:
			a.declareArena(d)
		case *ast.ConstDecl:
			a.declareConst(d)
		}
	}

	// Pass 2: Declare function signatures
	for _, decl := range prog.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			a.declareFunc(fn)
		}
	}

	// Pass 3: Check struct bodies (resolve field types)
	for _, decl := range prog.Decls {
		if st, ok := decl.(*ast.StructDecl); ok {
			a.resolveStructFields(st)
		}
	}

	// Pass 4: Check function bodies
	for _, decl := range prog.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			a.checkFuncBody(fn)
		}
	}
}

func (a *Analyzer) declareStruct(d *ast.StructDecl) {
	sym := &Symbol{
		Name: d.Name,
		Kind: SymbolType,
		Type: &types.StructType{Name: d.Name, IsPacked: d.IsPacked, IsCopy: d.IsCopy},
		Span: d.Span(),
	}
	if err := a.globalScope.Define(sym); err != nil {
		a.error(d.Span(), err.Error())
	}
}

func (a *Analyzer) declareArena(d *ast.ArenaDecl) {
	scope := a.curScope
	if scope == nil {
		scope = a.globalScope
	}
	sym := &Symbol{
		Name: d.Name,
		Kind: SymbolArena,
		Type: &types.ArenaType{Name: d.Name},
		Span: d.Span(),
	}
	if err := scope.Define(sym); err != nil {
		a.error(d.Span(), err.Error())
	}
}

func (a *Analyzer) declareConst(d *ast.ConstDecl) {
	scope := a.curScope
	if scope == nil {
		scope = a.globalScope
	}
	valType := a.checkExpr(d.Value)
	if d.Type != nil {
		annotated := a.resolveType(d.Type)
		if valType == types.UntypedInt && types.IsInteger(annotated) {
			valType = annotated
		}
	}
	sym := &Symbol{
		Name:  d.Name,
		Kind:  SymbolConst,
		Type:  valType,
		IsMut: false,
		Span:  d.Span(),
	}
	if err := scope.Define(sym); err != nil {
		a.error(d.Span(), err.Error())
	}
}

func (a *Analyzer) declareFunc(d *ast.FuncDecl) {
	paramTypes := make([]types.Type, len(d.Params))
	for i, p := range d.Params {
		paramTypes[i] = a.resolveType(p.Type)
	}

	var retType types.Type = types.Void
	if d.ReturnType != nil {
		retType = a.resolveType(d.ReturnType)
	}

	fnType := &types.FuncType{
		Params:     paramTypes,
		ReturnType: retType,
	}

	sym := &Symbol{
		Name: d.Name,
		Kind: SymbolFunc,
		Type: fnType,
		Span: d.Span(),
	}

	if err := a.globalScope.Define(sym); err != nil {
		a.error(d.Span(), err.Error())
	}
}

func (a *Analyzer) resolveStructFields(d *ast.StructDecl) {
	if a.structState[d.Name] == 2 {
		return
	}
	if a.structState[d.Name] == 1 {
		a.error(d.Span(), fmt.Sprintf("struct %s contains itself by value", d.Name))
		return
	}
	a.structState[d.Name] = 1
	sym := a.globalScope.Lookup(d.Name)
	if sym == nil {
		return
	}
	st, ok := sym.Type.(*types.StructType)
	if !ok {
		return
	}

	fields := make([]types.StructField, len(d.Fields))
	for i, f := range d.Fields {
		a.resolveStructDependencies(f.Type)
		fType := a.resolveType(f.Type)
		if fType == types.Void {
			a.error(f.Type.Span(), "struct field cannot have void type")
			fType = types.U8
		} else if fType.Align() == 0 {
			a.error(f.Type.Span(), fmt.Sprintf("struct field %s has an incomplete type", f.Name))
			fType = types.U8
		}
		fields[i] = types.StructField{
			Name: f.Name,
			Type: fType,
		}
	}
	if d.IsPacked {
		*st = *types.NewPackedStructType(d.Name, fields)
	} else {
		*st = *types.NewStructType(d.Name, fields)
	}
	st.IsCopy = d.IsCopy
	if d.IsCopy {
		for _, field := range fields {
			if !isCopyType(field.Type) {
				a.error(d.Span(), fmt.Sprintf("copy struct %s contains non-copy field %s", d.Name, field.Name))
			}
		}
	}
	a.structState[d.Name] = 2
}

func (a *Analyzer) resolveStructDependencies(t ast.Type) {
	switch ty := t.(type) {
	case *ast.NamedType:
		if decl := a.structDecls[ty.Name]; decl != nil {
			a.resolveStructFields(decl)
		}
	case *ast.GenericType:
		for _, arg := range ty.TypeArgs {
			a.resolveStructDependencies(arg)
		}
	}
}

func (a *Analyzer) checkFuncBody(d *ast.FuncDecl) {
	sym := a.globalScope.Lookup(d.Name)
	if sym == nil {
		return
	}
	fnType, ok := sym.Type.(*types.FuncType)
	if !ok {
		return
	}

	a.curFunc = d
	a.curFuncType = fnType
	a.curScope = NewScope(a.globalScope)
	a.curFuncScope = a.curScope

	// Add parameters to scope
	for i, p := range d.Params {
		pSym := &Symbol{
			Name:  p.Name,
			Kind:  SymbolVar,
			Type:  fnType.Params[i],
			IsMut: false,
			Span:  d.Span(),
		}
		if err := a.curScope.Define(pSym); err != nil {
			a.error(d.Span(), err.Error())
		}
	}

	a.checkBlock(d.Body)

	a.curFunc = nil
	a.curFuncType = nil
	a.curFuncScope = nil
	a.curScope = a.globalScope
}

func (a *Analyzer) checkBlock(b *ast.BlockExpr) {
	if b == nil {
		return
	}
	parent := a.curScope
	a.curScope = NewScope(parent)
	defer func() { a.curScope = parent }()

	for _, stmt := range b.Stmts {
		a.checkStmt(stmt)
	}
}

func (a *Analyzer) checkStmt(stmt ast.Stmt) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.LetStmt:
		a.checkLetStmt(s)
	case *ast.VarStmt:
		a.checkVarStmt(s)
	case *ast.AssignStmt:
		a.checkAssignStmt(s)
	case *ast.ReturnStmt:
		a.checkReturnStmt(s)
	case *ast.IfStmt:
		a.checkIfStmt(s)
	case *ast.ForStmt:
		a.checkForStmt(s)
	case *ast.WhileStmt:
		a.checkWhileStmt(s)
	case *ast.SwitchStmt:
		a.checkSwitchStmt(s)
	case *ast.ArenaDecl:
		a.declareArena(s)
	case *ast.ConstDecl:
		a.declareConst(s)
	case *ast.UnsafeBlockStmt:
		a.checkUnsafeBlock(s)
	case *ast.ExprStmt:
		a.checkExpr(s.Expression)
	case *ast.BreakStmt:
		if a.loopDepth == 0 {
			a.error(s.Span(), "break requires a loop")
		}
	case *ast.ContinueStmt:
		if a.loopDepth == 0 {
			a.error(s.Span(), "continue requires a loop")
		}
	default:
		// Unsupported stmt
	}
}

func (a *Analyzer) getArenaScope(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym := a.curScope.Lookup(e.Name); sym != nil {
			return sym.ArenaScope
		}
	case *ast.CallExpr:
		return a.lastAllocArena
	}
	return ""
}

func (a *Analyzer) checkLetStmt(s *ast.LetStmt) {
	a.lastAllocArena = ""
	var valType types.Type = types.Void
	var arenaScope string
	if s.Value != nil {
		valType = a.checkExpr(s.Value)
		arenaScope = a.getArenaScope(s.Value)
		a.consumeValue(s.Value)
	}

	if s.Type != nil {
		annotated := a.resolveType(s.Type)
		if valType == types.UntypedInt && types.IsInteger(annotated) {
			valType = annotated
		} else if s.Value != nil && !annotated.Equals(valType) && valType != types.Void && !a.adaptVariant(s.Value, valType, annotated) {
			a.errorWithHint(s.Span(),
				fmt.Sprintf("cannot assign expression of type %s to immutable variable of type %s", valType, annotated),
				"use explicit conversion 'as' if this cast was intentional")
		}
		valType = annotated
	} else if valType == types.UntypedInt {
		valType = types.U32 // default fallback
	}
	if s.Type == nil && needsVariantAnnotation(valType) {
		a.error(s.Span(), "algebraic constructor requires an explicit Option or Result type annotation")
	}

	sym := &Symbol{
		Name:       s.Name,
		Kind:       SymbolVar,
		Type:       valType,
		IsMut:      false,
		Span:       s.Span(),
		ArenaScope: arenaScope,
	}

	if err := a.curScope.Define(sym); err != nil {
		a.error(s.Span(), err.Error())
	}
}

func (a *Analyzer) checkVarStmt(s *ast.VarStmt) {
	a.lastAllocArena = ""
	var valType types.Type = types.Void
	var arenaScope string
	if s.Value != nil {
		valType = a.checkExpr(s.Value)
		arenaScope = a.getArenaScope(s.Value)
		a.consumeValue(s.Value)
	}

	if s.Type != nil {
		annotated := a.resolveType(s.Type)
		if valType == types.UntypedInt && types.IsInteger(annotated) {
			valType = annotated
		} else if s.Value != nil && !annotated.Equals(valType) && valType != types.Void && !a.adaptVariant(s.Value, valType, annotated) {
			a.errorWithHint(s.Span(),
				fmt.Sprintf("cannot assign expression of type %s to variable of type %s", valType, annotated),
				"use explicit conversion 'as' if this cast was intentional")
		}
		valType = annotated
	} else if valType == types.UntypedInt {
		valType = types.U32 // default fallback
	}
	if s.Type == nil && needsVariantAnnotation(valType) {
		a.error(s.Span(), "algebraic constructor requires an explicit Option or Result type annotation")
	}

	sym := &Symbol{
		Name:       s.Name,
		Kind:       SymbolVar,
		Type:       valType,
		IsMut:      true, // mutable
		Span:       s.Span(),
		ArenaScope: arenaScope,
	}

	if err := a.curScope.Define(sym); err != nil {
		a.error(s.Span(), err.Error())
	}
}

func (a *Analyzer) checkAssignStmt(s *ast.AssignStmt) {
	switch s.Target.(type) {
	case *ast.IdentExpr, *ast.MemberExpr:
	default:
		a.error(s.Target.Span(), "assignment target is not supported by the LLVM backend")
		return
	}

	var targetType types.Type
	if ident, ok := s.Target.(*ast.IdentExpr); ok && s.Op == "=" {
		if symbol := a.curScope.Lookup(ident.Name); symbol != nil {
			targetType = symbol.Type
		} else {
			targetType = a.checkExpr(s.Target)
		}
	} else {
		targetType = a.checkExpr(s.Target)
	}
	a.lastAllocArena = ""
	valType := a.checkExpr(s.Value)
	valArenaScope := a.getArenaScope(s.Value)
	a.consumeValue(s.Value)

	// Adapt untyped int literal
	if valType == types.UntypedInt && types.IsInteger(targetType) {
		valType = targetType
	}

	// Mutability verification
	if ident, ok := s.Target.(*ast.IdentExpr); ok {
		sym := a.curScope.Lookup(ident.Name)
		if sym != nil {
			if !sym.IsMut {
				a.diagnostics = append(a.diagnostics, diag.NewError(ident.Span(),
					fmt.Sprintf("cannot reassign immutable binding %q", ident.Name)).WithCode(diag.ImmutableBinding).
					WithHint("variables declared with 'let' are immutable; use 'var' to declare a mutable variable"))
			}
			if sym.IsMut && s.Op == "=" {
				sym.IsMoved = false
			}

			// Escape analysis for assignments:
			// Cannot assign reference from an inner arena to a variable in an outer scope
			if valArenaScope != "" {
				arenaSym := a.curScope.Lookup(valArenaScope)
				if arenaSym != nil && arenaSym.DefiningScope != nil && sym.DefiningScope != nil {
					if sym.DefiningScope.Depth() < arenaSym.DefiningScope.Depth() {
						a.errorWithHint(s.Span(),
							fmt.Sprintf("cannot assign reference from inner arena %q to outer scope variable %q", valArenaScope, ident.Name),
							"inner arena memory has shorter lifetime than outer variable; reference would dangle")
					}
				}
				// Propagate arena scope to target variable
				sym.ArenaScope = valArenaScope
			}
		}
	}

	// Type compatibility check
	if !targetType.Equals(valType) && targetType != types.Void && valType != types.Void && !a.adaptVariant(s.Value, valType, targetType) {
		a.errorWithHint(s.Span(),
			fmt.Sprintf("mismatched types in assignment: target is %s, value is %s", targetType, valType),
			"GSL does not perform implicit conversions; use explicit 'as'")
	}
}

func (a *Analyzer) checkReturnStmt(s *ast.ReturnStmt) {
	if a.curFuncType == nil {
		a.error(s.Span(), "return statement outside function")
		return
	}

	expected := a.curFuncType.ReturnType
	if s.Value == nil {
		if !expected.Equals(types.Void) {
			a.error(s.Span(), fmt.Sprintf("function expects return type %s, found empty return", expected))
		}
		return
	}

	a.lastAllocArena = ""
	actual := a.checkExpr(s.Value)
	arenaScope := a.getArenaScope(s.Value)
	a.consumeValue(s.Value)

	// Scoped Arena Escape check
	if arenaScope != "" {
		arenaSym := a.curScope.Lookup(arenaScope)
		if arenaSym != nil && arenaSym.DefiningScope != nil && a.curFuncScope != nil {
			if a.curFuncScope == arenaSym.DefiningScope || a.curFuncScope.IsAncestorOf(arenaSym.DefiningScope) {
				refName := "value"
				if ident, ok := s.Value.(*ast.IdentExpr); ok {
					refName = ident.Name
				}
				a.errorWithHint(s.Span(),
					fmt.Sprintf("cannot return reference %q allocated from scoped arena %q", refName, arenaScope),
					"arena memory will be destroyed at function exit; pass the destination arena as a parameter")
			}
		}
	}

	// Adapt untyped int literal for integer return types
	if actual == types.UntypedInt && types.IsInteger(expected) {
		actual = expected
	}

	// Check type match
	if !expected.Equals(actual) && actual != types.Void {
		if a.adaptVariant(s.Value, actual, expected) {
			return
		}
		a.errorWithHint(s.Span(),
			fmt.Sprintf("mismatched return type: expected %s, got %s", expected, actual),
			"ensure all return expressions match the function signature")
	}
}

func (a *Analyzer) checkIfStmt(s *ast.IfStmt) {
	condType := a.checkExpr(s.Condition)
	if !condType.Equals(types.Bool) && condType != types.Void {
		a.error(s.Condition.Span(), fmt.Sprintf("condition must be of type bool, got %s", condType))
	}

	before := snapshotMoved(a.curScope)
	a.checkBlock(s.ThenBlock)
	thenState := snapshotMoved(a.curScope)
	if blockReturns(s.ThenBlock) {
		thenState = before
	}
	restoreMoved(before)
	if s.ElseBranch != nil {
		a.checkStmt(s.ElseBranch)
	}
	elseState := snapshotMoved(a.curScope)
	if s.ElseBranch != nil && statementReturns(s.ElseBranch) {
		elseState = before
	}
	mergeMoved(thenState, elseState)
}

func (a *Analyzer) checkForStmt(s *ast.ForStmt) {
	collType := a.checkExpr(s.Collection)
	a.consumeValue(s.Collection)

	var elemType types.Type = types.Void
	if sl, ok := collType.(*types.SliceType); ok {
		elemType = sl.Elem
	}

	loopScope := NewScope(a.curScope)
	_ = loopScope.Define(&Symbol{
		Name:  s.Item,
		Kind:  SymbolVar,
		Type:  elemType,
		IsMut: false,
		Span:  s.Span(),
	})

	oldScope := a.curScope
	before := snapshotMoved(oldScope)
	a.curScope = loopScope
	a.loopDepth++
	a.checkBlock(s.Body)
	a.loopDepth--
	a.curScope = oldScope
	a.checkLoopMoves(before, s.Span())
}

func (a *Analyzer) checkWhileStmt(s *ast.WhileStmt) {
	condType := a.checkExpr(s.Condition)
	if !condType.Equals(types.Bool) && condType != types.Void {
		a.error(s.Condition.Span(), fmt.Sprintf("while condition must be bool, got %s", condType))
	}
	loopScope := NewScope(a.curScope)
	oldScope := a.curScope
	before := snapshotMoved(oldScope)
	a.curScope = loopScope
	a.loopDepth++
	a.checkBlock(s.Body)
	a.loopDepth--
	a.curScope = oldScope
	a.checkLoopMoves(before, s.Span())
}

func (a *Analyzer) checkUnsafeBlock(s *ast.UnsafeBlockStmt) {
	oldUnsafe := a.curScope.IsUnsafe
	a.curScope.IsUnsafe = true
	a.checkBlock(s.Body)
	a.curScope.IsUnsafe = oldUnsafe
}

func (a *Analyzer) checkSwitchStmt(s *ast.SwitchStmt) {
	targetType := a.checkExpr(s.Expr)
	before := snapshotMoved(a.curScope)
	merged := snapshotMoved(a.curScope)
	seen := make(map[string]bool)
	seenTrue, seenFalse := false, false
	for _, c := range s.Cases {
		restoreMoved(before)
		caseScope := NewScope(a.curScope)
		oldScope := a.curScope
		a.curScope = caseScope

		patternType := a.checkExpr(c.Pattern)
		if patternType == types.UntypedInt && types.IsInteger(targetType) {
			patternType = targetType
		}
		if !targetType.Equals(patternType) && targetType != types.Void && patternType != types.Void {
			a.error(c.Pattern.Span(), fmt.Sprintf("match pattern has type %s, expected %s", patternType, targetType))
		}
		if seen[c.Pattern.String()] {
			a.error(c.Pattern.Span(), "duplicate match pattern")
		}
		seen[c.Pattern.String()] = true
		if literal, ok := c.Pattern.(*ast.BoolLitExpr); ok {
			if literal.Value {
				seenTrue = true
			} else {
				seenFalse = true
			}
		} else if targetType.Equals(types.Bool) {
			a.error(c.Pattern.Span(), "bool match patterns must be true or false")
		}

		for _, st := range c.Body {
			a.checkStmt(st)
		}
		a.curScope = oldScope
		for symbol, moved := range snapshotMoved(a.curScope) {
			merged[symbol] = merged[symbol] || moved
		}
	}

	if s.HasDefault {
		restoreMoved(before)
		defScope := NewScope(a.curScope)
		oldScope := a.curScope
		a.curScope = defScope
		for _, st := range s.Default {
			a.checkStmt(st)
		}
		a.curScope = oldScope
		for symbol, moved := range snapshotMoved(a.curScope) {
			merged[symbol] = merged[symbol] || moved
		}
	} else if targetType.Equals(types.Bool) {
		if !seenTrue || !seenFalse {
			a.error(s.Span(), "non-exhaustive bool match: cover true and false or add default")
		}
	} else {
		a.error(s.Span(), "non-exhaustive match: add a default case")
	}
	restoreMoved(merged)
}

func (a *Analyzer) checkExpr(expr ast.Expr) types.Type {
	if expr == nil {
		return types.Void
	}

	t := a.computeExprType(expr)
	a.exprTypes[expr] = t
	return t
}

func (a *Analyzer) computeExprType(expr ast.Expr) types.Type {
	switch e := expr.(type) {
	case *ast.IntLitExpr:
		if _, err := e.Uint64(); err != nil {
			a.error(e.Span(), fmt.Sprintf("invalid integer literal: %v", err))
		}
		return types.UntypedInt
	case *ast.FloatLitExpr:
		return types.UntypedFloat
	case *ast.StringLitExpr:
		if len(e.Value) > 0 && e.Value[0] == '\'' {
			a.error(e.Span(), "character literals are not supported by the LLVM backend")
			return types.Void
		}
		if _, err := strconv.Unquote(e.Value); err != nil {
			a.error(e.Span(), fmt.Sprintf("invalid string literal: %v", err))
		}
		return types.String
	case *ast.BoolLitExpr:
		return types.Bool
	case *ast.IdentExpr:
		sym := a.curScope.Lookup(e.Name)
		if sym == nil {
			a.errorCode(e.Span(), diag.UndefinedIdentifier, fmt.Sprintf("undefined identifier %q", e.Name))
			return types.Void
		}
		if sym.IsMoved {
			a.error(e.Span(), fmt.Sprintf("use of moved value %q", e.Name))
		}
		return sym.Type
	case *ast.BinaryExpr:
		return a.checkBinaryExpr(e)
	case *ast.UnaryExpr:
		return a.checkUnaryExpr(e)
	case *ast.CallExpr:
		return a.checkCallExpr(e)
	case *ast.MemberExpr:
		return a.checkMemberExpr(e)
	case *ast.IndexExpr:
		return a.checkIndexExpr(e)
	case *ast.CastExpr:
		return a.checkCastExpr(e)
	case *ast.QuestionExpr:
		return a.checkQuestionExpr(e)
	case *ast.MatchExpr:
		return a.checkMatchExpr(e)
	case *ast.ComptimeExpr:
		return a.checkExpr(e.Expr)
	case *ast.AsmExpr:
		if !a.curScope.IsUnsafe {
			a.errorWithHint(e.Span(),
				"inline asm requires an 'unsafe' block",
				"wrap asm(...) in 'unsafe { ... }' to use inline assembly")
		}
		return types.Void
	case *ast.BlockExpr:
		a.checkBlock(e)
		if e.Result != nil {
			return a.checkExpr(e.Result)
		}
		return types.Void
	default:
		return types.Void
	}
}

func (a *Analyzer) checkBinaryExpr(e *ast.BinaryExpr) types.Type {
	left := a.checkExpr(e.Left)
	right := a.checkExpr(e.Right)

	// Adapt untyped int literals in binary operations
	if left == types.UntypedInt && types.IsInteger(right) {
		left = right
	} else if right == types.UntypedInt && types.IsInteger(left) {
		right = left
	} else if left == types.UntypedInt && right == types.UntypedInt {
		left = types.U32
		right = types.U32
	}

	// No implicit casting between typed variables in GSL!
	if !left.Equals(right) && left != types.Void && right != types.Void {
		a.errorWithHint(e.Span(),
			fmt.Sprintf("mismatched types in binary expression %q: left is %s, right is %s", e.Op, left, right),
			"GSL forbids implicit type promotion; cast explicitly with 'as'")
		return types.Void
	}

	// Comparisons evaluate to bool
	switch e.Op {
	case "==", "!=", "<", "<=", ">", ">=":
		return types.Bool
	case "&&", "||":
		if !left.Equals(types.Bool) {
			a.error(e.Span(), fmt.Sprintf("logical operator %q requires bool operands, got %s", e.Op, left))
		}
		return types.Bool
	default:
		return left
	}
}

func (a *Analyzer) checkUnaryExpr(e *ast.UnaryExpr) types.Type {
	operand := a.checkExpr(e.Right)
	switch e.Op {
	case "!":
		if !operand.Equals(types.Bool) {
			a.error(e.Span(), fmt.Sprintf("operator '!' requires bool, got %s", operand))
		}
		return types.Bool
	case "-":
		return operand
	case "&":
		// Address-of: produces a pointer; mutable if the operand is a mutable variable
		isMut := false
		if ident, ok := e.Right.(*ast.IdentExpr); ok {
			if sym := a.curScope.Lookup(ident.Name); sym != nil {
				isMut = sym.IsMut
			}
		}
		return &types.PointerType{Elem: operand, IsMut: isMut}
	case "*":
		// Dereference pointer
		if !a.curScope.IsUnsafe {
			a.error(e.Span(), "raw pointer dereference requires an 'unsafe' block")
		}
		if ptr, ok := operand.(*types.PointerType); ok {
			return ptr.Elem
		}
		a.error(e.Span(), fmt.Sprintf("cannot dereference non-pointer type %s", operand))
		return types.Void
	default:
		return operand
	}
}

func (a *Analyzer) checkCallExpr(e *ast.CallExpr) types.Type {
	calleeType := a.checkExpr(e.Callee)

	// Check if calling volatile or atomic primitives outside unsafe
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		atomicOps := map[string]bool{
			"volatile_read":  true,
			"volatile_write": true,
			"atomic_load":    true,
			"atomic_store":   true,
			"atomic_cas":     true,
			"syscall_write":  true,
			"syscall":        true,
			"inb":            true,
			"outb":           true,
			"inw":            true,
			"outw":           true,
			"hlt":            true,
			"cli":            true,
			"sti":            true,
			"lgdt":           true,
			"lidt":           true,
			"symbol_address": true,
		}
		if atomicOps[ident.Name] && !a.curScope.IsUnsafe {
			a.diagnostics = append(a.diagnostics, diag.NewError(e.Span(),
				fmt.Sprintf("call to hardware primitive %q requires an 'unsafe' block", ident.Name)).WithCode(diag.UnsafeOperation).
				WithHint("wrap this operation in 'unsafe { ... }' to access hardware registers"))
		}

		if ident.Name == "syscall" {
			if len(e.Args) != 4 {
				a.error(e.Span(), fmt.Sprintf("syscall expects 4 arguments (num, a1, a2, a3), got %d", len(e.Args)))
				return types.I64
			}
			for _, arg := range e.Args {
				a.checkExpr(arg)
			}
			return types.I64
		}

		if ident.Name == "syscall_write" {
			if len(e.Args) != 3 {
				a.error(e.Span(), fmt.Sprintf("syscall_write expects 3 arguments (fd, buf, len), got %d", len(e.Args)))
				return types.I64
			}
			for _, arg := range e.Args {
				a.checkExpr(arg)
			}
			return types.I64
		}
		if ident.Name == "size_of" || ident.Name == "align_of" {
			if len(e.Args) != 1 {
				a.error(e.Span(), fmt.Sprintf("%s expects exactly 1 argument", ident.Name))
			}
			return types.Usize
		}

		if ident.Name == "yield" {
			if len(e.Args) != 0 {
				a.error(e.Span(), "yield expects 0 arguments")
			}
			return types.Void
		}

		switch ident.Name {
		case "symbol_address":
			if len(e.Args) != 1 {
				a.error(e.Span(), "symbol_address expects one symbol name")
				return types.Usize
			}
			literal, ok := e.Args[0].(*ast.StringLitExpr)
			if !ok {
				a.error(e.Args[0].Span(), "symbol_address requires a string literal")
				return types.Usize
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
				a.error(e.Args[0].Span(), "invalid symbol name")
			}
			return types.Usize
		case "lgdt", "lidt":
			if len(e.Args) != 1 {
				a.error(e.Span(), fmt.Sprintf("%s expects one descriptor pointer", ident.Name))
				return types.Void
			}
			if argType := a.checkExpr(e.Args[0]); !argType.Equals(types.Usize) {
				a.error(e.Args[0].Span(), fmt.Sprintf("%s requires a usize descriptor pointer", ident.Name))
			}
			return types.Void
		case "inb", "inw":
			if len(e.Args) != 1 {
				a.error(e.Span(), fmt.Sprintf("%s expects 1 argument (port: u16), got %d", ident.Name, len(e.Args)))
				return types.U8
			}
			a.checkExpr(e.Args[0])
			if ident.Name == "inb" {
				return types.U8
			}
			return types.U16
		case "outb":
			if len(e.Args) != 2 {
				a.error(e.Span(), fmt.Sprintf("outb expects 2 arguments (port: u16, val: u8), got %d", len(e.Args)))
				return types.Void
			}
			a.checkExpr(e.Args[0])
			a.checkExpr(e.Args[1])
			return types.Void
		case "outw":
			if len(e.Args) != 2 {
				a.error(e.Span(), fmt.Sprintf("outw expects 2 arguments (port: u16, val: u16), got %d", len(e.Args)))
				return types.Void
			}
			a.checkExpr(e.Args[0])
			a.checkExpr(e.Args[1])
			return types.Void
		case "hlt", "cli", "sti":
			if len(e.Args) != 0 {
				a.error(e.Span(), fmt.Sprintf("%s expects 0 arguments, got %d", ident.Name, len(e.Args)))
			}
			return types.Void
		}
	}

	// Result constructor calls: Result.Ok(val)
	if member, ok := e.Callee.(*ast.MemberExpr); ok {
		if targetIdent, ok := member.Target.(*ast.IdentExpr); ok && targetIdent.Name == "Option" {
			if member.Field == "Some" && len(e.Args) == 1 {
				argT := a.checkExpr(e.Args[0])
				a.consumeValue(e.Args[0])
				return &types.OptionType{Elem: argT}
			}
			if member.Field == "None" && len(e.Args) == 0 {
				return &types.OptionType{Elem: types.Void}
			}
			a.error(e.Span(), "invalid Option constructor")
			return types.Void
		}
		if targetIdent, ok := member.Target.(*ast.IdentExpr); ok && targetIdent.Name == "Result" {
			if member.Field == "Ok" && len(e.Args) == 1 {
				argT := a.checkExpr(e.Args[0])
				a.consumeValue(e.Args[0])
				return &types.ResultType{OkType: argT, ErrType: types.Void}
			}
			if member.Field == "Err" && len(e.Args) == 1 {
				argT := a.checkExpr(e.Args[0])
				a.consumeValue(e.Args[0])
				return &types.ResultType{OkType: types.Void, ErrType: argT}
			}
			a.error(e.Span(), "invalid Result constructor")
			return types.Void
		}

		// Channel constructor: Channel.new(capacity)
		if targetIdent, ok := member.Target.(*ast.IdentExpr); ok && targetIdent.Name == "Channel" {
			if member.Field == "new" {
				if len(e.Args) > 0 {
					a.checkExpr(e.Args[0])
				}
				return &types.ChannelType{Elem: types.U32}
			}
		}

		// Channel methods: ch.send(val) and ch.recv()
		targetType := a.checkExpr(member.Target)
		if chType, ok := targetType.(*types.ChannelType); ok {
			if member.Field == "send" {
				if len(e.Args) != 1 {
					a.error(e.Span(), "channel.send expects 1 argument")
					return types.Bool
				}
				argT := a.checkExpr(e.Args[0])
				a.consumeValue(e.Args[0])
				if !chType.Elem.Equals(argT) && argT != types.UntypedInt {
					a.error(e.Args[0].Span(), fmt.Sprintf("cannot send value of type %s into Channel[%s]", argT, chType.Elem))
				}
				return types.Bool
			}
			if member.Field == "recv" {
				return &types.OptionType{Elem: chType.Elem}
			}
		}

		// Arena allocation: pool.alloc(Type) or pool.alloc(size)
		if targetIdent, ok := member.Target.(*ast.IdentExpr); ok {
			sym := a.curScope.Lookup(targetIdent.Name)
			if sym != nil && (sym.Kind == SymbolArena || isArenaType(sym.Type)) {
				if member.Field == "alloc" {
					if len(e.Args) != 1 {
						a.error(e.Span(), "arena.alloc expects exactly 1 argument (type or size)")
						return types.Void
					}
					a.lastAllocArena = sym.Name
					// Check if argument is a type identifier (e.g. Packet or u32)
					if argIdent, ok := e.Args[0].(*ast.IdentExpr); ok {
						if typeSym := a.curScope.Lookup(argIdent.Name); typeSym != nil && typeSym.Kind == SymbolType {
							return &types.PointerType{Elem: typeSym.Type, IsMut: true}
						}
						if prim := types.LookupPrimitive(argIdent.Name); prim != nil {
							return &types.PointerType{Elem: prim, IsMut: true}
						}
					}
					// Otherwise, argument is size expression
					argT := a.checkExpr(e.Args[0])
					if !types.IsInteger(argT) && argT != types.UntypedInt {
						a.error(e.Args[0].Span(), "arena.alloc size must be an integer expression or a type name")
					}
					return &types.PointerType{Elem: types.U8, IsMut: true}
				}
			}
		}
		if member.Field == "alloc" {
			a.error(e.Span(), "alloc requires an arena receiver")
			for _, arg := range e.Args {
				a.checkExpr(arg)
			}
			return types.Void
		}

		// Event constructor or other enum calls
		if len(e.Args) > 0 {
			a.checkExpr(e.Args[0])
			return types.Void
		}
	}

	if fn, ok := calleeType.(*types.FuncType); ok {
		if len(e.Args) != len(fn.Params) {
			a.errorCode(e.Span(), diag.ArgumentCount, fmt.Sprintf("function expects %d arguments, got %d", len(fn.Params), len(e.Args)))
		}
		for i := 0; i < len(e.Args) && i < len(fn.Params); i++ {
			argType := a.checkExpr(e.Args[i])
			a.consumeValue(e.Args[i])
			if argType == types.UntypedInt && types.IsInteger(fn.Params[i]) {
				argType = fn.Params[i]
			}
			if paramPtr, ok := fn.Params[i].(*types.PointerType); ok {
				if argPtr, ok := argType.(*types.PointerType); ok {
					if paramPtr.Elem.Equals(argPtr.Elem) {
						continue
					}
				}
			}
			if !fn.Params[i].Equals(argType) && argType != types.Void && !a.adaptVariant(e.Args[i], argType, fn.Params[i]) {
				a.error(e.Args[i].Span(), fmt.Sprintf("argument %d: expected %s, got %s", i+1, fn.Params[i], argType))
			}
		}
		return fn.ReturnType
	}

	return types.Void
}

func (a *Analyzer) checkMemberExpr(e *ast.MemberExpr) types.Type {
	targetType := a.checkExpr(e.Target)

	if ptr, ok := targetType.(*types.PointerType); ok {
		targetType = ptr.Elem
	}

	// Struct field access
	if st, ok := targetType.(*types.StructType); ok {
		for _, f := range st.Fields {
			if f.Name == e.Field {
				return f.Type
			}
		}
		a.error(e.Span(), fmt.Sprintf("struct %s has no field named %q", st.Name, e.Field))
		return types.Void
	}

	// Namespaces (e.g. Result.Ok, Option.Some, log.warn)
	return types.Void
}

func (a *Analyzer) checkIndexExpr(e *ast.IndexExpr) types.Type {
	targetType := a.checkExpr(e.Target)
	idxType := a.checkExpr(e.Index)

	if !idxType.Equals(types.Usize) && !idxType.Equals(types.U32) && idxType != types.Void {
		a.error(e.Index.Span(), fmt.Sprintf("slice index must be an integer (usize), got %s", idxType))
	}

	if sl, ok := targetType.(*types.SliceType); ok {
		a.error(e.Span(), "slice indexing is not supported by the LLVM backend")
		return sl.Elem
	}

	a.error(e.Span(), fmt.Sprintf("cannot index into non-slice type %s", targetType))
	return types.Void
}

func (a *Analyzer) checkCastExpr(e *ast.CastExpr) types.Type {
	_ = a.checkExpr(e.Target)
	toType := a.resolveType(e.ToType)

	// Casting to raw pointer requires unsafe
	if _, ok := toType.(*types.PointerType); ok && !a.curScope.IsUnsafe {
		a.errorWithHint(e.Span(),
			"raw pointer cast requires an 'unsafe' block",
			"cast between integers and raw pointers is a low-level operation; wrap it in 'unsafe { ... }'")
	}

	return toType
}

func (a *Analyzer) checkQuestionExpr(e *ast.QuestionExpr) types.Type {
	targetType := a.checkExpr(e.Target)
	a.consumeValue(e.Target)

	if res, ok := targetType.(*types.ResultType); ok {
		if a.curFuncType == nil {
			a.error(e.Span(), "operator '?' requires a function returning Result")
		} else if fnRes, ok := a.curFuncType.ReturnType.(*types.ResultType); !ok || !fnRes.ErrType.Equals(res.ErrType) {
			a.error(e.Span(), fmt.Sprintf("cannot propagate error of type %s in function returning %s", res.ErrType, a.curFuncType.ReturnType))
		}
		return res.OkType
	}

	if opt, ok := targetType.(*types.OptionType); ok {
		if a.curFuncType == nil {
			a.error(e.Span(), "operator '?' requires a function returning Option")
		} else if _, ok := a.curFuncType.ReturnType.(*types.OptionType); !ok {
			a.error(e.Span(), fmt.Sprintf("cannot propagate Option absence in function returning %s", a.curFuncType.ReturnType))
		}
		return opt.Elem
	}

	a.error(e.Span(), fmt.Sprintf("operator '?' can only be applied to Result or Option, got %s", targetType))
	return types.Void
}

func (a *Analyzer) resolveType(t ast.Type) types.Type {
	if t == nil {
		return types.Void
	}

	switch ty := t.(type) {
	case *ast.NamedType:
		if p := types.LookupPrimitive(ty.Name); p != nil {
			return p
		}
		sym := a.globalScope.Lookup(ty.Name)
		if sym != nil && sym.Kind == SymbolType {
			return sym.Type
		}
		a.error(ty.Span(), fmt.Sprintf("unknown type %q", ty.Name))
		return types.Void

	case *ast.PointerType:
		elem := a.resolveType(ty.ElemType)
		return &types.PointerType{Elem: elem, IsMut: ty.IsMut}

	case *ast.SliceType:
		elem := a.resolveType(ty.ElemType)
		return &types.SliceType{Elem: elem}

	case *ast.GenericType:
		if ty.Base == "Result" && len(ty.TypeArgs) == 2 {
			okT := a.resolveType(ty.TypeArgs[0])
			errT := a.resolveType(ty.TypeArgs[1])
			return &types.ResultType{OkType: okT, ErrType: errT}
		}
		if ty.Base == "Option" && len(ty.TypeArgs) == 1 {
			elemT := a.resolveType(ty.TypeArgs[0])
			return &types.OptionType{Elem: elemT}
		}
		if ty.Base == "Channel" && len(ty.TypeArgs) == 1 {
			elemT := a.resolveType(ty.TypeArgs[0])
			return &types.ChannelType{Elem: elemT}
		}
		a.error(ty.Span(), fmt.Sprintf("unsupported generic type %s", ty.Base))
		return types.Void

	default:
		return types.Void
	}
}

func isArenaType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.(*types.ArenaType)
	return ok
}
