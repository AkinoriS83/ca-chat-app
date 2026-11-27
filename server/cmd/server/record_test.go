package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// RL-32: 仕様「記録」の出来事が、種類、送信元、利用者の識別子(定めた場合)とともに、記録される。
func TestEvents_AreRecordedWithSourceAndUser(t *testing.T) {
	s := startServer(t, withLimits(func(r *domain.LimitRules) {
		r.PeerLookupPerUser = domain.LimitRule{Count: 1, Window: time.Hour}
	}))
	a := presetAccounts["alice"]

	s.mustRegister(dave)
	daveID := s.userID(dave.Contact)
	requireNoContent(t, s.requestSignup(a.Contact)) // 登録済みの連絡先の申し込みも、受け付けたこととして記録する
	alice := s.mustLogin(a.Contact, a.Secret)
	requireError(t, s.loginResponse(a.Contact, "wrong passphrase for tests", ""), http.StatusUnauthorized, "login_failed")
	requireError(t, s.loginResponse("nobody@example.com", "wrong passphrase for tests", ""), http.StatusUnauthorized, "login_failed")
	requireError(t, s.historyFrom(&browser{}, "bob"), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.findUser(alice, "nobody"), http.StatusNotFound, "not_found")
	requireRateLimited(t, s.findUser(alice, "nobody"))
	requireNoContent(t, s.logout(alice))

	type want struct {
		kind   string
		count  int
		userID any // 最初の記録の、利用者の識別子。nil は、添えないことを表す
	}
	for _, w := range []want{
		{"signup_requested", 2, nil},
		{"signup_completed", 1, daveID},
		{"login_succeeded", 1, a.ID},
		{"login_failed", 2, a.ID}, // 連絡先が登録済みの失敗には、その利用者を添える
		{"authentication_rejected", 1, nil},
		{"limit_exceeded", 1, a.ID},
		{"logged_out", 1, a.ID},
	} {
		events := s.eventsOf(w.kind)
		if len(events) != w.count {
			t.Fatalf("%s の記録の数が違う: %d", w.kind, len(events))
		}
		for _, e := range events {
			if e["source"] != "127.0.0.1" || e["time"] == nil {
				t.Fatalf("%s の記録に、送信元か、発生時刻がない: %v", w.kind, e)
			}
		}
		if events[0]["user_id"] != w.userID {
			t.Fatalf("%s の記録の、利用者の識別子が違う: %v", w.kind, events[0])
		}
	}
	// 登録されていない連絡先に対する失敗には、利用者を添えない。
	if failed := s.eventsOf("login_failed"); failed[1]["user_id"] != nil {
		t.Fatalf("利用者の識別子が、添えられている: %v", failed[1])
	}
	if got := s.eventsOf("operation_failed"); len(got) != 0 {
		t.Fatalf("完了した操作と、拒否が、エラーとして記録された: %v", got)
	}
}

// RL-32: 片付けのエラーは、送信元を添えずに、記録される。
func TestEvents_SweepFailureIsRecorded(t *testing.T) {
	s := startServer(t)
	if _, err := s.pool.Exec(context.Background(), "ALTER TABLE signups RENAME TO signups_gone"); err != nil {
		t.Fatal(err)
	}

	if err := s.housekeeper.Sweep(context.Background()); err == nil {
		t.Fatal("片付けが、エラーを返さなかった")
	}

	events := s.eventsOf("operation_failed")
	if len(events) != 1 || events[0]["operation"] != "sweep" || events[0]["source"] != nil || !strings.Contains(fmt.Sprint(events[0]["error"]), "delete expired signups") {
		t.Fatalf("片付けのエラーの記録が違う: %v", events)
	}
}

