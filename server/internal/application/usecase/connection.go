package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// ConnectionKeeper は、「リアルタイムの接続を受け付ける」と「接続の終わりを受ける」ユースケースである。
type ConnectionKeeper struct {
	users    port.UserDirectory
	registry port.ConnectionRegistry
}

// NewConnectionKeeper は、ConnectionKeeper を作る。
func NewConnectionKeeper(users port.UserDirectory, registry port.ConnectionRegistry) *ConnectionKeeper {
	return &ConnectionKeeper{users: users, registry: registry}
}

// Accept は、利用者を確かめた後に、接続を、配信の宛先として登録する。
func (k *ConnectionKeeper) Accept(ctx context.Context, in AcceptConnectionInput) error {
	user, found, err := k.users.FindByHandle(ctx, in.Actor)
	if err != nil {
		return err
	}
	if !found {
		return rejected(Unauthenticated)
	}
	k.registry.Register(ctx, user.ID, in.Connection)
	return nil
}

// Close は、接続の登録を解く。
func (k *ConnectionKeeper) Close(ctx context.Context, in CloseConnectionInput) {
	k.registry.Unregister(ctx, in.Connection)
}
