package sema

import (
	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/types"
)

type variantPattern struct {
	name    string
	binding *ast.IdentExpr
	payload types.Type
	tag     int
}

func parseVariantPattern(pattern ast.Expr, target types.Type) (variantPattern, bool) {
	call, ok := pattern.(*ast.CallExpr)
	if !ok {
		return variantPattern{}, false
	}
	member, ok := call.Callee.(*ast.MemberExpr)
	if !ok {
		return variantPattern{}, false
	}
	namespace, ok := member.Target.(*ast.IdentExpr)
	if !ok {
		return variantPattern{}, false
	}
	var variant variantPattern
	switch t := target.(type) {
	case *types.OptionType:
		if namespace.Name != "Option" {
			return variant, false
		}
		switch member.Field {
		case "Some":
			variant = variantPattern{name: "Some", payload: t.Elem, tag: 1}
		case "None":
			variant = variantPattern{name: "None", tag: 0}
		default:
			return variant, false
		}
	case *types.ResultType:
		if namespace.Name != "Result" {
			return variant, false
		}
		switch member.Field {
		case "Ok":
			variant = variantPattern{name: "Ok", payload: t.OkType, tag: 0}
		case "Err":
			variant = variantPattern{name: "Err", payload: t.ErrType, tag: 1}
		default:
			return variant, false
		}
	default:
		return variant, false
	}
	if variant.payload == nil {
		return variant, len(call.Args) == 0
	}
	if len(call.Args) != 1 {
		return variant, false
	}
	variant.binding, ok = call.Args[0].(*ast.IdentExpr)
	return variant, ok && variant.binding.Name != "_"
}

func (a *Analyzer) adaptVariant(expr ast.Expr, actual, expected types.Type) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	member, ok := call.Callee.(*ast.MemberExpr)
	if !ok {
		return false
	}
	namespace, ok := member.Target.(*ast.IdentExpr)
	if !ok {
		return false
	}
	compatible := func(got, want types.Type) bool {
		return got.Equals(want) || got == types.UntypedInt && types.IsInteger(want)
	}
	switch want := expected.(type) {
	case *types.OptionType:
		got, ok := actual.(*types.OptionType)
		if !ok || namespace.Name != "Option" {
			return false
		}
		if member.Field == "None" && len(call.Args) == 0 || member.Field == "Some" && len(call.Args) == 1 && compatible(got.Elem, want.Elem) {
			a.exprTypes[expr] = expected
			return true
		}
	case *types.ResultType:
		got, ok := actual.(*types.ResultType)
		if !ok || namespace.Name != "Result" || len(call.Args) != 1 {
			return false
		}
		if member.Field == "Ok" && compatible(got.OkType, want.OkType) || member.Field == "Err" && compatible(got.ErrType, want.ErrType) {
			a.exprTypes[expr] = expected
			return true
		}
	}
	return false
}

func needsVariantAnnotation(t types.Type) bool {
	switch value := t.(type) {
	case *types.OptionType:
		return value.Elem == types.Void || value.Elem == types.UntypedInt
	case *types.ResultType:
		return value.OkType == types.Void || value.ErrType == types.Void ||
			value.OkType == types.UntypedInt || value.ErrType == types.UntypedInt
	}
	return false
}
