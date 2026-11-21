package wsdelivery

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

var (
	alice = domain.User{ID: "00000000-0000-4000-8000-000000000003", Handle: "alice", DisplayName: "Alice"}
	bob   = domain.User{ID: "00000000-0000-4000-8000-000000000001", Handle: "bob", DisplayName: "Bob"}
	carol = domain.User{ID: "00000000-0000-4000-8000-000000000002", Handle: "carol", DisplayName: "Carol"}
)

// recordingSink は、port.ConnectionSink の、テスト用の実装である。
type recordingSink struct {
	mu     sync.Mutex
	accept bool
	sent   [][]byte
	closed int
}

func newRecordingSink() *recordingSink { return &recordingSink{accept: true} }

func (s *recordingSink) Send(data []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accept {
		return false
	}
	s.sent = append(s.sent, data)
	return true
}

func (s *recordingSink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
}

func newTestHub() *Hub {
	return NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func testMessage(sender domain.User, body string) domain.Message {
	return domain.Message{
		ConversationID: "c0000000-0000-4000-8000-000000000001",
		Seq:            7,
		SenderID:       sender.ID,
		SendID:         domain.RestoreSendID("11111111-1111-4111-8111-111111111111"),
		Body:           domain.RestoreBody(body),
		SentAt:         time.Date(2026, 11, 21, 3, 4, 5, 123456000, time.FixedZone("JST", 9*60*60)),
	}
}

func TestDeliver_SendsToEveryConnectionOfParticipantsOnly(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	alice1, alice2, bob1, carol1 := newRecordingSink(), newRecordingSink(), newRecordingSink(), newRecordingSink()
	hub.Register(ctx, alice.ID, alice1)
	hub.Register(ctx, alice.ID, alice2)
	hub.Register(ctx, bob.ID, bob1)
	hub.Register(ctx, carol.ID, carol1)

	hub.Deliver(ctx, []domain.User{alice, bob}, testMessage(alice, "hello"))

	for name, s := range map[string]*recordingSink{"alice1": alice1, "alice2": alice2, "bob1": bob1} {
		if len(s.sent) != 1 {
			t.Fatalf("%s に、1通が届いていない: %d", name, len(s.sent))
		}
	}
	if len(carol1.sent) != 0 {
		t.Fatal("参加者でない利用者の端末に届いた")
	}
	if string(alice1.sent[0]) != string(bob1.sent[0]) {
		t.Fatal("端末によって、内容が違う")
	}
}

func TestDeliver_EncodesMessageEvent(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	s := newRecordingSink()
	hub.Register(ctx, bob.ID, s)

	// 参加者の順は、識別子の順(bob、alice)で渡す。participants は、識別名の昇順になる。
	hub.Deliver(ctx, []domain.User{bob, alice}, testMessage(alice, "line1\nline2 <b>"))

	var got map[string]any
	if err := json.Unmarshal(s.sent[0], &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type":   "message",
		"sendId": "11111111-1111-4111-8111-111111111111",
		"message": map[string]any{
			"participants": []any{"alice", "bob"},
			"seq":          float64(7),
			"sender":       map[string]any{"handle": "alice", "displayName": "Alice"},
			"body":         "line1\nline2 <b>",
			"sentAt":       "2026-11-20T18:04:05.123456Z", // UTC にして表す
		},
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("イベントが違う:\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

// 登録を解いた書き込み口に、Send が呼ばれない(WS-41)。
func TestDeliver_DoesNotSendToUnregisteredConnection(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	gone, staying := newRecordingSink(), newRecordingSink()
	hub.Register(ctx, alice.ID, gone)
	hub.Register(ctx, alice.ID, staying)

	hub.Unregister(ctx, gone)
	hub.Unregister(ctx, gone) // 登録されていない接続には、何もしない
	hub.Deliver(ctx, []domain.User{alice, bob}, testMessage(bob, "hello"))

	if len(gone.sent) != 0 {
		t.Fatal("登録を解いた書き込み口に届いた")
	}
	if len(staying.sent) != 1 {
		t.Fatal("登録されている書き込み口に届いていない")
	}
}

// 登録し直した書き込み口は、新しい利用者の端末としてだけ扱う。
func TestRegister_AgainMovesConnectionToNewUser(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	s := newRecordingSink()
	hub.Register(ctx, alice.ID, s)
	hub.Register(ctx, carol.ID, s)

	hub.Deliver(ctx, []domain.User{alice, bob}, testMessage(alice, "hello"))

	if len(s.sent) != 0 {
		t.Fatal("前の利用者の会話のメッセージが届いた")
	}
}

// 列に加えられなかった端末の接続は、閉じる。他の端末への配信は、妨げない(DD-05)。
func TestDeliver_ClosesConnectionThatCannotQueue(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	full, healthy := newRecordingSink(), newRecordingSink()
	full.accept = false
	hub.Register(ctx, bob.ID, full)
	hub.Register(ctx, bob.ID, healthy)

	hub.Deliver(ctx, []domain.User{alice, bob}, testMessage(alice, "hello"))

	if full.closed != 1 {
		t.Fatalf("加えられなかった書き込み口を、閉じていない: %d", full.closed)
	}
	if healthy.closed != 0 || len(healthy.sent) != 1 {
		t.Fatal("他の端末への配信が、妨げられた")
	}
}

func TestDeliver_SkipsWhenSenderIsNotParticipant(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	s := newRecordingSink()
	hub.Register(ctx, alice.ID, s)

	hub.Deliver(ctx, []domain.User{alice, bob}, testMessage(carol, "hello"))

	if len(s.sent) != 0 {
		t.Fatal("送信者が参加者でないメッセージを、配信した")
	}
}

func TestHub_ConcurrentUse(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s := newRecordingSink()
				hub.Register(ctx, alice.ID, s)
				hub.Unregister(ctx, s)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				hub.Deliver(ctx, []domain.User{alice, bob}, testMessage(alice, "hello"))
			}
		}()
	}
	wg.Wait()

	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.byUser) != 0 || len(hub.owners) != 0 {
		t.Fatalf("登録が残っている: %d, %d", len(hub.byUser), len(hub.owners))
	}
}
