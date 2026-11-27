// Package port は、application が定義し、adapter が実装するインターフェイス(ポート)を置く。
// ポートの境界では、domain の型を、そのまま使う(ADR-005)。
package port

import (
	"context"
	"errors"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// UserDirectory は、登録を完了した利用者を調べる。
type UserDirectory interface {
	// FindByHandle は、識別名が一致する利用者を探す。大文字と小文字を、区別しない。
	FindByHandle(ctx context.Context, handle domain.Handle) (user domain.User, found bool, err error)
	// FindByContact は、連絡先が一致する利用者と、保管した秘密を探す。
	FindByContact(ctx context.Context, contact domain.Contact) (user domain.User, secret domain.SecretHash, found bool, err error)
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
	// InSignupTransaction は、登録の完了を、一体として行う(ADR-016)。取り消しと確定は、InTransaction と同じである。
	InSignupTransaction(ctx context.Context, fn func(ctx context.Context, w SignupWriter) error) error
}

// SignupWriter は、登録の完了の、トランザクションの中でだけ行える操作である。
// InSignupTransaction が、fn に渡す。
type SignupWriter interface {
	// ConsumeSignup は、資格に当たる、now の時点で有効な申し込みを、使用済みにして、返す。
	ConsumeSignup(ctx context.Context, credential string, now time.Time) (signup domain.Signup, found bool, err error)
	// CreateUser は、利用者を作る。識別名が使われていれば(大文字と小文字を区別しない)、ErrHandleTaken を返す。
	// 連絡先が登録済みであれば、ErrContactTaken を返す。
	CreateUser(ctx context.Context, contact domain.Contact, handle domain.Handle, name domain.DisplayName, secret domain.SecretHash) (domain.User, error)
}

var (
	// ErrHandleTaken は、識別名が、使われていることを示す。
	ErrHandleTaken = errors.New("port: handle taken")
	// ErrContactTaken は、連絡先が、登録済みであることを示す。
	ErrContactTaken = errors.New("port: contact taken")
)

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
// now の時点で、期限を過ぎたログイン状態による接続には、届けずに、閉じる(RL-18)。
type MessageDelivery interface {
	Deliver(ctx context.Context, now time.Time, participants []domain.User, msg domain.Message)
}

// ConnectionSink は、接続への書き込み口である。接続の受け付けの adapter が実装する。
// 接続は、書き込み口の値そのもので識別する。
type ConnectionSink interface {
	// Send は、1通を、送信待ちの列に加える。待たずに戻る。
	// 列が一杯で、加えられなかった場合は、false を返す。
	Send(data []byte) (queued bool)
	// Close は、接続を閉じる。何度呼んでもよい。
	Close()
	// CloseSessionEnded は、ログイン状態が無効になったことを伝えて、接続を閉じる。何度呼んでもよい。
	// 閉じる理由を、引数で渡す形にしないのは、受け付けの adapter が、ポートの型を import できないためである。
	CloseSessionEnded()
}

// ConnectionRegistry は、接続中の端末の登録と解除である。配信の adapter が実装する。
// 接続を、利用者に加えて、ログイン状態と対応付ける(ADR-014)。
type ConnectionRegistry interface {
	Register(ctx context.Context, session domain.Session, sink ConnectionSink)
	Unregister(ctx context.Context, sink ConnectionSink)
}

// SessionStore は、ログイン状態を保持する(ADR-016)。
// 資格そのものは、保管しない。実装が、資格の要約で保管し、要約で引く(ADR-014)。
// 時刻を要する操作は、now を、引数で受ける。実装は、自分では、時刻を読まない。
type SessionStore interface {
	// Create は、資格に当たるログイン状態を作る。
	Create(ctx context.Context, credential string, user domain.UserID, expiresAt time.Time) (domain.Session, error)
	// Find は、資格に当たる、now の時点で有効なログイン状態と、その利用者を探す。
	Find(ctx context.Context, credential string, now time.Time) (session domain.Session, user domain.User, found bool, err error)
	// Revoke は、資格に当たるログイン状態を、無効にする。
	Revoke(ctx context.Context, credential string) (id domain.SessionID, found bool, err error)
	// DeleteExpired は、now の時点で、期限を過ぎたログイン状態を消す。
	DeleteExpired(ctx context.Context, now time.Time) error
}

// SignupStore は、登録の申し込みを保持する(ADR-016)。資格の扱いは、SessionStore と同じである。
type SignupStore interface {
	// Put は、申し込みを置く。同じ連絡先の、以前の申し込みを置き換える。
	Put(ctx context.Context, contact domain.Contact, credential string, expiresAt time.Time) error
	// Find は、資格に当たる、now の時点で有効な申し込みを探す。
	Find(ctx context.Context, credential string, now time.Time) (signup domain.Signup, found bool, err error)
	// DeleteExpired は、now の時点で、期限を過ぎた申し込みを消す。
	DeleteExpired(ctx context.Context, now time.Time) error
}

// SessionConnections は、ログイン状態による接続を閉じる。配信の adapter が実装する(ADR-014)。
type SessionConnections interface {
	// CloseBySession は、そのログイン状態による接続を、すべて閉じる。
	CloseBySession(ctx context.Context, id domain.SessionID)
	// CloseExpired は、now の時点で、期限を過ぎたログイン状態による接続を、すべて閉じる。
	CloseExpired(ctx context.Context, now time.Time)
}

// CredentialSource は、資格(ログイン状態を示す資格、確認のための資格)を作る(ADR-019)。
type CredentialSource interface {
	NewCredential() (string, error)
}

// SecretHasher は、秘密を、保管する形に変え、照合する(ADR-015)。
type SecretHasher interface {
	Hash(ctx context.Context, secret domain.Secret) (domain.SecretHash, error)
	// Verify は、入力された秘密を、保管した秘密と照合する。入力は、規則で検証していない文字列である。
	Verify(ctx context.Context, secret string, stored domain.SecretHash) (matched bool, err error)
	// VerifyDummy は、照合する相手がないログインで、Verify と同じ重さの計算を、1回行う(RL-13)。
	VerifyDummy(ctx context.Context) error
}

// CommonSecretList は、秘密が、よく使われる秘密の一覧にあるかを確かめる(ADR-017)。
type CommonSecretList interface {
	Contains(secret domain.Secret) bool
}

// NoticeSender は、連絡先へ、案内を送る(ADR-018)。
// 返すエラーは、宛先を含まない形に、作り直したものとする。
type NoticeSender interface {
	// SendSignupNotice は、登録を続けるための案内を送る。
	SendSignupNotice(ctx context.Context, to domain.Contact, credential string, expiresAt time.Time) error
	// SendAlreadyRegisteredNotice は、登録済みであることを伝える案内を送る。
	SendAlreadyRegisteredNotice(ctx context.Context, to domain.Contact) error
}

// AttemptLimiter は、上限を数える(ADR-016)。
// メモリの上の実装は、ctx と error を使わない。共有の保管先を使う実装に差し替えるとき、
// 内側の層を変えないために、持つ。計数を確かめられなかった操作は、受け付けない。
type AttemptLimiter interface {
	// Admit は、limits のすべてが、上限に達していないことを確かめ、達していなければ、それぞれに、1回と数える。
	// 達しているものがあれば、どれも数えずに、最初に当たったものを返す。
	Admit(ctx context.Context, now time.Time, limits ...domain.Limit) (exceeded domain.Limit, admitted bool, err error)
	// Forgive は、Admit が数えた1回を、戻す。
	Forgive(ctx context.Context, limit domain.Limit) error
	// Reset は、計数を、0に戻す。
	Reset(ctx context.Context, limit domain.Limit) error
}

// Clock は、時刻の源である。期限と上限の判定は、ここから得た時刻に揃える。
type Clock interface {
	Now() time.Time
}

// EventRecorder は、出来事を記録する(R-6、ADR-021)。
// 記録の失敗は、操作の結果を変えないため、エラーを返さない。
type EventRecorder interface {
	Record(ctx context.Context, event domain.Event)
}
