package lexer

import (
	"unicode"
	"unicode/utf8"

	"github.com/GioTld/gslc/pkg/diag"
)

// Lexer transforms GSL source code into a stream of Tokens with Automatic Semicolon Insertion (ASI).
type Lexer struct {
	filename      string
	source        string
	offset        int  // current byte offset of ch
	readOffset    int  // reading byte offset
	ch            rune // current character
	line          int  // 1-indexed current line
	column        int  // 1-indexed current column
	lastTokenKind TokenKind
	diagnostics   []diag.Diagnostic
}

// New creates and initializes a Lexer for the given source text.
func New(filename, source string) *Lexer {
	l := &Lexer{
		filename:      filename,
		source:        source,
		line:          1,
		column:        0,
		lastTokenKind: TokenIllegal,
	}
	l.readChar()
	return l
}

// Diagnostics returns all diagnostics recorded during lexing.
func (l *Lexer) Diagnostics() []diag.Diagnostic {
	return l.diagnostics
}

func (l *Lexer) currentPos() diag.Position {
	return diag.Position{
		Filename: l.filename,
		Line:     l.line,
		Column:   l.column,
		Offset:   l.offset,
	}
}

func (l *Lexer) readChar() {
	if l.readOffset >= len(l.source) {
		l.ch = 0
		l.offset = len(l.source)
	} else {
		r, width := utf8.DecodeRuneInString(l.source[l.readOffset:])
		l.ch = r
		l.offset = l.readOffset
		l.readOffset += width
	}

	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

func (l *Lexer) peekChar() rune {
	if l.readOffset >= len(l.source) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.source[l.readOffset:])
	return r
}

func canInsertSemicolon(kind TokenKind) bool {
	switch kind {
	case TokenIdent,
		TokenInt, TokenFloat, TokenString, TokenChar,
		TokenBreak, TokenContinue, TokenReturn, TokenTrue, TokenFalse,
		TokenRParen, TokenRBracket, TokenRBrace, TokenQuestion:
		return true
	default:
		return false
	}
}

