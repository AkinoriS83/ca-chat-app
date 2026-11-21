// Package port は、application が定義し、adapter が実装するインターフェイス(ポート)を置く。
// ポートの境界では、domain の型を、そのまま使う(ADR-005)。
package port

import (
	"context"
	"errors"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// UserDirectory は、利用者を調べる。
// 仮実装(WS-P1): 本スライスの実装は、固定ユーザーである。
type UserDirectory interface {
	FindByHandle(ctx context.Context, handle string) (user domain.User, found bool, err error)
}

// ConversationReader は、保管先の読み取りである。トランザクションの外で使う。
type ConversationReader interface {
	FindDirect(ctx context.Context, pair domain.DirectPair) (conv domain.Conversation, found bool, err error)
	// ListMessages は、会話のすべてのメッセージを、連番の順に返す。
	// 仮実装(WS-P4): 分割せずに、一度に取得する。フェーズ1で、分割しての取得に置き換える。
	ListMessages(ctx context.Context, id domain.ConversationID) ([]domain.Message, error)
	FindMessageBySend(ctx context.Context, sender domain.UserID, sendID domain.SendID) (msg domain.Message, found bool, err error)
}

// Transactor は、一体として行う操作を、1つの関数として受ける。
// fn がエラーを返すと、その中で行った操作を、すべて取り消す。返さなければ、確定する(ADR-012)。
// 確定は、ctx の取り消しに従わない。fn がエラーを返さずに戻った後は、ctx が取り消されても、確定を試みる。
type Transactor interface {
	InTransaction(ctx context.Context, fn func(ctx context.Context, w ConversationWriter) error) error
}

// ConversationWriter は、トランザクションの中でだけ行える操作である。
// InTransaction が、fn に渡す。
type ConversationWriter interface {
	// EstablishDirect は、会話を成立させる。既に成立していれば、その会話を返す。
	EstablishDirect(ctx context.Context, pair domain.DirectPair) (domain.Conversation, error)
	// SerializeAppends は、その会話への追記を、直列にする。
	// トランザクションが終わるまで、他の追記を待たせる。
	SerializeAppends(ctx context.Context, id domain.ConversationID) error
	// AppendMessage は、メッセージを追記する。連番と送信の日時を付けて返す。
	// 同じ送信者の、同じ送信の識別子のメッセージが保管済みであれば、ErrDuplicateSend を返す。
	AppendMessage(ctx context.Context, id domain.ConversationID, sender domain.UserID, sendID domain.SendID, body domain.Body) (domain.Message, error)
}

// ErrDuplicateSend は、同じ送信者の、同じ送信の識別子のメッセージが、保管済みであることを示す。
var ErrDuplicateSend = errors.New("port: duplicate send")

// MessageDelivery は、配信である。
// 保管が確定したメッセージを、participants がアプリを開いている端末のすべてに届ける。
// 送信待ちの列に加えるまでを、呼び出しの中で終える。端末への書き込みは、待たない。
// 配信が届かなくても、保管と完了の応答は取り消さないため、エラーを返さない。
type MessageDelivery interface {
	Deliver(ctx context.Context, participants []domain.User, msg domain.Message)
}

// ConnectionSink は、接続への書き込み口である。接続の受け付けの adapter が実装する。
// 接続は、書き込み口の値そのもので識別する。
type ConnectionSink interface {
	// Send は、1通を、送信待ちの列に加える。待たずに戻る。
	// 列が一杯で、加えられなかった場合は、false を返す。
	Send(data []byte) (queued bool)
	// Close は、接続を閉じる。何度呼んでもよい。
	Close()
}

// ConnectionRegistry は、接続中の端末の登録と解除である。配信の adapter が実装する。
type ConnectionRegistry interface {
	Register(ctx context.Context, user domain.UserID, sink ConnectionSink)
	Unregister(ctx context.Context, sink ConnectionSink)
}
