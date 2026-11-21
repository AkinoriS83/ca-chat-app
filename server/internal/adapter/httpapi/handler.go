// Package httpapi は、HTTP の操作を受ける adapter である。ユースケースを呼ぶ(ADR-005)。
// 通信の形式(生成した型)と、ユースケースの入力・出力の間を、詰め替える。
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// 仮実装(WS-P2): 仮のログイン。利用者を添える、要求ヘッダーの名前。
const fixedUserHeader = "X-Fixed-User"

// maxRequestBodyBytes は、要求の本文の大きさの上限である(1 MiB)。
// 正しい送信の要求は、最大でも約48KBである(4,000文字が、すべてエスケープされた場合)。
const maxRequestBodyBytes = 1 << 20

// Handler は、生成した openapi.ServerInterface を実装する。
type Handler struct {
	sender  *usecase.MessageSender
	history *usecase.HistoryReader
	logger  *slog.Logger
}

// NewHandler は、Handler を作る。受け取るのは、ユースケースと、ログの出力先だけである。
func NewHandler(sender *usecase.MessageSender, history *usecase.HistoryReader, logger *slog.Logger) *Handler {
	return &Handler{sender: sender, history: history, logger: logger}
}

var _ openapi.ServerInterface = (*Handler)(nil)

// Register は、HTTP の操作を、mux に登録する。
func (h *Handler) Register(mux *http.ServeMux) {
	openapi.HandlerWithOptions(h, openapi.StdHTTPServerOptions{
		BaseRouter: mux,
		// 生成したコードの既定の処理は、エラーの文言を、そのまま応答に書く。置き換える(WS-30)。
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeError(w, openapi.InvalidInput)
		},
	})
}

// actorOf は、操作を行う利用者を、要求から取り出す。
// 添えられていない場合は、空の文字列を返す(usecase が、拒否する)。
//
// 仮実装(WS-P2): 仮のログイン。添えられた識別名を、そのまま使う。なりすましを防がない。
func actorOf(r *http.Request) string {
	return r.Header.Get(fixedUserHeader)
}

// ListDirectMessages は、「会話の履歴を読む」を受ける。
func (h *Handler) ListDirectMessages(w http.ResponseWriter, r *http.Request, peer string) {
	out, err := h.history.Read(r.Context(), usecase.ReadHistoryInput{Actor: actorOf(r), Peer: peer})
	if err != nil {
		h.writeUsecaseError(w, "list direct messages", err)
		return
	}

	// 仮実装(WS-P4): 会話のすべてのメッセージを、一度に返す。
	messages := make([]openapi.Message, 0, len(out.Messages))
	for _, m := range out.Messages {
		messages = append(messages, toMessage(m))
	}
	writeJSON(w, http.StatusOK, openapi.MessageList{Messages: messages})
}

// SendDirectMessage は、「メッセージを送る」を受ける。
func (h *Handler) SendDirectMessage(w http.ResponseWriter, r *http.Request, peer string) {
	sendID, body, ok := decodeSendMessageRequest(w, r)
	if !ok {
		writeError(w, openapi.InvalidInput)
		return
	}

	out, err := h.sender.Send(r.Context(), usecase.SendMessageInput{Actor: actorOf(r), Peer: peer, SendID: sendID, Body: body})
	if err != nil {
		h.writeUsecaseError(w, "send direct message", err)
		return
	}
	writeJSON(w, http.StatusOK, openapi.SendMessageResponse{Message: toMessage(out.Message)})
}

// decodeSendMessageRequest は、送信の要求の本文を復号する。
// 形式に合わない場合(JSON として壊れている、必須の項目がない、項目の型が違う、
// 大きさの上限を超える)は、ok に false を返す。
//
// 本文(body)は、置き換えずに復号し、正しくない並びを、正しくないまま返す。
// 本文の値は、ここでは拒否しない。判定は、domain が行う(ADR-003)。
func decodeSendMessageRequest(w http.ResponseWriter, r *http.Request) (sendID, body string, ok bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err != nil {
		return "", "", false
	}

	// json.Unmarshal は、JSON の値の後ろに余分な内容がある場合も、エラーにする。未知の項目は、無視する。
	var req struct {
		SendID *string         `json:"sendId"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return "", "", false
	}
	if req.SendID == nil || req.Body == nil {
		return "", "", false
	}

	// body が、JSON の文字列でない場合(数値、null など)は、形式に合わない。
	body, ok = decodeJSONString(bytes.TrimSpace(req.Body))
	if !ok {
		return "", "", false
	}
	return *req.SendID, body, true
}

// writeUsecaseError は、ユースケースが返したエラーを、応答にする。
// 拒否は、種類に対応する code を返す。拒否でないエラーは、internal(500)を返し、ログに出力する。
func (h *Handler) writeUsecaseError(w http.ResponseWriter, operation string, err error) {
	var rejected *usecase.RejectedError
	if errors.As(err, &rejected) {
		writeError(w, codeOf(rejected.Kind))
		return
	}

	// 出力するのは、操作の名前と、エラーだけである。要求の本文と、メッセージの本文は、出力しない(WS-33)。
	// 保管先の adapter は、行の内容を含まないエラーに、作り直して返す。
	h.logger.Error("operation failed", "operation", operation, "error", err.Error())
	writeError(w, openapi.Internal)
}

// codeOf は、拒否の種類に対応する code を返す。
func codeOf(kind usecase.Rejection) openapi.ErrorCode {
	switch kind {
	case usecase.Unauthenticated:
		return openapi.Unauthenticated
	case usecase.InvalidBody:
		return openapi.InvalidBody
	case usecase.InvalidInput:
		return openapi.InvalidInput
	case usecase.InvalidPeer:
		return openapi.InvalidPeer
	case usecase.NotFound:
		return openapi.NotFound
	default:
		return openapi.Internal
	}
}

// toMessage は、ユースケースの出力を、通信の形式に詰め替える。
// 同じ変換が、配信の adapter(wsdelivery)にもある。2つの adapter は、互いに import できないためである。
func toMessage(m usecase.MessageView) openapi.Message {
	return openapi.Message{
		Participants: m.Participants,
		Seq:          m.Seq,
		Sender:       openapi.UserProfile{Handle: m.SenderHandle, DisplayName: m.SenderDisplayName},
		Body:         m.Body,
		SentAt:       m.SentAt.UTC(),
	}
}
