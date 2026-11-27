// Package smtpmail は、案内を、SMTP で送る adapter である。port.NoticeSender を実装する(ADR-018)。
//
// 暗号化と認証は、行わない。接続先は、設定の読み込みが、ループバックのアドレスに限る(cmd/server)。
// 案内は、書式を持たない、UTF-8 のテキストである。訪問者が入力した内容は、宛先のほかに、含めない(RL-11)。
// 返すエラーは、操作の名前と、SMTP の応答の番号だけを持つ形に、作り直す。相手が返した文言は、
// 宛先を含みうるためである(RL-33)。
package smtpmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// DefaultTimeout は、接続から、引き渡しの完了までを待つ時間の上限である。
const DefaultTimeout = 10 * time.Second

// SignupFragment は、登録を続けるためのアドレスの、# より後の、資格の前に置く部分である。
// # より後は、サーバーへ送られない(RL-33)。クライアントが、同じ形で読み取る。
const SignupFragment = "#signup="

// Sender は、案内を、SMTP のサーバーに引き渡す。
type Sender struct {
	addr      string // SMTP のサーバーの接続先(ホスト:ポート)
	from      string // 送信元のアドレス
	appOrigin string // アプリのオリジン。登録を続けるためのアドレスを、組み立てる
	timeout   time.Duration
}

// NewSender は、Sender を作る。
func NewSender(addr, from, appOrigin string) *Sender {
	return &Sender{addr: addr, from: from, appOrigin: appOrigin, timeout: DefaultTimeout}
}

var _ port.NoticeSender = (*Sender)(nil)

// SendError は、案内を、引き渡せなかったことを示す。
// 持つのは、操作の名前と、SMTP の応答の番号だけである。元のエラーは、保持しない。
type SendError struct {
	Op   string // 操作の名前
	Code int    // SMTP の応答の番号。応答によるエラーでない場合は、0
}

func (e *SendError) Error() string {
	if e.Code == 0 {
		return "smtpmail: " + e.Op + " failed"
	}
	return "smtpmail: " + e.Op + " failed (SMTP " + strconv.Itoa(e.Code) + ")"
}

func newSendError(op string, err error) error {
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) {
		return &SendError{Op: op, Code: protoErr.Code}
	}
	return &SendError{Op: op}
}

// SendSignupNotice は、登録を続けるための案内を送る。
func (s *Sender) SendSignupNotice(ctx context.Context, to domain.Contact, credential string, expiresAt time.Time) error {
	body := "登録の申し込みを受け付けました。\r\n" +
		"\r\n" +
		"次のアドレスを開いて、登録を続けてください。\r\n" +
		s.appOrigin + "/" + SignupFragment + credential + "\r\n" +
		"\r\n" +
		"このアドレスは、申し込みから24時間、有効です(" + expiresAt.UTC().Format("2006-01-02 15:04") + " UTC まで)。\r\n" +
		"\r\n" +
		"心当たりがない場合は、何もする必要はありません。\r\n"
	return s.send(ctx, to, "登録の確認", body)
}

// SendAlreadyRegisteredNotice は、登録済みであることを伝える案内を送る。資格は、含めない(RL-03)。
func (s *Sender) SendAlreadyRegisteredNotice(ctx context.Context, to domain.Contact) error {
	body := "この連絡先で、登録の申し込みがありました。\r\n" +
		"\r\n" +
		"この連絡先は、既に登録されています。\r\n" +
		"\r\n" +
		"心当たりがない場合は、何もする必要はありません。\r\n"
	return s.send(ctx, to, "登録の申し込みについて", body)
}

// send は、1通を、SMTP のサーバーに引き渡す。接続から、引き渡しの完了までに、時間の上限を設ける。
func (s *Sender) send(ctx context.Context, to domain.Contact, subject, body string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return newSendError("connect", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(deadline); err != nil {
		return newSendError("connect", err)
	}

	host, _, err := net.SplitHostPort(s.addr)
	if err != nil {
		return newSendError("connect", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return newSendError("greet", err)
	}
	if err := client.Mail(s.from); err != nil {
		return newSendError("mail from", err)
	}
	if err := client.Rcpt(to.String()); err != nil {
		return newSendError("rcpt to", err)
	}
	w, err := client.Data()
	if err != nil {
		return newSendError("data", err)
	}
	if _, err := w.Write(s.compose(to, subject, body)); err != nil {
		return newSendError("data", err)
	}
	// 引き渡しは、ここで完了する。
	if err := w.Close(); err != nil {
		return newSendError("data", err)
	}
	// 引き渡した後の、終わりの挨拶の失敗は、送れなかったこととしない。
	_ = client.Quit()
	return nil
}

// compose は、ヘッダーと本文を組み立てる。件名と本文は、標準ライブラリで符号化する。
// 宛先は、入力の規則を満たした連絡先であり、ヘッダーとして解釈される文字を含まない(ADR-018)。
func (s *Sender) compose(to domain.Contact, subject, body string) []byte {
	var msg bytes.Buffer
	header := func(name, value string) {
		msg.WriteString(name + ": " + value + "\r\n")
	}
	header("From", s.from)
	header("To", to.String())
	header("Subject", mime.BEncoding.Encode("UTF-8", subject))
	// Date は、メールの形式が求める、作成の日時である。期限などの判定には、使わない。
	header("Date", time.Now().UTC().Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "base64")
	msg.WriteString("\r\n")

	// Base64 の行は、76文字で折り返す。
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for len(encoded) > 76 {
		msg.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	msg.WriteString(encoded + "\r\n")
	return msg.Bytes()
}
