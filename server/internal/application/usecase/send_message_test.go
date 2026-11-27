package usecase

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type sendFixture struct {
	*world
	sender *MessageSender
}

func newSendFixture() *sendFixture {
	w := newWorld().loginAll()
	return &sendFixture{world: w, sender: w.messageSender()}
}

func TestSend_StoresAndDelivers(t *testing.T) {
	f := newSendFixture()

	out, err := f.sender.Send(context.Background(), SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}

	want := MessageView{
		Participants:      []string{"alice", "bob"}, // 識別名の昇順。識別子の順(bob、alice)ではない
		Seq:               1,
		SenderHandle:      "alice",
		SenderDisplayName: "Alice",
		Body:              "hello",
		SentAt:            f.store.now,
	}
	if !reflect.DeepEqual(out.Message, want) {
		t.Fatalf("出力が違う:\n got %+v\nwant %+v", out.Message, want)
	}

	// 会話の成立、追記の直列化、メッセージの追記を、この順に、1つのトランザクションで行う。
	if got, want := f.store.calls, []string{"EstablishDirect", "SerializeAppends", "AppendMessage"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("トランザクションの中の操作が違う: got %v, want %v", got, want)
	}

	// 配信は、保管が確定した後に、参加者の2人を宛先として、1回だけ行う。
	if len(f.delivery.deliveries) != 1 {
		t.Fatalf("配信の回数が違う: %d", len(f.delivery.deliveries))
	}
	d := f.delivery.deliveries[0]
	if d.duringTx {
		t.Fatal("トランザクションの中で配信した")
	}
	if len(d.participants) != 2 || d.participants[0] != alice || d.participants[1] != bob {
		t.Fatalf("配信の参加者が違う: %+v", d.participants)
	}
	// 配信には、時刻の源から得た時刻を渡す(期限を過ぎたログイン状態による接続に、届けないため)。
	if !d.now.Equal(testNow) {
		t.Fatalf("配信に渡した時刻が違う: %v", d.now)
	}
	if d.msg.Seq != 1 || d.msg.SenderID != alice.ID || d.msg.SendID.String() != sendID1 || d.msg.Body.String() != "hello" {
		t.Fatalf("配信したメッセージが違う: %+v", d.msg)
	}
}

func TestSend_BothDirectionsShareOneConversation(t *testing.T) {
	f := newSendFixture()
	ctx := context.Background()

	first, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.sender.Send(ctx, SendMessageInput{Actor: as("bob"), Peer: "alice", SendID: sendID2, Body: "two"})
	if err != nil {
		t.Fatal(err)
	}

	if len(f.store.conversations) != 1 {
		t.Fatalf("会話の数が違う: %d", len(f.store.conversations))
	}
	if first.Message.Seq != 1 || second.Message.Seq != 2 {
		t.Fatalf("連番が違う: %d, %d", first.Message.Seq, second.Message.Seq)
	}
	if !reflect.DeepEqual(second.Message.Participants, []string{"alice", "bob"}) {
		t.Fatalf("参加者が違う: %v", second.Message.Participants)
	}
	if second.Message.SenderHandle != "bob" || second.Message.SenderDisplayName != "Bob" {
		t.Fatalf("送信者が違う: %+v", second.Message)
	}
}

func TestSend_NormalizesNewlines(t *testing.T) {
	f := newSendFixture()

	out, err := f.sender.Send(context.Background(), SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "a\r\nb\rc"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message.Body != "a\nb\nc" {
		t.Fatalf("改行が正規化されていない: %q", out.Message.Body)
	}
	if got := f.store.messages[0].Body.String(); got != "a\nb\nc" {
		t.Fatalf("保管した本文が、正規化されていない: %q", got)
	}
}

// 拒否は、仕様「メッセージを送る」の表の順に判定する。
// 拒否した送信は、保管せず、配信せず、会話も成立させない。
func TestSend_Rejections(t *testing.T) {
	tooLong := strings.Repeat("a", 4001)

	tests := []struct {
		name string
		in   SendMessageInput
		want Rejection
	}{
		{"利用者が添えられていない", SendMessageInput{Actor: as(""), Peer: "bob", SendID: sendID1, Body: "hello"}, Unauthenticated},
		{"当たるログイン状態がない", SendMessageInput{Actor: as("mallory"), Peer: "bob", SendID: sendID1, Body: "hello"}, Unauthenticated},
		{"利用者の確認は、本文の検証より先", SendMessageInput{Actor: as("mallory"), Peer: "nobody", SendID: "x", Body: ""}, Unauthenticated},
		{"ログイン状態の確認は、形式の確認より先", SendMessageInput{Actor: as("mallory"), Peer: "bob", Malformed: true}, Unauthenticated},
		{"形式に合わない", SendMessageInput{Actor: as("alice"), Peer: "bob", Malformed: true}, InvalidInput},
		{"形式の確認は、相手より先", SendMessageInput{Actor: as("alice"), Peer: "alice", Malformed: true}, InvalidInput},
		{"本文が空", SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: ""}, InvalidBody},
		{"本文が長すぎる", SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: tooLong}, InvalidBody},
		{"本文の検証は、送信の識別子、相手より先", SendMessageInput{Actor: as("alice"), Peer: "nobody", SendID: "x", Body: " "}, InvalidBody},
		{"送信の識別子の形式が正しくない", SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: "not-a-uuid", Body: "hello"}, InvalidInput},
		{"送信の識別子の検証は、相手より先", SendMessageInput{Actor: as("alice"), Peer: "alice", SendID: "x", Body: "hello"}, InvalidInput},
		{"自分自身を相手に指定した", SendMessageInput{Actor: as("alice"), Peer: "alice", SendID: sendID1, Body: "hello"}, InvalidPeer},
		{"自分自身(大文字と小文字を違えた)", SendMessageInput{Actor: as("alice"), Peer: "ALICE", SendID: sendID1, Body: "hello"}, InvalidPeer},
		{"相手の指定が、識別名の規則を満たさない", SendMessageInput{Actor: as("alice"), Peer: "no such user!", SendID: sendID1, Body: "hello"}, NotFound},
		{"相手が、登録を完了した利用者の中に居ない", SendMessageInput{Actor: as("alice"), Peer: "nobody", SendID: sendID1, Body: "hello"}, NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSendFixture()

			_, err := f.sender.Send(context.Background(), tt.in)

			requireRejection(t, err, tt.want)
			if len(f.store.calls) != 0 || len(f.store.conversations) != 0 || len(f.store.messages) != 0 {
				t.Fatalf("拒否した送信で、保管先を変えた: calls %v", f.store.calls)
			}
			if len(f.delivery.deliveries) != 0 {
				t.Fatal("拒否した送信を、配信した")
			}
		})
	}
}

