package Repository

import (
	"reflect"
	"strings"
	"testing"

	"database/sql"
)

type whereTestModel struct {
	Id   int
	Name string
	Age  int `db:"age_years"`
}

func whereTestFields(t *testing.T) []modelField {
	t.Helper()

	fields, err := buildModelFields[whereTestModel]()
	if err != nil {
		t.Fatal(err)
	}

	return fields
}

func TestBuildWhereShapes(t *testing.T) {
	fields := whereTestFields(t)

	tests := []struct {
		name  string
		start int
		exprs []Expr
		want  string
		args  int
	}{
		{"no expressions", 1, nil, "", 0},
		{"single filter", 1, []Expr{Eq("Name", "a")}, " WHERE ([Name] = @p1)", 1},
		{"column tag and alias", 1, []Expr{Gt("age_years", 3)}, " WHERE ([age_years] > @p1)", 1},
		{"separate filters are ANDed", 1, []Expr{Eq("Id", 1), Eq("Name", "x")}, " WHERE ([Id] = @p1 AND [Name] = @p2)", 2},
		{"OR group inside AND", 1, []Expr{Eq("Id", 1), Or(Eq("Name", "a"), Eq("Name", "b"))}, " WHERE ([Id] = @p1 AND ([Name] = @p2 OR [Name] = @p3))", 3},
		{"start index offsets names", 4, []Expr{Eq("Id", 1)}, " WHERE ([Id] = @p4)", 1},
		{"NULL equality", 1, []Expr{Eq("Name", nil)}, " WHERE ([Name] IS NULL)", 0},
		{"NULL inequality", 1, []Expr{Filter{Field: "Name", Op: OpNe, Value: nil}}, " WHERE ([Name] IS NOT NULL)", 0},
		{"LIKE", 1, []Expr{Filter{Field: "Name", Op: OpLike, Value: "%a%"}}, " WHERE ([Name] LIKE @p1)", 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, args, err := buildWhere(fields, test.start, test.exprs...)
			if err != nil {
				t.Fatal(err)
			}

			if got != test.want {
				t.Fatalf("where = %q, want %q", got, test.want)
			}

			if len(args) != test.args {
				t.Fatalf("args = %d, want %d", len(args), test.args)
			}
		})
	}
}

func TestBuildWhereArgumentsAreNamedParameters(t *testing.T) {
	_, args, err := buildWhere(whereTestFields(t), 1, Eq("Id", 7), Eq("Name", "bob"))
	if err != nil {
		t.Fatal(err)
	}

	want := []any{sql.Named("p1", 7), sql.Named("p2", "bob")}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildWhereValuesNeverEnterSQLText(t *testing.T) {
	hostile := "'; DROP TABLE Users; --"

	got, _, err := buildWhere(whereTestFields(t), 1, Eq("Name", hostile))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "DROP") {
		t.Fatalf("value leaked into SQL: %q", got)
	}
}

func TestBuildWhereRejectsInvalidInput(t *testing.T) {
	fields := whereTestFields(t)

	tests := []struct {
		name  string
		start int
		expr  Expr
	}{
		{"unknown field", 1, Eq("Nope", 1)},
		{"injection-looking field", 1, Eq("Name]; DROP TABLE x;--", 1)},
		{"invalid operator", 1, Filter{Field: "Id", Op: Operator("; --"), Value: 1}},
		{"NULL with greater-than", 1, Filter{Field: "Id", Op: OpGt, Value: nil}},
		{"invalid logic", 1, Group{Logic: Logic("XOR"), Items: []Expr{Eq("Id", 1)}}},
		{"nil expression", 1, nil},
		{"start index below one", 0, Eq("Id", 1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := buildWhere(fields, test.start, test.expr); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestOperatorSQLAllowList(t *testing.T) {
	for _, op := range []Operator{OpEq, OpNe, OpGt, OpGte, OpLt, OpLte, OpLike} {
		if _, err := op.SQL(); err != nil {
			t.Fatalf("%q: %v", op, err)
		}
	}

	if _, err := Operator("OR 1=1").SQL(); err == nil {
		t.Fatal("unknown operator must be rejected")
	}
}
