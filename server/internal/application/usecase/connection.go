package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// ConnectionKeeper は、「リアルタイムの接続を受け付ける」と「接続の終わりを受ける」ユースケースである。
type ConnectionKeeper struct {
	gate     *SessionGate
	registry port.ConnectionRegistry
	events   port.EventRecorder
}

// NewConnectionKeeper は、ConnectionKeeper を作る。
func NewConnectionKeeper(gate *SessionGate, registry port.ConnectionRegistry, events port.EventRecorder) *ConnectionKeeper {
	return &ConnectionKeeper{gate: gate, registry: registry, events: events}
}

// Accept は、ログイン状態を確かめた後に、接続を、ログイン状態と対応付けて、配信の宛先として登録する。
func (k *ConnectionKeeper) Accept(ctx context.Context, in AcceptConnectionInput) (err error) {
	a := begin(k.events, "accept connection", in.Source)
	defer a.finish(ctx, &err)

	session, _, err := k.gate.verify(ctx, a, in.Actor)
	if err != nil {
		return err
	}
	// 形式に合わない要求は、ログイン状態の確認のすぐ後に、拒否する。接続は、登録しない。
	if in.Malformed {
		return rejected(InvalidInput)
	}
	k.registry.Register(ctx, session, in.Connection)

	// 登録した後に、もう一度確かめる。確かめてから登録するまでの間に、ログアウトが行われると、
	// 登録が、接続を閉じる処理の後になり、無効なログイン状態の接続が、期限まで残るためである。
	if _, _, err := k.gate.verify(ctx, a, in.Actor); err != nil {
		k.registry.Unregister(ctx, in.Connection)
		return err
	}
	return nil
}

// Close は、接続の登録を解く。
func (k *ConnectionKeeper) Close(ctx context.Context, in CloseConnectionInput) {
	k.registry.Unregister(ctx, in.Connection)
}
