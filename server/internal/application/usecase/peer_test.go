package usecase

import (
	"context"
	"reflect"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// RL-24: 相手を特定すると、相手の識別名(登録した通り)と表示名を返す。大文字と小文字を、区別しない。
func TestFindPeer_ReturnsHandleAsRegistered(t *testing.T) {
	for _, input := range []string{"Bob_Smith", "bob_smith", "BOB_SMITH"} {
		w := newWorld().loginAll()
		w.users.users = append(w.users.users, testUser("00000000-0000-4000-8000-000000000009", "Bob_Smith", "Bob S."))

		out, err := w.peerFinder().Find(context.Background(), FindPeerInput{Actor: as("alice"), Source: source, Handle: input})
		if err != nil {
			t.Fatal(err)
		}
		if want := (UserView{Handle: "Bob_Smith", DisplayName: "Bob S."}); out.Peer != want {
			t.Fatalf("%q: 出力が違う: %+v", input, out.Peer)
		}
		// 特定は、会話を開かない。成立させない。
		if len(w.store.calls) != 0 || len(w.store.conversations) != 0 {
			t.Fatal("特定で、会話を成立させた")
		}
	}
}

// 仕様「相手を特定する」の拒否の順: ログイン状態 → 識別名の規則 → 自分自身 → 上限 → 居ない。
func TestFindPeer_Rejections(t *testing.T) {
	tests := []struct {
		name     string
		actor    Actor
		handle   string
		exceeded bool
		want     Rejection
		counted  int
	}{
		{"ログイン状態がない", as(""), "bob", false, Unauthenticated, 0},
		{"ログイン状態の確認は、識別名の規則より先", as("mallory"), "!", false, Unauthenticated, 0},
		{"識別名の規則を満たさない(短い)", as("alice"), "bo", false, InvalidInput, 0},
		{"識別名の規則を満たさない(使えない文字)", as("alice"), "bob smith", false, InvalidInput, 0},
		{"識別名の規則は、上限より先", as("alice"), "!", true, InvalidInput, 0},
		{"自分自身", as("alice"), "alice", false, InvalidPeer, 0},
		{"自分自身(大文字と小文字を違えた)", as("alice"), "Alice", false, InvalidPeer, 0},
		{"自分自身は、上限より先", as("alice"), "ALICE", true, InvalidPeer, 0},
		{"上限は、居ないことより先", as("alice"), "nobody", true, RateLimited, 0},
		{"上限の間は、居る相手も、同じく拒否する", as("alice"), "bob", true, RateLimited, 0},
		{"居ない", as("alice"), "nobody", false, NotFound, 1},
		{"識別名の一部", as("alice"), "bo_", false, NotFound, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld().loginAll()
			w.limiter.exceeded[domain.LimitPeerLookupPerUser] = tt.exceeded

			_, err := w.peerFinder().Find(context.Background(), FindPeerInput{Actor: tt.actor, Source: source, Handle: tt.handle})

			requireRejection(t, err, tt.want)
			// 入力の規則を満たさない要求と、自分自身を指定した要求は、数えない。居なかった要求は、数える。
			if len(w.limiter.admitted) != tt.counted {
				t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
			}
			if tt.want == RateLimited {
				requireEvents(t, w.events, domain.EventLimitExceeded)
				if e := w.events.events[0]; e.Limit != domain.LimitPeerLookupPerUser || e.UserID != alice.ID || e.Source != source {
					t.Fatalf("記録の項目が違う: %+v", e)
				}
			}
		})
	}
}

