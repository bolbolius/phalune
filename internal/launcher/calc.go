package launcher

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"
	"strings"
)

// EvaluateMath attempts to evaluate a safe arithmetic expression.
// Returns formatted result (or empty string if not a valid math expression).
func EvaluateMath(expr string) (string, bool) {
	trimmed := strings.TrimSpace(expr)
	if len(trimmed) < 2 {
		return "", false
	}

	// Must contain at least one arithmetic operator
	if !strings.ContainsAny(trimmed, "+-*/%^") {
		return "", false
	}

	// Replace ^ with ** or handle in evaluator
	sanitized := strings.ReplaceAll(trimmed, "×", "*")
	sanitized = strings.ReplaceAll(sanitized, "÷", "/")

	// Reject letters to prevent identifier abuse
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			return "", false
		}
	}

	parsed, err := parser.ParseExpr(sanitized)
	if err != nil {
		return "", false
	}

	val, ok := evalNode(parsed, 0)
	if !ok || math.IsNaN(val) || math.IsInf(val, 0) {
		return "", false
	}

	// Format cleanly (e.g. 42 instead of 42.0000)
	if val == math.Trunc(val) && math.Abs(val) < 1e15 {
		return fmt.Sprintf("%d", int64(val)), true
	}
	return fmt.Sprintf("%.6g", val), true
}

func evalNode(node ast.Node, depth int) (float64, bool) {
	if depth > 50 {
		return 0, false
	}

	switch n := node.(type) {
	case *ast.BasicLit:
		if n.Kind == token.INT || n.Kind == token.FLOAT {
			v, err := strconv.ParseFloat(n.Value, 64)
			return v, err == nil
		}
	case *ast.ParenExpr:
		return evalNode(n.X, depth+1)
	case *ast.UnaryExpr:
		val, ok := evalNode(n.X, depth+1)
		if !ok {
			return 0, false
		}
		if n.Op == token.SUB {
			return -val, true
		} else if n.Op == token.ADD {
			return val, true
		}
	case *ast.BinaryExpr:
		left, okL := evalNode(n.X, depth+1)
		right, okR := evalNode(n.Y, depth+1)
		if !okL || !okR {
			return 0, false
		}
		switch n.Op {
		case token.ADD:
			return left + right, true
		case token.SUB:
			return left - right, true
		case token.MUL:
			return left * right, true
		case token.QUO:
			if right == 0 {
				return 0, false
			}
			return left / right, true
		case token.REM:
			if right == 0 {
				return 0, false
			}
			return math.Mod(left, right), true
		case token.XOR: // Treat ^ as power if integers or standard power
			return math.Pow(left, right), true
		}
	}
	return 0, false
}
