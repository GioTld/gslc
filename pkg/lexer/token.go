package lexer

import (
	"fmt"

	"github.com/GioTld/gslc/pkg/diag"
)

// TokenKind represents the type of a lexical token.
type TokenKind int

const (
	// Special tokens
	TokenEOF TokenKind = iota
	TokenIllegal

	// Literals
	TokenIdent
	TokenInt
	TokenFloat
	TokenString
	TokenChar

	// Keywords
	keywordBegin
	TokenLet
	TokenVar
	TokenConst
	TokenFunc
	TokenReturn
	TokenIf
	TokenElse
	TokenSwitch
	TokenMatch
	TokenCase
	TokenDefault
	TokenWhile
	TokenFor
	TokenIn
	TokenBreak
	TokenContinue
	TokenStruct
	TokenEnum
	TokenType
	TokenImport
	TokenAs
	TokenArena
	TokenComptime
	TokenUnsafe
	TokenVolatile
	TokenAsm
	TokenPacked
	TokenCopy
	TokenTrue
	TokenFalse
	keywordEnd

	// Operators & Delimiters
	TokenPlus          // +
	TokenMinus         // -
	TokenStar          // *
	TokenSlash         // /
	TokenPercent       // %
	TokenAssign        // =
	TokenPlusAssign    // +=
	TokenMinusAssign   // -=
	TokenStarAssign    // *=
	TokenSlashAssign   // /=
	TokenPercentAssign // %=

	TokenEqual        // ==
	TokenNotEqual     // !=
	TokenLess         // <
	TokenLessEqual    // <=
	TokenGreater      // >
	TokenGreaterEqual // >=

	TokenAnd // &&
	TokenOr  // ||
	TokenNot // !

	TokenBitAnd       // &
	TokenBitOr        // |
	TokenBitXor       // ^
	TokenBitNot       // ~
	TokenShiftLeft    // <<
	TokenShiftRight   // >>
	TokenBitAndAssign // &=
	TokenBitOrAssign  // |=
	TokenBitXorAssign // ^=
	TokenShlAssign    // <<=
	TokenShrAssign    // >>=

	TokenQuestion   // ?
	TokenArrow      // ->
	TokenFatArrow   // =>
	TokenColonColon // ::

	TokenLParen    // (
	TokenRParen    // )
	TokenLBrace    // {
	TokenRBrace    // }
	TokenLBracket  // [
	TokenRBracket  // ]
	TokenComma     // ,
	TokenSemicolon // ;
	TokenColon     // :
	TokenDot       // .
)

var keywords = map[string]TokenKind{
	"let":      TokenLet,
	"var":      TokenVar,
	"const":    TokenConst,
	"func":     TokenFunc,
	"fn":       TokenFunc, // friendly alias for func
	"return":   TokenReturn,
	"if":       TokenIf,
	"else":     TokenElse,
	"switch":   TokenSwitch,
	"match":    TokenMatch,
	"case":     TokenCase,
	"default":  TokenDefault,
	"while":    TokenWhile,
	"for":      TokenFor,
	"in":       TokenIn,
	"break":    TokenBreak,
	"continue": TokenContinue,
	"struct":   TokenStruct,
	"enum":     TokenEnum,
	"type":     TokenType,
	"import":   TokenImport,
	"as":       TokenAs,
	"arena":    TokenArena,
	"comptime": TokenComptime,
	"unsafe":   TokenUnsafe,
	"volatile": TokenVolatile,
	"asm":      TokenAsm,
	"packed":   TokenPacked,
	"copy":     TokenCopy,
	"true":     TokenTrue,
	"false":    TokenFalse,
}

var tokenNames = map[TokenKind]string{
	TokenEOF:           "EOF",
	TokenIllegal:       "ILLEGAL",
	TokenIdent:         "IDENT",
	TokenInt:           "INT",
	TokenFloat:         "FLOAT",
	TokenString:        "STRING",
	TokenChar:          "CHAR",
	TokenLet:           "let",
	TokenVar:           "var",
	TokenConst:         "const",
	TokenFunc:          "func",
	TokenReturn:        "return",
	TokenIf:            "if",
	TokenElse:          "else",
	TokenSwitch:        "switch",
	TokenMatch:         "match",
	TokenCase:          "case",
	TokenDefault:       "default",
	TokenWhile:         "while",
	TokenFor:           "for",
	TokenIn:            "in",
	TokenBreak:         "break",
	TokenContinue:      "continue",
	TokenStruct:        "struct",
	TokenEnum:          "enum",
	TokenType:          "type",
	TokenImport:        "import",
	TokenAs:            "as",
	TokenArena:         "arena",
	TokenComptime:      "comptime",
	TokenUnsafe:        "unsafe",
	TokenVolatile:      "volatile",
	TokenAsm:           "asm",
	TokenPacked:        "packed",
	TokenCopy:          "copy",
	TokenTrue:          "true",
	TokenFalse:         "false",
	TokenPlus:          "+",
	TokenMinus:         "-",
	TokenStar:          "*",
	TokenSlash:         "/",
	TokenPercent:       "%",
	TokenAssign:        "=",
	TokenPlusAssign:    "+=",
	TokenMinusAssign:   "-=",
	TokenStarAssign:    "*=",
	TokenSlashAssign:   "/=",
	TokenPercentAssign: "%=",
	TokenEqual:         "==",
	TokenNotEqual:      "!=",
	TokenLess:          "<",
	TokenLessEqual:     "<=",
	TokenGreater:       ">",
	TokenGreaterEqual:  ">=",
	TokenAnd:           "&&",
	TokenOr:            "||",
	TokenNot:           "!",
	TokenBitAnd:        "&",
	TokenBitOr:         "|",
	TokenBitXor:        "^",
	TokenBitNot:        "~",
	TokenShiftLeft:     "<<",
	TokenShiftRight:    ">>",
	TokenBitAndAssign:  "&=",
	TokenBitOrAssign:   "|=",
	TokenBitXorAssign:  "^=",
	TokenShlAssign:     "<<=",
	TokenShrAssign:     ">>=",
	TokenQuestion:      "?",
	TokenArrow:         "->",
	TokenFatArrow:      "=>",
	TokenColonColon:    "::",
	TokenLParen:        "(",
	TokenRParen:        ")",
	TokenLBrace:        "{",
	TokenRBrace:        "}",
	TokenLBracket:      "[",
	TokenRBracket:      "]",
	TokenComma:         ",",
	TokenSemicolon:     ";",
	TokenColon:         ":",
	TokenDot:           ".",
}

func (k TokenKind) String() string {
	if name, ok := tokenNames[k]; ok {
		return name
	}
	return fmt.Sprintf("Token(%d)", k)
}

func (k TokenKind) IsKeyword() bool {
	return k > keywordBegin && k < keywordEnd
}

func LookupKeyword(ident string) TokenKind {
	if kind, ok := keywords[ident]; ok {
		return kind
	}
	return TokenIdent
}

type Token struct {
	Kind    TokenKind
	Literal string
	Span    diag.Span
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q) at %s", t.Kind, t.Literal, t.Span)
}
