package usecase

import (
	"context"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

func TestAccept_RegistersConnectionForUser(t *testing.T) {
	registry := newFakeRegistry()
	keeper := NewConnectionKeeper(newFakeUsers(), registry)
	sink := &fakeSink{}

	if err := keeper.Accept(context.Background(), AcceptConnectionInput{Actor: "alice", Connection: sink}); err != nil {
		t.Fatal(err)
	}

	if got, ok := registry.registered[sink]; !ok || got != alice.ID {
		t.Fatalf("接続が、利用者の識別子で登録されていない: %q, %v", got, ok)
	}
}

func TestAccept_RejectsUnknownUserWithoutRegistering(t *testing.T) {
	for _, actor := range []string{"", "mallory"} {
		registry := newFakeRegistry()
		keeper := NewConnectionKeeper(newFakeUsers(), registry)

		err := keeper.Accept(context.Background(), AcceptConnectionInput{Actor: actor, Connection: &fakeSink{}})

		requireRejection(t, err, Unauthenticated)
		if len(registry.registered) != 0 {
			t.Fatalf("利用者を特定できない接続を、登録した(%q)", actor)
		}
	}
}

func TestAccept_UserDirectoryFailure_IsFailure(t *testing.T) {
	users := newFakeUsers()
	users.err = errBoom
	registry := newFakeRegistry()
	keeper := NewConnectionKeeper(users, registry)

	err := keeper.Accept(context.Background(), AcceptConnectionInput{Actor: "alice", Connection: &fakeSink{}})

	requireFailure(t, err)
	if len(registry.registered) != 0 {
		t.Fatal("接続を登録した")
	}
}

func TestClose_UnregistersConnection(t *testing.T) {
	registry := newFakeRegistry()
	keeper := NewConnectionKeeper(newFakeUsers(), registry)
	sink := &fakeSink{}
	ctx := context.Background()
	if err := keeper.Accept(ctx, AcceptConnectionInput{Actor: "alice", Connection: sink}); err != nil {
		t.Fatal(err)
	}

	keeper.Close(ctx, CloseConnectionInput{Connection: sink})

	if len(registry.registered) != 0 {
		t.Fatal("登録が解かれていない")
	}
	if len(registry.unregistered) != 1 || registry.unregistered[0] != port.ConnectionSink(sink) {
		t.Fatalf("登録を解いた接続が違う: %v", registry.unregistered)
	}
}
