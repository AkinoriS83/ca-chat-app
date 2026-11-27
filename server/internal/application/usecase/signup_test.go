package usecase

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

const (
	newContact = "dave@example.com"
	newSecret  = "dave passphrase for tests"
)

// RL-01: 登録されていない連絡先での申し込みは、申し込みを置き、同じ資格の案内を送る。
func TestRequestSignup_PutsSignupAndSendsNotice(t *testing.T) {
	w := newWorld()

	err := w.signupRequester().Request(context.Background(), RequestSignupInput{Contact: "  Dave@Example.com ", Source: source})
	if err != nil {
		t.Fatal(err)
	}

	// 連絡先は、揃えた後の値で扱う。有効期限は、申し込みから24時間である。
	expiresAt := testNow.Add(domain.SignupLifetime)
	if got, want := w.signups.byContact[newContact], (fakeSignup{credential: "credential-1", expiresAt: expiresAt}); got != want {
		t.Fatalf("置いた申し込みが違う: %+v", got)
	}
	if want := []fakeNotice{{to: newContact, credential: "credential-1", expiresAt: expiresAt}}; !reflect.DeepEqual(w.notices.sent, want) {
		t.Fatalf("送った案内が違う: %+v", w.notices.sent)
	}
	// 申し込みを置いてから、案内を送る。
	if want := []string{"signups.Put"}; !reflect.DeepEqual(w.log.calls, want) {
		t.Fatalf("呼んだ順が違う: %v", w.log.calls)
	}

	// 送信元ごとと、宛先の連絡先ごとに、数える。
	wantLimits := []domain.Limit{w.rules.SignupRequestBySource(source), w.rules.SignupRequestByContact(domain.RestoreContact(newContact))}
	if !reflect.DeepEqual(w.limiter.admitted, wantLimits) {
		t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
	}
	requireEvents(t, w.events, domain.EventSignupRequested)
	if e := w.events.events[0]; e.Source != source || e.UserID != "" {
		t.Fatalf("記録の項目が違う: %+v", e)
	}
}

// RL-03: 登録済みの連絡先での申し込みは、申し込みを置かず、資格を含まない案内を送る。完了は、同じである。
func TestRequestSignup_RegisteredContact_SendsAlreadyRegisteredNotice(t *testing.T) {
	w := newWorld()

	err := w.signupRequester().Request(context.Background(), RequestSignupInput{Contact: aliceContact, Source: source})
	if err != nil {
		t.Fatal(err)
	}

	if len(w.signups.byContact) != 0 || len(w.credentials.issued) != 0 {
		t.Fatal("登録済みの連絡先に、申し込みを置いた")
	}
	if want := []fakeNotice{{registered: true, to: aliceContact}}; !reflect.DeepEqual(w.notices.sent, want) {
		t.Fatalf("送った案内が違う: %+v", w.notices.sent)
	}
	// 登録済みの連絡先も、同じく数え、同じく記録する(RL-30)。
	if len(w.limiter.admitted) != 2 {
		t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
	}
	requireEvents(t, w.events, domain.EventSignupRequested)
}

// UC-01 E1: 連絡先が、入力の規則を満たさない申し込みは、上限より先に拒否し、数えない。
func TestRequestSignup_InvalidContact(t *testing.T) {
	w := newWorld()
	w.limiter.exceeded[domain.LimitSignupRequestPerSource] = true

	err := w.signupRequester().Request(context.Background(), RequestSignupInput{Contact: "not-a-contact", Source: source})

	requireInvalidFields(t, err, FieldContact)
	if len(w.limiter.admitted) != 0 || len(w.notices.sent) != 0 {
		t.Fatal("規則を満たさない申し込みを、数えたか、案内を送った")
	}
	requireEvents(t, w.events)
}

