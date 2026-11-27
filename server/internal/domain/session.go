package domain

import "time"

// 有効期限である(仕様「ログイン状態」「登録する」)。
const (
	// SessionLifetime は、ログイン状態の有効期限である。ログインから数える。操作によって、延びない。
	SessionLifetime = 30 * 24 * time.Hour
	// SignupLifetime は、確認のための資格の有効期限である。申し込みから数える。
	SignupLifetime = 24 * time.Hour
)

// SessionID は、ログイン状態の識別子である(ADR-014)。UUID の文字列(小文字)で表す。
// 資格とは、別の値である。秘密ではなく、これだけでは、操作できない。
type SessionID string

// Session は、ログイン状態である。
type Session struct {
	ID        SessionID
	UserID    UserID
	ExpiresAt time.Time
}

// ValidAt は、now の時点で、ログイン状態が有効であるか(now が、期限より前であるか)を返す。
// 保管先の問い合わせ(expires_at > now)と、同じ判定である。
// 配信の adapter も、これを呼ぶ(ADR-020)。
func (s Session) ValidAt(now time.Time) bool {
	return now.Before(s.ExpiresAt)
}

// Signup は、登録の申し込みである。
type Signup struct {
	Contact   Contact
	ExpiresAt time.Time
}
