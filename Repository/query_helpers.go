package Repository

import (
	"database/sql"
	"fmt"
	"strings"
)

// startIndex, parametre numaralandırmasının başlangıcıdır.
// SELECT için 1; UPDATE'te SET parametrelerinden sonraki sayı verilir.
func buildWhere(
	fields []modelField,
	startIndex int,
	expressions ...Expr,
) (string, []any, error) {
	if startIndex < 1 {
		return "", nil, fmt.Errorf("başlangıç parametre numarası en az 1 olmalı")
	}

	args := make([]any, 0)
	paramIndex := startIndex

	var build func(Expr) (string, error)

	build = func(expression Expr) (string, error) {
		switch expr := expression.(type) {
		case Filter:
			field, err := findModelField(fields, expr.Field)
			if err != nil {
				return "", err
			}

			operator, err := expr.Op.SQL()
			if err != nil {
				return "", err
			}

			column := quoteIdentifier(field.Column)

			// SQL'de NULL karşılaştırması = ile yapılmaz.
			if expr.Value == nil {
				switch expr.Op {
				case OpEq:
					return column + " IS NULL", nil
				case OpNe:
					return column + " IS NOT NULL", nil
				default:
					return "", fmt.Errorf(
						"NULL yalnızca OpEq veya OpNe ile kullanılabilir",
					)
				}
			}

			paramName := fmt.Sprintf("p%d", paramIndex)
			paramIndex++

			args = append(args, sql.Named(paramName, expr.Value))

			return fmt.Sprintf(
				"%s %s @%s",
				column,
				operator,
				paramName,
			), nil

		case Group:
			if expr.Logic != LogicAnd && expr.Logic != LogicOr {
				return "", fmt.Errorf(
					"geçersiz mantıksal operatör: %q",
					expr.Logic,
				)
			}

			parts := make([]string, 0, len(expr.Items))

			for _, item := range expr.Items {
				part, err := build(item)
				if err != nil {
					return "", err
				}

				if part != "" {
					parts = append(parts, part)
				}
			}

			if len(parts) == 0 {
				return "", nil
			}

			separator := " " + string(expr.Logic) + " "

			return "(" + strings.Join(parts, separator) + ")", nil

		default:
			return "", fmt.Errorf(
				"desteklenmeyen ifade tipi: %T",
				expression,
			)
		}
	}

	// Ayrı verilen filtreler varsayılan olarak AND ile birleşir.
	condition, err := build(And(expressions...))
	if err != nil {
		return "", nil, err
	}

	if condition == "" {
		return "", args, nil
	}

	return " WHERE " + condition, args, nil
}