// RL-33: 秘密、資格、連絡先、識別名、表示名が、サーバーが出力するログに、含まれない。
// 秘密と資格は、拒否とエラーの応答にも、ログインの応答の本文にも、含まれない。
// 操作が完了した場合、拒否された場合、エラーとなった場合の、いずれでも同じである。
func TestLogsAndResponses_DoNotContainSecretsOrPersonalData(t *testing.T) {
	const marker = "MARKER7f3a9c"
	visitor := account{
		Contact:     "marker7f3a9c@example.com",
		Handle:      "u_" + marker,
		DisplayName: "Name " + marker,
		Secret:      "secret " + marker + " 0123456789",
	}

	failing := &failBeforeCommit{}
	s := startServer(t, withTransactor(func(inner port.Transactor) port.Transactor {
		failing.inner = inner
		return failing
	}))
	var bodies [][]byte
	var credentials []string
	collect := func(resp response) response {
		bodies = append(bodies, resp.Body)
		return resp
	}

	// 拒否された場合。
	collect(s.requestSignup("MARKER7F3A9C@@example.com"))
	collect(s.completeSignup(marker+"-not-a-credential", visitor.Handle, visitor.DisplayName, visitor.Secret))
	collect(s.loginResponse(visitor.Contact, visitor.Secret, marker+"-not-a-cookie"))
	collect(s.do(http.MethodPost, "/api/users", nil, []byte(`{"signupCredential":"`+marker+`","handle":`)))

	// エラーとなった場合(案内を送れない、登録の完了の確定に失敗する)。
	s.notices.setFailure(errors.New("smtp failure"))
	requireError(t, collect(s.requestSignup(" "+strings.ToUpper(visitor.Contact))), http.StatusInternalServerError, "internal")
	s.notices.setFailure(nil)
	signupCredential := s.mustRequestSignup(visitor.Contact, visitor.Contact)
	credentials = append(credentials, signupCredential)
	failing.signupErr = errors.New("commit failure")
	requireError(t, collect(s.completeSignup(signupCredential, visitor.Handle, visitor.DisplayName, visitor.Secret)), http.StatusInternalServerError, "internal")
	failing.signupErr = nil
	// 保管先が、行の内容を含むエラーを返す場合。利用者の行を拒否する制約を、このテストのデータベースにだけ加える。
	if _, err := s.pool.Exec(context.Background(), "ALTER TABLE users ADD CONSTRAINT test_reject_marker CHECK (display_name NOT LIKE '%"+marker+"%')"); err != nil {
		t.Fatal(err)
	}
	requireError(t, collect(s.completeSignup(signupCredential, visitor.Handle, visitor.DisplayName, visitor.Secret)), http.StatusInternalServerError, "internal")
	if _, err := s.pool.Exec(context.Background(), "ALTER TABLE users DROP CONSTRAINT test_reject_marker"); err != nil {
		t.Fatal(err)
	}

	// 入力の規則による拒否と、識別名が使われている拒否。
	collect(s.completeSignup(signupCredential, visitor.Handle, visitor.DisplayName, marker))
	collect(s.completeSignup(signupCredential, "alice", visitor.DisplayName, visitor.Secret))

	// 完了した場合。
	requireNoContent(t, s.completeSignup(signupCredential, visitor.Handle, visitor.DisplayName, visitor.Secret))
	collect(s.loginResponse(visitor.Contact, "wrong "+marker+" 0123456789", ""))
	login := collect(s.loginResponse(visitor.Contact, visitor.Secret, ""))
	b := &browser{Cookie: sessionCookieOf(login).Value}
	credentials = append(credentials, b.Cookie)
	current := s.getSession(b.Cookie)
	if !strings.Contains(string(current.Body), visitor.Handle) {
		t.Fatalf("ログインしていない: %s", current.Body)
	}
	alice := s.as("alice")
	credentials = append(credentials, alice.Cookie)
	if resp := s.findUser(alice, visitor.Handle); resp.Status != http.StatusOK {
		t.Fatalf("特定が、完了しなかった: %d", resp.Status)
	}
	collect(s.findUser(alice, "x_"+marker))
	if resp := s.sendFrom(alice, visitor.Handle, "hello"); resp.Status != http.StatusOK {
		t.Fatalf("送信が、完了しなかった: %d", resp.Status)
	}
	collect(s.historyFrom(&browser{Cookie: b.Cookie, SessionID: marker}, "alice"))
	conn := s.connect("alice")
	requireNoContent(t, s.logout(alice))
	conn.requireSessionEnded()

	logs := s.logs.String()
	// 目印を含めた、秘密、連絡先、識別名、表示名は、ログに現れない。大文字と小文字を、区別しない。
	if strings.Contains(strings.ToLower(logs), strings.ToLower(marker)) {
		t.Fatalf("ログに、記録しないと定めたものが含まれる:\n%s", logs)
	}
	for _, forbidden := range append(credentials, "alice@example.com", presetAccounts["alice"].Secret, "argon2id") {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("ログに、資格か、連絡先か、秘密が含まれる(%d 文字の値)", len(forbidden))
		}
	}
	// 完了できなかった操作は、操作の名前と、SQLSTATE とともに、記録されている。
	for _, want := range []string{`"event":"signup_notice_failed"`, `"operation":"complete signup"`, "create user", "23514"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("ログに、%q がない:\n%s", want, logs)
		}
	}

	// 秘密と資格は、応答の本文に現れない。
	for _, body := range bodies {
		for _, forbidden := range append(credentials, visitor.Secret, marker+" 0123456789", "argon2id") {
			if bytes.Contains(body, []byte(forbidden)) {
				t.Fatalf("応答の本文に、秘密か資格が含まれる: %s", body)
			}
		}
	}
}

