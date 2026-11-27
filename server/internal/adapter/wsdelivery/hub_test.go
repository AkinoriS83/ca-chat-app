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

func user(id, handle, displayName string) domain.User {
	return domain.User{ID: domain.UserID(id), Handle: domain.RestoreHandle(handle), DisplayName: domain.RestoreDisplayName(displayName)}
}

var (
	alice = user("00000000-0000-4000-8000-000000000003", "alice", "Alice")
	bob   = user("00000000-0000-4000-8000-000000000001", "bob", "Bob")
	carol = user("00000000-0000-4000-8000-000000000002", "carol", "Carol")

	testNow = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
)

// sessionOf は、利用者ごとに1つの、期限の前のログイン状態を返す。
func sessionOf(u domain.User) domain.Session {
	return domain.Session{ID: domain.SessionID("session-of-" + u.Handle.String()), UserID: u.ID, ExpiresAt: testNow.Add(time.Hour)}
}

// recordingSink は、port.ConnectionSink の、テスト用の実装である。
type recordingSink struct {
	mu     sync.Mutex
	accept bool
	sent   [][]byte
	closed int
	ended  int // CloseSessionEnded が呼ばれた回数
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

func (s *recordingSink) CloseSessionEnded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended++
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
	hub.Register(ctx, sessionOf(alice), alice1)
	hub.Register(ctx, sessionOf(alice), alice2)
	hub.Register(ctx, sessionOf(bob), bob1)
	hub.Register(ctx, sessionOf(carol), carol1)

	hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(alice, "hello"))

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
	hub.Register(ctx, sessionOf(bob), s)

	// 参加者の順は、識別子の順(bob、alice)で渡す。participants は、識別名の昇順になる。
	hub.Deliver(ctx, testNow, []domain.User{bob, alice}, testMessage(alice, "line1\nline2 <b>"))

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
	hub.Register(ctx, sessionOf(alice), gone)
	hub.Register(ctx, sessionOf(alice), staying)

	hub.Unregister(ctx, gone)
	hub.Unregister(ctx, gone) // 登録されていない接続には、何もしない
	hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(bob, "hello"))

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
	hub.Register(ctx, sessionOf(alice), s)
	hub.Register(ctx, sessionOf(carol), s)

	hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(alice, "hello"))

	if len(s.sent) != 0 {
		t.Fatal("前の利用者の会話のメッセージが届いた")
	}
}

// 列に加えられなかった端末の接続は、閉じる。他の端末への配信は、妨げない。
func TestDeliver_ClosesConnectionThatCannotQueue(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	full, healthy := newRecordingSink(), newRecordingSink()
	full.accept = false
	hub.Register(ctx, sessionOf(bob), full)
	hub.Register(ctx, sessionOf(bob), healthy)

	hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(alice, "hello"))

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
	hub.Register(ctx, sessionOf(alice), s)

	hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(carol, "hello"))

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
				hub.Register(ctx, sessionOf(alice), s)
				hub.Unregister(ctx, s)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(alice, "hello"))
			}
		}()
	}
	wg.Wait()

	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.byUser) != 0 || len(hub.sessions) != 0 || len(hub.bySession) != 0 {
		t.Fatalf("登録が残っている: %d, %d, %d", len(hub.byUser), len(hub.sessions), len(hub.bySession))
	}
}

