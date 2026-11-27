package usecase

import (
	"context"
	"reflect"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// RL-12、RL-14: 照合に成功したログインは、新しいログイン状態を与える。
func TestLogin_GrantsNewSession(t *testing.T) {
	w := newWorld()

	out, err := w.authenticator().Login(context.Background(), LoginInput{Contact: " Alice@Example.COM ", Secret: aliceSecret, Source: source})
	if err != nil {
		t.Fatal(err)
	}

	want := LoginOutput{
		Credential: "credential-1",
		SessionID:  "session-1",
		ExpiresAt:  testNow.Add(domain.SessionLifetime),
		User:       UserView{Handle: "alice", DisplayName: "Alice"},
	}
	if out != want {
		t.Fatalf("出力が違う:\n got %+v\nwant %+v", out, want)
	}
	if got := w.sessions.byCredential["credential-1"]; got.UserID != alice.ID || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("置いたログイン状態が違う: %+v", got)
	}

	// 失敗として数えた1回を、戻す。送信元ごとは、1回を戻し、連絡先ごとは、0に戻す。
	contact := domain.RestoreContact(aliceContact)
	if want := []domain.Limit{w.rules.LoginFailureBySource(source), w.rules.LoginFailureByContact(contact)}; !reflect.DeepEqual(w.limiter.admitted, want) {
		t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
	}
	if want := []domain.Limit{w.rules.LoginFailureBySource(source)}; !reflect.DeepEqual(w.limiter.forgiven, want) {
		t.Fatalf("戻したものが違う: %+v", w.limiter.forgiven)
	}
	if want := []domain.Limit{w.rules.LoginFailureByContact(contact)}; !reflect.DeepEqual(w.limiter.reset, want) {
		t.Fatalf("0に戻したものが違う: %+v", w.limiter.reset)
	}

	if w.hasher.calculations() != 1 || w.hasher.verifies != 1 {
		t.Fatalf("秘密の計算の回数が違う: %d", w.hasher.calculations())
	}
	requireEvents(t, w.events, domain.EventLoginSucceeded)
	if e := w.events.events[0]; e.UserID != alice.ID || e.Source != source {
		t.Fatalf("記録の項目が違う: %+v", e)
	}
	// 要求に、資格が付いていなければ、無効にするログイン状態は、ない。
	if len(w.connections.closedSessions) != 0 {
		t.Fatal("接続を閉じた")
	}
}

// RL-14: ログイン状態を持つブラウザでのログインは、それまでのログイン状態を無効にしてから、
// その接続を閉じ、新しいログイン状態を与える。
func TestLogin_RevokesPreviousSessionOfBrowser(t *testing.T) {
	w := newWorld()
	previous := w.sessions.login(bob, "previous-credential")
	w.log.calls = nil

	out, err := w.authenticator().Login(context.Background(), LoginInput{Credential: previous.Credential, Contact: aliceContact, Secret: aliceSecret, Source: source})
	if err != nil {
		t.Fatal(err)
	}

	if _, remains := w.sessions.byCredential[previous.Credential]; remains {
		t.Fatal("それまでのログイン状態が、残っている")
	}
	if want := []domain.SessionID{domain.SessionID(previous.SessionID)}; !reflect.DeepEqual(w.connections.closedSessions, want) {
		t.Fatalf("閉じた接続のログイン状態が違う: %v", w.connections.closedSessions)
	}
	if want := []string{"sessions.Revoke", "connections.CloseBySession", "sessions.Create"}; !reflect.DeepEqual(w.log.calls, want) {
		t.Fatalf("呼んだ順が違う: %v", w.log.calls)
	}
	if out.Credential == previous.Credential || out.SessionID == previous.SessionID {
		t.Fatalf("新しい資格と識別子を、与えていない: %+v", out)
	}
}