// 秘密の型を、書式と、ログの JSON で出力しても、値が現れない。
// (domain のテストは、書式とログの標準ライブラリを import できないため、ここに置く。)
func TestSecret_IsRedactedInFormattingAndLogs(t *testing.T) {
	const raw = "marker-secret-value-0123456789"
	secret, err := domain.NewSecret(raw, domain.RestoreContact("someone@example.com"), "someone")
	if err != nil {
		t.Fatal(err)
	}
	type holder struct {
		Secret domain.Secret
		Ptr    *domain.Secret
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	logger.Info("secret", "value", secret, "pointer", &secret, "holder", holder{Secret: secret, Ptr: &secret})
	slog.New(slog.NewTextHandler(&logs, nil)).Info("secret", "value", secret, "holder", holder{Secret: secret})

	outputs := []string{
		fmt.Sprint(secret),
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%+v", secret),
		fmt.Sprintf("%#v", secret),
		fmt.Sprintf("value=%s", secret),
		fmt.Sprintf("%q", secret),
		fmt.Sprintf("%v", &secret),
		fmt.Sprintf("%v", holder{Secret: secret, Ptr: &secret}),
		fmt.Sprintf("%+v", holder{Secret: secret, Ptr: &secret}),
		fmt.Sprintf("%#v", holder{Secret: secret}),
		fmt.Errorf("failed with %v: %w", secret, errors.New("x")).Error(),
		logs.String(),
	}
	for _, out := range outputs {
		if strings.Contains(out, raw) || strings.Contains(out, "marker") {
			t.Fatalf("秘密の値が、出力に現れた: %s", out)
		}
	}
	if secret.Expose() != raw {
		t.Fatal("Expose が、値を返さない")
	}
}

// RL-34: 秘密は、復元できない形(Argon2id で計算した値)でだけ、保管される。
// 同じ秘密を持つ2人の利用者の、保管された値は、異なる。
func TestSecrets_AreStoredOnlyAsHashes(t *testing.T) {
	s := startServer(t)
	const shared = "the same secret for two people"
	first := account{Contact: "first@example.com", Handle: "first", DisplayName: "First", Secret: shared}
	second := account{Contact: "second@example.com", Handle: "second", DisplayName: "Second", Secret: shared}
	s.mustRegister(first)
	s.mustRegister(second)

	rows, err := s.pool.Query(context.Background(), "SELECT secret_hash, u::text FROM users u WHERE contact IN ($1, $2) ORDER BY contact", first.Contact, second.Contact)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var hashes []string
	for rows.Next() {
		var hash, row string
		if err := rows.Scan(&hash, &row); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
			t.Fatal("保管した値が、Argon2id の形でない")
		}
		if strings.Contains(row, shared) {
			t.Fatal("利用者の行が、秘密を含む")
		}
		hashes = append(hashes, hash)
	}
	if len(hashes) != 2 || hashes[0] == hashes[1] {
		t.Fatalf("同じ秘密の、保管された値が、異ならない(%d 件)", len(hashes))
	}
	s.mustLogin(first.Contact, shared)
	s.mustLogin(second.Contact, shared)
}

// RL-42: 識別名を、要求に添えるだけで、その利用者として操作する手段が、残っていない。
func TestTemporaryLogin_IsGone(t *testing.T) {
	s := startServer(t)
	s.mustSend("alice", "bob", "secret")
	fixed := map[string]string{"Content-Type": "application/json", "X-Fixed-User": "alice"}

	requireError(t, s.do(http.MethodGet, messagesPath("bob"), fixed, nil), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.do(http.MethodPost, messagesPath("bob"), fixed, []byte(`{"sendId":"0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70","body":"hello"}`)), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.do(http.MethodGet, "/api/users/bob", fixed, nil), http.StatusUnauthorized, "unauthenticated")
	requireError(t, s.do(http.MethodGet, messagesPath("bob")+"?user=alice", nil, nil), http.StatusUnauthorized, "unauthenticated")

	// リアルタイムの接続。クエリ user を添えただけの要求。
	upgrade := map[string]string{"Upgrade": "websocket", "Connection": "Upgrade"}
	requireError(t, s.do(http.MethodGet, "/api/realtime?user=alice", upgrade, nil), http.StatusUnauthorized, "unauthenticated")
	s.requireStored(1, 1)
}

