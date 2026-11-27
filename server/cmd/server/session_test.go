package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

const sessionLifetime = 30 * 24 * time.Hour

// requireAuthenticatedAs は、b の Cookie で、ログイン中の利用者が、handle であることを確かめる。
func (s *testServer) requireAuthenticatedAs(b *browser, handle string) {
	s.t.Helper()
	resp := s.getSession(b.Cookie)
	want := `"handle":"` + handle + `"`
	if resp.Status != http.StatusOK || !strings.Contains(string(resp.Body), `"authenticated":true`) || !strings.Contains(string(resp.Body), want) {
		s.t.Fatalf("%s として、ログインしていない: %d %s", handle, resp.Status, resp.Body)
	}
}

// requireNotAuthenticated は、cookie で、ログインしていないことが示されることを確かめる。
func (s *testServer) requireNotAuthenticated(cookie string) {
	s.t.Helper()
	resp := s.getSession(cookie)
	if resp.Status != http.StatusOK || string(resp.Body) != `{"authenticated":false}` {
		s.t.Fatalf("ログインしていないことが、示されない: %d %s", resp.Status, resp.Body)
	}
}

// RL-12: 登録した連絡先と秘密でログインすると、以後、その利用者として操作できる。
// 連絡先は、大文字と小文字の違いと、前後の空白に依らない。
func TestLogin_GrantsSession(t *testing.T) {
	s := startServer(t)
	a := fixedAccounts["alice"]

	for _, contact := range []string{a.Contact, "ALICE@Example.COM", "  alice@example.com\n"} {
		resp := s.loginResponse(contact, a.Secret, "")
		if resp.Status != http.StatusOK {
			t.Fatalf("%q: ログインが完了しなかった: %d %s", contact, resp.Status, resp.Body)
		}
		// 応答の本文は、識別子と利用者だけを持つ。資格は、Cookie にだけ載る。
		var session struct {
			SessionID string `json:"sessionId"`
			User      struct {
				Handle      string `json:"handle"`
				DisplayName string `json:"displayName"`
			} `json:"user"`
		}
		decodeStrict(t, resp.Body, &session)
		cookie := sessionCookieOf(resp)
		if cookie == nil || session.SessionID == "" || session.User.Handle != "alice" || session.User.DisplayName != "Alice" {
			t.Fatalf("ログインの応答が違う: %s", resp.Body)
		}
		if strings.Contains(string(resp.Body), cookie.Value) {
			t.Fatal("応答の本文が、資格を含む")
		}
		// Cookie の属性(ADR-014)。有効期限は、ログイン状態の有効期限である。
		if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.Secure {
			t.Fatalf("Cookie の属性が違う: %+v", cookie)
		}
		if !cookie.Expires.Equal(testStart.Add(sessionLifetime)) {
			t.Fatalf("Cookie の有効期限が違う: %v", cookie.Expires)
		}
		if len(cookie.Value) != 43 {
			t.Fatalf("資格の長さが違う: %d", len(cookie.Value))
		}

		b := &browser{Cookie: cookie.Value, SessionID: session.SessionID}
		msg := s.sendFrom(b, "bob", "hello from alice")
		if msg.Status != http.StatusOK || !strings.Contains(string(msg.Body), `"sender":{"displayName":"Alice","handle":"alice"}`) {
			t.Fatalf("alice として、操作できない: %d %s", msg.Status, msg.Body)
		}
	}
}

// RL-12: 秘密は、全体が照合に働く。上限の長さの秘密で登録した利用者は、末尾の1文字だけが異なる秘密では、
// ログインできない。秘密は、入力された通りに扱う。
func TestLogin_WholeSecretMatters(t *testing.T) {
	s := startServer(t)
	long := strings.Repeat("\U0001F600", 127) + "a"
	a := account{Contact: "long@example.com", Handle: "long_secret", DisplayName: "Long", Secret: long}
	s.mustRegister(a)

	s.mustLogin(a.Contact, long)
	for name, secret := range map[string]string{
		"末尾の1文字が異なる":  strings.Repeat("\U0001F600", 127) + "b",
		"末尾の1文字がない":   strings.Repeat("\U0001F600", 127),
		"末尾に空白がある":    long + " ",
		"大文字と小文字が異なる": strings.Repeat("\U0001F600", 127) + "A",
		"先頭の72バイトだけ":  long[:72],
	} {
		requireError(t, s.loginResponse(a.Contact, secret, ""), http.StatusUnauthorized, "login_failed")
		_ = name
	}
}

