package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthGet(t *testing.T) {
	rec := httptest.NewRecorder()
	HealthHandler(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	if body["status"] != "ok" || len(body) != 1 {
		t.Errorf("body = %v", body)
	}
}

func TestHealthOtherMethods(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		HealthHandler(rec, httptest.NewRequest(m, "/healthz", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want 405", m, rec.Code)
		}
		if allow := rec.Header().Get("Allow"); allow != "GET" {
			t.Errorf("%s Allow = %q, want GET", m, allow)
		}
	}
}

func TestHealthIsAHandlerFunc(t *testing.T) {
	var _ http.HandlerFunc = HealthHandler
}