// NextToken returns the next lexical token from the source code.
func (l *Lexer) NextToken() Token {
	for {
		// Skip spaces and tabs
		for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
			l.readChar()
		}

		// Handle comments
		if l.ch == '/' {
			if l.peekChar() == '/' {
				l.skipLineComment()
				// The newline after a line comment is processed for ASI
				continue
			} else if l.peekChar() == '*' {
				l.skipBlockComment()
				continue
			}
		}

		// Handle newlines with Automatic Semicolon Insertion
		if l.ch == '\n' {
			startPos := l.currentPos()
			l.readChar()
			if canInsertSemicolon(l.lastTokenKind) {
				l.lastTokenKind = TokenSemicolon
				return Token{
					Kind:    TokenSemicolon,
					Literal: "\n",
					Span:    diag.NewSpan(startPos, l.currentPos()),
				}
			}
			continue
		}

		break
	}

	startPos := l.currentPos()

	// Check for EOF
	if l.ch == 0 {
		if canInsertSemicolon(l.lastTokenKind) {
			l.lastTokenKind = TokenSemicolon
			return Token{
				Kind:    TokenSemicolon,
				Literal: "\n",
				Span:    diag.NewSpan(startPos, startPos),
			}
		}
		l.lastTokenKind = TokenEOF
		return Token{
			Kind:    TokenEOF,
			Literal: "",
			Span:    diag.NewSpan(startPos, startPos),
		}
	}

	// Identifiers and keywords
	if isIdentStart(l.ch) {
		tok := l.readIdent(startPos)
		l.lastTokenKind = tok.Kind
		return tok
	}

	// Numbers
	if unicode.IsDigit(l.ch) {
		tok := l.readNumber(startPos)
		l.lastTokenKind = tok.Kind
		return tok
	}

	// Strings
	if l.ch == '"' {
		tok := l.readString(startPos)
		l.lastTokenKind = tok.Kind
		return tok
	}

	// Characters
	if l.ch == '\'' {
		tok := l.readCharLiteral(startPos)
		l.lastTokenKind = tok.Kind
		return tok
	}

	// Operators and Delimiters
	ch := l.ch
	l.readChar()

	var tok Token
	switch ch {
	case '+':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenPlusAssign, Literal: "+=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenPlus, Literal: "+", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '-':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenMinusAssign, Literal: "-=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else if l.ch == '>' {
			l.readChar()
			tok = Token{Kind: TokenArrow, Literal: "->", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenMinus, Literal: "-", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '*':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenStarAssign, Literal: "*=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenStar, Literal: "*", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '/':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenSlashAssign, Literal: "/=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenSlash, Literal: "/", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '%':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenPercentAssign, Literal: "%=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenPercent, Literal: "%", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '=':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenEqual, Literal: "==", Span: diag.NewSpan(startPos, l.currentPos())}
		} else if l.ch == '>' {
			l.readChar()
			tok = Token{Kind: TokenFatArrow, Literal: "=>", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenAssign, Literal: "=", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '!':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenNotEqual, Literal: "!=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenNot, Literal: "!", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '<':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenLessEqual, Literal: "<=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else if l.ch == '<' {
			l.readChar()
			if l.ch == '=' {
				l.readChar()
				tok = Token{Kind: TokenShlAssign, Literal: "<<=", Span: diag.NewSpan(startPos, l.currentPos())}
			} else {
				tok = Token{Kind: TokenShiftLeft, Literal: "<<", Span: diag.NewSpan(startPos, l.currentPos())}
			}
		} else {
			tok = Token{Kind: TokenLess, Literal: "<", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '>':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenGreaterEqual, Literal: ">=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else if l.ch == '>' {
			l.readChar()
			if l.ch == '=' {
				l.readChar()
				tok = Token{Kind: TokenShrAssign, Literal: ">>=", Span: diag.NewSpan(startPos, l.currentPos())}
			} else {
				tok = Token{Kind: TokenShiftRight, Literal: ">>", Span: diag.NewSpan(startPos, l.currentPos())}
			}
		} else {
			tok = Token{Kind: TokenGreater, Literal: ">", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '&':
		if l.ch == '&' {
			l.readChar()
			tok = Token{Kind: TokenAnd, Literal: "&&", Span: diag.NewSpan(startPos, l.currentPos())}
		} else if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenBitAndAssign, Literal: "&=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenBitAnd, Literal: "&", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '|':
		if l.ch == '|' {
			l.readChar()
			tok = Token{Kind: TokenOr, Literal: "||", Span: diag.NewSpan(startPos, l.currentPos())}
		} else if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenBitOrAssign, Literal: "|=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenBitOr, Literal: "|", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '^':
		if l.ch == '=' {
			l.readChar()
			tok = Token{Kind: TokenBitXorAssign, Literal: "^=", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenBitXor, Literal: "^", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case '~':
		tok = Token{Kind: TokenBitNot, Literal: "~", Span: diag.NewSpan(startPos, l.currentPos())}
	case '?':
		tok = Token{Kind: TokenQuestion, Literal: "?", Span: diag.NewSpan(startPos, l.currentPos())}
	case ':':
		if l.ch == ':' {
			l.readChar()
			tok = Token{Kind: TokenColonColon, Literal: "::", Span: diag.NewSpan(startPos, l.currentPos())}
		} else {
			tok = Token{Kind: TokenColon, Literal: ":", Span: diag.NewSpan(startPos, l.currentPos())}
		}
	case ';':
		tok = Token{Kind: TokenSemicolon, Literal: ";", Span: diag.NewSpan(startPos, l.currentPos())}
	case ',':
		tok = Token{Kind: TokenComma, Literal: ",", Span: diag.NewSpan(startPos, l.currentPos())}
	case '.':
		tok = Token{Kind: TokenDot, Literal: ".", Span: diag.NewSpan(startPos, l.currentPos())}
	case '(':
		tok = Token{Kind: TokenLParen, Literal: "(", Span: diag.NewSpan(startPos, l.currentPos())}
	case ')':
		tok = Token{Kind: TokenRParen, Literal: ")", Span: diag.NewSpan(startPos, l.currentPos())}
	case '{':
		tok = Token{Kind: TokenLBrace, Literal: "{", Span: diag.NewSpan(startPos, l.currentPos())}
	case '}':
		tok = Token{Kind: TokenRBrace, Literal: "}", Span: diag.NewSpan(startPos, l.currentPos())}
	case '[':
		tok = Token{Kind: TokenLBracket, Literal: "[", Span: diag.NewSpan(startPos, l.currentPos())}
	case ']':
		tok = Token{Kind: TokenRBracket, Literal: "]", Span: diag.NewSpan(startPos, l.currentPos())}
	default:
		span := diag.NewSpan(startPos, l.currentPos())
		err := diag.NewError(span, "unexpected character '"+string(ch)+"'")
		l.diagnostics = append(l.diagnostics, err)
		tok = Token{Kind: TokenIllegal, Literal: string(ch), Span: span}
	}

	l.lastTokenKind = tok.Kind
	return tok
}

func (l *Lexer) skipLineComment() {
	l.readChar() // skip second '/'
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

func (l *Lexer) skipBlockComment() {
	startPos := l.currentPos()
	l.readChar() // skip '*'
	depth := 1

	for depth > 0 && l.ch != 0 {
		if l.ch == '/' && l.peekChar() == '*' {
			l.readChar()
			l.readChar()
			depth++
		} else if l.ch == '*' && l.peekChar() == '/' {
			l.readChar()
			l.readChar()
			depth--
		} else {
			l.readChar()
		}
	}

	if depth > 0 {
		err := diag.NewError(diag.NewSpan(startPos, l.currentPos()), "unterminated block comment")
		l.diagnostics = append(l.diagnostics, err)
	}
}

func (l *Lexer) readIdent(startPos diag.Position) Token {
	startOffset := startPos.Offset
	for isIdentPart(l.ch) {
		l.readChar()
	}
	literal := l.source[startOffset:l.offset]
	kind := LookupKeyword(literal)
	return Token{
		Kind:    kind,
		Literal: literal,
		Span:    diag.NewSpan(startPos, l.currentPos()),
	}
}

func (l *Lexer) readNumber(startPos diag.Position) Token {
	startOffset := startPos.Offset

	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		l.readChar()
		l.readChar()
		for isHexDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}
		return Token{
			Kind:    TokenInt,
			Literal: l.source[startOffset:l.offset],
			Span:    diag.NewSpan(startPos, l.currentPos()),
		}
	}

	if l.ch == '0' && (l.peekChar() == 'b' || l.peekChar() == 'B') {
		l.readChar()
		l.readChar()
		for l.ch == '0' || l.ch == '1' || l.ch == '_' {
			l.readChar()
		}
		return Token{
			Kind:    TokenInt,
			Literal: l.source[startOffset:l.offset],
			Span:    diag.NewSpan(startPos, l.currentPos()),
		}
	}

	if l.ch == '0' && (l.peekChar() == 'o' || l.peekChar() == 'O') {
		l.readChar()
		l.readChar()
		for (l.ch >= '0' && l.ch <= '7') || l.ch == '_' {
			l.readChar()
		}
		return Token{
			Kind:    TokenInt,
			Literal: l.source[startOffset:l.offset],
			Span:    diag.NewSpan(startPos, l.currentPos()),
		}
	}

	for unicode.IsDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	isFloat := false
	if l.ch == '.' && unicode.IsDigit(l.peekChar()) {
		isFloat = true
		l.readChar()
		for unicode.IsDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}
	}

	if l.ch == 'e' || l.ch == 'E' {
		isFloat = true
		l.readChar()
		if l.ch == '+' || l.ch == '-' {
			l.readChar()
		}
		for unicode.IsDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}
	}

	kind := TokenInt
	if isFloat {
		kind = TokenFloat
	}

	return Token{
		Kind:    kind,
		Literal: l.source[startOffset:l.offset],
		Span:    diag.NewSpan(startPos, l.currentPos()),
	}
}

func (l *Lexer) readString(startPos diag.Position) Token {
	startOffset := startPos.Offset
	l.readChar()

	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
			if l.ch != 0 {
				l.readChar()
			}
		} else {
			l.readChar()
		}
	}

	if l.ch == 0 {
		err := diag.NewError(diag.NewSpan(startPos, l.currentPos()), "unterminated string literal")
		l.diagnostics = append(l.diagnostics, err)
		return Token{
			Kind:    TokenIllegal,
			Literal: l.source[startOffset:l.offset],
			Span:    diag.NewSpan(startPos, l.currentPos()),
		}
	}

	l.readChar()

	return Token{
		Kind:    TokenString,
		Literal: l.source[startOffset:l.offset],
		Span:    diag.NewSpan(startPos, l.currentPos()),
	}
}

func (l *Lexer) readCharLiteral(startPos diag.Position) Token {
	startOffset := startPos.Offset
	l.readChar()

	if l.ch == '\\' {
		l.readChar()
		if l.ch != 0 {
			l.readChar()
		}
	} else if l.ch != 0 && l.ch != '\'' {
		l.readChar()
	}

	if l.ch != '\'' {
		err := diag.NewError(diag.NewSpan(startPos, l.currentPos()), "unterminated character literal")
		l.diagnostics = append(l.diagnostics, err)
		return Token{
			Kind:    TokenIllegal,
			Literal: l.source[startOffset:l.offset],
			Span:    diag.NewSpan(startPos, l.currentPos()),
		}
	}

	l.readChar()

	return Token{
		Kind:    TokenChar,
		Literal: l.source[startOffset:l.offset],
		Span:    diag.NewSpan(startPos, l.currentPos()),
	}
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func isHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
