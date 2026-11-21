package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

const testAppOrigin = "http://localhost:5173"

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func requireErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("状態が違う: got %d, want %d", rec.Code, status)
	}
	if got, want := rec.Body.String(), `{"code":"`+code+`"}`; got != want {
		t.Fatalf("本文が違う: got %s, want %s", got, want)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type が違う: %s", got)
	}
	for name := range rec.Header() {
		if len(name) >= 15 && name[:15] == "Access-Control-" {
			t.Fatalf("CORS のヘッダーがある: %s", name)
		}
	}
}

func TestRequireAppOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origins []string
		allowed bool
	}{
		{"Origin がない", nil, true},
		{"アプリのオリジン", []string{testAppOrigin}, true},
		{"他のオリジン", []string{"http://evil.example"}, false},
		{"ポートが違う", []string{"http://localhost:5174"}, false},
		{"スキームが違う", []string{"https://localhost:5173"}, false},
		{"ホストの表記が違う", []string{"http://127.0.0.1:5173"}, false},
		{"末尾にスラッシュがある", []string{testAppOrigin + "/"}, false},
		{"大文字と小文字が違う", []string{"HTTP://LOCALHOST:5173"}, false},
		{"null", []string{"null"}, false},
		{"空の Origin", []string{""}, false},
		{"Origin が複数ある", []string{testAppOrigin, "http://evil.example"}, false},
		{"アプリのオリジンが2つある", []string{testAppOrigin, testAppOrigin}, false},
	}
	handler := RequireAppOrigin(testAppOrigin)(okHandler())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
				r := httptest.NewRequest(method, "/api/anything", nil)
				for _, origin := range tt.origins {
					r.Header.Add("Origin", origin)
				}
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, r)

				if tt.allowed {
					if rec.Code != http.StatusNoContent {
						t.Fatalf("%s: 通さなかった: %d", method, rec.Code)
					}
					continue
				}
				requireErrorResponse(t, rec, http.StatusForbidden, "forbidden_origin")
			}
		})
	}
}

func TestRequireCanonicalPath(t *testing.T) {
	tests := []struct {
		path    string
		allowed bool
	}{
		{"/", true},
		{"/api/direct-conversations/bob/messages", true},
		{"/api/realtime", true},
		{"//api/realtime", false},
		{"/api//realtime", false},
		{"/api/./realtime", false},
		{"/api/../api/realtime", false},
		{"/api/realtime/", false},
		{"/api/realtime/..", false},
		{"/..", false},
	}
	handler := RequireCanonicalPath(okHandler())
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			r.URL.Path = tt.path
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, r)

			if tt.allowed {
				if rec.Code != http.StatusNoContent {
					t.Fatalf("通さなかった: %d", rec.Code)
				}
				return
			}
			requireErrorResponse(t, rec, http.StatusNotFound, "not_found")
		})
	}
}

func TestRequireCanonicalPath_RejectsPathWithoutLeadingSlash(t *testing.T) {
	handler := RequireCanonicalPath(okHandler())
	for _, p := range []string{"*", "", "api/realtime"} {
		r := httptest.NewRequest(http.MethodOptions, "http://localhost/", nil)
		r.URL.Path = p
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, r)

		requireErrorResponse(t, rec, http.StatusNotFound, "not_found")
	}
}

func TestNotFoundHandler(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions, http.MethodDelete} {
		rec := httptest.NewRecorder()

		NotFoundHandler().ServeHTTP(rec, httptest.NewRequest(method, "/anything", nil))

		requireErrorResponse(t, rec, http.StatusNotFound, "not_found")
	}
}

func TestStatusOf(t *testing.T) {
	tests := map[string]int{
		"forbidden_origin": http.StatusForbidden,
		"invalid_input":    http.StatusBadRequest,
		"invalid_body":     http.StatusBadRequest,
		"invalid_peer":     http.StatusBadRequest,
		"unauthenticated":  http.StatusUnauthorized,
		"not_found":        http.StatusNotFound,
		"internal":         http.StatusInternalServerError,
	}
	for code, want := range tests {
		rec := httptest.NewRecorder()
		writeError(rec, openapi.ErrorCode(code))
		requireErrorResponse(t, rec, want, code)
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control が違う: %s", got)
		}
	}
}
