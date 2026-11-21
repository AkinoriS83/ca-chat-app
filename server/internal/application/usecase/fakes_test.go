package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// テスト用の利用者。識別子の順は、識別名の順と、わざと変えてある(alice が最も大きい)。
var (
	alice = domain.User{ID: "00000000-0000-4000-8000-000000000003", Handle: "alice", DisplayName: "Alice"}
	bob   = domain.User{ID: "00000000-0000-4000-8000-000000000001", Handle: "bob", DisplayName: "Bob"}
	carol = domain.User{ID: "00000000-0000-4000-8000-000000000002", Handle: "carol", DisplayName: "Carol"}
)

const (
	sendID1 = "11111111-1111-4111-8111-111111111111"
	sendID2 = "22222222-2222-4222-8222-222222222222"
)

var errBoom = errors.New("boom")

// fakeUsers は、port.UserDirectory の、テスト用の実装である。
type fakeUsers struct {
	users []domain.User
	err   error
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{users: []domain.User{alice, bob, carol}}
}

func (f *fakeUsers) FindByHandle(_ context.Context, handle string) (domain.User, bool, error) {
	if f.err != nil {
		return domain.User{}, false, f.err
	}
	for _, u := range f.users {
		if u.Handle == handle {
			return u, true, nil
		}
	}
	return domain.User{}, false, nil
}

// fakeStore は、port.ConversationReader、port.Transactor、port.ConversationWriter の、
// テスト用の実装である。メモリの上に保持する。取り消しは、再現しない(ADR-012)。
type fakeStore struct {
	conversations []domain.Conversation
	messages      []domain.Message
	now           time.Time

	appendErr error // AppendMessage が返すエラー
	commitErr error // fn が成功した後に、InTransaction が返すエラー(確定の失敗)
	findErr   error // FindMessageBySend が返すエラー

	inTx  bool
	calls []string // トランザクションの中で呼ばれた操作
}

func newFakeStore() *fakeStore {
	return &fakeStore{now: time.Date(2026, 11, 21, 3, 4, 5, 123456000, time.UTC)}
}

func (f *fakeStore) FindDirect(_ context.Context, pair domain.DirectPair) (domain.Conversation, bool, error) {
	for _, c := range f.conversations {
		if c.Pair == pair {
			return c, true, nil
		}
	}
	return domain.Conversation{}, false, nil
}

func (f *fakeStore) ListMessages(_ context.Context, id domain.ConversationID) ([]domain.Message, error) {
	var out []domain.Message
	for _, m := range f.messages {
		if m.ConversationID == id {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeStore) FindMessageBySend(_ context.Context, sender domain.UserID, sendID domain.SendID) (domain.Message, bool, error) {
	if f.findErr != nil {
		return domain.Message{}, false, f.findErr
	}
	for _, m := range f.messages {
		if m.SenderID == sender && m.SendID == sendID {
			return m, true, nil
		}
	}
	return domain.Message{}, false, nil
}

func (f *fakeStore) InTransaction(ctx context.Context, fn func(ctx context.Context, w port.ConversationWriter) error) error {
	f.inTx = true
	err := fn(ctx, f)
	f.inTx = false
	if err != nil {
		return err
	}
	return f.commitErr
}

func (f *fakeStore) EstablishDirect(ctx context.Context, pair domain.DirectPair) (domain.Conversation, error) {
	f.calls = append(f.calls, "EstablishDirect")
	if conv, found, _ := f.FindDirect(ctx, pair); found {
		return conv, nil
	}
	id := domain.ConversationID("c0000000-0000-4000-8000-00000000000" + string(rune('1'+len(f.conversations))))
	conv := domain.Conversation{ID: id, Pair: pair}
	f.conversations = append(f.conversations, conv)
	return conv, nil
}

func (f *fakeStore) SerializeAppends(_ context.Context, _ domain.ConversationID) error {
	f.calls = append(f.calls, "SerializeAppends")
	return nil
}

func (f *fakeStore) AppendMessage(_ context.Context, id domain.ConversationID, sender domain.UserID, sendID domain.SendID, body domain.Body) (domain.Message, error) {
	f.calls = append(f.calls, "AppendMessage")
	if f.appendErr != nil {
		return domain.Message{}, f.appendErr
	}
	seq := int64(1)
	for _, m := range f.messages {
		if m.SenderID == sender && m.SendID == sendID {
			return domain.Message{}, port.ErrDuplicateSend
		}
		if m.ConversationID == id {
			seq++
		}
	}
	msg := domain.Message{ConversationID: id, Seq: seq, SenderID: sender, SendID: sendID, Body: body, SentAt: f.now}
	f.messages = append(f.messages, msg)
	return msg, nil
}

// fakeDelivery は、port.MessageDelivery の、テスト用の実装である。
type fakeDelivery struct {
	store      *fakeStore
	deliveries []delivered
}

type delivered struct {
	participants []domain.User
	msg          domain.Message
	duringTx     bool
}

func (f *fakeDelivery) Deliver(_ context.Context, participants []domain.User, msg domain.Message) {
	f.deliveries = append(f.deliveries, delivered{participants: participants, msg: msg, duringTx: f.store.inTx})
}

// fakeRegistry は、port.ConnectionRegistry の、テスト用の実装である。
type fakeRegistry struct {
	registered   map[port.ConnectionSink]domain.UserID
	unregistered []port.ConnectionSink
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{registered: map[port.ConnectionSink]domain.UserID{}}
}

func (f *fakeRegistry) Register(_ context.Context, user domain.UserID, sink port.ConnectionSink) {
	f.registered[sink] = user
}

func (f *fakeRegistry) Unregister(_ context.Context, sink port.ConnectionSink) {
	delete(f.registered, sink)
	f.unregistered = append(f.unregistered, sink)
}

// fakeSink は、port.ConnectionSink の、テスト用の実装である。
type fakeSink struct{}

func (*fakeSink) Send([]byte) bool { return true }
func (*fakeSink) Close()           {}

// requireRejection は、err が、指定した種類の拒否であることを確かめる。
func requireRejection(t *testing.T, err error, want Rejection) {
	t.Helper()
	var rejectedErr *RejectedError
	if !errors.As(err, &rejectedErr) {
		t.Fatalf("拒否ではない: %v", err)
	}
	if rejectedErr.Kind != want {
		t.Fatalf("拒否の種類が違う: got %v, want %v", rejectedErr.Kind, want)
	}
}

// requireFailure は、err が、拒否ではないエラー(完了できなかったこと)であることを確かめる。
func requireFailure(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("エラーを返さなかった")
	}
	var rejectedErr *RejectedError
	if errors.As(err, &rejectedErr) {
		t.Fatalf("拒否として返した: %v", err)
	}
}
