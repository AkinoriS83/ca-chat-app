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
		h := s.auth("alice")
		h["Origin"] = origin
		return h
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
		"Access-Control-Request-Headers": "content-type,x-session-id",
	}, nil)
	requireError(t, preflight, http.StatusForbidden, "forbidden_origin")
	requireNoCORS(t, preflight)
	sameOriginOptions := s.do(http.MethodOptions, messagesPath("bob"), map[string]string{"Origin": testAppOrigin}, nil)
	requireError(t, sameOriginOptions, http.StatusNotFound, "not_found")
	requireNoCORS(t, sameOriginOptions)
	if allow := sameOriginOptions.Header.Get("Allow"); allow != "" {
		t.Fatalf("OPTIONS に、Allow を返した: %s", allow)
	}

	// Origin が付かない GET(画像の読み込みなどの形で、他のページから送れる)。
	// 他のサイトのページからの要求には、SameSite=Strict によって、資格が付かない。
	requireError(t, s.do(http.MethodGet, messagesPath("bob"), nil, nil), http.StatusUnauthorized, "unauthenticated")
	// 同じサイトで、オリジンが異なるページ(localhost の別のポートなど)からの要求には、資格が付く。
	// そのページは、ログイン状態の識別子を読めず、専用の要求ヘッダーも付けられないため、拒否される(ADR-014)。
	cookieOnly := map[string]string{"Cookie": sessionCookieName + "=" + s.as("alice").Cookie}
	requireError(t, s.do(http.MethodGet, messagesPath("bob"), cookieOnly, nil), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.do(http.MethodGet, "/api/users/bob", cookieOnly, nil), http.StatusUnauthorized, "unauthenticated")
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
		history := doWithHost(http.MethodGet, host, s.auth("alice"), nil)
		requireError(t, history, http.StatusForbidden, "forbidden_origin")
		requireNoCORS(t, history)

		// 送信。Origin がない場合も、Origin が、そのサイトのものである場合も、拒否する。
		for _, origin := range []string{"", "http://" + host} {
			header := s.auth("alice")
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
	viaApp := doWithHost(http.MethodGet, strings.TrimPrefix(testAppOrigin, "http://"), s.auth("alice"), nil)
	if viaApp.Status != http.StatusOK {
		t.Fatalf("アプリのオリジンのホストに向けた履歴の取得が、完了しなかった: %d %s", viaApp.Status, viaApp.Body)
	}
}

// アプリのオリジンからの要求と、Origin のない要求は、通す。どの応答にも、CORS のヘッダーを付けない。
func TestAppOrigin_IsAllowedWithoutCORSHeaders(t *testing.T) {
	s := startServer(t)

	withOrigin := s.auth("alice")
	withOrigin["Origin"] = testAppOrigin
	send := s.do(http.MethodPost, messagesPath("bob"), withOrigin,
		[]byte(`{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"hello"}`))
	if send.Status != http.StatusOK {
		t.Fatalf("アプリのオリジンからの送信が、完了しなかった: %d %s", send.Status, send.Body)
	}
	requireNoCORS(t, send)

	history := s.do(http.MethodGet, messagesPath("bob"), s.auth("alice"), nil)
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
