package usecase

import (
	"context"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// failureCase は、公開するメソッドと、失敗させるポートの組である。
// call は、準備を済ませた world に対して、メソッドを1回呼び、エラーを返す。
type failureCase struct {
	name      string
	operation string
	fail      func(w *world)
	call      func(t *testing.T, w *world) error
}

// RL-32: ユースケースの公開するメソッドは、どのポートが失敗しても、完了できなかったことを返し、
// operation_failed を、1回だけ記録する。記録の通り道(attempt)を通さないメソッドを加えると、
// このテストが検出する。公開するメソッドを加えたときは、ここに加える。
func TestOperationFailure_IsRecordedExactlyOnce(t *testing.T) {
	for _, tt := range failureCases() {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			tt.fail(w)

			err := tt.call(t, w)

			requireFailure(t, err)
			if got := w.events.count(domain.EventOperationFailed); got != 1 {
				t.Fatalf("operation_failed の回数が違う: %d(%v)", got, w.events.kinds())
			}
			for _, e := range w.events.events {
				if e.Kind != domain.EventOperationFailed {
					continue
				}
				if e.Operation != tt.operation || e.Error != errBoom.Error() {
					t.Fatalf("記録の項目が違う: %+v", e)
				}
			}
		})
	}
}

func failureCases() []failureCase {
	ctx := context.Background()

	requestSignup := func(_ *testing.T, w *world) error {
		return w.signupRequester().Request(ctx, RequestSignupInput{Contact: newContact, Source: source})
	}
	completeSignup := func(t *testing.T, w *world) error {
		// 申し込みは、失敗させる前の状態で、直接に置く。
		w.signups.byContact[newContact] = fakeSignup{credential: "signup-credential", expiresAt: testNow.Add(domain.SignupLifetime)}
		return w.signupCompleter().Complete(ctx, CompleteSignupInput{Credential: "signup-credential", Handle: "dave", DisplayName: "Dave", Secret: newSecret, Source: source})
	}
	login := func(_ *testing.T, w *world) error {
		w.sessions.byCredential["previous"] = domain.Session{ID: "previous-session", UserID: bob.ID, ExpiresAt: testNow.Add(domain.SessionLifetime)}
		_, err := w.authenticator().Login(ctx, LoginInput{Credential: "previous", Contact: aliceContact, Secret: aliceSecret, Source: source})
		return err
	}
	current := func(_ *testing.T, w *world) error {
		_, err := w.sessionKeeper().Current(ctx, CurrentSessionInput{Credential: "c1", Source: source})
		return err
	}
	logout := func(_ *testing.T, w *world) error {
		w.sessions.byCredential["c1"] = domain.Session{ID: "s1", UserID: alice.ID, ExpiresAt: testNow.Add(domain.SessionLifetime)}
		return w.sessionKeeper().Logout(ctx, LogoutInput{Actor: Actor{Credential: "c1", SessionID: "s1"}, Source: source})
	}
	sweep := func(_ *testing.T, w *world) error {
		return w.housekeeper().Sweep(ctx)
	}
	findPeer := func(_ *testing.T, w *world) error {
		w.loginAll()
		_, err := w.peerFinder().Find(ctx, FindPeerInput{Actor: as("alice"), Source: source, Handle: "bob"})
		return err
	}
	readHistory := func(_ *testing.T, w *world) error {
		w.loginAll()
		// 履歴の取得まで進むように、会話を成立させておく。
		w.store.conversations = append(w.store.conversations, domain.Conversation{ID: "c0000000-0000-4000-8000-000000000001", Pair: mustPair(alice.ID, bob.ID)})
		_, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: "bob"})
		return err
	}
	send := func(_ *testing.T, w *world) error {
		w.loginAll()
		_, err := w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: "hello"})
		return err
	}
	accept := func(_ *testing.T, w *world) error {
		w.loginAll()
		return w.connectionKeeper().Accept(ctx, AcceptConnectionInput{Actor: as("alice"), Source: source, Connection: &fakeSink{}})
	}

	return []failureCase{
		{"登録の申し込み/上限", "request signup", func(w *world) { w.limiter.err = errBoom }, requestSignup},
		{"登録の申し込み/利用者", "request signup", func(w *world) { w.users.err = errBoom }, requestSignup},
		{"登録の申し込み/資格の生成", "request signup", func(w *world) { w.credentials.err = errBoom }, requestSignup},
		{"登録の申し込み/申し込みを置く", "request signup", func(w *world) { w.signups.putErr = errBoom }, requestSignup},
		{"登録の申し込み/案内", "request signup", func(w *world) { w.notices.err = errBoom }, requestSignup},

		{"登録の完了/上限", "complete signup", func(w *world) { w.limiter.err = errBoom }, completeSignup},
		{"登録の完了/申し込みを探す", "complete signup", func(w *world) { w.signups.findErr = errBoom }, completeSignup},
		{"登録の完了/秘密の計算", "complete signup", func(w *world) { w.hasher.err = errBoom }, completeSignup},
		{"登録の完了/使用済みにする", "complete signup", func(w *world) { w.store.consumeErr = errBoom }, completeSignup},
		{"登録の完了/利用者を作る", "complete signup", func(w *world) { w.store.createUserErr = errBoom }, completeSignup},
		{"登録の完了/確定", "complete signup", func(w *world) { w.store.commitErr = errBoom }, completeSignup},

		{"ログイン/上限", "login", func(w *world) { w.limiter.err = errBoom }, login},
		{"ログイン/利用者", "login", func(w *world) { w.users.err = errBoom }, login},
		{"ログイン/照合", "login", func(w *world) { w.hasher.err = errBoom }, login},
		{"ログイン/無効にする", "login", func(w *world) { w.sessions.revokeErr = errBoom }, login},
		{"ログイン/資格の生成", "login", func(w *world) { w.credentials.err = errBoom }, login},
		{"ログイン/ログイン状態を作る", "login", func(w *world) { w.sessions.createErr = errBoom }, login},

		{"ログイン状態を確かめる/ログイン状態", "current session", func(w *world) { w.sessions.findErr = errBoom }, current},

		{"ログアウト/確認", "logout", func(w *world) { w.sessions.findErr = errBoom }, logout},
		{"ログアウト/無効にする", "logout", func(w *world) { w.sessions.revokeErr = errBoom }, logout},

		{"相手の特定/確認", "find peer", func(w *world) { w.sessions.findErr = errBoom }, findPeer},
		{"相手の特定/利用者", "find peer", func(w *world) { w.users.err = errBoom }, findPeer},
		{"相手の特定/会話", "find peer", func(w *world) { w.store.findDirectErr = errBoom }, findPeer},
		{"相手の特定/上限", "find peer", func(w *world) { w.limiter.err = errBoom }, findPeer},

		{"履歴/確認", "read history", func(w *world) { w.sessions.findErr = errBoom }, readHistory},
		{"履歴/利用者", "read history", func(w *world) { w.users.err = errBoom }, readHistory},
		{"履歴/会話", "read history", func(w *world) { w.store.findDirectErr = errBoom }, readHistory},
		{"履歴/メッセージ", "read history", func(w *world) { w.store.listErr = errBoom }, readHistory},

		{"送信/確認", "send message", func(w *world) { w.sessions.findErr = errBoom }, send},
		{"送信/利用者", "send message", func(w *world) { w.users.err = errBoom }, send},
		{"送信/上限", "send message", func(w *world) { w.limiter.err = errBoom }, send},
		{"送信/追記", "send message", func(w *world) { w.store.appendErr = errBoom }, send},
		{"送信/確定", "send message", func(w *world) { w.store.commitErr = errBoom }, send},

		{"接続の受け付け/確認", "accept connection", func(w *world) { w.sessions.findErr = errBoom }, accept},
		{"接続の受け付け/登録の後の確認", "accept connection", func(w *world) { w.sessions.failFindFrom = 2 }, accept},

		{"片付け/ログイン状態", "sweep", func(w *world) { w.sessions.deleteErr = errBoom }, sweep},
		{"片付け/申し込み", "sweep", func(w *world) { w.signups.deleteErr = errBoom }, sweep},
	}
}

// ログインで、照合する相手がない場合の計算(VerifyDummy)の失敗も、同じく記録する。
func TestLogin_DummyVerificationFailure(t *testing.T) {
	w := newWorld()
	w.hasher.err = errBoom

	_, err := w.authenticator().Login(context.Background(), LoginInput{Contact: "nobody@example.com", Secret: aliceSecret, Source: source})

	requireFailure(t, err)
	requireEvents(t, w.events, domain.EventOperationFailed)
}

func mustPair(a, b domain.UserID) domain.DirectPair {
	pair, err := domain.NewDirectPair(a, b)
	if err != nil {
		panic(err)
	}
	return pair
}

// RL-32: エラーの記録には、特定できていれば、利用者を添える。
func TestOperationFailure_RecordsIdentifiedUser(t *testing.T) {
	w := newWorld().loginAll()
	w.store.appendErr = errBoom

	_, err := w.messageSender().Send(context.Background(), SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: "hello"})

	requireFailure(t, err)
	requireEvents(t, w.events, domain.EventOperationFailed)
	if e := w.events.events[0]; e.UserID != alice.ID || e.Source != source || e.Operation != "send message" {
		t.Fatalf("記録の項目が違う: %+v", e)
	}
}
