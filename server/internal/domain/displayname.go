package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxDisplayNameLength は、表示名の長さの上限である。Unicode のコードポイントで数える。
const MaxDisplayNameLength = 50

// ErrInvalidDisplayName は、表示名が、仕様「入力の規則」を満たさないことを示す。
var ErrInvalidDisplayName = errors.New("domain: invalid display name")

// DisplayName は、表示名である。前後の空白文字を除いた値を保持する。
// 中の値は、パッケージの外から直接には作れない。NewDisplayName か RestoreDisplayName で作る。
type DisplayName struct {
	value string
}

// NewDisplayName は、入力から表示名を作る。
// 前後の空白文字を除いてから、1文字以上、50文字以下で、文字列として正しく、
// 制御文字(改行とタブを含む)を含まないことを確かめる。
// 規則を満たさない場合は、ErrInvalidDisplayName を返す。
func NewDisplayName(raw string) (DisplayName, error) {
	if !utf8.ValidString(raw) {
		return DisplayName{}, ErrInvalidDisplayName
	}
	value := strings.TrimSpace(raw)

	length := 0
	for _, r := range value {
		if unicode.Is(unicode.Cc, r) {
			return DisplayName{}, ErrInvalidDisplayName
		}
		length++
	}
	if length < 1 || length > MaxDisplayNameLength {
		return DisplayName{}, ErrInvalidDisplayName
	}
	return DisplayName{value: value}, nil
}

// RestoreDisplayName は、保管した値から、表示名を組み立てる。規則は、確かめない。
// 呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする(規約)。
func RestoreDisplayName(stored string) DisplayName {
	return DisplayName{value: stored}
}

// String は、表示名の文字列を返す。
func (n DisplayName) String() string { return n.value }