// UC-01 E4、RL-30: 上限を超えた申し込みは、登録済みかどうかに依らず、同じく拒否し、案内を送らない。
func TestRequestSignup_RateLimited(t *testing.T) {
	for _, kind := range []domain.LimitKind{domain.LimitSignupRequestPerSource, domain.LimitSignupRequestPerContact} {
		for _, contact := range []string{newContact, aliceContact} {
			t.Run(kind.String()+"/"+contact, func(t *testing.T) {
				w := newWorld()
				w.limiter.exceeded[kind] = true

				err := w.signupRequester().Request(context.Background(), RequestSignupInput{Contact: contact, Source: source})

				requireRejection(t, err, RateLimited)
				if len(w.notices.sent) != 0 || len(w.signups.byContact) != 0 {
					t.Fatal("上限を超えた申し込みで、案内を送ったか、申し込みを置いた")
				}
				requireEvents(t, w.events, domain.EventLimitExceeded)
				if e := w.events.events[0]; e.Limit != kind || e.Source != source {
					t.Fatalf("記録の項目が違う: %+v", e)
				}
			})
		}
	}
}

// RL-10: 案内を送れなかった場合は、完了できなかったことを返し、その旨を記録する。
// 連絡先が登録済みかどうかに、依らない。この申し込みも、上限に数えたままとする。
func TestRequestSignup_NoticeFailure(t *testing.T) {
	for _, contact := range []string{newContact, aliceContact} {
		t.Run(contact, func(t *testing.T) {
			w := newWorld()
			w.notices.err = errBoom

			err := w.signupRequester().Request(context.Background(), RequestSignupInput{Contact: contact, Source: source})

			requireFailure(t, err)
			requireEvents(t, w.events, domain.EventSignupRequested, domain.EventSignupNoticeFailed, domain.EventOperationFailed)
			if len(w.limiter.admitted) != 2 || len(w.limiter.forgiven) != 0 {
				t.Fatal("案内を送れなかった申し込みの計数を、戻した")
			}
		})
	}
}

// requested は、テストの準備として、申し込みを置き、確認のための資格を返す。
func (w *world) requested(t *testing.T, contact string) string {
	t.Helper()
	if err := w.signupRequester().Request(context.Background(), RequestSignupInput{Contact: contact, Source: source}); err != nil {
		t.Fatal(err)
	}
	w.events.events = nil
	w.limiter.admitted = nil
	w.log.calls = nil
	return w.signups.byContact[contact].credential
}

// RL-01: 正しい入力での完了は、申し込みを使用済みにし、利用者を作る。ログイン状態は、与えない。
func TestCompleteSignup_CreatesUser(t *testing.T) {
	w := newWorld()
	credential := w.requested(t, newContact)

	err := w.signupCompleter().Complete(context.Background(), CompleteSignupInput{
		Credential: credential, Handle: "Dave_01", DisplayName: "  Dave  ", Secret: newSecret, Source: source,
	})
	if err != nil {
		t.Fatal(err)
	}

	account, found := w.users.accounts[newContact]
	if !found {
		t.Fatal("利用者が、作られていない")
	}
	// 識別名は、入力された通りに、表示名は、前後の空白文字を除いて、秘密は、保管する形で渡す。
	if account.user.Handle.String() != "Dave_01" || account.user.DisplayName.String() != "Dave" || account.secret != fakeHashOf(newSecret) {
		t.Fatalf("作った利用者が違う: %+v", account)
	}
	if len(w.signups.byContact) != 0 {
		t.Fatal("申し込みが、使用済みになっていない")
	}
	if len(w.sessions.byCredential) != 0 {
		t.Fatal("登録の完了で、ログイン状態を与えた")
	}
	// 秘密の計算は、トランザクションの前に、1回だけ行う。
	if w.hasher.hashes != 1 || w.hasher.hashedDuring {
		t.Fatalf("秘密の計算が違う: %d 回, トランザクションの中 = %v", w.hasher.hashes, w.hasher.hashedDuring)
	}
	if want := []string{"ConsumeSignup", "CreateUser"}; !reflect.DeepEqual(w.store.calls, want) {
		t.Fatalf("トランザクションの中の操作が違う: %v", w.store.calls)
	}
	if want := []domain.Limit{w.rules.SignupCompletionBySource(source)}; !reflect.DeepEqual(w.limiter.admitted, want) {
		t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
	}
	requireEvents(t, w.events, domain.EventSignupCompleted)
	if e := w.events.events[0]; e.UserID != account.user.ID || e.Source != source {
		t.Fatalf("記録の項目が違う: %+v", e)
	}
}

