package parser

import (
	"github.com/stephens2424/php/ast"
	"github.com/stephens2424/php/token"
)

func (p *Parser) parseTopStmt() ast.Statement {
	switch p.current.Typ {
	case token.Namespace:
		// TODO check that this comes before anything but a declare statement
		p.expect(token.Identifier)
		p.namespace = ast.NewNamespace(p.current.Val)
		p.file.Namespace = p.namespace
		p.expectStmtEnd()
		return nil
	case token.Use:
		p.expect(token.Identifier)
		if p.peek().Typ == token.AsOperator {
			p.expect(token.AsOperator)
			p.expect(token.Identifier)
		}
		p.expectStmtEnd()
		// We are ignoring this for now
		return nil
	case token.Declare:
		return p.parseDeclareBlock()
	default:
		return p.parseStmt()
	}
}

func (p *Parser) parseStmt() ast.Statement {
	switch p.current.Typ {
	case token.BlockBegin:
		p.backup()
		return p.parseBlock()
	case token.Global:
		p.next()
		g := &ast.GlobalDeclaration{
			Identifiers: make([]*ast.Variable, 0, 1),
		}
		for p.current.Typ == token.VariableOperator {
			variable, ok := p.parseVariable().(*ast.Variable)
			if !ok {
				p.errorf("global declarations must be of standard variables")
				break
			}
			g.Identifiers = append(g.Identifiers, variable)
			if p.peek().Typ != token.Comma {
				break
			}
			p.expect(token.Comma)
			p.next()
		}
		p.expectStmtEnd()
		return g
	case token.Static:
		if p.peek().Typ == token.ScopeResolutionOperator {
			p.errorf("static keyword outside of class context")
			expr := p.parseExpression()
			p.expectStmtEnd()
			return expr
		}

		s := &ast.StaticVariableDeclaration{Declarations: make([]ast.Dynamic, 0)}
		for {
			p.next()
			v, ok := p.parseVariable().(*ast.Variable)
			if !ok {
				p.errorf("global static declaration must be a variable")
				return nil
			}

			if _, ok := v.Name.(*ast.Identifier); !ok {
				p.errorf("static variable declarations must not be dynamic")
			}

			// check if there's an initial assignment
			if p.peek().Typ == token.AssignmentOperator {
				p.expect(token.AssignmentOperator)
				op := p.current.Val
				p.expect(token.Null, token.StringLiteral, token.BooleanLiteral, token.NumberLiteral, token.Array)
				switch p.current.Typ {
				case token.Array:
					s.Declarations = append(s.Declarations, &ast.AssignmentExpr{Assignee: v, Value: p.parseArrayDeclaration(), Operator: op})
				default:
					s.Declarations = append(s.Declarations, &ast.AssignmentExpr{Assignee: v, Value: p.parseLiteral(), Operator: op})
				}
			} else {
				s.Declarations = append(s.Declarations, v)
			}
			if p.peek().Typ != token.Comma {
				break
			}
			p.next()
		}
		p.expectStmtEnd()
		return s
	case token.VariableOperator, token.UnaryOperator:
		expr := ast.ExprStmt{Expr: p.parseExpression()}
		p.expectStmtEnd()
		return expr
	case token.Print:
		requireParen := false
		if p.peek().Typ == token.OpenParen {
			p.expect(token.OpenParen)
			requireParen = true
		}
		stmt := ast.Echo(p.parseNextExpression())
		if requireParen {
			p.expect(token.CloseParen)
		}
		p.expectStmtEnd()
		return stmt
	case token.Function:
		return p.parseFunctionStmt(false)
	case token.PHPEnd:
		if p.peek().Typ == token.EOF {
			return nil
		}
		var expr ast.Statement
		if p.accept(token.HTML) {
			expr = ast.Echo(&ast.Literal{Type: ast.String, Value: p.current.Val})
		}
		p.next()
		if p.current.Typ != token.EOF {
			p.expectCurrent(token.PHPBegin)
		}
		return expr
	case token.Echo:
		exprs := []ast.Expr{
			p.parseNextExpression(),
		}
		for p.peek().Typ == token.Comma {
			p.expect(token.Comma)
			exprs = append(exprs, p.parseNextExpression())
		}
		p.expectStmtEnd()
		echo := ast.Echo(exprs...)
		return echo
	case token.If:
		return p.parseIf()
	case token.While:
		return p.parseWhile()
	case token.Do:
		return p.parseDo()
	case token.For:
		return p.parseFor()
	case token.Foreach:
		return p.parseForeach()
	case token.Switch:
		return p.parseSwitch()
	case token.Abstract, token.Final, token.Class:
		return p.parseClass()
	case token.Interface:
		return p.parseInterface()
	case token.Return:
		p.next()
		stmt := &ast.ReturnStmt{}
		if p.current.Typ != token.StatementEnd {
			stmt.Expr = p.parseExpression()
			p.expectStmtEnd()
		}
		return stmt
	case token.Break:
		p.next()
		stmt := &ast.BreakStmt{}
		if p.current.Typ != token.StatementEnd {
			stmt.Expr = p.parseExpression()
			p.expectStmtEnd()
		}
		return stmt
	case token.Continue:
		p.next()
		stmt := &ast.ContinueStmt{}
		if p.current.Typ != token.StatementEnd {
			stmt.Expr = p.parseExpression()
			p.expectStmtEnd()
		}
		return stmt
	case token.Throw:
		stmt := ast.ThrowStmt{Expr: p.parseNextExpression()}
		p.expectStmtEnd()
		return stmt
	case token.Exit:
		stmt := &ast.ExitStmt{}
		if p.peek().Typ == token.OpenParen {
			p.expect(token.OpenParen)
			if p.peek().Typ != token.CloseParen {
				stmt.Expr = p.parseNextExpression()
			}
			p.expect(token.CloseParen)
		}
		p.expectStmtEnd()
		return stmt
	case token.Try:
		stmt := &ast.TryStmt{}
		stmt.TryBlock = p.parseBlock()
		for p.expect(token.Catch); p.current.Typ == token.Catch; p.next() {
			caught := &ast.CatchStmt{}
			p.expect(token.OpenParen)
			p.expect(token.Identifier)
			caught.CatchType = p.current.Val
			p.expect(token.VariableOperator)
			p.expect(token.Identifier)
			caught.CatchVar = ast.NewVariable(p.current.Val)
			p.expect(token.CloseParen)
			caught.CatchBlock = p.parseBlock()
			stmt.CatchStmts = append(stmt.CatchStmts, caught)
		}
		p.backup()
		return stmt
	case token.IgnoreErrorOperator:
		// Ignore this operator
		p.next()
		return p.parseStmt()
	case token.StatementEnd:
		// this is an empty statement
		return &ast.EmptyStatement{}
	default:
		expr := p.parseExpression()
		if expr != nil {
			p.expectStmtEnd()
			return ast.ExprStmt{Expr: expr}
		}
		p.errorf("Found %s, statement or expression", p.current)
		return nil
	}
}

func (p *Parser) expectStmtEnd() {
	if p.peek().Typ != token.PHPEnd {
		p.expect(token.StatementEnd)
	}
}
