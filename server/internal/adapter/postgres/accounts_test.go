package postgres

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// 時刻は、保管先の精度(マイクロ秒)で扱う。
var testNow = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

// putSignup は、申し込みを置く。
func putSignup(t *testing.T, store *Store, contact, credential string, expiresAt time.Time) {
	t.Helper()
	if err := store.Signups().Put(context.Background(), domain.RestoreContact(contact), credential, expiresAt); err != nil {
		t.Fatal(err)
	}
}

// completeSignup は、申し込みを使用済みにすることと、利用者を作ることを、1つのトランザクションで行う。
// 申し込みが使えなかった場合は、consumed に false を返す。
func completeSignup(ctx context.Context, store *Store, credential, handle, displayName string, now time.Time) (user domain.User, consumed bool, err error) {
	err = store.InSignupTransaction(ctx, func(ctx context.Context, w port.SignupWriter) error {
		signup, found, err := w.ConsumeSignup(ctx, credential, now)
		if err != nil || !found {
			return err
		}
		consumed = true
		user, err = w.CreateUser(ctx, signup.Contact, domain.RestoreHandle(handle), domain.RestoreDisplayName(displayName), "stored-hash-of-"+domain.SecretHash(handle))
		return err
	})
	return user, consumed, err
}

// mustUser は、テストの準備として、利用者を作る。
func mustUser(t *testing.T, store *Store, contact, handle, displayName string) domain.User {
	t.Helper()
	credential := "signup-credential-for-" + contact
	putSignup(t, store, contact, credential, testNow.Add(time.Hour))
	user, consumed, err := completeSignup(context.Background(), store, credential, handle, displayName, testNow)
	if err != nil || !consumed {
		t.Fatalf("利用者を作れない: consumed = %v, %v", consumed, err)
	}
	return user
}

// RL-01、RL-24: 作った利用者を、連絡先と、識別名で探せる。識別名の照合は、大文字と小文字を区別しない。
// 識別名は、登録した通りに返る。
func TestUsers_CreateAndFind(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	created := mustUser(t, store, "alice@example.com", "Alice_01", "Alice A")

	if created.ID == "" || created.Handle.String() != "Alice_01" || created.DisplayName.String() != "Alice A" {
		t.Fatalf("作った利用者が違う: %+v", created)
	}

	byContact, secret, found, err := store.FindByContact(ctx, domain.RestoreContact("alice@example.com"))
	if err != nil || !found {
		t.Fatalf("連絡先で、見つからない: %v, %v", found, err)
	}
	if byContact != created || secret != "stored-hash-of-Alice_01" {
		t.Fatalf("利用者か、保管した秘密が違う: %+v, %q", byContact, secret)
	}

	for _, handle := range []string{"Alice_01", "alice_01", "ALICE_01"} {
		byHandle, found, err := store.FindByHandle(ctx, domain.RestoreHandle(handle))
		if err != nil || !found {
			t.Fatalf("識別名 %q で、見つからない: %v, %v", handle, found, err)
		}
		if byHandle != created {
			t.Fatalf("利用者が違う: %+v", byHandle)
		}
	}
}

// RL-25: 一致する利用者が居ない場合。識別名の一部と、LIKE の特別な文字は、一致しない。
func TestUsers_NotFound(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	mustUser(t, store, "alice@example.com", "alice", "Alice")

	for _, handle := range []string{"alic", "alicee", "a%", "_lice", "", "bob"} {
		if _, found, err := store.FindByHandle(ctx, domain.RestoreHandle(handle)); err != nil || found {
			t.Fatalf("識別名 %q: found = %v, err = %v", handle, found, err)
		}
	}
	if _, _, found, err := store.FindByContact(ctx, domain.RestoreContact("nobody@example.com")); err != nil || found {
		t.Fatalf("連絡先: found = %v, err = %v", found, err)
	}
}

