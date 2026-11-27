package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// dave は、テストの中で、登録する訪問者である。
var dave = account{Contact: "dave@example.com", Handle: "Dave_01", DisplayName: "Dave D", Secret: "dave passphrase for tests"}

// requireNoContent は、応答が、本文のない、完了の応答であることを確かめる。
func requireNoContent(t *testing.T, resp response) {
	t.Helper()
	if resp.Status != http.StatusNoContent || len(resp.Body) != 0 {
		t.Fatalf("完了の応答ではない: %d %s", resp.Status, resp.Body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control が違う: %q", got)
	}
}

// requireInvalidFields は、応答が、指定した項目を、この順に挙げる、invalid_fields の拒否であることを確かめる。
func requireInvalidFields(t *testing.T, resp response, fields ...string) {
	t.Helper()
	want, err := json.Marshal(map[string]any{"code": "invalid_fields", "fields": fields})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusBadRequest {
		t.Fatalf("状態が違う: %d %s", resp.Status, resp.Body)
	}
	var got, wantValue any
	if err := json.Unmarshal(resp.Body, &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(want, &wantValue)
	if canonicalJSON(got) != canonicalJSON(wantValue) {
		t.Fatalf("応答の本文が違う: got %s, want %s", resp.Body, want)
	}
}

// requireSameResponse は、2つの応答の、状態と本文が、一致することを確かめる。
func requireSameResponse(t *testing.T, a, b response) {
	t.Helper()
	if a.Status != b.Status || string(a.Body) != string(b.Body) {
		t.Fatalf("応答が一致しない: %d %s / %d %s", a.Status, a.Body, b.Status, b.Body)
	}
	if a.Header.Get("Set-Cookie") != b.Header.Get("Set-Cookie") {
		t.Fatal("応答の Cookie が、一致しない")
	}
}

// RL-01: 登録されていない連絡先で申し込むと、案内が1通送られる。案内の資格で、登録を完了でき、
// その連絡先と秘密で、ログインできる。登録の完了は、ログイン状態を与えない。
func TestSignup_RequestCompleteAndLogin(t *testing.T) {
	s := startServer(t)

	requireNoContent(t, s.requestSignup(dave.Contact))
	notices := s.notices.all()
	if len(notices) != 1 || notices[0].Registered || notices[0].To != dave.Contact || len(notices[0].Credential) != 43 {
		t.Fatalf("送った案内が違う: %d 通", len(notices))
	}
	// 有効期限は、申し込みから24時間である。
	if !notices[0].ExpiresAt.Equal(testStart.Add(24 * time.Hour)) {
		t.Fatalf("案内の有効期限が違う: %v", notices[0].ExpiresAt)
	}

	completed := s.completeSignup(notices[0].Credential, dave.Handle, dave.DisplayName, dave.Secret)
	requireNoContent(t, completed)
	if sessionCookieOf(completed) != nil || s.count("sessions") != 0 {
		t.Fatal("登録の完了で、ログイン状態を与えた")
	}
	if s.count("users") != presetUserCount+1 || s.count("signups") != 0 {
		t.Fatalf("利用者か、申し込みの行の数が違う: %d, %d", s.count("users"), s.count("signups"))
	}

	b := s.mustLogin(dave.Contact, dave.Secret)
	var status struct {
		Authenticated bool `json:"authenticated"`
		Session       struct {
			SessionID string `json:"sessionId"`
			User      struct {
				Handle      string `json:"handle"`
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"session"`
	}
	decodeStrict(t, s.getSession(b.Cookie).Body, &status)
	if !status.Authenticated || status.Session.SessionID != b.SessionID || status.Session.User.Handle != "Dave_01" || status.Session.User.DisplayName != "Dave D" {
		t.Fatalf("ログイン中の利用者が違う: %+v", status)
	}
}

// RL-02: 登録を完了するまでは、その連絡先でログインできず、他の利用者から特定されない。
// 入力しかけた識別名は、他の人が登録に使える。
func TestSignup_NothingExistsBeforeCompletion(t *testing.T) {
	s := startServer(t)
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)
	// 入力の規則で拒否された完了(識別名は正しく、表示名が正しくない)。
	requireInvalidFields(t, s.completeSignup(credential, dave.Handle, "", dave.Secret), "displayName")

	requireError(t, s.loginResponse(dave.Contact, dave.Secret, ""), http.StatusUnauthorized, "login_failed")
	requireError(t, s.findUser(s.as("alice"), dave.Handle), http.StatusNotFound, "not_found")

	erin := account{Contact: "erin@example.com", Handle: dave.Handle, DisplayName: "Erin", Secret: "erin passphrase for tests"}
	s.mustRegister(erin)
	s.mustLogin(erin.Contact, erin.Secret)
}

// RL-03: 登録済みの連絡先での申し込みの応答は、登録されていない連絡先での応答と、同じである。
// アカウントは作られず、既存のアカウントの秘密とログイン状態は、変わらない。
// その連絡先には、登録済みであることを伝える、資格を含まない案内が送られる。
func TestSignup_RegisteredContactGetsSameResponse(t *testing.T) {
	s := startServer(t)
	alice := s.as("alice")
	a := presetAccounts["alice"]
	var hashBefore string
	if err := s.pool.QueryRow(context.Background(), "SELECT secret_hash FROM users WHERE contact = $1", a.Contact).Scan(&hashBefore); err != nil {
		t.Fatal(err)
	}

	unregistered := s.requestSignup("nobody@example.com")
	registered := s.requestSignup("  ALICE@example.com ")

	requireNoContent(t, registered)
	requireSameResponse(t, unregistered, registered)

	notices := s.notices.to(a.Contact)
	if len(notices) != 1 || !notices[0].Registered || notices[0].Credential != "" {
		t.Fatalf("送った案内が違う: %+v", notices)
	}
	// 申し込みは、置かれない。置かれるのは、登録されていない連絡先のものだけである。
	if s.count("users") != presetUserCount || s.count("signups") != 1 {
		t.Fatalf("利用者か、申し込みの行の数が違う: %d, %d", s.count("users"), s.count("signups"))
	}
	var hashAfter string
	if err := s.pool.QueryRow(context.Background(), "SELECT secret_hash FROM users WHERE contact = $1", a.Contact).Scan(&hashAfter); err != nil {
		t.Fatal(err)
	}
	if hashAfter != hashBefore {
		t.Fatal("既存のアカウントの秘密が、変わった")
	}
	if resp := s.historyFrom(alice, "bob"); resp.Status != http.StatusOK {
		t.Fatalf("既存のログイン状態が、使えなくなった: %d", resp.Status)
	}
	s.mustLogin(a.Contact, a.Secret)
}

// RL-04: 同じ連絡先で、再び申し込むと、新しい案内が送られる。以前の案内では、完了できず、新しい案内では、完了できる。
func TestSignup_RepeatedRequestReplacesCredential(t *testing.T) {
	s := startServer(t)
	first := s.mustRequestSignup(dave.Contact, dave.Contact)
	s.clock.advance(time.Hour)
	second := s.mustRequestSignup(dave.Contact, dave.Contact)

	if first == second || len(s.notices.to(dave.Contact)) != 2 || s.count("signups") != 1 {
		t.Fatal("新しい案内が、送られていない")
	}
	// 有効期限は、新しい申し込みから数える。
	if got := s.notices.to(dave.Contact)[1].ExpiresAt; !got.Equal(testStart.Add(25 * time.Hour)) {
		t.Fatalf("新しい案内の有効期限が違う: %v", got)
	}

	requireError(t, s.completeSignup(first, dave.Handle, dave.DisplayName, dave.Secret), http.StatusBadRequest, "invalid_signup_credential")
	requireNoContent(t, s.completeSignup(second, dave.Handle, dave.DisplayName, dave.Secret))
}

// RL-05: 確認のための資格は、有効期限の直前には使え、過ぎると使えない。完了した資格は、再び使えない。
// 正しくない資格は、使えない。これらの拒否の応答は、同じであり、アカウントは作られない。
func TestSignup_InvalidCredentials(t *testing.T) {
	t.Run("期限の直前には、使える", func(t *testing.T) {
		s := startServer(t)
		credential := s.mustRequestSignup(dave.Contact, dave.Contact)
		s.clock.advance(24*time.Hour - time.Microsecond)

		requireNoContent(t, s.completeSignup(credential, dave.Handle, dave.DisplayName, dave.Secret))
	})

	t.Run("期限、使用済み、正しくない資格は、同じ応答で拒否される", func(t *testing.T) {
		s := startServer(t)
		expired := s.mustRequestSignup(dave.Contact, dave.Contact)
		s.clock.advance(24 * time.Hour)
		atExpiry := s.completeSignup(expired, dave.Handle, dave.DisplayName, dave.Secret)

		erin := account{Contact: "erin@example.com", Handle: "erin", DisplayName: "Erin", Secret: "erin passphrase for tests"}
		used := s.mustRequestSignup(erin.Contact, erin.Contact)
		requireNoContent(t, s.completeSignup(used, erin.Handle, erin.DisplayName, erin.Secret))
		usedAgain := s.completeSignup(used, "erin2", erin.DisplayName, erin.Secret)

		wrong := s.completeSignup(strings.Repeat("A", 43), "frank", "Frank", "frank passphrase for tests")
		empty := s.completeSignup("", "frank", "Frank", "frank passphrase for tests")

		requireError(t, atExpiry, http.StatusBadRequest, "invalid_signup_credential")
		for _, resp := range []response{usedAgain, wrong, empty} {
			requireSameResponse(t, atExpiry, resp)
		}
		if got := s.count("users"); got != presetUserCount+1 {
			t.Fatalf("利用者の行の数が違う: %d", got)
		}
	})
}

// RL-05: 同じ資格での完了が、同時に行われた場合、完了するのは、1つである。
func TestSignup_ConcurrentCompletionsWithSameCredential(t *testing.T) {
	s := startServer(t)
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)

	const n = 6
	responses := make([]response, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses[i] = s.completeSignup(credential, "dave_"+string(rune('a'+i)), dave.DisplayName, dave.Secret)
		}()
	}
	wg.Wait()

	completed := 0
	for _, resp := range responses {
		switch resp.Status {
		case http.StatusNoContent:
			completed++
		default:
			requireError(t, resp, http.StatusBadRequest, "invalid_signup_credential")
		}
	}
	if completed != 1 || s.count("users") != presetUserCount+1 {
		t.Fatalf("完了した数が違う: %d", completed)
	}
}

// RL-06: 入力の規則の外側の入力は、正しくない項目を、すべて示して拒否される。サーバーへ直接送った要求に対して、
// 確かめる。規則ごとの境界の値は、domain のテストで確かめる。拒否の後、同じ案内のまま、入力し直して完了できる。
func TestSignup_InvalidFields(t *testing.T) {
	s := startServer(t)

	// 申し込み: 連絡先。
	for _, contact := range []string{"", "not-a-contact", "a@b", "アリス@example.com", strings.Repeat("a", 65) + "@example.com"} {
		requireInvalidFields(t, s.requestSignup(contact), "contact")
	}
	if len(s.notices.all()) != 0 || s.count("signups") != 0 {
		t.Fatal("拒否した申し込みで、案内を送ったか、申し込みを置いた")
	}

	credential := s.mustRequestSignup(dave.Contact, dave.Contact)
	commonSecret := "123456789012345" // よく使われる秘密の一覧にある値
	tests := []struct {
		name                        string
		handle, displayName, secret string
		fields                      []string
	}{
		{"識別名が短い", "ab", dave.DisplayName, dave.Secret, []string{"handle"}},
		{"識別名が長い", strings.Repeat("a", 21), dave.DisplayName, dave.Secret, []string{"handle"}},
		{"識別名に、使えない文字", "dave-01", dave.DisplayName, dave.Secret, []string{"handle"}},
		{"表示名が空白文字だけ", dave.Handle, " \t ", dave.Secret, []string{"displayName"}},
		{"表示名が長い", dave.Handle, strings.Repeat("あ", 51), dave.Secret, []string{"displayName"}},
		{"表示名に、制御文字", dave.Handle, "Da\nve", dave.Secret, []string{"displayName"}},
		{"秘密が短い", dave.Handle, dave.DisplayName, strings.Repeat("a", 14), []string{"secret"}},
		{"秘密が長い", dave.Handle, dave.DisplayName, strings.Repeat("a", 129), []string{"secret"}},
		{"秘密が、よく使われる秘密の一覧にある", dave.Handle, dave.DisplayName, commonSecret, []string{"secret"}},
		{"秘密が、連絡先と同じ(大文字と小文字を区別しない)", dave.Handle, dave.DisplayName, "DAVE@Example.com", []string{"secret"}},
		{"秘密が、識別名と同じ(大文字と小文字を区別しない)", "Dave_0123456789ab", dave.DisplayName, "dave_0123456789AB", []string{"secret"}},
		{"すべてが、正しくない", "", "", "", []string{"handle", "displayName", "secret"}},
		{"識別名と秘密が、正しくない", "da ve", dave.DisplayName, "short", []string{"handle", "secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.completeSignup(credential, tt.handle, tt.displayName, tt.secret)

			requireInvalidFields(t, resp, tt.fields...)
			requireNoInternals(t, resp)
		})
	}
	// 文字列として正しくない入力(対にならないサロゲートのエスケープ)。置き換えずに、内側へ渡す。
	lone := string(rune(0x5C)) + "ud83d"
	raw := `{"signupCredential":"` + credential + `","handle":"` + dave.Handle + `","displayName":"a` + lone + `","secret":"` + strings.Repeat("a", 15) + lone + `"}`
	requireInvalidFields(t, s.do(http.MethodPost, "/api/users", map[string]string{"Content-Type": "application/json"}, []byte(raw)), "displayName", "secret")

	if s.count("users") != presetUserCount || s.count("signups") != 1 {
		t.Fatal("拒否した完了で、利用者を作ったか、申し込みを使用済みにした")
	}

	// 境界の内側の入力は、受け付けられる(128文字の秘密、20文字の識別名、50文字の表示名)。
	requireNoContent(t, s.completeSignup(credential, strings.Repeat("d", 20), strings.Repeat("あ", 50), strings.Repeat("\U0001F600", 128)))
}

// RL-07: 使われている識別名(大文字と小文字だけが異なるものを含む)での完了は、別の識別名を求めて拒否される。
// 訪問者は、同じ案内のまま、別の識別名で完了できる。
func TestSignup_HandleTaken(t *testing.T) {
	s := startServer(t)
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)

	for _, handle := range []string{"alice", "ALICE", "Alice"} {
		requireError(t, s.completeSignup(credential, handle, dave.DisplayName, dave.Secret), http.StatusConflict, "handle_taken")
	}
	if s.count("users") != presetUserCount || s.count("signups") != 1 {
		t.Fatal("拒否した完了で、利用者を作ったか、申し込みを使用済みにした")
	}

	requireNoContent(t, s.completeSignup(credential, dave.Handle, dave.DisplayName, dave.Secret))
}

// RL-07: 同じ識別名での完了が、同時に行われた場合、完了するのは、1つである。
func TestSignup_ConcurrentCompletionsWithSameHandle(t *testing.T) {
	s := startServer(t)
	const n = 6
	credentials := make([]string, n)
	for i := range credentials {
		contact := "visitor" + string(rune('a'+i)) + "@example.com"
		credentials[i] = s.mustRequestSignup(contact, contact)
	}

	responses := make([]response, n)
	var wg sync.WaitGroup
	for i := range credentials {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle := "samename"
			if i%2 == 1 {
				handle = "SameName"
			}
			responses[i] = s.completeSignup(credentials[i], handle, "Same", "same passphrase for tests")
		}()
	}
	wg.Wait()

	completed := 0
	for _, resp := range responses {
		switch resp.Status {
		case http.StatusNoContent:
			completed++
		default:
			requireError(t, resp, http.StatusConflict, "handle_taken")
		}
	}
	if completed != 1 || s.count("users") != presetUserCount+1 || s.count("signups") != n-1 {
		t.Fatalf("完了した数が違う: %d", completed)
	}
}

// RL-08: 資格が無効な、登録の完了の要求に対する応答は、指定した識別名が使われているかどうかによって、変わらない。
func TestSignup_InvalidCredentialDoesNotRevealHandle(t *testing.T) {
	s := startServer(t)

	taken := s.completeSignup("no-such-credential", "alice", dave.DisplayName, dave.Secret)
	unused := s.completeSignup("no-such-credential", "unused_handle", dave.DisplayName, dave.Secret)
	invalid := s.completeSignup("no-such-credential", "!", "", "")

	requireError(t, taken, http.StatusBadRequest, "invalid_signup_credential")
	requireSameResponse(t, taken, unused)
	requireSameResponse(t, taken, invalid)
}

// RL-09: 有効期限を過ぎた申し込みは、連絡先とともに、保管データから消える。期限の前は、残る。
func TestSignup_ExpiredSignupIsDeleted(t *testing.T) {
	s := startServer(t)
	s.mustRequestSignup(dave.Contact, dave.Contact)

	s.clock.advance(24*time.Hour - time.Microsecond)
	s.sweep()
	if got := s.count("signups"); got != 1 {
		t.Fatalf("期限の前の申し込みが、消えた: %d", got)
	}

	s.clock.advance(time.Microsecond)
	s.sweep()
	if got := s.count("signups"); got != 0 {
		t.Fatalf("期限を過ぎた申し込みが、残っている: %d", got)
	}
	var n int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM signups WHERE contact = $1", dave.Contact).Scan(&n); err != nil || n != 0 {
		t.Fatalf("連絡先が、保管データに残っている: %d, %v", n, err)
	}
}

// RL-10: 案内を送れなかった場合、訪問者には、完了できなかったことが示され、出来事が記録される。
// この応答は、連絡先が登録済みかどうかによって、変わらない。
func TestSignup_NoticeFailure(t *testing.T) {
	s := startServer(t)
	s.notices.setFailure(errors.New("smtp is down"))

	unregistered := s.requestSignup(dave.Contact)
	registered := s.requestSignup(presetAccounts["alice"].Contact)

	requireError(t, unregistered, http.StatusInternalServerError, "internal")
	requireNoInternals(t, unregistered)
	requireSameResponse(t, unregistered, registered)

	failed := s.eventsOf("signup_notice_failed")
	if len(failed) != 2 || failed[0]["source"] != "127.0.0.1" {
		t.Fatalf("案内を送れなかったことが、記録されていない: %v", failed)
	}
	if got := s.eventsOf("operation_failed"); len(got) != 2 || got[0]["operation"] != "request signup" {
		t.Fatalf("完了できなかったことが、記録されていない: %v", got)
	}
}

// RL-11: 案内の宛先は、揃えた後の連絡先である。訪問者が入力した通りの連絡先(大文字と空白を含む)は、
// 案内を送るポートに、渡らない。案内の本文は、smtpmail のテストで確かめる。
func TestSignup_NoticeGoesToNormalizedContact(t *testing.T) {
	s := startServer(t)

	requireNoContent(t, s.requestSignup("  Dave@EXAMPLE.com\t"))

	notices := s.notices.all()
	if len(notices) != 1 || notices[0].To != "dave@example.com" {
		t.Fatalf("案内の宛先が違う: %+v", notices)
	}
	var stored string
	if err := s.pool.QueryRow(context.Background(), "SELECT contact FROM signups").Scan(&stored); err != nil || stored != "dave@example.com" {
		t.Fatalf("保管した連絡先が、揃えた後の値でない: %q, %v", stored, err)
	}
}

// RL-41(WS-27、WS-28): 表示名は、保管と表示の、どの経路でも、文字として扱われる。
// 画面の構造や、問い合わせとして解釈されうる記述を、そのまま保管し、そのまま返す。
func TestSignup_DisplayNameIsStoredAsText(t *testing.T) {
	s := startServer(t)
	names := []string{
		`<script>alert(1)</script>`,
		`<img src=x onerror="alert(1)">`,
		`Robert'); DROP TABLE users;--`,
		`"quoted" \ back\slash & amp;`,
	}
	for i, name := range names {
		a := account{Contact: "x" + string(rune('a'+i)) + "@example.com", Handle: "tester_" + string(rune('a'+i)), DisplayName: name, Secret: "tester passphrase for tests"}
		s.mustRegister(a)

		var found struct {
			Handle      string `json:"handle"`
			DisplayName string `json:"displayName"`
		}
		resp := s.findUser(s.as("alice"), a.Handle)
		decodeStrict(t, resp.Body, &found)
		if found.DisplayName != name {
			t.Fatalf("表示名が、入力された通りでない: got %q, want %q", found.DisplayName, name)
		}

		// メッセージの送信者としても、配信でも、同じである。
		aliceConn := s.connect("alice")
		b := s.mustLogin(a.Contact, a.Secret)
		resp = s.sendFrom(b, "alice", "hello")
		if resp.Status != http.StatusOK {
			t.Fatalf("送信が完了しなかった: %d %s", resp.Status, resp.Body)
		}
		if got := aliceConn.next().Message.Sender.DisplayName; got != name {
			t.Fatalf("配信の表示名が、入力された通りでない: %q", got)
		}
		aliceConn.close()
	}
	if got := s.count("users"); got != presetUserCount+len(names) {
		t.Fatalf("利用者の表が、変わった: %d", got)
	}
}
