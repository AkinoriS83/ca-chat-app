// Package httpapi は、HTTP の操作を受ける adapter である。ユースケースを呼ぶ(ADR-005)。
// 通信の形式(生成した型)と、ユースケースの入力・出力の間を、詰め替える。
//
// ユースケースが返した拒否とエラーは、応答に置き換えるだけとする。ログには、出力しない。
// 出来事の記録は、usecase が行う(ADR-021)。
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// ログイン状態の添え方(ADR-014)。資格は、Cookie で、ログイン状態の識別子は、要求ヘッダーで受ける。
// Cookie の名前は、wsaccept にも、同じ値の定数がある。2つの adapter は、互いに import できないためである。
const (
	sessionCookie   = "chat_session"
	sessionIDHeader = "X-Session-Id"
)

const (
	// maxRequestBodyBytes は、送信の要求の本文の大きさの上限である(1 MiB)。
	// 正しい送信の要求は、最大でも約48KBである(4,000文字が、すべてエスケープされた場合)。
	maxRequestBodyBytes = 1 << 20
	// maxAccountBodyBytes は、登録とログインの要求の本文の大きさの上限である(16 KiB)。
	// 正しい要求は、最大でも約3KBである(128文字の秘密と、50文字の表示名が、すべてエスケープされた場合)。
	// 照合の計算に渡す入力の大きさも、これで限られる。
	maxAccountBodyBytes = 16 << 10
)

// Usecases は、Handler が呼ぶユースケースである。
type Usecases struct {
	SignupRequester *usecase.SignupRequester
	SignupCompleter *usecase.SignupCompleter
	Authenticator   *usecase.Authenticator
	SessionKeeper   *usecase.SessionKeeper
	PeerFinder      *usecase.PeerFinder
	MessageSender   *usecase.MessageSender
	HistoryReader   *usecase.HistoryReader
}

// Handler は、生成した openapi.ServerInterface を実装する。
type Handler struct {
	uc Usecases
}

// NewHandler は、Handler を作る。受け取るのは、ユースケースだけである。
func NewHandler(uc Usecases) *Handler {
	return &Handler{uc: uc}
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

// actorOf は、操作を行う利用者(資格と、ログイン状態の識別子)を、要求から取り出す。
// 添えられていない場合は、空の文字列を渡す。確かめるのは、usecase である(ADR-019)。
func actorOf(r *http.Request) usecase.Actor {
	return usecase.Actor{Credential: credentialOf(r), SessionID: r.Header.Get(sessionIDHeader)}
}

// credentialOf は、要求に付いた、ログイン状態を示す資格を返す。なければ、空の文字列を返す。
func credentialOf(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// sourceOf は、送信元(要求の接続の相手のアドレス)を返す。中継が付けるヘッダー(X-Forwarded-For など)は、
// 読まない。同じ処理が、wsaccept にもある。
func sourceOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RequestSignup は、「登録を申し込む」を受ける。
func (h *Handler) RequestSignup(w http.ResponseWriter, r *http.Request) {
	fields, ok := decodeStringFields(w, r, maxAccountBodyBytes, "contact")
	if !ok {
		writeError(w, openapi.InvalidInput)
		return
	}
	err := h.uc.SignupRequester.Request(r.Context(), usecase.RequestSignupInput{Contact: fields["contact"], Source: sourceOf(r)})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	writeNoContent(w)
}

// CompleteSignup は、「登録を完了する」を受ける。ログイン状態は、与えない。
func (h *Handler) CompleteSignup(w http.ResponseWriter, r *http.Request) {
	fields, ok := decodeStringFields(w, r, maxAccountBodyBytes, "signupCredential", "handle", "displayName", "secret")
	if !ok {
		writeError(w, openapi.InvalidInput)
		return
	}
	err := h.uc.SignupCompleter.Complete(r.Context(), usecase.CompleteSignupInput{
		Credential:  fields["signupCredential"],
		Handle:      fields["handle"],
		DisplayName: fields["displayName"],
		Secret:      fields["secret"],
		Source:      sourceOf(r),
	})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	writeNoContent(w)
}

// Login は、「ログインする」を受ける。
// 新しく与えた資格は、Cookie にだけ載せる。応答の本文には、識別子と利用者だけを載せる(ADR-014)。
// 拒否とエラーの応答では、Cookie を変えない。
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	fields, ok := decodeStringFields(w, r, maxAccountBodyBytes, "contact", "secret")
	if !ok {
		writeError(w, openapi.InvalidInput)
		return
	}
	out, err := h.uc.Authenticator.Login(r.Context(), usecase.LoginInput{
		Credential: credentialOf(r),
		Contact:    fields["contact"],
		Secret:     fields["secret"],
		Source:     sourceOf(r),
	})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	cookie := newSessionCookie(out.Credential)
	cookie.Expires = out.ExpiresAt.UTC()
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusOK, openapi.Session{SessionId: out.SessionID, User: toUserProfile(out.User)})
}

// GetSession は、「現在のログイン状態を確かめる」を受ける。Cookie だけを見る。
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.SessionKeeper.Current(r.Context(), usecase.CurrentSessionInput{Credential: credentialOf(r), Source: sourceOf(r)})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	status := openapi.SessionStatus{Authenticated: out.Authenticated}
	if out.Authenticated {
		status.Session = &openapi.Session{SessionId: out.SessionID, User: toUserProfile(out.User)}
	}
	writeJSON(w, http.StatusOK, status)
}

