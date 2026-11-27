package domain

import "errors"

// 識別名の長さの下限と上限である(仕様「入力の規則」)。
const (
	MinHandleLength = 3
	MaxHandleLength = 20
)

// ErrInvalidHandle は、識別名が、仕様「入力の規則」を満たさないことを示す。
var ErrInvalidHandle = errors.New("domain: invalid handle")

// Handle は、識別名である。入力された通りに(大文字と小文字を保って)保持する。
// 中の値は、パッケージの外から直接には作れない。NewHandle か RestoreHandle で作る。
type Handle struct {
	value string
}

// NewHandle は、入力から識別名を作る。
// 3文字以上、20文字以下で、英字、数字、アンダースコアだけからなることを確かめる。
// 規則を満たさない場合は、ErrInvalidHandle を返す。
func NewHandle(raw string) (Handle, error) {
	// 使える文字は、ASCII だけであるため、バイトの数が、文字の数である。
	if len(raw) < MinHandleLength || len(raw) > MaxHandleLength {
		return Handle{}, ErrInvalidHandle
	}
	for i := 0; i < len(raw); i++ {
		if !isASCIILetterOrDigit(raw[i]) && raw[i] != '_' {
			return Handle{}, ErrInvalidHandle
		}
	}
	return Handle{value: raw}, nil
}

// RestoreHandle は、保管した値から、識別名を組み立てる。規則は、確かめない。
// 呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする(規約)。
func RestoreHandle(stored string) Handle {
	return Handle{value: stored}
}

// String は、識別名を、登録した通りの文字列で返す。
func (h Handle) String() string { return h.value }

// Key は、小文字に揃えた値を返す。重複と、照合の判定に使う(大文字と小文字を、区別しない)。
func (h Handle) Key() string { return lowerASCII(h.value) }
