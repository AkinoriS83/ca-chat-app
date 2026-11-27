package main

// サーバーの振る舞いのテストの道具立て。
//
// 実際の adapter を使うサーバーを、main と同じ関数で組み立て、httptest.Server で起動する。
// 差し替えるのは、時刻の源(テストから進める)と、案内の送信(送った案内を、覚える)である。
// 秘密の計算は、実際に行う(ADR-015)。上限は、既定では、当たらない値にする。
// 保管先は、テストごとに、雛形から複製したデータベースである(make test-server)。
// 要求と接続は、クライアントを介さずに、サーバーへ直接に行う(WS-12、WS-26)。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/argon2id"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/commonsecrets"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/eventlog"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres/pgtest"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/ratelimit"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/system"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/wsdelivery"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

const (
	// testAppOrigin は、テストのサーバーに与える、アプリのオリジンである。
	testAppOrigin = "http://app.test"
	// eventTimeout は、次のイベントを待つ時間の上限である。失敗を検出するためのものであり、
	// 成功する場合の所要時間には、影響しない。
	eventTimeout = 5 * time.Second
)

// syncBuffer は、複数の goroutine から書き込める、ログの出力先である。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testClock は、テストから進める、時刻の源である。時刻は、保管先の精度(マイクロ秒)で扱う。
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

// testStart は、テストの時刻の始まりである。
var testStart = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance は、時刻を進める。
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// sentNotice は、送った案内である。
type sentNotice struct {
	Registered bool // 登録済みであることを伝える案内
	To         string
	Credential string // 確認のための資格。登録済みであることを伝える案内では、空
	ExpiresAt  time.Time
}

// recordingNotices は、送った案内を覚える、port.NoticeSender の実装である。
// fail に、エラーを置くと、送れなかった場合になる(RL-10)。
type recordingNotices struct {
	mu   sync.Mutex
	sent []sentNotice
	fail error
}

func (n *recordingNotices) SendSignupNotice(_ context.Context, to domain.Contact, credential string, expiresAt time.Time) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.fail != nil {
		return n.fail
	}
	n.sent = append(n.sent, sentNotice{To: to.String(), Credential: credential, ExpiresAt: expiresAt})
	return nil
}

func (n *recordingNotices) SendAlreadyRegisteredNotice(_ context.Context, to domain.Contact) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.fail != nil {
		return n.fail
	}
	n.sent = append(n.sent, sentNotice{Registered: true, To: to.String()})
	return nil
}

func (n *recordingNotices) setFailure(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.fail = err
}

// all は、送った案内を、送った順に返す。
func (n *recordingNotices) all() []sentNotice {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]sentNotice(nil), n.sent...)
}

// to は、宛先に送った案内を、送った順に返す。
func (n *recordingNotices) to(contact string) []sentNotice {
	var out []sentNotice
	for _, notice := range n.all() {
		if notice.To == contact {
			out = append(out, notice)
		}
	}
	return out
}

type testServer struct {
	t           *testing.T
	url         string
	pool        *pgxpool.Pool
	logs        *syncBuffer
	http        *http.Client
	clock       *testClock
	notices     *recordingNotices
	housekeeper *usecase.Housekeeper
	options     serverOptions

	mu       sync.Mutex
	browsers map[string]*browser // あらかじめ置く利用者(alice、bob、carol)ごとの、既定のブラウザ
}

type serverOption func(*serverOptions)

type serverOptions struct {
	wrapTx       func(port.Transactor) port.Transactor
	wrapSessions func(port.SessionStore) port.SessionStore
	limits       domain.LimitRules
}

// withTransactor は、トランザクションのポートを、差し替えた実装で包む(WS-31)。
func withTransactor(wrap func(port.Transactor) port.Transactor) serverOption {
	return func(o *serverOptions) { o.wrapTx = wrap }
}

