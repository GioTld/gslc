package ir

import (
	"fmt"
	"maps"
	"math"
	"strconv"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/sema"
	"github.com/GioTld/gslc/pkg/types"
)

// Builder translates a checked AST into an IR Module.
type Builder struct {
	sem         *sema.Analyzer
	regCounter  int
	lblCounter  int
	curFunc     *Function
	curBlock    *BasicBlock
	locals      map[string]Value      // maps variable name to its alloca pointer or value
	constInts   map[string]int64      // maps constant name to its evaluated integer value
	constTypes  map[string]types.Type // maps constant name to its declared type
	fnTypes     map[string]types.Type // maps function name to its return type
	blockArenas [][]Value             // stack of arena values per block for scope cleanup
	structs     map[string]*types.StructType
	breakLabel  string
	contLabel   string
}

// NewBuilder creates a new IR Builder.
func NewBuilder(sem *sema.Analyzer) *Builder {
	return &Builder{
		sem:        sem,
		locals:     make(map[string]Value),
		constInts:  make(map[string]int64),
		constTypes: make(map[string]types.Type),
		fnTypes:    make(map[string]types.Type),
		structs:    make(map[string]*types.StructType),
	}
}

func (b *Builder) newReg(t types.Type) *Reg {
	b.regCounter++
	return &Reg{ID: b.regCounter, ValType: t}
}

func (b *Builder) newLabel(prefix string) string {
	b.lblCounter++
	return fmt.Sprintf("%s_%d", prefix, b.lblCounter)
}

func (b *Builder) newBlock(name string) *BasicBlock {
	bb := &BasicBlock{Name: name}
	b.curFunc.Blocks = append(b.curFunc.Blocks, bb)
	return bb
}

func (b *Builder) emit(inst Instruction) {
	b.curBlock.Insts = append(b.curBlock.Insts, inst)
}

// Build lowers a full GSL Program into an IR Module.
func (b *Builder) Build(prog *ast.Program) *Module {
	mod := &Module{
		Name: "main",
	}

	for _, decl := range prog.Decls {
		if st, ok := decl.(*ast.StructDecl); ok {
			stType := b.sem.StructType(st.Name)
			b.structs[st.Name] = stType
			mod.Structs = append(mod.Structs, stType)
		} else if ar, ok := decl.(*ast.ArenaDecl); ok {
			cap := b.evalConstInt(ar.SizeExpr)
			if cap <= 0 {
				cap = 64 * 1024
			}
			b.locals[ar.Name] = &Reg{ID: 0, ValType: &types.ArenaType{Name: ar.Name, Capacity: cap}}
		} else if cd, ok := decl.(*ast.ConstDecl); ok {
			v := b.evalConstInt(cd.Value)
			b.constInts[cd.Name] = v
			var cType types.Type = types.Usize
			if cd.Type != nil {
				cType = b.resolveType(cd.Type)
			} else if b.sem.ExprType(cd.Value) == types.Bool {
				cType = types.Bool
			}
			b.constTypes[cd.Name] = cType
			b.locals[cd.Name] = &ConstInt{Val: v, ValType: cType}
		}
	}

	for _, decl := range prog.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			var retT types.Type = types.Void
			if fn.ReturnType != nil {
				retT = b.resolveType(fn.ReturnType)
			}
			b.fnTypes[fn.Name] = retT
		}
	}

	for _, decl := range prog.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			irFn := b.buildFunc(fn)
			mod.Functions = append(mod.Functions, irFn)
		}
	}

	return mod
}

func (b *Builder) buildFunc(fn *ast.FuncDecl) *Function {
	b.regCounter = 0
	b.locals = make(map[string]Value)

	irFn := &Function{
		Name: fn.Name,
	}
	b.curFunc = irFn
	b.curBlock = b.newBlock("entry")

	// Lower parameters
	for _, p := range fn.Params {
		pType := b.resolveType(p.Type)
		pReg := b.newReg(pType)
		irFn.Params = append(irFn.Params, pReg)
	}

	for i, p := range fn.Params {
		pType := b.resolveType(p.Type)
		pReg := irFn.Params[i]
		// Allocate stack slot for parameter
		slot := b.newReg(&types.PointerType{Elem: pType, IsMut: true})
		b.emit(&InstAlloca{Dest: slot, ElemType: pType})
		b.emit(&InstStore{Val: pReg, DestPtr: slot})
		b.locals[p.Name] = slot
	}

	if fn.ReturnType != nil {
		irFn.ReturnType = b.resolveType(fn.ReturnType)
	} else {
		irFn.ReturnType = types.Void
	}

	// Lower function body
	if fn.Body != nil {
		b.buildBlock(fn.Body)
	}

	// Ensure function ends with a return terminator
	if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
		b.emit(&InstReturn{})
	}

	return irFn
}

func isTerminator(inst Instruction) bool {
	switch inst.(type) {
	case *InstReturn, *InstBranch, *InstBranchCond, *InstUnreachable:
		return true
	default:
		return false
	}
}

