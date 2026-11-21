package main

import (
	"net/http"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// WS-23、WS-24、WS-26: carol は、alice と bob の会話のメッセージを、提供されるどの操作によっても
// 取得できず、その会話へ送信できない。要求は、サーバーへ直接に送る。
//
// 本スライスの操作は、会話を、相手の識別名で指定する。会話の識別子で指定する操作は、ない。
// carol が行える操作は、alice、bob のそれぞれを相手とする、履歴の取得と送信である。
func TestNonParticipant_CannotReadOrWriteConversation(t *testing.T) {
	s := startServer(t)
	aliceConn := s.connect("alice")
	bobConn := s.connect("bob")
	original := []openapi.Message{
		s.mustSend("alice", "bob", "secret one"),
		s.mustSend("bob", "alice", "secret two"),
	}
	for _, msg := range original {
		aliceConn.requireNext(msg)
		bobConn.requireNext(msg)
	}

	// 履歴の取得: carol が得るのは、carol 自身の会話(まだ成立していない)だけである。
	for _, peer := range []string{"alice", "bob"} {
		if got := s.mustHistory("carol", peer); len(got) != 0 {
			t.Fatalf("carol が、参加者でない会話のメッセージを得た: %+v", got)
		}
	}
	// 自分以外の利用者を、操作を行う利用者として指定することは、仮のログインでは防がない(WS-P2)。
	// 相手として、会話(2人の組)を指定する形はない。次は、いずれも、存在しない相手と同じ応答になる。
	for _, peer := range []string{"alice,bob", "alice%2Fbob", "bob/messages/../../alice", "*"} {
		resp := s.history("carol", peer)
		if resp.Status != http.StatusNotFound || string(resp.Body) != `{"code":"not_found"}` {
			t.Fatalf("相手 %q の応答が、存在しない相手の応答と違う: %d %s", peer, resp.Status, resp.Body)
		}
	}

	// 送信: carol の送信は、carol 自身の会話に保管され、alice と bob の会話には増えない。
	toAlice := s.mustSend("carol", "alice", "from carol to alice")
	toBob := s.mustSend("carol", "bob", "from carol to bob")
	if toAlice.Seq != 1 || toBob.Seq != 1 {
		t.Fatalf("carol の送信が、新しい会話の最初のメッセージではない: %d, %d", toAlice.Seq, toBob.Seq)
	}
	for _, viewer := range [][2]string{{"alice", "bob"}, {"bob", "alice"}} {
		got := s.mustHistory(viewer[0], viewer[1])
		if canonicalJSON(got) != canonicalJSON(original) {
			t.Fatalf("alice と bob の会話が変わった: %+v", got)
		}
	}
	// carol の会話の履歴にも、alice と bob の会話のメッセージは、現れない。
	if got := s.mustHistory("carol", "alice"); len(got) != 1 || !sameMessage(got[0], toAlice) {
		t.Fatalf("carol と alice の会話が違う: %+v", got)
	}
	if got := s.mustHistory("carol", "bob"); len(got) != 1 || !sameMessage(got[0], toBob) {
		t.Fatalf("carol と bob の会話が違う: %+v", got)
	}
	s.requireStored(3, 4)
}

// WS-41: 端末で、固定ユーザーを alice から carol へ選び直した後、その端末には、
// alice が参加し carol が参加しない会話のメッセージが、届かない。
func TestReconnectAsAnotherUser_ReceivesOnlyNewUsersMessages(t *testing.T) {
	s := startServer(t)

	// alice としての接続。配信が届くことを、確かめておく。
	asAlice := s.connect("alice")
	asAlice.requireNext(s.mustSend("bob", "alice", "while alice"))

	// 選び直し: alice としての接続を閉じ、carol として、接続を確立し直す。
	asAlice.close()
	asCarol := s.connect("carol")

	s.mustSend("alice", "bob", "after reselect, from alice")
	s.mustSend("bob", "alice", "after reselect, from bob")

	// 番兵: carol としての接続が次に受け取るのは、carol が参加する会話のメッセージである。
	sentinel := s.mustSend("alice", "carol", "sentinel")
	asCarol.requireNext(sentinel)
}

// 同じ固定ユーザーを、複数の端末で同時に選べる。端末の1つが閉じても、他の端末には届く。
func TestDelivery_ContinuesAfterOneDeviceCloses(t *testing.T) {
	s := startServer(t)
	bob1 := s.connect("bob")
	bob2 := s.connect("bob")

	bob1.close()
	msg := s.mustSend("alice", "bob", "hello")

	bob2.requireNext(msg)
}