// RL-41(WS-29): 第三者のサイトに置かれたページから、本スライスで加えた操作を行えない。
func TestOtherOrigin_IsRejectedForAccountOperations(t *testing.T) {
	s := startServer(t)
	alice := s.as("alice")
	a := presetAccounts["alice"]
	credential := s.mustRequestSignup(dave.Contact, dave.Contact)
	noticesBefore := len(s.notices.all())

	for _, origin := range []string{"https://evil.example", "null", "http://app.test:8080"} {
		with := func(h map[string]string) map[string]string {
			h["Origin"] = origin
			return h
		}
		json := func() map[string]string { return map[string]string{"Content-Type": "application/json"} }
		responses := []response{
			s.do(http.MethodPost, "/api/signups", with(json()), []byte(`{"contact":"erin@example.com"}`)),
			s.do(http.MethodPost, "/api/users", with(json()), []byte(`{"signupCredential":"`+credential+`","handle":"dave","displayName":"Dave","secret":"`+dave.Secret+`"}`)),
			s.do(http.MethodPost, "/api/session", with(json()), []byte(`{"contact":"`+a.Contact+`","secret":"`+a.Secret+`"}`)),
			s.do(http.MethodGet, "/api/session", with(alice.header()), nil),
			s.do(http.MethodDelete, "/api/session", with(alice.header()), nil),
			s.do(http.MethodGet, "/api/users/bob", with(alice.header()), nil),
		}
		for _, resp := range responses {
			requireError(t, resp, http.StatusForbidden, "forbidden_origin")
			requireNoCORS(t, resp)
			if len(resp.Header.Values("Set-Cookie")) != 0 {
				t.Fatal("拒否の応答が、Cookie を変えた")
			}
		}
	}
	// どの操作も、行われていない。
	if len(s.notices.all()) != noticesBefore || s.count("users") != presetUserCount || s.count("sessions") != 1 {
		t.Fatal("他のオリジンからの操作が、行われた")
	}
	s.requireAuthenticatedAs(alice, "alice")
}

// RL-41(WS-30): 本スライスで加えた操作の、拒否の応答とエラー応答も、内部の構造が分かる情報を含まない。
// 形式の壊れた要求など、想定しない入力に対しても同じである。
func TestMalformedAccountRequests_GetCodeOnlyResponses(t *testing.T) {
	s := startServer(t)
	alice := s.as("alice")
	tooLarge := `{"contact":"` + strings.Repeat("a", 16<<10) + `"}`

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{"申し込み: 壊れた JSON", "POST", "/api/signups", `{"contact":`, 400, "invalid_input"},
		{"申し込み: 空の本文", "POST", "/api/signups", ``, 400, "invalid_input"},
		{"申し込み: 項目がない", "POST", "/api/signups", `{}`, 400, "invalid_input"},
		{"申し込み: 項目の型が違う", "POST", "/api/signups", `{"contact":["a@example.com"]}`, 400, "invalid_input"},
		{"申し込み: 大きさの上限を超える", "POST", "/api/signups", tooLarge, 400, "invalid_input"},
		{"完了: 壊れた JSON", "POST", "/api/users", `{"signupCredential":"x"`, 400, "invalid_input"},
		{"完了: 項目がない", "POST", "/api/users", `{"signupCredential":"x","handle":"dave","displayName":"Dave"}`, 400, "invalid_input"},
		{"完了: 項目が null", "POST", "/api/users", `{"signupCredential":"x","handle":"dave","displayName":"Dave","secret":null}`, 400, "invalid_input"},
		{"ログイン: 壊れた JSON", "POST", "/api/session", `not json`, 400, "invalid_input"},
		{"ログイン: 項目がない", "POST", "/api/session", `{"contact":"alice@example.com"}`, 400, "invalid_input"},
		{"ログイン: 項目の型が違う", "POST", "/api/session", `{"contact":"alice@example.com","secret":12345}`, 400, "invalid_input"},
		{"ログイン: 大きさの上限を超える", "POST", "/api/session", `{"contact":"alice@example.com","secret":"` + strings.Repeat("a", 16<<10) + `"}`, 400, "invalid_input"},
		{"定義されていない方法(PUT /api/session)", "PUT", "/api/session", `{}`, 404, "not_found"},
		{"定義されていない方法(DELETE /api/users/bob)", "DELETE", "/api/users/bob", ``, 404, "not_found"},
		{"定義されていない方法(GET /api/signups)", "GET", "/api/signups", ``, 404, "not_found"},
		{"定義されていないパス(/api/users/bob/contact)", "GET", "/api/users/bob/contact", ``, 404, "not_found"},
		{"定義されていないパス(/api/sessions)", "GET", "/api/sessions", ``, 404, "not_found"},
		{"相手が空のパス", "GET", "/api/users//", ``, 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.do(tt.method, tt.path, alice.header(), []byte(tt.body))

			requireError(t, resp, tt.status, tt.code)
			requireNoInternals(t, resp)
		})
	}
	if s.count("signups") != 0 || len(s.notices.all()) != 0 {
		t.Fatal("形式に合わない要求が、行われた")
	}
}
