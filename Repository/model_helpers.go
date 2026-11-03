package Repository

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type modelField struct {
	Name   string
	Column string
	Index  int
}

// Anahtar: modelin tipi.
// Değer: reflection ile çıkarılan alan bilgileri.
var modelFieldsCache sync.Map

// T modelindeki SQL'e ait alanları çıkarır.
// Bu örnekte T, embedded alan içermeyen düz bir struct olmalı.
func buildModelFields[T any]() ([]modelField, error) {
	modelType := reflect.TypeOf((*T)(nil)).Elem()

	if modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("T bir struct olmalı")
	}

	fields := make([]modelField, 0, modelType.NumField())
	columns := make(map[string]bool)

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Private alanları ve db:"-" alanlarını atla.
		if field.PkgPath != "" || field.Tag.Get("db") == "-" {
			continue
		}

		if field.Anonymous {
			return nil, fmt.Errorf(
				"embedded alan desteklenmiyor: %s",
				field.Name,
			)
		}

		column := field.Tag.Get("db")
		if column == "" {
			column = field.Name
		}

		columnKey := strings.ToLower(column)

		if columns[columnKey] {
			return nil, fmt.Errorf("tekrarlanan SQL kolonu: %s", column)
		}

		columns[columnKey] = true

		fields = append(fields, modelField{
			Name:   field.Name,
			Column: column,
			Index:  i,
		})
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("modelde SQL alanı bulunamadı")
	}

	return fields, nil
}

// Filtre alanını modelden bulur.
// Kullanıcıdan gelen alan adını doğrudan SQL'e eklemeyeceğiz.
func findModelField(
	fields []modelField,
	name string,
) (modelField, error) {
	for _, field := range fields {
		if strings.EqualFold(field.Name, name) ||
			strings.EqualFold(field.Column, name) {
			return field, nil
		}
	}

	return modelField{}, fmt.Errorf("modelde alan bulunamadı: %s", name)
}

// SQL Server tablo ve kolon adlarını köşeli paranteze alır.
func quoteIdentifier(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// SELECT kolonları, fields ile aynı sırada olmalı.
func scanModel[T any](
	rows *sql.Rows,
	fields []modelField,
) (T, error) {
	var entity T

	value := reflect.ValueOf(&entity).Elem()
	destinations := make([]any, len(fields))

	for i, field := range fields {
		destinations[i] = value.Field(field.Index).Addr().Interface()
	}

	err := rows.Scan(destinations...)

	return entity, err
}
func getModelFields[T any]() ([]modelField, error) {
	modelType := reflect.TypeOf((*T)(nil)).Elem()

	if cached, ok := modelFieldsCache.Load(modelType); ok {
		return cached.([]modelField), nil
	}

	fields, err := buildModelFields[T]()
	if err != nil {
		return nil, err
	}

	actual, _ := modelFieldsCache.LoadOrStore(modelType, fields)

	return actual.([]modelField), nil
}
