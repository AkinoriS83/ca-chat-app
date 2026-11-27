package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// RL-24: 相手の識別名を入力すると、相手の識別名と表示名が示される。大文字と小文字を違えて入力しても、
// 相手が登録した通りの識別名が示される。会話を開き、送信できる。既に会話がある場合は、その履歴が示される。
func TestFindUser_ReturnsHandleAsRegistered(t *testing.T) {
	s := startServer(t)
	s.mustRegister(dave) // 識別名は、Dave_01
	alice := s.as("alice")

	for _, input := range []string{"Dave_01", "dave_01", "DAVE_01"} {
		resp := s.findUser(alice, input)
		if resp.Status != http.StatusOK || string(resp.Body) != `{"displayName":"Dave D","handle":"Dave_01"}` {
			t.Fatalf("%q: 特定の応答が違う: %d %s", input, resp.Status, resp.Body)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control が違う: %q", got)
		}
	}
	// 特定は、会話を開かない。成立させない。
	s.requireStored(0, 0)

	// 大文字と小文字を違えた指定でも、同じ会話を開き、送信できる。
	if got := s.historyFrom(alice, "dave_01"); got.Status != http.StatusOK || string(got.Body) != `{"messages":[]}` {
		t.Fatalf("履歴の応答が違う: %d %s", got.Status, got.Body)
	}
	sent := s.sendFrom(alice, "DAVE_01", "hello dave")
	if sent.Status != http.StatusOK || !strings.Contains(string(sent.Body), `"participants":["Dave_01","alice"]`) {
		t.Fatalf("送信の応答が違う: %d %s", sent.Status, sent.Body)
	}
	daveBrowser := s.mustLogin(dave.Contact, dave.Secret)
	history := s.historyFrom(daveBrowser, "ALICE")
	if history.Status != http.StatusOK || !strings.Contains(string(history.Body), `"body":"hello dave"`) {
		t.Fatalf("既にある会話の履歴が、示されない: %d %s", history.Status, history.Body)
	}
	s.requireStored(1, 1)
}

// RL-25: 一致する利用者が居ない識別名(他の利用者の識別名の一部である場合を含む)と、登録を完了していない人が
// 入力しかけた識別名は、同じ応答で、見つからないと示される。応答は、候補を含まない。
// RL-26: 自分の識別名を入力すると(大文字と小文字を違えた場合を含む)、相手にできないことが示される。
func TestFindUser_Rejections(t *testing.T) {
	s := startServer(t)
	alice := s.as("alice")
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)
	requireInvalidFields(t, s.completeSignup(credential, dave.Handle, "", dave.Secret), "displayName")

	notFound := s.findUser(alice, "nobody")
	requireError(t, notFound, http.StatusNotFound, "not_found")
	requireNoInternals(t, notFound)
	for _, handle := range []string{"bo_", "bobb", "carol_", "aro", dave.Handle, "00000000"} {
		requireSameResponse(t, notFound, s.findUser(alice, handle))
	}
	// 履歴と送信の「見つからない」応答とも、区別できない。
	requireSameResponse(t, notFound, s.historyFrom(alice, "nobody"))

	for _, handle := range []string{"alice", "ALICE", "Alice"} {
		requireError(t, s.findUser(alice, handle), http.StatusBadRequest, "invalid_peer")
	}
	// 入力が、識別名の規則を満たさない(R-3)。
	for _, handle := range []string{"bo", "b%25", "bob%20", "bob.", strings.Repeat("a", 21), "%E3%81%82%E3%81%84%E3%81%86"} {
		requireError(t, s.findUser(alice, handle), http.StatusBadRequest, "invalid_input")
	}
}