// withSessionStore は、ログイン状態を保持するポートを、差し替えた実装で包む(RL-23)。
func withSessionStore(wrap func(port.SessionStore) port.SessionStore) serverOption {
	return func(o *serverOptions) { o.wrapSessions = wrap }
}

// withLimits は、上限の値を変える(RL-28)。
func withLimits(change func(rules *domain.LimitRules)) serverOption {
	return func(o *serverOptions) { change(&o.limits) }
}

// unlimited は、テストが当たらない、上限の値である。上限のテストだけが、小さな値を与える。
func unlimited() domain.LimitRules {
	rule := domain.LimitRule{Count: 1_000_000, Window: time.Hour}
	return domain.LimitRules{
		SignupRequestPerSource:    rule,
		SignupRequestPerContact:   rule,
		SignupCompletionPerSource: rule,
		LoginFailurePerContact:    rule,
		LoginFailurePerSource:     rule,
		PeerLookupPerUser:         rule,
	}
}

// startServer は、実際の adapter を使うサーバーを、main と同じ関数で組み立てて、起動する。
func startServer(t *testing.T, opts ...serverOption) *testServer {
	t.Helper()
	options := serverOptions{limits: unlimited()}
	for _, opt := range opts {
		opt(&options)
	}
	return startServerOn(t, pgtest.NewDatabase(t), &testClock{now: testStart}, options)
}

// restart は、同じ保管先と、同じ時刻の源で、サーバーを組み立て直す(RL-21)。
// サーバーのメモリの上の状態(接続、上限の計数)は、引き継がれない。
func (s *testServer) restart() *testServer {
	s.t.Helper()
	return startServerOn(s.t, s.pool, s.clock, s.options)
}

func startServerOn(t *testing.T, pool *pgxpool.Pool, clock *testClock, options serverOptions) *testServer {
	t.Helper()
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	store := postgres.NewStore(pool)
	hub := wsdelivery.NewHub(logger)
	var tx port.Transactor = store
	if options.wrapTx != nil {
		tx = options.wrapTx(store)
	}
	var sessions port.SessionStore = store.Sessions()
	if options.wrapSessions != nil {
		sessions = options.wrapSessions(sessions)
	}
	notices := &recordingNotices{}

	app, err := assemble(ports{
		Users:         store,
		Reader:        store,
		Tx:            tx,
		Sessions:      sessions,
		Signups:       store.Signups(),
		Delivery:      hub,
		Registry:      hub,
		Connections:   hub,
		Credentials:   system.CredentialSource{},
		Hasher:        testHasher,
		CommonSecrets: commonsecrets.NewList(),
		Notices:       notices,
		Limiter:       ratelimit.NewLimiter(),
		Clock:         clock,
		Events:        eventlog.NewRecorder(logger),
	}, options.limits, testAppOrigin, logger)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewUnstartedServer(app.Handler)
	ts.Config = newHTTPServer("", app.Handler)
	ts.Start()
	t.Cleanup(ts.Close)

	client := ts.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	s := &testServer{
		t: t, url: ts.URL, pool: pool, logs: logs, http: client,
		clock: clock, notices: notices, housekeeper: app.Housekeeper, options: options,
		browsers: map[string]*browser{},
	}
	// あらかじめ置く利用者の行は、最初から置く。相手として指定されるためである。
	for name := range presetAccounts {
		s.ensurePreset(name)
	}
	return s
}

// presetUserCount は、あらかじめ置く利用者の数である。
const presetUserCount = 3

// testHasher は、秘密の計算を、実際に行う。同時に行う計算の数の上限を、テストのプロセスの全体で
// 共有するために、1つだけ作る(メモリを、約76 MiB に限る)。
var testHasher = argon2id.NewHasher()

// sweep は、片付けを、1回行う。
func (s *testServer) sweep() {
	s.t.Helper()
	if err := s.housekeeper.Sweep(context.Background()); err != nil {
		s.t.Fatalf("片付けが、完了しなかった: %v", err)
	}
}

