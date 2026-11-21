package main

// サーバーの振る舞いのテストの道具立て。
//
// 実際の adapter を使うサーバーを組み立て、httptest.Server で起動する。
// 保管先は、テストごとに、雛形から複製したデータベースである(make test-server)。
// 要求と接続は、クライアントを介さずに、サーバーへ直接に行う(WS-12、WS-26)。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
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

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/fixeduser"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres/pgtest"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/wsdelivery"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
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

type testServer struct {
	t    *testing.T
	url  string
	pool *pgxpool.Pool
	logs *syncBuffer
	http *http.Client
}

type serverOption func(*serverOptions)

type serverOptions struct {
	wrapTx func(port.Transactor) port.Transactor
}

// withTransactor は、トランザクションのポートを、差し替えた実装で包む(WS-31)。
func withTransactor(wrap func(port.Transactor) port.Transactor) serverOption {
	return func(o *serverOptions) { o.wrapTx = wrap }
}

// startServer は、実際の adapter を使うサーバーを、main と同じ関数で組み立てて、起動する。
func startServer(t *testing.T, opts ...serverOption) *testServer {
	t.Helper()
	var options serverOptions
	for _, opt := range opts {
		opt(&options)
	}

	pool := pgtest.NewDatabase(t)
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	store := postgres.NewStore(pool)
	hub := wsdelivery.NewHub(logger)
	var tx port.Transactor = store
	if options.wrapTx != nil {
		tx = options.wrapTx(store)
	}

	handler, err := newHandler(ports{
		Users:    fixeduser.NewDirectory(),
		Reader:   store,
		Tx:       tx,
		Delivery: hub,
		Registry: hub,
	}, testAppOrigin, logger)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewUnstartedServer(handler)
	ts.Config = newHTTPServer("", handler)
	ts.Start()
	t.Cleanup(ts.Close)

	client := ts.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	return &testServer{t: t, url: ts.URL, pool: pool, logs: logs, http: client}
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

func userHeader(actor string) map[string]string {
	if actor == "" {
		return map[string]string{"Content-Type": "application/json"}
	}
	return map[string]string{"Content-Type": "application/json", "X-Fixed-User": actor}
}

// sendRaw は、actor として、peer へ、要求の本文をそのまま送る。
func (s *testServer) sendRaw(actor, peer string, rawBody []byte) response {
	s.t.Helper()
	return s.do(http.MethodPost, messagesPath(peer), userHeader(actor), rawBody)
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
	header := map[string]string{}
	if actor != "" {
		header["X-Fixed-User"] = actor
	}
	return s.do(http.MethodGet, messagesPath(peer), header, nil)
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

func (s *testServer) realtimeURL(user string) string {
	u := "ws" + strings.TrimPrefix(s.url, "http") + "/api/realtime"
	if user != "" {
		u += "?user=" + user
	}
	return u
}

// dial は、user として、WebSocket の接続を確立する要求を送る。
// origin が空でなければ、Origin ヘッダーを付ける。
func (s *testServer) dial(user, origin string) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	return websocket.Dial(ctx, s.realtimeURL(user), &websocket.DialOptions{HTTPClient: s.http, HTTPHeader: header})
}

// connect は、user として接続を確立し、受信を開始できたことの知らせ(ready)を受けるまで待つ。
// 戻った時点で、この接続は、配信の宛先として登録されている(ADR-010)。
func (s *testServer) connect(user string) *wsClient {
	s.t.Helper()
	conn, _, err := s.dial(user, testAppOrigin)
	if err != nil {
		s.t.Fatalf("%s として接続できない: %v", user, err)
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
