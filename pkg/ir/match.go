package ir

import (
	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/types"
)

func (b *Builder) buildMatchExpr(expr *ast.MatchExpr) Value {
	target := b.buildExpr(expr.Target)
	var tag Value
	switch target.Type().(type) {
	case *types.OptionType, *types.ResultType:
		tagReg := b.newReg(types.U64)
		b.emit(&InstVariantTag{Dest: tagReg, Value: target})
		tag = tagReg
	}
	resultType := b.sem.ExprType(expr)
	resultSlot := b.newReg(&types.PointerType{Elem: resultType, IsMut: true})
	b.emit(&InstAlloca{Dest: resultSlot, ElemType: resultType})
	endLabel := b.newLabel("match_end")

	for _, arm := range expr.Arms {
		armLabel := b.newLabel("match_arm")
		nextLabel := b.newLabel("match_next")
		var binding string
		var payloadType types.Type
		if ident, ok := arm.Pattern.(*ast.IdentExpr); ok && ident.Name == "_" {
			b.emit(&InstBranch{Target: armLabel})
		} else if call, ok := arm.Pattern.(*ast.CallExpr); ok && tag != nil {
			member := call.Callee.(*ast.MemberExpr)
			variantTag := int64(0)
			if member.Field == "Some" || member.Field == "Err" {
				variantTag = 1
			}
			if len(call.Args) == 1 {
				binding = call.Args[0].(*ast.IdentExpr).Name
				switch t := target.Type().(type) {
				case *types.OptionType:
					payloadType = t.Elem
				case *types.ResultType:
					if variantTag == 0 {
						payloadType = t.OkType
					} else {
						payloadType = t.ErrType
					}
				}
			}
			condition := b.newReg(types.Bool)
			b.emit(&InstBinary{Dest: condition, Op: "eq", Left: tag, Right: &ConstInt{Val: variantTag, ValType: types.U64}})
			b.emit(&InstBranchCond{Cond: condition, TrueLabel: armLabel, FalseLabel: nextLabel})
		} else {
			pattern := b.buildExpr(arm.Pattern)
			condition := b.newReg(types.Bool)
			b.emit(&InstBinary{Dest: condition, Op: "eq", Left: target, Right: pattern})
			b.emit(&InstBranchCond{Cond: condition, TrueLabel: armLabel, FalseLabel: nextLabel})
		}

		b.curBlock = b.newBlock(armLabel)
		oldBinding, hadBinding := b.locals[binding]
		if binding != "" {
			payload := b.newReg(payloadType)
			b.emit(&InstVariantPayload{Dest: payload, Value: target})
			slot := b.newReg(&types.PointerType{Elem: payloadType, IsMut: false})
			b.emit(&InstAlloca{Dest: slot, ElemType: payloadType})
			b.emit(&InstStore{Val: payload, DestPtr: slot})
			b.locals[binding] = slot
		}
		value := b.buildExpr(arm.Value)
		if binding != "" {
			if hadBinding {
				b.locals[binding] = oldBinding
			} else {
				delete(b.locals, binding)
			}
		}
		if integer, ok := value.(*ConstInt); ok && types.IsInteger(resultType) {
			integer.ValType = resultType
		}
		b.emit(&InstStore{Val: value, DestPtr: resultSlot})
		b.emit(&InstBranch{Target: endLabel})
		b.curBlock = b.newBlock(nextLabel)
	}
	b.emit(&InstUnreachable{})
	b.curBlock = b.newBlock(endLabel)
	result := b.newReg(resultType)
	b.emit(&InstLoad{Dest: result, SrcPtr: resultSlot, ValType: resultType})
	return result
}

func (b *Builder) buildQuestionExpr(expr *ast.QuestionExpr) Value {
	value := b.buildExpr(expr.Target)
	tag := b.newReg(types.U64)
	b.emit(&InstVariantTag{Dest: tag, Value: value})
	successTag := int64(0)
	var payloadType types.Type
	switch variant := value.Type().(type) {
	case *types.OptionType:
		successTag = 1
		payloadType = variant.Elem
	case *types.ResultType:
		payloadType = variant.OkType
	}
	condition := b.newReg(types.Bool)
	b.emit(&InstBinary{Dest: condition, Op: "eq", Left: tag, Right: &ConstInt{Val: successTag, ValType: types.U64}})
	successLabel := b.newLabel("question_success")
	failureLabel := b.newLabel("question_failure")
	b.emit(&InstBranchCond{Cond: condition, TrueLabel: successLabel, FalseLabel: failureLabel})

	b.curBlock = b.newBlock(failureLabel)
	var errorPayload Value
	if variant, ok := value.Type().(*types.ResultType); ok {
		errorReg := b.newReg(variant.ErrType)
		b.emit(&InstVariantPayload{Dest: errorReg, Value: value})
		errorPayload = errorReg
	}
	failure := b.newReg(b.curFunc.ReturnType)
	failureTag := 0
	if errorPayload != nil {
		failureTag = 1
	}
	b.emit(&InstVariantMake{Dest: failure, Tag: failureTag, Payload: errorPayload})
	for i := len(b.blockArenas) - 1; i >= 0; i-- {
		for j := len(b.blockArenas[i]) - 1; j >= 0; j-- {
			b.emit(&InstArenaReset{Arena: b.blockArenas[i][j]})
		}
	}
	b.emit(&InstReturn{Val: failure})

	b.curBlock = b.newBlock(successLabel)
	result := b.newReg(payloadType)
	b.emit(&InstVariantPayload{Dest: result, Value: value})
	return result
}
