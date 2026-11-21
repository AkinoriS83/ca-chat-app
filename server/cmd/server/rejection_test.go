package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// WS-09〜WS-16、WS-12: 拒否のそれぞれについて、応答の状態と code を確かめる。
// 拒否された送信は、保管されず、配信されず、会話を成立させない。
func TestSend_Rejections(t *testing.T) {
	esc := func(hex string) string { return string(rune(0x5C)) + "u" + hex }
	quoted := func(s string) string { return `"` + s + `"` }
	validID := "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70"
	astral := string(rune(0x1F600))

	tests := []struct {
		name     string
		actor    string
		peer     string
		sendID   string
		bodyJSON string // 本文の項目の、JSON の表現
		status   int
		code     string
	}{
		// 本文(WS-09)
		{"空の本文", "alice", "bob", validID, quoted(""), 400, "invalid_body"},
		{"U+0020 だけの本文", "alice", "bob", validID, quoted("   "), 400, "invalid_body"},
		{"改行とタブだけの本文", "alice", "bob", validID, quoted(`\n\t\r\n`), 400, "invalid_body"},
		{"U+3000 だけの本文", "alice", "bob", validID, quoted(esc("3000")), 400, "invalid_body"},
		{"U+00A0 と U+2028 だけの本文", "alice", "bob", validID, quoted(esc("00a0") + esc("2028")), 400, "invalid_body"},
		// 長さ(WS-10)
		{"4,001文字の本文", "alice", "bob", validID, quoted(strings.Repeat("a", 4001)), 400, "invalid_body"},
		{"基本多言語面の外の文字だけで4,001文字", "alice", "bob", validID, quoted(strings.Repeat(astral, 4001)), 400, "invalid_body"},
		{"改行の正規化の後も4,001文字", "alice", "bob", validID, quoted(strings.Repeat("a", 4000) + `\r\n`), 400, "invalid_body"},
		// 使える文字(WS-11)
		{"U+0000 を含む本文", "alice", "bob", validID, quoted("a" + esc("0000") + "b"), 400, "invalid_body"},
		{"U+001F を含む本文", "alice", "bob", validID, quoted("a" + esc("001f") + "b"), 400, "invalid_body"},
		{"U+007F を含む本文", "alice", "bob", validID, quoted("a\x7Fb"), 400, "invalid_body"},
		{"U+0085 を含む本文", "alice", "bob", validID, quoted("a" + esc("0085") + "b"), 400, "invalid_body"},
		{"対にならない上位サロゲートのエスケープ", "alice", "bob", validID, quoted("a" + esc("d83d") + "b"), 400, "invalid_body"},
		{"対にならない下位サロゲートのエスケープ", "alice", "bob", validID, quoted("a" + esc("de00") + "b"), 400, "invalid_body"},
		{"正しくない UTF-8 のバイト", "alice", "bob", validID, quoted("a\xFFb"), 400, "invalid_body"},
		{"UTF-8 の規則で符号化したサロゲートのバイト", "alice", "bob", validID, quoted("a\xED\xA0\x80b"), 400, "invalid_body"},
		// 送信の識別子(WS-13)
		{"送信の識別子が空", "alice", "bob", "", quoted("hello"), 400, "invalid_input"},
		{"送信の識別子が短い", "alice", "bob", validID[:35], quoted("hello"), 400, "invalid_input"},
		{"送信の識別子が長い", "alice", "bob", validID + "0", quoted("hello"), 400, "invalid_input"},
		{"送信の識別子のハイフンの位置が違う", "alice", "bob", "0b6f2d3e8-a51-4c0e-9f1d-2a7c4e5b6d70", quoted("hello"), 400, "invalid_input"},
		{"送信の識別子に、16進数でない文字がある", "alice", "bob", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d7g", quoted("hello"), 400, "invalid_input"},
		{"送信の識別子にハイフンがない", "alice", "bob", "0b6f2d3e8a514c0e9f1d2a7c4e5b6d70", quoted("hello"), 400, "invalid_input"},
		// 相手(WS-14、WS-15)
		{"自分自身を相手とする", "alice", "alice", validID, quoted("hello"), 400, "invalid_peer"},
		{"固定ユーザーの中に居ない相手", "alice", "dave", validID, quoted("hello"), 404, "not_found"},
		{"相手の識別名の、大文字と小文字が違う", "alice", "Bob", validID, quoted("hello"), 404, "not_found"},
		{"相手に、利用者の識別子を指定する", "alice", "00000000-0000-4000-8000-000000000001", validID, quoted("hello"), 404, "not_found"},
		// 利用者(WS-16)
		{"利用者を添えない", "", "bob", validID, quoted("hello"), 401, "unauthenticated"},
		{"固定ユーザーでない利用者", "mallory", "bob", validID, quoted("hello"), 401, "unauthenticated"},
		{"利用者に、利用者の識別子を指定する", "00000000-0000-4000-8000-000000000003", "bob", validID, quoted("hello"), 401, "unauthenticated"},
		// 複数の場合に当たる送信には、仕様の表の上にあるものの応答を返す。
		{"利用者の確認は、本文の検証より先", "mallory", "dave", "x", quoted(""), 401, "unauthenticated"},
		{"本文の検証は、送信の識別子の検証より先", "alice", "dave", "x", quoted(""), 400, "invalid_body"},
		{"送信の識別子の検証は、相手の確認より先", "alice", "alice", "x", quoted("hello"), 400, "invalid_input"},
		{"自分自身の判定は、相手が居ないことより先", "alice", "alice", validID, quoted("hello"), 400, "invalid_peer"},
	}

	s := startServer(t)
	alice := s.connect("alice")
	bob := s.connect("bob")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.sendBodyJSON(tt.actor, tt.peer, tt.sendID, tt.bodyJSON)

			requireError(t, resp, tt.status, tt.code)
			requireNoInternals(t, resp)
			s.requireStored(0, 0)
		})
	}

	// 拒否された送信は、配信されていない(番兵)。
	sentinel := s.mustSend("carol", "alice", "sentinel for alice")
	alice.requireNext(sentinel)
	sentinel = s.mustSend("carol", "bob", "sentinel for bob")
	bob.requireNext(sentinel)
	if got := s.mustHistory("alice", "bob"); len(got) != 0 {
		t.Fatalf("拒否された送信が、履歴に現れた: %+v", got)
	}
}

