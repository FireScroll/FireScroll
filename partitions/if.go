package partitions

import (
	"fmt"
	"github.com/antonmedv/expr"
)

var (
	testExprEnv = expr.Env(map[string]any{
		"pk":          "",
		"sk":          "",
		"data":        map[string]any{},
		"_updated_at": 0,
		"_created_at": 0,
	})
	exprOpts = []expr.Option{
		testExprEnv,
		expr.AllowUndefinedVariables(), // Allow to use undefined variables.
		expr.AsBool(),                  // the result of the program should be a boolean
	}
)

// VerifyIfStatement compiles the statement to verify that it will work against the types that a document can represent.
// Specifically, it requires that the result is a boolean value.
func VerifyIfStatement(statement string) error {
	_, err := expr.Compile(statement, exprOpts...)
	return err
}

type compareRecord map[string]any

func CompareIfStatement(statement string, cmpr compareRecord) (bool, error) {
	program, err := expr.Compile(statement, exprOpts...)
	if err != nil {
		return false, fmt.Errorf("error in expr.Compile: %w", err)
	}

	fmt.Println("checking program", statement, "against", cmpr)
	output, err := expr.Run(program, map[string]any(cmpr))
	if err != nil {
		return false, fmt.Errorf("error in expr.Run: %w", err)
	}
	return output.(bool), nil
}
