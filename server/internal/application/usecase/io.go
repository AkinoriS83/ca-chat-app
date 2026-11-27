// Package usecase は、ユースケースと、その入力・出力のデータ構造を置く。
// 入力と出力には、domain の型を含めない(ADR-005「型参照の範囲」)。
package usecase

import (
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// MessageView は、ユースケースが出力するメッセージである。
type MessageView struct {
	Participants      []string // 会話の参加者2人の識別名。昇順
	Seq               int64
	SenderHandle      string
	SenderDisplayName string
	Body              string
	SentAt            time.Time
}

// SendMessageInput は、「メッセージを送る」の入力である。Peer は、識別名である。Source は、送信元である。
type SendMessageInput struct {
	Actor  Actor  // 操作を行う利用者
	Source string // 送信元
	Peer   string // 相手
	SendID string // 送信の識別子
	Body   string // 本文
	// Malformed は、要求が、形式に合わないことを示す。SendID と Body は、空である。
	// 形式に合わない要求も、adapter は拒否せずに、ユースケースを呼ぶ。ログイン状態の確認を、
	// 形式の確認より先に行うためである(RL-15)。
	Malformed bool
}

// SendMessageOutput は、「メッセージを送る」の出力である。
type SendMessageOutput struct {
	Message MessageView
}

// ReadHistoryInput は、「会話の履歴を読む」の入力である。Peer は、識別名である。
type ReadHistoryInput struct {
	Actor  Actor  // 操作を行う利用者
	Source string // 送信元
	Peer   string // 相手
}

// ReadHistoryOutput は、「会話の履歴を読む」の出力である。
type ReadHistoryOutput struct {
	Messages []MessageView // 連番の順
}

// AcceptConnectionInput は、「リアルタイムの接続を受け付ける」の入力である。
type AcceptConnectionInput struct {
	Actor  Actor  // 操作を行う利用者
	Source string // 送信元
	// Malformed は、WebSocket の接続を確立する要求でないことを示す(SendMessageInput と同じ扱い)。
	Malformed bool
	// Connection は、接続への書き込み口である。
	// 入力に、port の型を含めるのは、ADR-005 が定める例外である。
	Connection port.ConnectionSink
}

// CloseConnectionInput は、「接続の終わりを受ける」の入力である。
type CloseConnectionInput struct {
	Connection port.ConnectionSink
}

// Actor は、操作を行う利用者である。ユースケースを呼ぶ adapter が、要求から取り出して、
// そのまま渡す(ADR-019)。添えられていなければ、空の文字列である。確かめるのは、usecase である。
type Actor struct {
	Credential string // ログイン状態を示す資格
	SessionID  string // ログイン状態の識別子
}

// UserView は、ユースケースが出力する利用者である。識別名と表示名だけを持つ(RL-27)。
type UserView struct {
	Handle      string
	DisplayName string
}

// RequestSignupInput は、「登録を申し込む」の入力である。Source は、送信元である。
type RequestSignupInput struct {
	Contact string
	Source  string
}

// CompleteSignupInput は、「登録を完了する」の入力である。Credential は、確認のための資格である。
type CompleteSignupInput struct {
	Credential  string
	Handle      string
	DisplayName string
	Secret      string
	Source      string
}

// LoginInput は、「ログインする」の入力である。
type LoginInput struct {
	Credential string // 要求に付いていた、ログイン状態を示す資格。なければ、空
	Contact    string
	Secret     string
	Source     string
}

// LoginOutput は、「ログインする」の出力である。資格を含む出力は、これだけである(ADR-019)。
type LoginOutput struct {
	Credential string // 新しく与えた資格。adapter は、Cookie にだけ載せる
	SessionID  string
	ExpiresAt  time.Time
	User       UserView
}

// CurrentSessionInput は、「現在のログイン状態を確かめる」の入力である。
type CurrentSessionInput struct {
	Credential string
	Source     string
}

// CurrentSessionOutput は、「現在のログイン状態を確かめる」の出力である。
// SessionID と User は、Authenticated が真の場合だけ、値を持つ。
type CurrentSessionOutput struct {
	Authenticated bool
	SessionID     string
	User          UserView
}

// LogoutInput は、「ログアウトする」の入力である。
type LogoutInput struct {
	Actor  Actor
	Source string
}

// FindPeerInput は、「相手を特定する」の入力である。Handle は、相手の識別名である。
type FindPeerInput struct {
	Actor  Actor
	Source string
	Handle string
}

// FindPeerOutput は、「相手を特定する」の出力である。識別名は、相手が登録した通りである。
type FindPeerOutput struct {
	Peer UserView
}
