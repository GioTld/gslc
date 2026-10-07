package llvm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GioTld/gslc/pkg/ir"
	"github.com/GioTld/gslc/pkg/types"
)

// Generator produces textual LLVM IR (.ll) from an IR Module.
type Generator struct {
	mod          *ir.Module
	tempSeq      int
	strLiterals  map[string]string
	strSizes     map[string]int
	TargetTriple string
	Kernel       bool
}

// NewGenerator creates a new LLVM IR generator.
func NewGenerator(mod *ir.Module) *Generator {
	return &Generator{mod: mod, TargetTriple: "x86_64-unknown-linux-gnu"}
}

func llvmEscapeString(s string) (string, int) {
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}
	var sb strings.Builder
	sb.WriteString("c\"")
	bytes := []byte(s)
	count := len(bytes) + 1
	for _, b := range bytes {
		if b >= 32 && b <= 126 && b != '"' && b != '\\' {
			sb.WriteByte(b)
		} else {
			sb.WriteString(fmt.Sprintf("\\%02X", b))
		}
	}
	sb.WriteString("\\00\"")
	return sb.String(), count
}

// Generate emits valid textual LLVM IR representation.
func (g *Generator) Generate() string {
	g.strLiterals = make(map[string]string)
	g.strSizes = make(map[string]int)
	var stringOrder []string
	strCounter := 0
	for _, fn := range g.mod.Functions {
		for _, blk := range fn.Blocks {
			for _, inst := range blk.Insts {
				var candidates []*ir.ConstString
				switch v := inst.(type) {
				case *ir.InstStringAddr:
					candidates = append(candidates, v.Literal)
				case *ir.InstVariantMake:
					if cs, ok := v.Payload.(*ir.ConstString); ok {
						candidates = append(candidates, cs)
					}
				case *ir.InstSyscallWrite:
					if cs, ok := v.Buf.(*ir.ConstString); ok {
						candidates = append(candidates, cs)
					}
				case *ir.InstCall:
					for _, arg := range v.Args {
						if cs, ok := arg.(*ir.ConstString); ok {
							candidates = append(candidates, cs)
						}
					}
				case *ir.InstSyscall:
					for _, a := range []ir.Value{v.Num, v.A1, v.A2, v.A3} {
						if cs, ok := a.(*ir.ConstString); ok {
							candidates = append(candidates, cs)
						}
					}
				case *ir.InstStore:
					if cs, ok := v.Val.(*ir.ConstString); ok {
						candidates = append(candidates, cs)
					}
				}
				for _, cs := range candidates {
					if _, exists := g.strLiterals[cs.Val]; !exists {
						strCounter++
						sym := fmt.Sprintf("@.str.%d", strCounter)
						g.strLiterals[cs.Val] = sym
						stringOrder = append(stringOrder, cs.Val)
						_, size := llvmEscapeString(cs.Val)
						g.strSizes[cs.Val] = size
					}
				}
			}
		}
	}

	var sb strings.Builder

	sb.WriteString("; ModuleID = 'gslc'\n")
	sb.WriteString("source_filename = \"" + g.mod.Name + ".gsl\"\n")
	sb.WriteString("target datalayout = \"e-m:e-p270:32:32-p271:32:32-p272:64:64-i64:64-i128:128-f80:128-n8:16:32:64-S128\"\n")
	sb.WriteString("target triple = \"" + g.TargetTriple + "\"\n\n")

	sb.WriteString("%struct.Arena = type { ptr, i64, i64 }\n")
	sb.WriteString("%struct.Allocator = type { ptr, ptr, ptr }\n")
	sb.WriteString("%struct.Channel = type { i64, i64, i64, ptr }\n")
	for _, st := range g.mod.Structs {
		var fieldTypes []string
		for _, f := range st.Fields {
			fieldTypes = append(fieldTypes, llvmType(f.Type))
		}
		if st.IsPacked {
			sb.WriteString(fmt.Sprintf("%%struct.%s = type <{ %s }>\n", st.Name, strings.Join(fieldTypes, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("%%struct.%s = type { %s }\n", st.Name, strings.Join(fieldTypes, ", ")))
		}
	}
	sb.WriteString("\n")
	sb.WriteString("declare void @llvm.trap()\n\n")

	for _, val := range stringOrder {
		sym := g.strLiterals[val]
		esc, size := llvmEscapeString(val)
		sb.WriteString(fmt.Sprintf("%s = private unnamed_addr constant [%d x i8] %s, align 1\n", sym, size, esc))
	}
	if len(g.strLiterals) > 0 {
		sb.WriteString("\n")
	}
	defined := make(map[string]bool)
	for _, fn := range g.mod.Functions {
		defined[fn.Name] = true
	}
	external := make(map[string]bool)
	for _, fn := range g.mod.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Insts {
				if address, ok := inst.(*ir.InstSymbolAddress); ok && !defined[address.Name] && !external[address.Name] {
					sb.WriteString(fmt.Sprintf("@%s = external global i8\n", address.Name))
					external[address.Name] = true
				}
			}
		}
	}
	if len(external) > 0 {
		sb.WriteString("\n")
	}

	for _, fn := range g.mod.Functions {
		g.generateFunc(&sb, fn)
		sb.WriteString("\n")
	}
	if g.Kernel {
		sb.WriteString("attributes #0 = { \"no-red-zone\"=\"true\" \"target-features\"=\"-sse,-sse2,-mmx,-avx\" }\n")
	}

	return sb.String()
}