// RL-13: ログインの失敗の応答は、連絡先が登録されていない場合、登録を完了していない申し込みである場合、
// 秘密が違う場合、入力の規則を満たさない場合で、内容が同じである。Cookie は、変えない。
// 秘密の計算の回数は、usecase のテストで確かめる。
func TestLogin_FailuresAreIndistinguishable(t *testing.T) {
	s := startServer(t)
	a := fixedAccounts["alice"]
	s.mustRequestSignup(dave.Contact, dave.Contact)

	wrongSecret := s.loginResponse(a.Contact, "wrong passphrase for tests", "")
	requireError(t, wrongSecret, http.StatusUnauthorized, "login_failed")
	requireNoInternals(t, wrongSecret)

	others := map[string]response{
		"連絡先が、登録されていない":    s.loginResponse("nobody@example.com", a.Secret, ""),
		"登録を完了していない申し込み":   s.loginResponse(dave.Contact, dave.Secret, ""),
		"連絡先が、入力の規則を満たさない": s.loginResponse("not-a-contact", a.Secret, ""),
		"連絡先が空": s.loginResponse("", a.Secret, ""),
		"秘密が、入力の規則を満たさない(短い)": s.loginResponse(a.Contact, "x", ""),
		"秘密が空": s.loginResponse(a.Contact, "", ""),
		"秘密が、入力の規則を満たさない(長い)": s.loginResponse(a.Contact, strings.Repeat("a", 129), ""),
		"他の利用者の秘密":            s.loginResponse(a.Contact, fixedAccounts["bob"].Secret, ""),
		"秘密が、文字列として正しくない":     s.loginResponse(a.Contact, "\xff\xfe", ""),
	}
	for name, resp := range others {
		if resp.Status != wrongSecret.Status || string(resp.Body) != string(wrongSecret.Body) {
			t.Fatalf("%s: 応答が一致しない: %d %s", name, resp.Status, resp.Body)
		}
		if len(resp.Header.Values("Set-Cookie")) != 0 {
			t.Fatalf("%s: 失敗したログインが、Cookie を変えた", name)
		}
	}
	if got := s.count("sessions"); got != 0 {
		t.Fatalf("失敗したログインが、ログイン状態を作った: %d", got)
	}
}

// RL-14: 別のブラウザでログインしても、元のブラウザのログイン状態は、有効なまま残る。
// ログインのたびに、新しい資格が与えられる。
func TestLogin_AnotherBrowserKeepsFirstSession(t *testing.T) {
	s := startServer(t)

	first := s.newBrowser("alice")
	second := s.newBrowser("alice")

	if first.Cookie == second.Cookie || first.SessionID == second.SessionID {
		t.Fatal("ログインのたびに、新しい資格と識別子が、与えられていない")
	}
	s.requireAuthenticatedAs(first, "alice")
	s.requireAuthenticatedAs(second, "alice")
	for _, b := range []*browser{first, second} {
		if resp := s.historyFrom(b, "bob"); resp.Status != http.StatusOK {
			t.Fatalf("ログイン状態が、使えない: %d", resp.Status)
		}
	}
}

// RL-14: ログインの前にブラウザが持っていた資格は、ログインの後、使えない。
// 照合に失敗したログインは、それまでのログイン状態を変えない。
func TestLogin_InBrowserWithSessionRevokesPrevious(t *testing.T) {
	s := startServer(t)
	a := fixedAccounts["alice"]
	before := s.newBrowser("alice")

	requireError(t, s.loginResponse(a.Contact, "wrong passphrase for tests", before.Cookie), http.StatusUnauthorized, "login_failed")
	s.requireAuthenticatedAs(before, "alice")

	after := s.mustLoginIn(a.Contact, a.Secret, before.Cookie)

	if after.Cookie == before.Cookie {
		t.Fatal("新しい資格が、与えられていない")
	}
	s.requireNotAuthenticated(before.Cookie)
	requireError(t, s.historyFrom(before, "bob"), http.StatusUnauthorized, "unauthenticated")
	s.requireAuthenticatedAs(after, "alice")
	if got := s.count("sessions"); got != 1 {
		t.Fatalf("ログイン状態の行の数が違う: %d", got)
	}
}

