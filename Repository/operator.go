package Repository

import "fmt"

type Operator string

const (
	OpEq   Operator = "="
	OpNe   Operator = "<>"
	OpGt   Operator = ">"
	OpGte  Operator = ">="
	OpLt   Operator = "<"
	OpLte  Operator = "<="
	OpLike Operator = "LIKE"
)

// SQL yalnızca tanımlı operatörleri kabul eder.
func (op Operator) SQL() (string, error) {
	switch op {
	case OpEq, OpNe, OpGt, OpGte, OpLt, OpLte, OpLike:
		return string(op), nil
	default:
		return "", fmt.Errorf("geçersiz operatör: %q", op)
	}
}