func (g *Generator) generateFunc(sb *strings.Builder, fn *ir.Function) {
	retTypeStr := llvmType(fn.ReturnType)

	params := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = fmt.Sprintf("%s %s", llvmType(p.ValType), p.String())
	}

	attributes := ""
	if g.Kernel {
		attributes = " #0"
	}
	sb.WriteString(fmt.Sprintf("define %s @%s(%s)%s {\n", retTypeStr, fn.Name, strings.Join(params, ", "), attributes))

	for _, block := range fn.Blocks {
		sb.WriteString(block.Name + ":\n")
		for _, inst := range block.Insts {
			g.generateInst(sb, inst)
		}
	}

	sb.WriteString("}\n")
}

func (g *Generator) generateInst(sb *strings.Builder, inst ir.Instruction) {
	switch i := inst.(type) {
	case *ir.InstStringAddr:
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds [%d x i8], ptr %s, i64 0, i64 0\n",
			i.Dest, g.strSizes[i.Literal.Val], g.strLiterals[i.Literal.Val]))
	case *ir.InstAlloca:
		sb.WriteString(fmt.Sprintf("  %s = alloca %s, align %d\n",
			i.Dest, llvmType(i.ElemType), i.ElemType.Align()))

	case *ir.InstVariantMake:
		t := llvmType(i.Dest.Type())
		g.tempSeq++
		slot := fmt.Sprintf("%%variant_slot_%d", g.tempSeq)
		g.tempSeq++
		tagPtr := fmt.Sprintf("%%variant_tag_ptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = alloca %s, align 8\n", slot, t))
		sb.WriteString(fmt.Sprintf("  store %s zeroinitializer, ptr %s, align 8\n", t, slot))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr %s, ptr %s, i32 0, i32 0\n", tagPtr, t, slot))
		sb.WriteString(fmt.Sprintf("  store i64 %d, ptr %s, align 8\n", i.Tag, tagPtr))
		if i.Payload != nil {
			g.tempSeq++
			payloadPtr := fmt.Sprintf("%%variant_payload_ptr_%d", g.tempSeq)
			sb.WriteString(fmt.Sprintf("  %s = getelementptr %s, ptr %s, i32 0, i32 1\n", payloadPtr, t, slot))
			payload := i.Payload.String()
			if cs, ok := i.Payload.(*ir.ConstString); ok {
				g.tempSeq++
				literalPtr := fmt.Sprintf("%%variant_string_%d", g.tempSeq)
				sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds [%d x i8], ptr %s, i64 0, i64 0\n", literalPtr, g.strSizes[cs.Val], g.strLiterals[cs.Val]))
				payload = literalPtr
			}
			sb.WriteString(fmt.Sprintf("  store %s %s, ptr %s, align 1\n", llvmType(i.Payload.Type()), payload, payloadPtr))
		}
		sb.WriteString(fmt.Sprintf("  %s = load %s, ptr %s, align 8\n", i.Dest, t, slot))

	case *ir.InstVariantTag:
		sb.WriteString(fmt.Sprintf("  %s = extractvalue %s %s, 0\n", i.Dest, llvmType(i.Value.Type()), i.Value))

	case *ir.InstVariantPayload:
		t := llvmType(i.Value.Type())
		g.tempSeq++
		slot := fmt.Sprintf("%%variant_read_slot_%d", g.tempSeq)
		g.tempSeq++
		payloadPtr := fmt.Sprintf("%%variant_read_ptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = alloca %s, align 8\n", slot, t))
		sb.WriteString(fmt.Sprintf("  store %s %s, ptr %s, align 8\n", t, i.Value, slot))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr %s, ptr %s, i32 0, i32 1\n", payloadPtr, t, slot))
		sb.WriteString(fmt.Sprintf("  %s = load %s, ptr %s, align 1\n", i.Dest, llvmType(i.Dest.Type()), payloadPtr))

	case *ir.InstLoad:
		sb.WriteString(fmt.Sprintf("  %s = load %s, ptr %s, align %d\n",
			i.Dest, llvmType(i.ValType), i.SrcPtr, i.ValType.Align()))

	case *ir.InstStore:
		valT := i.Val.Type()
		valStr := i.Val.String()
		if cs, ok := i.Val.(*ir.ConstString); ok {
			sym := g.strLiterals[cs.Val]
			size := g.strSizes[cs.Val]
			g.tempSeq++
			ptrReg := fmt.Sprintf("%%str_store_%d", g.tempSeq)
			sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds [%d x i8], ptr %s, i64 0, i64 0\n", ptrReg, size, sym))
			valStr = ptrReg
		}
		if _, isPtr := valT.(*types.PointerType); isPtr {
			if ci, ok := i.Val.(*ir.ConstInt); ok && ci.Val == 0 {
				valStr = "null"
			}
		}
		sb.WriteString(fmt.Sprintf("  store %s %s, ptr %s, align %d\n",
			llvmType(valT), valStr, i.DestPtr, valT.Align()))

	case *ir.InstBinary:
		llvmOp := mapLLVMOp(i.Op, i.Left.Type())
		tStr := llvmType(i.Left.Type())
		if i.Op == "sdiv" || i.Op == "srem" || i.Op == "shl" || i.Op == "ashr" {
			g.tempSeq++
			id := g.tempSeq
			invalid := fmt.Sprintf("%%partial_invalid_%d", id)
			if i.Op == "shl" || i.Op == "ashr" {
				sb.WriteString(fmt.Sprintf("  %s = icmp uge %s %s, %d\n", invalid, tStr, i.Right, i.Left.Type().Size()*8))
			} else {
				sb.WriteString(fmt.Sprintf("  %s = icmp eq %s %s, 0\n", invalid, tStr, i.Right))
				if isSignedInteger(i.Left.Type()) {
					minimum := int64(-1) << (i.Left.Type().Size()*8 - 1)
					sb.WriteString(fmt.Sprintf("  %%partial_min_%d = icmp eq %s %s, %d\n", id, tStr, i.Left, minimum))
					sb.WriteString(fmt.Sprintf("  %%partial_minus_one_%d = icmp eq %s %s, -1\n", id, tStr, i.Right))
					sb.WriteString(fmt.Sprintf("  %%partial_overflow_%d = and i1 %%partial_min_%d, %%partial_minus_one_%d\n", id, id, id))
					sb.WriteString(fmt.Sprintf("  %%partial_bad_%d = or i1 %s, %%partial_overflow_%d\n", id, invalid, id))
					invalid = fmt.Sprintf("%%partial_bad_%d", id)
				}
			}
			sb.WriteString(fmt.Sprintf("  br i1 %s, label %%partial_trap_%d, label %%partial_ok_%d\n", invalid, id, id))
			sb.WriteString(fmt.Sprintf("partial_trap_%d:\n", id))
			sb.WriteString("  call void @llvm.trap()\n  unreachable\n")
			sb.WriteString(fmt.Sprintf("partial_ok_%d:\n", id))
		}
		sb.WriteString(fmt.Sprintf("  %s = %s %s %s, %s\n",
			i.Dest, llvmOp, tStr, i.Left, i.Right))

	case *ir.InstUnary:
		tStr := llvmType(i.Operand.Type())
		if i.Op == "neg" {
			sb.WriteString(fmt.Sprintf("  %s = sub %s 0, %s\n", i.Dest, tStr, i.Operand))
		} else if i.Op == "not" {
			sb.WriteString(fmt.Sprintf("  %s = xor i1 %s, true\n", i.Dest, i.Operand))
		} else if i.Op == "bitnot" {
			sb.WriteString(fmt.Sprintf("  %s = xor %s %s, -1\n", i.Dest, tStr, i.Operand))
		}

	case *ir.InstCall:
		retT := llvmType(i.RetTyp)
		args := make([]string, len(i.Args))
		for idx, a := range i.Args {
			if cs, ok := a.(*ir.ConstString); ok {
				sym := g.strLiterals[cs.Val]
				size := g.strSizes[cs.Val]
				g.tempSeq++
				ptrReg := fmt.Sprintf("%%arg_str_%d", g.tempSeq)
				sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds [%d x i8], ptr %s, i64 0, i64 0\n", ptrReg, size, sym))
				args[idx] = fmt.Sprintf("ptr %s", ptrReg)
			} else {
				args[idx] = fmt.Sprintf("%s %s", llvmType(a.Type()), a)
			}
		}
		if i.Dest != nil && retT != "void" {
			sb.WriteString(fmt.Sprintf("  %s = call %s @%s(%s)\n",
				i.Dest, retT, i.Func, strings.Join(args, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("  call %s @%s(%s)\n",
				retT, i.Func, strings.Join(args, ", ")))
		}

	case *ir.InstReturn:
		if i.Val != nil {
			sb.WriteString(fmt.Sprintf("  ret %s %s\n", llvmType(i.Val.Type()), i.Val))
		} else {
			sb.WriteString("  ret void\n")
		}

	case *ir.InstUnreachable:
		sb.WriteString("  unreachable\n")

	case *ir.InstBranch:
		sb.WriteString(fmt.Sprintf("  br label %%%s\n", i.Target))

	case *ir.InstBranchCond:
		sb.WriteString(fmt.Sprintf("  br i1 %s, label %%%s, label %%%s\n",
			i.Cond, i.TrueLabel, i.FalseLabel))

	case *ir.InstCast:
		fromT := llvmType(i.Source.Type())
		toT := llvmType(i.ToType)
		op := castOp(i.Source.Type(), i.ToType)
		sb.WriteString(fmt.Sprintf("  %s = %s %s %s to %s\n",
			i.Dest, op, fromT, i.Source, toT))

	case *ir.InstGetFieldPtr:
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.%s, ptr %s, i32 0, i32 %d\n",
			i.Dest, i.StructName, i.StructPtr, i.FieldIndex))

	case *ir.InstLoadDescriptorTable:
		sb.WriteString(fmt.Sprintf("  call void asm sideeffect \"%s ($0)\", \"r,~{memory}\"(i64 %s)\n", i.Kind, i.Ptr))

	case *ir.InstSymbolAddress:
		sb.WriteString(fmt.Sprintf("  %s = ptrtoint ptr @%s to i64\n", i.Dest, i.Name))

	case *ir.InstVolatileStore:
		valT := i.Val.Type()
		sb.WriteString(fmt.Sprintf("  store volatile %s %s, ptr %s, align %d\n",
			llvmType(valT), i.Val, i.DestPtr, valT.Align()))

	case *ir.InstVolatileLoad:
		sb.WriteString(fmt.Sprintf("  %s = load volatile %s, ptr %s, align %d\n",
			i.Dest, llvmType(i.ValType), i.SrcPtr, i.ValType.Align()))

	case *ir.InstArenaInit:
		g.tempSeq++
		bufReg := fmt.Sprintf("%%buf_%d", g.tempSeq)
		bufSlot := fmt.Sprintf("%%buf_slot_%d", g.tempSeq)
		offSlot := fmt.Sprintf("%%off_slot_%d", g.tempSeq)
		capSlot := fmt.Sprintf("%%cap_slot_%d", g.tempSeq)

		sb.WriteString(fmt.Sprintf("  %s = alloca [%d x i8], align 8\n", bufReg, i.Capacity))
		sb.WriteString(fmt.Sprintf("  %s = alloca %%struct.Arena, align 8\n", i.Dest))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 0\n", bufSlot, i.Dest))
		sb.WriteString(fmt.Sprintf("  store ptr %s, ptr %s, align 8\n", bufReg, bufSlot))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 1\n", offSlot, i.Dest))
		sb.WriteString(fmt.Sprintf("  store i64 0, ptr %s, align 8\n", offSlot))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 2\n", capSlot, i.Dest))
		sb.WriteString(fmt.Sprintf("  store i64 %d, ptr %s, align 8\n", i.Capacity, capSlot))

	case *ir.InstArenaAlloc:
		g.tempSeq++
		id := g.tempSeq
		bufPtrPtr := fmt.Sprintf("%%buf_ptr_ptr_%d", id)
		bufBase := fmt.Sprintf("%%buf_base_%d", id)
		offPtr := fmt.Sprintf("%%off_ptr_%d", id)
		curOff := fmt.Sprintf("%%cur_off_%d", id)
		capPtr := fmt.Sprintf("%%cap_ptr_%d", id)
		capacity := fmt.Sprintf("%%capacity_%d", id)
		allocI8 := fmt.Sprintf("%%alloc_i8_%d", id)
		nextOff := fmt.Sprintf("%%next_off_%d", id)
		align := i.Align
		if align < 1 {
			align = 1
		}

		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 0\n", bufPtrPtr, i.Arena))
		sb.WriteString(fmt.Sprintf("  %s = load ptr, ptr %s, align 8\n", bufBase, bufPtrPtr))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 1\n", offPtr, i.Arena))
		sb.WriteString(fmt.Sprintf("  %s = load i64, ptr %s, align 8\n", curOff, offPtr))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 2\n", capPtr, i.Arena))
		sb.WriteString(fmt.Sprintf("  %s = load i64, ptr %s, align 8\n", capacity, capPtr))
		sb.WriteString(fmt.Sprintf("  %%arena_offset_valid_%d = icmp ule i64 %s, %s\n", id, curOff, capacity))
		sb.WriteString(fmt.Sprintf("  br i1 %%arena_offset_valid_%d, label %%arena_align_%d, label %%arena_trap_%d\n", id, id, id))
		sb.WriteString(fmt.Sprintf("arena_align_%d:\n", id))
		sb.WriteString(fmt.Sprintf("  %%arena_align_add_%d = add i64 %s, %d\n", id, curOff, align-1))
		sb.WriteString(fmt.Sprintf("  %%arena_aligned_%d = and i64 %%arena_align_add_%d, %d\n", id, id, -align))
		sb.WriteString(fmt.Sprintf("  %%arena_no_wrap_%d = icmp uge i64 %%arena_aligned_%d, %s\n", id, id, curOff))
		sb.WriteString(fmt.Sprintf("  %%arena_aligned_fits_%d = icmp ule i64 %%arena_aligned_%d, %s\n", id, id, capacity))
		sb.WriteString(fmt.Sprintf("  %%arena_align_valid_%d = and i1 %%arena_no_wrap_%d, %%arena_aligned_fits_%d\n", id, id, id))
		sb.WriteString(fmt.Sprintf("  br i1 %%arena_align_valid_%d, label %%arena_check_%d, label %%arena_trap_%d\n", id, id, id))
		sb.WriteString(fmt.Sprintf("arena_check_%d:\n", id))
		sb.WriteString(fmt.Sprintf("  %%arena_remaining_%d = sub i64 %s, %%arena_aligned_%d\n", id, capacity, id))
		sb.WriteString(fmt.Sprintf("  %%arena_fits_%d = icmp ule i64 %s, %%arena_remaining_%d\n", id, i.Size, id))
		sb.WriteString(fmt.Sprintf("  br i1 %%arena_fits_%d, label %%arena_ok_%d, label %%arena_trap_%d\n", id, id, id))
		sb.WriteString(fmt.Sprintf("arena_trap_%d:\n", id))
		sb.WriteString("  call void @llvm.trap()\n  unreachable\n")
		sb.WriteString(fmt.Sprintf("arena_ok_%d:\n", id))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds i8, ptr %s, i64 %%arena_aligned_%d\n", allocI8, bufBase, id))
		sb.WriteString(fmt.Sprintf("  %s = add i64 %%arena_aligned_%d, %s\n", nextOff, id, i.Size))
		sb.WriteString(fmt.Sprintf("  store i64 %s, ptr %s, align 8\n", nextOff, offPtr))
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds i8, ptr %s, i64 0\n", i.Dest, allocI8))

	case *ir.InstArenaReset:
		g.tempSeq++
		resetOffPtr := fmt.Sprintf("%%reset_off_ptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Arena, ptr %s, i32 0, i32 1\n", resetOffPtr, i.Arena))
		sb.WriteString(fmt.Sprintf("  store i64 0, ptr %s, align 8\n", resetOffPtr))

	case *ir.InstAtomicLoad:
		ptrT := llvmType(i.ValType)
		sb.WriteString(fmt.Sprintf("  %s = load atomic %s, ptr %s seq_cst, align 8\n",
			i.Dest, ptrT, i.Ptr))

	case *ir.InstAtomicStore:
		valT := llvmType(i.Val.Type())
		sb.WriteString(fmt.Sprintf("  store atomic %s %s, ptr %s seq_cst, align 8\n",
			valT, i.Val, i.Ptr))

	case *ir.InstAtomicCAS:
		valT := llvmType(i.Expected.Type())
		g.tempSeq++
		pairReg := fmt.Sprintf("%%cas_pair_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = cmpxchg ptr %s, %s %s, %s %s seq_cst seq_cst\n",
			pairReg, i.Ptr, valT, i.Expected, valT, i.Desired))
		sb.WriteString(fmt.Sprintf("  %s = extractvalue { %s, i1 } %s, 1\n",
			i.Dest, valT, pairReg))

	case *ir.InstSyscallWrite:
		g.tempSeq++
		retReg := fmt.Sprintf("%%syscall_ret_%d", g.tempSeq)
		if i.Dest != nil {
			retReg = i.Dest.String()
		}
		fdArg := g.formatSyscallArg(sb, i.Fd)
		bufArg := g.formatSyscallArg(sb, i.Buf)
		lenArg := g.formatSyscallArg(sb, i.Len)
		sb.WriteString(fmt.Sprintf("  %s = call i64 asm sideeffect \"syscall\", \"={rax},{rax},{rdi},{rsi},{rdx},~{rcx},~{r11},~{memory}\"(i64 1, %s, %s, %s)\n",
			retReg, fdArg, bufArg, lenArg))

	case *ir.InstSyscall:
		numArg := g.formatSyscallArg(sb, i.Num)
		a1Arg := g.formatSyscallArg(sb, i.A1)
		a2Arg := g.formatSyscallArg(sb, i.A2)
		a3Arg := g.formatSyscallArg(sb, i.A3)
		dest := i.Dest.String()
		sb.WriteString(fmt.Sprintf("  %s = call i64 asm sideeffect \"syscall\", \"={rax},{rax},{rdi},{rsi},{rdx},~{rcx},~{r11},~{memory}\"(%s, %s, %s, %s)\n",
			dest, numArg, a1Arg, a2Arg, a3Arg))

	case *ir.InstYield:
		sb.WriteString("  call void asm sideeffect \"pause\", \"~{memory}\"()\n")

	case *ir.InstAsm:
		sb.WriteString(fmt.Sprintf("  call void asm sideeffect %q, \"~{dirflag},~{fpsr},~{flags},~{memory}\"()\n", i.Instruction))

	case *ir.InstHlt:
		sb.WriteString("  call void asm sideeffect \"hlt\", \"~{memory}\"()\n")

	case *ir.InstCli:
		sb.WriteString("  call void asm sideeffect \"cli\", \"~{memory}\"()\n")

	case *ir.InstSti:
		sb.WriteString("  call void asm sideeffect \"sti\", \"~{memory}\"()\n")

	case *ir.InstInb:
		sb.WriteString(fmt.Sprintf("  %s = call i8 asm sideeffect \"inb %%dx, %%al\", \"={al},{dx},~{dirflag},~{fpsr},~{flags}\"(i16 %s)\n",
			i.Dest, i.Port))

	case *ir.InstOutb:
		sb.WriteString(fmt.Sprintf("  call void asm sideeffect \"outb %%al, %%dx\", \"{dx},{al},~{dirflag},~{fpsr},~{flags}\"(i16 %s, i8 %s)\n",
			i.Port, i.Val))

	case *ir.InstInw:
		sb.WriteString(fmt.Sprintf("  %s = call i16 asm sideeffect \"inw %%dx, %%ax\", \"={ax},{dx},~{dirflag},~{fpsr},~{flags}\"(i16 %s)\n",
			i.Dest, i.Port))

	case *ir.InstOutw:
		sb.WriteString(fmt.Sprintf("  call void asm sideeffect \"outw %%ax, %%dx\", \"{dx},{ax},~{dirflag},~{fpsr},~{flags}\"(i16 %s, i16 %s)\n",
			i.Port, i.Val))

	case *ir.InstChannelInit:
		g.tempSeq++
		bufReg := fmt.Sprintf("%%ch_buf_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = alloca [%d x i64], align 8\n", bufReg, i.Capacity))
		sb.WriteString(fmt.Sprintf("  %s = alloca %%struct.Channel, align 8\n", i.Dest))
		g.tempSeq++
		gepHead := fmt.Sprintf("%%ch_head_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 0\n", gepHead, i.Dest))
		sb.WriteString(fmt.Sprintf("  store i64 0, ptr %s, align 8\n", gepHead))
		g.tempSeq++
		gepTail := fmt.Sprintf("%%ch_tail_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 1\n", gepTail, i.Dest))
		sb.WriteString(fmt.Sprintf("  store i64 0, ptr %s, align 8\n", gepTail))
		g.tempSeq++
		gepCap := fmt.Sprintf("%%ch_cap_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 2\n", gepCap, i.Dest))
		sb.WriteString(fmt.Sprintf("  store i64 %d, ptr %s, align 8\n", i.Capacity, gepCap))
		g.tempSeq++
		gepBuf := fmt.Sprintf("%%ch_bufptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 3\n", gepBuf, i.Dest))
		g.tempSeq++
		decayBuf := fmt.Sprintf("%%ch_decay_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds [%d x i64], ptr %s, i64 0, i64 0\n", decayBuf, i.Capacity, bufReg))
		sb.WriteString(fmt.Sprintf("  store ptr %s, ptr %s, align 8\n", decayBuf, gepBuf))

	case *ir.InstChannelSend:
		valT := llvmType(i.Val.Type())
		g.tempSeq++
		tailPtr := fmt.Sprintf("%%ch_tail_ptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 1\n", tailPtr, i.Channel))
		g.tempSeq++
		tailVal := fmt.Sprintf("%%ch_tail_val_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = load atomic i64, ptr %s seq_cst, align 8\n", tailVal, tailPtr))
		g.tempSeq++
		bufPtrPtr := fmt.Sprintf("%%ch_buf_pp_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 3\n", bufPtrPtr, i.Channel))
		g.tempSeq++
		bufPtr := fmt.Sprintf("%%ch_buf_p_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = load ptr, ptr %s, align 8\n", bufPtr, bufPtrPtr))
		g.tempSeq++
		elemPtr := fmt.Sprintf("%%ch_elem_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds i64, ptr %s, i64 %s\n", elemPtr, bufPtr, tailVal))
		sb.WriteString(fmt.Sprintf("  store atomic %s %s, ptr %s seq_cst, align 8\n", valT, i.Val, elemPtr))
		g.tempSeq++
		nextTail := fmt.Sprintf("%%ch_next_tail_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = add i64 %s, 1\n", nextTail, tailVal))
		sb.WriteString(fmt.Sprintf("  store atomic i64 %s, ptr %s seq_cst, align 8\n", nextTail, tailPtr))
		sb.WriteString(fmt.Sprintf("  %s = icmp ne i64 1, 0\n", i.Dest))

	case *ir.InstChannelRecv:
		optionType := llvmType(i.Dest.Type())
		g.tempSeq++
		resultSlot := fmt.Sprintf("%%ch_result_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = alloca %s, align 8\n", resultSlot, optionType))
		sb.WriteString(fmt.Sprintf("  store %s zeroinitializer, ptr %s, align 8\n", optionType, resultSlot))
		g.tempSeq++
		headPtr := fmt.Sprintf("%%ch_head_ptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 0\n", headPtr, i.Channel))
		g.tempSeq++
		headVal := fmt.Sprintf("%%ch_head_val_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = load atomic i64, ptr %s seq_cst, align 8\n", headVal, headPtr))
		g.tempSeq++
		tailPtr := fmt.Sprintf("%%ch_recv_tail_ptr_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 1\n", tailPtr, i.Channel))
		g.tempSeq++
		tailVal := fmt.Sprintf("%%ch_recv_tail_val_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = load atomic i64, ptr %s seq_cst, align 8\n", tailVal, tailPtr))
		g.tempSeq++
		hasValue := fmt.Sprintf("%%ch_has_value_%d", g.tempSeq)
		readLabel := fmt.Sprintf("ch_read_%d", g.tempSeq)
		endLabel := fmt.Sprintf("ch_read_end_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = icmp ult i64 %s, %s\n", hasValue, headVal, tailVal))
		sb.WriteString(fmt.Sprintf("  br i1 %s, label %%%s, label %%%s\n", hasValue, readLabel, endLabel))
		sb.WriteString(readLabel + ":\n")
		g.tempSeq++
		bufPtrPtr := fmt.Sprintf("%%ch_buf_pp_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds %%struct.Channel, ptr %s, i32 0, i32 3\n", bufPtrPtr, i.Channel))
		g.tempSeq++
		bufPtr := fmt.Sprintf("%%ch_buf_p_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = load ptr, ptr %s, align 8\n", bufPtr, bufPtrPtr))
		g.tempSeq++
		elemPtr := fmt.Sprintf("%%ch_elem_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds i64, ptr %s, i64 %s\n", elemPtr, bufPtr, headVal))
		g.tempSeq++
		rawValue := fmt.Sprintf("%%ch_raw_value_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = load atomic i32, ptr %s seq_cst, align 8\n", rawValue, elemPtr))
		g.tempSeq++
		nextHead := fmt.Sprintf("%%ch_next_head_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = add i64 %s, 1\n", nextHead, headVal))
		sb.WriteString(fmt.Sprintf("  store atomic i64 %s, ptr %s seq_cst, align 8\n", nextHead, headPtr))
		g.tempSeq++
		tagPtr := fmt.Sprintf("%%ch_option_tag_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr %s, ptr %s, i32 0, i32 0\n", tagPtr, optionType, resultSlot))
		sb.WriteString(fmt.Sprintf("  store i64 1, ptr %s, align 8\n", tagPtr))
		g.tempSeq++
		payloadPtr := fmt.Sprintf("%%ch_option_payload_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr %s, ptr %s, i32 0, i32 1\n", payloadPtr, optionType, resultSlot))
		sb.WriteString(fmt.Sprintf("  store i32 %s, ptr %s, align 1\n", rawValue, payloadPtr))
		sb.WriteString(fmt.Sprintf("  br label %%%s\n", endLabel))
		sb.WriteString(endLabel + ":\n")
		sb.WriteString(fmt.Sprintf("  %s = load %s, ptr %s, align 8\n", i.Dest, optionType, resultSlot))
	}
}

