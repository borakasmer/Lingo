package Repository

import (
	"database/sql"
	"testing"

	_ "github.com/microsoft/go-mssqldb"
)

type modelTestTagged struct {
	Id      int
	Name    string `db:"full_name"`
	Skipped string `db:"-"`
	private int
}

func (modelTestTagged) TableName() string { return "dbo.Tagged" }

type modelTestEmbedded struct {
	modelTestTagged
}

type modelTestDuplicate struct {
	A string `db:"x"`
	B string `db:"X"`
}

type modelTestNoFields struct {
	hidden int
}

type modelTestAmbiguous struct {
	Id   int
	Name string `db:"Id2"`
	Id2  string `db:"Name"`
}

func TestBuildModelFields(t *testing.T) {
	fields, err := buildModelFields[modelTestTagged]()
	if err != nil {
		t.Fatal(err)
	}

	if len(fields) != 2 {
		t.Fatalf("fields = %#v", fields)
	}

	if fields[0] != (modelField{Name: "Id", Column: "Id", Index: 0}) ||
		fields[1] != (modelField{Name: "Name", Column: "full_name", Index: 1}) {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestBuildModelFieldsRejectsUnsupportedModels(t *testing.T) {
	if _, err := buildModelFields[modelTestEmbedded](); err == nil {
		t.Fatal("embedded struct must be rejected")
	}

	if _, err := buildModelFields[modelTestDuplicate](); err == nil {
		t.Fatal("duplicate columns (case-insensitive) must be rejected")
	}

	if _, err := buildModelFields[modelTestNoFields](); err == nil {
		t.Fatal("model without exported fields must be rejected")
	}

	if _, err := buildModelFields[int](); err == nil {
		t.Fatal("non-struct must be rejected")
	}

	if _, err := buildModelFields[*modelTestTagged](); err == nil {
		t.Fatal("pointer type must be rejected")
	}
}

func TestGetModelFieldsIsCached(t *testing.T) {
	first, err := getModelFields[modelTestTagged]()
	if err != nil {
		t.Fatal(err)
	}

	second, err := getModelFields[modelTestTagged]()
	if err != nil {
		t.Fatal(err)
	}

	if &first[0] != &second[0] {
		t.Fatal("expected the cached slice to be returned")
	}
}

func TestFindModelFieldMatchesNameOrColumnCaseInsensitively(t *testing.T) {
	fields, _ := buildModelFields[modelTestTagged]()

	for _, name := range []string{"Name", "name", "full_name", "FULL_NAME"} {
		field, err := findModelField(fields, name)
		if err != nil || field.Column != "full_name" {
			t.Fatalf("%q -> %#v, %v", name, field, err)
		}
	}

	for _, name := range []string{"", "Skipped", "private", "Name; DROP"} {
		if _, err := findModelField(fields, name); err == nil {
			t.Fatalf("%q must not resolve", name)
		}
	}
}

func TestQuoteIdentifierEscapesClosingBracket(t *testing.T) {
	tests := map[string]string{
		"Name":       "[Name]",
		"a]b":        "[a]]b]",
		"]":          "[]]]",
		"x]; DROP":   "[x]]; DROP]",
		"with space": "[with space]",
	}

	for in, want := range tests {
		if got := quoteIdentifier(in); got != want {
			t.Fatalf("quoteIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuoteSQLName(t *testing.T) {
	got, err := quoteSQLName("dbo.Users")
	if err != nil || got != "[dbo].[Users]" {
		t.Fatalf("got %q, %v", got, err)
	}

	got, err = quoteSQLName("Users")
	if err != nil || got != "[Users]" {
		t.Fatalf("got %q, %v", got, err)
	}

	for _, bad := range []string{"", ".", "a.", ".b", "a.b.c", " . "} {
		if _, err := quoteSQLName(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

func TestNewRepositoryValidation(t *testing.T) {
	db, err := sql.Open("sqlserver", "sqlserver://user:pass@localhost:1433?database=test")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := NewRepository[modelTestTagged](nil); err == nil {
		t.Fatal("nil db must be rejected")
	}

	if _, err := NewRepository[modelTestTagged](db, "a", "b"); err == nil {
		t.Fatal("two table names must be rejected")
	}

	if _, err := NewRepository[modelTestAmbiguous](db, "dbo.T"); err == nil {
		t.Fatal("ambiguous name/column aliases must be rejected")
	}

	type noTable struct{ Id int }
	if _, err := NewRepository[noTable](db); err == nil {
		t.Fatal("missing table name must be rejected")
	}

	repo, err := NewRepository[modelTestTagged](db, "other.Table")
	if err != nil {
		t.Fatal(err)
	}

	if repo.table != "[other].[Table]" {
		t.Fatalf("explicit table name must win, got %q", repo.table)
	}
}
