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

// SendMessageInput は、「メッセージを送る」の入力である。Actor と Peer は、識別名である。
type SendMessageInput struct {
	Actor  string // 操作を行う利用者
	Peer   string // 相手
	SendID string // 送信の識別子
	Body   string // 本文
}

// SendMessageOutput は、「メッセージを送る」の出力である。
type SendMessageOutput struct {
	Message MessageView
}

// ReadHistoryInput は、「会話の履歴を読む」の入力である。Actor と Peer は、識別名である。
type ReadHistoryInput struct {
	Actor string // 操作を行う利用者
	Peer  string // 相手
}

// ReadHistoryOutput は、「会話の履歴を読む」の出力である。
type ReadHistoryOutput struct {
	Messages []MessageView // 連番の順
}

// AcceptConnectionInput は、「リアルタイムの接続を受け付ける」の入力である。
type AcceptConnectionInput struct {
	Actor string // 操作を行う利用者(識別名)
	// Connection は、接続への書き込み口である。
	// 入力に、port の型を含めるのは、ADR-005 が定める例外である。
	Connection port.ConnectionSink
}

// CloseConnectionInput は、「接続の終わりを受ける」の入力である。
type CloseConnectionInput struct {
	Connection port.ConnectionSink
}
