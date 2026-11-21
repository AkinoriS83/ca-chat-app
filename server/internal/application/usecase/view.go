package usecase

import (
	"errors"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// errSenderNotParticipant は、保管したメッセージの送信者が、会話の参加者でないことを示す。
// 保管先の内容が壊れている場合にだけ起こる。完了できなかったものとして扱う。
var errSenderNotParticipant = errors.New("usecase: message sender is not a participant")

// toMessageView は、domain のメッセージを、出力のデータ構造に詰め替える。
// a と b は、会話の参加者2人である。
func toMessageView(msg domain.Message, a, b domain.User) (MessageView, error) {
	var sender domain.User
	switch msg.SenderID {
	case a.ID:
		sender = a
	case b.ID:
		sender = b
	default:
		return MessageView{}, errSenderNotParticipant
	}

	participants := []string{a.Handle, b.Handle}
	if participants[0] > participants[1] {
		participants[0], participants[1] = participants[1], participants[0]
	}

	return MessageView{
		Participants:      participants,
		Seq:               msg.Seq,
		SenderHandle:      sender.Handle,
		SenderDisplayName: sender.DisplayName,
		Body:              msg.Body.String(),
		SentAt:            msg.SentAt,
	}, nil
}
