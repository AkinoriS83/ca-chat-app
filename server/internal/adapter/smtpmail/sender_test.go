package smtpmail

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

const (
	testFrom   = "no-reply@chat.example"
	testOrigin = "http://localhost:5173"
)

// mailpit は、ローカルの開発環境の Mailpit である(ADR-018)。make db-up が起動する。
// 接続先は、環境変数(SMTP_ADDR、MAILPIT_UI_PORT)から読む。make test-server が、.env から与える。
type mailpit struct {
	smtpAddr string
	apiBase  string
}

func requireMailpit(t *testing.T) mailpit {
	t.Helper()
	smtpAddr, uiPort := os.Getenv("SMTP_ADDR"), os.Getenv("MAILPIT_UI_PORT")
	if smtpAddr == "" || uiPort == "" {
		t.Fatal("SMTP_ADDR と MAILPIT_UI_PORT が、与えられていません。make test-server を使ってください")
	}
	return mailpit{smtpAddr: smtpAddr, apiBase: "http://127.0.0.1:" + uiPort}
}

// uniqueContact は、テストごとの宛先を作る。Mailpit は、テストの間で共有されるためである。
func uniqueContact(t *testing.T) domain.Contact {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return domain.RestoreContact("smtpmail-test-" + hex.EncodeToString(b) + "@example.com")
}

type receivedMail struct {
	Subject string
	Text    string
	From    struct{ Address string }
	To      []struct{ Address string }
	raw     string
}

func (m mailpit) get(t *testing.T, path string, v any) []byte {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(m.apiBase + path)
	if err != nil {
		t.Fatalf("Mailpit の API に、届きません。make db-up で起動してください: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Mailpit の API の応答が違う: %s %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			t.Fatal(err)
		}
	}
	return body
}

// received は、宛先を指定して、届いた案内を読む。
func (m mailpit) received(t *testing.T, to domain.Contact) []receivedMail {
	t.Helper()
	var list struct {
		Messages []struct{ ID string }
	}
	m.get(t, "/api/v1/search?query="+url.QueryEscape("to:"+to.String()), &list)

	mails := make([]receivedMail, 0, len(list.Messages))
	for _, item := range list.Messages {
		var mail receivedMail
		m.get(t, "/api/v1/message/"+item.ID, &mail)
		mail.raw = string(m.get(t, "/api/v1/message/"+item.ID+"/raw", nil))
		mails = append(mails, mail)
	}
	return mails
}

func (m mailpit) one(t *testing.T, to domain.Contact) receivedMail {
	t.Helper()
	mails := m.received(t, to)
	if len(mails) != 1 {
		t.Fatalf("届いた案内の数が違う: %d", len(mails))
	}
	mail := mails[0]
	if mail.From.Address != testFrom || len(mail.To) != 1 || mail.To[0].Address != to.String() {
		t.Fatalf("送信元か、宛先が違う: %+v", mail)
	}
	return mail
}

// RL-01、RL-11: 登録を続けるための案内が、1通届く。内容は、登録を続けるためのアドレス、有効期限、
// 心当たりがない場合のことである。訪問者が入力した内容(連絡先)は、宛先のほかに、現れない。
func TestSendSignupNotice(t *testing.T) {
	m := requireMailpit(t)
	to := uniqueContact(t)
	const credential = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE"
	expiresAt := time.Date(2026, 11, 28, 9, 30, 0, 0, time.FixedZone("JST", 9*3600))

	if err := NewSender(m.smtpAddr, testFrom, testOrigin).SendSignupNotice(context.Background(), to, credential, expiresAt); err != nil {
		t.Fatal(err)
	}

	mail := m.one(t, to)
	if mail.Subject != "登録の確認" {
		t.Fatalf("件名が違う: %q", mail.Subject)
	}
	want := "登録の申し込みを受け付けました。\r\n" +
		"\r\n" +
		"次のアドレスを開いて、登録を続けてください。\r\n" +
		"http://localhost:5173/#signup=" + credential + "\r\n" +
		"\r\n" +
		"このアドレスは、申し込みから24時間、有効です(2026-11-28 00:30 UTC まで)。\r\n" +
		"\r\n" +
		"心当たりがない場合は、何もする必要はありません。\r\n"
	if mail.Text != want {
		t.Fatalf("本文が違う:\n got %q\nwant %q", mail.Text, want)
	}
	requireContactOnlyInTo(t, mail, to)
}

