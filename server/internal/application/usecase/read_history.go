package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// HistoryReader は、「会話の履歴を読む」ユースケースである(UC-11)。
type HistoryReader struct {
	gate     *SessionGate
	resolver *peerResolver
	reader   port.ConversationReader
	events   port.EventRecorder
}

// NewHistoryReader は、HistoryReader を作る。
func NewHistoryReader(gate *SessionGate, users port.UserDirectory, reader port.ConversationReader, limiter port.AttemptLimiter, rules domain.LimitRules, clock port.Clock, events port.EventRecorder) *HistoryReader {
	return &HistoryReader{
		gate:     gate,
		resolver: &peerResolver{users: users, reader: reader, limiter: limiter, rules: rules, clock: clock},
		reader:   reader,
		events:   events,
	}
}

// Read は、相手との1on1会話の、すべてのメッセージを、連番の順に返す。
// 成立していない会話は、空の一覧を返す。会話は、成立させない(WS-06)。
// 相手の指定が、識別名の規則を満たさない場合は、相手が居ない場合として扱う。
//
// 仮実装(WS-P4): 分割せずに、一度に返す。フェーズ1で、入力(ReadHistoryInput)に位置と件数を加え、
// 出力(ReadHistoryOutput)を、分割して返す形に改める。
func (h *HistoryReader) Read(ctx context.Context, in ReadHistoryInput) (out ReadHistoryOutput, err error) {
	a := begin(h.events, "read history", in.Source)
	defer a.finish(ctx, &err)

	_, actor, err := h.gate.verify(ctx, a, in.Actor)
	if err != nil {
		return ReadHistoryOutput{}, err
	}
	peer, err := h.resolver.resolve(ctx, a, actor, in.Peer, NotFound)
	if err != nil {
		return ReadHistoryOutput{}, err
	}
	if !peer.established {
		return ReadHistoryOutput{Messages: []MessageView{}}, nil
	}

	messages, err := h.reader.ListMessages(ctx, peer.conversation.ID)
	if err != nil {
		return ReadHistoryOutput{}, err
	}

	views := make([]MessageView, 0, len(messages))
	for _, msg := range messages {
		view, err := toMessageView(msg, actor, peer.user)
		if err != nil {
			return ReadHistoryOutput{}, err
		}
		views = append(views, view)
	}
	return ReadHistoryOutput{Messages: views}, nil
}
