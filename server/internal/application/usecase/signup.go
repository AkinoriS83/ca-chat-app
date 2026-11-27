package usecase

import (
	"context"
	"errors"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// SignupRequester は、「登録を申し込む」ユースケースである(UC-01 基本フロー 1〜3、A1、A2)。
type SignupRequester struct {
	users       port.UserDirectory
	signups     port.SignupStore
	credentials port.CredentialSource
	notices     port.NoticeSender
	limiter     port.AttemptLimiter
	rules       domain.LimitRules
	clock       port.Clock
	events      port.EventRecorder
}

// NewSignupRequester は、SignupRequester を作る。
func NewSignupRequester(users port.UserDirectory, signups port.SignupStore, credentials port.CredentialSource, notices port.NoticeSender, limiter port.AttemptLimiter, rules domain.LimitRules, clock port.Clock, events port.EventRecorder) *SignupRequester {
	return &SignupRequester{users: users, signups: signups, credentials: credentials, notices: notices, limiter: limiter, rules: rules, clock: clock, events: events}
}

// Request は、登録の申し込みを受け付け、連絡先へ、案内を送る。
// 完了は、連絡先が登録済みかどうかに依らず、同じである(RL-03)。
// 拒否は、仕様「登録する」の表の順に判定する。
func (r *SignupRequester) Request(ctx context.Context, in RequestSignupInput) (err error) {
	a := begin(r.events, "request signup", in.Source)
	defer a.finish(ctx, &err)

	contact, err := domain.NewContact(in.Contact)
	if err != nil {
		return rejectedFields(FieldContact)
	}

	now := r.clock.Now()
	exceeded, admitted, err := r.limiter.Admit(ctx, now, r.rules.SignupRequestBySource(in.Source), r.rules.SignupRequestByContact(contact))
	if err != nil {
		return err
	}
	if !admitted {
		a.limitExceeded(ctx, exceeded)
		return rejected(RateLimited)
	}
	a.record(ctx, domain.EventSignupRequested)

	_, _, registered, err := r.users.FindByContact(ctx, contact)
	if err != nil {
		return err
	}
	if registered {
		// 申し込みは、置かない。アカウントとログイン状態は、変えない(RL-03)。
		return r.sent(ctx, a, r.notices.SendAlreadyRegisteredNotice(ctx, contact))
	}

	credential, err := r.credentials.NewCredential()
	if err != nil {
		return err
	}
	// 申し込みを置いてから、案内を送る。逆の順では、置くことに失敗したとき、使えない資格の案内が届く。
	expiresAt := now.Add(domain.SignupLifetime)
	if err := r.signups.Put(ctx, contact, credential, expiresAt); err != nil {
		return err
	}
	return r.sent(ctx, a, r.notices.SendSignupNotice(ctx, contact, credential, expiresAt))
}

// sent は、案内を送った結果を受ける。送れなかった場合は、その旨を記録して、エラーを返す(RL-10)。
// この申し込みも、上限に数えたままとする(仕様「登録する」)。
func (r *SignupRequester) sent(ctx context.Context, a *attempt, err error) error {
	if err != nil {
		a.record(ctx, domain.EventSignupNoticeFailed)
	}
	return err
}

// SignupCompleter は、「登録を完了する」ユースケースである(UC-01 基本フロー 4〜6)。
type SignupCompleter struct {
	signups       port.SignupStore
	commonSecrets port.CommonSecretList
	hasher        port.SecretHasher
	tx            port.Transactor
	limiter       port.AttemptLimiter
	rules         domain.LimitRules
	clock         port.Clock
	events        port.EventRecorder
}

// NewSignupCompleter は、SignupCompleter を作る。
func NewSignupCompleter(signups port.SignupStore, commonSecrets port.CommonSecretList, hasher port.SecretHasher, tx port.Transactor, limiter port.AttemptLimiter, rules domain.LimitRules, clock port.Clock, events port.EventRecorder) *SignupCompleter {
	return &SignupCompleter{signups: signups, commonSecrets: commonSecrets, hasher: hasher, tx: tx, limiter: limiter, rules: rules, clock: clock, events: events}
}

// errSignupConsumed は、トランザクションの中で、申し込みが、既に使えなくなっていたことを示す。
var errSignupConsumed = errors.New("usecase: signup already consumed")

// Complete は、確認のための資格を確かめて、利用者を作る。ログイン状態は、与えない。
// 拒否は、仕様「登録する」の表の順に判定する。
func (c *SignupCompleter) Complete(ctx context.Context, in CompleteSignupInput) (err error) {
	a := begin(c.events, "complete signup", in.Source)
	defer a.finish(ctx, &err)

	now := c.clock.Now()
	exceeded, admitted, err := c.limiter.Admit(ctx, now, c.rules.SignupCompletionBySource(in.Source))
	if err != nil {
		return err
	}
	if !admitted {
		a.limitExceeded(ctx, exceeded)
		return rejected(RateLimited)
	}

	// 資格の確認を、識別名の確認より先に行う。識別名が使われているかどうかを、
	// 案内を受け取った人にだけ示すためである(RL-08)。
	signup, found, err := c.signups.Find(ctx, in.Credential, now)
	if err != nil {
		return err
	}
	if !found {
		return rejected(InvalidSignupCredential)
	}

	// 正しくない項目を、すべて挙げる(UC-01 E1)。
	var invalid []InputField
	handle, err := domain.NewHandle(in.Handle)
	if err != nil {
		invalid = append(invalid, FieldHandle)
	}
	name, err := domain.NewDisplayName(in.DisplayName)
	if err != nil {
		invalid = append(invalid, FieldDisplayName)
	}
	secret, err := domain.NewSecret(in.Secret, signup.Contact, in.Handle)
	if err != nil || c.commonSecrets.Contains(secret) {
		invalid = append(invalid, FieldSecret)
	}
	if len(invalid) > 0 {
		return rejectedFields(invalid...)
	}

	// 秘密の計算は、順番を待つことがある。トランザクションの前に行い、
	// その間、申し込みの行のロックを保たない。
	hash, err := c.hasher.Hash(ctx, secret)
	if err != nil {
		return err
	}

	// 申し込みを使用済みにすることと、利用者を作ることを、一体として行う(ADR-016)。
	// 関数の中では、渡された操作のほかに、ポートを呼ばない。
	var user domain.User
	err = c.tx.InSignupTransaction(ctx, func(ctx context.Context, w port.SignupWriter) error {
		consumed, found, err := w.ConsumeSignup(ctx, in.Credential, now)
		if err != nil {
			return err
		}
		if !found {
			return errSignupConsumed
		}
		user, err = w.CreateUser(ctx, consumed.Contact, handle, name, hash)
		return err
	})
	switch {
	case errors.Is(err, errSignupConsumed):
		// 同時の完了か、資格を確かめた後の、再度の申し込みである。
		return rejected(InvalidSignupCredential)
	case errors.Is(err, port.ErrHandleTaken):
		// 取り消されるため、申し込みは、使用済みにならない(UC-01 E2)。
		return rejected(HandleTaken)
	case errors.Is(err, port.ErrContactTaken):
		// 登録済みの連絡先に、申し込みが残っていた。申し込み直しを求める。
		return rejected(InvalidSignupCredential)
	case err != nil:
		return err
	}

	a.identify(user.ID)
	a.record(ctx, domain.EventSignupCompleted)
	return nil
}
