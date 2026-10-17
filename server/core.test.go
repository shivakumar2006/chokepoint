package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	cases := []struct {
		mode, origin, method string
		wantOrigin           string
		wantStatus           int
	}{
		{"dev", "http://localhost:5173", "GET", "http://localhost:5173", 200},
		{"dev", "http://127.0.0.1:5174", "GET", "http://127.0.0.1:5174", 200},
		{"dev", "https://evil.example", "GET", "", 200},
		{"dev", "http://localhost:5173", "OPTIONS", "http://localhost:5173", 204},
		{"*", "https://anything.dev", "GET", "https://anything.dev", 200},
		{"https://app.example", "https://app.example", "GET", "https://app.example", 200},
		{"https://app.example", "http://localhost:5173", "GET", "", 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "/api/graph", nil)
		req.Header.Set("Origin", c.origin)
		rec := httptest.NewRecorder()
		cors(c.mode, ok).ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != c.wantOrigin {
			t.Errorf("mode=%s origin=%s: allow-origin=%q want %q", c.mode, c.origin, got, c.wantOrigin)
		}
		if rec.Code != c.wantStatus {
			t.Errorf("mode=%s method=%s: status=%d want %d", c.mode, c.method, rec.Code, c.wantStatus)
		}
	}
}