func (b *Builder) buildBlock(blk *ast.BlockExpr) {
	outerLocals, outerInts, outerTypes := b.locals, b.constInts, b.constTypes
	b.locals = maps.Clone(outerLocals)
	b.constInts = maps.Clone(outerInts)
	b.constTypes = maps.Clone(outerTypes)
	defer func() {
		b.locals, b.constInts, b.constTypes = outerLocals, outerInts, outerTypes
	}()
	b.blockArenas = append(b.blockArenas, nil)
	for _, stmt := range blk.Stmts {
		b.buildStmt(stmt)
	}

	// Reset arenas declared in this block if block doesn't terminate with return/branch
	arenasToReset := b.blockArenas[len(b.blockArenas)-1]
	if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
		for i := len(arenasToReset) - 1; i >= 0; i-- {
			b.emit(&InstArenaReset{Arena: arenasToReset[i]})
		}
	}
	b.blockArenas = b.blockArenas[:len(b.blockArenas)-1]
}

func (b *Builder) buildStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.ArenaDecl:
		cap := b.evalConstInt(s.SizeExpr)
		if cap <= 0 {
			cap = 64 * 1024
		}
		arenaType := &types.ArenaType{Name: s.Name, Capacity: cap}
		arenaReg := b.newReg(arenaType)
		b.emit(&InstArenaInit{Dest: arenaReg, Name: s.Name, Capacity: cap})
		b.locals[s.Name] = arenaReg
		if len(b.blockArenas) > 0 {
			b.blockArenas[len(b.blockArenas)-1] = append(b.blockArenas[len(b.blockArenas)-1], arenaReg)
		}

	case *ast.ConstDecl:
		v := b.evalConstInt(s.Value)
		b.constInts[s.Name] = v
		var cType types.Type = types.Usize
		if s.Type != nil {
			cType = b.resolveType(s.Type)
		}
		b.constTypes[s.Name] = cType
		val := b.buildExpr(s.Value)
		b.locals[s.Name] = val

	case *ast.LetStmt:
		var expected types.Type
		if s.Type != nil {
			expected = b.resolveType(s.Type)
		}
		val := b.buildExprExpected(s.Value, expected)
		valType := val.Type()
		slot := b.newReg(&types.PointerType{Elem: valType, IsMut: false})
		b.emit(&InstAlloca{Dest: slot, ElemType: valType})
		b.emit(&InstStore{Val: val, DestPtr: slot})
		b.locals[s.Name] = slot

	case *ast.VarStmt:
		var val Value
		var valType types.Type = types.U32
		if s.Type != nil {
			valType = b.resolveType(s.Type)
		}
		if s.Value != nil {
			var expected types.Type
			if s.Type != nil {
				expected = valType
			}
			val = b.buildExprExpected(s.Value, expected)
			// If a type was explicitly declared and the value is an untyped/u32 integer const,
			// coerce it to the declared type to avoid width mismatches (e.g. usize vs i32).
			if s.Type != nil {
				if ci, ok := val.(*ConstInt); ok && (ci.ValType == types.U32 || ci.ValType == types.UntypedInt) {
					ci.ValType = valType
				}
			} else {
				valType = val.Type()
			}
		} else {
			if _, isStruct := valType.(*types.StructType); !isStruct {
				val = &ConstInt{Val: 0, ValType: valType}
			}
		}
		slot := b.newReg(&types.PointerType{Elem: valType, IsMut: true})
		b.emit(&InstAlloca{Dest: slot, ElemType: valType})
		if val != nil {
			b.emit(&InstStore{Val: val, DestPtr: slot})
		}
		b.locals[s.Name] = slot

	case *ast.AssignStmt:
		expected := b.sem.ExprType(s.Target)
		if ident, ok := s.Target.(*ast.IdentExpr); ok {
			if slot, ok := b.locals[ident.Name]; ok {
				if ptr, ok := slot.Type().(*types.PointerType); ok {
					expected = ptr.Elem
				}
			}
		}
		val := b.buildExprExpected(s.Value, expected)
		if ident, ok := s.Target.(*ast.IdentExpr); ok {
			if slot, ok := b.locals[ident.Name]; ok {
				if s.Op == "=" {
					b.emit(&InstStore{Val: val, DestPtr: slot})
				} else {
					// Compound assignment: +=, -=, etc.
					slotType := slot.Type().(*types.PointerType).Elem
					loaded := b.newReg(slotType)
					b.emit(&InstLoad{Dest: loaded, SrcPtr: slot, ValType: slotType})
					if ci, ok := val.(*ConstInt); ok && types.IsInteger(slotType) {
						ci.ValType = slotType
					}
					res := b.newReg(slotType)
					binOp := mapCompoundOp(s.Op)
					b.emit(&InstBinary{Dest: res, Op: binOp, Left: loaded, Right: val})
					b.emit(&InstStore{Val: res, DestPtr: slot})
				}
			}
		} else if member, ok := s.Target.(*ast.MemberExpr); ok {
			fieldPtr, fType := b.buildFieldPtr(member)
			if fieldPtr != nil && fType != types.Void {
				if ci, ok := val.(*ConstInt); ok && (ci.ValType == types.U32 || ci.ValType == types.UntypedInt) {
					ci.ValType = fType
				}
				if s.Op == "=" {
					b.emit(&InstStore{Val: val, DestPtr: fieldPtr})
				} else {
					loaded := b.newReg(fType)
					b.emit(&InstLoad{Dest: loaded, SrcPtr: fieldPtr, ValType: fType})
					if ci, ok := val.(*ConstInt); ok && types.IsInteger(fType) {
						ci.ValType = fType
					}
					res := b.newReg(fType)
					binOp := mapCompoundOp(s.Op)
					b.emit(&InstBinary{Dest: res, Op: binOp, Left: loaded, Right: val})
					b.emit(&InstStore{Val: res, DestPtr: fieldPtr})
				}
			}
		}

	case *ast.ReturnStmt:
		var val Value
		if s.Value != nil {
			val = b.buildExprExpected(s.Value, b.curFunc.ReturnType)
			if ci, ok := val.(*ConstInt); ok && b.curFunc != nil && b.curFunc.ReturnType != nil && b.curFunc.ReturnType != types.Void {
				if types.IsInteger(b.curFunc.ReturnType) {
					ci.ValType = b.curFunc.ReturnType
				}
			}
			if val.Type() == types.Void {
				val = nil
			}
		}
		// Reset active scoped arenas in current function before return
		for i := len(b.blockArenas) - 1; i >= 0; i-- {
			for j := len(b.blockArenas[i]) - 1; j >= 0; j-- {
				b.emit(&InstArenaReset{Arena: b.blockArenas[i][j]})
			}
		}
		b.emit(&InstReturn{Val: val})

	case *ast.IfStmt:
		condVal := b.buildExpr(s.Condition)
		thenLbl := b.newLabel("then")
		elseLbl := b.newLabel("else")
		mergeLbl := b.newLabel("if_merge")

		if s.ElseBranch != nil {
			b.emit(&InstBranchCond{Cond: condVal, TrueLabel: thenLbl, FalseLabel: elseLbl})
		} else {
			b.emit(&InstBranchCond{Cond: condVal, TrueLabel: thenLbl, FalseLabel: mergeLbl})
		}

		// Then block
		b.curBlock = b.newBlock(thenLbl)
		b.buildBlock(s.ThenBlock)
		if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
			b.emit(&InstBranch{Target: mergeLbl})
		}

		// Else block
		if s.ElseBranch != nil {
			b.curBlock = b.newBlock(elseLbl)
			if blk, ok := s.ElseBranch.(*ast.BlockExpr); ok {
				b.buildBlock(blk)
			} else if innerIf, ok := s.ElseBranch.(*ast.IfStmt); ok {
				b.buildStmt(innerIf)
			}
			if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
				b.emit(&InstBranch{Target: mergeLbl})
			}
		}

		b.curBlock = b.newBlock(mergeLbl)

	case *ast.ForStmt:
		// Basic loop lowering
		loopLbl := b.newLabel("loop")
		exitLbl := b.newLabel("loop_exit")

		oldBreak := b.breakLabel
		oldCont := b.contLabel
		b.breakLabel = exitLbl
		b.contLabel = loopLbl

		b.emit(&InstBranch{Target: loopLbl})
		b.curBlock = b.newBlock(loopLbl)

		b.buildBlock(s.Body)

		if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
			b.emit(&InstBranch{Target: loopLbl})
		}

		b.curBlock = b.newBlock(exitLbl)
		b.breakLabel = oldBreak
		b.contLabel = oldCont

	case *ast.WhileStmt:
		headerLbl := b.newLabel("while_header")
		bodyLbl := b.newLabel("while_body")
		exitLbl := b.newLabel("while_exit")

		oldBreak := b.breakLabel
		oldCont := b.contLabel
		b.breakLabel = exitLbl
		b.contLabel = headerLbl

		b.emit(&InstBranch{Target: headerLbl})
		b.curBlock = b.newBlock(headerLbl)
		condVal := b.buildExpr(s.Condition)
		b.emit(&InstBranchCond{Cond: condVal, TrueLabel: bodyLbl, FalseLabel: exitLbl})

		b.curBlock = b.newBlock(bodyLbl)
		b.buildBlock(s.Body)
		if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
			b.emit(&InstBranch{Target: headerLbl})
		}

		b.curBlock = b.newBlock(exitLbl)
		b.breakLabel = oldBreak
		b.contLabel = oldCont

	case *ast.BreakStmt:
		if b.breakLabel != "" {
			b.emit(&InstBranch{Target: b.breakLabel})
		}

	case *ast.ContinueStmt:
		if b.contLabel != "" {
			b.emit(&InstBranch{Target: b.contLabel})
		}

	case *ast.UnsafeBlockStmt:
		b.buildBlock(s.Body)

	case *ast.SwitchStmt:
		condVal := b.buildExpr(s.Expr)
		endLbl := b.newLabel("switch_end")
		oldBreak := b.breakLabel
		b.breakLabel = endLbl

		for _, c := range s.Cases {
			caseLbl := b.newLabel("case")
			nextLbl := b.newLabel("case_next")

			caseVal := b.buildExpr(c.Pattern)
			cmpReg := b.newReg(types.Bool)
			b.emit(&InstBinary{Dest: cmpReg, Op: "eq", Left: condVal, Right: caseVal})
			b.emit(&InstBranchCond{Cond: cmpReg, TrueLabel: caseLbl, FalseLabel: nextLbl})

			b.curBlock = b.newBlock(caseLbl)
			for _, stmt := range c.Body {
				b.buildStmt(stmt)
			}
			if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
				b.emit(&InstBranch{Target: endLbl})
			}

			b.curBlock = b.newBlock(nextLbl)
		}

		if len(s.Default) > 0 {
			for _, stmt := range s.Default {
				b.buildStmt(stmt)
			}
		}

		if len(b.curBlock.Insts) == 0 || !isTerminator(b.curBlock.Insts[len(b.curBlock.Insts)-1]) {
			b.emit(&InstBranch{Target: endLbl})
		}

		b.curBlock = b.newBlock(endLbl)
		b.breakLabel = oldBreak

	case *ast.ExprStmt:
		b.buildExpr(s.Expression)
	}
}

