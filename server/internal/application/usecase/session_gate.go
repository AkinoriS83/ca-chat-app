package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// SessionGate は、ログイン状態が有効であることを確かめる(ADR-019)。
// main が1つ作り、認証を要するユースケースに渡す。各ユースケースは、最初の手順として、verify を呼ぶ。
// 確かめる操作は、公開しない。adapter からは、呼べない。
type SessionGate struct {
	sessions port.SessionStore
	clock    port.Clock
}

// NewSessionGate は、SessionGate を作る。
func NewSessionGate(sessions port.SessionStore, clock port.Clock) *SessionGate {
	return &SessionGate{sessions: sessions, clock: clock}
}

// verify は、操作を行う利用者のログイン状態を確かめ、ログイン状態と利用者を返す。
// 有効でない場合は、どの理由でも、Unauthenticated で拒否し、認証の拒否を記録する(RL-15)。
// 確かめられなかった場合は、エラーを返す。操作は、受け付けない(RL-23)。
// 確認できた利用者は、a に入れる。
func (g *SessionGate) verify(ctx context.Context, a *attempt, actor Actor) (domain.Session, domain.User, error) {
	if actor.Credential == "" || actor.SessionID == "" {
		return g.reject(ctx, a)
	}
	session, user, found, err := g.sessions.Find(ctx, actor.Credential, g.clock.Now())
	if err != nil {
		return domain.Session{}, domain.User{}, err
	}
	// 識別子が合わない要求は、別のログイン状態の端末からのものである(ADR-014)。
	if !found || string(session.ID) != actor.SessionID {
		return g.reject(ctx, a)
	}
	a.identify(user.ID)
	return session, user, nil
}

func (g *SessionGate) reject(ctx context.Context, a *attempt) (domain.Session, domain.User, error) {
	a.record(ctx, domain.EventAuthenticationRejected)
	return domain.Session{}, domain.User{}, rejected(Unauthenticated)
}