// RL-27: 特定の応答と、会話の中で他の利用者に届く情報は、利用者について、識別名と表示名のほかを含まない。
// 連絡先を含まない。利用者の一覧を取得できる操作と、部分一致で利用者を探せる操作がない。
func TestUserInformation_IsLimitedToHandleAndDisplayName(t *testing.T) {
	s := startServer(t)
	alice := s.as("alice")
	bobConn := s.connect("bob")

	find := s.findUser(alice, "bob")
	sent := s.sendFrom(alice, "bob", "hello")
	history := s.historyFrom(alice, "bob")
	event := bobConn.next()
	login := s.loginResponse(presetAccounts["alice"].Contact, presetAccounts["alice"].Secret, "")
	current := s.getSession(alice.Cookie)

	var profile map[string]any
	if err := json.Unmarshal(find.Body, &profile); err != nil || len(profile) != 2 || profile["handle"] != "bob" || profile["displayName"] != "Bob" {
		t.Fatalf("特定の応答が、識別名と表示名のほかを含む: %s", find.Body)
	}
	for name, body := range map[string][]byte{"特定": find.Body, "送信": sent.Body, "履歴": history.Body, "配信": event.Raw, "ログイン": login.Body, "ログイン状態": current.Body} {
		for _, forbidden := range []string{"@", "contact", "secret", "argon2", presetAccounts["bob"].ID, presetAccounts["alice"].ID} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s の応答が、%q を含む: %s", name, forbidden, body)
			}
		}
	}

	// 利用者の一覧を取得できる操作と、部分一致で探せる操作は、定義されていない。
	for _, path := range []string{"/api/users", "/api/users/", "/api/users?handle=bo", "/api/users?q=b", "/api/users/b*", "/api/users/b%25"} {
		resp := s.do(http.MethodGet, path, alice.header(), nil)
		if resp.Status == http.StatusOK {
			t.Fatalf("%s: 利用者を返した: %s", path, resp.Body)
		}
	}
}

// requireRateLimited は、応答が、上限を超えたことを示す拒否であることを確かめる。
func requireRateLimited(t *testing.T, resp response) {
	t.Helper()
	requireError(t, resp, http.StatusTooManyRequests, "rate_limited")
	requireNoInternals(t, resp)
}

// RL-28: 登録の申し込みの上限(送信元ごと)。上限に達するまでは受け付けられ、達した後は拒否される。
// 定めた時間が過ぎると、再び受け付けられる。拒否された要求と、規則を満たさない申し込みは、数えない。
func TestLimit_SignupRequestPerSource(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.SignupRequestPerSource = domain.LimitRule{Count: 2, Window: time.Hour}
	}))

	requireInvalidFields(t, s.requestSignup("not-a-contact"), "contact")
	requireNoContent(t, s.requestSignup("a@example.com"))
	s.clock.advance(10 * time.Minute)
	requireNoContent(t, s.requestSignup("b@example.com"))

	for i := 0; i < 3; i++ {
		requireRateLimited(t, s.requestSignup("c@example.com"))
	}
	if len(s.notices.all()) != 2 || s.count("signups") != 2 {
		t.Fatal("上限で拒否した申し込みで、案内を送ったか、申し込みを置いた")
	}
	// 連絡先の規則は、上限より先に判定する。
	requireInvalidFields(t, s.requestSignup("not-a-contact"), "contact")

	// 最初の申し込みから、1時間の直前までは、拒否される。ちょうど1時間で、1回分が空く。
	s.clock.advance(50*time.Minute - time.Microsecond)
	requireRateLimited(t, s.requestSignup("c@example.com"))
	s.clock.advance(time.Microsecond)
	requireNoContent(t, s.requestSignup("c@example.com"))
	requireRateLimited(t, s.requestSignup("d@example.com"))

	events := s.eventsOf("limit_exceeded")
	if len(events) != 5 || events[0]["limit"] != "signup_request_per_source" || events[0]["source"] != "127.0.0.1" {
		t.Fatalf("上限の超過の記録が違う: %v", events)
	}
}

// RL-28、RL-30: 登録の申し込みの上限(宛先の連絡先ごと)。連絡先が登録済みかどうかによって、変わらない。
// 大文字と小文字、前後の空白を違えた連絡先は、同じ宛先として数える。
func TestLimit_SignupRequestPerContact(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.SignupRequestPerContact = domain.LimitRule{Count: 2, Window: time.Hour}
	}))
	registered := presetAccounts["alice"].Contact

	var limited []response
	for _, contact := range []string{dave.Contact, registered} {
		requireNoContent(t, s.requestSignup(contact))
		requireNoContent(t, s.requestSignup(" "+strings.ToUpper(contact)+" "))
		resp := s.requestSignup(contact)
		requireRateLimited(t, resp)
		limited = append(limited, resp)
	}
	requireSameResponse(t, limited[0], limited[1])
	if got := len(s.notices.all()); got != 4 {
		t.Fatalf("送った案内の数が違う: %d", got)
	}
	// 別の宛先は、受け付けられる。
	requireNoContent(t, s.requestSignup("erin@example.com"))

	s.clock.advance(time.Hour)
	requireNoContent(t, s.requestSignup(dave.Contact))
	requireNoContent(t, s.requestSignup(registered))
	if got := s.eventsOf("limit_exceeded"); len(got) != 2 || got[0]["limit"] != "signup_request_per_contact" {
		t.Fatalf("上限の超過の記録が違う: %v", got)
	}
}

