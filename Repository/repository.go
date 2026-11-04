// Package Repository contains the SQL Server repository implementation.
// Put this file alongside the existing operator.go, filter.go, expression.go,
// model_helpers.go and query_helpers.go files. Those helpers are NOT redefined here.
//
// Connection lifecycle: the application calls DB.SqlOpen once and passes its pool
// to NewRepository. The application closes that pool when it shuts down.
package Repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sqlOrm/Models"
	"strings"

	_ "sqlOrm/Models"

	mssql "github.com/microsoft/go-mssqldb"
)

const sqlServerParameterLimit = 2100

type Repository[T any] struct {
	db           *sql.DB
	table        string
	fields       []modelField
	columns      string
	defaultOrder string
}

// NewRepository binds model T to a table. T must be a non-pointer, flat struct.
// The existing helpers support db:"ColumnName" and db:"-" tags.
// Go field Id is treated as an identity: not inserted or updated.
func NewRepository[T any](db *sql.DB, tableName ...string) (*Repository[T], error) {
	if db == nil {
		return nil, fmt.Errorf("SQL bağlantısı nil olamaz")
	}
	fields, err := getModelFields[T]()
	if err != nil {
		return nil, err
	}
	if len(tableName) > 1 {
		return nil, fmt.Errorf("tek tablo adı verilebilir")
	}

	var name string

	if len(tableName) == 1 {
		// Açıkça verilen tablo adı önceliklidir.
		name = tableName[0]
	} else {
		// Hem pointer hem value receiver ile tanımlanmış
		// TableName() metotlarını destekler.
		model, ok := any(new(T)).(interface {
			TableName() string
		})
		if !ok {
			return nil, fmt.Errorf(
				"tablo adı verilmeli veya model TableName() metodunu tanımlamalı",
			)
		}

		name = model.TableName()
	}

	table, err := quoteSQLName(name)
	if err != nil {
		return nil, err
	}

	columns := make([]string, len(fields))
	aliases := make(map[string]int)
	order := fields[0].Column
	for i, field := range fields {
		columns[i] = quoteIdentifier(field.Column)
		if strings.EqualFold(field.Name, "Id") {
			order = field.Column
		}
		for _, alias := range []string{field.Name, field.Column} {
			key := strings.ToLower(alias)
			if other, exists := aliases[key]; exists && other != i {
				return nil, fmt.Errorf("belirsiz model alanı: %s", alias)
			}
			aliases[key] = i
		}
	}
	return &Repository[T]{db: db, table: table, fields: fields, columns: strings.Join(columns, ", "), defaultOrder: order}, nil
}

// quoteSQLName supports Table or Schema.Table. These names are developer-supplied.
func quoteSQLName(name string) (string, error) {
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		return "", fmt.Errorf("SQL adı Name veya Schema.Name olmalı")
	}
	for i, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("SQL adı boş olamaz")
		}
		parts[i] = quoteIdentifier(part)
	}
	return strings.Join(parts, "."), nil
}

type queryOptions struct {
	filters   []Expr
	orderBy   string
	ascending bool
	page      int
	pageSize  int
}

// Query parameters retain the old repository's convenient style:
// Filter/Expr = predicate; string = order field; bool = ASC/DESC;
// one int = pageSize; two ints = page and pageSize.
// Unlike the old implementation, invalid fields and pagination return errors.
func (repo *Repository[T]) parseQuery(params []any) (queryOptions, error) {
	q := queryOptions{orderBy: repo.defaultOrder, ascending: true}
	integers := 0
	for _, param := range params {
		switch value := param.(type) {
		case Expr:
			q.filters = append(q.filters, value)
		case string:
			field, err := findModelField(repo.fields, value)
			if err == nil {
				q.orderBy = field.Column
			} else if strings.EqualFold(value, "CreatedDate") {
				// Modelde tanımlı olmasa da SQL'de sıralamayı deneyebiliriz.
				q.orderBy = "CreatedDate"
			} else {
				return q, err
			}
		case bool:
			q.ascending = value
		case int:
			integers++
			switch integers {
			case 1:
				q.page = value
			case 2:
				q.pageSize = value
			default:
				return q, fmt.Errorf("yalnızca page ve pageSize verilebilir")
			}
		default:
			return q, fmt.Errorf("desteklenmeyen sorgu parametresi: %T", param)
		}
	}
	if integers == 1 {
		q.pageSize, q.page = q.page, 1
	}
	if integers > 0 && (q.page < 1 || q.pageSize < 1) {
		return q, fmt.Errorf("page ve pageSize pozitif olmalı")
	}
	if q.pageSize > 0 && q.page-1 > int(^uint(0)>>1)/q.pageSize {
		return q, fmt.Errorf("sayfalama değeri çok büyük")
	}
	return q, nil
}

