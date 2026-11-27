package domain

import (
	"errors"
	"testing"
)

// RL-06: 識別名の規則の、境界の内側。入力された通りに保持する(RL-D6)。
func TestNewHandle_Accepts(t *testing.T) {
	for _, raw := range []string{"abc", "Alice_01", "___", "A1b2C3d4E5f6G7h8I9j0", "000"} {
		t.Run(raw, func(t *testing.T) {
			handle, err := NewHandle(raw)
			if err != nil {
				t.Fatalf("NewHandle がエラーを返した: %v", err)
			}
			if got := handle.String(); got != raw {
				t.Fatalf("識別名が違う: got %q, want %q", got, raw)
			}
		})
	}
}

// RL-06: 識別名の規則の、境界の外側。
func TestNewHandle_Rejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"空の文字列", ""},
		{"2文字", "ab"},
		{"21文字", "A1b2C3d4E5f6G7h8I9j0k"},
		{"ハイフン", "ali-ce"},
		{"空白", "ali ce"},
		{"前後の空白(除かない)", " alice "},
		{"ピリオド", "ali.ce"},
		{"ASCII でない英字", "al\U000000EDce"},
		{"全角の英字", "\U0000FF41\U0000FF42\U0000FF43"},
		{"漢字1文字(3バイト)", "\U00006F22"},
		{"改行", "alice\n"},
		{"正しくない UTF-8 の並び", "ali\xffce"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewHandle(tt.raw); !errors.Is(err, ErrInvalidHandle) {
				t.Fatalf("ErrInvalidHandle を返さなかった: %v", err)
			}
		})
	}
}

// 重複と、照合の判定では、大文字と小文字を区別しない(RL-D6)。
func TestHandle_Key(t *testing.T) {
	handle, err := NewHandle("Alice_01")
	if err != nil {
		t.Fatal(err)
	}
	if got := handle.Key(); got != "alice_01" {
		t.Fatalf("Key が違う: %q", got)
	}
	if got := RestoreHandle("BOB").Key(); got != "bob" {
		t.Fatalf("Key が違う: %q", got)
	}
}