func mapCompoundOp(op string) string {
	switch op {
	case "+=":
		return "add"
	case "-=":
		return "sub"
	case "*=":
		return "mul"
	case "/=":
		return "sdiv"
	case "%=":
		return "srem"
	case "&=":
		return "and"
	case "|=":
		return "or"
	case "^=":
		return "xor"
	case "<<=":
		return "shl"
	case ">>=":
		return "ashr"
	default:
		return "add"
	}
}

func (b *Builder) buildExprExpected(expr ast.Expr, expected types.Type) Value {
	if expected != nil && types.IsInteger(expected) && expected != types.UntypedInt && b.sem.ExprType(expr) == types.UntypedInt {
		switch e := expr.(type) {
		case *ast.IntLitExpr:
			value := b.buildExpr(e).(*ConstInt)
			value.ValType = expected
			return value
		case *ast.BinaryExpr:
			left := b.buildExprExpected(e.Left, expected)
			right := b.buildExprExpected(e.Right, expected)
			result := b.newReg(expected)
			b.emit(&InstBinary{Dest: result, Op: mapBinaryOp(e.Op), Left: left, Right: right})
			return result
		case *ast.UnaryExpr:
			operand := b.buildExprExpected(e.Right, expected)
			result := b.newReg(expected)
			b.emit(&InstUnary{Dest: result, Op: mapUnaryOp(e.Op), Operand: operand})
			return result
		case *ast.ComptimeExpr:
			return b.buildExprExpected(e.Expr, expected)
		}
	}
	return b.buildExpr(expr)
}

