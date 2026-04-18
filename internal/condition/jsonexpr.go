package condition

import "github.com/pbsladek/wait-for/internal/expr"

func EvaluateJSONExpression(body []byte, raw string) (bool, string, error) {
	return expr.EvaluateJSON(body, raw)
}
