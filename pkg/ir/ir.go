package ir

import (
	"fmt"
	"strings"

	"github.com/GioTld/gslc/pkg/types"
)

// Value represents an SSA value or immediate constant.
type Value interface {
	Type() types.Type
	String() string
}

type Reg struct {
	ID      int
	ValType types.Type
}

func (r *Reg) Type() types.Type { return r.ValType }
func (r *Reg) String() string   { return fmt.Sprintf("%%t%d", r.ID) }

type ConstInt struct {
	Val     int64
	ValType types.Type
}

func (c *ConstInt) Type() types.Type { return c.ValType }
func (c *ConstInt) String() string {
	if c.ValType == types.String && c.Val == 0 {
		return "null"
	}
	if _, ok := c.ValType.(*types.PointerType); ok && c.Val == 0 {
		return "null"
	}
	return fmt.Sprintf("%d", c.Val)
}

type ConstBool struct {
	Val bool
}

func (c *ConstBool) Type() types.Type { return types.Bool }
func (c *ConstBool) String() string   { return fmt.Sprintf("%t", c.Val) }

type ConstString struct {
	Val string
}

func (c *ConstString) Type() types.Type { return types.String }
func (c *ConstString) String() string   { return fmt.Sprintf("%q", c.Val) }

// Instruction represents an atomic TAC operation.
type Instruction interface {
	String() string
}

type InstAlloca struct {
	Dest     *Reg
	ElemType types.Type
}

type InstStringAddr struct {
	Dest    *Reg
	Literal *ConstString
}

func (i *InstStringAddr) String() string {
	return fmt.Sprintf("  %s = string_addr %s", i.Dest, i.Literal)
}

func (i *InstAlloca) String() string {
	return fmt.Sprintf("  %s = alloca %s", i.Dest, i.ElemType)
}

type InstLoad struct {
	Dest    *Reg
	SrcPtr  Value
	ValType types.Type
}

func (i *InstLoad) String() string {
	return fmt.Sprintf("  %s = load %s, %s", i.Dest, i.ValType, i.SrcPtr)
}

type InstStore struct {
	Val     Value
	DestPtr Value
}

func (i *InstStore) String() string {
	return fmt.Sprintf("  store %s, %s", i.Val, i.DestPtr)
}

type InstBinary struct {
	Dest  *Reg
	Op    string
	Left  Value
	Right Value
}

func (i *InstBinary) String() string {
	return fmt.Sprintf("  %s = %s %s, %s", i.Dest, i.Op, i.Left, i.Right)
}

type InstUnary struct {
	Dest    *Reg
	Op      string
	Operand Value
}

func (i *InstUnary) String() string {
	return fmt.Sprintf("  %s = %s %s", i.Dest, i.Op, i.Operand)
}

type InstCall struct {
	Dest   *Reg // nil if void
	Func   string
	Args   []Value
	RetTyp types.Type
}

func (i *InstCall) String() string {
	args := make([]string, len(i.Args))
	for idx, a := range i.Args {
		args[idx] = a.String()
	}
	if i.Dest != nil {
		return fmt.Sprintf("  %s = call @%s(%s)", i.Dest, i.Func, strings.Join(args, ", "))
	}
	return fmt.Sprintf("  call @%s(%s)", i.Func, strings.Join(args, ", "))
}

type InstReturn struct {
	Val Value // nil if void
}

type InstUnreachable struct{}

func (i *InstUnreachable) String() string { return "  unreachable" }

type InstVariantMake struct {
	Dest    *Reg
	Tag     int
	Payload Value
}

func (i *InstVariantMake) String() string {
	return fmt.Sprintf("  %s = variant tag %d", i.Dest, i.Tag)
}

type InstVariantTag struct {
	Dest  *Reg
	Value Value
}

func (i *InstVariantTag) String() string {
	return fmt.Sprintf("  %s = variant_tag %s", i.Dest, i.Value)
}

type InstVariantPayload struct {
	Dest  *Reg
	Value Value
}

