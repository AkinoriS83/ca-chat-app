package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// WS-01: 送信の応答が、連番、送信者、送信の日時を持つ。
func TestSend_CompletesWithStoredMessage(t *testing.T) {
	s := startServer(t)
	before := time.Now().Add(-time.Minute)

	resp := s.send("alice", "bob", newSendID(t), "hello")

	if resp.Status != http.StatusOK {
		t.Fatalf("送信が完了しなかった: %d %s", resp.Status, resp.Body)
	}
	var out openapi.SendMessageResponse
	decodeStrict(t, resp.Body, &out)
	msg := out.Message
	if msg.Seq != 1 || msg.Body != "hello" {
		t.Fatalf("メッセージが違う: %+v", msg)
	}
	if msg.Sender.Handle != "alice" || msg.Sender.DisplayName != "Alice" {
		t.Fatalf("送信者が違う: %+v", msg.Sender)
	}
	if !reflect.DeepEqual(msg.Participants, []string{"alice", "bob"}) {
		t.Fatalf("参加者が違う: %v", msg.Participants)
	}
	if msg.SentAt.Before(before) || msg.SentAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("送信の日時が、保管した時点ではない: %v", msg.SentAt)
	}
	// 送信の日時は、UTC の文字列で表す。
	var raw struct {
		Message struct {
			SentAt string `json:"sentAt"`
		} `json:"message"`
	}
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(raw.Message.SentAt, "Z") {
		t.Fatalf("送信の日時が、UTC の表現ではない: %s", raw.Message.SentAt)
	}
	// 送信の識別子は、Message には含めない(ADR-011)。decodeStrict が、未知の項目を拒否している。
	s.requireStored(1, 1)
}

// WS-04、WS-20: 接続のない bob が、後から、履歴として読める。
// 履歴は、連番の順で、送信者の表示名と日時を持つ。
func TestHistory_ReturnsAllMessagesInOrder(t *testing.T) {
	s := startServer(t)

	sent := []openapi.Message{
		s.mustSend("alice", "bob", "one"),
		s.mustSend("bob", "alice", "two"),
		s.mustSend("alice", "bob", "three"),
	}

	for _, viewer := range [][2]string{{"bob", "alice"}, {"alice", "bob"}} {
		got := s.mustHistory(viewer[0], viewer[1])
		if len(got) != len(sent) {
			t.Fatalf("%s の履歴の数が違う: %d", viewer[0], len(got))
		}
		for i := range sent {
			if got[i].Seq != int64(i+1) {
				t.Fatalf("連番の順ではない: %d 番目が %d", i, got[i].Seq)
			}
			// 履歴のメッセージは、完了の応答のメッセージと、一致する。
			if !sameMessage(got[i], sent[i]) {
				t.Fatalf("履歴が、完了の応答と違う:\n got %+v\nwant %+v", got[i], sent[i])
			}
		}
		if got[1].Sender.DisplayName != "Bob" || got[0].Sender.DisplayName != "Alice" {
			t.Fatalf("送信者の表示名が違う: %+v", got)
		}
	}
}

// WS-21、WS-06: 成立していない会話の履歴は、空の一覧である。
// 開いただけでは、会話は成立せず、相手の端末には、何も届かない。
func TestHistory_UnestablishedConversation(t *testing.T) {
	s := startServer(t)
	bob := s.connect("bob")

	resp := s.history("alice", "bob")

	if resp.Status != http.StatusOK || string(resp.Body) != `{"messages":[]}` {
		t.Fatalf("空の一覧ではない: %d %s", resp.Status, resp.Body)
	}
	s.requireStored(0, 0)

	// 番兵: bob の端末が次に受け取るのは、carol からのメッセージである。
	sentinel := s.mustSend("carol", "bob", "sentinel")
	bob.requireNext(sentinel)
}

// WS-05: 最初の送信で、会話が成立する。どちらが先に送信しても、会話は1つである。
func TestFirstSend_EstablishesOneConversation(t *testing.T) {
	t.Run("順に送る", func(t *testing.T) {
		s := startServer(t)
		s.requireStored(0, 0)

		first := s.mustSend("alice", "bob", "from alice")
		s.requireStored(1, 1)
		second := s.mustSend("bob", "alice", "from bob")
		s.requireStored(1, 2)

		if first.Seq != 1 || second.Seq != 2 {
			t.Fatalf("連番が違う: %d, %d", first.Seq, second.Seq)
		}
	})

	t.Run("同時に送る", func(t *testing.T) {
		s := startServer(t)

		var wg sync.WaitGroup
		results := make([]openapi.Message, 2)
		for i, pair := range [][2]string{{"alice", "bob"}, {"bob", "alice"}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = s.mustSend(pair[0], pair[1], "first from "+pair[0])
			}()
		}
		wg.Wait()

		s.requireStored(1, 2)
		seqs := []int64{results[0].Seq, results[1].Seq}
		sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
		if seqs[0] != 1 || seqs[1] != 2 {
			t.Fatalf("連番が、1と2ではない: %v", seqs)
		}
	})
}