// ---- 利用者と、ブラウザ ----

// account は、テストの利用者である。
type account struct {
	ID          string // 識別子を指定して、行を直接に作る場合だけ
	Contact     string
	Handle      string
	DisplayName string
	Secret      string
}

// あらかじめ置く利用者。ウォーキングスケルトンから引き継いだテスト(RL-41)が、名前で指定する。
// 識別子の順は、識別名の順と、わざと変えてある(WS-37)。通信の形式の participants は、識別名の昇順であり、
// domain.DirectPair は、識別子の昇順である。2つの順が同じだと、participants を DirectPair の順のまま
// 返す誤りを、テストで検出できない。登録の操作は、識別子を指定できないため、行を、直接に作る。
var presetAccounts = map[string]account{
	"alice": {ID: "00000000-0000-4000-8000-000000000003", Contact: "alice@example.com", Handle: "alice", DisplayName: "Alice", Secret: "alice passphrase for tests"},
	"bob":   {ID: "00000000-0000-4000-8000-000000000001", Contact: "bob@example.com", Handle: "bob", DisplayName: "Bob", Secret: "bob passphrase for tests"},
	"carol": {ID: "00000000-0000-4000-8000-000000000002", Contact: "carol@example.com", Handle: "carol", DisplayName: "Carol", Secret: "carol passphrase for tests"},
}

var (
	presetHashesOnce sync.Once
	presetHashes     map[string]domain.SecretHash
	presetHashesErr  error
)

// presetHash は、あらかじめ置く利用者の、保管する形の秘密を返す。計算は、テストのプロセスで、1回だけ行う。
func presetHash(t *testing.T, name string) domain.SecretHash {
	t.Helper()
	presetHashesOnce.Do(func() {
		presetHashes = map[string]domain.SecretHash{}
		for name, a := range presetAccounts {
			secret, err := domain.NewSecret(a.Secret, domain.RestoreContact(a.Contact), a.Handle)
			if err != nil {
				presetHashesErr = err
				return
			}
			hash, err := testHasher.Hash(context.Background(), secret)
			if err != nil {
				presetHashesErr = err
				return
			}
			presetHashes[name] = hash
		}
	})
	if presetHashesErr != nil {
		t.Fatal(presetHashesErr)
	}
	return presetHashes[name]
}