// RL-31: 相手の特定の上限は、特定、会話を開く要求、送信する要求が、同じ単位(利用者)で数える。
// 成立した会話がない相手を指定した要求を、相手が居るかどうかに依らずに、数える。
func TestPeerLookup_IsCountedByAllThreeOperations(t *testing.T) {
	ctx := context.Background()
	w := newWorld().loginAll()
	want := w.rules.PeerLookupByUser(alice.ID)

	if _, err := w.peerFinder().Find(ctx, FindPeerInput{Actor: as("alice"), Source: source, Handle: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	_, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: "nobody"})
	requireRejection(t, err, NotFound)

	if !reflect.DeepEqual(w.limiter.admitted, []domain.Limit{want, want, want, want}) {
		t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
	}

	// 会話が成立した後は、その相手を指定した要求を、数えない。
	w.limiter.admitted = nil
	if _, err := w.peerFinder().Find(ctx, FindPeerInput{Actor: as("alice"), Source: source, Handle: "BOB"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID2, Body: "again"}); err != nil {
		t.Fatal(err)
	}
	// 相手の側からの要求も、同じ会話であるため、数えない。
	if _, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("bob"), Source: source, Peer: "alice"}); err != nil {
		t.Fatal(err)
	}
	if len(w.limiter.admitted) != 0 {
		t.Fatalf("成立した会話がある相手を指定した要求を、数えた: %+v", w.limiter.admitted)
	}
}

// RL-31: 上限に達している間、成立した会話がない相手を指定した要求は、相手が居る場合と、居ない場合で、
// 同じ拒否になる。成立した会話がある相手を指定した要求は、受け付ける。
func TestPeerLookup_WhileRateLimited(t *testing.T) {
	ctx := context.Background()
	w := newWorld().loginAll()
	// alice と bob の間に、会話を成立させておく。
	if _, err := w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	w.limiter.exceeded[domain.LimitPeerLookupPerUser] = true
	deliveries := len(w.delivery.deliveries)

	for _, peer := range []string{"carol", "nobody"} {
		_, err := w.peerFinder().Find(ctx, FindPeerInput{Actor: as("alice"), Source: source, Handle: peer})
		requireRejection(t, err, RateLimited)
		_, err = w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: peer})
		requireRejection(t, err, RateLimited)
		_, err = w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: peer, SendID: sendID2, Body: "hello"})
		requireRejection(t, err, RateLimited)
	}
	if len(w.store.conversations) != 1 || len(w.delivery.deliveries) != deliveries {
		t.Fatal("上限で拒否した送信を、保管したか、配信した")
	}

	// 成立した会話がある相手は、上限に達している間も、受け付ける。
	if _, err := w.peerFinder().Find(ctx, FindPeerInput{Actor: as("alice"), Source: source, Handle: "bob"}); err != nil {
		t.Fatal(err)
	}
	if out, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: "bob"}); err != nil || len(out.Messages) != 1 {
		t.Fatalf("成立した会話の履歴を、読めない: %v", err)
	}
	if _, err := w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID2, Body: "again"}); err != nil {
		t.Fatal(err)
	}
}

// 送信の拒否の順: 本文と、送信の識別子の形式は、上限より先である。数えない。
func TestSend_BodyAndSendIDAreCheckedBeforeLimit(t *testing.T) {
	w := newWorld().loginAll()
	w.limiter.exceeded[domain.LimitPeerLookupPerUser] = true
	sender := w.messageSender()
	ctx := context.Background()

	_, err := sender.Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: " "})
	requireRejection(t, err, InvalidBody)
	_, err = sender.Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: "x", Body: "hello"})
	requireRejection(t, err, InvalidInput)
	_, err = sender.Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "alice", SendID: sendID1, Body: "hello"})
	requireRejection(t, err, InvalidPeer)
	_, err = sender.Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: "hello"})
	requireRejection(t, err, RateLimited)
}

// ログイン状態の確認は、保管先への照会を、操作につき、1回だけ行う。
func TestAuthenticatedOperations_VerifySessionOnce(t *testing.T) {
	ctx := context.Background()
	w := newWorld().loginAll()

	if _, err := w.peerFinder().Find(ctx, FindPeerInput{Actor: as("alice"), Source: source, Handle: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.historyReader().Read(ctx, ReadHistoryInput{Actor: as("alice"), Source: source, Peer: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.messageSender().Send(ctx, SendMessageInput{Actor: as("alice"), Source: source, Peer: "bob", SendID: sendID1, Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	if w.sessions.finds != 3 {
		t.Fatalf("ログイン状態を確かめた回数が違う: %d", w.sessions.finds)
	}
	// 利用者を、識別名で確かめる手順(仮のログイン)は、ない。識別名で探すのは、相手だけである。
	if w.users.handleLookups != 3 {
		t.Fatalf("識別名で、利用者を探した回数が違う: %d", w.users.handleLookups)
	}
}
