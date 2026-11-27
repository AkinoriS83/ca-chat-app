package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// rawRequest は、HTTP の要求を、書いた通りに送る(http.Client が直してしまう要求のため)。
func (s *testServer) rawRequest(request string) response {
	s.t.Helper()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(s.url, "http://"), eventTimeout)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(eventTimeout))

	if _, err := conn.Write([]byte(request)); err != nil {
		s.t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return response{Status: resp.StatusCode, Header: resp.Header, Body: data}
}

// WS-30: 拒否の応答とエラー応答は、内部の構造が分かる情報を含まない。
// 形式の壊れた要求など、想定しない入力に対しても同じである。
func TestMalformedRequests_GetCodeOnlyResponses(t *testing.T) {
	s := startServer(t)
	alice := s.auth("alice")
	realtimePath := "/api/realtime?session=" + s.as("alice").SessionID
	valid := `{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"hello"}`
	tooLarge := `{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"` + strings.Repeat("a", 1<<20) + `"}`

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		// 形式に合わない要求
		{"壊れた JSON", "POST", messagesPath("bob"), `{"sendId":`, 400, "invalid_input"},
		{"JSON ではない本文", "POST", messagesPath("bob"), `<xml/>`, 400, "invalid_input"},
		{"空の本文", "POST", messagesPath("bob"), ``, 400, "invalid_input"},
		{"必須の項目がない", "POST", messagesPath("bob"), `{"body":"hello"}`, 400, "invalid_input"},
		{"項目の型が違う(sendId)", "POST", messagesPath("bob"), `{"sendId":1,"body":"hello"}`, 400, "invalid_input"},
		{"項目の型が違う(body)", "POST", messagesPath("bob"), `{"sendId":"x","body":["hello"]}`, 400, "invalid_input"},
		{"項目が null", "POST", messagesPath("bob"), `{"sendId":"x","body":null}`, 400, "invalid_input"},
		{"JSON の値の後ろに、余分な内容がある", "POST", messagesPath("bob"), valid + valid, 400, "invalid_input"},
		{"大きさの上限を超える", "POST", messagesPath("bob"), tooLarge, 400, "invalid_input"},
		// 定義されていないパスと方法
		{"定義されていないパス", "GET", "/", ``, 404, "not_found"},
		{"定義されていないパス(/api)", "GET", "/api", ``, 404, "not_found"},
		{"定義されていないパス(会話の一覧)", "GET", "/api/direct-conversations", ``, 404, "not_found"},
		{"定義されていないパス(相手まで)", "GET", "/api/direct-conversations/bob", ``, 404, "not_found"},
		{"定義されていないパス(メッセージの指定)", "GET", messagesPath("bob") + "/1", ``, 404, "not_found"},
		{"定義されていない方法(PUT)", "PUT", messagesPath("bob"), valid, 404, "not_found"},
		{"定義されていない方法(DELETE)", "DELETE", messagesPath("bob"), ``, 404, "not_found"},
		{"定義されていない方法(PATCH)", "PATCH", messagesPath("bob"), valid, 404, "not_found"},
		{"定義されていない方法(OPTIONS)", "OPTIONS", messagesPath("bob"), ``, 404, "not_found"},
		{"接続先への POST", "POST", realtimePath, ``, 404, "not_found"},
		// 正規の形でないパス
		{"先頭に // があるパス", "GET", "//api/direct-conversations/bob/messages", ``, 404, "not_found"},
		{"途中に // があるパス", "GET", "/api//direct-conversations/bob/messages", ``, 404, "not_found"},
		{"末尾に / があるパス", "GET", messagesPath("bob") + "/", ``, 404, "not_found"},
		{". を含むパス", "GET", "/api/./direct-conversations/bob/messages", ``, 404, "not_found"},
		{".. を含むパス", "GET", "/api/x/../direct-conversations/bob/messages", ``, 404, "not_found"},
		{"相手が空のパス", "GET", "/api/direct-conversations//messages", ``, 404, "not_found"},
		{"POST で、// があるパス", "POST", "//api/direct-conversations/bob/messages", valid, 404, "not_found"},
		// WebSocket の接続を確立する要求でない、接続先への GET
		{"接続先への、通常の GET", "GET", realtimePath, ``, 400, "invalid_input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.do(tt.method, tt.path, alice, []byte(tt.body))

			requireError(t, resp, tt.status, tt.code)
			requireNoInternals(t, resp)
			if location := resp.Header.Get("Location"); location != "" {
				t.Fatalf("転送の応答を返した: %s", location)
			}
		})
	}
	s.requireStored(0, 0)
}

