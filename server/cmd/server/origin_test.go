package main

import (
	"io"
	"net/http"
	"testing"
)

// WS-29: 第三者のサイトに置かれたページから、利用者のブラウザを介して、
// 送信、履歴の取得、リアルタイムの接続を行えない。
func TestOtherOrigin_IsRejected(t *testing.T) {
	s := startServer(t)
	const evil = "https://evil.example"

	headers := func(origin string) map[string]string {
		return map[string]string{"Content-Type": "application/json", "X-Fixed-User": "alice", "Origin": origin}
	}
	body := []byte(`{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"hello"}`)

	for _, origin := range []string{evil, "null", "http://app.test:8080", "https://app.test", "http://app.test.evil.example"} {
		send := s.do(http.MethodPost, messagesPath("bob"), headers(origin), body)
		requireError(t, send, http.StatusForbidden, "forbidden_origin")
		requireNoCORS(t, send)

		history := s.do(http.MethodGet, messagesPath("bob"), headers(origin), nil)
		requireError(t, history, http.StatusForbidden, "forbidden_origin")
		requireNoCORS(t, history)

		conn, resp, err := s.dial("alice", origin)
		if err == nil {
			_ = conn.CloseNow()
			t.Fatalf("他のオリジン(%s)からの接続が、確立された", origin)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("他のオリジン(%s)からの接続の応答が違う: %v", origin, resp)
		}
		data, _ := io.ReadAll(resp.Body)
		if string(data) != `{"code":"forbidden_origin"}` {
			t.Fatalf("応答の本文が違う: %s", data)
		}
	}
	s.requireStored(0, 0)

	// 事前の確認(preflight)に、許可を返さない。OPTIONS は、定義されていない方法である。
	preflight := s.do(http.MethodOptions, messagesPath("bob"), map[string]string{
		"Origin":                         evil,
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "content-type,x-fixed-user",
	}, nil)
	requireError(t, preflight, http.StatusForbidden, "forbidden_origin")
	requireNoCORS(t, preflight)
	sameOriginOptions := s.do(http.MethodOptions, messagesPath("bob"), map[string]string{"Origin": testAppOrigin}, nil)
	requireError(t, sameOriginOptions, http.StatusNotFound, "not_found")
	requireNoCORS(t, sameOriginOptions)
	if allow := sameOriginOptions.Header.Get("Allow"); allow != "" {
		t.Fatalf("OPTIONS に、Allow を返した: %s", allow)
	}

	// 利用者も Origin もない GET(画像の読み込みなどの形で、第三者のサイトから送れる)は、
	// 利用者を特定できないため、拒否される。
	requireError(t, s.do(http.MethodGet, messagesPath("bob"), nil, nil), http.StatusUnauthorized, "unauthenticated")
}

// アプリのオリジンからの要求と、Origin のない要求は、通す。どの応答にも、CORS のヘッダーを付けない。
func TestAppOrigin_IsAllowedWithoutCORSHeaders(t *testing.T) {
	s := startServer(t)

	send := s.do(http.MethodPost, messagesPath("bob"),
		map[string]string{"Content-Type": "application/json", "X-Fixed-User": "alice", "Origin": testAppOrigin},
		[]byte(`{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"hello"}`))
	if send.Status != http.StatusOK {
		t.Fatalf("アプリのオリジンからの送信が、完了しなかった: %d %s", send.Status, send.Body)
	}
	requireNoCORS(t, send)

	history := s.do(http.MethodGet, messagesPath("bob"), map[string]string{"X-Fixed-User": "alice"}, nil)
	if history.Status != http.StatusOK {
		t.Fatalf("Origin のない履歴の取得が、完了しなかった: %d", history.Status)
	}
	requireNoCORS(t, history)

	// Origin のない接続の確立(ブラウザ以外のクライアント)は、通す。
	conn, _, err := s.dial("alice", "")
	if err != nil {
		t.Fatalf("Origin のない接続を、確立できない: %v", err)
	}
	_ = conn.CloseNow()
}
