package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/h44z/wg-portal/internal/app/api/v0/model"
)

func TestRespondAuthError(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	AuthEndpoint{}.respondAuthError(recorder, http.StatusUnauthorized, authErrorCodeLoginFailed, "login failed")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	var payload model.Error
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload.ErrorId != string(authErrorCodeLoginFailed) {
		t.Fatalf("error id = %q, want %q", payload.ErrorId, authErrorCodeLoginFailed)
	}
	if payload.Message != "login failed" {
		t.Fatalf("message = %q, want %q", payload.Message, "login failed")
	}
}
