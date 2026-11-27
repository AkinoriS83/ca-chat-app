package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// Housekeeper は、片付けのユースケースである。
// 期限に達したログイン状態による接続を閉じ、期限を過ぎたログイン状態と、登録の申し込みを消す。
// 操作を行う利用者は、居ない(「接続の終わりを受ける」と同じである)。
type Housekeeper struct {
	connections port.SessionConnections
	sessions    port.SessionStore
	signups     port.SignupStore
	clock       port.Clock
	events      port.EventRecorder
}

// NewHousekeeper は、Housekeeper を作る。
func NewHousekeeper(connections port.SessionConnections, sessions port.SessionStore, signups port.SignupStore, clock port.Clock, events port.EventRecorder) *Housekeeper {
	return &Housekeeper{connections: connections, sessions: sessions, signups: signups, clock: clock, events: events}
}

// Sweep は、片付けを、1回行う。どれかがエラーを返した場合は、そこで戻る。残りは、次の回で、やり直す。
// 削除は、何度行っても、同じ結果になる。
func (h *Housekeeper) Sweep(ctx context.Context) (err error) {
	a := begin(h.events, "sweep", "")
	defer a.finish(ctx, &err)

	now := h.clock.Now()
	h.connections.CloseExpired(ctx, now)
	if err := h.sessions.DeleteExpired(ctx, now); err != nil {
		return err
	}
	return h.signups.DeleteExpired(ctx, now)
}