// RL-03、RL-11: 登録済みであることを伝える案内は、資格と、登録を続けるためのアドレスを含まない。
func TestSendAlreadyRegisteredNotice(t *testing.T) {
	m := requireMailpit(t)
	to := uniqueContact(t)

	if err := NewSender(m.smtpAddr, testFrom, testOrigin).SendAlreadyRegisteredNotice(context.Background(), to); err != nil {
		t.Fatal(err)
	}

	mail := m.one(t, to)
	if mail.Subject != "登録の申し込みについて" {
		t.Fatalf("件名が違う: %q", mail.Subject)
	}
	want := "この連絡先で、登録の申し込みがありました。\r\n" +
		"\r\n" +
		"この連絡先は、既に登録されています。\r\n" +
		"\r\n" +
		"心当たりがない場合は、何もする必要はありません。\r\n"
	if mail.Text != want {
		t.Fatalf("本文が違う:\n got %q\nwant %q", mail.Text, want)
	}
	if strings.Contains(mail.raw, "signup") || strings.Contains(mail.raw, testOrigin) {
		t.Fatal("登録済みであることを伝える案内が、登録を続けるためのアドレスを含む")
	}
	requireContactOnlyInTo(t, mail, to)
}

// requireContactOnlyInTo は、連絡先が、宛先のヘッダーのほかに、現れないことを確かめる。
func requireContactOnlyInTo(t *testing.T, mail receivedMail, to domain.Contact) {
	t.Helper()
	local, _, _ := strings.Cut(to.String(), "@")
	if strings.Contains(mail.Text, local) || strings.Contains(mail.Subject, local) {
		t.Fatal("連絡先が、本文か、件名に現れる")
	}
	headers, _, _ := strings.Cut(mail.raw, "\r\n\r\n")
	for _, line := range strings.Split(headers, "\r\n") {
		name, _, _ := strings.Cut(line, ":")
		// Received と Return-Path などは、Mailpit が付ける。サーバーが付けるヘッダーだけを確かめる。
		switch strings.ToLower(name) {
		case "from", "subject", "date", "mime-version", "content-type", "content-transfer-encoding":
			if strings.Contains(line, local) {
				t.Fatalf("連絡先が、宛先のほかのヘッダーに現れる: %s", name)
			}
		}
	}
}

// fakeSMTP は、定めた応答を返す、SMTP のサーバーである。相手が拒否する場合を作る。
// rejectAt に当たる命令に、拒否の応答を返す。
func fakeSMTP(t *testing.T, rejectAt, rejection string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if rejectAt == "silent" {
					// 挨拶を返さない。時間の上限を、確かめる。
					_, _ = bufio.NewReader(conn).ReadString('\n')
					return
				}
				reply := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
				reply("220 fake ESMTP")
				reader := bufio.NewReader(conn)
				inData := false
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					command := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case inData:
						if command == "." {
							inData = false
							if rejectAt == "END" {
								reply(rejection)
							} else {
								reply("250 OK")
							}
						}
					case strings.HasPrefix(command, rejectAt):
						reply(rejection)
					case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
						reply("250 fake")
					case strings.HasPrefix(command, "DATA"):
						inData = true
						reply("354 go ahead")
					case strings.HasPrefix(command, "QUIT"):
						reply("221 bye")
						return
					default:
						reply("250 OK")
					}
				}
			}()
		}
	}()
	return listener.Addr().String()
}