// http.Client が直してしまう要求を、そのまま送る。
func TestRawRequests_GetCodeOnlyResponses(t *testing.T) {
	s := startServer(t)

	tests := []struct {
		name    string
		request string
		status  int
		code    string
	}{
		{"OPTIONS *", "OPTIONS * HTTP/1.1\r\nHost: app.test\r\n\r\n", 404, "not_found"},
		{"パーセントで符号化した ..", "GET /api/%2e%2e/api/direct-conversations/bob/messages HTTP/1.1\r\nHost: app.test\r\n\r\n", 404, "not_found"},
		{"パーセントで符号化した /", "GET /api/direct-conversations/bob%2Fmessages HTTP/1.1\r\nHost: app.test\r\n\r\n", 404, "not_found"},
		{"Host が、アプリのものでない", "GET /api/direct-conversations/bob/messages HTTP/1.1\r\nHost: other.example\r\n\r\n", 403, "forbidden_origin"},
		{"Host がない(HTTP/1.0)", "GET /api/direct-conversations/bob/messages HTTP/1.0\r\n\r\n", 403, "forbidden_origin"},
		// CONNECT では、要求の対象(ホスト:ポート)が、Host として扱われる。ループバックのアドレスを指定して、
		// Host の検証を通し、定義されていない方法としての応答を確かめる。
		{"CONNECT", "CONNECT 127.0.0.1:80 HTTP/1.1\r\nHost: 127.0.0.1:80\r\n\r\n", 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.rawRequest(tt.request)

			requireError(t, resp, tt.status, tt.code)
			requireNoInternals(t, resp)
		})
	}
}

// 接続の確立を、WebSocket のライブラリが拒否した場合の応答は、ライブラリが書く。
// この応答も、内部の構造が分かる情報を含まない(WS-30)。
func TestWebSocketHandshakeFailures_DoNotExposeInternals(t *testing.T) {
	s := startServer(t)
	// 接続を確立する要求に添える鍵(16バイトを、Base64 で表したもの)。秘密ではない。
	// 値を文字列で書くと、秘密の検出(gitleaks)が、誤って検出するため、ここで作る。
	sampleKey := base64.StdEncoding.EncodeToString([]byte("the sample nonce"))

	tests := []struct {
		name   string
		header map[string]string
	}{
		{"Connection ヘッダーがない", map[string]string{"Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": sampleKey}},
		{"鍵がない", map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13"}},
		{"鍵が正しくない", map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "short"}},
		{"版が違う", map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "8", "Sec-WebSocket-Key": sampleKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alice := s.as("alice")
			request := "GET /api/realtime?session=" + alice.SessionID + " HTTP/1.1\r\nHost: app.test\r\n" +
				"Cookie: " + sessionCookieName + "=" + alice.Cookie + "\r\n"
			for name, value := range tt.header {
				request += name + ": " + value + "\r\n"
			}
			resp := s.rawRequest(request + "\r\n")

			if resp.Status < 400 || resp.Status >= 500 {
				t.Fatalf("拒否の応答ではない: %d %s", resp.Status, resp.Body)
			}
			requireNoInternals(t, resp)
		})
	}

	// 失敗した接続の確立は、配信の宛先として残らない。以後の配信は、妨げられない。
	bob := s.connect("bob")
	bob.requireNext(s.mustSend("alice", "bob", "hello"))
}

