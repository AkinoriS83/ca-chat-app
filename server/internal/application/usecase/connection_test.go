package usecase

import (
	"context"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// ADR-014: 接続を、利用者に加えて、ログイン状態と対応付けて登録する。
func TestAccept_RegistersConnectionWithSession(t *testing.T) {
	w := newWorld().loginAll()
	sink := &fakeSink{}

	if err := w.connectionKeeper().Accept(context.Background(), AcceptConnectionInput{Actor: as("alice"), Source: source, Connection: sink}); err != nil {
		t.Fatal(err)
	}

	want := w.sessions.byCredential[as("alice").Credential]
	if got, ok := w.registry.registered[sink]; !ok || got != want || got.UserID != alice.ID {
		t.Fatalf("接続が、ログイン状態と対応付けて登録されていない: %+v, %v", got, ok)
	}
	// 登録の前と、後の、2回、確かめる(DD-26)。
	if w.sessions.finds != 2 {
		t.Fatalf("ログイン状態を確かめた回数が違う: %d", w.sessions.finds)
	}
	requireEvents(t, w.events)
}

// RL-15: ログイン状態が有効でない要求は、接続を登録せずに、拒否する。形式に合わない要求でも、同じである。
func TestAccept_RejectsUnauthenticatedWithoutRegistering(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		for _, actor := range []Actor{as(""), as("mallory"), {Credential: as("alice").Credential, SessionID: as("bob").SessionID}} {
			w := newWorld().loginAll()

			err := w.connectionKeeper().Accept(context.Background(), AcceptConnectionInput{Actor: actor, Source: source, Malformed: malformed, Connection: &fakeSink{}})

			requireRejection(t, err, Unauthenticated)
			if len(w.registry.registered) != 0 {
				t.Fatalf("ログイン状態が有効でない接続を、登録した(%+v)", actor)
			}
			requireEvents(t, w.events, domain.EventAuthenticationRejected)
		}
	}
}

// DD-18: WebSocket の接続を確立する要求でない要求は、ログイン状態の確認の後に、拒否する。接続は、登録しない。
func TestAccept_RejectsMalformedAfterAuthentication(t *testing.T) {
	w := newWorld().loginAll()

	err := w.connectionKeeper().Accept(context.Background(), AcceptConnectionInput{Actor: as("alice"), Source: source, Malformed: true, Connection: &fakeSink{}})

	requireRejection(t, err, InvalidInput)
	if len(w.registry.registered) != 0 || len(w.registry.unregistered) != 0 {
		t.Fatal("形式に合わない要求の接続を、登録した")
	}
}

// DD-26: 確かめてから登録するまでの間に、ログイン状態が無効になった接続は、登録を解いて、拒否する。
func TestAccept_SessionRevokedBetweenVerificationAndRegistration(t *testing.T) {
	w := newWorld().loginAll()
	sink := &fakeSink{}
	// 最初の確認が戻った直後に、ログアウトが行われる。
	w.sessions.afterFind = func() {
		delete(w.sessions.byCredential, as("alice").Credential)
	}

	err := w.connectionKeeper().Accept(context.Background(), AcceptConnectionInput{Actor: as("alice"), Source: source, Connection: sink})

	requireRejection(t, err, Unauthenticated)
	if len(w.registry.registered) != 0 {
		t.Fatal("無効なログイン状態の接続が、登録されたまま残った")
	}
	if len(w.registry.unregistered) != 1 || w.registry.unregistered[0] != port.ConnectionSink(sink) {
		t.Fatalf("登録を解いた接続が違う: %v", w.registry.unregistered)
	}
}

// RL-23: ログイン状態の確認を完了できなかった接続は、受け付けない。登録の後の確認でも、同じである。
func TestAccept_SessionStoreFailure(t *testing.T) {
	for _, failFrom := range []int{1, 2} {
		w := newWorld().loginAll()
		w.sessions.failFindFrom = failFrom

		err := w.connectionKeeper().Accept(context.Background(), AcceptConnectionInput{Actor: as("alice"), Source: source, Connection: &fakeSink{}})

		requireFailure(t, err)
		if len(w.registry.registered) != 0 {
			t.Fatalf("接続を登録した(%d 回目の確認の失敗)", failFrom)
		}
		requireEvents(t, w.events, domain.EventOperationFailed)
	}
}

func TestClose_UnregistersConnection(t *testing.T) {
	w := newWorld().loginAll()
	keeper := w.connectionKeeper()
	sink := &fakeSink{}
	ctx := context.Background()
	if err := keeper.Accept(ctx, AcceptConnectionInput{Actor: as("alice"), Source: source, Connection: sink}); err != nil {
		t.Fatal(err)
	}

	keeper.Close(ctx, CloseConnectionInput{Connection: sink})

	if len(w.registry.registered) != 0 {
		t.Fatal("登録が解かれていない")
	}
	if len(w.registry.unregistered) != 1 || w.registry.unregistered[0] != port.ConnectionSink(sink) {
		t.Fatalf("登録を解いた接続が違う: %v", w.registry.unregistered)
	}
}