func llvmType(t types.Type) string {
	if t == nil {
		return "void"
	}

	switch ty := t.(type) {
	case *types.BasicType:
		switch ty.Kind {
		case types.KindVoid:
			return "void"
		case types.KindBool:
			return "i1"
		case types.KindU8, types.KindI8:
			return "i8"
		case types.KindU16, types.KindI16:
			return "i16"
		case types.KindU32, types.KindI32, types.KindUntypedInt:
			return "i32"
		case types.KindU64, types.KindI64, types.KindUsize, types.KindIsize:
			return "i64"
		case types.KindF32:
			return "float"
		case types.KindF64, types.KindUntypedFloat:
			return "double"
		case types.KindString:
			return "ptr"
		default:
			return "i32"
		}

	case *types.PointerType:
		_ = ty
		return "ptr"

	case *types.SliceType:
		return "{ ptr, i64 }"

	case *types.StructType:
		return "%struct." + ty.Name

	case *types.ArenaType:
		return "ptr"

	case *types.ChannelType:
		return "ptr"

	case *types.AllocatorType:
		return "%struct.Allocator"

	case *types.OptionType:
		return fmt.Sprintf("{ i64, [%d x i8] }", ty.Elem.Size())

	case *types.ResultType:
		payloadSize := ty.OkType.Size()
		if ty.ErrType.Size() > payloadSize {
			payloadSize = ty.ErrType.Size()
		}
		return fmt.Sprintf("{ i64, [%d x i8] }", payloadSize)

	default:
		return "i32"
	}
}