// RL-18、RL-19: CloseBySession は、そのログイン状態による接続のすべてに、ログイン状態が無効になったことを
// 伝えて閉じ、配信の宛先から外す。同じ利用者の、別のログイン状態による接続は、残る。
func TestCloseBySession(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	first := sessionOf(alice)
	second := domain.Session{ID: "another-session-of-alice", UserID: alice.ID, ExpiresAt: testNow.Add(time.Hour)}
	tab1, tab2, otherBrowser, bob1 := newRecordingSink(), newRecordingSink(), newRecordingSink(), newRecordingSink()
	hub.Register(ctx, first, tab1)
	hub.Register(ctx, first, tab2)
	hub.Register(ctx, second, otherBrowser)
	hub.Register(ctx, sessionOf(bob), bob1)

	hub.CloseBySession(ctx, first.ID)
	hub.CloseBySession(ctx, first.ID) // 何度呼んでもよい
	hub.CloseBySession(ctx, "unknown-session")

	for _, s := range []*recordingSink{tab1, tab2} {
		if s.ended != 1 || s.closed != 0 {
			t.Fatalf("ログイン状態が無効になったことを伝えて、閉じていない: ended = %d, closed = %d", s.ended, s.closed)
		}
	}
	if otherBrowser.ended != 0 || bob1.ended != 0 {
		t.Fatal("別のログイン状態による接続を、閉じた")
	}

	hub.Deliver(ctx, testNow, []domain.User{alice, bob}, testMessage(bob, "after logout"))
	if len(tab1.sent) != 0 || len(tab2.sent) != 0 {
		t.Fatal("閉じた接続に、届けた")
	}
	if len(otherBrowser.sent) != 1 || len(bob1.sent) != 1 {
		t.Fatal("残った接続に、届いていない")
	}
	if len(hub.sessions) != 2 || len(hub.bySession) != 2 || len(hub.byUser) != 2 {
		t.Fatalf("登録が、解かれていない: %d, %d, %d", len(hub.sessions), len(hub.bySession), len(hub.byUser))
	}
}

// RL-18、RL-19: CloseExpired は、期限の直前には閉じず、期限には閉じる(domain.Session.ValidAt と同じ判定)。
func TestCloseExpired(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	expiresAt := testNow.Add(time.Hour)
	expiring := newRecordingSink()
	later := newRecordingSink()
	hub.Register(ctx, domain.Session{ID: "expiring", UserID: alice.ID, ExpiresAt: expiresAt}, expiring)
	hub.Register(ctx, domain.Session{ID: "later", UserID: alice.ID, ExpiresAt: expiresAt.Add(time.Microsecond)}, later)

	hub.CloseExpired(ctx, expiresAt.Add(-time.Microsecond))
	if expiring.ended != 0 || later.ended != 0 {
		t.Fatal("期限の直前に、閉じた")
	}

	hub.CloseExpired(ctx, expiresAt)
	if expiring.ended != 1 || later.ended != 0 {
		t.Fatalf("期限を過ぎた接続だけを、閉じていない: %d, %d", expiring.ended, later.ended)
	}
	if len(hub.sessions) != 1 {
		t.Fatalf("登録が、解かれていない: %d", len(hub.sessions))
	}
}

// RL-18: Deliver は、配信の時点で、期限を過ぎたログイン状態による接続に、届けずに、閉じる。
// 期限の直前には、届ける。
func TestDeliver_DoesNotSendToExpiredSession(t *testing.T) {
	hub := newTestHub()
	ctx := context.Background()
	expiresAt := testNow.Add(time.Hour)
	expiring := newRecordingSink()
	valid := newRecordingSink()
	hub.Register(ctx, domain.Session{ID: "expiring", UserID: bob.ID, ExpiresAt: expiresAt}, expiring)
	hub.Register(ctx, domain.Session{ID: "valid", UserID: bob.ID, ExpiresAt: expiresAt.Add(time.Hour)}, valid)

	hub.Deliver(ctx, expiresAt.Add(-time.Microsecond), []domain.User{alice, bob}, testMessage(alice, "just before"))
	if len(expiring.sent) != 1 || expiring.ended != 0 {
		t.Fatal("期限の直前の接続に、届いていない")
	}

	hub.Deliver(ctx, expiresAt, []domain.User{alice, bob}, testMessage(alice, "at expiry"))
	if len(expiring.sent) != 1 || expiring.ended != 1 {
		t.Fatalf("期限を過ぎた接続に、届けたか、閉じていない: sent = %d, ended = %d", len(expiring.sent), expiring.ended)
	}
	if len(valid.sent) != 2 || valid.ended != 0 {
		t.Fatal("期限の前の接続に、届いていない")
	}
	if len(hub.sessions) != 1 {
		t.Fatalf("登録が、解かれていない: %d", len(hub.sessions))
	}
}