func (b *Builder) buildExpr(expr ast.Expr) Value {
	if expr == nil {
		return &ConstInt{Val: 0, ValType: types.Void}
	}

	switch e := expr.(type) {
	case *ast.IntLitExpr:
		v, _ := e.Uint64()
		if v > math.MaxUint32 {
			return &ConstInt{Val: int64(v), ValType: types.U64}
		}
		return &ConstInt{Val: int64(v), ValType: types.U32}

	case *ast.BoolLitExpr:
		return &ConstBool{Val: e.Value}

	case *ast.StringLitExpr:
		reg := b.newReg(types.String)
		b.emit(&InstStringAddr{Dest: reg, Literal: &ConstString{Val: e.Value}})
		return reg

	case *ast.IdentExpr:
		if slot, ok := b.locals[e.Name]; ok {
			if constVal, ok := slot.(*ConstInt); ok {
				return constVal
			}
			var elemT types.Type = types.U32
			if ptrT, ok := slot.Type().(*types.PointerType); ok {
				elemT = ptrT.Elem
			}
			reg := b.newReg(elemT)
			b.emit(&InstLoad{Dest: reg, SrcPtr: slot, ValType: elemT})
			return reg
		}
		if v, ok := b.constInts[e.Name]; ok {
			var cType types.Type = types.Usize
			if t, ok := b.constTypes[e.Name]; ok {
				cType = t
			}
			return &ConstInt{Val: v, ValType: cType}
		}
		return &ConstInt{Val: 0, ValType: types.U32}

	case *ast.ComptimeExpr:
		return b.buildExpr(e.Expr)

	case *ast.MatchExpr:
		return b.buildMatchExpr(e)

	case *ast.QuestionExpr:
		return b.buildQuestionExpr(e)

	case *ast.AsmExpr:
		b.emit(&InstAsm{Instruction: e.Instruction})
		return &ConstInt{Val: 0, ValType: types.Void}

	case *ast.BinaryExpr:
		if e.Op == "&&" || e.Op == "||" {
			return b.buildLogicalExpr(e)
		}
		left := b.buildExprExpected(e.Left, b.sem.ExprType(e.Right))
		right := b.buildExprExpected(e.Right, b.sem.ExprType(e.Left))
		if literal, ok := left.(*ConstInt); ok && types.IsInteger(right.Type()) {
			literal.ValType = right.Type()
		}
		if literal, ok := right.(*ConstInt); ok && types.IsInteger(left.Type()) {
			literal.ValType = left.Type()
		}

		opName := mapBinaryOp(e.Op)
		resType := left.Type()
		switch e.Op {
		case "==", "!=", "<", "<=", ">", ">=", "&&", "||":
			resType = types.Bool
		}
		res := b.newReg(resType)
		b.emit(&InstBinary{Dest: res, Op: opName, Left: left, Right: right})
		return res

	case *ast.UnaryExpr:
		if e.Op == "&" {
			// Address-of: return the alloca/slot pointer for the local variable or field pointer for member expr
			if ident, ok := e.Right.(*ast.IdentExpr); ok {
				if slot, found := b.locals[ident.Name]; found {
					return slot
				}
			} else if member, ok := e.Right.(*ast.MemberExpr); ok {
				ptr, _ := b.buildFieldPtr(member)
				if ptr != nil {
					return ptr
				}
			} else if deref, ok := e.Right.(*ast.UnaryExpr); ok && deref.Op == "*" {
				return b.buildExpr(deref.Right)
			}
			// Fallback: build the operand
			return b.buildExpr(e.Right)
		}
		operand := b.buildExpr(e.Right)
		if e.Op == "*" {
			// Dereference
			var elemT types.Type = types.U8
			if ptrT, ok := operand.Type().(*types.PointerType); ok {
				elemT = ptrT.Elem
			}
			reg := b.newReg(elemT)
			b.emit(&InstLoad{Dest: reg, SrcPtr: operand, ValType: elemT})
			return reg
		}
		res := b.newReg(operand.Type())
		b.emit(&InstUnary{Dest: res, Op: mapUnaryOp(e.Op), Operand: operand})
		return res

	case *ast.CallExpr:
		return b.buildCallExpr(e)

	case *ast.CastExpr:
		target := b.buildExpr(e.Target)
		toT := b.resolveType(e.ToType)
		res := b.newReg(toT)
		b.emit(&InstCast{Dest: res, Source: target, ToType: toT})
		return res

	case *ast.MemberExpr:
		ptr, fType := b.buildFieldPtr(e)
		if ptr != nil && fType != types.Void {
			loaded := b.newReg(fType)
			b.emit(&InstLoad{Dest: loaded, SrcPtr: ptr, ValType: fType})
			return loaded
		}
		target := b.buildExpr(e.Target)
		return target

	default:
		return &ConstInt{Val: 0, ValType: types.U32}
	}
}

