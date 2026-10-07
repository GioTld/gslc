package types

import "testing"

func TestPrimitiveTypes(t *testing.T) {
	if U32.Size() != 4 || U32.Align() != 4 {
		t.Errorf("expected u32 size 4, got %d", U32.Size())
	}
	if U64.Size() != 8 || U64.Align() != 8 {
		t.Errorf("expected u64 size 8, got %d", U64.Size())
	}
	if Bool.Size() != 1 {
		t.Errorf("expected bool size 1, got %d", Bool.Size())
	}
}

func TestStructLayout(t *testing.T) {
	// struct { a: u8, b: u32, c: u8 } -> 1 byte + 3 padding + 4 bytes + 1 byte + 3 tail padding = 12 bytes
	st := NewStructType("TestStruct", []StructField{
		{Name: "a", Type: U8},
		{Name: "b", Type: U32},
		{Name: "c", Type: U8},
	})

	if st.Size() != 12 {
		t.Errorf("expected struct size 12 with padding, got %d", st.Size())
	}
	if st.Align() != 4 {
		t.Errorf("expected struct align 4, got %d", st.Align())
	}
	if st.Fields[1].Offset != 4 {
		t.Errorf("expected field b offset 4, got %d", st.Fields[1].Offset)
	}
	if st.Fields[2].Offset != 8 {
		t.Errorf("expected field c offset 8, got %d", st.Fields[2].Offset)
	}
}

func TestAlgebraicTypes(t *testing.T) {
	res := &ResultType{OkType: U32, ErrType: U64}
	if res.Size() != 16 { // 8 byte tag + 8 byte max payload
		t.Errorf("expected Result size 16, got %d", res.Size())
	}

	opt := &OptionType{Elem: U32}
	if opt.Size() != 16 { // 8 byte tag + 4 byte payload, rounded to 8 byte alignment
		t.Errorf("expected Option size 16, got %d", opt.Size())
	}
}

func TestPackedStructLayout(t *testing.T) {
	// packed struct { a: u8, b: u32, c: u8 } -> 1 + 4 + 1 = 6 bytes, no padding, align=1
	st := NewPackedStructType("GDTEntry", []StructField{
		{Name: "a", Type: U8},
		{Name: "b", Type: U32},
		{Name: "c", Type: U8},
	})

	if st.Size() != 6 {
		t.Errorf("expected packed struct size 6, got %d", st.Size())
	}
	if st.Align() != 1 {
		t.Errorf("expected packed struct align 1, got %d", st.Align())
	}
	if st.Fields[0].Offset != 0 {
		t.Errorf("expected field a offset 0, got %d", st.Fields[0].Offset)
	}
	if st.Fields[1].Offset != 1 {
		t.Errorf("expected field b offset 1, got %d", st.Fields[1].Offset)
	}
	if st.Fields[2].Offset != 5 {
		t.Errorf("expected field c offset 5, got %d", st.Fields[2].Offset)
	}
	if !st.IsPacked {
		t.Errorf("expected IsPacked=true")
	}
}
