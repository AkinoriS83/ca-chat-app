package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// Authenticator は、「ログインする」ユースケースである(UC-02)。
type Authenticator struct {
	users       port.UserDirectory
	hasher      port.SecretHasher
	sessions    port.SessionStore
	connections port.SessionConnections
	credentials port.CredentialSource
	limiter     port.AttemptLimiter
	rules       domain.LimitRules
	clock       port.Clock
	events      port.EventRecorder
}

// NewAuthenticator は、Authenticator を作る。
func NewAuthenticator(users port.UserDirectory, hasher port.SecretHasher, sessions port.SessionStore, connections port.SessionConnections, credentials port.CredentialSource, limiter port.AttemptLimiter, rules domain.LimitRules, clock port.Clock, events port.EventRecorder) *Authenticator {
	return &Authenticator{users: users, hasher: hasher, sessions: sessions, connections: connections, credentials: credentials, limiter: limiter, rules: rules, clock: clock, events: events}
}

// Login は、連絡先と秘密を照合し、新しいログイン状態を与える。
//
// 失敗の経路は、どれも、秘密の計算を、ちょうど1回行う(RL-13)。上限による拒否は、除く。
// 失敗は、照合の前に数え、照合に成功したときに、戻す。確かめることと数えることを分けると、
// 同時の要求が、上限を超えて照合に進むためである。
func (l *Authenticator) Login(ctx context.Context, in LoginInput) (out LoginOutput, err error) {
	a := begin(l.events, "login", in.Source)
	defer a.finish(ctx, &err)

	// 規則を満たさない連絡先は、照合の失敗として扱う。連絡先ごとには、数えない。
	contact, contactErr := domain.NewContact(in.Contact)
	contactValid := contactErr == nil

	now := l.clock.Now()
	bySource := l.rules.LoginFailureBySource(in.Source)
	byContact := l.rules.LoginFailureByContact(contact)
	limits := []domain.Limit{bySource}
	if contactValid {
		limits = append(limits, byContact)
	}
	exceeded, admitted, err := l.limiter.Admit(ctx, now, limits...)
	if err != nil {
		return LoginOutput{}, err
	}
	if !admitted {
		// 照合は、行わない。
		a.limitExceeded(ctx, exceeded)
		return LoginOutput{}, rejected(RateLimited)
	}

	matched, user, err := l.verify(ctx, a, contact, contactValid, in.Secret)
	if err != nil {
		return LoginOutput{}, err
	}
	if !matched {
		// それまでのログイン状態は、変えない。数えた1回は、失敗として残る。
		a.record(ctx, domain.EventLoginFailed)
		return LoginOutput{}, rejected(LoginFailed)
	}

	if err := l.limiter.Forgive(ctx, bySource); err != nil {
		return LoginOutput{}, err
	}
	if err := l.limiter.Reset(ctx, byContact); err != nil {
		return LoginOutput{}, err
	}

	// ログイン状態を持つブラウザでのログインでは、それまでのログイン状態を無効にする(RL-14)。
	// 無効にしてから、接続を閉じる。逆の順では、閉じた後、無効にする前に、同じ資格で接続できる。
	if in.Credential != "" {
		previous, found, err := l.sessions.Revoke(ctx, in.Credential)
		if err != nil {
			return LoginOutput{}, err
		}
		if found {
			l.connections.CloseBySession(ctx, previous)
		}
	}

	credential, err := l.credentials.NewCredential()
	if err != nil {
		return LoginOutput{}, err
	}
	session, err := l.sessions.Create(ctx, credential, user.ID, now.Add(domain.SessionLifetime))
	if err != nil {
		return LoginOutput{}, err
	}

	a.record(ctx, domain.EventLoginSucceeded)
	return LoginOutput{
		Credential: credential,
		SessionID:  string(session.ID),
		ExpiresAt:  session.ExpiresAt,
		User:       toUserView(user),
	}, nil
}

// verify は、連絡先に当たる利用者の、保管した秘密と照合する。
// 照合する相手がない場合(連絡先が正しくない、登録されていない)は、同じ重さの計算を、1回行う。
// 秘密は、規則で検証しない。規則を満たさない秘密は、保管した値と一致しない。
func (l *Authenticator) verify(ctx context.Context, a *attempt, contact domain.Contact, contactValid bool, secret string) (bool, domain.User, error) {
	if contactValid {
		user, stored, found, err := l.users.FindByContact(ctx, contact)
		if err != nil {
			return false, domain.User{}, err
		}
		if found {
			a.identify(user.ID)
			matched, err := l.hasher.Verify(ctx, secret, stored)
			return matched, user, err
		}
	}
	return false, domain.User{}, l.hasher.VerifyDummy(ctx)
}

// SessionKeeper は、「現在のログイン状態を確かめる」と「ログアウトする」ユースケースである(UC-03)。
type SessionKeeper struct {
	gate        *SessionGate
	sessions    port.SessionStore
	connections port.SessionConnections
	clock       port.Clock
	events      port.EventRecorder
}

// NewSessionKeeper は、SessionKeeper を作る。
func NewSessionKeeper(gate *SessionGate, sessions port.SessionStore, connections port.SessionConnections, clock port.Clock, events port.EventRecorder) *SessionKeeper {
	return &SessionKeeper{gate: gate, sessions: sessions, connections: connections, clock: clock, events: events}
}

// Current は、資格に当たる、ログイン中の利用者を返す。
// ログイン状態が有効でない場合は、拒否とせず、ログインしていないことを示す。理由は、区別せず、
// 認証の拒否として記録しない(RL-D9)。確かめられなかった場合は、エラーを返す(RL-23)。
func (k *SessionKeeper) Current(ctx context.Context, in CurrentSessionInput) (out CurrentSessionOutput, err error) {
	a := begin(k.events, "current session", in.Source)
	defer a.finish(ctx, &err)

	if in.Credential == "" {
		return CurrentSessionOutput{}, nil
	}
	session, user, found, err := k.sessions.Find(ctx, in.Credential, k.clock.Now())
	if err != nil {
		return CurrentSessionOutput{}, err
	}
	if !found {
		return CurrentSessionOutput{}, nil
	}
	return CurrentSessionOutput{Authenticated: true, SessionID: string(session.ID), User: toUserView(user)}, nil
}

// Logout は、ログイン状態を無効にし、そのログイン状態による接続を閉じる。
// 無効にしてから、閉じる(Login と同じ順である)。
func (k *SessionKeeper) Logout(ctx context.Context, in LogoutInput) (err error) {
	a := begin(k.events, "logout", in.Source)
	defer a.finish(ctx, &err)

	session, _, err := k.gate.verify(ctx, a, in.Actor)
	if err != nil {
		return err
	}
	if _, _, err := k.sessions.Revoke(ctx, in.Actor.Credential); err != nil {
		return err
	}
	k.connections.CloseBySession(ctx, session.ID)

	a.record(ctx, domain.EventLoggedOut)
	return nil
}