func isIntegerLLVMType(t types.Type) bool {
	bt, ok := t.(*types.BasicType)
	if !ok {
		return false
	}
	switch bt.Kind {
	case types.KindBool,
		types.KindU8, types.KindU16, types.KindU32, types.KindU64, types.KindUsize,
		types.KindI8, types.KindI16, types.KindI32, types.KindI64, types.KindIsize,
		types.KindUntypedInt:
		return true
	}
	return false
}

func isSignedInteger(t types.Type) bool {
	bt, ok := t.(*types.BasicType)
	if !ok {
		return false
	}
	switch bt.Kind {
	case types.KindI8, types.KindI16, types.KindI32, types.KindI64, types.KindIsize:
		return true
	}
	return false
}

func isPointerLLVMType(t types.Type) bool {
	if _, ok := t.(*types.PointerType); ok {
		return true
	}
	if b, ok := t.(*types.BasicType); ok && b.Kind == types.KindString {
		return true
	}
	if _, ok := t.(*types.ArenaType); ok {
		return true
	}
	if _, ok := t.(*types.ChannelType); ok {
		return true
	}
	return false
}

func castOp(from, to types.Type) string {
	fromIsInt := isIntegerLLVMType(from)
	toIsInt := isIntegerLLVMType(to)
	fromIsPtr := isPointerLLVMType(from)
	toIsPtr := isPointerLLVMType(to)

	switch {
	case fromIsInt && toIsPtr:
		return "inttoptr"
	case fromIsPtr && toIsInt:
		return "ptrtoint"
	case fromIsInt && toIsInt:
		fromBits := from.Size() * 8
		toBits := to.Size() * 8
		if from == types.Bool {
			fromBits = 1
		}
		if to == types.Bool {
			toBits = 1
		}
		if fromBits < toBits {
			if isSignedInteger(from) {
				return "sext"
			}
			return "zext"
		} else if fromBits > toBits {
			return "trunc"
		}
		return "bitcast"
	default:
		return "bitcast"
	}
}

