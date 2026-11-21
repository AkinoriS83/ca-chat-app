package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"
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

// WS-29: 第三者のサイトのページが、自分のホストの名前を、このサーバーのアドレスに向け直した場合
// (DNS rebinding)。そのページからの GET は、ブラウザにとって同一オリジンであり、Origin が付かない。
// 要求は、Host に、そのサイトの名前を持つ。Host の検証で、拒否する。
func TestOtherHost_IsRejected(t *testing.T) {
	s := startServer(t)
	// alice と bob の会話は、成立している。読めてはならない。
	s.mustSend("alice", "bob", "secret")
	body := []byte(`{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"hello"}`)

	doWithHost := func(method, host string, header map[string]string, body []byte) response {
		t.Helper()
		req, err := http.NewRequest(method, s.url+messagesPath("bob"), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		for name, value := range header {
			req.Header.Set(name, value)
		}
		resp, err := s.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response{Status: resp.StatusCode, Header: resp.Header, Body: data}
	}

	for _, host := range []string{"attacker.example:8080", "attacker.example", "localhost:8080", "127.0.0.1.attacker.example"} {
		// 履歴の取得。同一オリジンの GET の形(Origin なし)。
		history := doWithHost(http.MethodGet, host, map[string]string{"X-Fixed-User": "alice"}, nil)
		requireError(t, history, http.StatusForbidden, "forbidden_origin")
		requireNoCORS(t, history)

		// 送信。Origin がない場合も、Origin が、そのサイトのものである場合も、拒否する。
		for _, origin := range []string{"", "http://" + host} {
			header := map[string]string{"Content-Type": "application/json", "X-Fixed-User": "alice"}
			if origin != "" {
				header["Origin"] = origin
			}
			send := doWithHost(http.MethodPost, host, header, body)
			requireError(t, send, http.StatusForbidden, "forbidden_origin")
			requireNoCORS(t, send)
		}

		// リアルタイムの接続。
		ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
		conn, resp, err := websocket.Dial(ctx, s.realtimeURL("alice"), &websocket.DialOptions{HTTPClient: s.http, Host: host})
		cancel()
		if err == nil {
			_ = conn.CloseNow()
			t.Fatalf("他のホスト(%s)に向けた接続が、確立された", host)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("他のホスト(%s)に向けた接続の応答が違う: %v", host, resp)
		}
	}
	s.requireStored(1, 1)

	// アプリのオリジンのホスト(クライアントを配信するサーバーが、Host を書き換えずに中継する要求)は、通す。
	viaApp := doWithHost(http.MethodGet, strings.TrimPrefix(testAppOrigin, "http://"), map[string]string{"X-Fixed-User": "alice"}, nil)
	if viaApp.Status != http.StatusOK {
		t.Fatalf("アプリのオリジンのホストに向けた履歴の取得が、完了しなかった: %d %s", viaApp.Status, viaApp.Body)
	}
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