// WS-07: 2人が、並行して、複数通を送る。2人の履歴が一致し、連番に欠番と重複がない。
// 配信で受け取った連番の集合が、履歴と一致する。
func TestConcurrentSends_ShareOneOrder(t *testing.T) {
	s := startServer(t)
	aliceConn := s.connect("alice")
	bobConn := s.connect("bob")

	const perSender = 10
	var wg sync.WaitGroup
	for _, pair := range [][2]string{{"alice", "bob"}, {"bob", "alice"}} {
		for i := 0; i < perSender; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.mustSend(pair[0], pair[1], pair[0]+" message")
			}()
		}
	}
	wg.Wait()

	aliceHistory := s.mustHistory("alice", "bob")
	bobHistory := s.mustHistory("bob", "alice")
	if len(aliceHistory) != 2*perSender {
		t.Fatalf("履歴の数が違う: %d", len(aliceHistory))
	}
	if canonicalJSON(aliceHistory) != canonicalJSON(bobHistory) {
		t.Fatal("2人の履歴が、一致しない")
	}
	for i, msg := range aliceHistory {
		if msg.Seq != int64(i+1) {
			t.Fatalf("連番に、欠番か重複がある: %d 番目が %d", i, msg.Seq)
		}
		if i > 0 && msg.SentAt.Before(aliceHistory[i-1].SentAt) {
			t.Fatalf("送信の日時が、連番の順と前後した: seq %d", msg.Seq)
		}
	}

	for name, conn := range map[string]*wsClient{"alice": aliceConn, "bob": bobConn} {
		delivered := map[int64]openapi.Message{}
		for i := 0; i < 2*perSender; i++ {
			ev := conn.next()
			if _, dup := delivered[ev.Message.Seq]; dup {
				t.Fatalf("%s の端末に、同じ連番が2回届いた: %d", name, ev.Message.Seq)
			}
			delivered[ev.Message.Seq] = ev.Message
		}
		for _, msg := range aliceHistory {
			got, ok := delivered[msg.Seq]
			if !ok || !sameMessage(got, msg) {
				t.Fatalf("%s の端末に届いた内容が、履歴と違う: seq %d", name, msg.Seq)
			}
		}
	}
}

// WS-02、WS-03、WS-37: 相手の端末、送信者の別の端末、送信に使われた端末(を含む、送信者の端末)に、
// 操作なしに届く。完了の応答、履歴、配信の Message が、一致する。
func TestDelivery_ReachesAllDevicesOfParticipants(t *testing.T) {
	// 識別子の順と、識別名の順が異なる組(alice と bob、alice と carol)で確かめる。
	// participants は、識別名の昇順である。
	tests := []struct {
		sender, peer     string
		wantParticipants []string
	}{
		{"alice", "bob", []string{"alice", "bob"}},
		{"bob", "alice", []string{"alice", "bob"}},
		{"carol", "alice", []string{"alice", "carol"}},
		{"bob", "carol", []string{"bob", "carol"}},
	}
	for _, tt := range tests {
		t.Run(tt.sender+"→"+tt.peer, func(t *testing.T) {
			s := startServer(t)
			sender1 := s.connect(tt.sender)
			sender2 := s.connect(tt.sender)
			peer1 := s.connect(tt.peer)
			sendID := strings.ToUpper(newSendID(t))

			completed := s.mustSendWithID(tt.sender, tt.peer, sendID, "hello\nworld")

			if !reflect.DeepEqual(completed.Participants, tt.wantParticipants) {
				t.Fatalf("参加者が、識別名の昇順ではない: %v", completed.Participants)
			}
			history := s.mustHistory(tt.peer, tt.sender)
			if len(history) != 1 || !sameMessage(history[0], completed) {
				t.Fatalf("履歴が、完了の応答と違う: %+v", history)
			}

			var rawMessages []string
			for name, conn := range map[string]*wsClient{"送信者の端末1": sender1, "送信者の端末2": sender2, "相手の端末": peer1} {
				ev := conn.requireNext(completed)
				// 配信には、送信の識別子を含める(ADR-011)。小文字に揃えた値である。
				if ev.SendID != strings.ToLower(sendID) {
					t.Fatalf("%s に届いた、送信の識別子が違う: %s", name, ev.SendID)
				}
				rawMessages = append(rawMessages, string(ev.RawMsg))
			}

			// 変換が2箇所にあること(httpapi と wsdelivery)の検出。項目の表現(送信の日時の文字列を含む)が、
			// 完了の応答と配信で、一致する。
			var rawResp struct {
				Message json.RawMessage `json:"message"`
			}
			resp := s.send(tt.sender, tt.peer, sendID, "hello\nworld") // 送り直し。保管済みのメッセージを返す
			if err := json.Unmarshal(resp.Body, &rawResp); err != nil {
				t.Fatal(err)
			}
			for _, raw := range rawMessages {
				if normalizeJSON(t, raw) != normalizeJSON(t, string(rawResp.Message)) {
					t.Fatalf("配信と、完了の応答の、Message の表現が違う:\n配信 %s\n応答 %s", raw, rawResp.Message)
				}
			}
		})
	}
}

