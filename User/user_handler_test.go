package User

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/microsoft/go-mssqldb"
)

func TestDeleteUsersFromIDReturns500OnRepositoryError(t *testing.T) {
	// A closed DB makes the delete fail without a running server.
	db, err := sql.Open("sqlserver", "sqlserver://localhost")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	repository, err := NewUserRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewUserHandler(repository)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/bulk-delete?minID=1", nil)
	rec := httptest.NewRecorder()
	handler.DeleteUsersFromID(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
