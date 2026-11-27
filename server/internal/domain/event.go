package domain

// EventKind は、記録する出来事の種類である(R-6、仕様「記録」)。
type EventKind int

const (
	// EventSignupRequested は、登録の申し込みを、受け付けたことを示す。
	EventSignupRequested EventKind = iota + 1
	// EventSignupNoticeFailed は、案内を、送れなかったことを示す。
	EventSignupNoticeFailed
	// EventSignupCompleted は、登録を完了したことを示す。
	EventSignupCompleted
	// EventLoginSucceeded は、ログインの照合に、成功したことを示す。
	EventLoginSucceeded
	// EventLoginFailed は、ログインの照合に、失敗したことを示す。
	EventLoginFailed
	// EventLoggedOut は、ログアウトを完了したことを示す。
	EventLoggedOut
	// EventAuthenticationRejected は、ログイン状態が有効でない要求を、拒否したことを示す。
	EventAuthenticationRejected
	// EventLimitExceeded は、上限によって、拒否したことを示す。
	EventLimitExceeded
	// EventOperationFailed は、操作を完了できなかったことを示す。
	EventOperationFailed
)

// String は、出来事の種類の名前を返す。
func (k EventKind) String() string {
	switch k {
	case EventSignupRequested:
		return "signup_requested"
	case EventSignupNoticeFailed:
		return "signup_notice_failed"
	case EventSignupCompleted:
		return "signup_completed"
	case EventLoginSucceeded:
		return "login_succeeded"
	case EventLoginFailed:
		return "login_failed"
	case EventLoggedOut:
		return "logged_out"
	case EventAuthenticationRejected:
		return "authentication_rejected"
	case EventLimitExceeded:
		return "limit_exceeded"
	case EventOperationFailed:
		return "operation_failed"
	default:
		return "unknown"
	}
}

// Event は、記録する出来事である。発生時刻は、記録する側が付ける(ADR-021)。
//
// 連絡先、識別名、表示名、秘密、資格、本文の項目は、置かない。記録しないと定めたものである
// (仕様「記録」、RL-D8)。
type Event struct {
	Kind   EventKind
	Source string // 送信元。要求によらない操作(片付け)では、空
	UserID UserID // 利用者の識別子。特定できていない場合は、空
	// Limit は、数える単位の種類である。EventLimitExceeded の場合だけ。
	Limit LimitKind
	// Operation と Error は、操作の名前と、エラーの文言である。EventOperationFailed の場合だけ。
	// エラーの文言は、adapter が、行の内容や宛先を含まない形に作り直したものに限る。
	Operation string
	Error     string
}
