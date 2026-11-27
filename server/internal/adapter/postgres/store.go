// Package postgres は、保管先(PostgreSQL)へ問い合わせる adapter である。
// port.ConversationReader、port.Transactor と、利用者、ログイン状態、登録の申し込みのポートを実装する
// (ADR-004、ADR-012、ADR-016)。
//
// domain を使うのは、型の受け渡しと、保管した値からの組み立てに限る。
// domain の規則(NewBody、ParseSendID など)は、呼ばない(ADR-005。規約)。
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres/sqlcgen"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// Store は、保管先への問い合わせを行う。
type Store struct {
	pool *pgxpool.Pool
}

// NewStore は、接続プールを使う Store を作る。
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

var (
	_ port.ConversationReader = (*Store)(nil)
	_ port.Transactor         = (*Store)(nil)
	_ port.ConversationWriter = (*writer)(nil)
)

// FindDirect は、2人の間の会話を探す。
func (s *Store) FindDirect(ctx context.Context, pair domain.DirectPair) (domain.Conversation, bool, error) {
	return findDirect(ctx, sqlcgen.New(s.pool), pair)
}

// ListMessages は、会話のすべてのメッセージを、連番の順に返す。
// 仮実装(WS-P4): 分割せずに、一度に取得する。
func (s *Store) ListMessages(ctx context.Context, id domain.ConversationID) ([]domain.Message, error) {
	const op = "list messages"

	convID, err := toUUID(string(id))
	if err != nil {
		return nil, newOpError(op, err)
	}
	rows, err := sqlcgen.New(s.pool).ListMessages(ctx, convID)
	if err != nil {
		return nil, newOpError(op, err)
	}

	messages := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessage(row))
	}
	return messages, nil
}

