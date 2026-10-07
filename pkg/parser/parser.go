package parser

import (
	"fmt"
	"strconv"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/diag"
	"github.com/GioTld/gslc/pkg/lexer"
)

// Precedence levels for Pratt parsing
const (
	_ int = iota
	precLowest
	precAssign      // =, +=, -=, etc.
	precOr          // ||
	precAnd         // &&
	precBitOr       // |
	precBitXor      // ^
	precBitAnd      // &
	precEquals      // ==, !=
	precLessGreater // <, <=, >, >=
	precShift       // <<, >>
	precSum         // +, -
	precProduct     // *, /, %
	precAs          // as
	precPrefix      // -X, !X, ~X, *X
	precPostfix     // X?
	precCall        // X(...)
	precIndex       // X[...]
	precMember      // X.Y
)

var precedences = map[lexer.TokenKind]int{
	lexer.TokenOr:           precOr,
	lexer.TokenAnd:          precAnd,
	lexer.TokenBitOr:        precBitOr,
	lexer.TokenBitXor:       precBitXor,
	lexer.TokenBitAnd:       precBitAnd,
	lexer.TokenEqual:        precEquals,
	lexer.TokenNotEqual:     precEquals,
	lexer.TokenLess:         precLessGreater,
	lexer.TokenLessEqual:    precLessGreater,
	lexer.TokenGreater:      precLessGreater,
	lexer.TokenGreaterEqual: precLessGreater,
	lexer.TokenShiftLeft:    precShift,
	lexer.TokenShiftRight:   precShift,
	lexer.TokenPlus:         precSum,
	lexer.TokenMinus:        precSum,
	lexer.TokenStar:         precProduct,
	lexer.TokenSlash:        precProduct,
	lexer.TokenPercent:      precProduct,
	lexer.TokenAs:           precAs,
	lexer.TokenQuestion:     precPostfix,
	lexer.TokenLParen:       precCall,
	lexer.TokenLBracket:     precIndex,
	lexer.TokenDot:          precMember,
}

type (
	prefixParseFn func() ast.Expr
	infixParseFn  func(ast.Expr) ast.Expr
)

// Parser parses GSL tokens into an Abstract Syntax Tree.
type Parser struct {
	lex         *lexer.Lexer
	curToken    lexer.Token
	peekToken   lexer.Token
	diagnostics []diag.Diagnostic

	prefixParseFns map[lexer.TokenKind]prefixParseFn
	infixParseFns  map[lexer.TokenKind]infixParseFn
}

// New creates a new Parser instance.
func New(lex *lexer.Lexer) *Parser {
	p := &Parser{
		lex:            lex,
		prefixParseFns: make(map[lexer.TokenKind]prefixParseFn),
		infixParseFns:  make(map[lexer.TokenKind]infixParseFn),
	}

	// Register prefix parsers
	p.registerPrefix(lexer.TokenIdent, p.parseIdentExpr)
	p.registerPrefix(lexer.TokenInt, p.parseIntLitExpr)
	p.registerPrefix(lexer.TokenFloat, p.parseFloatLitExpr)
	p.registerPrefix(lexer.TokenString, p.parseStringLitExpr)
	p.registerPrefix(lexer.TokenChar, p.parseStringLitExpr)
	p.registerPrefix(lexer.TokenTrue, p.parseBoolLitExpr)
	p.registerPrefix(lexer.TokenFalse, p.parseBoolLitExpr)
	p.registerPrefix(lexer.TokenMinus, p.parsePrefixExpr)
	p.registerPrefix(lexer.TokenNot, p.parsePrefixExpr)
	p.registerPrefix(lexer.TokenBitNot, p.parsePrefixExpr)
	p.registerPrefix(lexer.TokenStar, p.parsePrefixExpr)
	p.registerPrefix(lexer.TokenBitAnd, p.parsePrefixExpr)
	p.registerPrefix(lexer.TokenLParen, p.parseGroupedExpr)
	p.registerPrefix(lexer.TokenLBrace, p.parseBlockExprAsExpr)
	p.registerPrefix(lexer.TokenComptime, p.parseComptimeExpr)
	p.registerPrefix(lexer.TokenAsm, p.parseAsmExpr)
	p.registerPrefix(lexer.TokenMatch, p.parseMatchExpr)
	// Register infix parsers
	p.registerInfix(lexer.TokenPlus, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenMinus, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenStar, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenSlash, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenPercent, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenEqual, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenNotEqual, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenLess, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenLessEqual, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenGreater, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenGreaterEqual, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenAnd, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenOr, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenBitAnd, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenBitOr, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenBitXor, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenShiftLeft, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenShiftRight, p.parseBinaryExpr)
	p.registerInfix(lexer.TokenAs, p.parseCastExpr)
	p.registerInfix(lexer.TokenQuestion, p.parseQuestionExpr)
	p.registerInfix(lexer.TokenLParen, p.parseCallExpr)
	p.registerInfix(lexer.TokenLBracket, p.parseIndexExpr)
	p.registerInfix(lexer.TokenDot, p.parseMemberExpr)

	// Read two tokens to prime curToken and peekToken
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) registerPrefix(kind lexer.TokenKind, fn prefixParseFn) {
	p.prefixParseFns[kind] = fn
}

