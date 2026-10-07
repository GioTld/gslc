package sema

import (
	"fmt"

	"github.com/GioTld/gslc/pkg/diag"
	"github.com/GioTld/gslc/pkg/types"
)

// SymbolKind represents the classification of an identifier.
type SymbolKind int

const (
	SymbolVar SymbolKind = iota
	SymbolFunc
	SymbolType
	SymbolArena
	SymbolConst
)

func (k SymbolKind) String() string {
	switch k {
	case SymbolVar:
		return "variable"
	case SymbolFunc:
		return "function"
	case SymbolType:
		return "type"
	case SymbolArena:
		return "arena"
	case SymbolConst:
		return "constant"
	default:
		return "symbol"
	}
}

// Symbol represents a declared entity with a type, mutability flag, and origin span.
type Symbol struct {
	Name          string
	Kind          SymbolKind
	Type          types.Type
	IsMut         bool
	IsMoved       bool
	Span          diag.Span
	ArenaScope    string // Non-empty if allocated from a scoped arena
	DefiningScope *Scope
}

// Scope represents a lexical scope nesting bindings.
type Scope struct {
	Parent    *Scope
	Symbols   map[string]*Symbol
	Arenas    map[string]*Symbol
	IsUnsafe  bool
	ArenaName string
}

// NewScope creates an inner child scope.
func NewScope(parent *Scope) *Scope {
	isUnsafe := false
	arenaName := ""
	if parent != nil {
		isUnsafe = parent.IsUnsafe
		arenaName = parent.ArenaName
	}
	return &Scope{
		Parent:    parent,
		Symbols:   make(map[string]*Symbol),
		Arenas:    make(map[string]*Symbol),
		IsUnsafe:  isUnsafe,
		ArenaName: arenaName,
	}
}

// Depth returns the nesting depth of this scope from the root.
func (s *Scope) Depth() int {
	if s == nil || s.Parent == nil {
		return 0
	}
	return s.Parent.Depth() + 1
}

// IsAncestorOf returns true if s is an ancestor of child.
func (s *Scope) IsAncestorOf(child *Scope) bool {
	for cur := child; cur != nil; cur = cur.Parent {
		if cur == s {
			return true
		}
	}
	return false
}

// Define registers a symbol in the current scope.
func (s *Scope) Define(sym *Symbol) error {
	if _, exists := s.Symbols[sym.Name]; exists {
		return fmt.Errorf("redefinition of identifier %q", sym.Name)
	}
	sym.DefiningScope = s
	s.Symbols[sym.Name] = sym
	if sym.Kind == SymbolArena {
		s.Arenas[sym.Name] = sym
		s.ArenaName = sym.Name
	}
	return nil
}

// Lookup finds a symbol in this scope or any ancestor scope.
func (s *Scope) Lookup(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	if s.Parent != nil {
		return s.Parent.Lookup(name)
	}
	return nil
}

// LookupLocal checks only the current scope.
func (s *Scope) LookupLocal(name string) *Symbol {
	return s.Symbols[name]
}
