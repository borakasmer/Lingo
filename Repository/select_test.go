package Repository

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/microsoft/go-mssqldb"
)

type selectTestModel struct {
	Id        int
	Name      string
	Age       int
	IsDeleted bool
}

func (selectTestModel) TableName() string { return "dbo.People" }

// sql.Open does not connect, so no server is needed to build SQL.
func selectTestRepo(t *testing.T) *Repository[selectTestModel] {
	t.Helper()

	db, err := sql.Open("sqlserver", "sqlserver://user:pass@localhost:1433?database=test")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	repo, err := NewRepository[selectTestModel](db)
	if err != nil {
		t.Fatal(err)
	}

	return repo
}

func TestSelectSQLDefaults(t *testing.T) {
	query, args, err := selectTestRepo(t).selectSQL(false, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := "SELECT [Id], [Name], [Age], [IsDeleted] FROM [dbo].[People] WHERE ISNULL([IsDeleted], 0) = 0 ORDER BY [Id] ASC"
	if query != want || len(args) != 0 {
		t.Fatalf("query = %q args=%d", query, len(args))
	}
}

func TestSelectSQLFilterOrderAndDirection(t *testing.T) {
	query, args, err := selectTestRepo(t).selectSQL(false, []any{Eq("Name", "a"), "age", false})
	if err != nil {
		t.Fatal(err)
	}

	for _, part := range []string{"WHERE ([Name] = @p1) AND ISNULL([IsDeleted], 0) = 0", "ORDER BY [Age] DESC"} {
		if !strings.Contains(query, part) {
			t.Fatalf("%q missing from %q", part, query)
		}
	}

	if len(args) != 1 {
		t.Fatalf("args = %d", len(args))
	}
}

func TestSelectSQLPaging(t *testing.T) {
	repo := selectTestRepo(t)

	query, args, err := repo.selectSQL(false, []any{3, 20})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(query, "ORDER BY [Id] ASC OFFSET @offset ROWS FETCH NEXT @pageSize ROWS ONLY") {
		t.Fatalf("query = %q", query)
	}

	if len(args) != 2 || args[0] != sql.Named("offset", 40) || args[1] != sql.Named("pageSize", 20) {
		t.Fatalf("args = %#v", args)
	}

	// A single integer means page size with page 1.
	_, args, err = repo.selectSQL(false, []any{5})
	if err != nil {
		t.Fatal(err)
	}

	if args[0] != sql.Named("offset", 0) || args[1] != sql.Named("pageSize", 5) {
		t.Fatalf("single int args = %#v", args)
	}
}

func TestSelectSQLFindOneUsesTop(t *testing.T) {
	query, _, err := selectTestRepo(t).selectSQL(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(query, "SELECT TOP (1) ") {
		t.Fatalf("query = %q", query)
	}
}

func TestSelectSQLRejectsInvalidQueries(t *testing.T) {
	repo := selectTestRepo(t)

	tests := []struct {
		name   string
		one    bool
		params []any
	}{
		{"unknown order field", false, []any{"Nope"}},
		{"three integers", false, []any{1, 2, 3}},
		{"zero page size", false, []any{0}},
		{"zero page", false, []any{0, 10}},
		{"negative page size", false, []any{1, -1}},
		{"overflowing page", false, []any{int(^uint(0) >> 1), 100}},
		{"FindOne with paging", true, []any{1, 10}},
		{"unsupported parameter type", false, []any{3.14}},
		{"unknown filter field", false, []any{Eq("Nope", 1)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := repo.selectSQL(test.one, test.params); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSelectSQLParameterLimit(t *testing.T) {
	filters := make([]any, 0, sqlServerParameterLimit+1)
	for i := 0; i <= sqlServerParameterLimit; i++ {
		filters = append(filters, Eq("Id", i))
	}

	if _, _, err := selectTestRepo(t).selectSQL(false, filters); err == nil {
		t.Fatal("expected the SQL Server parameter limit error")
	}
}