// 要求に付いていた資格に、当たるログイン状態がなければ、接続は閉じない。
func TestLogin_StaleCredentialOfBrowser(t *testing.T) {
	w := newWorld()

	_, err := w.authenticator().Login(context.Background(), LoginInput{Credential: "stale", Contact: aliceContact, Secret: aliceSecret, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.connections.closedSessions) != 0 {
		t.Fatal("接続を閉じた")
	}
}

// RL-13、RL-29: 失敗の経路は、どれも、同じ拒否になり、秘密の計算を、ちょうど1回行う。
// それまでのログイン状態は、変えない。数えた1回は、戻さない。
func TestLogin_Failures(t *testing.T) {
	tests := []struct {
		name            string
		contact, secret string
		countsByContact bool
		wantUser        domain.UserID
	}{
		{"秘密が違う", aliceContact, "wrong passphrase for tests", true, alice.ID},
		{"秘密が、入力の規則を満たさない", aliceContact, "x", true, alice.ID},
		{"秘密が空", aliceContact, "", true, alice.ID},
		{"連絡先が、登録されていない", "nobody@example.com", aliceSecret, true, ""},
		{"連絡先が、入力の規則を満たさない", "not-a-contact", aliceSecret, false, ""},
		{"連絡先が空", "", aliceSecret, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			previous := w.sessions.login(bob, "previous-credential")

			_, err := w.authenticator().Login(context.Background(), LoginInput{Credential: previous.Credential, Contact: tt.contact, Secret: tt.secret, Source: source})

			requireRejection(t, err, LoginFailed)
			if got := w.hasher.calculations(); got != 1 {
				t.Fatalf("秘密の計算の回数が違う: %d", got)
			}
			if _, remains := w.sessions.byCredential[previous.Credential]; !remains || len(w.sessions.byCredential) != 1 {
				t.Fatal("失敗したログインが、ログイン状態を変えた")
			}

			// 規則を満たさない連絡先は、連絡先ごとには数えず、送信元ごとにだけ数える。
			wantCounted := 1
			if tt.countsByContact {
				wantCounted = 2
			}
			if len(w.limiter.admitted) != wantCounted || w.limiter.admitted[0].Kind != domain.LimitLoginFailurePerSource {
				t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
			}
			if len(w.limiter.forgiven) != 0 || len(w.limiter.reset) != 0 {
				t.Fatal("失敗したログインの計数を、戻した")
			}

			// 失敗の記録には、連絡先が登録済みの場合に、その利用者を添える。
			requireEvents(t, w.events, domain.EventLoginFailed)
			if e := w.events.events[0]; e.UserID != tt.wantUser || e.Source != source {
				t.Fatalf("記録の項目が違う: %+v", e)
			}
		})
	}
}

// UC-02 E2、RL-29: 上限に達している間は、照合を行わずに拒否する。秘密が正しくても、
// 連絡先が登録されていなくても、同じである。
func TestLogin_RateLimited(t *testing.T) {
	for _, kind := range []domain.LimitKind{domain.LimitLoginFailurePerSource, domain.LimitLoginFailurePerContact} {
		for _, contact := range []string{aliceContact, "nobody@example.com"} {
			t.Run(kind.String()+"/"+contact, func(t *testing.T) {
				w := newWorld()
				w.limiter.exceeded[kind] = true

				_, err := w.authenticator().Login(context.Background(), LoginInput{Contact: contact, Secret: aliceSecret, Source: source})

				requireRejection(t, err, RateLimited)
				if w.hasher.calculations() != 0 {
					t.Fatal("上限に達している間に、照合を行った")
				}
				if len(w.sessions.byCredential) != 0 {
					t.Fatal("ログイン状態を与えた")
				}
				requireEvents(t, w.events, domain.EventLimitExceeded)
				if e := w.events.events[0]; e.Limit != kind {
					t.Fatalf("記録の項目が違う: %+v", e)
				}
			})
		}
	}
}

