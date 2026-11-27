package domain

import "time"

// LimitKind は、上限の種類(数える操作と、数える単位)である。仕様「上限」の表の6行に当たる。
type LimitKind int

const (
	// LimitSignupRequestPerSource は、登録の申し込みの、送信元ごとの上限である。
	LimitSignupRequestPerSource LimitKind = iota + 1
	// LimitSignupRequestPerContact は、登録の申し込みの、宛先の連絡先ごとの上限である。
	LimitSignupRequestPerContact
	// LimitSignupCompletionPerSource は、登録の完了の、送信元ごとの上限である。
	LimitSignupCompletionPerSource
	// LimitLoginFailurePerContact は、ログインの失敗の、連絡先ごとの上限である。
	LimitLoginFailurePerContact
	// LimitLoginFailurePerSource は、ログインの失敗の、送信元ごとの上限である。
	LimitLoginFailurePerSource
	// LimitPeerLookupPerUser は、相手の特定の、利用者ごとの上限である。
	LimitPeerLookupPerUser
)

// String は、種類の名前を返す。出来事の記録に使う。
func (k LimitKind) String() string {
	switch k {
	case LimitSignupRequestPerSource:
		return "signup_request_per_source"
	case LimitSignupRequestPerContact:
		return "signup_request_per_contact"
	case LimitSignupCompletionPerSource:
		return "signup_completion_per_source"
	case LimitLoginFailurePerContact:
		return "login_failure_per_contact"
	case LimitLoginFailurePerSource:
		return "login_failure_per_source"
	case LimitPeerLookupPerUser:
		return "peer_lookup_per_user"
	default:
		return "unknown"
	}
}

// LimitRule は、上限の値(回数と時間)である。
type LimitRule struct {
	Count  int
	Window time.Duration
}

// Valid は、回数と時間が、どちらも正の値であるかを返す。
func (r LimitRule) Valid() bool {
	return r.Count > 0 && r.Window > 0
}

// LimitMode は、上限の数え方である。
type LimitMode int

const (
	// LimitWindow は、「T に N 回」である。直近の T の間に数えた回数が、N に達している間、拒否する。
	LimitWindow LimitMode = iota + 1
	// LimitLockout は、「連続して N 回で、T の間、拒否する」である。直近の T の間に数えた回数が、
	// N に達すると、最後に数えた時点から T の間、拒否する。拒否の間が過ぎると、0から数える。
	LimitLockout
)

// Limit は、1つの数える対象に対する、上限である。
type Limit struct {
	Kind    LimitKind
	Subject string // 数える対象(送信元、連絡先、利用者の識別子)
	Rule    LimitRule
	Mode    LimitMode
}

// LimitRules は、上限の値の一式である。運営者が、設定で変えられる(RL-D5)。
type LimitRules struct {
	SignupRequestPerSource    LimitRule
	SignupRequestPerContact   LimitRule
	SignupCompletionPerSource LimitRule
	LoginFailurePerContact    LimitRule
	LoginFailurePerSource     LimitRule
	PeerLookupPerUser         LimitRule
}

// DefaultLimitRules は、既定値を返す(仕様「上限」の表)。
func DefaultLimitRules() LimitRules {
	return LimitRules{
		SignupRequestPerSource:    LimitRule{Count: 10, Window: time.Hour},
		SignupRequestPerContact:   LimitRule{Count: 3, Window: time.Hour},
		SignupCompletionPerSource: LimitRule{Count: 20, Window: time.Hour},
		LoginFailurePerContact:    LimitRule{Count: 10, Window: 15 * time.Minute},
		LoginFailurePerSource:     LimitRule{Count: 30, Window: 15 * time.Minute},
		PeerLookupPerUser:         LimitRule{Count: 30, Window: time.Hour},
	}
}

// SignupRequestBySource は、登録の申し込みの、送信元ごとの上限を返す。
func (r LimitRules) SignupRequestBySource(source string) Limit {
	return Limit{Kind: LimitSignupRequestPerSource, Subject: source, Rule: r.SignupRequestPerSource, Mode: LimitWindow}
}

// SignupRequestByContact は、登録の申し込みの、宛先の連絡先ごとの上限を返す。
func (r LimitRules) SignupRequestByContact(contact Contact) Limit {
	return Limit{Kind: LimitSignupRequestPerContact, Subject: contact.String(), Rule: r.SignupRequestPerContact, Mode: LimitWindow}
}

// SignupCompletionBySource は、登録の完了の、送信元ごとの上限を返す。
func (r LimitRules) SignupCompletionBySource(source string) Limit {
	return Limit{Kind: LimitSignupCompletionPerSource, Subject: source, Rule: r.SignupCompletionPerSource, Mode: LimitWindow}
}

// LoginFailureByContact は、ログインの失敗の、連絡先ごとの上限を返す。
func (r LimitRules) LoginFailureByContact(contact Contact) Limit {
	return Limit{Kind: LimitLoginFailurePerContact, Subject: contact.String(), Rule: r.LoginFailurePerContact, Mode: LimitLockout}
}

// LoginFailureBySource は、ログインの失敗の、送信元ごとの上限を返す。
func (r LimitRules) LoginFailureBySource(source string) Limit {
	return Limit{Kind: LimitLoginFailurePerSource, Subject: source, Rule: r.LoginFailurePerSource, Mode: LimitWindow}
}

// PeerLookupByUser は、相手の特定の、利用者ごとの上限を返す。
func (r LimitRules) PeerLookupByUser(user UserID) Limit {
	return Limit{Kind: LimitPeerLookupPerUser, Subject: string(user), Rule: r.PeerLookupPerUser, Mode: LimitWindow}
}
