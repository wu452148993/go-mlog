package transpiler

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
)

func statementToMLOG(statement ast.Stmt, options Options) ([]MLOGStatement, error) {
	results := make([]MLOGStatement, 0)

	switch statement.(type) {
	case *ast.ForStmt:
		forStatement := statement.(*ast.ForStmt)

		// TODO Switch from do while to while do

		if len(forStatement.Body.List) == 0 {
			break
		}

		initMlog, err := statementToMLOG(forStatement.Init, options)
		if err != nil {
			return nil, err
		}
		results = append(results, initMlog...)

		var loopStartJump *MLOGJump
		if binaryExpr, ok := forStatement.Cond.(*ast.BinaryExpr); ok {
			if translatedOp, ok := jumpOperators[binaryExpr.Op]; ok {
				var leftSide Resolvable
				var rightSide Resolvable

				if basicLit, ok := binaryExpr.X.(*ast.BasicLit); ok {
					leftSide = &Value{Value: basicLit.Value}
				} else if ident, ok := binaryExpr.X.(*ast.Ident); ok {
					leftSide = &NormalVariable{Name: ident.Name}
				} else {
					return nil, errors.New(fmt.Sprintf("unknown left side expression type: %T", binaryExpr.X))
				}

				if basicLit, ok := binaryExpr.Y.(*ast.BasicLit); ok {
					rightSide = &Value{Value: basicLit.Value}
				} else if ident, ok := binaryExpr.Y.(*ast.Ident); ok {
					rightSide = &NormalVariable{Name: ident.Name}
				} else {
					return nil, errors.New(fmt.Sprintf("unknown right side expression type: %T", binaryExpr.Y))
				}

				loopStartJump = &MLOGJump{
					MLOG: MLOG{
						Comment: "Jump to start of loop",
					},
					Condition: []Resolvable{
						&Value{Value: translatedOp},
						leftSide,
						rightSide,
					},
				}
				results = append(results)
			} else {
				return nil, errors.New(fmt.Sprintf("jump statement cannot use this operation: %T", binaryExpr.Op))
			}
		} else {
			return nil, errors.New("for loop can only have binary conditional expressions")
		}

		bodyMLOG, err := statementToMLOG(forStatement.Body, options)
		if err != nil {
			return nil, err
		}

		results = append(results, bodyMLOG...)

		instructions, err := statementToMLOG(forStatement.Post, options)
		if err != nil {
			return nil, err
		}
		results = append(results, instructions...)

		loopStartJump.JumpTarget = bodyMLOG[0]
		results = append(results, loopStartJump)

		break
	case *ast.ExprStmt:
		expressionStatement := statement.(*ast.ExprStmt)

		instructions, err := expressionToMLOG(nil, expressionStatement.X, options)
		if err != nil {
			return nil, err
		}

		results = append(results, instructions...)
		break
	case *ast.IfStmt:
		ifStmt := statement.(*ast.IfStmt)

		if ifStmt.Init != nil {
			instructions, err := statementToMLOG(ifStmt.Init, options)
			if err != nil {
				return nil, err
			}
			results = append(results, instructions...)
		}

		var condVar Resolvable
		if condIdent, ok := ifStmt.Cond.(*ast.Ident); ok {
			condVar = &NormalVariable{Name: condIdent.Name}
		} else {
			condVar = &DynamicVariable{}

			instructions, err := expressionToMLOG([]Resolvable{condVar}, ifStmt.Cond, options)
			if err != nil {
				return nil, err
			}

			results = append(results, instructions...)
		}

		blockInstructions, err := statementToMLOG(ifStmt.Body, options)
		if err != nil {
			return nil, err
		}

		results = append(results, &MLOGJump{
			MLOG: MLOG{
				Comment: "Jump to if block if true",
			},
			Condition: []Resolvable{
				&Value{Value: "equal"},
				condVar,
				&Value{Value: "1"},
			},
			JumpTarget: &StatementJumpTarget{
				Statement: blockInstructions[0],
			},
		})

		afterIfTarget := &StatementJumpTarget{
			After:     true,
			Statement: blockInstructions[len(blockInstructions)-1],
		}
		results = append(results, &MLOGJump{
			MLOG: MLOG{
				Comment: "Jump to after if block",
			},
			Condition: []Resolvable{
				&Value{Value: "always"},
			},
			JumpTarget: afterIfTarget,
		})

		results = append(results, blockInstructions...)

		if ifStmt.Else != nil {
			elseInstructions, err := statementToMLOG(ifStmt.Else, options)
			if err != nil {
				return nil, err
			}

			afterElseJump := &MLOGJump{
				MLOG: MLOG{
					Comment: "Jump to after else block",
				},
				Condition: []Resolvable{
					&Value{Value: "always"},
				},
				JumpTarget: &StatementJumpTarget{
					After:     true,
					Statement: elseInstructions[len(elseInstructions)-1],
				},
			}
			results = append(results, afterElseJump)
			afterIfTarget.Statement = afterElseJump

			results = append(results, elseInstructions...)
		}

		break
	case *ast.AssignStmt:
		assignMlog, err := assignStmtToMLOG(statement.(*ast.AssignStmt), options)
		if err != nil {
			return nil, err
		}
		results = append(results, assignMlog...)
		break
	case *ast.ReturnStmt:
		returnStmt := statement.(*ast.ReturnStmt)

		if len(returnStmt.Results) > 1 {
			// TODO Multi-value returns
			return nil, errors.New("only single value returns are supported")
		}

		if len(returnStmt.Results) > 0 {
			returnValue := returnStmt.Results[0]

			var resultVar Resolvable
			if ident, ok := returnValue.(*ast.Ident); ok {
				resultVar = &NormalVariable{Name: ident.Name}
			} else if basicLit, ok := returnValue.(*ast.BasicLit); ok {
				resultVar = &Value{Value: basicLit.Value}
			} else if expr, ok := returnValue.(ast.Expr); ok {
				dVar := &DynamicVariable{}

				instructions, err := expressionToMLOG([]Resolvable{dVar}, expr, options)
				if err != nil {
					return nil, err
				}

				results = append(results, instructions...)
				resultVar = dVar
			} else {
				return nil, errors.New(fmt.Sprintf("unknown return value type: %T", returnValue))
			}

			results = append(results, &MLOG{
				Comment: "Set return data",
				Statement: [][]Resolvable{
					{
						&Value{Value: "set"},
						&Value{Value: FunctionReturnVariable},
						resultVar,
					},
				},
			})
		}

		results = append(results, &MLOGTrampolineBack{})
		break
	case *ast.BlockStmt:
		blockStmt := statement.(*ast.BlockStmt)
		for _, s := range blockStmt.List {
			instructions, err := statementToMLOG(s, options)
			if err != nil {
				return nil, err
			}
			results = append(results, instructions...)
		}
		break
	case *ast.IncDecStmt:
		incDecStatement := statement.(*ast.IncDecStmt)
		name := &NormalVariable{Name: incDecStatement.X.(*ast.Ident).Name}
		op := "add"
		if incDecStatement.Tok == token.DEC {
			op = "sub"
		}
		results = append(results, &MLOG{
			Comment: "Execute for loop post condition increment/decrement",
			Statement: [][]Resolvable{
				{
					&Value{Value: "op"},
					&Value{Value: op},
					name,
					name,
					&Value{Value: "1"},
				},
			},
		})
		break
	default:
		return nil, errors.New(fmt.Sprintf("statement type not supported: %T", statement))
	}

	return results, nil
}

