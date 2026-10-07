package sema

import (
	"strings"
	"testing"
)

func TestConstDeclarationAndUsage(t *testing.T) {
	input := `
const BUFFER_SIZE: usize = 4096
const CHUNK_SIZE = 512

func calculate_chunks() usize {
    return BUFFER_SIZE / CHUNK_SIZE
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics, got: %v", sem.Diagnostics())
	}
}

func TestComptimeArenaSizing(t *testing.T) {
	input := `
const PACKET_CAP = comptime 64 * 1024

arena memory_pool(PACKET_CAP)

func test_arena() {
    arena local_pool(comptime 1024 * 4)
    let ptr = local_pool.alloc(u64)
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics, got: %v", sem.Diagnostics())
	}
}

func TestSizeOfAndAlignOfBuiltins(t *testing.T) {
	input := `
struct Header {
    magic: u32
    length: u64
}

func test_queries() usize {
    let s1 = size_of(u32)
    let s2 = size_of(Header)
    let a1 = align_of(Header)
    return s1 + s2 + a1
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics, got: %v", sem.Diagnostics())
	}
}

func TestChannelTypeAndMethods(t *testing.T) {
	input := `
func worker(ch: Channel[u32]) {
    let ok = ch.send(42 as u32)
    yield()
    let msg = ch.recv()
}
`
	sem := parseAndAnalyze(input)
	if len(sem.Diagnostics()) > 0 {
		t.Fatalf("expected 0 diagnostics, got: %v", sem.Diagnostics())
	}
}

func TestChannelTypeMismatchError(t *testing.T) {
	input := `
func bad_worker(ch: Channel[u32]) {
    ch.send("wrong type")
}
`
	sem := parseAndAnalyze(input)
	found := false
	for _, d := range sem.Diagnostics() {
		if strings.Contains(d.Message, "cannot send value of type string into Channel[u32]") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected channel type mismatch error, got: %v", sem.Diagnostics())
	}
}
