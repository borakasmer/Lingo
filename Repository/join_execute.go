package Repository

import (
	"context"
	"database/sql"
	"fmt"
)

// JoinFindMany, JOIN sonucunu R tipindeki model listesine okur.
// Kayıt yoksa boş liste döndürür.
func joinFindMany[R any](
	ctx context.Context,
	query *joinQuery,
) ([]R, error) {
	rows, fields, err := executeJoin[R](ctx, query, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]R, 0)

	for rows.Next() {
		entity, err := scanModel[R](rows, fields)
		if err != nil {
			return nil, fmt.Errorf("JOIN sonucu okunamadı: %w", err)
		}

		results = append(results, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// JoinFindOne, JOIN sonucundan tek kayıt okur.
// Kayıt yoksa sql.ErrNoRows döndürür.
//
// Sıralama belirtilmediği için birden fazla eşleşme varsa
// hangi kaydın geleceği garanti edilmez.
func joinFindOne[R any](
	ctx context.Context,
	query *joinQuery,
) (R, error) {
	var zero R

	rows, fields, err := executeJoin[R](ctx, query, true)
	if err != nil {
		return zero, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return zero, err
		}

		return zero, sql.ErrNoRows
	}

	entity, err := scanModel[R](rows, fields)
	if err != nil {
		return zero, fmt.Errorf("JOIN sonucu okunamadı: %w", err)
	}

	if err := rows.Err(); err != nil {
		return zero, err
	}

	return entity, nil
}

// SQL'i üretir, sonuç alanlarını doğrular ve sorguyu çalıştırır.
func executeJoin[R any](
	ctx context.Context,
	query *joinQuery,
	one bool,
) (*sql.Rows, []modelField, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("context nil olamaz")
	}

	if query == nil {
		return nil, nil, fmt.Errorf("JOIN sorgusu nil olamaz")
	}

	if query.db == nil {
		return nil, nil, fmt.Errorf("SQL bağlantısı nil olamaz")
	}

	sqlText, args, err := query.buildSQL()
	if err != nil {
		return nil, nil, err
	}

	fields, err := joinResultFields[R](query.columns)
	if err != nil {
		return nil, nil, err
	}

	if one {
		// SQL() her zaman "SELECT " ile başlayan sorgu üretir.
		// Yalnızca tek kayıt istendiğinde SQL Server TOP kullanılır.
		sqlText = "SELECT TOP (1) " + sqlText[len("SELECT "):]
	}

	rows, err := query.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("JOIN sorgusu başarısız: %w", err)
	}

	return rows, fields, nil
}

// SELECT sırasını sonuç modelinin alanlarıyla eşleştirir.
// Alan bilgilerini mevcut ortak cache üzerinden alır.
//
// Sonuç modelinde seçilmeyen alanlar varsa sıfır değerinde kalır.
// SELECT'teki her As değeri sonuç modelinde bulunmalıdır.
func joinResultFields[R any](
	columns []SelectColumn,
) ([]modelField, error) {
	modelFields, err := getModelFields[R]()
	if err != nil {
		return nil, err
	}

	fields := make([]modelField, 0, len(columns))
	usedFields := make(map[int]bool)

	for _, column := range columns {
		field, err := findModelField(modelFields, column.As)
		if err != nil {
			return nil, fmt.Errorf(
				"JOIN sonuç alanı %q: %w",
				column.As,
				err,
			)
		}

		if usedFields[field.Index] {
			return nil, fmt.Errorf(
				"sonuç alanı birden fazla kez seçilmiş: %s",
				field.Name,
			)
		}

		usedFields[field.Index] = true
		fields = append(fields, field)
	}

	return fields, nil
}
