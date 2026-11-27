// Package wsdelivery は、配信を行う adapter である。
// port.MessageDelivery、port.ConnectionRegistry、port.SessionConnections を実装する(ADR-005、ADR-014)。
//
// 接続中の端末と、配信の宛先と、接続とログイン状態の対応を保持する状態は、このパッケージだけが持つ
// (PERF-3、WS-35、RL-39)。domain の型を、通信の形式に変換し、接続への書き込み口に渡す。
// WebSocket のライブラリは、使わない。
//
// ログイン状態が有効かどうかは、保持したログイン状態の ValidAt で判定する。期限を、自分では比べない。
// ポートを実装する adapter が、domain の判定を呼ぶ例外である(ADR-020)。
package wsdelivery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// Hub は、接続中の端末を保持し、配信する。
type Hub struct {
	logger *slog.Logger

	mu        sync.Mutex
	sessions  map[port.ConnectionSink]domain.Session // 書き込み口ごとの、ログイン状態(利用者、識別子、期限)
	byUser    map[domain.UserID]map[port.ConnectionSink]struct{}
	bySession map[domain.SessionID]map[port.ConnectionSink]struct{}
}

// NewHub は、Hub を作る。
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger:    logger,
		sessions:  map[port.ConnectionSink]domain.Session{},
		byUser:    map[domain.UserID]map[port.ConnectionSink]struct{}{},
		bySession: map[domain.SessionID]map[port.ConnectionSink]struct{}{},
	}
}

var (
	_ port.MessageDelivery    = (*Hub)(nil)
	_ port.ConnectionRegistry = (*Hub)(nil)
	_ port.SessionConnections = (*Hub)(nil)
)

// Register は、接続を、ログイン状態による、利用者の端末として登録する。
func (h *Hub) Register(_ context.Context, session domain.Session, sink port.ConnectionSink) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeLocked(sink)
	h.sessions[sink] = session
	addTo(h.byUser, session.UserID, sink)
	addTo(h.bySession, session.ID, sink)
}

// Unregister は、接続の登録を解く。登録されていない接続には、何もしない。
func (h *Hub) Unregister(_ context.Context, sink port.ConnectionSink) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeLocked(sink)
}

// CloseBySession は、そのログイン状態による接続のすべてに、ログイン状態が無効になったことを伝えて、
// 閉じ、登録を解く。閉じた接続は、配信の宛先から外れる(RL-18)。
func (h *Hub) CloseBySession(_ context.Context, id domain.SessionID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sink := range h.bySession[id] {
		h.endLocked(sink)
	}
}

// CloseExpired は、now の時点で、期限を過ぎたログイン状態による接続に、同じことを行う。
func (h *Hub) CloseExpired(_ context.Context, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sink, session := range h.sessions {
		if !session.ValidAt(now) {
			h.endLocked(sink)
		}
	}
}

// endLocked は、接続に、ログイン状態が無効になったことを伝えて、閉じ、登録を解く。
func (h *Hub) endLocked(sink port.ConnectionSink) {
	sink.CloseSessionEnded()
	h.removeLocked(sink)
}

func (h *Hub) removeLocked(sink port.ConnectionSink) {
	session, ok := h.sessions[sink]
	if !ok {
		return
	}
	delete(h.sessions, sink)
	removeFrom(h.byUser, session.UserID, sink)
	removeFrom(h.bySession, session.ID, sink)
}

func addTo[K comparable](index map[K]map[port.ConnectionSink]struct{}, key K, sink port.ConnectionSink) {
	sinks, ok := index[key]
	if !ok {
		sinks = map[port.ConnectionSink]struct{}{}
		index[key] = sinks
	}
	sinks[sink] = struct{}{}
}

func removeFrom[K comparable](index map[K]map[port.ConnectionSink]struct{}, key K, sink port.ConnectionSink) {
	delete(index[key], sink)
	if len(index[key]) == 0 {
		delete(index, key)
	}
}

// Deliver は、メッセージを、participants がアプリを開いている端末のすべてに届ける。
// 送信待ちの列に加えるまでを、呼び出しの中で終える。端末への書き込みは、待たない。
// 列に加えられなかった端末の接続は、閉じる。登録は、受け付けの adapter が、接続の終わりとして解く。
// now の時点で、期限を過ぎたログイン状態による接続には、届けずに、閉じる(RL-18)。
func (h *Hub) Deliver(_ context.Context, now time.Time, participants []domain.User, msg domain.Message) {
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
			if !h.sessions[sink].ValidAt(now) {
				h.endLocked(sink)
				continue
			}
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
	handles := []string{participants[0].Handle.String(), participants[1].Handle.String()}
	if handles[0] > handles[1] {
		handles[0], handles[1] = handles[1], handles[0]
	}

	return openapi.Message{
		Participants: handles,
		Seq:          msg.Seq,
		Sender:       openapi.UserProfile{Handle: sender.Handle.String(), DisplayName: sender.DisplayName.String()},
		Body:         msg.Body.String(),
		SentAt:       msg.SentAt.UTC(),
	}, true
}