// RL-07: 使われている識別名(大文字と小文字だけが異なるものを含む)での作成は、port.ErrHandleTaken になる。
// トランザクションは、取り消され、申し込みは、使用済みにならない。同じ資格で、別の識別名で完了できる。
func TestCreateUser_HandleTaken(t *testing.T) {
	for _, handle := range []string{"alice", "ALICE", "Alice"} {
		t.Run(handle, func(t *testing.T) {
			store, pool := newTestStore(t)
			ctx := context.Background()
			mustUser(t, store, "alice@example.com", "alice", "Alice")
			putSignup(t, store, "dave@example.com", "dave-credential", testNow.Add(time.Hour))

			_, _, err := completeSignup(ctx, store, "dave-credential", handle, "Dave", testNow)

			if !errors.Is(err, port.ErrHandleTaken) {
				t.Fatalf("port.ErrHandleTaken を返さなかった: %v", err)
			}
			if countRows(t, pool, "users") != 1 || countRows(t, pool, "signups") != 1 {
				t.Fatal("取り消されていない")
			}

			if _, consumed, err := completeSignup(ctx, store, "dave-credential", "dave", "Dave", testNow); err != nil || !consumed {
				t.Fatalf("同じ資格で、別の識別名で完了できなかった: %v, %v", consumed, err)
			}
		})
	}
}

// 登録済みの連絡先での作成は、port.ErrContactTaken になる。
func TestCreateUser_ContactTaken(t *testing.T) {
	store, pool := newTestStore(t)
	mustUser(t, store, "alice@example.com", "alice", "Alice")
	// 登録済みの連絡先に、申し込みが残った状態を作る。
	putSignup(t, store, "alice@example.com", "late-credential", testNow.Add(time.Hour))

	_, _, err := completeSignup(context.Background(), store, "late-credential", "alice2", "Alice", testNow)

	if !errors.Is(err, port.ErrContactTaken) {
		t.Fatalf("port.ErrContactTaken を返さなかった: %v", err)
	}
	if countRows(t, pool, "users") != 1 || countRows(t, pool, "signups") != 1 {
		t.Fatal("取り消されていない")
	}
}

// RL-07: 同じ識別名での完了が、同時に行われた場合、完了するのは、1つである。
func TestCreateUser_ConcurrentSameHandle(t *testing.T) {
	store, pool := newTestStore(t)
	const n = 8
	contacts := make([]string, n)
	for i := range contacts {
		contacts[i] = string(rune('a'+i)) + "@example.com"
		putSignup(t, store, contacts[i], "credential-"+contacts[i], testNow.Add(time.Hour))
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range contacts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle := "samename"
			if i%2 == 1 {
				handle = "SameName"
			}
			_, _, errs[i] = completeSignup(context.Background(), store, "credential-"+contacts[i], handle, "Same", testNow)
		}()
	}
	wg.Wait()

	completed := 0
	for _, err := range errs {
		switch {
		case err == nil:
			completed++
		case !errors.Is(err, port.ErrHandleTaken):
			t.Fatalf("port.ErrHandleTaken でないエラー: %v", err)
		}
	}
	if completed != 1 || countRows(t, pool, "users") != 1 {
		t.Fatalf("完了した数が違う: %d", completed)
	}
	// 完了できなかった申し込みは、使用済みにならない。
	if got := countRows(t, pool, "signups"); got != n-1 {
		t.Fatalf("申し込みの数が違う: %d", got)
	}
}

// RL-05: 同じ資格での完了が、同時に行われた場合、申し込みを使えるのは、1つである。
func TestConsumeSignup_ConcurrentSameCredential(t *testing.T) {
	store, pool := newTestStore(t)
	putSignup(t, store, "dave@example.com", "dave-credential", testNow.Add(time.Hour))

	const n = 8
	var wg sync.WaitGroup
	consumed := make([]bool, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 識別名は、それぞれ変える。識別名の一意性ではなく、申し込みの行で、1つに保つことを確かめる。
			_, consumed[i], errs[i] = completeSignup(context.Background(), store, "dave-credential", "dave"+string(rune('0'+i)), "Dave", testNow)
		}()
	}
	wg.Wait()

	count := 0
	for i := range consumed {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if consumed[i] {
			count++
		}
	}
	if count != 1 || countRows(t, pool, "users") != 1 || countRows(t, pool, "signups") != 0 {
		t.Fatalf("申し込みを使えた数が違う: %d", count)
	}
}