func (b *Builder) buildLogicalExpr(expr *ast.BinaryExpr) Value {
	slot := b.newReg(&types.PointerType{Elem: types.Bool, IsMut: true})
	entry := b.curFunc.Blocks[0]
	entry.Insts = append([]Instruction{&InstAlloca{Dest: slot, ElemType: types.Bool}}, entry.Insts...)
	left := b.buildExpr(expr.Left)
	b.emit(&InstStore{Val: left, DestPtr: slot})
	rightLabel := b.newLabel("logical_right")
	mergeLabel := b.newLabel("logical_merge")
	branch := &InstBranchCond{Cond: left, TrueLabel: rightLabel, FalseLabel: mergeLabel}
	if expr.Op == "||" {
		branch.TrueLabel, branch.FalseLabel = mergeLabel, rightLabel
	}
	b.emit(branch)
	b.curBlock = b.newBlock(rightLabel)
	right := b.buildExpr(expr.Right)
	b.emit(&InstStore{Val: right, DestPtr: slot})
	b.emit(&InstBranch{Target: mergeLabel})
	b.curBlock = b.newBlock(mergeLabel)
	result := b.newReg(types.Bool)
	b.emit(&InstLoad{Dest: result, SrcPtr: slot, ValType: types.Bool})
	return result
}