func (p *Parser) registerInfix(kind lexer.TokenKind, fn infixParseFn) {
	p.infixParseFns[kind] = fn
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.lex.NextToken()
}

func (p *Parser) curTokenIs(k lexer.TokenKind) bool {
	return p.curToken.Kind == k
}

func (p *Parser) peekTokenIs(k lexer.TokenKind) bool {
	return p.peekToken.Kind == k
}

func (p *Parser) expect(k lexer.TokenKind) bool {
	if p.curTokenIs(k) {
		p.nextToken()
		return true
	}
	p.error(p.curToken.Span, fmt.Sprintf("expected %s, got %s", k, p.curToken.Kind))
	return false
}

func (p *Parser) expectPeek(k lexer.TokenKind) bool {
	if p.peekTokenIs(k) {
		p.nextToken()
		return true
	}
	p.error(p.peekToken.Span, fmt.Sprintf("expected %s, got %s", k, p.peekToken.Kind))
	return false
}

func (p *Parser) error(span diag.Span, msg string) {
	p.diagnostics = append(p.diagnostics, diag.NewError(span, msg))
}

func (p *Parser) Diagnostics() []diag.Diagnostic {
	all := append([]diag.Diagnostic{}, p.lex.Diagnostics()...)
	all = append(all, p.diagnostics...)
	return all
}

// ParseProgram parses an entire GSL source file into a Program AST node.
func (p *Parser) ParseProgram() *ast.Program {
	prog := &ast.Program{
		Decls: []ast.Decl{},
	}

	startSpan := p.curToken.Span

	for !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
			continue
		}

		decl := p.parseDecl()
		if decl != nil {
			prog.Decls = append(prog.Decls, decl)
		}
		p.nextToken()
	}

	prog.NodeSpan = startSpan.Merge(p.curToken.Span)
	return prog
}

func (p *Parser) parseDecl() ast.Decl {
	switch p.curToken.Kind {
	case lexer.TokenFunc:
		return p.parseFuncDecl()
	case lexer.TokenStruct:
		return p.parseStructDecl(false, false)
	case lexer.TokenPacked:
		if p.peekTokenIs(lexer.TokenStruct) {
			p.nextToken() // consume 'packed', curToken = 'struct'
			return p.parseStructDecl(true, false)
		}
		p.error(p.curToken.Span, "expected 'struct' after 'packed'")
		return nil
	case lexer.TokenCopy:
		if p.peekTokenIs(lexer.TokenStruct) {
			p.nextToken()
			return p.parseStructDecl(false, true)
		}
		p.error(p.curToken.Span, "expected 'struct' after 'copy'")
		return nil
	case lexer.TokenArena:
		return p.parseArenaDecl()
	case lexer.TokenConst:
		return p.parseConstDecl()
	case lexer.TokenImport:
		return p.parseImportDecl()
	default:
		p.error(p.curToken.Span, fmt.Sprintf("unexpected top-level token %s", p.curToken.Kind))
		return nil
	}
}

