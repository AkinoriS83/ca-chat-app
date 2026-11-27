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
	// InvalidFields は、入力の項目(連絡先、識別名、表示名、秘密)が、規則を満たさないことを示す。
	InvalidFields
	// RateLimited は、上限を超えたことを示す。
	RateLimited
	// InvalidSignupCredential は、確認のための資格が、無効であることを示す。理由は、区別しない。
	InvalidSignupCredential
	// HandleTaken は、識別名が、使われていることを示す。
	HandleTaken
	// LoginFailed は、ログインの照合に、失敗したことを示す。理由は、区別しない。
	LoginFailed
)

// InputField は、入力の項目である。InvalidFields の拒否が、正しくない項目を挙げるために使う。
type InputField int

const (
	// FieldContact は、連絡先である。
	FieldContact InputField = iota + 1
	// FieldHandle は、識別名である。
	FieldHandle
	// FieldDisplayName は、表示名である。
	FieldDisplayName
	// FieldSecret は、秘密である。
	FieldSecret
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
	case InvalidFields:
		return "invalid fields"
	case RateLimited:
		return "rate limited"
	case InvalidSignupCredential:
		return "invalid signup credential"
	case HandleTaken:
		return "handle taken"
	case LoginFailed:
		return "login failed"
	default:
		return "unknown rejection"
	}
}

// RejectedError は、ユースケースが操作を拒否したことを示す。
// ユースケースは、拒否を *RejectedError で、完了できなかったことを、そのほかの error で返す。
// 拒否には、秘密と資格を含めない(ADR-019)。
type RejectedError struct {
	Kind Rejection
	// Fields は、正しくない項目のすべてである。InvalidFields の場合だけ。
	Fields []InputField
}

func (e *RejectedError) Error() string {
	return "usecase: rejected: " + e.Kind.String()
}

func rejected(kind Rejection) error {
	return &RejectedError{Kind: kind}
}

func rejectedFields(fields ...InputField) error {
	return &RejectedError{Kind: InvalidFields, Fields: fields}
}
