package sema

import (
	"fmt"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/types"
)

func (a *Analyzer) checkMatchExpr(expr *ast.MatchExpr) types.Type {
	targetType := a.checkExpr(expr.Target)
	a.consumeValue(expr.Target)
	before := snapshotMoved(a.curScope)
	merged := snapshotMoved(a.curScope)
	seen := make(map[string]bool)
	seenTrue, seenFalse, wildcard := false, false, false
	var result types.Type

	for index, arm := range expr.Arms {
		restoreMoved(before)
		oldScope := a.curScope
		a.curScope = NewScope(oldScope)
		isWildcard := false
		patternKey := arm.Pattern.String()
		if ident, ok := arm.Pattern.(*ast.IdentExpr); ok && ident.Name == "_" {
			isWildcard = true
			wildcard = true
			if index != len(expr.Arms)-1 {
				a.error(arm.Pattern.Span(), "wildcard match arm must be last")
			}
		} else if variant, ok := parseVariantPattern(arm.Pattern, targetType); ok {
			patternKey = variant.name
			if variant.binding != nil {
				_ = a.curScope.Define(&Symbol{Name: variant.binding.Name, Kind: SymbolVar, Type: variant.payload, Span: variant.binding.Span()})
			}
		} else if _, ok := targetType.(*types.OptionType); ok {
			a.error(arm.Pattern.Span(), "Option match pattern must be Option.Some(name), Option.None(), or '_'")
		} else if _, ok := targetType.(*types.ResultType); ok {
			a.error(arm.Pattern.Span(), "Result match pattern must be Result.Ok(name), Result.Err(name), or '_'")
		} else {
			patternType := a.checkExpr(arm.Pattern)
			if patternType == types.UntypedInt && types.IsInteger(targetType) {
				patternType = targetType
			}
			if !targetType.Equals(patternType) && targetType != types.Void && patternType != types.Void {
				a.error(arm.Pattern.Span(), fmt.Sprintf("match pattern has type %s, expected %s", patternType, targetType))
			}
			if targetType.Equals(types.Bool) {
				literal, ok := arm.Pattern.(*ast.BoolLitExpr)
				if !ok {
					a.error(arm.Pattern.Span(), "bool match patterns must be true or false")
				} else if literal.Value {
					seenTrue = true
				} else {
					seenFalse = true
				}
			} else if _, ok := arm.Pattern.(*ast.IntLitExpr); !ok {
				a.error(arm.Pattern.Span(), "only integer, bool, and wildcard match patterns are supported")
			}
		}
		if !isWildcard && seen[patternKey] {
			a.error(arm.Pattern.Span(), "duplicate match pattern")
		}
		seen[patternKey] = true
		armType := a.checkExpr(arm.Value)
		if armType == types.Void {
			a.error(arm.Value.Span(), "match arm must produce a value")
		}
		a.consumeValue(arm.Value)
		if result == nil {
			result = armType
		} else if result == types.UntypedInt && types.IsInteger(armType) {
			result = armType
		} else if armType != types.UntypedInt || !types.IsInteger(result) {
			if !result.Equals(armType) && result != types.Void && armType != types.Void {
				a.error(arm.Value.Span(), fmt.Sprintf("match arm has type %s, expected %s", armType, result))
			}
		}
		a.curScope = oldScope
		for symbol, moved := range snapshotMoved(a.curScope) {
			merged[symbol] = merged[symbol] || moved
		}
	}
	if !wildcard {
		switch targetType.(type) {
		case *types.OptionType:
			if !seen["Some"] || !seen["None"] {
				a.error(expr.Span(), "non-exhaustive Option match: cover Some and None or add '_'")
			}
		case *types.ResultType:
			if !seen["Ok"] || !seen["Err"] {
				a.error(expr.Span(), "non-exhaustive Result match: cover Ok and Err or add '_'")
			}
		default:
			if targetType.Equals(types.Bool) {
				if !seenTrue || !seenFalse {
					a.error(expr.Span(), "non-exhaustive bool match: cover true and false or add '_'")
				}
			} else {
				a.error(expr.Span(), "non-exhaustive match: add a wildcard arm '_'")
			}
		}
	}
	restoreMoved(merged)
	if result == nil || result == types.UntypedInt {
		return types.U32
	}
	return result
}
