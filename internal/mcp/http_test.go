package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const pingBody = `{"jsonrpc":"2.0","id":1,"method":"ping"}`

func postMCP(t *testing.T, h http.Handler, contentType, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(pingBody))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHTTPAcceptsLocalJSONClient(t *testing.T) {
	h := newTestServer(t).HTTPHandler()
	cases := []struct{ contentType, origin string }{
		{"application/json", ""},
		{"application/json; charset=utf-8", ""},
		{"application/json", "http://localhost:6274"},
		{"application/json", "http://127.0.0.1:3000"},
		{"application/json", "http://[::1]:8080"},
	}
	for _, c := range cases {
		rec := postMCP(t, h, c.contentType, c.origin)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"result"`) {
			t.Errorf("content-type %q origin %q: got %d %s", c.contentType, c.origin, rec.Code, rec.Body.String())
		}
	}
}

func TestHTTPRejectsNonJSONContentType(t *testing.T) {
	h := newTestServer(t).HTTPHandler()
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		rec := postMCP(t, h, ct, "")
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("content-type %q: got %d, want 415", ct, rec.Code)
		}
	}
}

func TestHTTPRejectsNonLoopbackOrigin(t *testing.T) {
	h := newTestServer(t).HTTPHandler()
	for _, o := range []string{"https://example.com", "http://localhost.example.com", "http://192.168.1.5:8080", "null", "file://"} {
		rec := postMCP(t, h, "application/json", o)
		if rec.Code != http.StatusForbidden {
			t.Errorf("origin %q: got %d, want 403", o, rec.Code)
		}
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8080": true,
		"localhost:8080": true,
		"[::1]:8080":     true,
		":8080":          false,
		"0.0.0.0:8080":   false,
		"192.168.1.5:80": false,
	} {
		if got := IsLoopbackAddr(addr); got != want {
			t.Errorf("IsLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}