// invalidSessions は、ログイン状態が有効でない要求の、種類である(RL-15)。
// それぞれ、サーバーと、有効なログイン状態を受けて、有効でないブラウザを返す。
//
// cookieValid は、資格そのものは、有効であるかを示す(識別子だけが、合わない)。ログイン状態を確かめる操作は、
// Cookie だけを見るため、この場合には、ログイン中の利用者を示す(ADR-014)。
var invalidSessions = []struct {
	name        string
	cookieValid bool
	make        func(s *testServer, valid *browser) *browser
}{
	{"資格も識別子もない", false, func(*testServer, *browser) *browser { return &browser{} }},
	{"資格がない", false, func(_ *testServer, v *browser) *browser { return &browser{SessionID: v.SessionID} }},
	{"識別子がない", true, func(_ *testServer, v *browser) *browser { return &browser{Cookie: v.Cookie} }},
	{"資格が正しくない", false, func(_ *testServer, v *browser) *browser {
		return &browser{Cookie: strings.Repeat("A", 43), SessionID: v.SessionID}
	}},
	{"識別子が、別のログイン状態のもの", true, func(s *testServer, v *browser) *browser {
		return &browser{Cookie: v.Cookie, SessionID: s.as("bob").SessionID}
	}},
	{"有効期限が切れた", false, func(s *testServer, v *browser) *browser {
		s.clock.advance(sessionLifetime)
		return v
	}},
	{"ログアウトした", false, func(s *testServer, v *browser) *browser {
		requireNoContent(s.t, s.logout(v))
		return v
	}},
}

// RL-15: ログイン状態が有効でない要求は、登録とログインを除く、すべての操作と、リアルタイムの接続で、
// 同じ応答で拒否される。形式に合わない要求でも、同じである。サーバーへ直接送った要求と接続で、確かめる。
// ログイン状態を確かめる操作は、どの要求にも、同じ応答で、ログインしていないことを示し、認証の拒否として記録しない。
func TestInvalidSession_IsRejectedByEveryOperation(t *testing.T) {
	for _, kind := range invalidSessions {
		t.Run(kind.name, func(t *testing.T) {
			s := startServer(t)
			valid := s.newBrowser("alice")
			// alice と bob の会話は、成立している。読めてはならない。
			if resp := s.sendFrom(valid, "bob", "secret"); resp.Status != http.StatusOK {
				t.Fatal("準備の送信が、完了しなかった")
			}
			b := kind.make(s, valid)
			before := len(s.eventsOf("authentication_rejected"))

			// ログイン状態を確かめる操作は、拒否としない。記録しない。
			if kind.cookieValid {
				s.requireAuthenticatedAs(b, "alice")
			} else {
				s.requireNotAuthenticated(b.Cookie)
			}
			if got := len(s.eventsOf("authentication_rejected")); got != before {
				t.Fatal("ログイン状態を確かめる操作が、認証の拒否として記録された")
			}

			responses := map[string]response{
				"ログアウト": s.logout(b),
				"相手の特定": s.findUser(b, "bob"),
				"相手の特定(規則を満たさない)": s.findUser(b, "!"),
				"履歴":            s.historyFrom(b, "bob"),
				"履歴(自分自身)":      s.historyFrom(b, "alice"),
				"送信":            s.sendFrom(b, "bob", "hello"),
				"送信(形式に合わない)":   s.do(http.MethodPost, messagesPath("bob"), b.header(), []byte(`{"sendId":`)),
				"送信(本文が正しくない)":  s.do(http.MethodPost, messagesPath("bob"), b.header(), []byte(`{"sendId":"x","body":""}`)),
				"接続先への、通常の GET": s.do(http.MethodGet, strings.TrimPrefix(s.realtimeURLOf(b), "ws"+strings.TrimPrefix(s.url, "http")), b.header(), nil),
			}
			for name, resp := range responses {
				if resp.Status != http.StatusUnauthorized || string(resp.Body) != `{"code":"unauthenticated"}` {
					t.Fatalf("%s: 応答が違う: %d %s", name, resp.Status, resp.Body)
				}
				requireNoInternals(t, resp)
				if len(resp.Header.Values("Set-Cookie")) != 0 {
					t.Fatalf("%s: 拒否の応答が、Cookie を変えた", name)
				}
			}
			s.requireRejectedDial(b, http.StatusUnauthorized, "unauthenticated")

			// 拒否された操作は、行われていない。認証の拒否は、記録されている。
			s.requireStored(1, 1)
			rejected := s.eventsOf("authentication_rejected")
			if got := len(rejected) - before; got != len(responses)+1 {
				t.Fatalf("認証の拒否の記録の数が違う: %d", got)
			}
			if last := rejected[len(rejected)-1]; last["source"] != "127.0.0.1" || last["user_id"] != nil {
				t.Fatalf("認証の拒否の記録の項目が違う: %v", last)
			}
		})
	}
}