// 仕様「登録する」の拒否の順: 上限 → 資格が無効 → 入力の規則 → 識別名が使われている。
func TestCompleteSignup_RejectionOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("上限は、資格より先", func(t *testing.T) {
		w := newWorld()
		w.limiter.exceeded[domain.LimitSignupCompletionPerSource] = true

		err := w.signupCompleter().Complete(ctx, CompleteSignupInput{Credential: "wrong", Handle: "!", Source: source})

		requireRejection(t, err, RateLimited)
		requireEvents(t, w.events, domain.EventLimitExceeded)
	})

	// RL-08: 資格が無効な完了の拒否は、識別名が使われているかどうかと、入力が正しいかどうかに、依らない。
	t.Run("資格は、入力の規則と識別名より先", func(t *testing.T) {
		for _, handle := range []string{"alice", "unused_handle", "!"} {
			w := newWorld()

			err := w.signupCompleter().Complete(ctx, CompleteSignupInput{Credential: "wrong", Handle: handle, DisplayName: "", Secret: "", Source: source})

			requireRejection(t, err, InvalidSignupCredential)
			if w.hasher.calculations() != 0 || len(w.store.calls) != 0 {
				t.Fatal("資格が無効な完了で、秘密の計算か、トランザクションを行った")
			}
			// 拒否された完了も、上限に数える(仕様「上限」)。
			if len(w.limiter.admitted) != 1 {
				t.Fatalf("数えたものが違う: %+v", w.limiter.admitted)
			}
		}
	})

	t.Run("入力の規則は、識別名が使われていることより先", func(t *testing.T) {
		w := newWorld()
		credential := w.requested(t, newContact)

		err := w.signupCompleter().Complete(ctx, CompleteSignupInput{Credential: credential, Handle: "alice", DisplayName: "", Secret: newSecret, Source: source})

		requireInvalidFields(t, err, FieldDisplayName)
	})
}

// RL-05: 期限の直前の資格は使え、期限の資格は使えない。
func TestCompleteSignup_Expiry(t *testing.T) {
	in := CompleteSignupInput{Handle: "dave", DisplayName: "Dave", Secret: newSecret, Source: source}

	t.Run("期限", func(t *testing.T) {
		w := newWorld()
		in.Credential = w.requested(t, newContact)
		w.clock.now = testNow.Add(domain.SignupLifetime)

		requireRejection(t, w.signupCompleter().Complete(context.Background(), in), InvalidSignupCredential)
	})
	t.Run("期限の直前", func(t *testing.T) {
		w := newWorld()
		in.Credential = w.requested(t, newContact)
		w.clock.now = testNow.Add(domain.SignupLifetime - 1)

		if err := w.signupCompleter().Complete(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	})
}

// UC-01 E1、RL-06: 正しくない項目を、すべて挙げる。申し込みは、使用済みにならない。
func TestCompleteSignup_InvalidFields(t *testing.T) {
	const commonSecret = "common passphrase for tests"

	tests := []struct {
		name                        string
		handle, displayName, secret string
		want                        []InputField
	}{
		{"識別名", "a!", "Dave", newSecret, []InputField{FieldHandle}},
		{"表示名", "dave", "  ", newSecret, []InputField{FieldDisplayName}},
		{"秘密が短い", "dave", "Dave", "short", []InputField{FieldSecret}},
		{"秘密が、よく使われる秘密の一覧にある", "dave", "Dave", commonSecret, []InputField{FieldSecret}},
		{"秘密が、連絡先と同じ", "dave", "Dave", "DAVE@example.com", []InputField{FieldSecret}},
		{"秘密が、識別名と同じ", "Dave_0123456789ab", "Dave", "dave_0123456789AB", []InputField{FieldSecret}},
		{"すべて", "", "", "", []InputField{FieldHandle, FieldDisplayName, FieldSecret}},
		{"識別名と秘密", "da ve", "Dave", strings.Repeat("a", 129), []InputField{FieldHandle, FieldSecret}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld()
			w.common[commonSecret] = true
			credential := w.requested(t, newContact)

			err := w.signupCompleter().Complete(context.Background(), CompleteSignupInput{
				Credential: credential, Handle: tt.handle, DisplayName: tt.displayName, Secret: tt.secret, Source: source,
			})

			requireInvalidFields(t, err, tt.want...)
			if len(w.signups.byContact) != 1 || w.hasher.calculations() != 0 {
				t.Fatal("拒否した完了で、申し込みを使用済みにしたか、秘密の計算を行った")
			}
			requireEvents(t, w.events)
		})
	}
}

