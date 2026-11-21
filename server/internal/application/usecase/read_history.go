package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// HistoryReader は、「会話の履歴を読む」ユースケースである(UC-11)。
type HistoryReader struct {
	users  port.UserDirectory
	reader port.ConversationReader
}

// NewHistoryReader は、HistoryReader を作る。
func NewHistoryReader(users port.UserDirectory, reader port.ConversationReader) *HistoryReader {
	return &HistoryReader{users: users, reader: reader}
}

// Read は、相手との1on1会話の、すべてのメッセージを、連番の順に返す。
// 成立していない会話は、空の一覧を返す。会話は、成立させない(WS-06)。
func (h *HistoryReader) Read(ctx context.Context, in ReadHistoryInput) (ReadHistoryOutput, error) {
	actor, found, err := h.users.FindByHandle(ctx, in.Actor)
	if err != nil {
		return ReadHistoryOutput{}, err
	}
	if !found {
		return ReadHistoryOutput{}, rejected(Unauthenticated)
	}

	peer, found, err := h.users.FindByHandle(ctx, in.Peer)
	if err != nil {
		return ReadHistoryOutput{}, err
	}
	if !found {
		return ReadHistoryOutput{}, rejected(NotFound)
	}

	pair, err := domain.NewDirectPair(actor.ID, peer.ID)
	if err != nil {
		return ReadHistoryOutput{}, rejected(InvalidPeer)
	}

	conv, found, err := h.reader.FindDirect(ctx, pair)
	if err != nil {
		return ReadHistoryOutput{}, err
	}
	if !found {
		return ReadHistoryOutput{Messages: []MessageView{}}, nil
	}

	messages, err := h.reader.ListMessages(ctx, conv.ID)
	if err != nil {
		return ReadHistoryOutput{}, err
	}

	views := make([]MessageView, 0, len(messages))
	for _, msg := range messages {
		view, err := toMessageView(msg, actor, peer)
		if err != nil {
			return ReadHistoryOutput{}, err
		}
		views = append(views, view)
	}
	return ReadHistoryOutput{Messages: views}, nil
}