// RL-16: ログイン状態は、有効期限の直前には使え、過ぎると使えない。操作を続けていても、延びない。
func TestSession_ExpiresWithoutExtension(t *testing.T) {
	s := startServer(t)
	alice := s.newBrowser("alice")

	// 操作を続けながら、期限の直前まで、時刻を進める。
	for i := 0; i < 29; i++ {
		s.clock.advance(24 * time.Hour)
		if resp := s.historyFrom(alice, "bob"); resp.Status != http.StatusOK {
			t.Fatalf("%d 日目に、操作できない: %d", i+1, resp.Status)
		}
	}
	s.clock.advance(24*time.Hour - time.Microsecond)
	if resp := s.historyFrom(alice, "bob"); resp.Status != http.StatusOK {
		t.Fatalf("期限の直前に、操作できない: %d", resp.Status)
	}
	s.requireAuthenticatedAs(alice, "alice")
	conn := s.connectAs(alice)

	s.clock.advance(time.Microsecond)
	requireError(t, s.historyFrom(alice, "bob"), http.StatusUnauthorized, "unauthenticated")
	s.requireNotAuthenticated(alice.Cookie)
	s.requireRejectedDial(alice, http.StatusUnauthorized, "unauthenticated")

	// 期限を過ぎたログイン状態の行は、片付けが消す。期限に達した接続も、閉じる。
	s.sweep()
	conn.requireSessionEnded()
	if got := s.count("sessions"); got != 0 {
		t.Fatalf("期限を過ぎたログイン状態が、残っている: %d", got)
	}
}

// RL-17: ログアウトすると、その資格では操作できない。同じブラウザの、他のタブでも、操作できない。
// 他のブラウザでのログイン状態は、有効なまま残る。応答は、Cookie を消す。
func TestLogout_RevokesOnlyThatSession(t *testing.T) {
	s := startServer(t)
	browser1 := s.newBrowser("alice")
	browser2 := s.newBrowser("alice")

	resp := s.logout(browser1)

	requireNoContent(t, resp)
	cookie := sessionCookieOf(resp)
	if cookie == nil || cookie.Value != "" || cookie.MaxAge >= 0 || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("ログアウトの応答が、Cookie を消していない: %+v", cookie)
	}

	// 同じブラウザのタブは、同じ資格と識別子を持つ。
	requireError(t, s.historyFrom(browser1, "bob"), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.sendFrom(browser1, "bob", "hello"), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.logout(browser1), http.StatusUnauthorized, "unauthenticated")
	s.requireRejectedDial(browser1, http.StatusUnauthorized, "unauthenticated")
	s.requireNotAuthenticated(browser1.Cookie)

	s.requireAuthenticatedAs(browser2, "alice")
	if resp := s.sendFrom(browser2, "bob", "still here"); resp.Status != http.StatusOK {
		t.Fatalf("他のブラウザのログイン状態が、使えない: %d", resp.Status)
	}
	if got := s.count("sessions"); got != 1 {
		t.Fatalf("ログイン状態の行の数が違う: %d", got)
	}
}

