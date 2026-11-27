package domain

import (
	"errors"
	"strings"
)

// 連絡先の長さの上限である(仕様「入力の規則」)。
const (
	MaxContactLength      = 254
	maxContactLocalLength = 64
	maxContactLabelLength = 63
)

// ErrInvalidContact は、連絡先が、仕様「入力の規則」を満たさないことを示す。
var ErrInvalidContact = errors.New("domain: invalid contact")

// Contact は、連絡先(メールアドレス)である。前後の空白文字を除き、英字を小文字に揃えた値を保持する。
// 中の値は、パッケージの外から直接には作れない。NewContact か RestoreContact で作る。
type Contact struct {
	value string
}

// NewContact は、入力から連絡先を作る。
// 前後の空白文字を除き、英字を小文字に揃えてから、仕様「入力の規則」を確かめる。
// 規則を満たさない場合は、ErrInvalidContact を返す。
func NewContact(raw string) (Contact, error) {
	value := lowerASCII(strings.TrimSpace(raw))

	// ASCII の文字だけからなるため、バイトの数が、文字の数である。
	if len(value) > MaxContactLength {
		return Contact{}, ErrInvalidContact
	}
	// ローカル部は、@ を含まない。@ は、ちょうど1つである。
	local, domainPart, found := strings.Cut(value, "@")
	if !found || strings.Contains(domainPart, "@") {
		return Contact{}, ErrInvalidContact
	}
	if !isContactLocal(local) || !isContactDomain(domainPart) {
		return Contact{}, ErrInvalidContact
	}
	return Contact{value: value}, nil
}

// RestoreContact は、保管した値から、連絡先を組み立てる。規則は、確かめない。
// 呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする(規約)。
func RestoreContact(stored string) Contact {
	return Contact{value: stored}
}

// String は、連絡先の文字列(揃えた後の値)を返す。
func (c Contact) String() string { return c.value }

// lowerASCII は、ASCII の英大文字だけを、小文字にする。ほかのバイトは、そのまま写す。
func lowerASCII(s string) string {
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func isASCIILetterOrDigit(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

// isContactLocal は、ローカル部が、1文字以上、64文字以下で、英数字と、定めた記号だけからなるかを返す。
func isContactLocal(local string) bool {
	if len(local) < 1 || len(local) > maxContactLocalLength {
		return false
	}
	for i := 0; i < len(local); i++ {
		c := local[i]
		if !isASCIILetterOrDigit(c) && !strings.ContainsRune(".!#$%&'*+/=?^_`{|}~-", rune(c)) {
			return false
		}
	}
	return true
}

// isContactDomain は、ドメインが、. で区切った、2つ以上のラベルからなるかを返す。
// 各ラベルは、1文字以上、63文字以下の、英数字とハイフンからなり、ハイフンで始まらず、終わらない。
func isContactDomain(domainPart string) bool {
	labels := strings.Split(domainPart, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > maxContactLabelLength {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if !isASCIILetterOrDigit(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}