func (b *Builder) buildFieldPtr(member *ast.MemberExpr) (Value, types.Type) {
	var basePtr Value
	var st *types.StructType

	if id, ok := member.Target.(*ast.IdentExpr); ok {
		if slot, ok := b.locals[id.Name]; ok {
			basePtr = slot
			t := slot.Type()
			if ptr, ok := t.(*types.PointerType); ok {
				if s, ok := ptr.Elem.(*types.StructType); ok {
					st = s
				} else if ptr2, ok := ptr.Elem.(*types.PointerType); ok {
					if s, ok := ptr2.Elem.(*types.StructType); ok {
						st = s
						loaded := b.newReg(ptr.Elem)
						b.emit(&InstLoad{Dest: loaded, SrcPtr: slot, ValType: ptr.Elem})
						basePtr = loaded
					}
				}
			}
		}
	} else if subMember, ok := member.Target.(*ast.MemberExpr); ok {
		subPtr, subType := b.buildFieldPtr(subMember)
		if subPtr != nil {
			if s, ok := subType.(*types.StructType); ok {
				st = s
				basePtr = subPtr
			} else if ptr, ok := subType.(*types.PointerType); ok {
				if s, ok := ptr.Elem.(*types.StructType); ok {
					st = s
					loaded := b.newReg(subType)
					b.emit(&InstLoad{Dest: loaded, SrcPtr: subPtr, ValType: subType})
					basePtr = loaded
				}
			}
		}
	} else {
		val := b.buildExpr(member.Target)
		basePtr = val
		if ptr, ok := val.Type().(*types.PointerType); ok {
			if s, ok := ptr.Elem.(*types.StructType); ok {
				st = s
			}
		} else if s, ok := val.Type().(*types.StructType); ok {
			st = s
			slot := b.newReg(&types.PointerType{Elem: s})
			b.emit(&InstAlloca{Dest: slot, ElemType: s})
			basePtr = slot
			b.emit(&InstStore{Val: val, DestPtr: basePtr})
		}
	}

	if st != nil {
		if realSt, ok := b.structs[st.Name]; ok {
			st = realSt
		}
	}

	if st == nil {
		return nil, types.Void
	}

	for idx, f := range st.Fields {
		if f.Name == member.Field {
			ptrReg := b.newReg(&types.PointerType{Elem: f.Type, IsMut: true})
			b.emit(&InstGetFieldPtr{
				Dest:       ptrReg,
				StructPtr:  basePtr,
				StructName: st.Name,
				FieldIndex: idx,
				FieldType:  f.Type,
			})
			return ptrReg, f.Type
		}
	}
	return nil, types.Void
}