// RL-21、RL-D9: ログイン状態を確かめる操作は、有効な場合に、利用者と識別子を返す。
// 有効でない場合は、理由を区別せずに、ログインしていないことを示す。拒否とせず、記録しない。
func TestCurrentSession(t *testing.T) {
	ctx := context.Background()

	t.Run("有効", func(t *testing.T) {
		w := newWorld()
		actor := w.sessions.login(alice, "c1")

		out, err := w.sessionKeeper().Current(ctx, CurrentSessionInput{Credential: actor.Credential, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		want := CurrentSessionOutput{Authenticated: true, SessionID: actor.SessionID, User: UserView{Handle: "alice", DisplayName: "Alice"}}
		if out != want {
			t.Fatalf("出力が違う: %+v", out)
		}
		requireEvents(t, w.events)
	})

	notAuthenticated := []struct {
		name       string
		credential string
		prepare    func(w *world)
	}{
		{"資格がない", "", func(*world) {}},
		{"資格が正しくない", "unknown", func(*world) {}},
		{"期限", "c1", func(w *world) { w.clock.now = testNow.Add(domain.SessionLifetime) }},
		{"ログアウトした", "c1", func(w *world) { delete(w.sessions.byCredential, "c1") }},
	}
	for _, tt := range notAuthenticated {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			w.sessions.login(alice, "c1")
			tt.prepare(w)

			out, err := w.sessionKeeper().Current(ctx, CurrentSessionInput{Credential: tt.credential, Source: source})
			if err != nil {
				t.Fatal(err)
			}
			if out != (CurrentSessionOutput{}) {
				t.Fatalf("出力が違う: %+v", out)
			}
			requireEvents(t, w.events)
		})
	}

	// RL-23: 確かめられなかった場合は、ログインしていないとは示さない。
	t.Run("確かめられない", func(t *testing.T) {
		w := newWorld()
		w.sessions.findErr = errBoom

		_, err := w.sessionKeeper().Current(ctx, CurrentSessionInput{Credential: "c1", Source: source})

		requireFailure(t, err)
		requireEvents(t, w.events, domain.EventOperationFailed)
	})
}

// RL-17: ログアウトは、ログイン状態を無効にしてから、そのログイン状態による接続を閉じる。
func TestLogout(t *testing.T) {
	w := newWorld()
	actor := w.sessions.login(alice, "c1")
	other := w.sessions.login(alice, "c2")
	w.log.calls = nil

	if err := w.sessionKeeper().Logout(context.Background(), LogoutInput{Actor: actor, Source: source}); err != nil {
		t.Fatal(err)
	}

	if _, remains := w.sessions.byCredential["c1"]; remains {
		t.Fatal("ログイン状態が、残っている")
	}
	// 他のブラウザのログイン状態は、残る。
	if _, remains := w.sessions.byCredential[other.Credential]; !remains {
		t.Fatal("別のログイン状態を、無効にした")
	}
	if want := []string{"sessions.Revoke", "connections.CloseBySession"}; !reflect.DeepEqual(w.log.calls, want) {
		t.Fatalf("呼んだ順が違う: %v", w.log.calls)
	}
	if want := []domain.SessionID{domain.SessionID(actor.SessionID)}; !reflect.DeepEqual(w.connections.closedSessions, want) {
		t.Fatalf("閉じた接続のログイン状態が違う: %v", w.connections.closedSessions)
	}
	requireEvents(t, w.events, domain.EventLoggedOut)
	if e := w.events.events[0]; e.UserID != alice.ID || e.Source != source {
		t.Fatalf("記録の項目が違う: %+v", e)
	}
}

