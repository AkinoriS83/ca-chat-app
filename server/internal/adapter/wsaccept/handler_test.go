package wsaccept

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderHasToken(t *testing.T) {
	tests := []struct {
		values []string
		want   bool
	}{
		{nil, false},
		{[]string{"websocket"}, true},
		{[]string{"WebSocket"}, true},
		{[]string{"h2c, websocket"}, true},
		{[]string{"h2c", " websocket "}, true},
		{[]string{"websockets"}, false},
		{[]string{""}, false},
	}
	for _, tt := range tests {
		header := http.Header{}
		for _, v := range tt.values {
			header.Add("Upgrade", v)
		}
		if got := headerHasToken(header, "Upgrade", "websocket"); got != tt.want {
			t.Errorf("headerHasToken(%q) = %v, want %v", tt.values, got, tt.want)
		}
	}
}

func TestWriteError_HasOnlyCode(t *testing.T) {
	rec := httptest.NewRecorder()

	writeError(rec, http.StatusUnauthorized, "unauthenticated")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("状態が違う: %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"code":"unauthenticated"}` {
		t.Fatalf("本文が違う: %s", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type が違う: %s", got)
	}
}

func TestNewHandler_RejectsInvalidAppOrigin(t *testing.T) {
	for _, origin := range []string{"", "localhost:5173", "://"} {
		if _, err := NewHandler(nil, origin, nil); err == nil {
			t.Errorf("正しくないオリジン(%q)を受け付けた", origin)
		}
	}
}
