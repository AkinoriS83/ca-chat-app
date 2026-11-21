package usecase

import (
	"context"
	"errors"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// errStoredMessageMissing は、重複した送信と判定されたのに、保管済みのメッセージが見つからないことを示す。
var errStoredMessageMissing = errors.New("usecase: stored message for duplicate send is missing")

// MessageSender は、「メッセージを送る」ユースケースである(UC-09)。
type MessageSender struct {
	users    port.UserDirectory
	reader   port.ConversationReader
	tx       port.Transactor
	delivery port.MessageDelivery
}

// NewMessageSender は、MessageSender を作る。
func NewMessageSender(users port.UserDirectory, reader port.ConversationReader, tx port.Transactor, delivery port.MessageDelivery) *MessageSender {
	return &MessageSender{users: users, reader: reader, tx: tx, delivery: delivery}
}

// Send は、メッセージを保管し、配信する。
// 拒否は、仕様「メッセージを送る」の表の順に判定する。
func (s *MessageSender) Send(ctx context.Context, in SendMessageInput) (SendMessageOutput, error) {
	actor, found, err := s.users.FindByHandle(ctx, in.Actor)
	if err != nil {
		return SendMessageOutput{}, err
	}
	if !found {
		return SendMessageOutput{}, rejected(Unauthenticated)
	}

	body, err := domain.NewBody(in.Body)
	if err != nil {
		return SendMessageOutput{}, rejected(InvalidBody)
	}

	sendID, err := domain.ParseSendID(in.SendID)
	if err != nil {
		return SendMessageOutput{}, rejected(InvalidInput)
	}

	// 自分自身を相手に指定した場合、相手は必ず見つかる。
	// そのため、この順でも、仕様の表の順(自分自身 → 相手が居ない)と、同じ結果になる。
	peer, found, err := s.users.FindByHandle(ctx, in.Peer)
	if err != nil {
		return SendMessageOutput{}, err
	}
	if !found {
		return SendMessageOutput{}, rejected(NotFound)
	}

	pair, err := domain.NewDirectPair(actor.ID, peer.ID)
	if err != nil {
		return SendMessageOutput{}, rejected(InvalidPeer)
	}

	// 会話の成立、追記の直列化、メッセージの追記を、一体として行う(ADR-012)。
	// 関数の中では、渡された操作のほかに、ポートを呼ばない。
	var stored domain.Message
	err = s.tx.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
		conv, err := w.EstablishDirect(ctx, pair)
		if err != nil {
			return err
		}
		if err := w.SerializeAppends(ctx, conv.ID); err != nil {
			return err
		}
		stored, err = w.AppendMessage(ctx, conv.ID, actor.ID, sendID, body)
		return err
	})
	if errors.Is(err, port.ErrDuplicateSend) {
		return s.completeDuplicate(ctx, actor, peer, pair, sendID)
	}
	if err != nil {
		// 保管に失敗した送信は、完了として示さず、配信しない(WS-31)。
		return SendMessageOutput{}, err
	}

	// 配信は、保管が確定した後に行う。
	s.delivery.Deliver(ctx, []domain.User{actor, peer}, stored)

	view, err := toMessageView(stored, actor, peer)
	if err != nil {
		return SendMessageOutput{}, err
	}
	return SendMessageOutput{Message: view}, nil
}

// completeDuplicate は、重複した送信を扱う(ADR-009、ADR-012)。
// トランザクションが取り消された後に、その外で、保管済みのメッセージを取得する。
// 指定した会話のものであれば、配信せずに、完了を示す。別の会話のものであれば、拒否する。
func (s *MessageSender) completeDuplicate(ctx context.Context, actor, peer domain.User, pair domain.DirectPair, sendID domain.SendID) (SendMessageOutput, error) {
	existing, found, err := s.reader.FindMessageBySend(ctx, actor.ID, sendID)
	if err != nil {
		return SendMessageOutput{}, err
	}
	if !found {
		return SendMessageOutput{}, errStoredMessageMissing
	}

	conv, found, err := s.reader.FindDirect(ctx, pair)
	if err != nil {
		return SendMessageOutput{}, err
	}
	if !found || conv.ID != existing.ConversationID {
		return SendMessageOutput{}, rejected(InvalidInput)
	}

	view, err := toMessageView(existing, actor, peer)
	if err != nil {
		return SendMessageOutput{}, err
	}
	return SendMessageOutput{Message: view}, nil
}
