package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres/pgtest"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

const (
	userA domain.UserID = "00000000-0000-4000-8000-000000000001"
	userB domain.UserID = "00000000-0000-4000-8000-000000000002"
	userC domain.UserID = "00000000-0000-4000-8000-000000000003"

	sendID1 = "11111111-1111-4111-8111-111111111111"
	sendID2 = "22222222-2222-4222-8222-222222222222"
)

func newTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.NewDatabase(t)
	return NewStore(pool), pool
}

// newTestStoreWithUsers は、会話とメッセージのテストが使う利用者(userA、userB、userC)の行を、
// 識別子を指定して、直接に置いた Store を返す。会話とメッセージは、利用者の表を参照する。
func newTestStoreWithUsers(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	store, pool := newTestStore(t)
	for i, id := range []domain.UserID{userA, userB, userC} {
		name := "user" + string(rune('a'+i))
		if _, err := pool.Exec(context.Background(),
			"INSERT INTO users (id, contact, handle, display_name, secret_hash) VALUES ($1, $2, $3, $4, 'unused')",
			string(id), name+"@example.com", name, name); err != nil {
			t.Fatal(err)
		}
	}
	return store, pool
}

// mustPair は、ポートに渡す、2人の組を作る。
// adapter から domain の規則を呼ばない規約の、例外である(ADR-005)。DirectPair は、中の値を
// パッケージの外から作れず、復元用の関数もないため、テストの入力の用意に限って、NewDirectPair を呼ぶ。
func mustPair(t *testing.T, a, b domain.UserID) domain.DirectPair {
	t.Helper()
	pair, err := domain.NewDirectPair(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

// appendOne は、会話の成立、追記の直列化、メッセージの追記を、1つのトランザクションで行う。
func appendOne(ctx context.Context, store *Store, pair domain.DirectPair, sender domain.UserID, sendID, body string) (domain.Message, error) {
	var msg domain.Message
	err := store.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
		conv, err := w.EstablishDirect(ctx, pair)
		if err != nil {
			return err
		}
		if err := w.SerializeAppends(ctx, conv.ID); err != nil {
			return err
		}
		msg, err = w.AppendMessage(ctx, conv.ID, sender, domain.RestoreSendID(sendID), domain.RestoreBody(body))
		return err
	})
	return msg, err
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAppendAndList(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx := context.Background()
	pair := mustPair(t, userA, userB)

	if _, found, err := store.FindDirect(ctx, pair); err != nil || found {
		t.Fatalf("成立していない会話が見つかった: found %v, err %v", found, err)
	}

	before := time.Now().Add(-time.Minute)
	first, err := appendOne(ctx, store, pair, userA, sendID1, "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := appendOne(ctx, store, mustPair(t, userB, userA), userB, sendID2, "two\n'; DROP TABLE messages; --")
	if err != nil {
		t.Fatal(err)
	}

	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("連番が違う: %d, %d", first.Seq, second.Seq)
	}
	if first.ConversationID != second.ConversationID {
		t.Fatal("同じ2人の会話が、2つできた")
	}
	if countRows(t, pool, "conversations") != 1 {
		t.Fatal("会話の行が、1つではない")
	}
	if first.SentAt.Location() != time.UTC {
		t.Fatalf("送信の日時が UTC ではない: %v", first.SentAt)
	}
	if first.SentAt.Before(before) || first.SentAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("送信の日時が、保管した時点ではない: %v", first.SentAt)
	}
	if second.SentAt.Before(first.SentAt) {
		t.Fatalf("送信の日時が、連番の順と前後した: %v, %v", first.SentAt, second.SentAt)
	}

	conv, found, err := store.FindDirect(ctx, pair)
	if err != nil || !found {
		t.Fatalf("会話が見つからない: found %v, err %v", found, err)
	}
	if conv.ID != first.ConversationID || conv.Pair != pair {
		t.Fatalf("会話が違う: %+v", conv)
	}

	messages, err := store.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 追記が返した値と、履歴として読んだ値が、一致する(送信の日時は、マイクロ秒まで)。
	if len(messages) != 2 || messages[0] != first || messages[1] != second {
		t.Fatalf("履歴が、追記した内容と違う:\n got %+v\nwant %+v", messages, []domain.Message{first, second})
	}
}

