package usecase

// Rejection は、ユースケースが操作を拒否した理由の種類である。
// domain が返した規則の違反は、usecase が、この種類に置き換える(ADR-005)。
type Rejection int

const (
	// Unauthenticated は、利用者を特定できないことを示す。
	Unauthenticated Rejection = iota + 1
	// InvalidBody は、本文が正しくないことを示す。
	InvalidBody
	// InvalidInput は、入力が正しくないことを示す。
	InvalidInput
	// InvalidPeer は、相手にできないことを示す。
	InvalidPeer
	// NotFound は、見つからないことを示す。
	NotFound
)

func (r Rejection) String() string {
	switch r {
	case Unauthenticated:
		return "unauthenticated"
	case InvalidBody:
		return "invalid body"
	case InvalidInput:
		return "invalid input"
	case InvalidPeer:
		return "invalid peer"
	case NotFound:
		return "not found"
	default:
		return "unknown rejection"
	}
}

// RejectedError は、ユースケースが操作を拒否したことを示す。
// ユースケースは、拒否を *RejectedError で、完了できなかったことを、そのほかの error で返す。
type RejectedError struct {
	Kind Rejection
}

func (e *RejectedError) Error() string {
	return "usecase: rejected: " + e.Kind.String()
}

func rejected(kind Rejection) error {
	return &RejectedError{Kind: kind}
}