func (i *InstVariantPayload) String() string {
	return fmt.Sprintf("  %s = variant_payload %s", i.Dest, i.Value)
}

func (i *InstReturn) String() string {
	if i.Val != nil {
		return fmt.Sprintf("  ret %s", i.Val)
	}
	return "  ret void"
}

type InstBranch struct {
	Target string
}

func (i *InstBranch) String() string {
	return fmt.Sprintf("  br label %%%s", i.Target)
}

type InstBranchCond struct {
	Cond       Value
	TrueLabel  string
	FalseLabel string
}

func (i *InstBranchCond) String() string {
	return fmt.Sprintf("  br %s, label %%%s, label %%%s", i.Cond, i.TrueLabel, i.FalseLabel)
}

type InstCast struct {
	Dest   *Reg
	Source Value
	ToType types.Type
}

func (i *InstCast) String() string {
	return fmt.Sprintf("  %s = cast %s to %s", i.Dest, i.Source, i.ToType)
}

type InstVolatileStore struct {
	Val     Value
	DestPtr Value
}

func (i *InstVolatileStore) String() string {
	return fmt.Sprintf("  store volatile %s, %s", i.Val, i.DestPtr)
}

type InstVolatileLoad struct {
	Dest    *Reg
	SrcPtr  Value
	ValType types.Type
}

func (i *InstVolatileLoad) String() string {
	return fmt.Sprintf("  %s = load volatile %s, %s", i.Dest, i.ValType, i.SrcPtr)
}

type InstArenaInit struct {
	Dest     *Reg
	Name     string
	Capacity int64
}

func (i *InstArenaInit) String() string {
	return fmt.Sprintf("  %s = arena_init %s, %d", i.Dest, i.Name, i.Capacity)
}

type InstArenaAlloc struct {
	Dest    *Reg
	Arena   Value
	Size    Value
	Align   int
	ValType types.Type
}

func (i *InstArenaAlloc) String() string {
	return fmt.Sprintf("  %s = arena_alloc %s, size=%s, align=%d", i.Dest, i.Arena, i.Size, i.Align)
}

type InstArenaReset struct {
	Arena Value
}

func (i *InstArenaReset) String() string {
	return fmt.Sprintf("  arena_reset %s", i.Arena)
}

type InstAtomicLoad struct {
	Dest    *Reg
	Ptr     Value
	ValType types.Type
}

func (i *InstAtomicLoad) String() string {
	return fmt.Sprintf("  %s = atomic_load %s, %s", i.Dest, i.ValType, i.Ptr)
}

type InstAtomicStore struct {
	Val Value
	Ptr Value
}

func (i *InstAtomicStore) String() string {
	return fmt.Sprintf("  atomic_store %s, %s", i.Val, i.Ptr)
}

type InstAtomicCAS struct {
	Dest     *Reg
	Ptr      Value
	Expected Value
	Desired  Value
}

func (i *InstAtomicCAS) String() string {
	return fmt.Sprintf("  %s = atomic_cas %s, %s, %s", i.Dest, i.Ptr, i.Expected, i.Desired)
}

type InstGetFieldPtr struct {
	Dest       *Reg
	StructPtr  Value
	StructName string
	FieldIndex int
	FieldType  types.Type
}

func (i *InstGetFieldPtr) String() string {
	return fmt.Sprintf("  %s = getfieldptr %%%s, %s, %d", i.Dest, i.StructName, i.StructPtr, i.FieldIndex)
}

type InstSyscallWrite struct {
	Dest *Reg
	Fd   Value
	Buf  Value
	Len  Value
}

func (i *InstSyscallWrite) String() string {
	return fmt.Sprintf("  syscall_write %s, %s, %s", i.Fd, i.Buf, i.Len)
}

type InstSyscall struct {
	Dest *Reg
	Num  Value
	A1   Value
	A2   Value
	A3   Value
}

