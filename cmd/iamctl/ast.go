package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"text/template"

	"golang.org/x/tools/go/ast/astutil"
)

func buildStrings(input interface{}) ast.Expr {
	switch val := input.(type) {
	case string:
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: "\"" + val + "\"",
		}
	case []string:
		ret := make([]ast.Expr, 0, len(val))
		for _, s := range val {
			ret = append(ret, buildStrings(s))
		}
		return &ast.CompositeLit{
			Type: ast.NewIdent("[]string"),
			Elts: ret,
		}
	case AWSValue:
		ret := make([]ast.Expr, 0, len(val))
		for _, s := range val {
			ret = append(ret, buildStrings(s))
		}
		return &ast.CompositeLit{
			Type: ast.NewIdent("[]string"),
			Elts: ret,
		}
	default:
		panic("unsported type for string expr")
	}
}

func buildKeyValueExpr(input interface{}) ast.Expr {
	switch val := input.(type) {
	case *iamPolicyCondition:
		if val == nil {
			return ast.NewIdent("nil")
		}
		exprs := make([]ast.Expr, 0, 1)

		for k, v := range *val {
			exprs = append(exprs, &ast.KeyValueExpr{
				Key:   buildStrings(k),
				Value: buildKeyValueExpr(v),
			})
		}
		return &ast.CompositeLit{
			Type: &ast.SelectorExpr{
				X:   ast.NewIdent("cco"),
				Sel: ast.NewIdent("IAMPolicyCondition"),
			},
			Elts: exprs,
		}
	case iamPolicyConditionKeyValue:
		exprs := make([]ast.Expr, 0, 1)
		for k, v := range val {
			exprs = append(exprs, &ast.KeyValueExpr{
				Key:   buildStrings(k),
				Value: buildStrings(v),
			})
		}
		return &ast.CompositeLit{
			Type: &ast.SelectorExpr{
				X:   ast.NewIdent("cco"),
				Sel: ast.NewIdent("IAMPolicyConditionKeyValue"),
			},
			Elts: exprs,
		}
	default:
		panic("unsported type for key/val expr")
	}
}

func generateIAMPolicy(input, output, pkg string) {
	tmpl, err := template.New("").Parse(filetemplate)
	if err != nil {
		panic(err)
	}

	var in bytes.Buffer
	err = tmpl.Execute(&in, pkg)
	if err != nil {
		panic(err)
	}

	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "", in.String(), 0)
	if err != nil {
		panic(fmt.Errorf("failed to parse template %v", err))
	}

	jsFs, err := ioutil.ReadFile(input)
	if err != nil {
		panic(fmt.Errorf("failed to read input file %v", err))
	}

	policy := iamPolicy{}

	err = json.Unmarshal([]byte(jsFs), &policy)
	if err != nil {
		panic(fmt.Errorf("failed to parse policy JSON %v", err))
	}

	// Minifying here as a workaround for current limitations
	// in credential requests length (2048 max bytes).
	miniPoliicy := minify(policy)

	exprs := make([]ast.Expr, 0, len(miniPoliicy.Statement))
	for _, p := range miniPoliicy.Statement {
		// Workaround since cloud credential operator doesn't
		// support multiple resources in its spec.
		for _, r := range p.Resource {
			policyList := make([]ast.Expr, 4)

			policyList[0] = &ast.KeyValueExpr{
				Key:   ast.NewIdent(effect),
				Value: buildStrings(p.Effect),
			}
			policyList[1] = &ast.KeyValueExpr{
				Key:   ast.NewIdent(action),
				Value: buildStrings(p.Action),
			}

			policyList[2] = &ast.KeyValueExpr{
				Key:   ast.NewIdent(resource),
				Value: buildStrings(r),
			}

			policyList[3] = &ast.KeyValueExpr{
				Key:   ast.NewIdent(policycondition),
				Value: buildKeyValueExpr(p.Condition),
			}
			exprs = append(exprs, &ast.CompositeLit{Elts: policyList})
		}
	}

	astutil.Apply(file, nil, func(c *astutil.Cursor) bool {
		n := c.Node()
		switch x := n.(type) {
		case *ast.ReturnStmt:
			c.Replace(&ast.ReturnStmt{
				Return: x.Pos(),
				Results: []ast.Expr{
					&ast.CompositeLit{
						Type: ast.NewIdent("IAMPolicy"),
						Elts: []ast.Expr{
							&ast.KeyValueExpr{
								Key:   ast.NewIdent("Version"),
								Value: buildStrings(miniPoliicy.Version),
							},
							&ast.KeyValueExpr{
								Key: ast.NewIdent("Statement"),
								Value: &ast.CompositeLit{
									Type: &ast.ArrayType{
										Elt: &ast.SelectorExpr{
											X:   ast.NewIdent("cco"),
											Sel: ast.NewIdent("StatementEntry"),
										},
									},
									Elts: exprs,
								},
							},
						},
					},
				},
			})
		}

		return true
	})

	opFs, err := os.Create(output)
	if err != nil {
		panic(err)
	}

	printer.Fprint(opFs, fs, file)
}
