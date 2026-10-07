package types

import "fmt"

// Type represents a static data type in GSL.
type Type interface {
	String() string
	Equals(other Type) bool
	Size() int  // in bytes
	Align() int // in bytes
}

// BasicKind enumerates built-in primitive types.
type BasicKind int

const (
	KindVoid BasicKind = iota
	KindBool
	KindU8
	KindU16
	KindU32
	KindU64
	KindUsize
	KindI8
	KindI16
	KindI32
	KindI64
	KindIsize
	KindF32
	KindF64
	KindString
	KindUntypedInt
	KindUntypedFloat
)

type BasicType struct {
	Kind BasicKind
	Name string
}

func (b *BasicType) String() string { return b.Name }

func (b *BasicType) Equals(other Type) bool {
	if ob, ok := other.(*BasicType); ok {
		return b.Kind == ob.Kind
	}
	return false
}

func (b *BasicType) Size() int {
	switch b.Kind {
	case KindVoid:
		return 0
	case KindBool, KindU8, KindI8:
		return 1
	case KindU16, KindI16:
		return 2
	case KindU32, KindI32, KindF32:
		return 4
	case KindU64, KindI64, KindF64, KindUsize, KindIsize:
		return 8
	case KindString:
		return 8 // pointer to NUL-terminated bytes in the bootstrap profile
	default:
		return 8
	}
}

func (b *BasicType) Align() int {
	return b.Size()
}

// Built-in singletons
var (
	Void         = &BasicType{Kind: KindVoid, Name: "void"}
	Bool         = &BasicType{Kind: KindBool, Name: "bool"}
	U8           = &BasicType{Kind: KindU8, Name: "u8"}
	U16          = &BasicType{Kind: KindU16, Name: "u16"}
	U32          = &BasicType{Kind: KindU32, Name: "u32"}
	U64          = &BasicType{Kind: KindU64, Name: "u64"}
	Usize        = &BasicType{Kind: KindUsize, Name: "usize"}
	I8           = &BasicType{Kind: KindI8, Name: "i8"}
	I16          = &BasicType{Kind: KindI16, Name: "i16"}
	I32          = &BasicType{Kind: KindI32, Name: "i32"}
	I64          = &BasicType{Kind: KindI64, Name: "i64"}
	Isize        = &BasicType{Kind: KindIsize, Name: "isize"}
	F32          = &BasicType{Kind: KindF32, Name: "f32"}
	F64          = &BasicType{Kind: KindF64, Name: "f64"}
	String       = &BasicType{Kind: KindString, Name: "string"}
	UntypedInt   = &BasicType{Kind: KindUntypedInt, Name: "untyped int"}
	UntypedFloat = &BasicType{Kind: KindUntypedFloat, Name: "untyped float"}
)

// IsInteger returns true if t is a signed or unsigned integer type, or UntypedInt.
func IsInteger(t Type) bool {
	if b, ok := t.(*BasicType); ok {
		switch b.Kind {
		case KindU8, KindU16, KindU32, KindU64, KindUsize,
			KindI8, KindI16, KindI32, KindI64, KindIsize,
			KindUntypedInt:
			return true
		}
	}
	return false
}

// PointerType represents a raw machine pointer (*T).
type PointerType struct {
	Elem  Type
	IsMut bool
}

func (p *PointerType) String() string {
	if p.IsMut {
		return "*" + p.Elem.String()
	}
	return "*const " + p.Elem.String()
}

func (p *PointerType) Equals(other Type) bool {
	if op, ok := other.(*PointerType); ok {
		return p.IsMut == op.IsMut && p.Elem.Equals(op.Elem)
	}
	return false
}

func (p *PointerType) Size() int  { return 8 }
func (p *PointerType) Align() int { return 8 }

// SliceType represents a slice ([]T) with a pointer and length.
type SliceType struct {
	Elem Type
}

func (s *SliceType) String() string { return "[]" + s.Elem.String() }

func (s *SliceType) Equals(other Type) bool {
	if os, ok := other.(*SliceType); ok {
		return s.Elem.Equals(os.Elem)
	}
	return false
}

func (s *SliceType) Size() int  { return 16 } // ptr (8) + len (8)
func (s *SliceType) Align() int { return 8 }

// StructField represents a named field in a struct type.
type StructField struct {
	Name   string
	Type   Type
	Offset int
}

// StructType represents a compound structure layout.
type StructType struct {
	Name     string
	Fields   []StructField
	IsPacked bool
	IsCopy   bool
	size     int
	align    int
}

func NewStructType(name string, fields []StructField) *StructType {
	st := &StructType{Name: name, Fields: fields}
	st.computeLayout()
	return st
}

func NewPackedStructType(name string, fields []StructField) *StructType {
	st := &StructType{Name: name, Fields: fields, IsPacked: true}
	st.computeLayout()
	return st
}