// RL-04: 同じ連絡先での、再度の申し込みは、以前の申し込みを置き換える。以前の資格は、使えない。
func TestSignups_PutReplacesPrevious(t *testing.T) {
	store, pool := newTestStore(t)
	ctx := context.Background()
	signups := store.Signups()
	putSignup(t, store, "dave@example.com", "first", testNow.Add(time.Hour))
	putSignup(t, store, "dave@example.com", "second", testNow.Add(2*time.Hour))

	if countRows(t, pool, "signups") != 1 {
		t.Fatal("申し込みが、置き換わっていない")
	}
	if _, found, err := signups.Find(ctx, "first", testNow); err != nil || found {
		t.Fatalf("以前の資格が、使える: %v, %v", found, err)
	}
	signup, found, err := signups.Find(ctx, "second", testNow)
	if err != nil || !found {
		t.Fatalf("新しい資格が、使えない: %v, %v", found, err)
	}
	// 有効期限は、新しい申し込みから数える。
	if signup.Contact.String() != "dave@example.com" || !signup.ExpiresAt.Equal(testNow.Add(2*time.Hour)) {
		t.Fatalf("申し込みが違う: %+v", signup)
	}
}

// RL-05、RL-09: 申し込みは、期限の直前には使え、期限には使えない。片付けは、期限を過ぎたものだけを消す。
func TestSignups_Expiry(t *testing.T) {
	store, pool := newTestStore(t)
	ctx := context.Background()
	signups := store.Signups()
	expiresAt := testNow.Add(24 * time.Hour)
	putSignup(t, store, "dave@example.com", "dave-credential", expiresAt)
	putSignup(t, store, "erin@example.com", "erin-credential", expiresAt.Add(time.Microsecond))
	justBefore := expiresAt.Add(-time.Microsecond)

	if _, found, err := signups.Find(ctx, "dave-credential", justBefore); err != nil || !found {
		t.Fatalf("期限の直前に、使えない: %v, %v", found, err)
	}
	if _, found, err := signups.Find(ctx, "dave-credential", expiresAt); err != nil || found {
		t.Fatalf("期限に、使える: %v, %v", found, err)
	}
	if _, consumed, err := completeSignup(ctx, store, "dave-credential", "dave", "Dave", expiresAt); err != nil || consumed {
		t.Fatalf("期限に、使用済みにできる: %v, %v", consumed, err)
	}

	if err := signups.DeleteExpired(ctx, justBefore); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, pool, "signups"); got != 2 {
		t.Fatalf("期限の前の申し込みを、消した: %d", got)
	}
	if err := signups.DeleteExpired(ctx, expiresAt); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, pool, "signups"); got != 1 {
		t.Fatalf("期限を過ぎた申し込みだけを、消していない: %d", got)
	}
	if _, found, _ := signups.Find(ctx, "erin-credential", expiresAt); !found {
		t.Fatal("期限の前の申し込みを、消した")
	}
}

// RL-14、RL-16: ログイン状態は、資格で引ける。期限の直前には使え、期限には使えない。
func TestSessions_CreateFindExpiry(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	sessions := store.Sessions()
	alice := mustUser(t, store, "alice@example.com", "Alice", "Alice A")
	expiresAt := testNow.Add(30 * 24 * time.Hour)

	created, err := sessions.Create(ctx, "alice-credential", alice.ID, expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.UserID != alice.ID || !created.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("作ったログイン状態が違う: %+v", created)
	}

	found, user, ok, err := sessions.Find(ctx, "alice-credential", expiresAt.Add(-time.Microsecond))
	if err != nil || !ok {
		t.Fatalf("期限の直前に、使えない: %v, %v", ok, err)
	}
	if found != created || user != alice {
		t.Fatalf("ログイン状態か、利用者が違う: %+v, %+v", found, user)
	}

	if _, _, ok, err := sessions.Find(ctx, "alice-credential", expiresAt); err != nil || ok {
		t.Fatalf("期限に、使える: %v, %v", ok, err)
	}
	if _, _, ok, err := sessions.Find(ctx, "other-credential", testNow); err != nil || ok {
		t.Fatalf("正しくない資格で、使える: %v, %v", ok, err)
	}
}

