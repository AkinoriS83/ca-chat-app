package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxBodyLength は、本文の長さの上限である。Unicode のコードポイントで数える(仕様 WS-D3)。
const MaxBodyLength = 4000

// ErrInvalidBody は、本文が、仕様 WS-D3 の規則を満たさないことを示す。
var ErrInvalidBody = errors.New("domain: invalid message body")

// Body は、メッセージの本文である。
// 中の値は、パッケージの外から直接には作れない。NewBody か RestoreBody で作る。
type Body struct {
	value string
}

// NewBody は、入力から本文を作る。
// 改行を正規化し(CRLF と、単独の CR を、LF に置き換える)、仕様 WS-D3 の規則を確かめる。
// 規則を満たさない場合は、ErrInvalidBody を返す。
func NewBody(raw string) (Body, error) {
	// 1. 文字列として正しいこと。対にならないサロゲートは、正しくない UTF-8 の並びとして渡される。
	if !utf8.ValidString(raw) {
		return Body{}, ErrInvalidBody
	}

	// 2. 改行の正規化。以後は、正規化した後の値を扱う。
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	hasNonSpace := false
	length := 0
	for _, r := range normalized {
		// 3. 制御文字を含まないこと。改行とタブは、除く。
		if unicode.Is(unicode.Cc, r) && r != '\n' && r != '\t' {
			return Body{}, ErrInvalidBody
		}
		// 4. 空白文字以外の文字を、1文字以上含むこと。
		if !unicode.Is(unicode.White_Space, r) {
			hasNonSpace = true
		}
		length++
	}
	if !hasNonSpace {
		return Body{}, ErrInvalidBody
	}

	// 5. コードポイントの数が、上限以下であること。
	if length > MaxBodyLength {
		return Body{}, ErrInvalidBody
	}
	return Body{value: normalized}, nil
}

// RestoreBody は、保管した値から、本文を組み立てる。規則は、確かめない。
// 保管した値は、保管の時点で、NewBody の検証を通っているためである。
// 呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする(規約)。
func RestoreBody(stored string) Body {
	return Body{value: stored}
}

// String は、本文の文字列を返す。
func (b Body) String() string { return b.value }
