package Repository

import (
	"database/sql"
	"fmt"
	"strings"
)

// JoinQuery, tek bir JOIN sorgusunun bilgilerini tutar.
// Her sorgu için repo.Join(...) ile yeni bir nesne oluşturulur.
type joinQuery struct {
	db      *sql.DB
	base    joinSource
	joins   []joinClause
	filters []Expr
	columns []SelectColumn
	err     error
}

// JoinSource, repository'nin tablo ve cache edilmiş alan bilgilerini taşır.
func (repo *Repository[T]) newJoinSource(alias string) joinSource {
	return joinSource{
		table:  repo.table,
		alias:  alias,
		fields: repo.fields,
	}
}

// Join, ana tablo için yeni bir JOIN sorgusu başlatır.
//
// Örnek:
// query := userRepo.Join("u")
func (repo *Repository[T]) newJoinQuery(alias string) *joinQuery {
	query := &joinQuery{
		db:   repo.db,
		base: repo.newJoinSource(alias),
	}

	query.err = validateJoinAlias(alias)

	return query
}

func (query *joinQuery) innerJoin(
	source joinSource,
	on ...Expr,
) *joinQuery {
	return query.addJoin(joinInner, source, on...)
}

func (query *joinQuery) leftJoin(
	source joinSource,
	on ...Expr,
) *joinQuery {
	return query.addJoin(joinLeft, source, on...)
}

func (query *joinQuery) rightJoin(
	source joinSource,
	on ...Expr,
) *joinQuery {
	return query.addJoin(joinRight, source, on...)
}

func (query *joinQuery) addJoin(
	kind joinType,
	source joinSource,
	on ...Expr,
) *joinQuery {
	if query.err != nil {
		return query
	}

	switch kind {
	case joinInner, joinLeft, joinRight:
	default:
		query.err = fmt.Errorf("geçersiz JOIN türü: %q", kind)
		return query
	}

	if err := validateJoinAlias(source.alias); err != nil {
		query.err = err
		return query
	}

	if source.table == "" || len(source.fields) == 0 {
		query.err = fmt.Errorf(
			"JOIN kaynağı repository.JoinSource ile oluşturulmalı",
		)
		return query
	}

	if strings.EqualFold(query.base.alias, source.alias) {
		query.err = fmt.Errorf(
			"tekrarlanan tablo takma adı: %s",
			source.alias,
		)
		return query
	}

	for _, join := range query.joins {
		if strings.EqualFold(join.source.alias, source.alias) {
			query.err = fmt.Errorf(
				"tekrarlanan tablo takma adı: %s",
				source.alias,
			)
			return query
		}
	}

	if len(on) == 0 {
		query.err = fmt.Errorf("JOIN için ON koşulu gerekli")
		return query
	}

	query.joins = append(query.joins, joinClause{
		kind:   kind,
		source: source,
		on:     append([]Expr(nil), on...),
	})

	return query
}

// Where, mevcut Filter / And / Or ifadelerini kabul eder.
// Ayrı verilen ifadeler AND ile birleştirilecek.
func (query *joinQuery) where(expressions ...Expr) *joinQuery {
	if query.err == nil {
		query.filters = append(query.filters, expressions...)
	}

	return query
}

// Select, sonuç modeline aktarılacak kolonları belirler.
func (query *joinQuery) selectColumns(columns ...SelectColumn) *joinQuery {
	if query.err == nil {
		query.columns = append(query.columns, columns...)
	}

	return query
}

// Err, sorgu kurulurken oluşan ilk hatayı döndürür.
func (query *joinQuery) getError() error {
	return query.err
}

// Takma adlar harf veya _ ile başlamalı.
// Devamında harf, rakam ve _ kullanılabilir.
func validateJoinAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("tablo takma adı boş olamaz")
	}

	for i, char := range alias {
		letter := char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z'

		digit := char >= '0' && char <= '9'

		if letter || char == '_' || (i > 0 && digit) {
			continue
		}

		return fmt.Errorf("geçersiz tablo takma adı: %q", alias)
	}

	return nil
}