// WS-13: 送信の識別子の、16進数の大文字は、受け付ける。大文字と小文字は、区別しない。
func TestSend_UppercaseSendIDIsAccepted(t *testing.T) {
	s := startServer(t)
	id := "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70"

	first := s.mustSendWithID("alice", "bob", strings.ToUpper(id), "hello")
	again := s.mustSendWithID("alice", "bob", id, "hello")

	if !sameMessage(first, again) {
		t.Fatal("大文字と小文字だけが異なる識別子が、別の送信として扱われた")
	}
	s.requireStored(1, 1)
}

// WS-38、WS-14、WS-15: 会話を開くこと(履歴の取得)の拒否。
// 「見つからない」応答は、送信と履歴で、状態と本文が一致する。
func TestHistory_Rejections(t *testing.T) {
	s := startServer(t)
	// alice と bob の会話は、成立している。
	s.mustSend("alice", "bob", "hello")

	requireError(t, s.history("alice", "alice"), http.StatusBadRequest, "invalid_peer")
	requireError(t, s.history("", "bob"), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.history("mallory", "bob"), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.history("mallory", "dave"), http.StatusUnauthorized, "unauthenticated")

	historyNotFound := s.history("alice", "dave")
	sendNotFound := s.send("alice", "dave", newSendID(t), "hello")
	requireError(t, historyNotFound, http.StatusNotFound, "not_found")
	requireError(t, sendNotFound, http.StatusNotFound, "not_found")
	if historyNotFound.Status != sendNotFound.Status || string(historyNotFound.Body) != string(sendNotFound.Body) {
		t.Fatal("「見つからない」応答が、送信と履歴で一致しない")
	}
	// 定義されていないパスの応答とも、区別できない。
	undefined := s.do(http.MethodGet, "/api/direct-conversations/dave", map[string]string{"X-Fixed-User": "alice"}, nil)
	if undefined.Status != historyNotFound.Status || string(undefined.Body) != string(historyNotFound.Body) {
		t.Fatal("「見つからない」応答が、定義されていないパスの応答と一致しない")
	}
	s.requireStored(1, 1)
}

// 形式の検証は、利用者の確認より先に行う(api/openapi.yaml「拒否とエラーの応答」の順)。
func TestSend_FormatIsCheckedBeforeUser(t *testing.T) {
	s := startServer(t)

	resp := s.sendRaw("", "bob", []byte(`{"sendId":"x"`))

	requireError(t, resp, http.StatusBadRequest, "invalid_input")
}

// WS-18: 保管済みのメッセージと、送信者と送信の識別子がともに一致する送信に対して、完了を示す。
// 新しいメッセージは保管されず、再び配信もされない。
func TestResend_CompletesWithoutStoringOrDelivering(t *testing.T) {
	s := startServer(t)
	bob := s.connect("bob")
	alice := s.connect("alice")
	sendID := newSendID(t)

	first := s.mustSendWithID("alice", "bob", sendID, "original")
	bob.requireNext(first)
	alice.requireNext(first)

	// 本文が異なっていても、最初に保管した内容を保つ。
	again := s.mustSendWithID("alice", "bob", sendID, "changed")

	if !sameMessage(again, first) {
		t.Fatalf("保管済みのメッセージと違う:\n got %+v\nwant %+v", again, first)
	}
	s.requireStored(1, 1)
	if history := s.mustHistory("bob", "alice"); len(history) != 1 || !sameMessage(history[0], first) {
		t.Fatalf("履歴が、1通だけではない: %+v", history)
	}

	// 再び配信されていない(番兵)。
	sentinel := s.mustSend("alice", "bob", "sentinel")
	bob.requireNext(sentinel)
	alice.requireNext(sentinel)
}