// RL-10、RL-33: 引き渡せなかった場合は、エラーを返す。エラーは、操作の名前と、応答の番号だけを持ち、
// 相手が返した文言と、宛先を、含まない。
func TestSend_RejectedBySMTPServer(t *testing.T) {
	to := domain.RestoreContact("marker-recipient@example.com")
	tests := []struct {
		name      string
		rejectAt  string
		rejection string
		wantOp    string
		wantCode  int
	}{
		{"送信元を拒否", "MAIL", "550 sender no-reply@chat.example rejected", "mail from", 550},
		{"宛先を拒否", "RCPT", "550 5.1.1 <marker-recipient@example.com>: Recipient address rejected", "rcpt to", 550},
		{"本文の受け付けを拒否", "DATA", "451 try again later, marker-recipient@example.com", "data", 451},
		{"引き渡しの完了を拒否", "END", "552 message for marker-recipient@example.com too large", "data", 552},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := NewSender(fakeSMTP(t, tt.rejectAt, tt.rejection), testFrom, testOrigin)

			for _, err := range []error{
				sender.SendSignupNotice(context.Background(), to, "marker-credential", time.Now()),
				sender.SendAlreadyRegisteredNotice(context.Background(), to),
			} {
				var sendErr *SendError
				if !errors.As(err, &sendErr) || sendErr.Op != tt.wantOp || sendErr.Code != tt.wantCode {
					t.Fatalf("エラーが違う: %#v", err)
				}
				if strings.Contains(err.Error(), "marker") || strings.Contains(err.Error(), "rejected") {
					t.Fatalf("エラーが、宛先か、相手の文言を含む: %v", err)
				}
			}
		})
	}
}

// 引き渡しが完了した後の、終わりの挨拶の失敗は、送れなかったこととしない。
func TestSend_QuitFailureIsNotFailure(t *testing.T) {
	sender := NewSender(fakeSMTP(t, "QUIT", "421 closing"), testFrom, testOrigin)

	if err := sender.SendAlreadyRegisteredNotice(context.Background(), domain.RestoreContact("a@example.com")); err != nil {
		t.Fatal(err)
	}
}

// RL-10: 接続できない場合と、相手が応答しない場合も、エラーを返す。待つ時間には、上限がある。
func TestSend_ConnectionFailureAndTimeout(t *testing.T) {
	to := domain.RestoreContact("marker-recipient@example.com")

	t.Run("接続できない", func(t *testing.T) {
		// 待ち受けていないポートを得る。
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := listener.Addr().String()
		_ = listener.Close()

		err = NewSender(addr, testFrom, testOrigin).SendAlreadyRegisteredNotice(context.Background(), to)

		var sendErr *SendError
		if !errors.As(err, &sendErr) || sendErr.Op != "connect" || strings.Contains(err.Error(), "marker") || strings.Contains(err.Error(), addr) {
			t.Fatalf("エラーが違う: %#v (%v)", err, err)
		}
	})

	t.Run("相手が応答しない", func(t *testing.T) {
		sender := NewSender(fakeSMTP(t, "silent", ""), testFrom, testOrigin)
		sender.timeout = 200 * time.Millisecond

		start := time.Now()
		err := sender.SendAlreadyRegisteredNotice(context.Background(), to)

		var sendErr *SendError
		if !errors.As(err, &sendErr) || sendErr.Op != "greet" {
			t.Fatalf("エラーが違う: %#v", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("時間の上限が、働いていない: %v", elapsed)
		}
	})

	t.Run("要求が取り消された", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := NewSender(fakeSMTP(t, "silent", ""), testFrom, testOrigin).SendAlreadyRegisteredNotice(ctx, to)

		var sendErr *SendError
		if !errors.As(err, &sendErr) {
			t.Fatalf("エラーが違う: %#v", err)
		}
	})
}

func TestDefaultTimeout(t *testing.T) {
	if got := NewSender("127.0.0.1:1025", testFrom, testOrigin).timeout; got != 10*time.Second {
		t.Fatalf("待つ時間の上限が違う: %v", got)
	}
}