// RL-28: 登録の完了の上限(送信元ごと)。拒否された完了の要求(E1〜E3)も、数える。
func TestLimit_SignupCompletionPerSource(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.SignupCompletionPerSource = domain.LimitRule{Count: 3, Window: time.Hour}
	}))
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)

	requireError(t, s.completeSignup("wrong", dave.Handle, dave.DisplayName, dave.Secret), http.StatusBadRequest, "invalid_signup_credential")
	requireInvalidFields(t, s.completeSignup(credential, "!", dave.DisplayName, dave.Secret), "handle")
	requireError(t, s.completeSignup(credential, "alice", dave.DisplayName, dave.Secret), http.StatusConflict, "handle_taken")

	// 上限は、資格の確認より先に判定する。正しい完了も、無効な資格も、同じ応答になる。
	valid := s.completeSignup(credential, dave.Handle, dave.DisplayName, dave.Secret)
	requireRateLimited(t, valid)
	requireSameResponse(t, valid, s.completeSignup("wrong", "alice", "", ""))
	if s.count("users") != presetUserCount {
		t.Fatal("上限で拒否した完了で、利用者を作った")
	}

	s.clock.advance(time.Hour)
	requireNoContent(t, s.completeSignup(credential, dave.Handle, dave.DisplayName, dave.Secret))
	if got := s.eventsOf("limit_exceeded"); len(got) != 2 || got[0]["limit"] != "signup_completion_per_source" {
		t.Fatalf("上限の超過の記録が違う: %v", got)
	}
}

// RL-28、RL-29: ログインの上限(連絡先ごと)。失敗が、連続して上限の回数に達すると、最後の失敗から、定めた時間、
// 拒否する。拒否の間は、正しい秘密でも拒否される。連絡先が登録されているかどうかによって、変わらない。
// 既に与えたログイン状態では、操作を続けられる。拒否の間が過ぎると、0から数える。
func TestLimit_LoginFailurePerContact(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.LoginFailurePerContact = domain.LimitRule{Count: 3, Window: 15 * time.Minute}
	}))
	a := presetAccounts["alice"]
	existing := s.newBrowser("alice")
	const wrong = "wrong passphrase for tests"

	var limited []response
	for _, contact := range []string{a.Contact, "nobody@example.com"} {
		for i := 0; i < 3; i++ {
			requireError(t, s.loginResponse(contact, wrong, ""), http.StatusUnauthorized, "login_failed")
			s.clock.advance(time.Minute)
		}
		// 大文字と小文字を違えた連絡先も、同じ連絡先として数える。
		resp := s.loginResponse(strings.ToUpper(contact), a.Secret, "")
		requireRateLimited(t, resp)
		limited = append(limited, resp)
	}
	requireSameResponse(t, limited[0], limited[1])

	// 既に与えたログイン状態には、影響しない。別の連絡先のログインも、受け付けられる。
	if resp := s.historyFrom(existing, "bob"); resp.Status != http.StatusOK {
		t.Fatalf("既に与えたログイン状態で、操作できない: %d", resp.Status)
	}
	s.newBrowser("bob")

	// alice の最後の失敗は、始まりから2分の時点である。そこから15分の直前までは、拒否される。
	// (ここまでに、6分が過ぎている。)拒否された要求は、失敗に数えない。
	s.clock.advance(11*time.Minute - time.Microsecond)
	requireRateLimited(t, s.loginResponse(a.Contact, a.Secret, ""))
	s.clock.advance(time.Microsecond)
	s.mustLogin(a.Contact, a.Secret)

	// 成功すると、0に戻る。続く失敗は、上限の回数まで、照合の失敗として扱われる。
	for i := 0; i < 2; i++ {
		requireError(t, s.loginResponse(a.Contact, wrong, ""), http.StatusUnauthorized, "login_failed")
	}
	s.mustLogin(a.Contact, a.Secret)
	for i := 0; i < 2; i++ {
		requireError(t, s.loginResponse(a.Contact, wrong, ""), http.StatusUnauthorized, "login_failed")
	}
	s.mustLogin(a.Contact, a.Secret)

	if got := s.eventsOf("limit_exceeded"); len(got) != 3 || got[0]["limit"] != "login_failure_per_contact" || got[0]["user_id"] != nil {
		t.Fatalf("上限の超過の記録が違う: %v", got)
	}
}

