package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewBody_Accepts(t *testing.T) {
	// 基本多言語面の外の文字。UTF-16 で2つ、UTF-8 で4バイトになる。
	const astral = "\U0001F600"
	// 複数のコードポイントを結合した絵文字(U+200D を含む)。
	const family = "\U0001F468\U0000200D\U0001F469\U0000200D\U0001F467"

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"通常の本文", "こんにちは", "こんにちは"},
		{"前後の空白を保つ", "  hello  ", "  hello  "},
		{"4,000文字", strings.Repeat("a", 4000), strings.Repeat("a", 4000)},
		{"基本多言語面の外の文字だけで4,000文字", strings.Repeat(astral, 4000), strings.Repeat(astral, 4000)},
		{"U+200B だけ(White_Space ではない)", "\U0000200B", "\U0000200B"},
		{"U+FEFF だけ(White_Space ではない)", "\U0000FEFF", "\U0000FEFF"},
		{"改行(LF)とタブを含む", "a\n\tb", "a\n\tb"},
		{"CRLF は LF になる", "a\r\nb", "a\nb"},
		{"単独の CR は LF になる", "a\rb", "a\nb"},
		{"CR と CRLF が続く", "a\r\r\nb", "a\n\nb"},
		{"正規化の前は4,001文字、後は4,000文字", strings.Repeat("a", 3999) + "\r\n", strings.Repeat("a", 3999) + "\n"},
		{"結合した絵文字を、そのまま保つ", family, family},
		{"右から左へ書く文字と、書式を制御する文字", "\U0000202Eabc\U0000202C", "\U0000202Eabc\U0000202C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := NewBody(tt.raw)
			if err != nil {
				t.Fatalf("NewBody がエラーを返した: %v", err)
			}
			if got := body.String(); got != tt.want {
				t.Fatalf("本文が違う: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewBody_Rejects(t *testing.T) {
	const astral = "\U0001F600"

	tests := []struct {
		name string
		raw  string
	}{
		{"空の文字列", ""},
		{"U+0020 だけ", "   "},
		{"U+3000 だけ", "\U00003000"},
		{"U+00A0 だけ", "\U000000A0"},
		{"U+2028 だけ", "\U00002028"},
		{"改行だけ", "\n\n"},
		{"CRLF だけ", "\r\n"},
		{"タブだけ", "\t"},
		{"空白文字の組み合わせだけ", " \t\n\U00003000\U000000A0\U00002028"},
		{"4,001文字", strings.Repeat("a", 4001)},
		{"基本多言語面の外の文字だけで4,001文字", strings.Repeat(astral, 4001)},
		{"正規化の後も4,001文字", strings.Repeat("a", 4000) + "\r\n"},
		{"U+0000 を含む", "a\x00b"},
		{"U+001F を含む", "a\x1Fb"},
		{"U+007F を含む", "a\x7Fb"},
		{"U+0085 を含む", "a\u0085b"},
		{"U+009F を含む", "a\u009Fb"},
		{"正しくない UTF-8 の並び", "a\xFFb"},
		{"途中で切れた UTF-8 の並び", "a\xE3\x81"},
		{"UTF-8 の規則で符号化した、対にならない上位サロゲート", "a\xED\xA0\x80b"},
		{"UTF-8 の規則で符号化した、対にならない下位サロゲート", "a\xED\xB0\x80b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewBody(tt.raw); !errors.Is(err, ErrInvalidBody) {
				t.Fatalf("ErrInvalidBody を返さなかった: %v", err)
			}
		})
	}
}

func TestRestoreBody_DoesNotValidate(t *testing.T) {
	// 規則を変えた後も、変える前に保管した値を読めるように、検証しない。
	stored := strings.Repeat("a", 4001)
	if got := RestoreBody(stored).String(); got != stored {
		t.Fatalf("保管した値と違う")
	}
}
