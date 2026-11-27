package usecase

import (
	"context"
	"testing"
)

type historyFixture struct {
	*world
	sender *MessageSender
	reader *HistoryReader
}

func newHistoryFixture() *historyFixture {
	w := newWorld().loginAll()
	return &historyFixture{world: w, sender: w.messageSender(), reader: w.historyReader()}
}

func TestRead_ReturnsMessagesInOrderWithSenders(t *testing.T) {
	f := newHistoryFixture()
	ctx := context.Background()
	for _, in := range []SendMessageInput{
		{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "one"},
		{Actor: as("bob"), Peer: "alice", SendID: sendID2, Body: "two"},
	} {
		if _, err := f.sender.Send(ctx, in); err != nil {
			t.Fatal(err)
		}
	}

	// どちらが開いても、同じ会話の、同じ履歴を得る。
	for _, in := range []ReadHistoryInput{{Actor: as("alice"), Peer: "bob"}, {Actor: as("bob"), Peer: "alice"}} {
		out, err := f.reader.Read(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Messages) != 2 {
			t.Fatalf("メッセージの数が違う: %d", len(out.Messages))
		}
		first, second := out.Messages[0], out.Messages[1]
		if first.Seq != 1 || first.Body != "one" || first.SenderHandle != "alice" || first.SenderDisplayName != "Alice" {
			t.Fatalf("1通目が違う: %+v", first)
		}
		if second.Seq != 2 || second.Body != "two" || second.SenderHandle != "bob" || second.SenderDisplayName != "Bob" {
			t.Fatalf("2通目が違う: %+v", second)
		}
		if first.Participants[0] != "alice" || first.Participants[1] != "bob" {
			t.Fatalf("参加者が違う: %v", first.Participants)
		}
		if !first.SentAt.Equal(f.store.now) {
			t.Fatalf("送信の日時が違う: %v", first.SentAt)
		}
	}
}

func TestRead_UnestablishedConversation_ReturnsEmptyWithoutEstablishing(t *testing.T) {
	f := newHistoryFixture()

	out, err := f.reader.Read(context.Background(), ReadHistoryInput{Actor: as("alice"), Peer: "bob"})
	if err != nil {
		t.Fatal(err)
	}

	if out.Messages == nil || len(out.Messages) != 0 {
		t.Fatalf("空の一覧ではない: %#v", out.Messages)
	}
	if len(f.store.conversations) != 0 || len(f.store.calls) != 0 {
		t.Fatal("履歴の取得で、会話を成立させた")
	}
}

func TestRead_DoesNotReturnOtherConversations(t *testing.T) {
	f := newHistoryFixture()
	ctx := context.Background()
	if _, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "secret"}); err != nil {
		t.Fatal(err)
	}

	for _, in := range []ReadHistoryInput{{Actor: as("carol"), Peer: "alice"}, {Actor: as("carol"), Peer: "bob"}} {
		out, err := f.reader.Read(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Messages) != 0 {
			t.Fatalf("参加者でない会話のメッセージを得た: %+v", out.Messages)
		}
	}
}

func TestRead_Rejections(t *testing.T) {
	tests := []struct {
		name string
		in   ReadHistoryInput
		want Rejection
	}{
		{"利用者が添えられていない", ReadHistoryInput{Actor: as(""), Peer: "bob"}, Unauthenticated},
		{"当たるログイン状態がない", ReadHistoryInput{Actor: as("mallory"), Peer: "nobody"}, Unauthenticated},
		{"相手が、登録を完了した利用者の中に居ない", ReadHistoryInput{Actor: as("alice"), Peer: "nobody"}, NotFound},
		{"自分自身を相手に指定した", ReadHistoryInput{Actor: as("alice"), Peer: "alice"}, InvalidPeer},
		{"自分自身(大文字と小文字を違えた)", ReadHistoryInput{Actor: as("alice"), Peer: "Alice"}, InvalidPeer},
		{"相手の指定が、識別名の規則を満たさない", ReadHistoryInput{Actor: as("alice"), Peer: "a b"}, NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newHistoryFixture()

			_, err := f.reader.Read(context.Background(), tt.in)

			requireRejection(t, err, tt.want)
		})
	}
}
