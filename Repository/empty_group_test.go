package Repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
)

type groupPerson struct {
	Id        int
	TenantId  int
	Name      string
	IsDeleted bool
}

type groupRole struct {
	Id        int
	Name      string
	IsDeleted bool
}

// sql.Open does not connect; these tests only check the generated SQL.
func newGroupRepo[T any](t *testing.T, db *sql.DB, table string) *Repository[T] {
	t.Helper()
	repo, err := NewRepository[T](db, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	return repo
}

func newGroupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlserver", "sqlserver://unused@127.0.0.1:1?database=none")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func namedArgs(t *testing.T, args []any) []string {
	t.Helper()
	out := make([]string, 0, len(args))
	for _, arg := range args {
		n, ok := arg.(sql.NamedArg)
		if !ok {
			t.Fatalf("arg %#v is not sql.NamedArg", arg)
		}
		out = append(out, n.Name+"="+fmt.Sprint(n.Value))
	}
	return out
}

func TestBuildWhere_EmptyGroups(t *testing.T) {
	fields := newGroupRepo[groupPerson](t, newGroupDB(t), "dbo.People").fields

	tests := []struct {
		name  string
		exprs []Expr
		where string
		args  []string
	}{
		{"empty or", []Expr{Or()}, " WHERE (1=0)", []string{}},
		{"empty and", []Expr{And()}, "", []string{}},
		{"filter and empty or", []Expr{Eq("TenantId", 7), Or()}, " WHERE ([TenantId] = @p1 AND 1=0)", []string{"p1=7"}},
		{"filter and empty and", []Expr{Eq("TenantId", 7), And()}, " WHERE ([TenantId] = @p1)", []string{"p1=7"}},
		{"or of empty ors", []Expr{Or(Or(), Or())}, " WHERE ((1=0 OR 1=0))", []string{}},
		{"or with empty and is true", []Expr{Eq("TenantId", 7), Or(Eq("Name", "a"), And())}, " WHERE ([TenantId] = @p1)", []string{"p1=7"}},
		{"params after dropped or", []Expr{Or(Eq("Name", "a"), And()), Eq("TenantId", 7)}, " WHERE ([TenantId] = @p1)", []string{"p1=7"}},
		{"or with nested empty or", []Expr{Or(Eq("Name", "a"), Or())}, " WHERE (([Name] = @p1 OR 1=0))", []string{"p1=a"}},
		{"and with nested empty or", []Expr{And(Eq("Name", "a"), And(Or()))}, " WHERE (([Name] = @p1 AND (1=0)))", []string{"p1=a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args, err := buildWhere(fields, 1, tt.exprs...)
			if err != nil {
				t.Fatal(err)
			}
			if where != tt.where {
				t.Errorf("where\n got: %q\nwant: %q", where, tt.where)
			}
			if got := namedArgs(t, args); !reflect.DeepEqual(got, tt.args) {
				t.Errorf("args = %v, want %v", got, tt.args)
			}
		})
	}
}

func TestBuildWhere_OrWithEmptyAndStillValidates(t *testing.T) {
	fields := newGroupRepo[groupPerson](t, newGroupDB(t), "dbo.People").fields

	if _, _, err := buildWhere(fields, 1, Or(And(), Eq("Missing", 1))); err == nil {
		t.Error("unknown field error was swallowed")
	}
}

func TestDeleteWhere_EmptyAndRejected(t *testing.T) {
	repo := newGroupRepo[groupPerson](t, newGroupDB(t), "dbo.People")

	for _, filters := range [][]Expr{
		nil,
		{And()},
		{And(And())},
		{Or(Eq("TenantId", 7), And())},
	} {
		if _, err := repo.DeleteWhere(context.Background(), filters...); err == nil {
			t.Errorf("DeleteWhere(%v) allowed a delete without a filter", filters)
		}
	}
}

func TestUpdateSQL_EmptyAndRejected(t *testing.T) {
	repo := newGroupRepo[groupPerson](t, newGroupDB(t), "dbo.People")

	if _, _, err := repo.updateSQL(map[string]any{"Name": "x"}, []Expr{And()}); err == nil {
		t.Error("updateSQL(And()) allowed an update without a filter")
	}

	query, _, err := repo.updateSQL(map[string]any{"Name": "x"}, []Expr{Or()})
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE [dbo].[People] SET [Name] = @p1 WHERE (1=0) AND ISNULL([IsDeleted], 0) = 0"
	if query != want {
		t.Errorf("SQL\n got: %s\nwant: %s", query, want)
	}
}

func TestJoinSQL_EmptyGroups(t *testing.T) {
	db := newGroupDB(t)
	people := newGroupRepo[groupPerson](t, db, "dbo.People")
	roles := newGroupRepo[groupRole](t, db, "dbo.Roles")

	base := "SELECT [p].[Id] AS [Id]" +
		" FROM (SELECT * FROM [dbo].[People] WHERE ISNULL([IsDeleted], 0) = 0) AS [p]" +
		" INNER JOIN (SELECT * FROM [dbo].[Roles] WHERE ISNULL([IsDeleted], 0) = 0) AS [r] ON ([r].[Id] = [p].[Id])"

	tests := []struct {
		name  string
		exprs []Expr
		where string
		args  []string
	}{
		{"empty or", []Expr{Eq("p.TenantId", 7), Or()}, " WHERE ([p].[TenantId] = @p1 AND 1=0)", []string{"p1=7"}},
		{"empty and", []Expr{Eq("p.TenantId", 7), And()}, " WHERE ([p].[TenantId] = @p1)", []string{"p1=7"}},
		{"or with empty and", []Expr{Or(Eq("r.Name", "a"), And()), Eq("p.TenantId", 7)}, " WHERE ([p].[TenantId] = @p1)", []string{"p1=7"}},
		{"only empty and", []Expr{And()}, "", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := people.Join("p").
				InnerJoin(roles.JoinSource("r"), EqColumn("r.Id", "p.Id")).
				Where(tt.exprs...).
				Select(SelectColumn{Field: "p.Id", As: "Id"})

			sqlText, args, err := query.SQL()
			if err != nil {
				t.Fatal(err)
			}
			if want := base + tt.where; sqlText != want {
				t.Errorf("SQL\n got: %s\nwant: %s", sqlText, want)
			}
			if got := namedArgs(t, args); !reflect.DeepEqual(got, tt.args) {
				t.Errorf("args = %v, want %v", got, tt.args)
			}
		})
	}
}
