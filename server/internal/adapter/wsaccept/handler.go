// Package wsaccept は、WebSocket の接続を受け付ける adapter である。ユースケースを呼ぶ(ADR-005)。
//
// 接続を確立する要求を受け、ユースケースを呼んで、ログイン状態を確かめる。受信を開始できたことを、
// 端末に知らせる(ADR-010)。接続が終わったことを、ユースケースに伝える。
// 配信の宛先を保持する状態は、持たない(配信の adapter が持つ)。
package wsaccept

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// Path は、WebSocket の接続先のパスである。
const Path = "/api/realtime"

// ログイン状態の添え方(ADR-014)。資格は、Cookie で、ログイン状態の識別子は、クエリで受ける。
// Cookie の名前は、httpapi にも、同じ値の定数がある。2つの adapter は、互いに import できないためである。
const (
	sessionCookie = "chat_session"
	sessionQuery  = "session"
)

// StatusSessionEnded は、ログイン状態が無効になった接続を閉じる、終了コードである。
// アプリが使える範囲(4000〜4999)の値である。
const StatusSessionEnded websocket.StatusCode = 4401

// writeTimeout は、端末への、1通の書き込みの時間の上限である。
const writeTimeout = 5 * time.Second

// Handler は、WebSocket の接続を確立する要求を受ける。
type Handler struct {
	keeper     *usecase.ConnectionKeeper
	originHost string
	logger     *slog.Logger
	ready      []byte
}

// NewHandler は、Handler を作る。appOrigin は、アプリのオリジン(例: http://localhost:5173)である。
func NewHandler(keeper *usecase.ConnectionKeeper, appOrigin string, logger *slog.Logger) (*Handler, error) {
	u, err := url.Parse(appOrigin)
	if err != nil || u.Host == "" {
		return nil, errors.New("wsaccept: invalid app origin")
	}
	ready, err := json.Marshal(openapi.ReadyEvent{Type: openapi.Ready})
	if err != nil {
		return nil, err
	}
	return &Handler{keeper: keeper, originHost: u.Host, logger: logger, ready: ready}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 書き込み口を作り、接続を確立する前に、ユースケースを呼ぶ。
	//    資格と、ログイン状態の識別子は、取り出して、そのまま渡す。確かめるのは、usecase である(ADR-019)。
	//    WebSocket の接続を確立する要求でなければ、拒否せずに、印を付けて渡す。ログイン状態の確認を、
	//    形式の確認より先に行うためである(RL-15)。
	s := newSink()
	err := h.keeper.Accept(r.Context(), usecase.AcceptConnectionInput{
		Actor:      usecase.Actor{Credential: cookieValue(r, sessionCookie), SessionID: r.URL.Query().Get(sessionQuery)},
		Source:     sourceOf(r),
		Malformed:  !headerHasToken(r.Header, "Upgrade", "websocket"),
		Connection: s,
	})
	if err != nil {
		// ユースケースが返した拒否とエラーは、応答に置き換えるだけとする。記録は、usecase が行う(ADR-021)。
		var rejected *usecase.RejectedError
		switch {
		case errors.As(err, &rejected) && rejected.Kind == usecase.Unauthenticated:
			writeError(w, http.StatusUnauthorized, openapi.Unauthenticated)
		case errors.As(err, &rejected) && rejected.Kind == usecase.InvalidInput:
			writeError(w, http.StatusBadRequest, openapi.InvalidInput)
		default:
			writeError(w, http.StatusInternalServerError, openapi.Internal)
		}
		return
	}
	// 2. 以後、どの経路で終わる場合も、登録を解く。
	defer h.keeper.Close(context.WithoutCancel(r.Context()), usecase.CloseConnectionInput{Connection: s})

	// 3. 接続を確立する。失敗した場合の応答は、ライブラリが書く。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{h.originHost}})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	// 4. 端末がデータのメッセージを送った場合と、接続が切れた場合に、ctx が取り消される。
	//    端末からは、何も送らない取り決めである。送った場合、ライブラリが、接続を閉じる。
	ctx := conn.CloseRead(r.Context())

	// 5. 受信を開始できたことを知らせる。登録(手順1)の後であり、ADR-010 の順を満たす。
	if !h.write(ctx, conn, h.ready) {
		return
	}

	// 6. 列から1通ずつ取り出して、書き込む。手順1から、ここまでの間に配信されたメッセージは、
	//    列に溜まっており、ここで届く。
	for {
		select {
		case data := <-s.queue:
			if !h.write(ctx, conn, data) {
				return
			}
		case <-s.closed:
			if s.sessionEnded.Load() {
				// ログイン状態が無効になった(ログアウト、ログインし直し、有効期限)。
				h.logger.Info("connection closed", "reason", "session_ended")
				_ = conn.Close(StatusSessionEnded, "")
				return
			}
			// 送信待ちの列が一杯になり、配信の adapter が閉じた。
			h.logger.Warn("connection closed", "reason", "send_queue_full")
			_ = conn.Close(websocket.StatusPolicyViolation, "")
			return
		case <-ctx.Done():
			return
		}
	}
}

// write は、1通を、時間の上限を設けて書き込む。失敗した場合は、false を返す。
// ログには、理由の区分だけを出力する(WS-33)。
func (h *Handler) write(ctx context.Context, conn *websocket.Conn, data []byte) bool {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	err := conn.Write(writeCtx, websocket.MessageText, data)
	if err == nil {
		return true
	}
	switch {
	case errors.Is(writeCtx.Err(), context.DeadlineExceeded):
		h.logger.Warn("connection closed", "reason", "write_timeout")
	case ctx.Err() != nil:
		// 接続が切れた。端末が閉じた場合を含むため、失敗としては記録しない。
	default:
		h.logger.Warn("connection closed", "reason", "write_failed")
	}
	return false
}

// cookieValue は、Cookie の値を返す。なければ、空の文字列を返す。
func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// sourceOf は、送信元(要求の接続の相手のアドレス)を返す。中継が付けるヘッダーは、読まない。
// 同じ処理が、httpapi にもある。
func sourceOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// headerHasToken は、ヘッダーの値(カンマ区切り)に、token が含まれるかを返す。大文字と小文字は、区別しない。
func headerHasToken(header http.Header, name, token string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// writeError は、接続を確立する前の拒否を、HTTP の応答として書く。
// 応答は、code だけを持つ(WS-30)。同じ形の応答が、httpapi にもある。
// 2つの adapter は、互いに import できないためである。
func writeError(w http.ResponseWriter, status int, code openapi.ErrorCode) {
	data, err := json.Marshal(openapi.ErrorResponse{Code: code})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
