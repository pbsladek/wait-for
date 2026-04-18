package expr

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Expression struct {
	comparison jsonComparison
}

type jsonComparison struct {
	path string
	op   string
	want string
}

// Compile supports a deliberately small JSONPath-like subset:
// .field, .field.subfield, .items[0].name, and comparisons with ==, !=, =,
// >=, <=, >, <. Kubernetes-style {.status.phase}=Running is also accepted.
func Compile(raw string) (*Expression, error) {
	cmp, err := parseJSONComparison(raw)
	if err != nil {
		return nil, err
	}
	return &Expression{comparison: cmp}, nil
}

func (e *Expression) EvaluateJSON(body []byte) (bool, string, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return false, "", fmt.Errorf("parse json: %w", err)
	}
	return e.Evaluate(doc)
}

func (e *Expression) Evaluate(doc any) (bool, string, error) {
	cmp := e.comparison
	got, ok, err := lookupJSONPath(doc, cmp.path)
	if err != nil {
		return false, "", err
	}
	if !ok {
		return false, "", nil
	}

	if cmp.op == "" {
		return truthy(got), fmt.Sprintf("%s is %v", cmp.path, got), nil
	}

	want, err := parseLiteral(cmp.want)
	if err != nil {
		return false, "", err
	}
	matched, err := compareValues(got, want, cmp.op)
	if err != nil {
		return false, "", err
	}
	return matched, fmt.Sprintf("%s %s %v", cmp.path, cmp.op, want), nil
}

func EvaluateJSON(body []byte, raw string) (bool, string, error) {
	expression, err := Compile(raw)
	if err != nil {
		return false, "", err
	}
	return expression.EvaluateJSON(body)
}

func parseJSONComparison(expr string) (jsonComparison, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.Trim(expr, "'\"")
	if expr == "" {
		return jsonComparison{}, fmt.Errorf("jsonpath expression is required")
	}

	operators := []string{">=", "<=", "==", "!=", ">", "<", "="}
	for _, op := range operators {
		if idx := strings.Index(expr, op); idx >= 0 {
			path := strings.TrimSpace(expr[:idx])
			want := strings.TrimSpace(expr[idx+len(op):])
			path = strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(path, "{"), "}"), "{")
			path = strings.TrimSuffix(path, "}")
			if path == "" || want == "" {
				return jsonComparison{}, fmt.Errorf("invalid jsonpath comparison %q", expr)
			}
			if op == "=" {
				op = "=="
			}
			return jsonComparison{path: normalizeJSONPath(path), op: op, want: want}, nil
		}
	}

	return jsonComparison{path: normalizeJSONPath(expr)}, nil
}

func normalizeJSONPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "{")
	path = strings.TrimSuffix(path, "}")
	return path
}

func lookupJSONPath(doc any, path string) (any, bool, error) {
	if !strings.HasPrefix(path, ".") {
		return nil, false, fmt.Errorf("jsonpath must start with '.' or '{.': %q", path)
	}
	cur := doc
	remaining := strings.TrimPrefix(path, ".")
	for remaining != "" {
		part, rest, _ := strings.Cut(remaining, ".")
		field, indexes, err := splitJSONPathPart(part)
		if err != nil {
			return nil, false, err
		}
		if field != "" {
			obj, ok := cur.(map[string]any)
			if !ok {
				return nil, false, nil
			}
			cur, ok = obj[field]
			if !ok {
				return nil, false, nil
			}
		}
		for _, idx := range indexes {
			arr, ok := cur.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return nil, false, nil
			}
			cur = arr[idx]
		}
		remaining = rest
	}
	return cur, true, nil
}

func splitJSONPathPart(part string) (string, []int, error) {
	field, rest, _ := strings.Cut(part, "[")
	var indexes []int
	for rest != "" {
		raw, tail, ok := strings.Cut(rest, "]")
		if !ok {
			return "", nil, fmt.Errorf("invalid jsonpath index in %q", part)
		}
		idx, err := strconv.Atoi(raw)
		if err != nil {
			return "", nil, fmt.Errorf("invalid jsonpath index %q", raw)
		}
		indexes = append(indexes, idx)
		rest = strings.TrimPrefix(tail, "[")
		if tail != "" && !strings.HasPrefix(tail, "[") {
			return "", nil, fmt.Errorf("invalid jsonpath segment %q", part)
		}
	}
	return field, indexes, nil
}

func parseLiteral(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "'\"")
	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	if raw == "null" {
		return nil, nil
	}
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return n, nil
	}
	return raw, nil
}

func compareValues(got any, want any, op string) (bool, error) {
	if got == nil || want == nil {
		switch op {
		case "==":
			return got == want, nil
		case "!=":
			return got != want, nil
		default:
			return false, fmt.Errorf("cannot compare null with %s", op)
		}
	}

	if gn, ok := asFloat(got); ok {
		wn, ok := asFloat(want)
		if !ok {
			return false, nil
		}
		switch op {
		case "==":
			return gn == wn, nil
		case "!=":
			return gn != wn, nil
		case ">=":
			return gn >= wn, nil
		case "<=":
			return gn <= wn, nil
		case ">":
			return gn > wn, nil
		case "<":
			return gn < wn, nil
		}
	}

	if gb, ok := got.(bool); ok {
		wb, ok := want.(bool)
		if !ok {
			return false, nil
		}
		switch op {
		case "==":
			return gb == wb, nil
		case "!=":
			return gb != wb, nil
		default:
			return false, fmt.Errorf("operator %s is not supported for booleans", op)
		}
	}

	gs := fmt.Sprint(got)
	ws := fmt.Sprint(want)
	switch op {
	case "==":
		return gs == ws, nil
	case "!=":
		return gs != ws, nil
	default:
		return false, fmt.Errorf("operator %s is only supported for numbers", op)
	}
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		n, err := t.Float64()
		return n, err == nil
	default:
		return 0, false
	}
}

func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	default:
		return !reflect.ValueOf(v).IsZero()
	}
}