func (s *StructType) computeLayout() {
	offset := 0
	if s.IsPacked {
		for i := range s.Fields {
			s.Fields[i].Offset = offset
			offset += s.Fields[i].Type.Size()
		}
		s.size = offset
		s.align = 1
		return
	}

	maxAlign := 1
	for i := range s.Fields {
		fAlign := s.Fields[i].Type.Align()
		if fAlign > maxAlign {
			maxAlign = fAlign
		}
		if rem := offset % fAlign; rem != 0 {
			offset += fAlign - rem
		}
		s.Fields[i].Offset = offset
		offset += s.Fields[i].Type.Size()
	}
	if rem := offset % maxAlign; rem != 0 {
		offset += maxAlign - rem
	}
	s.size = offset
	s.align = maxAlign
}

func (s *StructType) String() string { return s.Name }

func (s *StructType) Equals(other Type) bool {
	if os, ok := other.(*StructType); ok {
		return s.Name == os.Name
	}
	return false
}

func (s *StructType) Size() int  { return s.size }
func (s *StructType) Align() int { return s.align }

// FuncType represents a function signature.
type FuncType struct {
	Params     []Type
	ReturnType Type
}

func (f *FuncType) String() string {
	return fmt.Sprintf("func(%d params) %s", len(f.Params), f.ReturnType)
}

func (f *FuncType) Equals(other Type) bool {
	if of, ok := other.(*FuncType); ok {
		if len(f.Params) != len(of.Params) {
			return false
		}
		for i := range f.Params {
			if !f.Params[i].Equals(of.Params[i]) {
				return false
			}
		}
		return f.ReturnType.Equals(of.ReturnType)
	}
	return false
}

func (f *FuncType) Size() int  { return 8 }
func (f *FuncType) Align() int { return 8 }

// ResultType represents Result[T, E].
type ResultType struct {
	OkType  Type
	ErrType Type
}

func (r *ResultType) String() string {
	return fmt.Sprintf("Result[%s, %s]", r.OkType, r.ErrType)
}

func (r *ResultType) Equals(other Type) bool {
	if or, ok := other.(*ResultType); ok {
		return r.OkType.Equals(or.OkType) && r.ErrType.Equals(or.ErrType)
	}
	return false
}

func (r *ResultType) Size() int {
	maxVal := r.OkType.Size()
	if r.ErrType.Size() > maxVal {
		maxVal = r.ErrType.Size()
	}
	return alignUp(8+maxVal, 8)
}

func (r *ResultType) Align() int { return 8 }

// OptionType represents Option[T].
type OptionType struct {
	Elem Type
}

func (o *OptionType) String() string {
	return fmt.Sprintf("Option[%s]", o.Elem)
}

func (o *OptionType) Equals(other Type) bool {
	if oo, ok := other.(*OptionType); ok {
		return o.Elem.Equals(oo.Elem)
	}
	return false
}

func (o *OptionType) Size() int  { return alignUp(8+o.Elem.Size(), 8) }
func (o *OptionType) Align() int { return 8 }

func alignUp(size, align int) int {
	return (size + align - 1) / align * align
}

// ArenaType represents an explicit memory arena instance.
type ArenaType struct {
	Name     string
	Capacity int64
}

func (a *ArenaType) String() string { return "arena(" + a.Name + ")" }

func (a *ArenaType) Equals(other Type) bool {
	if oa, ok := other.(*ArenaType); ok {
		return a.Name == oa.Name
	}
	return false
}

func (a *ArenaType) Size() int  { return 24 } // buffer_ptr (8) + offset (8) + capacity (8)
func (a *ArenaType) Align() int { return 8 }

// AllocatorType represents the parametric memory allocator interface.
type AllocatorType struct {
	Name string
}

func (a *AllocatorType) String() string { return "Allocator" }

func (a *AllocatorType) Equals(other Type) bool {
	_, ok := other.(*AllocatorType)
	return ok
}

func (a *AllocatorType) Size() int  { return 24 } // ctx ptr (8) + alloc_fn (8) + free_fn (8)
func (a *AllocatorType) Align() int { return 8 }

// ChannelType represents a strongly-typed concurrency channel Channel[T].
type ChannelType struct {
	Elem Type
}

func (c *ChannelType) String() string { return fmt.Sprintf("Channel[%s]", c.Elem) }

func (c *ChannelType) Equals(other Type) bool {
	if oc, ok := other.(*ChannelType); ok {
		return c.Elem.Equals(oc.Elem)
	}
	return false
}

func (c *ChannelType) Size() int  { return 32 } // head (8) + tail (8) + capacity (8) + buffer ptr (8)
func (c *ChannelType) Align() int { return 8 }

// Allocator is the singleton instance of AllocatorType.
var Allocator = &AllocatorType{Name: "Allocator"}

// LookupPrimitive returns the built-in primitive type by name, or nil.
func LookupPrimitive(name string) Type {
	switch name {
	case "void":
		return Void
	case "bool":
		return Bool
	case "u8":
		return U8
	case "u16":
		return U16
	case "u32":
		return U32
	case "u64":
		return U64
	case "usize":
		return Usize
	case "i8":
		return I8
	case "i16":
		return I16
	case "i32":
		return I32
	case "i64":
		return I64
	case "isize":
		return Isize
	case "f32":
		return F32
	case "f64":
		return F64
	case "string":
		return String
	case "Allocator":
		return Allocator
	default:
		return nil
	}
}