func (p *Parser) parseFuncDecl() *ast.FuncDecl {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	name := p.curToken.Literal

	if !p.expectPeek(lexer.TokenLParen) {
		return nil
	}

	params := []ast.Param{}
	p.nextToken() // move past '('

	for !p.curTokenIs(lexer.TokenRParen) && !p.curTokenIs(lexer.TokenEOF) {
		if !p.curTokenIs(lexer.TokenIdent) {
			p.error(p.curToken.Span, "expected parameter name")
			break
		}
		paramName := p.curToken.Literal

		if !p.expectPeek(lexer.TokenColon) {
			break
		}
		p.nextToken() // move to type

		paramType := p.parseType()
		params = append(params, ast.Param{Name: paramName, Type: paramType})

		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken() // consume ','
			p.nextToken() // move to next param
		} else {
			p.nextToken()
		}
	}

	// curToken is now ')'
	var retType ast.Type
	if p.peekTokenIs(lexer.TokenIdent) || p.peekTokenIs(lexer.TokenStar) || p.peekTokenIs(lexer.TokenLBracket) {
		p.nextToken() // move to return type
		retType = p.parseType()
	}

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}

	body := p.parseBlockExpr()

	return &ast.FuncDecl{
		Name:       name,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		NodeSpan:   startSpan.Merge(body.Span()),
	}
}