func mapLLVMOp(op string, operandType types.Type) string {
	signed := isSignedInteger(operandType)
	switch op {
	case "add":
		return "add"
	case "sub":
		return "sub"
	case "mul":
		return "mul"
	case "sdiv":
		if signed {
			return "sdiv"
		}
		return "udiv"
	case "srem":
		if signed {
			return "srem"
		}
		return "urem"
	case "and":
		return "and"
	case "or":
		return "or"
	case "xor":
		return "xor"
	case "shl":
		return "shl"
	case "ashr":
		if signed {
			return "ashr"
		}
		return "lshr"
	case "eq":
		return "icmp eq"
	case "ne":
		return "icmp ne"
	case "slt":
		if signed {
			return "icmp slt"
		}
		return "icmp ult"
	case "sle":
		if signed {
			return "icmp sle"
		}
		return "icmp ule"
	case "sgt":
		if signed {
			return "icmp sgt"
		}
		return "icmp ugt"
	case "sge":
		if signed {
			return "icmp sge"
		}
		return "icmp uge"
	default:
		return "add"
	}
}

func (g *Generator) formatSyscallArg(sb *strings.Builder, val ir.Value) string {
	if cs, ok := val.(*ir.ConstString); ok {
		sym := g.strLiterals[cs.Val]
		size := g.strSizes[cs.Val]
		g.tempSeq++
		ptrReg := fmt.Sprintf("%%str_ptr_%d", g.tempSeq)
		g.tempSeq++
		intReg := fmt.Sprintf("%%str_int_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = getelementptr inbounds [%d x i8], ptr %s, i64 0, i64 0\n", ptrReg, size, sym))
		sb.WriteString(fmt.Sprintf("  %s = ptrtoint ptr %s to i64\n", intReg, ptrReg))
		return fmt.Sprintf("i64 %s", intReg)
	}
	if isPointerLLVMType(val.Type()) {
		g.tempSeq++
		intReg := fmt.Sprintf("%%buf_int_%d", g.tempSeq)
		sb.WriteString(fmt.Sprintf("  %s = ptrtoint ptr %s to i64\n", intReg, val))
		return fmt.Sprintf("i64 %s", intReg)
	}
	t := llvmType(val.Type())
	if isIntegerLLVMType(val.Type()) && t != "i64" {
		g.tempSeq++
		extReg := fmt.Sprintf("%%ext_%d", g.tempSeq)
		op := "zext"
		if isSignedInteger(val.Type()) {
			op = "sext"
		}
		sb.WriteString(fmt.Sprintf("  %s = %s %s %s to i64\n", extReg, op, t, val))
		return fmt.Sprintf("i64 %s", extReg)
	}
	return fmt.Sprintf("i64 %s", val)
}
