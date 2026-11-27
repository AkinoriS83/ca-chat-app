package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// 秘密の長さの下限と上限である。Unicode のコードポイントで数える(仕様「入力の規則」、RL-D3)。
const (
	MinSecretLength = 15
	MaxSecretLength = 128
)

// ErrInvalidSecret は、秘密が、仕様「入力の規則」を満たさないことを示す。
var ErrInvalidSecret = errors.New("domain: invalid secret")

// redactedSecret は、秘密を文字列にする操作が返す、伏せた文字列である。
const redactedSecret = "[secret]"

// Secret は、認証に用いる秘密である。入力された通りに保持する。
//
// 公開しない項目を持つ構造体とし、文字列にする操作(String、GoString)では、伏せた文字列を返す。
// 誤って、ログやエラーの文言に差し込んでも、値が現れないようにするためである(SEC-1)。
// 値は、Expose でだけ取り出す。保管した値から組み立てる関数は、置かない(保管するのは、SecretHash である)。
type Secret struct {
	value string
}

// NewSecret は、入力から秘密を作る。contact は、本人の連絡先、rawHandle は、本人が入力した識別名である。
// 15文字以上、128文字以下で、文字列として正しく、連絡先とも、識別名とも、同じでないこと
// (大文字と小文字を区別しない)を確かめる。文字は、置き換えない。
// よく使われる秘密の一覧との照合は、ポートで行う。
// 規則を満たさない場合は、ErrInvalidSecret を返す。
func NewSecret(raw string, contact Contact, rawHandle string) (Secret, error) {
	if !utf8.ValidString(raw) {
		return Secret{}, ErrInvalidSecret
	}
	length := utf8.RuneCountInString(raw)
	if length < MinSecretLength || length > MaxSecretLength {
		return Secret{}, ErrInvalidSecret
	}
	if strings.EqualFold(raw, contact.String()) || strings.EqualFold(raw, rawHandle) {
		return Secret{}, ErrInvalidSecret
	}
	return Secret{value: raw}, nil
}

// Expose は、秘密の値を取り出す。呼ぶのは、秘密を計算に渡す箇所と、一覧と照合する箇所だけとする。
func (s Secret) Expose() string { return s.value }

// String は、伏せた文字列を返す。値は、返さない。
func (Secret) String() string { return redactedSecret }

// GoString は、伏せた文字列を返す(%#v の書式)。値は、返さない。
func (Secret) GoString() string { return redactedSecret }

// SecretHash は、保管する形の秘密である(ADR-015)。規則を持たない。
type SecretHash string
