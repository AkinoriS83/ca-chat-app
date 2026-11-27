package domain

import (
	"errors"
	"strings"
	"testing"
)

// RL-06: 表示名の規則の、境界の内側。
func TestNewDisplayName_Accepts(t *testing.T) {
	const astral = "\U0001F600"

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"通常の表示名", "Alice", "Alice"},
		{"1文字", "a", "a"},
		{"50文字", strings.Repeat("a", 50), strings.Repeat("a", 50)},
		{"基本多言語面の外の文字だけで50文字", strings.Repeat(astral, 50), strings.Repeat(astral, 50)},
		{"前後の空白文字を除く", "  山田 太郎\U00003000", "山田 太郎"},
		{"前後の改行とタブは、空白文字として除く", "\n\tAlice\r\n", "Alice"},
		{"除いた後に50文字", " " + strings.Repeat("a", 50) + " ", strings.Repeat("a", 50)},
		{"中の空白は、保つ", "a  b", "a  b"},
		{"画面の構造として解釈されうる記述", "<script>alert(1)</script>", "<script>alert(1)</script>"},
		{"U+200B だけ(White_Space ではない)", "\U0000200B", "\U0000200B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := NewDisplayName(tt.raw)
			if err != nil {
				t.Fatalf("NewDisplayName がエラーを返した: %v", err)
			}
			if got := name.String(); got != tt.want {
				t.Fatalf("表示名が違う: got %q, want %q", got, tt.want)
			}
		})
	}
}

// RL-06: 表示名の規則の、境界の外側。
func TestNewDisplayName_Rejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"空の文字列", ""},
		{"空白文字だけ", " \U00003000\t"},
		{"51文字", strings.Repeat("a", 51)},
		{"中の改行", "a\nb"},
		{"中のタブ", "a\tb"},
		{"制御文字(U+0000)", "a\x00b"},
		{"制御文字(U+007F)", "a\x7fb"},
		{"制御文字(U+0085)", "a\U00000085b"},
		{"正しくない UTF-8 の並び", "a\xffb"},
		{"対にならないサロゲート", "a\xed\xa0\x80b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewDisplayName(tt.raw); !errors.Is(err, ErrInvalidDisplayName) {
				t.Fatalf("ErrInvalidDisplayName を返さなかった: %v", err)
			}
		})
	}
}