func (repo *Repository[T]) selectSQL(one bool, params []any) (string, []any, error) {
	options, err := repo.parseQuery(params)
	if err != nil {
		return "", nil, err
	}
	if one && options.pageSize > 0 {
		return "", nil, fmt.Errorf("FindOne sayfalama parametresi kabul etmez")
	}
	where, args, err := buildWhere(repo.fields, 1, options.filters...)
	if err != nil {
		return "", nil, err
	}
	if where == "" {
		where = " WHERE ISNULL(" + quoteIdentifier("IsDeleted") + ", 0) = 0"
	} else {
		where += " AND ISNULL(" + quoteIdentifier("IsDeleted") + ", 0) = 0"
	}
	prefix := "SELECT "
	if one {
		prefix += "TOP (1) "
	}
	query := prefix + repo.columns + " FROM " + repo.table + where + " ORDER BY " + quoteIdentifier(options.orderBy)
	if options.ascending {
		query += " ASC"
	} else {
		query += " DESC"
	}
	if !one && options.pageSize > 0 {
		query += " OFFSET @offset ROWS FETCH NEXT @pageSize ROWS ONLY"
		args = append(args, sql.Named("offset", (options.page-1)*options.pageSize), sql.Named("pageSize", options.pageSize))
	}
	if len(args) > sqlServerParameterLimit {
		return "", nil, fmt.Errorf("SQL Server parametre sınırı aşıldı")
	}
	return query, args, nil
}