func (i *InstSyscall) String() string {
	if i.Dest != nil {
		return fmt.Sprintf("  %s = syscall %s, %s, %s, %s", i.Dest, i.Num, i.A1, i.A2, i.A3)
	}
	return fmt.Sprintf("  syscall %s, %s, %s, %s", i.Num, i.A1, i.A2, i.A3)
}

type InstYield struct{}

func (i *InstYield) String() string {
	return "  yield"
}

type InstAsm struct {
	Instruction string
}

func (i *InstAsm) String() string {
	return fmt.Sprintf("  asm(%q)", i.Instruction)
}

type InstInb struct {
	Dest *Reg
	Port Value
}

func (i *InstInb) String() string {
	return fmt.Sprintf("  %s = inb %s", i.Dest, i.Port)
}

type InstOutb struct {
	Port Value
	Val  Value
}

func (i *InstOutb) String() string {
	return fmt.Sprintf("  outb %s, %s", i.Port, i.Val)
}

type InstInw struct {
	Dest *Reg
	Port Value
}

func (i *InstInw) String() string {
	return fmt.Sprintf("  %s = inw %s", i.Dest, i.Port)
}

type InstOutw struct {
	Port Value
	Val  Value
}

func (i *InstOutw) String() string {
	return fmt.Sprintf("  outw %s, %s", i.Port, i.Val)
}

type InstHlt struct{}

func (i *InstHlt) String() string { return "  hlt" }

type InstCli struct{}

func (i *InstCli) String() string { return "  cli" }

type InstSti struct{}

func (i *InstSti) String() string { return "  sti" }

type InstLoadDescriptorTable struct {
	Kind string
	Ptr  Value
}

func (i *InstLoadDescriptorTable) String() string {
	return fmt.Sprintf("  %s %s", i.Kind, i.Ptr)
}

type InstSymbolAddress struct {
	Dest *Reg
	Name string
}

func (i *InstSymbolAddress) String() string {
	return fmt.Sprintf("  %s = symbol_address %s", i.Dest, i.Name)
}

type InstChannelInit struct {
	Dest     *Reg
	Capacity int64
}

func (i *InstChannelInit) String() string {
	return fmt.Sprintf("  %s = channel_init cap=%d", i.Dest, i.Capacity)
}

type InstChannelSend struct {
	Dest    *Reg
	Channel Value
	Val     Value
}

func (i *InstChannelSend) String() string {
	return fmt.Sprintf("  %s = channel_send %s, %s", i.Dest, i.Channel, i.Val)
}

type InstChannelRecv struct {
	Dest    *Reg
	Channel Value
}

func (i *InstChannelRecv) String() string {
	return fmt.Sprintf("  %s = channel_recv %s", i.Dest, i.Channel)
}

// BasicBlock represents a linear sequence of instructions ending in a terminator.
type BasicBlock struct {
	Name  string
	Insts []Instruction
}

func (b *BasicBlock) String() string {
	var sb strings.Builder
	sb.WriteString(b.Name + ":\n")
	for _, inst := range b.Insts {
		sb.WriteString(inst.String() + "\n")
	}
	return sb.String()
}

// Function represents a compiled GSL function.
type Function struct {
	Name       string
	Params     []*Reg
	ReturnType types.Type
	Blocks     []*BasicBlock
}

func (f *Function) String() string {
	var sb strings.Builder
	params := make([]string, len(f.Params))
	for i, p := range f.Params {
		params[i] = fmt.Sprintf("%s: %s", p, p.Type())
	}
	sb.WriteString(fmt.Sprintf("define @%s(%s) %s {\n", f.Name, strings.Join(params, ", "), f.ReturnType))
	for _, b := range f.Blocks {
		sb.WriteString(b.String())
	}
	sb.WriteString("}\n")
	return sb.String()
}

// Module represents the complete IR container for a program.
type Module struct {
	Name      string
	Functions []*Function
	Structs   []*types.StructType
}

func (m *Module) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("; Module: %s\n\n", m.Name))
	for _, f := range m.Functions {
		sb.WriteString(f.String() + "\n")
	}
	return sb.String()
}