func (p *Parser) parseStructDecl(isPacked, isCopy bool) *ast.StructDecl {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	name := p.curToken.Literal

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}

	p.nextToken() // move past '{'

	fields := []ast.Field{}
	for !p.curTokenIs(lexer.TokenRBrace) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
			continue
		}

		if !p.curTokenIs(lexer.TokenIdent) {
			p.error(p.curToken.Span, "expected field name in struct")
			break
		}
		fieldName := p.curToken.Literal

		if !p.expectPeek(lexer.TokenColon) {
			break
		}
		p.nextToken() // move to type

		fieldType := p.parseType()
		fields = append(fields, ast.Field{Name: fieldName, Type: fieldType})

		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken() // consume ','
			p.nextToken() // move to next field
		} else {
			p.nextToken()
		}
	}

	endSpan := p.curToken.Span

	return &ast.StructDecl{
		Name:     name,
		Fields:   fields,
		IsPacked: isPacked,
		IsCopy:   isCopy,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseArenaDecl() *ast.ArenaDecl {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	name := p.curToken.Literal

	if !p.expectPeek(lexer.TokenLParen) {
		return nil
	}

	p.nextToken() // move past '('
	sizeExpr := p.parseExpr(precLowest)

	if !p.expectPeek(lexer.TokenRParen) {
		return nil
	}

	endSpan := p.curToken.Span

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	return &ast.ArenaDecl{
		Name:     name,
		SizeExpr: sizeExpr,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseType() ast.Type {
	startSpan := p.curToken.Span

	if p.curTokenIs(lexer.TokenStar) {
		p.nextToken()
		elem := p.parseType()
		return &ast.PointerType{
			IsMut:    true,
			ElemType: elem,
			TypeSpan: startSpan.Merge(elem.Span()),
		}
	}

	if p.curTokenIs(lexer.TokenLBracket) && p.peekTokenIs(lexer.TokenRBracket) {
		p.nextToken() // consume ']'
		p.nextToken() // move to elem type
		elem := p.parseType()
		return &ast.SliceType{
			ElemType: elem,
			TypeSpan: startSpan.Merge(elem.Span()),
		}
	}

	if p.curTokenIs(lexer.TokenIdent) {
		name := p.curToken.Literal
		endSpan := p.curToken.Span

		if p.peekTokenIs(lexer.TokenLBracket) {
			p.nextToken() // move to '['
			p.nextToken() // move past '['
			typeArgs := []ast.Type{}
			for !p.curTokenIs(lexer.TokenRBracket) && !p.curTokenIs(lexer.TokenEOF) {
				arg := p.parseType()
				typeArgs = append(typeArgs, arg)
				if p.peekTokenIs(lexer.TokenComma) {
					p.nextToken() // consume ','
					p.nextToken() // move to next arg
				} else {
					p.nextToken()
				}
			}
			closeSpan := p.curToken.Span
			return &ast.GenericType{
				Base:     name,
				TypeArgs: typeArgs,
				TypeSpan: startSpan.Merge(closeSpan),
			}
		}

		return &ast.NamedType{
			Name:     name,
			TypeSpan: startSpan.Merge(endSpan),
		}
	}

	p.error(p.curToken.Span, fmt.Sprintf("unexpected token for type: %s", p.curToken.Kind))
	return &ast.NamedType{Name: "unknown", TypeSpan: p.curToken.Span}
}

func (p *Parser) parseBlockExpr() *ast.BlockExpr {
	startSpan := p.curToken.Span
	stmts := []ast.Stmt{}

	p.nextToken() // move past '{'

	for !p.curTokenIs(lexer.TokenRBrace) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
			continue
		}

		stmt := p.parseStmt()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
		p.nextToken()
	}

	endSpan := p.curToken.Span

	return &ast.BlockExpr{
		Stmts:    stmts,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseBlockExprAsExpr() ast.Expr {
	return p.parseBlockExpr()
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.curToken.Kind {
	case lexer.TokenLet:
		return p.parseLetStmt()
	case lexer.TokenVar:
		return p.parseVarStmt()
	case lexer.TokenReturn:
		return p.parseReturnStmt()
	case lexer.TokenBreak:
		return p.parseBreakStmt()
	case lexer.TokenContinue:
		return p.parseContinueStmt()
	case lexer.TokenIf:
		return p.parseIfStmt()
	case lexer.TokenFor:
		return p.parseForStmt()
	case lexer.TokenWhile:
		return p.parseWhileStmt()
	case lexer.TokenSwitch:
		return p.parseSwitchStmt()
	case lexer.TokenArena:
		return p.parseArenaDecl()
	case lexer.TokenConst:
		return p.parseConstDecl()
	case lexer.TokenUnsafe:
		return p.parseUnsafeBlockStmt()
	default:
		return p.parseExprOrAssignStmt()
	}
}

func (p *Parser) parseLetStmt() *ast.LetStmt {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	name := p.curToken.Literal

	var varType ast.Type
	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // consume ':'
		p.nextToken() // move to type
		varType = p.parseType()
	}

	if !p.expectPeek(lexer.TokenAssign) {
		return nil
	}
	p.nextToken() // move to value

	val := p.parseExpr(precLowest)
	if val == nil {
		return nil
	}

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	return &ast.LetStmt{
		Name:     name,
		Type:     varType,
		Value:    val,
		NodeSpan: startSpan.Merge(val.Span()),
	}
}

func (p *Parser) parseVarStmt() *ast.VarStmt {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	name := p.curToken.Literal

	var varType ast.Type
	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // consume ':'
		p.nextToken() // move to type
		varType = p.parseType()
	}

	var val ast.Expr
	endSpan := p.curToken.Span
	if varType != nil {
		endSpan = varType.Span()
	}

	if p.peekTokenIs(lexer.TokenAssign) {
		p.nextToken() // consume '='
		p.nextToken() // move to value
		val = p.parseExpr(precLowest)
		if val != nil {
			endSpan = val.Span()
		}
	}

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	return &ast.VarStmt{
		Name:     name,
		Type:     varType,
		Value:    val,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	startSpan := p.curToken.Span

	var val ast.Expr
	if !p.peekTokenIs(lexer.TokenSemicolon) && !p.peekTokenIs(lexer.TokenRBrace) && !p.peekTokenIs(lexer.TokenEOF) {
		p.nextToken()
		val = p.parseExpr(precLowest)
	}

	endSpan := startSpan
	if val != nil {
		endSpan = val.Span()
	}

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	return &ast.ReturnStmt{
		Value:    val,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseBreakStmt() *ast.BreakStmt {
	span := p.curToken.Span
	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}
	return &ast.BreakStmt{NodeSpan: span}
}

func (p *Parser) parseContinueStmt() *ast.ContinueStmt {
	span := p.curToken.Span
	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}
	return &ast.ContinueStmt{NodeSpan: span}
}

func (p *Parser) parseIfStmt() *ast.IfStmt {
	startSpan := p.curToken.Span
	p.nextToken() // move to condition
	cond := p.parseExpr(precLowest)

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	thenBlock := p.parseBlockExpr()

	var elseBranch ast.Stmt
	if p.peekTokenIs(lexer.TokenElse) {
		p.nextToken() // curToken is 'else'
		if p.peekTokenIs(lexer.TokenIf) {
			p.nextToken() // curToken is 'if'
			elseBranch = p.parseIfStmt()
		} else if p.peekTokenIs(lexer.TokenLBrace) {
			p.nextToken() // curToken is '{'
			elseBranch = p.parseBlockExpr()
		}
	}

	endSpan := thenBlock.Span()
	if elseBranch != nil {
		endSpan = elseBranch.Span()
	}

	return &ast.IfStmt{
		Condition:  cond,
		ThenBlock:  thenBlock,
		ElseBranch: elseBranch,
		NodeSpan:   startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseForStmt() *ast.ForStmt {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	item := p.curToken.Literal

	if !p.expectPeek(lexer.TokenIn) {
		return nil
	}
	p.nextToken() // move to collection

	coll := p.parseExpr(precLowest)

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	body := p.parseBlockExpr()

	return &ast.ForStmt{
		Item:       item,
		Collection: coll,
		Body:       body,
		NodeSpan:   startSpan.Merge(body.Span()),
	}
}

func (p *Parser) parseWhileStmt() *ast.WhileStmt {
	startSpan := p.curToken.Span
	p.nextToken() // move past 'while' to condition expression

	cond := p.parseExpr(precLowest)

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	body := p.parseBlockExpr()

	return &ast.WhileStmt{
		Condition: cond,
		Body:      body,
		NodeSpan:  startSpan.Merge(body.Span()),
	}
}

func (p *Parser) parseUnsafeBlockStmt() *ast.UnsafeBlockStmt {
	startSpan := p.curToken.Span
	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	body := p.parseBlockExpr()

	return &ast.UnsafeBlockStmt{
		Body:     body,
		NodeSpan: startSpan.Merge(body.Span()),
	}
}

func (p *Parser) parseSwitchStmt() *ast.SwitchStmt {
	startSpan := p.curToken.Span
	p.nextToken() // move to target expr
	target := p.parseExpr(precLowest)

	if !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken() // move past '{'

	cases := []ast.CaseClause{}
	var defaultStmts []ast.Stmt
	hasDefault := false

	for !p.curTokenIs(lexer.TokenRBrace) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenCase) {
			caseStart := p.curToken.Span
			p.nextToken() // move to pattern
			pat := p.parseExpr(precLowest)

			if !p.expectPeek(lexer.TokenColon) {
				break
			}
			p.nextToken() // move past ':'

			var body []ast.Stmt
			for !p.curTokenIs(lexer.TokenCase) && !p.curTokenIs(lexer.TokenDefault) && !p.curTokenIs(lexer.TokenRBrace) && !p.curTokenIs(lexer.TokenEOF) {
				if p.curTokenIs(lexer.TokenSemicolon) {
					p.nextToken()
					continue
				}
				stmt := p.parseStmt()
				if stmt != nil {
					body = append(body, stmt)
				}
				p.nextToken()
			}
			cases = append(cases, ast.CaseClause{
				Pattern:  pat,
				Body:     body,
				NodeSpan: caseStart.Merge(p.curToken.Span),
			})
		} else if p.curTokenIs(lexer.TokenDefault) {
			hasDefault = true
			if !p.expectPeek(lexer.TokenColon) {
				break
			}
			p.nextToken() // move past ':'

			for !p.curTokenIs(lexer.TokenCase) && !p.curTokenIs(lexer.TokenRBrace) && !p.curTokenIs(lexer.TokenEOF) {
				if p.curTokenIs(lexer.TokenSemicolon) {
					p.nextToken()
					continue
				}
				stmt := p.parseStmt()
				if stmt != nil {
					defaultStmts = append(defaultStmts, stmt)
				}
				p.nextToken()
			}
		} else {
			p.nextToken()
		}
	}

	endSpan := p.curToken.Span

	return &ast.SwitchStmt{
		Expr:       target,
		Cases:      cases,
		Default:    defaultStmts,
		HasDefault: hasDefault,
		NodeSpan:   startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseExprOrAssignStmt() ast.Stmt {
	startSpan := p.curToken.Span
	expr := p.parseExpr(precLowest)

	if isAssignOp(p.peekToken.Kind) {
		p.nextToken() // curToken is assign operator
		op := p.curToken.Literal
		p.nextToken() // curToken is value start
		val := p.parseExpr(precLowest)

		if p.peekTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
		}

		return &ast.AssignStmt{
			Target:   expr,
			Op:       op,
			Value:    val,
			NodeSpan: startSpan.Merge(val.Span()),
		}
	}

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	return &ast.ExprStmt{
		Expression: expr,
		NodeSpan:   startSpan.Merge(p.curToken.Span),
	}
}

func isAssignOp(k lexer.TokenKind) bool {
	switch k {
	case lexer.TokenAssign, lexer.TokenPlusAssign, lexer.TokenMinusAssign,
		lexer.TokenStarAssign, lexer.TokenSlashAssign, lexer.TokenPercentAssign,
		lexer.TokenBitAndAssign, lexer.TokenBitOrAssign, lexer.TokenBitXorAssign,
		lexer.TokenShlAssign, lexer.TokenShrAssign:
		return true
	default:
		return false
	}
}

// --- Pratt Expression Parsing ---

func (p *Parser) parseExpr(precedence int) ast.Expr {
	prefix := p.prefixParseFns[p.curToken.Kind]
	if prefix == nil {
		p.error(p.curToken.Span, fmt.Sprintf("no prefix parse function for %s", p.curToken.Kind))
		return nil
	}

	left := prefix()

	for !p.peekTokenIs(lexer.TokenSemicolon) && !p.peekTokenIs(lexer.TokenEOF) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Kind]
		if infix == nil {
			return left
		}
		p.nextToken()
		left = infix(left)
	}

	return left
}

func (p *Parser) peekPrecedence() int {
	if prec, ok := precedences[p.peekToken.Kind]; ok {
		return prec
	}
	return precLowest
}

func (p *Parser) curPrecedence() int {
	if prec, ok := precedences[p.curToken.Kind]; ok {
		return prec
	}
	return precLowest
}

func (p *Parser) parseIdentExpr() ast.Expr {
	return &ast.IdentExpr{Name: p.curToken.Literal, NodeSpan: p.curToken.Span}
}

func (p *Parser) parseIntLitExpr() ast.Expr {
	return &ast.IntLitExpr{Value: p.curToken.Literal, NodeSpan: p.curToken.Span}
}

func (p *Parser) parseFloatLitExpr() ast.Expr {
	return &ast.FloatLitExpr{Value: p.curToken.Literal, NodeSpan: p.curToken.Span}
}

func (p *Parser) parseStringLitExpr() ast.Expr {
	return &ast.StringLitExpr{Value: p.curToken.Literal, NodeSpan: p.curToken.Span}
}

func (p *Parser) parseBoolLitExpr() ast.Expr {
	return &ast.BoolLitExpr{Value: p.curToken.Kind == lexer.TokenTrue, NodeSpan: p.curToken.Span}
}

func (p *Parser) parsePrefixExpr() ast.Expr {
	tok := p.curToken
	p.nextToken()
	right := p.parseExpr(precPrefix)
	return &ast.UnaryExpr{
		Op:       tok.Literal,
		Right:    right,
		NodeSpan: tok.Span.Merge(right.Span()),
	}
}

func (p *Parser) parseGroupedExpr() ast.Expr {
	p.nextToken() // consume '('
	expr := p.parseExpr(precLowest)
	if !p.expectPeek(lexer.TokenRParen) {
		return nil
	}
	return expr
}

func (p *Parser) parseBinaryExpr(left ast.Expr) ast.Expr {
	tok := p.curToken
	prec := p.curPrecedence()
	p.nextToken()
	right := p.parseExpr(prec)

	return &ast.BinaryExpr{
		Left:     left,
		Op:       tok.Literal,
		Right:    right,
		NodeSpan: left.Span().Merge(right.Span()),
	}
}

func (p *Parser) parseCastExpr(left ast.Expr) ast.Expr {
	p.nextToken() // consume 'as'
	toType := p.parseType()

	return &ast.CastExpr{
		Target:   left,
		ToType:   toType,
		NodeSpan: left.Span().Merge(toType.Span()),
	}
}

func (p *Parser) parseQuestionExpr(left ast.Expr) ast.Expr {
	span := p.curToken.Span

	return &ast.QuestionExpr{
		Target:   left,
		NodeSpan: left.Span().Merge(span),
	}
}

func (p *Parser) parseCallExpr(left ast.Expr) ast.Expr {
	args := []ast.Expr{}

	if !p.peekTokenIs(lexer.TokenRParen) {
		p.nextToken() // move to first arg
		for {
			arg := p.parseExpr(precLowest)
			if arg != nil {
				args = append(args, arg)
			}
			if p.peekTokenIs(lexer.TokenComma) {
				p.nextToken() // consume ','
				p.nextToken() // move to next arg
			} else {
				break
			}
		}
	}

	if !p.expectPeek(lexer.TokenRParen) {
		return nil
	}

	endSpan := p.curToken.Span

	return &ast.CallExpr{
		Callee:   left,
		Args:     args,
		NodeSpan: left.Span().Merge(endSpan),
	}
}

func (p *Parser) parseIndexExpr(left ast.Expr) ast.Expr {
	p.nextToken() // move past '['
	idx := p.parseExpr(precLowest)

	if !p.expectPeek(lexer.TokenRBracket) {
		return nil
	}
	endSpan := p.curToken.Span

	return &ast.IndexExpr{
		Target:   left,
		Index:    idx,
		NodeSpan: left.Span().Merge(endSpan),
	}
}

func (p *Parser) parseMemberExpr(left ast.Expr) ast.Expr {
	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	field := p.curToken.Literal
	endSpan := p.curToken.Span

	return &ast.MemberExpr{
		Target:   left,
		Field:    field,
		NodeSpan: left.Span().Merge(endSpan),
	}
}

func (p *Parser) parseConstDecl() *ast.ConstDecl {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	name := p.curToken.Literal

	var constType ast.Type
	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // consume ':'
		p.nextToken() // move to type
		constType = p.parseType()
	}

	if !p.expectPeek(lexer.TokenAssign) {
		return nil
	}

	p.nextToken() // move to value expr
	val := p.parseExpr(precLowest)

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	var endSpan diag.Span
	if val != nil {
		endSpan = val.Span()
	} else {
		endSpan = p.curToken.Span
	}

	return &ast.ConstDecl{
		Name:     name,
		Type:     constType,
		Value:    val,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseComptimeExpr() ast.Expr {
	startSpan := p.curToken.Span
	p.nextToken() // move past 'comptime'
	expr := p.parseExpr(precLowest)
	var endSpan diag.Span
	if expr != nil {
		endSpan = expr.Span()
	} else {
		endSpan = p.curToken.Span
	}
	return &ast.ComptimeExpr{
		Expr:     expr,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseImportDecl() *ast.ImportDecl {
	startSpan := p.curToken.Span

	if !p.expectPeek(lexer.TokenString) && !p.expectPeek(lexer.TokenIdent) {
		return nil
	}

	importPath := p.curToken.Literal
	if unquoted, err := strconv.Unquote(importPath); err == nil {
		importPath = unquoted
	}

	if p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}

	endSpan := p.curToken.Span
	return &ast.ImportDecl{
		Path:     importPath,
		NodeSpan: startSpan.Merge(endSpan),
	}
}

func (p *Parser) parseAsmExpr() ast.Expr {
	startSpan := p.curToken.Span
	if !p.expectPeek(lexer.TokenLParen) {
		return nil
	}
	if !p.expectPeek(lexer.TokenString) {
		p.error(p.curToken.Span, "expected string literal in asm(...)")
		return nil
	}
	raw := p.curToken.Literal
	instr, err := strconv.Unquote(raw)
	if err != nil {
		instr = raw
	}
	if !p.expectPeek(lexer.TokenRParen) {
		return nil
	}
	return &ast.AsmExpr{
		Instruction: instr,
		NodeSpan:    startSpan.Merge(p.curToken.Span),
	}
}

func (p *Parser) parseMatchExpr() ast.Expr {
	start := p.curToken.Span
	p.nextToken()
	target := p.parseExpr(precLowest)
	if target == nil || !p.expectPeek(lexer.TokenLBrace) {
		return nil
	}
	p.nextToken()
	var arms []ast.MatchArm
	for !p.curTokenIs(lexer.TokenRBrace) && !p.curTokenIs(lexer.TokenEOF) {
		pattern := p.parseExpr(precLowest)
		if pattern == nil || !p.expectPeek(lexer.TokenFatArrow) {
			return nil
		}
		p.nextToken()
		value := p.parseExpr(precLowest)
		if value == nil {
			return nil
		}
		arms = append(arms, ast.MatchArm{Pattern: pattern, Value: value})
		if p.peekTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
			if p.peekTokenIs(lexer.TokenRBrace) {
				p.nextToken()
				break
			}
			p.nextToken()
			continue
		}
		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken()
			p.nextToken()
			continue
		}
		if p.peekTokenIs(lexer.TokenRBrace) {
			p.nextToken()
			break
		}
		p.error(p.peekToken.Span, "expected ',' or '}' after match arm")
		return nil
	}
	if len(arms) == 0 {
		p.error(start, "match requires at least one arm")
	}
	return &ast.MatchExpr{Target: target, Arms: arms, NodeSpan: start.Merge(p.curToken.Span)}
}