// RL-15(WS-16 に代わる): ログイン状態を添えない、リアルタイムの接続は、拒否される。
// 接続は、確立されない。識別名を添えるだけの接続も、同じである(RL-42)。
func TestRealtime_RejectsUnknownUser(t *testing.T) {
	s := startServer(t)

	for _, user := range []string{"", "mallory", "Alice", "00000000-0000-4000-8000-000000000003"} {
		conn, resp, err := s.dial(user, testAppOrigin)
		if err == nil {
			_ = conn.CloseNow()
			t.Fatalf("利用者を特定できない接続(%q)が、確立された", user)
		}
		if resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("応答が違う(%q): %v", user, resp)
		}
		data, _ := io.ReadAll(resp.Body)
		if string(data) != `{"code":"unauthenticated"}` {
			t.Fatalf("応答の本文が違う(%q): %s", user, data)
		}
	}
}

// 端末からは、何も送らない。データのメッセージを受けた場合、サーバーは、接続を閉じる。
func TestRealtime_ClosesConnectionWhenDeviceSendsData(t *testing.T) {
	s := startServer(t)
	alice := s.connect("alice")

	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()
	if err := alice.conn.Write(ctx, websocket.MessageText, []byte(`{"type":"message"}`)); err != nil {
		t.Fatal(err)
	}

	_, _, err := alice.conn.Read(ctx)
	if err == nil {
		t.Fatal("接続が、閉じられなかった")
	}
	if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
		t.Fatalf("閉じた理由が違う: %v (%v)", status, err)
	}

	// 他の端末への配信は、妨げられない。
	bob := s.connect("bob")
	bob.requireNext(s.mustSend("alice", "bob", "hello"))
}

// failBeforeCommit は、トランザクションの中の操作を、実際に行った後、確定の直前に失敗する、
// port.Transactor の実装である。
type failBeforeCommit struct {
	inner port.Transactor
	err   error
	// signupErr は、登録の完了のトランザクションを、確定の直前に失敗させる。
	signupErr error
}

func (f *failBeforeCommit) InTransaction(ctx context.Context, fn func(ctx context.Context, w port.ConversationWriter) error) error {
	return f.inner.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
		if err := fn(ctx, w); err != nil {
			return err
		}
		return f.err
	})
}

func (f *failBeforeCommit) InSignupTransaction(ctx context.Context, fn func(ctx context.Context, w port.SignupWriter) error) error {
	return f.inner.InSignupTransaction(ctx, func(ctx context.Context, w port.SignupWriter) error {
		if err := fn(ctx, w); err != nil {
			return err
		}
		return f.signupErr
	})
}

// WS-31: 保管に失敗した送信は、完了として示されず、配信されない。
func TestStoreFailure_IsNotCompletedNorDelivered(t *testing.T) {
	const marker = "INJECTED-FAILURE-DETAIL at /home/secret/store.go:42"
	failing := &failBeforeCommit{err: errors.New(marker)}
	s := startServer(t, withTransactor(func(inner port.Transactor) port.Transactor {
		failing.inner = inner
		return failing
	}))
	alice := s.connect("alice")
	bob := s.connect("bob")

	resp := s.send("alice", "bob", newSendID(t), "will fail")

	requireError(t, resp, http.StatusInternalServerError, "internal")
	requireNoInternals(t, resp)
	s.requireStored(0, 0)
	if history := s.mustHistory("bob", "alice"); len(history) != 0 {
		t.Fatalf("保管に失敗した送信が、履歴に現れた: %+v", history)
	}

	// 保管できるようになった後の送信は、完了する。失敗した送信は、配信されていない(番兵)。
	failing.err = nil
	sentinel := s.mustSend("alice", "bob", "sentinel")
	if sentinel.Seq != 1 {
		t.Fatalf("失敗した送信が、連番を使った: %d", sentinel.Seq)
	}
	bob.requireNext(sentinel)
	alice.requireNext(sentinel)
}