// FindMessageBySend は、送信者と、送信の識別子が一致する、保管済みのメッセージを探す。
func (s *Store) FindMessageBySend(ctx context.Context, sender domain.UserID, sendID domain.SendID) (domain.Message, bool, error) {
	const op = "find message by send"

	senderID, err := toUUID(string(sender))
	if err != nil {
		return domain.Message{}, false, newOpError(op, err)
	}
	sendUUID, err := toUUID(sendID.String())
	if err != nil {
		return domain.Message{}, false, newOpError(op, err)
	}
	row, err := sqlcgen.New(s.pool).FindMessageBySend(ctx, sqlcgen.FindMessageBySendParams{SenderID: senderID, SendID: sendUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, false, nil
	}
	if err != nil {
		return domain.Message{}, false, newOpError(op, err)
	}
	return toMessage(row), true, nil
}

// commitTimeout は、確定を待つ時間の上限である。
const commitTimeout = 5 * time.Second

// InTransaction は、fn を、1つのトランザクションの中で実行する。
// fn がエラーを返すか、パニックした場合は取り消し、そうでなければ確定する。
// 分離の水準は、既定(READ COMMITTED)とする。
//
// 確定は、ctx の取り消しに従わない(ADR-012)。確定を保管先に送った後に、待つのをやめると、
// 保管先では確定したのに、エラーを返すことがある。呼び出した側は、完了できなかったものとして扱い、
// 保管されたメッセージが、配信されない。代わりに、時間の上限を設ける。
func (s *Store) InTransaction(ctx context.Context, fn func(ctx context.Context, w port.ConversationWriter) error) error {
	return s.inTx(ctx, func(queries *sqlcgen.Queries) error {
		return fn(ctx, &writer{queries: queries})
	})
}

// inTx は、fn を、1つのトランザクションの中で実行する。InTransaction と InSignupTransaction が使う。
func (s *Store) inTx(ctx context.Context, fn func(queries *sqlcgen.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return newOpError("begin transaction", err)
	}
	// 確定した後の取り消しは、何もしない。fn がパニックした場合も、ここで取り消す。
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(sqlcgen.New(tx)); err != nil {
		return err
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancel()
	if err := tx.Commit(commitCtx); err != nil {
		return newOpError("commit transaction", err)
	}
	return nil
}

// writer は、トランザクションの中でだけ行える操作を実装する。
type writer struct {
	queries *sqlcgen.Queries
}

// EstablishDirect は、会話を成立させる。既に成立していれば、その会話を返す。
//
// 2人がほぼ同時に最初のメッセージを送った場合、後の INSERT は、先のトランザクションの確定を待ち、
// 何もしない。続く SELECT が、確定した会話を得る(WS-05)。
func (w *writer) EstablishDirect(ctx context.Context, pair domain.DirectPair) (domain.Conversation, error) {
	const op = "establish conversation"

	low, high, err := pairToUUIDs(pair)
	if err != nil {
		return domain.Conversation{}, newOpError(op, err)
	}
	if err := w.queries.InsertConversationIfAbsent(ctx, sqlcgen.InsertConversationIfAbsentParams{UserLow: low, UserHigh: high}); err != nil {
		return domain.Conversation{}, newOpError(op, err)
	}
	id, err := w.queries.FindConversationID(ctx, sqlcgen.FindConversationIDParams{UserLow: low, UserHigh: high})
	if err != nil {
		return domain.Conversation{}, newOpError(op, err)
	}
	return domain.Conversation{ID: domain.ConversationID(id.String()), Pair: pair}, nil
}

// SerializeAppends は、会話の行をロックし、その会話への追記を、直列にする(ADR-010)。
func (w *writer) SerializeAppends(ctx context.Context, id domain.ConversationID) error {
	const op = "serialize appends"

	convID, err := toUUID(string(id))
	if err != nil {
		return newOpError(op, err)
	}
	if _, err := w.queries.LockConversation(ctx, convID); err != nil {
		return newOpError(op, err)
	}
	return nil
}

// AppendMessage は、メッセージを追記する。連番と送信の日時は、保管先が付ける。
// 同じ送信者の、同じ送信の識別子のメッセージが保管済みであれば、port.ErrDuplicateSend を返す。
func (w *writer) AppendMessage(ctx context.Context, id domain.ConversationID, sender domain.UserID, sendID domain.SendID, body domain.Body) (domain.Message, error) {
	const op = "append message"

	convID, err := toUUID(string(id))
	if err != nil {
		return domain.Message{}, newOpError(op, err)
	}
	senderID, err := toUUID(string(sender))
	if err != nil {
		return domain.Message{}, newOpError(op, err)
	}
	sendUUID, err := toUUID(sendID.String())
	if err != nil {
		return domain.Message{}, newOpError(op, err)
	}

	row, err := w.queries.AppendMessage(ctx, sqlcgen.AppendMessageParams{
		ConversationID: convID,
		SenderID:       senderID,
		SendID:         sendUUID,
		Body:           body.String(),
	})
	if isUniqueViolation(err, "messages_sender_send_key") {
		return domain.Message{}, port.ErrDuplicateSend
	}
	if err != nil {
		return domain.Message{}, newOpError(op, err)
	}

	return domain.Message{
		ConversationID: id,
		Seq:            row.Seq,
		SenderID:       sender,
		SendID:         sendID,
		Body:           body,
		SentAt:         row.SentAt.UTC(),
	}, nil
}

func findDirect(ctx context.Context, queries *sqlcgen.Queries, pair domain.DirectPair) (domain.Conversation, bool, error) {
	const op = "find conversation"

	low, high, err := pairToUUIDs(pair)
	if err != nil {
		return domain.Conversation{}, false, newOpError(op, err)
	}
	id, err := queries.FindConversationID(ctx, sqlcgen.FindConversationIDParams{UserLow: low, UserHigh: high})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Conversation{}, false, nil
	}
	if err != nil {
		return domain.Conversation{}, false, newOpError(op, err)
	}
	return domain.Conversation{ID: domain.ConversationID(id.String()), Pair: pair}, true, nil
}

// toMessage は、保管先の行から、domain のメッセージを組み立てる。
// 保管した値は、保管の時点で検証を通っているため、復元用の関数を使う(規則は、呼ばない)。
func toMessage(row sqlcgen.Message) domain.Message {
	return domain.Message{
		ConversationID: domain.ConversationID(row.ConversationID.String()),
		Seq:            row.Seq,
		SenderID:       domain.UserID(row.SenderID.String()),
		SendID:         domain.RestoreSendID(row.SendID.String()),
		Body:           domain.RestoreBody(row.Body),
		SentAt:         row.SentAt.UTC(),
	}
}

func pairToUUIDs(pair domain.DirectPair) (low, high pgtype.UUID, err error) {
	low, err = toUUID(string(pair.Low()))
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	high, err = toUUID(string(pair.High()))
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	return low, high, nil
}

// toUUID は、UUID の文字列を、保管先の表現にする。
func toUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, errInvalidUUID
	}
	return u, nil
}