// RL-17: 無効にしたログイン状態は、使えない。別のログイン状態は、残る。
func TestSessions_Revoke(t *testing.T) {
	store, pool := newTestStore(t)
	ctx := context.Background()
	sessions := store.Sessions()
	alice := mustUser(t, store, "alice@example.com", "alice", "Alice")
	first, err := sessions.Create(ctx, "first", alice.ID, testNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Create(ctx, "second", alice.ID, testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	id, found, err := sessions.Revoke(ctx, "first")
	if err != nil || !found || id != first.ID {
		t.Fatalf("無効にできない: %q, %v, %v", id, found, err)
	}

	if _, _, ok, _ := sessions.Find(ctx, "first", testNow); ok {
		t.Fatal("無効にしたログイン状態が、使える")
	}
	if _, _, ok, _ := sessions.Find(ctx, "second", testNow); !ok {
		t.Fatal("別のログイン状態を、無効にした")
	}
	// 無効にした行を、残さない。
	if got := countRows(t, pool, "sessions"); got != 1 {
		t.Fatalf("ログイン状態の数が違う: %d", got)
	}
	// 当たるものがない資格では、何もしない。
	if _, found, err := sessions.Revoke(ctx, "first"); err != nil || found {
		t.Fatalf("無効にしたログイン状態を、もう一度無効にできた: %v, %v", found, err)
	}
}

func TestSessions_DeleteExpired(t *testing.T) {
	store, pool := newTestStore(t)
	ctx := context.Background()
	sessions := store.Sessions()
	alice := mustUser(t, store, "alice@example.com", "alice", "Alice")
	expiresAt := testNow.Add(time.Hour)
	for credential, at := range map[string]time.Time{"expired": expiresAt, "valid": expiresAt.Add(time.Microsecond)} {
		if _, err := sessions.Create(ctx, credential, alice.ID, at); err != nil {
			t.Fatal(err)
		}
	}

	if err := sessions.DeleteExpired(ctx, expiresAt.Add(-time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, pool, "sessions"); got != 2 {
		t.Fatalf("期限の前のログイン状態を、消した: %d", got)
	}
	if err := sessions.DeleteExpired(ctx, expiresAt); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, pool, "sessions"); got != 1 {
		t.Fatalf("期限を過ぎたログイン状態だけを、消していない: %d", got)
	}
}

// ADR-014: 資格そのものは、保管しない。保管するのは、要約(SHA-256。32バイト)である。
func TestCredentials_AreStoredAsDigests(t *testing.T) {
	store, pool := newTestStore(t)
	ctx := context.Background()
	alice := mustUser(t, store, "alice@example.com", "alice", "Alice")
	const sessionCredential = "MARKER-session-credential"
	const signupCredential = "MARKER-signup-credential"
	if _, err := store.Sessions().Create(ctx, sessionCredential, alice.ID, testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	putSignup(t, store, "dave@example.com", signupCredential, testNow.Add(time.Hour))

	for table, credential := range map[string]string{"sessions": sessionCredential, "signups": signupCredential} {
		var digest []byte
		if err := pool.QueryRow(ctx, "SELECT credential_digest FROM "+table).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		if len(digest) != 32 || bytes.Contains(digest, []byte(credential)) {
			t.Fatalf("%s: 保管した値が、要約でない(%d バイト)", table, len(digest))
		}
		requireNoRowContains(t, pool, table, "MARKER")
	}
}

// requireNoRowContains は、表のどの行も、文字列として、marker を含まないことを確かめる。
func requireNoRowContains(t *testing.T, pool *pgxpool.Pool, table, marker string) {
	t.Helper()
	var n int
	query := "SELECT count(*) FROM " + table + " t WHERE position($1 in convert_from(convert_to(t::text, 'UTF8'), 'UTF8')) > 0"
	if err := pool.QueryRow(context.Background(), query, marker).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%s の行が、%q を含む", table, marker)
	}
}

// 保管先が返したエラーは、操作の名前と SQLSTATE だけを持つエラーに、作り直して返す(RL-33)。
// 行の内容(連絡先、識別名など)を、含めない。
func TestAccounts_ErrorsAreRebuilt(t *testing.T) {
	store, pool := newTestStore(t)
	ctx := context.Background()
	const marker = "marker-contact@example.com"
	putSignup(t, store, marker, "credential", testNow.Add(time.Hour))
	// 利用者の表を除いて、利用者を作る操作を、失敗させる。
	if _, err := pool.Exec(ctx, "DROP TABLE users CASCADE"); err != nil {
		t.Fatal(err)
	}

	_, _, err := completeSignup(ctx, store, "credential", "dave", "Dave", testNow)

	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Op != "create user" || opErr.SQLState == "" {
		t.Fatalf("OpError でない: %#v", err)
	}
	if bytes.Contains([]byte(err.Error()), []byte("marker")) || bytes.Contains([]byte(err.Error()), []byte("dave")) {
		t.Fatalf("エラーが、行の内容を含む: %v", err)
	}
	// ログイン状態を作る操作も、同じである(利用者の識別子が、UUID でない)。
	if _, err := store.Sessions().Create(ctx, "c", "not-a-uuid", testNow); !errors.As(err, &opErr) {
		t.Fatalf("OpError でない: %#v", err)
	}
}
