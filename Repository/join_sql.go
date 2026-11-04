package Repository

import (
	"database/sql"
	"fmt"
	"strings"
)

// SQL, sorguyu ve parametrelerini üretir.
// Veritabanında çalıştırmaz.
func (query *joinQuery) buildSQL() (string, []any, error) {
	if query.err != nil {
		return "", nil, query.err
	}

	if len(query.columns) == 0 {
		return "", nil, fmt.Errorf("en az bir SELECT kolonu gerekli")
	}

	// SELECT ve WHERE tüm katılan tabloları görebilir.
	allSources := []joinSource{query.base}

	for _, join := range query.joins {
		allSources = append(allSources, join.source)
	}

	selectParts := make([]string, 0, len(query.columns))
	resultNames := make(map[string]bool)

	for _, selected := range query.columns {
		column, err := resolveJoinColumn(allSources, selected.Field)
		if err != nil {
			return "", nil, err
		}

		// Sonuç modelindeki kolon adı açıkça verilmeli.
		if err := validateJoinAlias(selected.As); err != nil {
			return "", nil, fmt.Errorf(
				"geçersiz SELECT sonuç adı %q: %w",
				selected.As,
				err,
			)
		}

		key := strings.ToLower(selected.As)

		if resultNames[key] {
			return "", nil, fmt.Errorf(
				"tekrarlanan SELECT sonuç adı: %s",
				selected.As,
			)
		}

		resultNames[key] = true

		selectParts = append(
			selectParts,
			column+" AS "+quoteIdentifier(selected.As),
		)
	}

	var builder strings.Builder

	builder.WriteString("SELECT ")
	builder.WriteString(strings.Join(selectParts, ", "))
	builder.WriteString(" FROM ")
	builder.WriteString(activeJoinSource(query.base))

	args := make([]any, 0)

	// Her ON yalnızca ana tabloyu ve o ana kadar eklenen
	// JOIN tablolarını görebilir.
	visibleSources := []joinSource{query.base}

	for _, join := range query.joins {
		switch join.kind {
		case joinInner, joinLeft, joinRight:
		default:
			return "", nil, fmt.Errorf(
				"geçersiz JOIN türü: %q",
				join.kind,
			)
		}

		visibleSources = append(visibleSources, join.source)

		condition, values, err := buildJoinConditions(
			visibleSources,
			len(args)+1,
			join.on...,
		)
		if err != nil {
			return "", nil, fmt.Errorf(
				"%s ON koşulu: %w",
				join.source.alias,
				err,
			)
		}

		if condition == "" {
			return "", nil, fmt.Errorf(
				"%s için ON koşulu boş olamaz",
				join.source.alias,
			)
		}

		builder.WriteString(" ")
		builder.WriteString(string(join.kind))
		builder.WriteString(" ")
		builder.WriteString(activeJoinSource(join.source))
		builder.WriteString(" ON ")
		builder.WriteString(condition)

		args = append(args, values...)
	}

	where, values, err := buildJoinConditions(
		allSources,
		len(args)+1,
		query.filters...,
	)
	if err != nil {
		return "", nil, err
	}

	if where != "" {
		builder.WriteString(" WHERE ")
		builder.WriteString(where)
	}

	args = append(args, values...)

	return builder.String(), args, nil
}

// Modelde IsDeleted varsa tabloyu JOIN öncesinde filtreler.
// Alan yoksa ilgili tabloya soft-delete koşulu eklemez.
//
// Böylece LEFT / RIGHT JOIN ile oluşan NULL sonuçlar,
// sonradan eklenen otomatik WHERE koşullarıyla elenmez.
func activeJoinSource(source joinSource) string {
	alias := quoteIdentifier(source.alias)

	for _, field := range source.fields {
		if strings.EqualFold(field.Name, "IsDeleted") ||
			strings.EqualFold(field.Column, "IsDeleted") {
			return fmt.Sprintf(
				"(SELECT * FROM %s WHERE ISNULL(%s, 0) = 0) AS %s",
				source.table,
				quoteIdentifier(field.Column),
				alias,
			)
		}
	}

	return source.table + " AS " + alias
}

// "u.IdRole" gibi alanları model bilgisi üzerinden doğrular.
// Tablo takma adı zorunludur; belirsiz kolon kabul edilmez.
func resolveJoinColumn(
	sources []joinSource,
	name string,
) (string, error) {
	parts := strings.Split(name, ".")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf(
			"JOIN alanı alias.Alan biçiminde olmalı: %q",
			name,
		)
	}

	for _, source := range sources {
		if !strings.EqualFold(source.alias, parts[0]) {
			continue
		}

		field, err := findModelField(source.fields, parts[1])
		if err != nil {
			return "", err
		}

		return quoteIdentifier(source.alias) + "." +
			quoteIdentifier(field.Column), nil
	}

	return "", fmt.Errorf(
		"tablo takma adı bulunamadı veya henüz JOIN'e katılmadı: %s",
		parts[0],
	)
}

// Mevcut Filter, And ve Or tiplerini kullanır.
// ColumnRef kolon karşılaştırması; diğer değerler SQL parametresidir.
func buildJoinConditions(
	sources []joinSource,
	startIndex int,
	expressions ...Expr,
) (string, []any, error) {
	if startIndex < 1 {
		return "", nil, fmt.Errorf(
			"başlangıç parametre numarası en az 1 olmalı",
		)
	}

	args := make([]any, 0)
	paramIndex := startIndex

	var build func(Expr) (string, error)

	build = func(expression Expr) (string, error) {
		switch expr := expression.(type) {
		case Filter:
			column, err := resolveJoinColumn(sources, expr.Field)
			if err != nil {
				return "", err
			}

			operator, err := expr.Op.SQL()
			if err != nil {
				return "", err
			}

			if reference, ok := expr.Value.(ColumnRef); ok {
				otherColumn, err := resolveJoinColumn(
					sources,
					reference.Field,
				)
				if err != nil {
					return "", err
				}

				return column + " " + operator + " " +
					otherColumn, nil
			}

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

			return column + " " + operator + " @" + paramName, nil

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
				// Öğesiz OR (ör. boş bir id listesinden) hiçbir satırla eşleşmez;
				// atlanırsa çevreleyen AND koşulu sessizce genişler.
				if expr.Logic == LogicOr && len(expr.Items) == 0 {
					return "1 = 0", nil
				}

				return "", nil
			}

			return "(" + strings.Join(
				parts,
				" "+string(expr.Logic)+" ",
			) + ")", nil

		default:
			return "", fmt.Errorf(
				"desteklenmeyen JOIN ifade tipi: %T",
				expression,
			)
		}
	}

	condition, err := build(And(expressions...))
	if err != nil {
		return "", nil, err
	}

	return condition, args, nil
}
