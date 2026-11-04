package Repository

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/microsoft/go-mssqldb"
)

type joinTestUser struct {
	Id        int
	Name      string
	IsDeleted bool
}

func (joinTestUser) TableName() string { return "dbo.Users" }

type joinTestOrder struct {
	Id     int
	IdUser int
	Total  int
}

func (joinTestOrder) TableName() string { return "dbo.Orders" }

func joinTestRepos(t *testing.T) (*Repository[joinTestUser], *Repository[joinTestOrder]) {
	t.Helper()

	db, err := sql.Open("sqlserver", "sqlserver://user:pass@localhost:1433?database=test")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	users, err := NewRepository[joinTestUser](db)
	if err != nil {
		t.Fatal(err)
	}

	orders, err := NewRepository[joinTestOrder](db)
	if err != nil {
		t.Fatal(err)
	}

	return users, orders
}

func TestJoinSQLShape(t *testing.T) {
	users, orders := joinTestRepos(t)

	query, args, err := users.Join("u").
		LeftJoin(orders.JoinSource("o"), EqColumn("o.IdUser", "u.Id"), Gt("o.Total", 10)).
		Where(Eq("u.Name", "ann")).
		Select(
			SelectColumn{Field: "u.Name", As: "Name"},
			SelectColumn{Field: "o.Total", As: "Total"},
		).SQL()
	if err != nil {
		t.Fatal(err)
	}

	want := "SELECT [u].[Name] AS [Name], [o].[Total] AS [Total] " +
		"FROM (SELECT * FROM [dbo].[Users] WHERE ISNULL([IsDeleted], 0) = 0) AS [u] " +
		"LEFT JOIN [dbo].[Orders] AS [o] ON ([o].[IdUser] = [u].[Id] AND [o].[Total] > @p1) " +
		"WHERE ([u].[Name] = @p2)"
	if query != want {
		t.Fatalf("query =\n%s\nwant\n%s", query, want)
	}

	if len(args) != 2 || args[0] != sql.Named("p1", 10) || args[1] != sql.Named("p2", "ann") {
		t.Fatalf("args = %#v", args)
	}
}

func TestJoinSQLRejectsInvalidQueries(t *testing.T) {
	users, orders := joinTestRepos(t)

	sel := SelectColumn{Field: "u.Id", As: "Id"}

	tests := []struct {
		name  string
		query *JoinQuery
	}{
		{"empty alias", users.Join("").Select(sel)},
		{"alias with injection", users.Join("u]; DROP TABLE x;--").Select(sel)},
		{"alias starting with digit", users.Join("1u").Select(sel)},
		{"no select columns", users.Join("u")},
		{"duplicate alias", users.Join("u").InnerJoin(orders.JoinSource("U"), EqColumn("U.IdUser", "u.Id")).Select(sel)},
		{"join without ON", users.Join("u").InnerJoin(orders.JoinSource("o")).Select(sel)},
		{"join source not from repository", users.Join("u").InnerJoin(joinSource{alias: "o"}, Eq("o.Id", 1)).Select(sel)},
		{"unknown alias in select", users.Join("u").Select(SelectColumn{Field: "x.Id", As: "Id"})},
		{"unknown field in select", users.Join("u").Select(SelectColumn{Field: "u.Nope", As: "Id"})},
		{"field without alias", users.Join("u").Select(SelectColumn{Field: "Id", As: "Id"})},
		{"duplicate result name", users.Join("u").Select(sel, SelectColumn{Field: "u.Name", As: "id"})},
		{"invalid result name", users.Join("u").Select(SelectColumn{Field: "u.Id", As: "bad name"})},
		{"ON references a later join", users.Join("u").InnerJoin(orders.JoinSource("o"), EqColumn("o.IdUser", "p.Id")).Select(sel)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := test.query.SQL(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestJoinQueryErrKeepsFirstError(t *testing.T) {
	users, _ := joinTestRepos(t)

	query := users.Join("").Where(Eq("u.Id", 1))
	if err := query.Err(); err == nil || !strings.Contains(err.Error(), "boş") {
		t.Fatalf("err = %v", err)
	}

	var nilQuery *JoinQuery
	if _, _, err := nilQuery.SQL(); err == nil {
		t.Fatal("nil JoinQuery must return an error, not panic")
	}
}

func TestJoinSourceWithoutIsDeletedAddsNoSubquery(t *testing.T) {
	users, orders := joinTestRepos(t)

	query, _, err := users.Join("u").
		InnerJoin(orders.JoinSource("o"), EqColumn("o.IdUser", "u.Id")).
		Select(SelectColumn{Field: "o.Total", As: "Total"}).SQL()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(query, "INNER JOIN [dbo].[Orders] AS [o] ON") {
		t.Fatalf("query = %q", query)
	}
}
