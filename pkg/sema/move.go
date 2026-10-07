package sema

import (
	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/diag"
	"github.com/GioTld/gslc/pkg/types"
)

func isCopyType(t types.Type) bool {
	switch value := t.(type) {
	case *types.ArenaType, *types.ChannelType:
		return false
	case *types.StructType:
		return value.IsCopy
	case *types.OptionType:
		return isCopyType(value.Elem)
	case *types.ResultType:
		return isCopyType(value.OkType) && isCopyType(value.ErrType)
	}
	return true
}

func (a *Analyzer) consumeValue(expr ast.Expr) {
	if valueType := a.exprTypes[expr]; valueType != nil && isCopyType(valueType) {
		return
	}
	var ident *ast.IdentExpr
	switch value := expr.(type) {
	case *ast.IdentExpr:
		ident = value
	case *ast.MemberExpr:
		ident, _ = value.Target.(*ast.IdentExpr)
	case *ast.IndexExpr:
		ident, _ = value.Target.(*ast.IdentExpr)
	}
	if ident == nil {
		return
	}
	symbol := a.curScope.Lookup(ident.Name)
	if symbol == nil || symbol.Kind != SymbolVar {
		return
	}
	if _, direct := expr.(*ast.IdentExpr); !direct {
		if _, pointer := symbol.Type.(*types.PointerType); pointer {
			a.error(expr.Span(), "cannot move a value out of borrowed pointer content")
			return
		}
	} else if isCopyType(symbol.Type) {
		return
	}
	if symbol.IsMoved {
		return
	}
	symbol.IsMoved = true
}

func snapshotMoved(scope *Scope) map[*Symbol]bool {
	state := make(map[*Symbol]bool)
	for current := scope; current != nil; current = current.Parent {
		for _, symbol := range current.Symbols {
			state[symbol] = symbol.IsMoved
		}
	}
	return state
}

func restoreMoved(state map[*Symbol]bool) {
	for symbol, moved := range state {
		symbol.IsMoved = moved
	}
}

func mergeMoved(left, right map[*Symbol]bool) {
	for symbol, moved := range left {
		symbol.IsMoved = moved || right[symbol]
	}
}

func (a *Analyzer) checkLoopMoves(before map[*Symbol]bool, span diag.Span) {
	for symbol, moved := range before {
		if !moved && symbol.IsMoved {
			a.error(span, "moving a value from an outer scope inside a repeating loop requires moving it before the loop")
		}
	}
}

func statementReturns(statement ast.Stmt) bool {
	switch node := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BlockExpr:
		return blockReturns(node)
	case *ast.IfStmt:
		return blockReturns(node.ThenBlock) && node.ElseBranch != nil && statementReturns(node.ElseBranch)
	}
	return false
}

func blockReturns(block *ast.BlockExpr) bool {
	if block == nil {
		return false
	}
	for _, statement := range block.Stmts {
		if statementReturns(statement) {
			return true
		}
	}
	return false
}
