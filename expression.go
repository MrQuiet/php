package php

import "stephensearles.com/php/ast"

/*

Valid Expression Patterns
Expr [Binary Op] Expr
[Unary Op] Expr
Expr [Unary Op]
Expr [Tertiary Op 1] Expr [Tertiary Op 2] Expr
Identifier
Literal
Function Call

Parentesis always triggers sub-expression

non-associative clone new clone and new
left  [ array()
right ++ -- ~ (int) (float) (string) (array) (object) (bool) @  types and increment/decrement
non-associative instanceof  types
right ! logical
left  * / % arithmetic
left  + - . arithmetic and string
left  << >> bitwise
non-associative < <= > >= comparison
non-associative == != === !== <>  comparison
left  & bitwise and references
left  ^ bitwise
left  | bitwise
left  &&  logical
left  ||  logical
left  ? : ternary
right = += -= *= /= .= %= &= |= ^= <<= >>= => assignment
left  and logical
left  xor logical
left  or  logical
left  , many uses

*/

var operatorPrecedence = map[ItemType]int{
	itemArrayLookupOperatorLeft:  19,
	itemArrayLookupOperatorRight: 19,
	itemUnaryOperator:            18,
	itemCastOperator:             18,
	itemInstanceofOperator:       17,
	itemNegationOperator:         16,
	itemMultOperator:             15,
	itemAdditionOperator:         14,
	itemSubtractionOperator:      14,
	itemConcatenationOperator:    14,

	itemBitwiseShiftOperator: 13,
	itemComparisonOperator:   12,
	itemEqualityOperator:     11,

	itemAmpersandOperator:  10,
	itemBitwiseXorOperator: 9,
	itemBitwiseOrOperator:  8,
	itemAndOperator:        7,
	itemOrOperator:         6,
	itemTernaryOperator1:   5,
	itemTernaryOperator2:   5,
	itemAssignmentOperator: 4,
	itemWrittenAndOperator: 3,
	itemWrittenXorOperator: 2,
	itemWrittenOrOperator:  1,
}

func (p *parser) parseExpression() (expr ast.Expression) {
	// consume expression
	originalParenLev := p.parenLevel
	switch p.current.typ {
	case itemNewOperator:
		return &ast.NewExpression{
			Expression: p.parseNextExpression(),
		}
	case itemUnaryOperator, itemNegationOperator:
		op := p.current
		expr = p.parseUnaryExpressionRight(p.parseNextExpression(), op)
	case itemArray:
		return p.parseArrayDeclaration()
	case itemIdentifier:
		if p.peek().typ == itemAssignmentOperator {
			assignee := p.parseIdentifier().(ast.Assignable)
			p.next()
			return ast.AssignmentExpression{
				Assignee: assignee,
				Operator: p.current.val,
				Value:    p.parseNextExpression(),
			}
		}
		fallthrough
	case itemNonVariableIdentifier, itemStringLiteral, itemNumberLiteral, itemBooleanLiteral:
		expr = p.parseOperation(originalParenLev, p.expressionize())
	case itemOpenParen:
		p.parenLevel += 1
		p.next()
		expr = p.parseExpression()
		p.expect(itemCloseParen)
		p.parenLevel -= 1
	default:
		p.errorf("Expected expression. Found %s", p.current)
		return nil
	}
	if p.parenLevel != originalParenLev {
		p.errorf("unbalanced parens: %d prev: %d", p.parenLevel, originalParenLev)
		return nil
	}
	return
}

func (p *parser) parseOperation(originalParenLevel int, lhs ast.Expression) (expr ast.Expression) {
	p.next()
	switch p.current.typ {
	case itemUnaryOperator:
		expr = p.parseUnaryExpressionLeft(lhs, p.current)
	case itemAdditionOperator, itemSubtractionOperator, itemConcatenationOperator, itemComparisonOperator, itemMultOperator, itemAndOperator, itemOrOperator, itemAmpersandOperator, itemBitwiseXorOperator, itemBitwiseOrOperator, itemBitwiseShiftOperator, itemWrittenAndOperator, itemWrittenXorOperator, itemWrittenOrOperator:
		expr = p.parseBinaryOperation(lhs, p.current, originalParenLevel)
	case itemCloseParen:
		if p.parenLevel <= originalParenLevel {
			p.backup()
			return lhs
		}
		p.parenLevel -= 1
		return p.parseOperation(originalParenLevel, lhs)
	default:
		p.backup()
		return lhs
	}
	return p.parseOperation(originalParenLevel, expr)
}

