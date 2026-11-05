package Repository

import "testing"

type limitModel struct {
	ID    int
	Name  string
	Age   int
	Email string
}

func newLimitRepo() *Repository[limitModel] {
	return &Repository[limitModel]{
		table: "[LimitModel]",
		fields: []modelField{
			{Column: "ID", Index: 0},
			{Column: "Name", Index: 1},
			{Column: "Age", Index: 2},
			{Column: "Email", Index: 3},
		},
	}
}

func TestMaxUserParameters(t *testing.T) {
	if maxUserParameters != 2098 {
		t.Fatalf("maxUserParameters = %d, want 2098", maxUserParameters)
	}
}

func TestInsertSQLParameterBoundary(t *testing.T) {
	repo := newLimitRepo()
	fieldCount := len(repo.insertFields())
	if fieldCount != 4 {
		t.Fatalf("insertFields = %d, want 4", fieldCount)
	}
	maxRows := maxUserParameters / fieldCount // 524 rows = 2096 params

	ok := make([]limitModel, maxRows)
	_, args, err := repo.insertSQL(ok)
	if err != nil {
		t.Fatalf("%d rows should be accepted: %v", maxRows, err)
	}
	if len(args) > maxUserParameters {
		t.Fatalf("got %d args, limit %d", len(args), maxUserParameters)
	}

	if _, _, err := repo.insertSQL(make([]limitModel, maxRows+1)); err == nil {
		t.Fatalf("%d rows (%d params) should be rejected", maxRows+1, (maxRows+1)*fieldCount)
	}
}