func TestListMessages_ReturnsOnlyThatConversation(t *testing.T) {
	store, _ := newTestStoreWithUsers(t)
	ctx := context.Background()

	ab, err := appendOne(ctx, store, mustPair(t, userA, userB), userA, sendID1, "to b")
	if err != nil {
		t.Fatal(err)
	}
	ac, err := appendOne(ctx, store, mustPair(t, userA, userC), userA, sendID2, "to c")
	if err != nil {
		t.Fatal(err)
	}

	if ac.Seq != 1 {
		t.Fatalf("連番が、会話ごとではない: %d", ac.Seq)
	}
	messages, err := store.ListMessages(ctx, ab.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body.String() != "to b" {
		t.Fatalf("別の会話のメッセージを返した: %+v", messages)
	}
}

func TestAppendMessage_DuplicateSend(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx := context.Background()

	first, err := appendOne(ctx, store, mustPair(t, userA, userB), userA, sendID1, "original")
	if err != nil {
		t.Fatal(err)
	}

	// 同じ送信者の、同じ送信の識別子。別の会話への追記でも、重複した送信である。
	_, err = appendOne(ctx, store, mustPair(t, userA, userC), userA, sendID1, "again")
	if !errors.Is(err, port.ErrDuplicateSend) {
		t.Fatalf("ErrDuplicateSend を返さなかった: %v", err)
	}
	// 併せて行った会話の成立も、取り消される。
	if countRows(t, pool, "conversations") != 1 || countRows(t, pool, "messages") != 1 {
		t.Fatal("重複した送信で、会話かメッセージが増えた")
	}

	found, ok, err := store.FindMessageBySend(ctx, userA, domain.RestoreSendID(sendID1))
	if err != nil || !ok {
		t.Fatalf("保管済みのメッセージが見つからない: ok %v, err %v", ok, err)
	}
	if found != first {
		t.Fatalf("保管済みのメッセージが違う:\n got %+v\nwant %+v", found, first)
	}

	// 別の送信者の、同じ送信の識別子は、別の送信である。
	if _, ok, err := store.FindMessageBySend(ctx, userB, domain.RestoreSendID(sendID1)); err != nil || ok {
		t.Fatalf("別の送信者のメッセージが見つかった: ok %v, err %v", ok, err)
	}
	other, err := appendOne(ctx, store, mustPair(t, userA, userB), userB, sendID1, "from b")
	if err != nil {
		t.Fatal(err)
	}
	if other.Seq != 2 {
		t.Fatalf("連番が違う(取り消された追記が、欠番を作った): %d", other.Seq)
	}
}

// 確定は、呼び出しの文脈の取り消しに従わない。fn が、エラーを返さずに戻った後は、
// 端末が切断しても(要求の文脈が取り消されても)、確定し、結果を返す。
// 取り消しに従うと、保管先では確定したのに、エラーを返すことがあり、そのメッセージは、配信されない。
func TestInTransaction_CommitsEvenIfContextIsCanceledAfterOperations(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := store.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
		conv, err := w.EstablishDirect(ctx, mustPair(t, userA, userB))
		if err != nil {
			return err
		}
		if _, err := w.AppendMessage(ctx, conv.ID, userA, domain.RestoreSendID(sendID1), domain.RestoreBody("x")); err != nil {
			return err
		}
		// 操作を終えた後、確定の前に、端末が切断した。
		cancel()
		return nil
	})

	if err != nil {
		t.Fatalf("確定せずに、エラーを返した: %v", err)
	}
	if countRows(t, pool, "conversations") != 1 || countRows(t, pool, "messages") != 1 {
		t.Fatal("確定していない")
	}
}

// 取り消された文脈の下での操作は、失敗し、トランザクションは、取り消される。
func TestInTransaction_RollsBackWhenOperationFailsByCanceledContext(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := store.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
		conv, err := w.EstablishDirect(ctx, mustPair(t, userA, userB))
		if err != nil {
			return err
		}
		cancel()
		_, err = w.AppendMessage(ctx, conv.ID, userA, domain.RestoreSendID(sendID1), domain.RestoreBody("x"))
		return err
	})

	if err == nil {
		t.Fatal("取り消された文脈の下での操作が、完了した")
	}
	if countRows(t, pool, "conversations") != 0 || countRows(t, pool, "messages") != 0 {
		t.Fatal("取り消されていない")
	}
}

func TestInTransaction_RollsBackOnError(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx := context.Background()
	wantErr := errors.New("stop")

	err := store.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
		conv, err := w.EstablishDirect(ctx, mustPair(t, userA, userB))
		if err != nil {
			return err
		}
		if _, err := w.AppendMessage(ctx, conv.ID, userA, domain.RestoreSendID(sendID1), domain.RestoreBody("x")); err != nil {
			return err
		}
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("fn のエラーを返さなかった: %v", err)
	}
	if countRows(t, pool, "conversations") != 0 || countRows(t, pool, "messages") != 0 {
		t.Fatal("取り消されていない")
	}
}