// RL-18、RL-19: ログアウトすると、そのログイン状態による接続(同じブラウザの、すべてのタブ)は、
// ログイン状態が無効になったことを示して、閉じられる。その後に保管されたメッセージは、届かない。
// 他のブラウザの接続には、届く。
func TestLogout_ClosesConnectionsAndStopsDelivery(t *testing.T) {
	s := startServer(t)
	browser1 := s.newBrowser("alice")
	browser2 := s.newBrowser("alice")
	tab1 := s.connectAs(browser1)
	tab2 := s.connectAs(browser1)
	other := s.connectAs(browser2)

	requireNoContent(t, s.logout(browser1))
	after := s.mustSend("bob", "alice", "after logout")

	// 閉じられる前に、メッセージが届いていれば、失敗する。
	tab1.requireSessionEnded()
	tab2.requireSessionEnded()
	other.requireNext(after)
}

// RL-18: 有効期限が切れた後に保管されたメッセージは、片付けの前でも、そのログイン状態による端末に、届かない。
// 期限の直前に保管されたメッセージは、届く。
func TestExpiry_StopsDeliveryBeforeSweep(t *testing.T) {
	s := startServer(t)
	bobBrowser := s.newBrowser("bob")
	bobConn := s.connectAs(bobBrowser)
	// alice は、bob の1時間後に、ログインする。bob の期限の時点で、alice のログイン状態は、有効である。
	s.clock.advance(time.Hour)
	alice := s.newBrowser("alice")
	aliceConn := s.connectAs(alice)

	s.clock.advance(sessionLifetime - time.Hour - time.Microsecond)
	justBefore := s.sendFrom(alice, "bob", "just before expiry")
	if justBefore.Status != http.StatusOK {
		t.Fatalf("送信が完了しなかった: %d", justBefore.Status)
	}
	if ev := bobConn.next(); ev.Message.Body != "just before expiry" {
		t.Fatalf("期限の直前のメッセージが、届かない: %s", ev.Raw)
	}
	aliceConn.next()

	s.clock.advance(time.Microsecond)
	if resp := s.sendFrom(alice, "bob", "at expiry"); resp.Status != http.StatusOK {
		t.Fatalf("送信が完了しなかった: %d", resp.Status)
	}

	// bob の接続は、メッセージを受けずに、閉じられる。alice の接続には、届く。
	bobConn.requireSessionEnded()
	if ev := aliceConn.next(); ev.Message.Body != "at expiry" {
		t.Fatalf("送信した端末に、届かない: %s", ev.Raw)
	}
}

// RL-20: 同じブラウザで、alice がログアウトし、carol がログインした後(または、ログインし直した後)、
// alice のログイン状態による接続は、閉じられる。前の識別子と、新しい Cookie の組の操作は、拒否される。
// alice を示したままの端末が、carol として送信することはない。
func TestReloginInSameBrowser_OldTabCannotActAsNewUser(t *testing.T) {
	c := fixedAccounts["carol"]

	t.Run("ログインし直し(ログアウトを経ない)", func(t *testing.T) {
		s := startServer(t)
		asAlice := s.newBrowser("alice")
		oldTab := s.connectAs(asAlice)

		asCarol := s.mustLoginIn(c.Contact, c.Secret, asAlice.Cookie)

		// 接続を保っている端末は、操作なしに、ログイン状態が無効になったことを知る。
		oldTab.requireSessionEnded()
		// 接続が切れている端末の、次の操作。ブラウザは、新しい Cookie を付け、端末は、前の識別子を添える。
		staleTab := &browser{Cookie: asCarol.Cookie, SessionID: asAlice.SessionID}
		requireError(t, s.sendFrom(staleTab, "bob", "typed while showing alice"), http.StatusUnauthorized, "unauthenticated")
		requireError(t, s.historyFrom(staleTab, "bob"), http.StatusUnauthorized, "unauthenticated")
		s.requireRejectedDial(staleTab, http.StatusUnauthorized, "unauthenticated")
		s.requireStored(0, 0)

		s.requireAuthenticatedAs(asCarol, "carol")
		// carol の接続には、alice が参加し、carol が参加しない会話のメッセージが、届かない(番兵)。
		carolConn := s.connectAs(asCarol)
		s.mustSend("bob", "alice", "for alice only")
		carolConn.requireNext(s.mustSend("bob", "carol", "sentinel"))
	})

	t.Run("ログアウトの後のログイン", func(t *testing.T) {
		s := startServer(t)
		asAlice := s.newBrowser("alice")
		oldTab := s.connectAs(asAlice)

		requireNoContent(t, s.logout(asAlice))
		asCarol := s.mustLogin(c.Contact, c.Secret)

		oldTab.requireSessionEnded()
		staleTab := &browser{Cookie: asCarol.Cookie, SessionID: asAlice.SessionID}
		requireError(t, s.sendFrom(staleTab, "bob", "typed while showing alice"), http.StatusUnauthorized, "unauthenticated")
		s.requireStored(0, 0)
	})

	t.Run("同じ利用者としてのログインし直しでも、前の識別子は使えない", func(t *testing.T) {
		s := startServer(t)
		a := fixedAccounts["alice"]
		before := s.newBrowser("alice")
		oldTab := s.connectAs(before)

		after := s.mustLoginIn(a.Contact, a.Secret, before.Cookie)

		oldTab.requireSessionEnded()
		requireError(t, s.sendFrom(&browser{Cookie: after.Cookie, SessionID: before.SessionID}, "bob", "hello"), http.StatusUnauthorized, "unauthenticated")
	})
}