func assignStmtToMLOG(statement *ast.AssignStmt, options Options) ([]MLOGStatement, error) {
	mlog := make([]MLOGStatement, 0)

	if len(statement.Lhs) != len(statement.Rhs) {
		if len(statement.Rhs) == 1 {
			leftSide := make([]Resolvable, len(statement.Lhs))

			for i, lhs := range statement.Lhs {
				leftSide[i] = &NormalVariable{Name: lhs.(*ast.Ident).Name}
			}

			exprMLOG, err := expressionToMLOG(leftSide, statement.Rhs[0], options)
			if err != nil {
				return nil, err
			}
			mlog = append(mlog, exprMLOG...)
		} else {
			return nil, errors.New("mismatched variable assignment sides")
		}
	} else {
		for i, expr := range statement.Lhs {
			if ident, ok := expr.(*ast.Ident); ok {
				if statement.Tok != token.ASSIGN && statement.Tok != token.DEFINE {
					return nil, errors.New("only direct assignment is supported")
				}

				exprMLOG, err := expressionToMLOG([]Resolvable{&NormalVariable{Name: ident.Name}}, statement.Rhs[i], options)
				if err != nil {
					return nil, err
				}
				mlog = append(mlog, exprMLOG...)
			} else {
				return nil, errors.New("left side variable assignment can only contain identifications")
			}
		}
	}

	return mlog, nil
}