// Logout は、「ログアウトする」を受ける。完了では、同じ名前と属性で、Cookie を消す。
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.SessionKeeper.Logout(r.Context(), usecase.LogoutInput{Actor: actorOf(r), Source: sourceOf(r)}); err != nil {
		writeUsecaseError(w, err)
		return
	}
	cookie := newSessionCookie("")
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	writeNoContent(w)
}

// newSessionCookie は、資格を載せる Cookie を作る。属性は、ADR-014 の表の通りである。
// Secure と、名前の __Host- 接頭辞は、ローカルの開発環境では、付けない(経路を暗号化しないため。RL-D1)。
// Domain は、付けない(送る先を、アプリのホストに限る)。
func newSessionCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

// FindUser は、「相手を特定する」を受ける。
func (h *Handler) FindUser(w http.ResponseWriter, r *http.Request, handle string) {
	out, err := h.uc.PeerFinder.Find(r.Context(), usecase.FindPeerInput{Actor: actorOf(r), Source: sourceOf(r), Handle: handle})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserProfile(out.Peer))
}

// ListDirectMessages は、「会話の履歴を読む」を受ける。
func (h *Handler) ListDirectMessages(w http.ResponseWriter, r *http.Request, peer string) {
	out, err := h.uc.HistoryReader.Read(r.Context(), usecase.ReadHistoryInput{Actor: actorOf(r), Source: sourceOf(r), Peer: peer})
	if err != nil {
		writeUsecaseError(w, err)
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
// 形式に合わない要求は、ここでは拒否せず、印を付けて、ユースケースを呼ぶ。
// ログイン状態の確認を、形式の確認より先に行うためである(RL-15)。
func (h *Handler) SendDirectMessage(w http.ResponseWriter, r *http.Request, peer string) {
	fields, ok := decodeStringFields(w, r, maxRequestBodyBytes, "sendId", "body")

	out, err := h.uc.MessageSender.Send(r.Context(), usecase.SendMessageInput{
		Actor:     actorOf(r),
		Source:    sourceOf(r),
		Peer:      peer,
		SendID:    fields["sendId"],
		Body:      fields["body"],
		Malformed: !ok,
	})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, openapi.SendMessageResponse{Message: toMessage(out.Message)})
}

// decodeStringFields は、要求の本文(JSON のオブジェクト)から、文字列の項目を復号する。
// 形式に合わない場合(JSON として壊れている、必須の項目がない、項目が文字列でない、
// 大きさの上限を超える)は、ok に false を返す。未知の項目は、無視する。
//
// 文字列は、どの項目も、置き換えずに復号し、正しくない並びを、正しくないまま返す。
// 値は、ここでは拒否しない。判定は、domain が行う(ADR-003、ADR-015)。
func decodeStringFields(w http.ResponseWriter, r *http.Request, maxBytes int64, names ...string) (fields map[string]string, ok bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if err != nil {
		return nil, false
	}

	// json.Unmarshal は、JSON の値の後ろに余分な内容がある場合も、エラーにする。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return nil, false
	}

	fields = make(map[string]string, len(names))
	for _, name := range names {
		value, present := raw[name]
		if !present {
			return nil, false
		}
		// JSON の文字列でない場合(数値、null など)は、形式に合わない。
		decoded, ok := decodeJSONString(bytes.TrimSpace(value))
		if !ok {
			return nil, false
		}
		fields[name] = decoded
	}
	return fields, true
}

// writeUsecaseError は、ユースケースが返したエラーを、応答にする。
// 拒否は、種類に対応する code を返す。拒否でないエラーは、internal(500)を返す。
// ログには、出力しない。完了できなかった操作は、usecase が記録する(ADR-021)。
func writeUsecaseError(w http.ResponseWriter, err error) {
	var rejected *usecase.RejectedError
	if !errors.As(err, &rejected) {
		writeError(w, openapi.Internal)
		return
	}
	if rejected.Kind == usecase.InvalidFields {
		fields := make([]openapi.InputField, 0, len(rejected.Fields))
		for _, f := range rejected.Fields {
			fields = append(fields, fieldOf(f))
		}
		writeJSON(w, statusOf(openapi.InvalidFields), openapi.ErrorResponse{Code: openapi.InvalidFields, Fields: &fields})
		return
	}
	writeError(w, codeOf(rejected.Kind))
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
	case usecase.InvalidFields:
		return openapi.InvalidFields
	case usecase.RateLimited:
		return openapi.RateLimited
	case usecase.InvalidSignupCredential:
		return openapi.InvalidSignupCredential
	case usecase.HandleTaken:
		return openapi.HandleTaken
	case usecase.LoginFailed:
		return openapi.LoginFailed
	default:
		return openapi.Internal
	}
}

// fieldOf は、入力の項目を、通信の形式に詰め替える。
func fieldOf(field usecase.InputField) openapi.InputField {
	switch field {
	case usecase.FieldContact:
		return openapi.Contact
	case usecase.FieldHandle:
		return openapi.Handle
	case usecase.FieldDisplayName:
		return openapi.DisplayName
	default:
		return openapi.Secret
	}
}

// toUserProfile は、ユースケースの出力を、通信の形式に詰め替える。識別名と表示名だけを持つ(RL-27)。
func toUserProfile(u usecase.UserView) openapi.UserProfile {
	return openapi.UserProfile{Handle: u.Handle, DisplayName: u.DisplayName}
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