// UC-01 E2、RL-07: 使われている識別名(大文字と小文字だけが異なるものを含む)での完了は、拒否する。
// 申し込みは、使用済みにならず、同じ資格で、別の識別名で完了できる。
func TestCompleteSignup_HandleTaken(t *testing.T) {
	for _, handle := range []string{"alice", "ALICE", "Alice"} {
		t.Run(handle, func(t *testing.T) {
			w := newWorld()
			credential := w.requested(t, newContact)
			completer := w.signupCompleter()
			in := CompleteSignupInput{Credential: credential, Handle: handle, DisplayName: "Dave", Secret: newSecret, Source: source}

			requireRejection(t, completer.Complete(context.Background(), in), HandleTaken)
			if len(w.signups.byContact) != 1 {
				t.Fatal("拒否した完了で、申し込みを使用済みにした")
			}

			in.Handle = "dave"
			if err := completer.Complete(context.Background(), in); err != nil {
				t.Fatalf("同じ資格で、別の識別名で完了できなかった: %v", err)
			}
		})
	}
}

// RL-05: トランザクションの中で、申し込みが使えなくなっていた完了(同時の完了)と、
// 連絡先が登録済みであった完了は、資格が無効な場合として拒否する。
func TestCompleteSignup_RaceIsInvalidCredential(t *testing.T) {
	in := CompleteSignupInput{Handle: "dave", DisplayName: "Dave", Secret: newSecret, Source: source}

	t.Run("同時の完了", func(t *testing.T) {
		w := newWorld()
		in.Credential = w.requested(t, newContact)
		// 資格を確かめた後、トランザクションの前に(秘密の計算の間に)、申し込みがなくなる。
		completer := w.signupCompleter()
		completer.hasher = &consumingHasher{fakeHasher: w.hasher, consume: func() { w.signups.byContact = map[string]fakeSignup{} }}

		requireRejection(t, completer.Complete(context.Background(), in), InvalidSignupCredential)
		if len(w.users.accounts) != 2 {
			t.Fatal("利用者を作った")
		}
		requireEvents(t, w.events)
	})

	t.Run("連絡先が登録済み", func(t *testing.T) {
		w := newWorld()
		in.Credential = w.requested(t, newContact)
		// 申し込みを置いた後に、同じ連絡先の登録が、確定した。
		w.users.accounts[newContact] = fakeAccount{user: carol, secret: "x"}

		requireRejection(t, w.signupCompleter().Complete(context.Background(), in), InvalidSignupCredential)
		// 取り消されるため、申し込みは、期限まで残る。片付けが消す。
		if len(w.signups.byContact) != 1 {
			t.Fatal("申し込みが、取り消されていない")
		}
		requireEvents(t, w.events)
	})
}

// consumingHasher は、秘密の計算の間に、状態を変える。
type consumingHasher struct {
	*fakeHasher
	consume func()
}

func (h *consumingHasher) Hash(ctx context.Context, secret domain.Secret) (domain.SecretHash, error) {
	h.consume()
	return h.fakeHasher.Hash(ctx, secret)
}

var _ port.SecretHasher = (*consumingHasher)(nil)

// トランザクションの失敗は、完了できなかったこととして返す。利用者は、作られない。
func TestCompleteSignup_StoreFailure(t *testing.T) {
	w := newWorld()
	credential := w.requested(t, newContact)
	w.store.commitErr = errBoom

	err := w.signupCompleter().Complete(context.Background(), CompleteSignupInput{Credential: credential, Handle: "dave", DisplayName: "Dave", Secret: newSecret, Source: source})

	requireFailure(t, err)
	if _, found := w.users.accounts[newContact]; found || len(w.signups.byContact) != 1 {
		t.Fatal("取り消されていない")
	}
	requireEvents(t, w.events, domain.EventOperationFailed)
}