func TestSend_Duplicate_CompletesWithStoredMessageWithoutDelivering(t *testing.T) {
	f := newSendFixture()
	ctx := context.Background()

	first, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "original"})
	if err != nil {
		t.Fatal(err)
	}

	// 送り直し。識別子は、大文字でも一致する。本文が異なっていても、最初に保管した内容を保つ。
	again, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: strings.ToUpper(sendID1), Body: "changed"})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(again, first) {
		t.Fatalf("保管済みのメッセージと違う:\n got %+v\nwant %+v", again, first)
	}
	if len(f.store.messages) != 1 {
		t.Fatalf("メッセージの数が違う: %d", len(f.store.messages))
	}
	if len(f.delivery.deliveries) != 1 {
		t.Fatalf("送り直しで、再び配信した: %d", len(f.delivery.deliveries))
	}
}

func TestSend_Duplicate_InAnotherConversation_IsRejected(t *testing.T) {
	f := newSendFixture()
	ctx := context.Background()

	if _, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "to bob"}); err != nil {
		t.Fatal(err)
	}
	// alice と carol の会話を、別の識別子で成立させておく。
	if _, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "carol", SendID: sendID2, Body: "to carol"}); err != nil {
		t.Fatal(err)
	}

	_, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "carol", SendID: sendID1, Body: "to carol again"})

	requireRejection(t, err, InvalidInput)
	if len(f.store.messages) != 2 {
		t.Fatalf("メッセージの数が違う: %d", len(f.store.messages))
	}
	if got := f.store.messages[0].Body.String(); got != "to bob" {
		t.Fatalf("保管済みのメッセージが変わった: %q", got)
	}
	if len(f.delivery.deliveries) != 2 {
		t.Fatalf("拒否した送信を、配信した: %d", len(f.delivery.deliveries))
	}
}

func TestSend_Duplicate_WithInvalidBody_IsRejectedAsInvalidBody(t *testing.T) {
	f := newSendFixture()
	ctx := context.Background()

	if _, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "original"}); err != nil {
		t.Fatal(err)
	}

	_, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "   "})

	requireRejection(t, err, InvalidBody)
	if got := f.store.messages[0].Body.String(); got != "original" {
		t.Fatalf("保管済みのメッセージが変わった: %q", got)
	}
}

func TestSend_SameSendIDFromAnotherSender_IsAnotherMessage(t *testing.T) {
	f := newSendFixture()
	ctx := context.Background()

	if _, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "from alice"}); err != nil {
		t.Fatal(err)
	}
	out, err := f.sender.Send(ctx, SendMessageInput{Actor: as("bob"), Peer: "alice", SendID: sendID1, Body: "from bob"})
	if err != nil {
		t.Fatal(err)
	}

	if out.Message.Seq != 2 || out.Message.Body != "from bob" || out.Message.SenderHandle != "bob" {
		t.Fatalf("別のメッセージとして保管されていない: %+v", out.Message)
	}
	if len(f.delivery.deliveries) != 2 {
		t.Fatalf("配信の回数が違う: %d", len(f.delivery.deliveries))
	}
}

// 保管に失敗した送信は、完了として示さず、配信しない(WS-31)。
func TestSend_StoreFailure_DoesNotDeliver(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(f *sendFixture)
	}{
		{"追記の失敗", func(f *sendFixture) { f.store.appendErr = errBoom }},
		{"確定の失敗", func(f *sendFixture) { f.store.commitErr = errBoom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSendFixture()
			tt.prepare(f)

			_, err := f.sender.Send(context.Background(), SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "hello"})

			requireFailure(t, err)
			if len(f.delivery.deliveries) != 0 {
				t.Fatal("保管に失敗した送信を、配信した")
			}
		})
	}
}

func TestSend_Duplicate_LookupFailure_IsFailure(t *testing.T) {
	f := newSendFixture()
	ctx := context.Background()
	if _, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "original"}); err != nil {
		t.Fatal(err)
	}
	f.store.findErr = errBoom

	_, err := f.sender.Send(ctx, SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "original"})

	requireFailure(t, err)
}

func TestSend_UserDirectoryFailure_IsFailure(t *testing.T) {
	f := newSendFixture()
	f.users.err = errBoom

	_, err := f.sender.Send(context.Background(), SendMessageInput{Actor: as("alice"), Peer: "bob", SendID: sendID1, Body: "hello"})

	requireFailure(t, err)
	if len(f.store.calls) != 0 {
		t.Fatalf("保管先を呼んだ: %v", f.store.calls)
	}
}