// RL-15: ログイン状態が有効でない要求は、どの理由でも、同じ拒否になり、認証の拒否を記録する。
// ログアウトを通して、SessionGate の確認を確かめる。
func TestSessionGate_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(valid Actor, other Actor) Actor
		prepare func(w *world)
	}{
		{"資格も識別子もない", func(Actor, Actor) Actor { return Actor{} }, nil},
		{"資格がない", func(v, _ Actor) Actor { return Actor{SessionID: v.SessionID} }, nil},
		{"識別子がない", func(v, _ Actor) Actor { return Actor{Credential: v.Credential} }, nil},
		{"資格が正しくない", func(v, _ Actor) Actor { return Actor{Credential: "unknown", SessionID: v.SessionID} }, nil},
		{"識別子が、別のログイン状態のもの", func(v, o Actor) Actor { return Actor{Credential: v.Credential, SessionID: o.SessionID} }, nil},
		{"期限", func(v, _ Actor) Actor { return v }, func(w *world) { w.clock.now = testNow.Add(domain.SessionLifetime) }},
		{"ログアウトした", func(v, _ Actor) Actor { return v }, func(w *world) { delete(w.sessions.byCredential, "c1") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			valid := w.sessions.login(alice, "c1")
			other := w.sessions.login(bob, "c2")
			w.log.calls = nil
			if tt.prepare != nil {
				tt.prepare(w)
			}

			err := w.sessionKeeper().Logout(context.Background(), LogoutInput{Actor: tt.actor(valid, other), Source: source})

			requireRejection(t, err, Unauthenticated)
			if len(w.log.calls) != 0 {
				t.Fatalf("拒否した要求で、操作を行った: %v", w.log.calls)
			}
			requireEvents(t, w.events, domain.EventAuthenticationRejected)
			if e := w.events.events[0]; e.UserID != "" || e.Source != source {
				t.Fatalf("記録の項目が違う: %+v", e)
			}
		})
	}
}

// RL-16: ログイン状態は、期限の直前には、使える。
func TestSessionGate_ValidJustBeforeExpiry(t *testing.T) {
	w := newWorld()
	actor := w.sessions.login(alice, "c1")
	w.clock.now = testNow.Add(domain.SessionLifetime - 1)

	if err := w.sessionKeeper().Logout(context.Background(), LogoutInput{Actor: actor, Source: source}); err != nil {
		t.Fatal(err)
	}
}

// RL-23: ログイン状態の確認を完了できなかった操作は、受け付けない。
func TestSessionGate_StoreFailure(t *testing.T) {
	w := newWorld()
	actor := w.sessions.login(alice, "c1")
	w.log.calls = nil
	w.sessions.findErr = errBoom

	err := w.sessionKeeper().Logout(context.Background(), LogoutInput{Actor: actor, Source: source})

	requireFailure(t, err)
	if len(w.log.calls) != 0 {
		t.Fatalf("確かめられなかった要求で、操作を行った: %v", w.log.calls)
	}
	requireEvents(t, w.events, domain.EventOperationFailed)
}

// 片付けは、期限に達した接続を閉じ、期限を過ぎたログイン状態と、申し込みを消す。同じ時刻を渡す。
func TestSweep(t *testing.T) {
	w := newWorld()
	w.clock.now = testNow.Add(42)

	if err := w.housekeeper().Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	if want := []string{"connections.CloseExpired", "sessions.DeleteExpired", "signups.DeleteExpired"}; !reflect.DeepEqual(w.log.calls, want) {
		t.Fatalf("呼んだ順が違う: %v", w.log.calls)
	}
	now := w.clock.now
	if !w.connections.closedExpiredAt[0].Equal(now) || !w.sessions.deletedAt[0].Equal(now) || !w.signups.deletedAt[0].Equal(now) {
		t.Fatal("渡した時刻が違う")
	}
	requireEvents(t, w.events)
}

// 片付けは、どれかがエラーを返した場合、そこで戻り、記録する。送信元は、添えない。
func TestSweep_Failure(t *testing.T) {
	w := newWorld()
	w.sessions.deleteErr = errBoom

	requireFailure(t, w.housekeeper().Sweep(context.Background()))

	if want := []string{"connections.CloseExpired", "sessions.DeleteExpired"}; !reflect.DeepEqual(w.log.calls, want) {
		t.Fatalf("呼んだ順が違う: %v", w.log.calls)
	}
	requireEvents(t, w.events, domain.EventOperationFailed)
	if e := w.events.events[0]; e.Source != "" || e.Operation != "sweep" || e.Error != errBoom.Error() {
		t.Fatalf("記録の項目が違う: %+v", e)
	}
}
