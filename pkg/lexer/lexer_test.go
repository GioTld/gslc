package lexer

import (
	"testing"
)

func TestLexerKeywordsAndIdentifiers(t *testing.T) {
	input := `
func compute_hash(val: u64) u64 {
    let base = 100
    var count = val + base
    return count
}
`
	expected := []struct {
		kind    TokenKind
		literal string
	}{
		{TokenFunc, "func"},
		{TokenIdent, "compute_hash"},
		{TokenLParen, "("},
		{TokenIdent, "val"},
		{TokenColon, ":"},
		{TokenIdent, "u64"},
		{TokenRParen, ")"},
		{TokenIdent, "u64"},
		{TokenLBrace, "{"},
		{TokenLet, "let"},
		{TokenIdent, "base"},
		{TokenAssign, "="},
		{TokenInt, "100"},
		{TokenSemicolon, "\n"}, // ASI inserted
		{TokenVar, "var"},
		{TokenIdent, "count"},
		{TokenAssign, "="},
		{TokenIdent, "val"},
		{TokenPlus, "+"},
		{TokenIdent, "base"},
		{TokenSemicolon, "\n"}, // ASI inserted
		{TokenReturn, "return"},
		{TokenIdent, "count"},
		{TokenSemicolon, "\n"}, // ASI inserted
		{TokenRBrace, "}"},
		{TokenSemicolon, "\n"}, // ASI inserted after }
		{TokenEOF, ""},
	}

	l := New("test.gsl", input)
	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Kind != exp.kind {
			t.Fatalf("[%d] expected token kind %s, got %s (literal: %q)", i, exp.kind, tok.Kind, tok.Literal)
		}
		if tok.Literal != exp.literal {
			t.Fatalf("[%d] expected literal %q, got %q", i, exp.literal, tok.Literal)
		}
	}
}

func TestAutomaticSemicolonInsertion(t *testing.T) {
	input := `
let x = 10
let y = 20 +
    30
return x + y
`
	expected := []struct {
		kind TokenKind
	}{
		{TokenLet}, {TokenIdent}, {TokenAssign}, {TokenInt},
		{TokenSemicolon}, // inserted after 10
		{TokenLet}, {TokenIdent}, {TokenAssign}, {TokenInt}, {TokenPlus},
		// No semicolon after '+'!
		{TokenInt},
		{TokenSemicolon}, // inserted after 30
		{TokenReturn}, {TokenIdent}, {TokenPlus}, {TokenIdent},
		{TokenSemicolon}, // inserted after y
		{TokenEOF},
	}

	l := New("test.gsl", input)
	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Kind != exp.kind {
			t.Fatalf("[%d] expected token %s, got %s (literal: %q)", i, exp.kind, tok.Kind, tok.Literal)
		}
	}
}

func TestLexerSwitchCase(t *testing.T) {
	input := `
switch event {
case Event.Data:
    handle()
default:
    ignore()
}
`
	l := New("test.gsl", input)
	tok := l.NextToken()
	if tok.Kind != TokenSwitch {
		t.Fatalf("expected switch, got %s", tok.Kind)
	}
}

func TestLexerNumberLiterals(t *testing.T) {
	input := `123 0xFF_A0 0b1011_0001 0o755 3.1415 1e-4 2.5e+3`

	expected := []struct {
		kind    TokenKind
		literal string
	}{
		{TokenInt, "123"},
		{TokenInt, "0xFF_A0"},
		{TokenInt, "0b1011_0001"},
		{TokenInt, "0o755"},
		{TokenFloat, "3.1415"},
		{TokenFloat, "1e-4"},
		{TokenFloat, "2.5e+3"},
		{TokenSemicolon, "\n"},
		{TokenEOF, ""},
	}

	l := New("test.gsl", input)
	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Kind != exp.kind {
			t.Fatalf("[%d] expected token kind %s, got %s (literal: %q)", i, exp.kind, tok.Kind, tok.Literal)
		}
		if tok.Literal != exp.literal {
			t.Fatalf("[%d] expected literal %q, got %q", i, exp.literal, tok.Literal)
		}
	}
}

func TestLexerComments(t *testing.T) {
	input := `
// This is a line comment
let a = 1
/* This is a block comment */
let b = 2
/* Outer comment /* nested comment */ still outer */
let c = 3
`
	l := New("test.gsl", input)
	for tok := l.NextToken(); tok.Kind != TokenEOF; tok = l.NextToken() {
		// Just verify scanning without errors
	}

	if len(l.Diagnostics()) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(l.Diagnostics()))
	}
}

func TestLexerKernelKeywords(t *testing.T) {
	input := `packed struct IDT { asm("cli"); }`
	expected := []struct {
		kind    TokenKind
		literal string
	}{
		{TokenPacked, "packed"},
		{TokenStruct, "struct"},
		{TokenIdent, "IDT"},
		{TokenLBrace, "{"},
		{TokenAsm, "asm"},
		{TokenLParen, "("},
		{TokenString, "\"cli\""},
		{TokenRParen, ")"},
		{TokenSemicolon, ";"},
		{TokenRBrace, "}"},
		{TokenSemicolon, "\n"},
		{TokenEOF, ""},
	}

	l := New("test.gsl", input)
	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Kind != exp.kind {
			t.Fatalf("[%d] expected token kind %s, got %s (literal: %q)", i, exp.kind, tok.Kind, tok.Literal)
		}
		if tok.Literal != exp.literal {
			t.Fatalf("[%d] expected literal %q, got %q", i, exp.literal, tok.Literal)
		}
	}
}