func TestInTransaction_RollsBackOnPanic(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx := context.Background()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("パニックが、呼び出し元に伝わらなかった")
			}
		}()
		_ = store.InTransaction(ctx, func(ctx context.Context, w port.ConversationWriter) error {
			if _, err := w.EstablishDirect(ctx, mustPair(t, userA, userB)); err != nil {
				return err
			}
			panic("boom")
		})
	}()

	if countRows(t, pool, "conversations") != 0 {
		t.Fatal("取り消されていない")
	}
}

// 2人が、ほぼ同時に、最初のメッセージを含む複数通を送る。
// 会話は1つで、連番に、欠番と重複がない(WS-05、WS-07)。
func TestAppend_ConcurrentSendersShareOneGaplessSequence(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx := context.Background()

	const perSender = 10
	sendIDFor := func(sender, i int) string {
		return "00000000-0000-4000-8000-0000000" + string(rune('0'+sender)) + "00" + string(rune('0'+i/10)) + string(rune('0'+i%10))
	}

	var wg sync.WaitGroup
	results := make(chan domain.Message, 2*perSender)
	errs := make(chan error, 2*perSender)
	for s, sender := range []domain.UserID{userA, userB} {
		for i := 0; i < perSender; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				msg, err := appendOne(ctx, store, mustPair(t, userA, userB), sender, sendIDFor(s, i), "m")
				if err != nil {
					errs <- err
					return
				}
				results <- msg
			}()
		}
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	seen := map[int64]bool{}
	for msg := range results {
		if seen[msg.Seq] {
			t.Fatalf("連番が重複した: %d", msg.Seq)
		}
		seen[msg.Seq] = true
	}
	for seq := int64(1); seq <= 2*perSender; seq++ {
		if !seen[seq] {
			t.Fatalf("連番に欠番がある: %d", seq)
		}
	}
	if countRows(t, pool, "conversations") != 1 {
		t.Fatal("会話の行が、1つではない")
	}
}

// 保管先が返したエラーを、そのまま返さない。文言と詳細に、行の内容(本文)が含まれることがある(WS-33)。
func TestErrors_DoNotCarryRowContents(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	ctx := context.Background()
	const marker = "SECRET-BODY-MARKER"

	// 違反したときの詳細(DETAIL)に、行の内容が現れる制約を、このテストのデータベースにだけ加える。
	if _, err := pool.Exec(ctx, "ALTER TABLE messages ADD CONSTRAINT test_reject_marker CHECK (body NOT LIKE '%"+marker+"%')"); err != nil {
		t.Fatal(err)
	}
	// 前提の確認: 保管先が返すエラーそのものには、本文が含まれる。
	_, rawErr := pool.Exec(ctx,
		"INSERT INTO conversations (id, user_low, user_high) VALUES ('c0000000-0000-4000-8000-000000000001', $1, $2)", string(userA), string(userB))
	if rawErr != nil {
		t.Fatal(rawErr)
	}
	_, rawErr = pool.Exec(ctx,
		"INSERT INTO messages (conversation_id, seq, sender_id, send_id, body, sent_at) VALUES ('c0000000-0000-4000-8000-000000000001', 1, $1, $2, $3, now())",
		string(userA), sendID2, "raw "+marker)
	var rawPgErr *pgconn.PgError
	if !errors.As(rawErr, &rawPgErr) || !strings.Contains(rawPgErr.Detail, marker) {
		t.Fatalf("前提が成り立たない(保管先のエラーの詳細に、本文が含まれるはず): %v", rawErr)
	}

	_, err := appendOne(ctx, store, mustPair(t, userA, userB), userA, sendID1, "hello "+marker)

	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("OpError ではない: %T %v", err, err)
	}
	if opErr.Op != "append message" || opErr.SQLState != "23514" {
		t.Fatalf("操作の名前か SQLSTATE が違う: %+v", opErr)
	}
	if strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), "Failing row") {
		t.Fatalf("エラーの文言に、行の内容が含まれる: %v", err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		t.Fatal("保管先が返したエラーを、取り出せる")
	}
}

func TestErrors_NonDatabaseFailure(t *testing.T) {
	store, pool := newTestStoreWithUsers(t)
	pool.Close()

	_, _, err := store.FindDirect(context.Background(), mustPair(t, userA, userB))

	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Op != "find conversation" || opErr.SQLState != "" {
		t.Fatalf("操作の名前だけを持つ OpError ではない: %T %v", err, err)
	}
}
