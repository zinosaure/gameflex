package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/credential/internal/auth"
)

// L'IDENTIFIANT GOOGLE CAST : absent d'abord, réglé, refusé s'il n'en est pas un, retiré par une chaîne vide.
func TestTheCastApplicationIsSetInTheAdministration(t *testing.T) {
	server, db := tokenServer(t)
	admin := auth.Principal{ID: userIDOf(t, db, "admin"), Username: "admin", RoleLevel: auth.RoleAdmin}
	call := func(method, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/api/v1/cast", strings.NewReader(body))
		if method == http.MethodGet {
			server.handleGetCast(recorder, request, admin)
		} else {
			server.handlePutCast(recorder, request, admin)
		}

		return recorder
	}

	if answer := call(http.MethodGet, ""); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"application_id":""`) {
		t.Fatalf("empty: %d %s", answer.Code, answer.Body.String())
	}
	if answer := call(http.MethodPut, `{"application_id":" cc1ad845 "}`); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"application_id":"CC1AD845"`) {
		t.Fatalf("set: %d %s", answer.Code, answer.Body.String())
	}
	for _, body := range []string{`{}`, `{"application_id":"pas-un-id"}`, `{"application_id":"CC1AD8450"}`} {
		if answer := call(http.MethodPut, body); answer.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, answer.Code, answer.Body.String())
		}
	}
	if answer := call(http.MethodGet, ""); !strings.Contains(answer.Body.String(), `"application_id":"CC1AD845"`) {
		t.Fatalf("kept after refusals: %s", answer.Body.String())
	}
	if answer := call(http.MethodPut, `{"application_id":""}`); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"application_id":""`) {
		t.Fatalf("removed: %d %s", answer.Code, answer.Body.String())
	}
}