// RL-29: 数える期間より前の失敗は、数えられない。入力の規則を満たさない連絡先でのログインは、
// 連絡先ごとには、数えられない。
func TestLimit_LoginFailurePerContact_WindowAndInvalidContact(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.LoginFailurePerContact = domain.LimitRule{Count: 3, Window: 15 * time.Minute}
	}))
	a := presetAccounts["alice"]
	const wrong = "wrong passphrase for tests"

	for i := 0; i < 2; i++ {
		requireError(t, s.loginResponse(a.Contact, wrong, ""), http.StatusUnauthorized, "login_failed")
	}
	// 15分が過ぎた後の失敗は、1回目から数える。
	s.clock.advance(15 * time.Minute)
	for i := 0; i < 2; i++ {
		requireError(t, s.loginResponse(a.Contact, wrong, ""), http.StatusUnauthorized, "login_failed")
	}
	s.mustLogin(a.Contact, a.Secret)

	// 規則を満たさない連絡先は、何度失敗しても、連絡先ごとの上限に当たらない。
	for i := 0; i < 10; i++ {
		requireError(t, s.loginResponse("not-a-contact", wrong, ""), http.StatusUnauthorized, "login_failed")
	}
}

// RL-28: ログインの上限(送信元ごと)。照合の失敗を数える。成功は、数えない。
func TestLimit_LoginFailurePerSource(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.LoginFailurePerSource = domain.LimitRule{Count: 3, Window: 15 * time.Minute}
	}))
	a := presetAccounts["alice"]

	// 成功したログインは、何度行っても、数えない。
	for i := 0; i < 5; i++ {
		s.mustLogin(a.Contact, a.Secret)
	}
	// 多くの連絡先に、少しずつ試す。規則を満たさない連絡先も、送信元ごとには、数える。
	for _, contact := range []string{"one@example.com", "two@example.com", "not-a-contact"} {
		requireError(t, s.loginResponse(contact, "wrong passphrase for tests", ""), http.StatusUnauthorized, "login_failed")
		s.clock.advance(time.Minute)
	}
	requireRateLimited(t, s.loginResponse(a.Contact, a.Secret, ""))
	requireRateLimited(t, s.loginResponse("three@example.com", "x", ""))

	// 最初の失敗から、15分で、1回分が空く。
	s.clock.advance(12*time.Minute - time.Microsecond)
	requireRateLimited(t, s.loginResponse(a.Contact, a.Secret, ""))
	s.clock.advance(time.Microsecond)
	s.mustLogin(a.Contact, a.Secret)

	if got := s.eventsOf("limit_exceeded"); len(got) != 3 || got[0]["limit"] != "login_failure_per_source" {
		t.Fatalf("上限の超過の記録が違う: %v", got)
	}
}

