package Repository

import (
	"database/sql"
	"strings"
	"testing"

	"sqlOrm/Models"

	_ "github.com/microsoft/go-mssqldb"
)

type writeTestModel struct {
	Id        int
	Name      string
	Age       int `db:"age_years"`
	IsDeleted bool
}

func (writeTestModel) TableName() string { return "dbo.People" }

type writeTestIdOnly struct{ Id int }

func (writeTestIdOnly) TableName() string { return "dbo.IdOnly" }

func writeTestRepo[T any](t *testing.T) *Repository[T] {
	t.Helper()

	db, err := sql.Open("sqlserver", "sqlserver://user:pass@localhost:1433?database=test")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	repo, err := NewRepository[T](db)
	if err != nil {
		t.Fatal(err)
	}

	return repo
}

func TestUpdateSQLOrdersColumnsAndSkipsId(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	query, args, err := repo.updateSQL(
		map[string]any{"Name": "a", "age_years": 3, "Id": 99},
		[]Expr{Eq("Id", 1)},
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "UPDATE [dbo].[People] SET [Name] = @p1, [age_years] = @p2 WHERE ([Id] = @p3) AND ISNULL([IsDeleted], 0) = 0"
	if query != want {
		t.Fatalf("query = %q", query)
	}

	if len(args) != 3 {
		t.Fatalf("args = %d", len(args))
	}
}

func TestUpdateSQLAllowsZeroValues(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	_, args, err := repo.updateSQL(map[string]any{"Age": 0, "Name": ""}, []Expr{Eq("Id", 1)})
	if err != nil {
		t.Fatal(err)
	}

	// Columns are sorted bytewise: "Name" < "age_years".
	if args[0] != sql.Named("p1", "") || args[1] != sql.Named("p2", 0) {
		t.Fatalf("args = %#v", args)
	}
}

func TestUpdateSQLRawValueIsInlinedWithoutParameter(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	query, args, err := repo.updateSQL(
		map[string]any{"Age": Models.RawValue{Expr: "[age_years] + 1"}},
		[]Expr{Eq("Id", 1)},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(query, "[age_years] = [age_years] + 1 WHERE ([Id] = @p1)") || len(args) != 1 {
		t.Fatalf("query = %q args=%d", query, len(args))
	}
}

func TestUpdateSQLRejectsInvalidInput(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	tests := []struct {
		name    string
		updates map[string]any
		filters []Expr
	}{
		{"missing filter", map[string]any{"Name": "a"}, nil},
		{"empty group is not a filter", map[string]any{"Name": "a"}, []Expr{And()}},
		{"unknown field", map[string]any{"Nope": 1}, []Expr{Eq("Id", 1)}},
		{"same column by name and tag", map[string]any{"Age": 1, "age_years": 2}, []Expr{Eq("Id", 1)}},
		{"empty raw value", map[string]any{"Name": Models.RawValue{Expr: "  "}}, []Expr{Eq("Id", 1)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := repo.updateSQL(test.updates, test.filters); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestUpdateSQLOnlyIdProducesNoStatement(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	query, _, err := repo.updateSQL(map[string]any{"Id": 5}, []Expr{Eq("Id", 1)})
	if err != nil || query != "" {
		t.Fatalf("query = %q err=%v", query, err)
	}
}

func TestInsertSQLSkipsIdAndNumbersParameters(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	query, args, err := repo.insertSQL([]writeTestModel{
		{Id: 1, Name: "a", Age: 1},
		{Id: 2, Name: "b", Age: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "INSERT INTO [dbo].[People] ([Name], [age_years], [IsDeleted]) VALUES (@p1, @p2, @p3), (@p4, @p5, @p6)"
	if query != want || len(args) != 6 {
		t.Fatalf("query = %q args=%d", query, len(args))
	}

	if args[0] != sql.Named("p1", "a") || args[3] != sql.Named("p4", "b") {
		t.Fatalf("args = %#v", args)
	}
}

func TestInsertSQLLimits(t *testing.T) {
	repo := writeTestRepo[writeTestModel](t)

	if _, _, err := repo.insertSQL(nil); err == nil {
		t.Fatal("empty insert must fail")
	}

	// 3 insertable columns: 2100/3 = 700 rows fit, 701 do not.
	if _, _, err := repo.insertSQL(make([]writeTestModel, 700)); err != nil {
		t.Fatalf("700 rows should fit: %v", err)
	}

	if _, _, err := repo.insertSQL(make([]writeTestModel, 701)); err == nil {
		t.Fatal("701 rows must exceed the parameter limit")
	}
}

func TestInsertSQLDefaultValuesOnlyForSingleRow(t *testing.T) {
	repo := writeTestRepo[writeTestIdOnly](t)

	query, args, err := repo.insertSQL([]writeTestIdOnly{{}})
	if err != nil || query != "INSERT INTO [dbo].[IdOnly] DEFAULT VALUES" || args != nil {
		t.Fatalf("query = %q args=%v err=%v", query, args, err)
	}

	if _, _, err := repo.insertSQL([]writeTestIdOnly{{}, {}}); err == nil {
		t.Fatal("DEFAULT VALUES with several rows must fail")
	}
}