// WS-25: alice と bob の会話のメッセージは、アプリを開いている carol の端末に届かない。
func TestDelivery_DoesNotReachNonParticipant(t *testing.T) {
	s := startServer(t)
	carol := s.connect("carol")

	s.mustSend("alice", "bob", "secret from alice")
	s.mustSend("bob", "alice", "secret from bob")

	// 番兵: carol の端末が次に受け取るのは、alice が carol に送ったメッセージである。
	sentinel := s.mustSend("alice", "carol", "sentinel")
	carol.requireNext(sentinel)
}

// WS-09〜WS-11、WS-28: 正しい本文は、改行の正規化を除き、入力された通りに保管し、返す。
func TestBodies_AreStoredAsSent(t *testing.T) {
	s := startServer(t)
	bob := s.connect("bob")

	astral := string(rune(0x1F600))
	family := string([]rune{0x1F468, 0x200D, 0x1F469, 0x200D, 0x1F467})
	tests := []struct {
		name string
		body string
		want string
	}{
		{"4,000文字", strings.Repeat("a", 4000), strings.Repeat("a", 4000)},
		{"基本多言語面の外の文字だけで4,000文字", strings.Repeat(astral, 4000), strings.Repeat(astral, 4000)},
		{"LF とタブ", "a\n\tb", "a\n\tb"},
		{"CRLF は LF になる", "a\r\nb", "a\nb"},
		{"単独の CR は LF になる", "a\rb", "a\nb"},
		{"前後の空白を保つ", "  hello  \n", "  hello  \n"},
		{"結合した絵文字", family, family},
		{"表示されない文字だけ", string(rune(0x200B)), string(rune(0x200B))},
		{"問い合わせの中で意味を持つ文字", `'; DROP TABLE messages; -- " \ % _ $1 $$ ; /* */`, `'; DROP TABLE messages; -- " \ % _ $1 $$ ; /* */`},
		{"画面の構造として解釈されうる記述", `<script>alert(1)</script><img src=x onerror=alert(1)>&lt;&amp;`, `<script>alert(1)</script><img src=x onerror=alert(1)>&lt;&amp;`},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.mustSend("alice", "bob", tt.body)
			if got.Body != tt.want {
				t.Fatalf("応答の本文が違う: got %q, want %q", got.Body, tt.want)
			}
			if got.Seq != int64(i+1) {
				t.Fatalf("連番が違う: %d", got.Seq)
			}
			if ev := bob.requireNext(got); ev.Message.Body != tt.want {
				t.Fatalf("配信の本文が違う: %q", ev.Message.Body)
			}
		})
	}

	history := s.mustHistory("bob", "alice")
	if len(history) != len(tests) {
		t.Fatalf("履歴の数が違う: %d", len(history))
	}
	for i, tt := range tests {
		if history[i].Body != tt.want {
			t.Fatalf("履歴の本文が違う(%s): %q", tt.name, history[i].Body)
		}
	}
	// 表は、残っている。
	s.requireStored(1, len(tests))
}

// 本文の項目を、JSON のエスケープで表した場合も、同じに扱う。
func TestBodies_EscapedInJSON(t *testing.T) {
	s := startServer(t)
	esc := func(hex string) string { return string(rune(0x5C)) + "u" + hex }

	// 対になったサロゲートのエスケープは、1つのコードポイントとして数える(4,000文字)。
	resp := s.sendBodyJSON("alice", "bob", newSendID(t), `"`+strings.Repeat(esc("d83d")+esc("de00"), 4000)+`"`)
	if resp.Status != http.StatusOK {
		t.Fatalf("送信が完了しなかった: %d %s", resp.Status, resp.Body)
	}
	var out openapi.SendMessageResponse
	decodeStrict(t, resp.Body, &out)
	if out.Message.Body != strings.Repeat(string(rune(0x1F600)), 4000) {
		t.Fatal("本文が違う")
	}

	resp = s.sendBodyJSON("alice", "bob", newSendID(t), `"`+strings.Repeat(esc("d83d")+esc("de00"), 4001)+`"`)
	requireError(t, resp, http.StatusBadRequest, "invalid_body")
}

func normalizeJSON(t *testing.T, raw string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("JSON として読めない: %v: %s", err, raw)
	}
	return canonicalJSON(v)
}