func (repo *Repository[T]) selectRows(
	ctx context.Context,
	one bool,
	params []any,
) ([]T, error) {
	query, args, err := repo.selectSQL(one, params)
	if err != nil {
		return nil, err
	}

	scanFields := repo.fields

	rows, err := repo.db.QueryContext(ctx, query, args...)

	// Orijinaldeki gibi listeleme ve sayfalama için fallback.
	if err != nil && !one && isMissingCreatedDate(err) {
		options, parseErr := repo.parseQuery(params)
		if parseErr != nil {
			return nil, parseErr
		}

		if strings.EqualFold(options.orderBy, "CreatedDate") {
			// Repository'nin ortak alanlarını değiştirmiyoruz.
			fallbackRepo := *repo
			fallbackRepo.fields = make([]modelField, 0, len(repo.fields))

			columns := make([]string, 0, len(repo.fields))

			for _, field := range repo.fields {
				// Kolon modelde tanımlı fakat tabloda yoksa
				// ikinci SELECT'te onu da seçmemeliyiz.
				if strings.EqualFold(field.Column, "CreatedDate") {
					continue
				}

				fallbackRepo.fields = append(
					fallbackRepo.fields,
					field,
				)

				columns = append(
					columns,
					quoteIdentifier(field.Column),
				)
			}

			fallbackRepo.columns = strings.Join(columns, ", ")

			// Son sıralama alanı önceki seçimi geçersiz kılar.
			// Diğer parametreler aynen korunur.
			fallbackParams := make([]any, 0, len(params)+1)
			fallbackParams = append(fallbackParams, params...)
			fallbackParams = append(fallbackParams, "Id")

			query, args, err = fallbackRepo.selectSQL(
				false,
				fallbackParams,
			)
			if err != nil {
				return nil, err
			}

			rows, err = repo.db.QueryContext(ctx, query, args...)
			scanFields = fallbackRepo.fields
		}
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]T, 0)

	for rows.Next() {
		entity, err := scanModel[T](rows, scanFields)
		if err != nil {
			return nil, err
		}

		result = append(result, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// Get/FindMany return [] for an empty result. All mapped fields are selected.
// No soft-delete predicate is injected: pass IsDeleted=false explicitly when needed.
func (repo *Repository[T]) Get(ctx context.Context, params ...any) ([]T, error) {
	return repo.selectRows(ctx, false, params)
}
func (repo *Repository[T]) FindMany(ctx context.Context, params ...any) ([]T, error) {
	return repo.Get(ctx, params...)
}
func (repo *Repository[T]) GetAll(ctx context.Context) ([]T, error) {
	return repo.Get(ctx)
}

// FindOne returns sql.ErrNoRows if there is no match. Default ordering: Id ASC,
// or the first mapped field if the model has no Go field named Id.
func (repo *Repository[T]) FindOne(ctx context.Context, params ...any) (*T, error) {
	result, err := repo.selectRows(ctx, true, params)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, sql.ErrNoRows
	}
	return &result[0], nil
}
func (repo *Repository[T]) GetByID(ctx context.Context, id any) (*T, error) {
	return repo.FindOne(ctx, Filter{Field: "Id", Op: OpEq, Value: id})
}

/*func (repo *Repository[T]) GetAllWithPaging(ctx context.Context, page, pageSize int, params ...any) ([]T, error) {
	args := []any{page, pageSize}
	args = append(args, params...)
	return repo.Get(ctx, args...)
}*/

func (repo *Repository[T]) GetAllWithPaging(
	ctx context.Context,
	page int,
	pageSize int,
	params ...any,
) ([]T, error) {
	if page < 1 {
		page = 1
	}

	if pageSize < 1 {
		pageSize = 10
	}

	if pageSize > 100 {
		pageSize = 100
	}

	args := []any{page, pageSize}
	args = append(args, params...)

	return repo.Get(ctx, args...)
}

// sqlExecutor is implemented by both *sql.DB and *sql.Tx.
type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (repo *Repository[T]) insertFields() []modelField {
	fields := make([]modelField, 0, len(repo.fields))
	for _, field := range repo.fields {
		if !strings.EqualFold(field.Name, "Id") {
			fields = append(fields, field)
		}
	}
	return fields
}
func (repo *Repository[T]) insertSQL(entities []T) (string, []any, error) {
	if len(entities) == 0 {
		return "", nil, fmt.Errorf("eklenecek kayıt bulunamadı")
	}
	fields := repo.insertFields()
	if len(fields) == 0 {
		if len(entities) != 1 {
			return "", nil, fmt.Errorf("DEFAULT VALUES tek kayıtla kullanılmalı")
		}
		return "INSERT INTO " + repo.table + " DEFAULT VALUES", nil, nil
	}
	if len(entities) > 1000 || len(entities) > sqlServerParameterLimit/len(fields) {
		return "", nil, fmt.Errorf("SQL Server toplu INSERT sınırı aşıldı")
	}
	columns := make([]string, len(fields))
	for i, field := range fields {
		columns[i] = quoteIdentifier(field.Column)
	}
	args := make([]any, 0, len(entities)*len(fields))
	groups := make([]string, 0, len(entities))
	for _, entity := range entities {
		value := reflect.ValueOf(entity)
		params := make([]string, len(fields))
		for i, field := range fields {
			name := fmt.Sprintf("p%d", len(args)+1)
			params[i] = "@" + name
			args = append(args, sql.Named(name, value.Field(field.Index).Interface()))
		}
		groups = append(groups, "("+strings.Join(params, ", ")+")")
	}
	query := "INSERT INTO " + repo.table + " (" + strings.Join(columns, ", ") + ") VALUES " + strings.Join(groups, ", ")
	return query, args, nil
}

// Insert returns affected rows, NOT the generated identity value.
func (repo *Repository[T]) Insert(ctx context.Context, entity T) (int64, error) {
	query, args, err := repo.insertSQL([]T{entity})
	if err != nil {
		return 0, err
	}
	result, err := repo.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// BulkInsertBatched issues multi-row INSERT statements in one transaction.
// batchSize<=0 uses the largest valid batch, capped by SQL Server's limits.
func (repo *Repository[T]) BulkInsertBatched(ctx context.Context, entities []T, batchSize int) (int64, error) {
	if len(entities) == 0 {
		return 0, nil
	}
	fieldCount := len(repo.insertFields())
	limit := 1000
	if fieldCount == 0 {
		limit = 1
	} else if sqlServerParameterLimit/fieldCount < limit {
		limit = sqlServerParameterLimit / fieldCount
	}
	if limit < 1 {
		return 0, fmt.Errorf("kolon sayısı parametre sınırını aşıyor")
	}
	if batchSize <= 0 || batchSize > limit {
		batchSize = limit
	}
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var total int64
	for start := 0; start < len(entities); {
		size := batchSize
		if size > len(entities)-start {
			size = len(entities) - start
		}
		end := start + size
		query, args, err := repo.insertSQL(entities[start:end])
		if err != nil {
			return 0, err
		}
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, fmt.Errorf("toplu INSERT başarısız: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += affected
		start = end
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}

// BulkInsert is the optional TVP/stored-procedure variant from the original code.
// The procedure must accept a single TVP parameter (its parameter name can differ).
// T's exported fields must match the SQL table TYPE in order and data type.
// This method passes the full T as-is; it does NOT skip Id or db:"-" fields.
// Use a dedicated TVP row struct/repository if the table TYPE differs from your entity.
// The stored procedure controls the target table; repo.table does not override it.
// Returns error only because stored procedures may use SET NOCOUNT ON.
func (repo *Repository[T]) BulkInsert(ctx context.Context, entities []T, typeName, procName string, size ...int) error {
	if len(entities) == 0 {
		return nil
	}
	if len(size) > 1 {
		return fmt.Errorf("tek batchSize verilebilir")
	}
	batchSize := 1000
	if len(size) == 1 {
		if size[0] <= 0 {
			return fmt.Errorf("batchSize pozitif olmalı")
		}
		batchSize = size[0]
	}
	if _, err := quoteSQLName(typeName); err != nil {
		return fmt.Errorf("TVP tipi: %w", err)
	}
	procedure, err := quoteSQLName(procName)
	if err != nil {
		return err
	}
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for start := 0; start < len(entities); {
		count := batchSize
		if count > len(entities)-start {
			count = len(entities) - start
		}
		end := start + count
		tvp := mssql.TVP{TypeName: typeName, Value: entities[start:end]}
		// @Data is a bound local parameter, passed positionally to the procedure.
		if _, err := tx.ExecContext(ctx, "EXEC "+procedure+" @Data", sql.Named("Data", tvp)); err != nil {
			return err
		}
		start = end
	}
	return tx.Commit()
}

func (repo *Repository[T]) updateSQL(updates map[string]any, filters []Expr) (string, []any, error) {
	// Resolve aliases before sorting to keep statement/parameter order deterministic.
	values := make(map[string]any, len(updates))
	for name, value := range updates {
		field, err := findModelField(repo.fields, name)
		if err != nil {
			return "", nil, err
		}
		if strings.EqualFold(field.Name, "Id") {
			continue
		}
		if _, exists := values[field.Column]; exists {
			return "", nil, fmt.Errorf("tekrarlanan kolon: %s", field.Column)
		}
		values[field.Column] = value
	}
	columns := make([]string, 0, len(values))
	for column := range values {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	args := make([]any, 0, len(columns))
	set := make([]string, 0, len(columns))
	for _, column := range columns {
		if raw, ok := values[column].(Models.RawValue); ok {
			if strings.TrimSpace(raw.Expr) == "" {
				return "", nil, fmt.Errorf("RawValue boş olamaz")
			}
			// RawValue is trusted developer SQL, never client/request input.
			set = append(set, quoteIdentifier(column)+" = "+raw.Expr)
			continue
		}
		name := fmt.Sprintf("p%d", len(args)+1)
		set = append(set, quoteIdentifier(column)+" = @"+name)
		args = append(args, sql.Named(name, values[column]))
	}
	where, filterArgs, err := buildWhere(repo.fields, len(args)+1, filters...)
	if err != nil {
		return "", nil, err
	}
	if where == "" {
		return "", nil, fmt.Errorf("Update için filtre belirtilmeli")
	}
	where += " AND ISNULL(" + quoteIdentifier("IsDeleted") + ", 0) = 0"
	args = append(args, filterArgs...)
	if len(args) > sqlServerParameterLimit {
		return "", nil, fmt.Errorf("SQL Server parametre sınırı aşıldı")
	}
	if len(set) == 0 {
		return "", nil, nil
	}
	return "UPDATE " + repo.table + " SET " + strings.Join(set, ", ") + where, args, nil
}
func (repo *Repository[T]) update(ctx context.Context, executor sqlExecutor, updates map[string]any, filters ...Expr) (int64, error) {
	query, args, err := repo.updateSQL(updates, filters)
	if err != nil {
		return 0, err
	}
	if query == "" {
		return 0, nil
	}
	result, err := executor.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Update changes only supplied fields, including zero/false/nil values.
func (repo *Repository[T]) Update(ctx context.Context, updates map[string]any, filters ...Expr) (int64, error) {
	if len(updates) == 0 {
		return 0, fmt.Errorf("güncellenecek alan belirtilmeli")
	}
	return repo.update(ctx, repo.db, updates, filters...)
}
func (repo *Repository[T]) BulkUpdateWithFilter(ctx context.Context, updates map[string]any, filters ...Expr) (int64, error) {
	return repo.Update(ctx, updates, filters...)
}
func (repo *Repository[T]) changes(oldEntity, newEntity T) (map[string]any, error) {
	oldValue, newValue := reflect.ValueOf(oldEntity), reflect.ValueOf(newEntity)
	result := make(map[string]any)
	for _, field := range repo.fields {
		before, after := oldValue.Field(field.Index).Interface(), newValue.Field(field.Index).Interface()
		if reflect.DeepEqual(before, after) {
			continue
		}
		if strings.EqualFold(field.Name, "Id") {
			continue
		}
		result[field.Name] = after
	}
	return result, nil
}

// UpdateChanged replaces the old Core.GetChangedFields/ValidateStruct workflow.
// Only changed mapped fields are written. There is no encrypt/decrypt/validation.
// An unchanged entity returns 0,nil, but an explicit valid filter is still required.
// This is not optimistic concurrency control: include a version predicate if needed.
func (repo *Repository[T]) UpdateChanged(ctx context.Context, oldEntity, newEntity T, filters ...Expr) (int64, error) {
	updates, err := repo.changes(oldEntity, newEntity)
	if err != nil {
		return 0, err
	}
	return repo.update(ctx, repo.db, updates, filters...)
}

type UpdateItem struct {
	Values  map[string]any
	Filters []Expr
}

// BatchUpdate executes all items in one transaction. Identical UPDATE statements
// share a prepared statement; every item requires a nonempty explicit predicate.
// No match means 0 affected rows, not an error.
func (repo *Repository[T]) BatchUpdate(ctx context.Context, items []UpdateItem, batchSize ...int) (int64, error) {
	if len(batchSize) > 1 {
		return 0, fmt.Errorf("tek batchSize verilebilir")
	}

	size := len(items)

	if len(batchSize) == 1 && batchSize[0] > 0 {
		size = batchSize[0]
	}

	if len(items) == 0 {
		return 0, nil
	}
	// Validate every item before opening a transaction.
	type command struct {
		query string
		args  []any
	}
	commands := make([]command, 0, len(items))
	for i, item := range items {
		query, args, err := repo.updateSQL(item.Values, item.Filters)
		if err != nil {
			return 0, fmt.Errorf("%d. güncelleme: %w", i+1, err)
		}
		if query != "" {
			commands = append(commands, command{query, args})
		}
	}
	if len(commands) == 0 {
		return 0, nil
	}
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	statements := make(map[string]*sql.Stmt)
	defer func() {
		for _, stmt := range statements {
			stmt.Close()
		}
	}()
	var total int64
	for start := 0; start < len(commands); {
		count := size

		if count > len(commands)-start {
			count = len(commands) - start
		}

		end := start + count

		for _, command := range commands[start:end] {
			stmt, exists := statements[command.query]

			if !exists {
				stmt, err = tx.PrepareContext(ctx, command.query)
				if err != nil {
					return 0, err
				}

				statements[command.query] = stmt
			}

			result, err := stmt.ExecContext(ctx, command.args...)
			if err != nil {
				return 0, err
			}

			affected, err := result.RowsAffected()
			if err != nil {
				return 0, err
			}

			total += affected
		}

		start = end
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}

type UpdatePair[T any] struct {
	Old T
	New T
}

// BatchUpdateChanged preserves the old entity-pair batch update use case.
// identifierField is generic (Id, SKU, UserName, ...), not hardcoded to UserName.
// The OLD identifier is used and changing the identifier is rejected.
// batchSize, UPDATE işlemlerinin gruplama boyutudur.
// Her kayıt ayrı UPDATE olarak çalışır.
// Bütün gruplar aynı transaction içinde, tek commit ile tamamlanır.
func (repo *Repository[T]) BatchUpdateChanged(ctx context.Context, pairs []UpdatePair[T], identifierField string, batchSize ...int) (int64, error) {
	key, err := findModelField(repo.fields, identifierField)
	if err != nil {
		return 0, err
	}
	items := make([]UpdateItem, 0, len(pairs))
	for _, pair := range pairs {
		oldKey := reflect.ValueOf(pair.Old).Field(key.Index).Interface()
		v := reflect.ValueOf(oldKey)
		if oldKey == nil || (v.Kind() == reflect.Ptr && v.IsNil()) {
			return 0, fmt.Errorf("identifier NULL olamaz")
		}
		updates, err := repo.changes(pair.Old, pair.New)
		if err != nil {
			return 0, err
		}
		delete(updates, key.Name)
		items = append(items, UpdateItem{Values: updates, Filters: []Expr{Filter{Field: key.Name, Op: OpEq, Value: oldKey}}})
	}
	return repo.BatchUpdate(ctx, items, batchSize...)
}

func isMissingCreatedDate(err error) bool {
	var sqlErr mssql.Error

	return errors.As(err, &sqlErr) &&
		sqlErr.Number == 207 &&
		strings.Contains(
			strings.ToLower(sqlErr.Message),
			"createddate",
		)
}

// ColumnRef, filtre değerinin başka bir kolon olduğunu belirtir.
//
// Örnek:
//
//	Filter{
//	    Field: "ra.IdGoRole",
//	    Op:    OpEq,
//	    Value: ColumnRef{Field: "u.IdRole"},
//	}
//
// Üretilmesi amaçlanan SQL:
// [ra].[IdGoRole] = [u].[IdRole]
type ColumnRef struct {
	Field string
}

// SelectColumn, kaynak kolonu sonuç modelindeki alanla eşleştirir.
//
// Örnek:
//
//	SelectColumn{
//	    Field: "ra.ActionNumberTotal",
//	    As:    "ActionNumberTotal",
//	}
type SelectColumn struct {
	Field string
	As    string
}

// JoinQuery, JOIN sorgularının dışarıya açılan adaptörüdür.
// Asıl uygulama join_*.go dosyalarındadır.
type JoinQuery struct {
	inner *joinQuery
}

// JoinSource, repository'yi JOIN kaynağı olarak sunar.
func (repo *Repository[T]) JoinSource(alias string) joinSource {
	return repo.newJoinSource(alias)
}

// Join, her çağrıda yeni bir sorgu oluşturur.
func (repo *Repository[T]) Join(alias string) *JoinQuery {
	return &JoinQuery{
		inner: repo.newJoinQuery(alias),
	}
}

func (query *JoinQuery) InnerJoin(
	source joinSource,
	on ...Expr,
) *JoinQuery {
	query.inner.innerJoin(source, on...)
	return query
}

func (query *JoinQuery) LeftJoin(
	source joinSource,
	on ...Expr,
) *JoinQuery {
	query.inner.leftJoin(source, on...)
	return query
}

func (query *JoinQuery) RightJoin(
	source joinSource,
	on ...Expr,
) *JoinQuery {
	query.inner.rightJoin(source, on...)
	return query
}

func (query *JoinQuery) Where(expressions ...Expr) *JoinQuery {
	query.inner.where(expressions...)
	return query
}

func (query *JoinQuery) Select(columns ...SelectColumn) *JoinQuery {
	query.inner.selectColumns(columns...)
	return query
}

func (query *JoinQuery) Err() error {
	if query == nil || query.inner == nil {
		return fmt.Errorf("JOIN sorgusu başlatılmamış")
	}

	return query.inner.getError()
}

func (query *JoinQuery) SQL() (string, []any, error) {
	if err := query.Err(); err != nil {
		return "", nil, err
	}

	return query.inner.buildSQL()
}

func JoinFindMany[R any](
	ctx context.Context,
	query *JoinQuery,
) ([]R, error) {
	if err := query.Err(); err != nil {
		return nil, err
	}

	return joinFindMany[R](ctx, query.inner)
}

func JoinFindOne[R any](
	ctx context.Context,
	query *JoinQuery,
) (R, error) {
	if err := query.Err(); err != nil {
		var zero R
		return zero, err
	}

	return joinFindOne[R](ctx, query.inner)
}

func Eq(field string, value any) Filter {
	return Filter{
		Field: field,
		Op:    OpEq,
		Value: value,
	}
}

func Gt(field string, value any) Filter {
	return Filter{
		Field: field,
		Op:    OpGt,
		Value: value,
	}
}

func EqColumn(left, right string) Filter {
	return Eq(left, ColumnRef{Field: right})
}

// DeleteWhere, filtreye uyan kayıtları kalıcı olarak siler.
// IsDeleted durumuna bakmaz. Filtresiz silmeye izin vermez.
func (repo *Repository[T]) DeleteWhere(
	ctx context.Context,
	filters ...Expr,
) (int64, error) {
	where, args, err := buildWhere(repo.fields, 1, filters...)
	if err != nil {
		return 0, err
	}

	if strings.TrimSpace(where) == "" {
		return 0, fmt.Errorf("DeleteWhere için filtre belirtilmeli")
	}

	query := "DELETE FROM " + repo.table + where

	result, err := repo.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("kalıcı silme başarısız: %w", err)
	}

	return result.RowsAffected()
}