func (p *parser) parseBinaryOperation(lhs ast.Expression, operator Item, originalParenLevel int) ast.Expression {
	p.next()
	rhs := p.expressionize()
	for {
		p.next()
		nextOperator := p.current
		p.backup()
		nextOperatorPrecedence, ok := operatorPrecedence[nextOperator.typ]
		if ok && nextOperatorPrecedence > operatorPrecedence[operator.typ] {
			rhs = p.parseOperation(originalParenLevel, rhs)
		} else {
			break
		}
	}
	return newBinaryOperation(operator, lhs, rhs)
}

func (p *parser) parseUnaryExpressionRight(operand ast.Expression, operator Item) ast.Expression {
	return newUnaryOperation(operator, operand)
}

func (p *parser) parseUnaryExpressionLeft(operand ast.Expression, operator Item) ast.Expression {
	return newUnaryOperation(operator, operand)
}

// expressionize takes the current token and returns it as the simplest
// expression for that token. That means an expression with no operators
// except for the object operator.
func (p *parser) expressionize() ast.Expression {
	switch p.current.typ {
	case itemIdentifier:
		return p.parseIdentifier()
	case itemStringLiteral:
		return ast.Literal{Type: ast.String}
	case itemBooleanLiteral:
		return ast.Literal{Type: ast.Boolean}
	case itemNumberLiteral:
		return ast.Literal{Type: ast.Float}
	case itemNonVariableIdentifier:
		if p.peek().typ == itemOpenParen {
			return p.parseFunctionCall()
		}
		return ast.ConstantExpression{
			Identifier: ast.NewIdentifier(p.current.val),
		}
	case itemOpenParen:
		return p.parseExpression()
	}
	// error?
	return nil
}

func (p *parser) parseIdentifier() ast.Expression {
	ident := ast.NewIdentifier(p.current.val)
	switch pk := p.peek(); pk.typ {
	case itemObjectOperator:
		p.expect(itemObjectOperator)
		p.expect(itemNonVariableIdentifier)
		if pk = p.peek(); pk.typ == itemOpenParen {
			expr := &ast.MethodCallExpression{
				Receiver:               ident,
				FunctionCallExpression: p.parseFunctionCall(),
			}
			return expr
		}
		return &ast.PropertyExpression{
			Receiver: ident,
			Name:     p.current.val,
		}
	case itemArrayLookupOperatorLeft:
		return p.parseArrayLookup(ident)
	}
	return ident
}

func (p *parser) parseArrayLookup(e ast.Expression) ast.Expression {
	p.expect(itemArrayLookupOperatorLeft)
	if p.peek().typ == itemArrayLookupOperatorRight {
		p.expect(itemArrayLookupOperatorRight)
		return ast.ArrayAppendExpression{Array: e}
	}
	p.next()
	expr := &ast.ArrayLookupExpression{
		Array: e,
		Index: p.parseExpression(),
	}
	p.expect(itemArrayLookupOperatorRight)
	if p.peek().typ == itemArrayLookupOperatorLeft {
		return p.parseArrayLookup(expr)
	}
	return expr
}

func (p *parser) parseArrayDeclaration() ast.Expression {
	pairs := make([]ast.ArrayPair, 0)
	p.expect(itemOpenParen)
	var key, val ast.Expression
ArrayLoop:
	for {
		p.next()
		switch p.current.typ {
		case itemArrayKeyOperator:
			if val == nil {
				p.errorf("expected array key before =>.")
				return nil
			}
			key = val
			p.next()
			val = p.parseExpression()
		case itemCloseParen:
			if val != nil {
				pairs = append(pairs, ast.ArrayPair{key, val})
			}
			break ArrayLoop
		case itemArgumentSeparator:
			pairs = append(pairs, ast.ArrayPair{key, val})
			key = nil
			val = nil
		default:
			val = p.parseExpression()
		}
	}
	return &ast.ArrayExpression{Pairs: pairs}
}