func (b *Builder) buildCallExpr(e *ast.CallExpr) Value {
	// Hardware primitives
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		if ident.Name == "volatile_write" && len(e.Args) == 2 {
			addr := b.buildExpr(e.Args[0])
			val := b.buildExprExpected(e.Args[1], types.U8)
			b.emit(&InstVolatileStore{Val: val, DestPtr: addr})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "volatile_read" && len(e.Args) == 1 {
			addr := b.buildExpr(e.Args[0])
			res := b.newReg(types.U8)
			b.emit(&InstVolatileLoad{Dest: res, SrcPtr: addr, ValType: types.U8})
			return res
		}
		if ident.Name == "atomic_load" && len(e.Args) == 1 {
			ptr := b.buildExpr(e.Args[0])
			res := b.newReg(types.U64)
			b.emit(&InstAtomicLoad{Dest: res, Ptr: ptr, ValType: types.U64})
			return res
		}
		if ident.Name == "atomic_store" && len(e.Args) == 2 {
			ptr := b.buildExpr(e.Args[0])
			val := b.buildExpr(e.Args[1])
			b.emit(&InstAtomicStore{Val: val, Ptr: ptr})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "atomic_cas" && len(e.Args) == 3 {
			ptr := b.buildExpr(e.Args[0])
			expected := b.buildExpr(e.Args[1])
			desired := b.buildExpr(e.Args[2])
			res := b.newReg(types.Bool)
			b.emit(&InstAtomicCAS{Dest: res, Ptr: ptr, Expected: expected, Desired: desired})
			return res
		}
		if ident.Name == "syscall" && len(e.Args) == 4 {
			num := b.buildExprExpected(e.Args[0], types.U64)
			a1 := b.buildExprExpected(e.Args[1], types.U64)
			a2 := b.buildExprExpected(e.Args[2], types.U64)
			a3 := b.buildExprExpected(e.Args[3], types.U64)
			dest := b.newReg(types.I64)
			b.emit(&InstSyscall{Dest: dest, Num: num, A1: a1, A2: a2, A3: a3})
			return dest
		}
		if ident.Name == "syscall_write" && len(e.Args) == 3 {
			fd := b.buildExprExpected(e.Args[0], types.U64)
			buf := b.buildExprExpected(e.Args[1], types.U64)
			length := b.buildExprExpected(e.Args[2], types.U64)
			dest := b.newReg(types.I64)
			b.emit(&InstSyscallWrite{Dest: dest, Fd: fd, Buf: buf, Len: length})
			return dest
		}
		if ident.Name == "size_of" && len(e.Args) == 1 {
			size := b.evalTypeSize(e.Args[0])
			return &ConstInt{Val: size, ValType: types.Usize}
		}
		if ident.Name == "align_of" && len(e.Args) == 1 {
			align := b.evalTypeAlign(e.Args[0])
			return &ConstInt{Val: align, ValType: types.Usize}
		}
		if ident.Name == "yield" && len(e.Args) == 0 {
			b.emit(&InstYield{})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "symbol_address" && len(e.Args) == 1 {
			if name, ok := e.Args[0].(*ast.StringLitExpr); ok {
				res := b.newReg(types.Usize)
				symbol, _ := strconv.Unquote(name.Value)
				b.emit(&InstSymbolAddress{Dest: res, Name: symbol})
				return res
			}
		}
		if (ident.Name == "lgdt" || ident.Name == "lidt") && len(e.Args) == 1 {
			ptr := b.buildExpr(e.Args[0])
			b.emit(&InstLoadDescriptorTable{Kind: ident.Name, Ptr: ptr})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "inb" && len(e.Args) == 1 {
			port := b.buildExpr(e.Args[0])
			res := b.newReg(types.U8)
			b.emit(&InstInb{Dest: res, Port: port})
			return res
		}
		if ident.Name == "outb" && len(e.Args) == 2 {
			port := b.buildExpr(e.Args[0])
			val := b.buildExpr(e.Args[1])
			b.emit(&InstOutb{Port: port, Val: val})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "inw" && len(e.Args) == 1 {
			port := b.buildExpr(e.Args[0])
			res := b.newReg(types.U16)
			b.emit(&InstInw{Dest: res, Port: port})
			return res
		}
		if ident.Name == "outw" && len(e.Args) == 2 {
			port := b.buildExpr(e.Args[0])
			val := b.buildExpr(e.Args[1])
			b.emit(&InstOutw{Port: port, Val: val})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "hlt" && len(e.Args) == 0 {
			b.emit(&InstHlt{})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "cli" && len(e.Args) == 0 {
			b.emit(&InstCli{})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
		if ident.Name == "sti" && len(e.Args) == 0 {
			b.emit(&InstSti{})
			return &ConstInt{Val: 0, ValType: types.Void}
		}
	}

	// Result / Option constructors
	if member, ok := e.Callee.(*ast.MemberExpr); ok {
		if member.Field == "alloc" {
			if targetIdent, ok := member.Target.(*ast.IdentExpr); ok {
				if arenaVal, ok := b.locals[targetIdent.Name]; ok {
					var size Value = &ConstInt{Val: 8, ValType: types.Usize}
					align := 8
					var resType types.Type = &types.PointerType{Elem: types.U8, IsMut: true}

					if len(e.Args) == 1 {
						if argIdent, ok := e.Args[0].(*ast.IdentExpr); ok {
							if st, ok := b.structs[argIdent.Name]; ok {
								size = &ConstInt{Val: int64(st.Size()), ValType: types.Usize}
								align = st.Align()
								resType = &types.PointerType{Elem: st, IsMut: true}
							} else if prim := types.LookupPrimitive(argIdent.Name); prim != nil {
								size = &ConstInt{Val: int64(prim.Size()), ValType: types.Usize}
								align = prim.Align()
								resType = &types.PointerType{Elem: prim, IsMut: true}
							} else {
								size = b.buildExpr(e.Args[0])
							}
						} else {
							size = b.buildExpr(e.Args[0])
						}
					}
					if !size.Type().Equals(types.Usize) {
						converted := b.newReg(types.Usize)
						b.emit(&InstCast{Dest: converted, Source: size, ToType: types.Usize})
						size = converted
					}
					resReg := b.newReg(resType)
					b.emit(&InstArenaAlloc{Dest: resReg, Arena: arenaVal, Size: size, Align: align, ValType: resType})
					return resReg
				}
			}
		}

		if targetIdent, ok := member.Target.(*ast.IdentExpr); ok && (targetIdent.Name == "Result" || targetIdent.Name == "Option") {
			variantType := b.sem.ExprType(e)
			var payload Value
			if len(e.Args) == 1 {
				payload = b.buildExpr(e.Args[0])
			}
			tag := 0
			if member.Field == "Err" || member.Field == "Some" {
				tag = 1
			}
			result := b.newReg(variantType)
			b.emit(&InstVariantMake{Dest: result, Tag: tag, Payload: payload})
			return result
		}

		if targetIdent, ok := member.Target.(*ast.IdentExpr); ok && targetIdent.Name == "Channel" {
			if member.Field == "new" {
				cap := int64(16)
				if len(e.Args) > 0 {
					if c := b.evalConstInt(e.Args[0]); c > 0 {
						cap = c
					}
				}
				chReg := b.newReg(&types.ChannelType{Elem: types.U32})
				b.emit(&InstChannelInit{Dest: chReg, Capacity: cap})
				return chReg
			}
		}

		if member.Field == "send" && len(e.Args) == 1 {
			chVal := b.buildExpr(member.Target)
			val := b.buildExpr(e.Args[0])
			resReg := b.newReg(types.Bool)
			b.emit(&InstChannelSend{Dest: resReg, Channel: chVal, Val: val})
			return resReg
		}

		if member.Field == "recv" && len(e.Args) == 0 {
			chVal := b.buildExpr(member.Target)
			resReg := b.newReg(b.sem.ExprType(e))
			b.emit(&InstChannelRecv{Dest: resReg, Channel: chVal})
			return resReg
		}
	}

	fnName := "unknown"
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		fnName = ident.Name
	} else if member, ok := e.Callee.(*ast.MemberExpr); ok {
		fnName = fmt.Sprintf("%s_%s", member.Target.String(), member.Field)
	}

	args := make([]Value, len(e.Args))
	fnType, _ := b.sem.ExprType(e.Callee).(*types.FuncType)
	for i, a := range e.Args {
		var expected types.Type
		if fnType != nil && i < len(fnType.Params) {
			expected = fnType.Params[i]
		}
		args[i] = b.buildExprExpected(a, expected)
	}

	retType := types.Type(types.U32)
	if ft, ok := b.fnTypes[fnName]; ok {
		retType = ft
	} else if b.sem != nil {
		if t := b.sem.ExprType(e); t != nil {
			retType = t
		}
	}

	if retType == types.Void {
		b.emit(&InstCall{Dest: nil, Func: fnName, Args: args, RetTyp: types.Void})
		return &ConstInt{Val: 0, ValType: types.Void}
	}

	res := b.newReg(retType)
	b.emit(&InstCall{Dest: res, Func: fnName, Args: args, RetTyp: retType})
	return res
}

func mapBinaryOp(op string) string {
	switch op {
	case "+":
		return "add"
	case "-":
		return "sub"
	case "*":
		return "mul"
	case "/":
		return "sdiv"
	case "%":
		return "srem"
	case "==":
		return "eq"
	case "!=":
		return "ne"
	case "<":
		return "slt"
	case "<=":
		return "sle"
	case ">":
		return "sgt"
	case ">=":
		return "sge"
	case "&", "&&":
		return "and"
	case "|", "||":
		return "or"
	case "^":
		return "xor"
	case "<<":
		return "shl"
	case ">>":
		return "ashr"
	default:
		return "add"
	}
}

func mapUnaryOp(op string) string {
	switch op {
	case "-":
		return "neg"
	case "!":
		return "not"
	case "~":
		return "bitnot"
	default:
		return "neg"
	}
}

func (b *Builder) resolveType(t ast.Type) types.Type {
	if t == nil {
		return types.Void
	}
	switch ty := t.(type) {
	case *ast.NamedType:
		if p := types.LookupPrimitive(ty.Name); p != nil {
			return p
		}
		if st, ok := b.structs[ty.Name]; ok {
			return st
		}
		return &types.StructType{Name: ty.Name}
	case *ast.PointerType:
		return &types.PointerType{Elem: b.resolveType(ty.ElemType), IsMut: ty.IsMut}
	case *ast.SliceType:
		return &types.SliceType{Elem: b.resolveType(ty.ElemType)}
	case *ast.GenericType:
		switch ty.Base {
		case "Option":
			if len(ty.TypeArgs) == 1 {
				return &types.OptionType{Elem: b.resolveType(ty.TypeArgs[0])}
			}
		case "Result":
			if len(ty.TypeArgs) == 2 {
				return &types.ResultType{OkType: b.resolveType(ty.TypeArgs[0]), ErrType: b.resolveType(ty.TypeArgs[1])}
			}
		case "Channel":
			if len(ty.TypeArgs) == 1 {
				return &types.ChannelType{Elem: b.resolveType(ty.TypeArgs[0])}
			}
		}
	default:
		return types.U32
	}
	return types.U32
}

func (b *Builder) evalTypeSize(expr ast.Expr) int64 {
	if id, ok := expr.(*ast.IdentExpr); ok {
		if st, ok := b.structs[id.Name]; ok {
			return int64(st.Size())
		}
		if prim := types.LookupPrimitive(id.Name); prim != nil {
			return int64(prim.Size())
		}
	}
	return 8
}

func (b *Builder) evalTypeAlign(expr ast.Expr) int64 {
	if id, ok := expr.(*ast.IdentExpr); ok {
		if st, ok := b.structs[id.Name]; ok {
			return int64(st.Align())
		}
		if prim := types.LookupPrimitive(id.Name); prim != nil {
			return int64(prim.Align())
		}
	}
	return 8
}

func (b *Builder) evalConstInt(expr ast.Expr) int64 {
	if expr == nil {
		return 0
	}
	switch e := expr.(type) {
	case *ast.IntLitExpr:
		v, _ := e.Uint64()
		return int64(v)
	case *ast.BoolLitExpr:
		if e.Value {
			return 1
		}
		return 0
	case *ast.IdentExpr:
		if v, ok := b.constInts[e.Name]; ok {
			return v
		}
		return 0
	case *ast.ComptimeExpr:
		return b.evalConstInt(e.Expr)
	case *ast.CastExpr:
		return b.evalConstInt(e.Target)
	case *ast.CallExpr:
		if id, ok := e.Callee.(*ast.IdentExpr); ok {
			if id.Name == "size_of" && len(e.Args) == 1 {
				return b.evalTypeSize(e.Args[0])
			}
			if id.Name == "align_of" && len(e.Args) == 1 {
				return b.evalTypeAlign(e.Args[0])
			}
		}
	case *ast.UnaryExpr:
		v := b.evalConstInt(e.Right)
		if e.Op == "-" {
			return -v
		} else if e.Op == "~" {
			return ^v
		}
	case *ast.BinaryExpr:
		left := b.evalConstInt(e.Left)
		right := b.evalConstInt(e.Right)
		switch e.Op {
		case "+":
			return left + right
		case "-":
			return left - right
		case "*":
			return left * right
		case "/":
			if right != 0 {
				return left / right
			}
		case "%":
			if right != 0 {
				return left % right
			}
		case "&":
			return left & right
		case "|":
			return left | right
		case "^":
			return left ^ right
		case "<<":
			return left << right
		case ">>":
			return left >> right
		}
	}
	return 0
}
