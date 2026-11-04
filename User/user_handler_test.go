package User

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Validation failures return before the repository is touched, so a nil
// repository is enough to exercise them.
func TestHandlersRejectWrongMethod(t *testing.T) {
	handler := NewUserHandler(nil)

	tests := []struct {
		name    string
		call    func(http.ResponseWriter, *http.Request)
		method  string
		allowed string
	}{
		{"GetByName", handler.GetByName, http.MethodPost, http.MethodGet},
		{"SearchUsers", handler.SearchUsers, http.MethodPost, http.MethodGet},
		{"GetAllWithPaging", handler.GetAllWithPaging, http.MethodPost, http.MethodGet},
		{"AppendXToNamesOver40", handler.AppendXToNamesOver40, http.MethodGet, http.MethodPost},
		{"GetUserPermissions", handler.GetUserPermissions, http.MethodPost, http.MethodGet},
		{"InsertSampleUsers", handler.InsertSampleUsers, http.MethodGet, http.MethodPost},
		{"DeleteUsersFromID", handler.DeleteUsersFromID, http.MethodGet, http.MethodDelete},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			test.call(rec, httptest.NewRequest(test.method, "/x", nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("code = %d", rec.Code)
			}

			if rec.Header().Get("Allow") != test.allowed {
				t.Fatalf("Allow = %q", rec.Header().Get("Allow"))
			}
		})
	}
}

func TestHandlersRejectInvalidParameters(t *testing.T) {
	handler := NewUserHandler(nil)

	tests := []struct {
		name   string
		call   func(http.ResponseWriter, *http.Request)
		method string
		url    string
	}{
		{"GetByName missing name", handler.GetByName, http.MethodGet, "/x"},
		{"GetByName blank name", handler.GetByName, http.MethodGet, "/x?name=%20%20"},
		{"Search bad gender", handler.SearchUsers, http.MethodGet, "/x?gender=Other&age=1&name=a"},
		{"Search bad age", handler.SearchUsers, http.MethodGet, "/x?gender=Male&age=abc&name=a"},
		{"Search negative age", handler.SearchUsers, http.MethodGet, "/x?gender=Male&age=-1&name=a"},
		{"Search missing name", handler.SearchUsers, http.MethodGet, "/x?gender=Male&age=1"},
		{"Paging bad page", handler.GetAllWithPaging, http.MethodGet, "/x?page=a"},
		{"Paging bad pageSize", handler.GetAllWithPaging, http.MethodGet, "/x?pageSize=a"},
		{"Paging bad sortBy", handler.GetAllWithPaging, http.MethodGet, "/x?sortBy=Password"},
		{"Paging bad ascending", handler.GetAllWithPaging, http.MethodGet, "/x?ascending=maybe"},
		{"Append missing batchSize", handler.AppendXToNamesOver40, http.MethodPost, "/x"},
		{"Append zero batchSize", handler.AppendXToNamesOver40, http.MethodPost, "/x?batchSize=0"},
		{"Permissions bad user", handler.GetUserPermissions, http.MethodGet, "/x?userID=0&controllerID=1"},
		{"Permissions bad controller", handler.GetUserPermissions, http.MethodGet, "/x?userID=1&controllerID=x"},
		{"Delete missing minID", handler.DeleteUsersFromID, http.MethodDelete, "/x"},
		{"Delete zero minID", handler.DeleteUsersFromID, http.MethodDelete, "/x?minID=0"},
		{"Delete negative minID", handler.DeleteUsersFromID, http.MethodDelete, "/x?minID=-3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			test.call(rec, httptest.NewRequest(test.method, test.url, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}