// RL-21: ログイン状態は、サーバーを再起動した後も、保たれる。登録の申し込みも、再起動の後に、完了できる。
func TestRestart_KeepsSessionsAndSignups(t *testing.T) {
	s := startServer(t)
	alice := s.newBrowser("alice")
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)

	restarted := s.restart()

	restarted.requireAuthenticatedAs(alice, "alice")
	if resp := restarted.sendFrom(alice, "bob", "after restart"); resp.Status != http.StatusOK {
		t.Fatalf("再起動の後に、操作できない: %d %s", resp.Status, resp.Body)
	}
	restarted.connectAs(alice).close()
	requireNoContent(t, restarted.completeSignup(credential, dave.Handle, dave.DisplayName, dave.Secret))
	restarted.mustLogin(dave.Contact, dave.Secret)
}

// failingSessions は、ログイン状態を確かめる操作(Find)が、失敗する、port.SessionStore の実装である。
type failingSessions struct {
	port.SessionStore
	fail bool
}

func (f *failingSessions) Find(ctx context.Context, credential string, now time.Time) (domain.Session, domain.User, bool, error) {
	if f.fail {
		return domain.Session{}, domain.User{}, false, errors.New("session store is down")
	}
	return f.SessionStore.Find(ctx, credential, now)
}

// RL-23: ログイン状態の確認を完了できなかった操作は、受け付けられず、ログイン済みの操作として扱われない。
// ログイン状態を確かめる操作も、ログインしていないとは示さず、完了できなかったことを示す。
func TestSessionStoreFailure_OperationsAreNotAccepted(t *testing.T) {
	failing := &failingSessions{}
	s := startServer(t, withSessionStore(func(inner port.SessionStore) port.SessionStore {
		failing.SessionStore = inner
		return failing
	}))
	alice := s.newBrowser("alice")
	bobConn := s.connect("bob")
	failing.fail = true

	responses := map[string]response{
		"ログイン状態を確かめる": s.getSession(alice.Cookie),
		"ログアウト":       s.logout(alice),
		"相手の特定":       s.findUser(alice, "bob"),
		"履歴":          s.historyFrom(alice, "bob"),
		"送信":          s.sendFrom(alice, "bob", "must not be stored"),
	}
	for name, resp := range responses {
		if resp.Status != http.StatusInternalServerError || string(resp.Body) != `{"code":"internal"}` {
			t.Fatalf("%s: 応答が違う: %d %s", name, resp.Status, resp.Body)
		}
		requireNoInternals(t, resp)
	}
	s.requireRejectedDial(alice, http.StatusInternalServerError, "internal")
	s.requireStored(0, 0)
	if got := len(s.eventsOf("operation_failed")); got != len(responses)+1 {
		t.Fatalf("完了できなかった操作の記録の数が違う: %d", got)
	}

	// 確かめられるようになった後は、同じ資格で、操作できる。ログアウトは、行われていない。
	failing.fail = false
	sentinel := s.sendFrom(alice, "bob", "sentinel")
	if sentinel.Status != http.StatusOK {
		t.Fatalf("送信が完了しなかった: %d %s", sentinel.Status, sentinel.Body)
	}
	if ev := bobConn.next(); ev.Message.Body != "sentinel" || ev.Message.Seq != 1 {
		t.Fatalf("受け付けられなかった送信が、保管されたか、配信された: %s", ev.Raw)
	}
}