// WS-18: 同じ識別子の送信を、同時に受けた場合も、保管されるのは1通である。
func TestResend_ConcurrentSendsStoreOneMessage(t *testing.T) {
	s := startServer(t)
	bob := s.connect("bob")
	sendID := newSendID(t)

	const attempts = 8
	results := make(chan openapi.Message, attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			results <- s.mustSendWithID("alice", "bob", sendID, "same")
		}()
	}
	var first openapi.Message
	for i := 0; i < attempts; i++ {
		got := <-results
		if i == 0 {
			first = got
			continue
		}
		if !sameMessage(got, first) {
			t.Fatalf("同じ識別子の送信が、別のメッセージで完了した:\n%+v\n%+v", got, first)
		}
	}
	if first.Seq != 1 {
		t.Fatalf("連番が違う: %d", first.Seq)
	}
	s.requireStored(1, 1)

	// 配信は、1回だけである(番兵)。次の連番は、2である(取り消された送信が、欠番を作らない)。
	bob.requireNext(first)
	sentinel := s.mustSend("alice", "bob", "sentinel")
	if sentinel.Seq != 2 {
		t.Fatalf("欠番が生じた: %d", sentinel.Seq)
	}
	bob.requireNext(sentinel)
}

// WS-19: 別の送信者が、同じ送信の識別子で送信した場合、別のメッセージとして保管される。
func TestSameSendIDFromAnotherSender_IsAnotherMessage(t *testing.T) {
	s := startServer(t)
	sendID := newSendID(t)

	fromAlice := s.mustSendWithID("alice", "bob", sendID, "from alice")
	fromBob := s.mustSendWithID("bob", "alice", sendID, "from bob")

	if fromBob.Seq != 2 || fromBob.Body != "from bob" || fromBob.Sender.Handle != "bob" {
		t.Fatalf("別のメッセージとして保管されていない: %+v", fromBob)
	}
	s.requireStored(1, 2)
	// 互いの送り直しは、それぞれの保管済みのメッセージについて、完了する。
	if again := s.mustSendWithID("alice", "bob", sendID, "x"); !sameMessage(again, fromAlice) {
		t.Fatal("alice の送り直しが、alice のメッセージで完了しなかった")
	}
	if again := s.mustSendWithID("bob", "alice", sendID, "x"); !sameMessage(again, fromBob) {
		t.Fatal("bob の送り直しが、bob のメッセージで完了しなかった")
	}
}

// WS-39: 識別子が一致していても、本文が正しくない場合と、別の会話を指定した場合は、拒否する。
// 保管済みのメッセージは変わらず、別の相手との会話は成立しない。
func TestResend_RejectedWhenInvalidOrInAnotherConversation(t *testing.T) {
	s := startServer(t)
	carol := s.connect("carol")
	sendID := newSendID(t)
	original := s.mustSendWithID("alice", "bob", sendID, "original")

	requireError(t, s.send("alice", "bob", sendID, "   "), http.StatusBadRequest, "invalid_body")
	// 成立していない会話(alice と carol)を指定した場合。
	requireError(t, s.send("alice", "carol", sendID, "to carol"), http.StatusBadRequest, "invalid_input")
	s.requireStored(1, 1)
	requireError(t, s.send("alice", "alice", sendID, "to myself"), http.StatusBadRequest, "invalid_peer")
	requireError(t, s.send("alice", "dave", sendID, "to nobody"), http.StatusNotFound, "not_found")

	// 成立している会話(bob と carol は無関係。alice と carol を成立させる)を指定した場合。
	established := s.mustSend("alice", "carol", "hello carol")
	carol.requireNext(established)
	requireError(t, s.send("alice", "carol", sendID, "to carol again"), http.StatusBadRequest, "invalid_input")

	s.requireStored(2, 2)
	if history := s.mustHistory("alice", "bob"); len(history) != 1 || !sameMessage(history[0], original) {
		t.Fatalf("保管済みのメッセージが変わった: %+v", history)
	}
	if history := s.mustHistory("carol", "alice"); len(history) != 1 {
		t.Fatalf("拒否された送信が、別の会話に現れた: %+v", history)
	}
	// 拒否された送信は、carol の端末に、配信されていない(番兵)。
	sentinel := s.mustSend("bob", "carol", "sentinel")
	carol.requireNext(sentinel)
}
