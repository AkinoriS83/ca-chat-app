package domain

import (
	"errors"
	"strings"
	"testing"
)

func mustContact(t *testing.T, raw string) Contact {
	t.Helper()
	contact, err := NewContact(raw)
	if err != nil {
		t.Fatal(err)
	}
	return contact
}

// RL-06: 秘密の規則の、境界の内側。入力された通りに扱う(RL-D3)。
func TestNewSecret_Accepts(t *testing.T) {
	const astral = "\U0001F600"
	contact := mustContact(t, "alice@example.com")

	tests := []struct {
		name string
		raw  string
	}{
		{"15文字", strings.Repeat("a", 15)},
		{"128文字", strings.Repeat("a", 128)},
		{"基本多言語面の外の文字だけで15文字", strings.Repeat(astral, 15)},
		{"基本多言語面の外の文字だけで128文字(512バイト)", strings.Repeat(astral, 128)},
		{"前後の空白を保つ", "  correct horse  "},
		{"空白だけ(文字の種類は、問わない)", strings.Repeat(" ", 15)},
		{"改行と制御文字を含む", "abc\ndef\tghi\x00jklm"},
		{"連絡先を含むが、同じではない", "alice@example.com1"},
		{"識別名を含むが、同じではない", "Alice_0123456789x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, err := NewSecret(tt.raw, contact, "Alice_0123456789")
			if err != nil {
				t.Fatalf("NewSecret がエラーを返した: %v", err)
			}
			if got := secret.Expose(); got != tt.raw {
				t.Fatalf("秘密が、入力された通りでない: got %q, want %q", got, tt.raw)
			}
		})
	}
}

// RL-06: 秘密の規則の、境界の外側。
func TestNewSecret_Rejects(t *testing.T) {
	const astral = "\U0001F600"
	contact := mustContact(t, "alice@example.com")

	tests := []struct {
		name string
		raw  string
	}{
		{"空の文字列", ""},
		{"14文字", strings.Repeat("a", 14)},
		{"基本多言語面の外の文字だけで14文字(56バイト)", strings.Repeat(astral, 14)},
		{"129文字", strings.Repeat("a", 129)},
		{"正しくない UTF-8 の並び", strings.Repeat("a", 15) + "\xff"},
		{"対にならないサロゲート", strings.Repeat("a", 15) + "\xed\xa0\x80"},
		{"連絡先と同じ", "alice@example.com"},
		{"連絡先と、大文字と小文字だけが異なる", "ALICE@Example.com"},
		{"識別名と同じ", "Alice_0123456789"},
		{"識別名と、大文字と小文字だけが異なる", "alice_0123456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSecret(tt.raw, contact, "Alice_0123456789"); !errors.Is(err, ErrInvalidSecret) {
				t.Fatalf("ErrInvalidSecret を返さなかった: %v", err)
			}
		})
	}
}

// 文字列にする操作は、値を返さない。書式とログでの確認は、cmd/server のテストにある
// (domain のテストは、書式とログの標準ライブラリを import できない)。
func TestSecret_StringDoesNotExposeValue(t *testing.T) {
	const raw = "marker-secret-value-123"
	secret, err := NewSecret(raw, mustContact(t, "alice@example.com"), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(secret.String(), raw) || strings.Contains(secret.GoString(), raw) {
		t.Fatal("文字列にする操作が、秘密の値を返した")
	}
}
