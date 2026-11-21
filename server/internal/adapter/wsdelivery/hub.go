// Package wsdelivery は、配信を行う adapter である。
// port.MessageDelivery と port.ConnectionRegistry を実装する(ADR-005)。
//
// 接続中の端末と、配信の宛先を保持する状態は、このパッケージだけが持つ(PERF-3、WS-35)。
// domain の型を、通信の形式に変換し、接続への書き込み口に渡す。WebSocket のライブラリは、使わない。
package wsdelivery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// Hub は、接続中の端末を保持し、配信する。
type Hub struct {
	logger *slog.Logger

	mu     sync.Mutex
	byUser map[domain.UserID]map[port.ConnectionSink]struct{}
	owners map[port.ConnectionSink]domain.UserID
}

// NewHub は、Hub を作る。
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger: logger,
		byUser: map[domain.UserID]map[port.ConnectionSink]struct{}{},
		owners: map[port.ConnectionSink]domain.UserID{},
	}
}

var (
	_ port.MessageDelivery    = (*Hub)(nil)
	_ port.ConnectionRegistry = (*Hub)(nil)
)

// Register は、接続を、利用者の端末として登録する。
func (h *Hub) Register(_ context.Context, user domain.UserID, sink port.ConnectionSink) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeLocked(sink)
	sinks, ok := h.byUser[user]
	if !ok {
		sinks = map[port.ConnectionSink]struct{}{}
		h.byUser[user] = sinks
	}
	sinks[sink] = struct{}{}
	h.owners[sink] = user
}

// Unregister は、接続の登録を解く。登録されていない接続には、何もしない。
func (h *Hub) Unregister(_ context.Context, sink port.ConnectionSink) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeLocked(sink)
}

func (h *Hub) removeLocked(sink port.ConnectionSink) {
	user, ok := h.owners[sink]
	if !ok {
		return
	}
	delete(h.owners, sink)
	delete(h.byUser[user], sink)
	if len(h.byUser[user]) == 0 {
		delete(h.byUser, user)
	}
}

// Deliver は、メッセージを、participants がアプリを開いている端末のすべてに届ける。
// 送信待ちの列に加えるまでを、呼び出しの中で終える。端末への書き込みは、待たない。
// 列に加えられなかった端末の接続は、閉じる。登録は、受け付けの adapter が、接続の終わりとして解く。
func (h *Hub) Deliver(_ context.Context, participants []domain.User, msg domain.Message) {
	data, ok := encodeMessageEvent(participants, msg)
	if !ok {
		h.logger.Error("delivery skipped", "reason", "message_not_encodable")
		return
	}

	// Send と Close は、どちらも待たないため、排他の制御を保ったまま呼ぶ。
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, participant := range participants {
		for sink := range h.byUser[participant.ID] {
			if !sink.Send(data) {
				sink.Close()
			}
		}
	}
}

// encodeMessageEvent は、MessageEvent の JSON を組み立てる。
// 送信者が、participants の中に居ない場合は、ok に false を返す。
func encodeMessageEvent(participants []domain.User, msg domain.Message) (data []byte, ok bool) {
	message, ok := toMessage(participants, msg)
	if !ok {
		return nil, false
	}
	data, err := json.Marshal(openapi.MessageEvent{
		Type:    openapi.MessageEventTypeMessage,
		Message: message,
		SendId:  msg.SendID.String(),
	})
	if err != nil {
		return nil, false
	}
	return data, true
}

// toMessage は、domain のメッセージを、通信の形式に変換する。
// 同じ変換が、HTTP の操作を受ける adapter(httpapi)にもある。2つの adapter は、互いに import できない
// ためである。食い違いは、サーバーの振る舞いのテストで検出する(ADR-005 の Consequences)。
func toMessage(participants []domain.User, msg domain.Message) (openapi.Message, bool) {
	if len(participants) != 2 {
		return openapi.Message{}, false
	}

	var sender *domain.User
	for i := range participants {
		if participants[i].ID == msg.SenderID {
			sender = &participants[i]
		}
	}
	if sender == nil {
		return openapi.Message{}, false
	}

	// participants は、識別名の昇順とする。
	handles := []string{participants[0].Handle, participants[1].Handle}
	if handles[0] > handles[1] {
		handles[0], handles[1] = handles[1], handles[0]
	}

	return openapi.Message{
		Participants: handles,
		Seq:          msg.Seq,
		Sender:       openapi.UserProfile{Handle: sender.Handle, DisplayName: sender.DisplayName},
		Body:         msg.Body.String(),
		SentAt:       msg.SentAt.UTC(),
	}, true
}
