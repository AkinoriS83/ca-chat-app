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
	gate     *SessionGate
	resolver *peerResolver
	reader   port.ConversationReader
	tx       port.Transactor
	delivery port.MessageDelivery
	clock    port.Clock
	events   port.EventRecorder
}

// NewMessageSender は、MessageSender を作る。
func NewMessageSender(gate *SessionGate, users port.UserDirectory, reader port.ConversationReader, tx port.Transactor, delivery port.MessageDelivery, limiter port.AttemptLimiter, rules domain.LimitRules, clock port.Clock, events port.EventRecorder) *MessageSender {
	return &MessageSender{
		gate:     gate,
		resolver: &peerResolver{users: users, reader: reader, limiter: limiter, rules: rules, clock: clock},
		reader:   reader,
		tx:       tx,
		delivery: delivery,
		clock:    clock,
		events:   events,
	}
}

// Send は、メッセージを保管し、配信する。
// 拒否は、次の順に判定する。ログイン状態 → 形式 → 本文 → 送信の識別子の形式 → 自分自身 → 上限 →
// 相手が居ない → 別の会話との一致(仕様「メッセージを送る」「上限」)。
// 相手の指定が、識別名の規則を満たさない場合は、相手が居ない場合として扱う。
func (s *MessageSender) Send(ctx context.Context, in SendMessageInput) (out SendMessageOutput, err error) {
	a := begin(s.events, "send message", in.Source)
	defer a.finish(ctx, &err)

	_, actor, err := s.gate.verify(ctx, a, in.Actor)
	if err != nil {
		return SendMessageOutput{}, err
	}
	// 形式に合わない要求は、ログイン状態の確認のすぐ後に、ほかの手順より先に、拒否する。
	if in.Malformed {
		return SendMessageOutput{}, rejected(InvalidInput)
	}

	body, err := domain.NewBody(in.Body)
	if err != nil {
		return SendMessageOutput{}, rejected(InvalidBody)
	}

	sendID, err := domain.ParseSendID(in.SendID)
	if err != nil {
		return SendMessageOutput{}, rejected(InvalidInput)
	}

	resolved, err := s.resolver.resolve(ctx, a, actor, in.Peer, NotFound)
	if err != nil {
		return SendMessageOutput{}, err
	}
	peer, pair := resolved.user, resolved.pair

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

	// 配信は、保管が確定した後に行う。配信の時点で、期限を過ぎたログイン状態による接続には、届かない。
	s.delivery.Deliver(ctx, s.clock.Now(), []domain.User{actor, peer}, stored)

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