// ensurePreset は、あらかじめ置く利用者の行が、保管先にあるようにする。
func (s *testServer) ensurePreset(name string) account {
	s.t.Helper()
	a, ok := presetAccounts[name]
	if !ok {
		s.t.Fatalf("あらかじめ置く利用者ではない: %q", name)
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO users (id, contact, handle, display_name, secret_hash) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO NOTHING`,
		a.ID, a.Contact, a.Handle, a.DisplayName, string(presetHash(s.t, name)))
	if err != nil {
		s.t.Fatal(err)
	}
	return a
}

// browser は、1つのブラウザが持つ、ログイン状態である。Cookie(資格)と、ログイン状態の識別子を持つ。
// 値を持たない browser は、何も添えない要求を送る。
type browser struct {
	Cookie    string // 資格(Cookie chat_session の値)
	SessionID string
}

// header は、要求に添えるヘッダーを返す。
func (b *browser) header() map[string]string {
	h := map[string]string{"Content-Type": "application/json"}
	if b.Cookie != "" {
		h["Cookie"] = sessionCookieName + "=" + b.Cookie
	}
	if b.SessionID != "" {
		h["X-Session-Id"] = b.SessionID
	}
	return h
}

// sessionCookieName は、資格を載せる Cookie の名前である。
const sessionCookieName = "chat_session"

// loginResponse は、ログインの要求を送り、応答を返す。cookie が空でなければ、要求に付ける
// (ログイン状態を持つブラウザでのログイン)。
func (s *testServer) loginResponse(contact, secret, cookie string) response {
	s.t.Helper()
	body, err := json.Marshal(map[string]string{"contact": contact, "secret": secret})
	if err != nil {
		s.t.Fatal(err)
	}
	header := map[string]string{"Content-Type": "application/json"}
	if cookie != "" {
		header["Cookie"] = sessionCookieName + "=" + cookie
	}
	return s.do(http.MethodPost, "/api/session", header, body)
}

// sessionCookieOf は、応答が与えた、資格の Cookie を返す。なければ、nil を返す。
func sessionCookieOf(resp response) *http.Cookie {
	for _, c := range (&http.Response{Header: resp.Header}).Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	return nil
}

// mustLogin は、ログインが完了することを確かめ、新しいブラウザを返す。
func (s *testServer) mustLogin(contact, secret string) *browser {
	s.t.Helper()
	return s.mustLoginIn(contact, secret, "")
}

// mustLoginIn は、cookie を持つブラウザで、ログインする。
func (s *testServer) mustLoginIn(contact, secret, cookie string) *browser {
	s.t.Helper()
	resp := s.loginResponse(contact, secret, cookie)
	if resp.Status != http.StatusOK {
		s.t.Fatalf("ログインが完了しなかった: %d %s", resp.Status, resp.Body)
	}
	var out openapi.Session
	decodeStrict(s.t, resp.Body, &out)
	c := sessionCookieOf(resp)
	if c == nil || c.Value == "" || out.SessionId == "" {
		s.t.Fatalf("資格か、ログイン状態の識別子が、与えられていない: %v %s", resp.Header, resp.Body)
	}
	return &browser{Cookie: c.Value, SessionID: out.SessionId}
}

// newBrowser は、あらかじめ置く利用者として、新しいブラウザで、ログインする。
func (s *testServer) newBrowser(name string) *browser {
	s.t.Helper()
	a := s.ensurePreset(name)
	return s.mustLogin(a.Contact, a.Secret)
}

// as は、名前に当たる、既定のブラウザを返す。あらかじめ置く利用者(alice、bob、carol)は、最初に使うときに、
// 行を作って、ログインする。そのほかの名前(空を含む)は、何も添えないブラウザになる
// (ログイン状態がない要求を送るために使う)。
func (s *testServer) as(name string) *browser {
	s.t.Helper()
	if _, ok := presetAccounts[name]; !ok {
		return &browser{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.browsers[name]; ok {
		return b
	}
	b := s.newBrowser(name)
	s.browsers[name] = b
	return b
}

// auth は、名前に当たる、既定のブラウザの、要求に添えるヘッダーを返す。
func (s *testServer) auth(name string) map[string]string {
	s.t.Helper()
	return s.as(name).header()
}

// requestSignup は、登録の申し込みの要求を送る。
func (s *testServer) requestSignup(contact string) response {
	s.t.Helper()
	body, err := json.Marshal(map[string]string{"contact": contact})
	if err != nil {
		s.t.Fatal(err)
	}
	return s.do(http.MethodPost, "/api/signups", map[string]string{"Content-Type": "application/json"}, body)
}

// mustRequestSignup は、申し込みが完了することを確かめ、その連絡先に送った、最新の案内の資格を返す。
// normalized は、揃えた後の連絡先である。
func (s *testServer) mustRequestSignup(contact, normalized string) string {
	s.t.Helper()
	resp := s.requestSignup(contact)
	if resp.Status != http.StatusNoContent || len(resp.Body) != 0 {
		s.t.Fatalf("申し込みが完了しなかった: %d %s", resp.Status, resp.Body)
	}
	notices := s.notices.to(normalized)
	if len(notices) == 0 || notices[len(notices)-1].Credential == "" {
		s.t.Fatalf("登録を続けるための案内が、送られていない: %+v", s.notices.all())
	}
	return notices[len(notices)-1].Credential
}

// completeSignup は、登録の完了の要求を送る。
func (s *testServer) completeSignup(credential, handle, displayName, secret string) response {
	s.t.Helper()
	body, err := json.Marshal(map[string]string{"signupCredential": credential, "handle": handle, "displayName": displayName, "secret": secret})
	if err != nil {
		s.t.Fatal(err)
	}
	return s.do(http.MethodPost, "/api/users", map[string]string{"Content-Type": "application/json"}, body)
}

// mustRegister は、申し込みから、案内の資格での完了までを行う。連絡先は、揃えた後の形で指定する。
func (s *testServer) mustRegister(a account) {
	s.t.Helper()
	credential := s.mustRequestSignup(a.Contact, a.Contact)
	resp := s.completeSignup(credential, a.Handle, a.DisplayName, a.Secret)
	if resp.Status != http.StatusNoContent || len(resp.Body) != 0 {
		s.t.Fatalf("登録が完了しなかった: %d %s", resp.Status, resp.Body)
	}
}

// findUser は、b として、相手を特定する要求を送る。
func (s *testServer) findUser(b *browser, handle string) response {
	s.t.Helper()
	return s.do(http.MethodGet, "/api/users/"+handle, b.header(), nil)
}

// logout は、b として、ログアウトの要求を送る。
func (s *testServer) logout(b *browser) response {
	s.t.Helper()
	return s.do(http.MethodDelete, "/api/session", b.header(), nil)
}

// getSession は、cookie だけを添えて、現在のログイン状態を確かめる要求を送る。
func (s *testServer) getSession(cookie string) response {
	s.t.Helper()
	header := map[string]string{}
	if cookie != "" {
		header["Cookie"] = sessionCookieName + "=" + cookie
	}
	return s.do(http.MethodGet, "/api/session", header, nil)
}

// response は、HTTP の応答である。
type response struct {
	Status int
	Header http.Header
	Body   []byte
}

// do は、サーバーへ、HTTP の要求を送る。
func (s *testServer) do(method, path string, header map[string]string, body []byte) response {
	s.t.Helper()
	req, err := http.NewRequest(method, s.url+path, bytes.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	for name, value := range header {
		req.Header.Set(name, value)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return response{Status: resp.StatusCode, Header: resp.Header, Body: data}
}

func messagesPath(peer string) string {
	return "/api/direct-conversations/" + peer + "/messages"
}

// sendRaw は、actor として、peer へ、要求の本文をそのまま送る。
func (s *testServer) sendRaw(actor, peer string, rawBody []byte) response {
	s.t.Helper()
	return s.do(http.MethodPost, messagesPath(peer), s.auth(actor), rawBody)
}

// sendFrom は、b として、peer へ、メッセージを送る。
func (s *testServer) sendFrom(b *browser, peer, body string) response {
	s.t.Helper()
	raw, err := json.Marshal(map[string]string{"sendId": newSendID(s.t), "body": body})
	if err != nil {
		s.t.Fatal(err)
	}
	return s.do(http.MethodPost, messagesPath(peer), b.header(), raw)
}

// historyFrom は、b として、peer との会話の履歴を取得する。
func (s *testServer) historyFrom(b *browser, peer string) response {
	s.t.Helper()
	return s.do(http.MethodGet, messagesPath(peer), b.header(), nil)
}

// sendBodyJSON は、本文の項目を、JSON の表現のまま指定して送る。
// 対にならないサロゲートのエスケープなど、文字列として正しくない本文を送るために使う。
func (s *testServer) sendBodyJSON(actor, peer, sendID, bodyJSON string) response {
	s.t.Helper()
	return s.sendRaw(actor, peer, []byte(`{"sendId":"`+sendID+`","body":`+bodyJSON+`}`))
}

// send は、actor として、peer へ、メッセージを送る。
func (s *testServer) send(actor, peer, sendID, body string) response {
	s.t.Helper()
	raw, err := json.Marshal(map[string]string{"sendId": sendID, "body": body})
	if err != nil {
		s.t.Fatal(err)
	}
	return s.sendRaw(actor, peer, raw)
}

// mustSend は、送信が完了することを確かめ、保管されたメッセージを返す。
func (s *testServer) mustSend(actor, peer, body string) openapi.Message {
	s.t.Helper()
	return s.mustSendWithID(actor, peer, newSendID(s.t), body)
}

func (s *testServer) mustSendWithID(actor, peer, sendID, body string) openapi.Message {
	s.t.Helper()
	resp := s.send(actor, peer, sendID, body)
	if resp.Status != http.StatusOK {
		s.t.Fatalf("送信が完了しなかった: %d %s", resp.Status, resp.Body)
	}
	var out openapi.SendMessageResponse
	decodeStrict(s.t, resp.Body, &out)
	return out.Message
}

// history は、actor として、peer との会話の履歴を取得する。
func (s *testServer) history(actor, peer string) response {
	s.t.Helper()
	return s.do(http.MethodGet, messagesPath(peer), s.auth(actor), nil)
}

// mustHistory は、履歴の取得が完了することを確かめ、メッセージの一覧を返す。
func (s *testServer) mustHistory(actor, peer string) []openapi.Message {
	s.t.Helper()
	resp := s.history(actor, peer)
	if resp.Status != http.StatusOK {
		s.t.Fatalf("履歴を取得できなかった: %d %s", resp.Status, resp.Body)
	}
	var out openapi.MessageList
	decodeStrict(s.t, resp.Body, &out)
	if out.Messages == nil {
		s.t.Fatalf("messages が、配列ではない: %s", resp.Body)
	}
	return out.Messages
}

// count は、保管先の表の行の数を、直接に調べる。
func (s *testServer) count(table string) int {
	s.t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// requireStored は、保管先の、会話とメッセージの行の数を確かめる。
func (s *testServer) requireStored(conversations, messages int) {
	s.t.Helper()
	if got := s.count("conversations"); got != conversations {
		s.t.Fatalf("会話の行の数が違う: got %d, want %d", got, conversations)
	}
	if got := s.count("messages"); got != messages {
		s.t.Fatalf("メッセージの行の数が違う: got %d, want %d", got, messages)
	}
}

// decodeStrict は、JSON を、未知の項目を許さずに復号する。
func decodeStrict(t *testing.T, data []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("応答を復号できない: %v: %s", err, data)
	}
}

// requireError は、応答が、指定した状態と code の、拒否またはエラーの応答であることを確かめる。
// 応答は、code だけを持つ(WS-30)。
func requireError(t *testing.T, resp response, status int, code string) {
	t.Helper()
	if resp.Status != status {
		t.Fatalf("状態が違う: got %d, want %d (%s)", resp.Status, status, resp.Body)
	}
	if got, want := string(resp.Body), `{"code":"`+code+`"}`; got != want {
		t.Fatalf("応答の本文が違う: got %s, want %s", got, want)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type が違う: %q", got)
	}
}

// newSendID は、送信の識別子(UUID の形の文字列)を作る。
func newSendID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// wsClient は、WebSocket の接続(端末)である。
type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

// wsEvent は、端末が受け取ったイベントである。
type wsEvent struct {
	Type    string
	SendID  string
	Message openapi.Message
	Raw     []byte          // イベントの全体
	RawMsg  json.RawMessage // message の項目
}

// realtimeURLOf は、b の、WebSocket の接続先を返す。ログイン状態の識別子を、クエリで添える。
func (s *testServer) realtimeURLOf(b *browser) string {
	u := "ws" + strings.TrimPrefix(s.url, "http") + "/api/realtime"
	if b.SessionID != "" {
		u += "?session=" + b.SessionID
	}
	return u
}

// realtimeURL は、名前に当たる、既定のブラウザの、接続先を返す。
func (s *testServer) realtimeURL(user string) string {
	return s.realtimeURLOf(s.as(user))
}

// dialAs は、b として、WebSocket の接続を確立する要求を送る。資格は、Cookie で添える。
// origin が空でなければ、Origin ヘッダーを付ける。
func (s *testServer) dialAs(b *browser, origin string) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	if b.Cookie != "" {
		header.Set("Cookie", sessionCookieName+"="+b.Cookie)
	}
	return websocket.Dial(ctx, s.realtimeURLOf(b), &websocket.DialOptions{HTTPClient: s.http, HTTPHeader: header})
}

// dial は、名前に当たる、既定のブラウザとして、接続を確立する要求を送る。
func (s *testServer) dial(user, origin string) (*websocket.Conn, *http.Response, error) {
	return s.dialAs(s.as(user), origin)
}

// connect は、user として接続を確立し、受信を開始できたことの知らせ(ready)を受けるまで待つ。
// 戻った時点で、この接続は、配信の宛先として登録されている(ADR-010)。
func (s *testServer) connect(user string) *wsClient {
	s.t.Helper()
	return s.connectAs(s.as(user))
}

// connectAs は、b として接続を確立する。同じ b での複数の接続は、同じブラウザの、複数のタブに当たる。
func (s *testServer) connectAs(b *browser) *wsClient {
	s.t.Helper()
	conn, _, err := s.dialAs(b, testAppOrigin)
	if err != nil {
		s.t.Fatalf("接続できない: %v", err)
	}
	conn.SetReadLimit(1 << 20)
	c := &wsClient{t: s.t, conn: conn}
	s.t.Cleanup(func() { _ = conn.CloseNow() })

	first := c.next()
	if first.Type != "ready" || string(first.Raw) != `{"type":"ready"}` {
		s.t.Fatalf("最初のイベントが、ready ではない: %s", first.Raw)
	}
	return c
}

// requireRejectedDial は、b としての接続の確立が、指定した状態と code で、拒否されることを確かめる。
func (s *testServer) requireRejectedDial(b *browser, status int, code string) {
	s.t.Helper()
	conn, resp, err := s.dialAs(b, testAppOrigin)
	if err == nil {
		_ = conn.CloseNow()
		s.t.Fatal("接続が、確立された")
	}
	if resp == nil || resp.StatusCode != status {
		s.t.Fatalf("接続の確立の応答が違う: %v", resp)
	}
	data, _ := io.ReadAll(resp.Body)
	if string(data) != `{"code":"`+code+`"}` {
		s.t.Fatalf("接続の確立の応答の本文が違う: %s", data)
	}
}

// statusSessionEnded は、ログイン状態が無効になった接続を閉じる、終了コードである(ADR-014)。
const statusSessionEnded websocket.StatusCode = 4401

// requireSessionEnded は、接続が、次に、ログイン状態が無効になったことを示す終了コードで
// 閉じられることを確かめる。閉じられる前に、メッセージが届いた場合は、失敗とする。
func (c *wsClient) requireSessionEnded() {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()
	_, data, err := c.conn.Read(ctx)
	if err == nil {
		c.t.Fatalf("接続が閉じられる前に、イベントが届いた: %s", data)
	}
	var closeErr websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != statusSessionEnded {
		c.t.Fatalf("終了コードが違う: %v", err)
	}
}

// next は、次のイベントを待って返す。
func (c *wsClient) next() wsEvent {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()

	typ, data, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("次のイベントを受け取れない: %v", err)
	}
	if typ != websocket.MessageText {
		c.t.Fatalf("テキストのメッセージではない: %v", typ)
	}

	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		c.t.Fatalf("イベントを復号できない: %v: %s", err, data)
	}
	ev := wsEvent{Type: head.Type, Raw: data}
	if head.Type == "message" {
		var body struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
			SendID  string          `json:"sendId"`
		}
		decodeStrict(c.t, data, &body)
		ev.SendID = body.SendID
		ev.RawMsg = body.Message
		decodeStrict(c.t, body.Message, &ev.Message)
	}
	return ev
}

// requireNext は、端末が次に受け取るイベントが、want のメッセージであることを確かめる。
//
// 「配信されない」ことの判定に使う(番兵)。Deliver は、送信待ちの列に加えるまでを、送信の完了の
// 応答より前に終える。列は、加えた順に届く。そこで、届かないはずのメッセージMの後に、届くはずの
// 番兵Sを送り、次に受け取るのがSであることを確かめる。Mが届いていれば、Sより先に現れる。
func (c *wsClient) requireNext(want openapi.Message) wsEvent {
	c.t.Helper()
	ev := c.next()
	if ev.Type != "message" {
		c.t.Fatalf("メッセージのイベントではない: %s", ev.Raw)
	}
	if !sameMessage(ev.Message, want) {
		c.t.Fatalf("次に受け取ったイベントが違う:\n got %s\nwant %+v", ev.Raw, want)
	}
	return ev
}

func (c *wsClient) close() {
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
}

// sameMessage は、2つのメッセージが、すべての項目で一致するかを返す。
func sameMessage(a, b openapi.Message) bool {
	return canonicalJSON(a) == canonicalJSON(b)
}

func canonicalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// forbiddenInResponses は、応答に含まれてはならない文字列である(WS-30)。
// 呼び出しの経路、保管先の構造や問い合わせの内容、ファイルの場所、使用しているソフトウェアを表す。
var forbiddenInResponses = []string{
	"goroutine", "panic", "runtime.", ".go:", ".go ", "github.com", "golang", "net/http",
	"internal/", "usecase", "adapter", "pgx", "pgconn", "postgres", "PostgreSQL", "SQLSTATE",
	"SELECT", "INSERT", "conversations", "messages_", "/home/", "/usr/", "coder", "oapi", "openapi",
	"json:", "unmarshal", "Unmarshal",
}

// requireNoInternals は、応答が、内部の構造が分かる情報を含まないことを確かめる。
func requireNoInternals(t *testing.T, resp response) {
	t.Helper()
	for _, s := range forbiddenInResponses {
		if bytes.Contains(resp.Body, []byte(s)) {
			t.Fatalf("応答の本文が、内部の情報(%q)を含む: %s", s, resp.Body)
		}
	}
	for _, name := range []string{"Server", "X-Powered-By", "Via"} {
		if values := resp.Header.Values(name); len(values) != 0 {
			t.Fatalf("応答のヘッダーに、ソフトウェアの情報がある: %s: %v", name, values)
		}
	}
	requireNoCORS(t, resp)
}

// requireNoCORS は、応答に、CORS のヘッダーがないことを確かめる(WS-29)。
func requireNoCORS(t *testing.T, resp response) {
	t.Helper()
	for name := range resp.Header {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") {
			t.Fatalf("応答に、CORS のヘッダーがある: %s", name)
		}
	}
}

// logLines は、ログの出力を、行ごとに復号して返す。
func (s *testServer) logLines() []map[string]any {
	s.t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(s.logs.String()), "\n") {
		if line == "" {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			s.t.Fatalf("ログの行を、復号できない: %v: %s", err, line)
		}
		lines = append(lines, v)
	}
	return lines
}

// eventsOf は、指定した種類の、出来事の記録を、記録された順に返す。
func (s *testServer) eventsOf(kind string) []map[string]any {
	s.t.Helper()
	var events []map[string]any
	for _, line := range s.logLines() {
		if line["msg"] == eventlog.Message && line["event"] == kind {
			events = append(events, line)
		}
	}
	return events
}

// userID は、連絡先に当たる利用者の、識別子を、保管先から、直接に調べる。
func (s *testServer) userID(contact string) string {
	s.t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(), "SELECT id::text FROM users WHERE contact = $1", contact).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}
