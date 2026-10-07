package sema

import (
	"strings"
	"testing"
)

func TestScopedArenaValidUsage(t *testing.T) {
	input := `
struct Packet {
    id: u32,
    len: u32
}

func process() u32 {
    arena local_pool(4096)
    let p = local_pool.alloc(Packet)
    return 42
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		for _, d := range sem.Diagnostics() {
			t.Log(d.Format(input))
		}
		t.Fatalf("expected 0 diagnostics for valid scoped arena usage, got %d", len(sem.Diagnostics()))
	}
}

func TestScopedArenaReturnEscape(t *testing.T) {
	input := `
struct Packet {
    id: u32
}

func leak() *Packet {
    arena local_pool(1024)
    let p = local_pool.alloc(Packet)
    return p
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) == 0 {
		t.Fatalf("expected escape analysis error returning scoped arena reference, got 0")
	}

	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "cannot return reference \"p\" allocated from scoped arena \"local_pool\"") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected return escape error message, got: %v", sem.Diagnostics())
	}
}

func TestScopedArenaDirectReturnEscape(t *testing.T) {
	input := `
struct Packet {
    id: u32
}

func leak_direct() *Packet {
    arena local_pool(1024)
    return local_pool.alloc(Packet)
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) == 0 {
		t.Fatalf("expected escape analysis error returning direct arena alloc, got 0")
	}

	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "cannot return reference \"value\" allocated from scoped arena \"local_pool\"") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected direct return escape error message, got: %v", sem.Diagnostics())
	}
}

func TestScopedArenaOuterAssignmentEscape(t *testing.T) {
	input := `
struct Packet {
    id: u32
}

func escape_assignment() {
    var outer_p: *Packet
    if true {
        arena inner_pool(1024)
        let local_p = inner_pool.alloc(Packet)
        outer_p = local_p
    }
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) == 0 {
		t.Fatalf("expected escape analysis error assigning inner arena reference to outer variable, got 0")
	}

	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "cannot assign reference from inner arena \"inner_pool\" to outer scope variable \"outer_p\"") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected assignment escape error message, got: %v", sem.Diagnostics())
	}
}

func TestScopedArenaGlobalReturnAllowed(t *testing.T) {
	input := `
arena global_pool(64 * 1024)

struct Node {
    val: u64
}

func create_node() *Node {
    let n = global_pool.alloc(Node)
    return n
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		for _, d := range sem.Diagnostics() {
			t.Log(d.Format(input))
		}
		t.Fatalf("expected 0 diagnostics for returning reference from global arena, got %d", len(sem.Diagnostics()))
	}
}

func TestAtomicPrimitivesRequireUnsafe(t *testing.T) {
	input := `
func bad() {
    let addr: usize = 0x1000
    let ptr = addr as *u64
    let v = atomic_load(ptr)
}
`
	sem := parseAndAnalyze(input)
	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "atomic_load") && strings.Contains(d.Message, "unsafe") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unsafe error for atomic_load outside unsafe block, got: %v", sem.Diagnostics())
	}
}

func TestAtomicPrimitivesValidInUnsafe(t *testing.T) {
	input := `
func ok_atomics() {
    unsafe {
        let addr: usize = 0x1000
        let ptr = addr as *u64
        atomic_store(ptr, 42 as u64)
        let v = atomic_load(ptr)
        let ok = atomic_cas(ptr, 42 as u64, 99 as u64)
    }
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		for _, d := range sem.Diagnostics() {
			t.Log(d.Format(input))
		}
		t.Fatalf("expected 0 diagnostics for valid atomics in unsafe, got %d", len(sem.Diagnostics()))
	}
}