// RL-28、RL-31: 相手の特定の上限(利用者ごと)。特定の操作のほか、サーバーへ直接送った、会話を開く要求と、
// 送信する要求も数える。上限に達している間、成立した会話がない相手を指定した要求は、一致する利用者が居る場合と、
// 居ない場合で、同じ応答で拒否される。成立した会話がある相手を指定した要求は、数えられず、受け付けられる。
func TestLimit_PeerLookupPerUser(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.PeerLookupPerUser = domain.LimitRule{Count: 3, Window: time.Hour}
	}))
	alice := s.as("alice")
	aliceID := presetAccounts["alice"].ID

	// 成立した会話がある相手(bob)。最初の送信は、1回と数える。
	if resp := s.sendFrom(alice, "bob", "hello bob"); resp.Status != http.StatusOK {
		t.Fatal("準備の送信が、完了しなかった")
	}
	// 数えない要求: 入力の規則を満たさない、自分自身、成立した会話がある相手。
	requireError(t, s.findUser(alice, "!"), http.StatusBadRequest, "invalid_input")
	requireError(t, s.findUser(alice, "alice"), http.StatusBadRequest, "invalid_peer")
	requireError(t, s.historyFrom(alice, "alice"), http.StatusBadRequest, "invalid_peer")
	requireError(t, s.historyFrom(alice, "no such"), http.StatusNotFound, "not_found")
	for i := 0; i < 5; i++ {
		if resp := s.findUser(alice, "bob"); resp.Status != http.StatusOK {
			t.Fatalf("成立した会話がある相手の特定が、完了しなかった: %d", resp.Status)
		}
	}

	// 特定、会話を開く要求の、それぞれを数える(居た要求も、居なかった要求も)。ここで、上限に達する。
	if resp := s.findUser(alice, "carol"); resp.Status != http.StatusOK {
		t.Fatalf("特定が、完了しなかった: %d", resp.Status)
	}
	requireError(t, s.historyFrom(alice, "nobody"), http.StatusNotFound, "not_found")

	// 上限に達している間。居る相手と、居ない相手で、同じ応答になる。3つの操作で、同じである。
	limited := s.findUser(alice, "carol")
	requireRateLimited(t, limited)
	for name, resp := range map[string]response{
		"特定(居ない)": s.findUser(alice, "nobody"),
		"履歴(居る)":  s.historyFrom(alice, "carol"),
		"履歴(居ない)": s.historyFrom(alice, "nobody"),
		"送信(居る)":  s.sendFrom(alice, "carol", "hello carol"),
		"送信(居ない)": s.sendFrom(alice, "nobody", "hello"),
	} {
		if resp.Status != limited.Status || string(resp.Body) != string(limited.Body) {
			t.Fatalf("%s: 応答が一致しない: %d %s", name, resp.Status, resp.Body)
		}
	}
	s.requireStored(1, 1)

	// 成立した会話がある相手は、上限に達している間も、受け付けられる。
	if resp := s.findUser(alice, "BOB"); resp.Status != http.StatusOK {
		t.Fatalf("成立した会話がある相手の特定が、拒否された: %d", resp.Status)
	}
	if resp := s.historyFrom(alice, "bob"); resp.Status != http.StatusOK {
		t.Fatalf("成立した会話の履歴が、拒否された: %d", resp.Status)
	}
	if resp := s.sendFrom(alice, "bob", "still talking"); resp.Status != http.StatusOK {
		t.Fatalf("成立した会話への送信が、拒否された: %d", resp.Status)
	}
	// 上限は、利用者ごとである。別の利用者は、受け付けられる。
	if resp := s.findUser(s.as("bob"), "carol"); resp.Status != http.StatusOK {
		t.Fatalf("別の利用者の特定が、拒否された: %d", resp.Status)
	}

	// 最初に数えた要求から、1時間で、1回分が空く。
	s.clock.advance(time.Hour)
	if resp := s.sendFrom(alice, "carol", "hello carol"); resp.Status != http.StatusOK {
		t.Fatalf("時間が過ぎた後の送信が、拒否された: %d %s", resp.Status, resp.Body)
	}

	events := s.eventsOf("limit_exceeded")
	if len(events) != 6 || events[0]["limit"] != "peer_lookup_per_user" || events[0]["user_id"] != aliceID || events[0]["source"] != "127.0.0.1" {
		t.Fatalf("上限の超過の記録が違う: %v", events)
	}
}

// RL-28: 設定がない上限は、既定値で働く。登録の申し込みの、宛先の連絡先ごとの既定値(1時間に3回)で確かめる。
func TestLimit_Defaults(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) { *r = domain.DefaultLimitRules() }))

	for i := 0; i < 3; i++ {
		requireNoContent(t, s.requestSignup(dave.Contact))
	}
	requireRateLimited(t, s.requestSignup(dave.Contact))
}
