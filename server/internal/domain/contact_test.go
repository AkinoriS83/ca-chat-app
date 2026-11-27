package domain

import (
	"errors"
	"strings"
	"testing"
)

// RL-06: 連絡先の規則の、境界の内側。
func TestNewContact_Accepts(t *testing.T) {
	label63 := strings.Repeat("a", 63)
	local64 := strings.Repeat("a", 64)
	// 254文字: 64 + 1 + 63 + 1 + 63 + 1 + 61
	max254 := local64 + "@" + label63 + "." + label63 + "." + strings.Repeat("b", 61)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"通常の連絡先", "alice@example.com", "alice@example.com"},
		{"英字を小文字に揃える", "Alice.Smith@Example.COM", "alice.smith@example.com"},
		{"前後の空白文字を除く", "  alice@example.com\t\n", "alice@example.com"},
		{"前後の、ASCII でない空白文字を除く", "\U00003000alice@example.com\U000000A0", "alice@example.com"},
		{"ローカル部が1文字", "a@example.com", "a@example.com"},
		{"ローカル部が64文字", local64 + "@example.com", local64 + "@example.com"},
		{"ローカル部の記号", "a.!#$%&'*+/=?^_`{|}~-z@example.com", "a.!#$%&'*+/=?^_`{|}~-z@example.com"},
		{"ラベルが2つ", "a@b.c", "a@b.c"},
		{"ラベルが63文字", "a@" + label63 + ".com", "a@" + label63 + ".com"},
		{"ラベルの中のハイフンと数字", "a@ex-am-ple1.co2", "a@ex-am-ple1.co2"},
		{"254文字", max254, max254},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact, err := NewContact(tt.raw)
			if err != nil {
				t.Fatalf("NewContact がエラーを返した: %v", err)
			}
			if got := contact.String(); got != tt.want {
				t.Fatalf("連絡先が違う: got %q, want %q", got, tt.want)
			}
		})
	}
}

// RL-06: 連絡先の規則の、境界の外側。
func TestNewContact_Rejects(t *testing.T) {
	label63 := strings.Repeat("a", 63)
	local64 := strings.Repeat("a", 64)
	// 255文字: 64 + 1 + 63 + 1 + 63 + 1 + 62
	over255 := local64 + "@" + label63 + "." + label63 + "." + strings.Repeat("b", 62)

	tests := []struct {
		name string
		raw  string
	}{
		{"空の文字列", ""},
		{"空白文字だけ", "   "},
		{"@ がない", "alice.example.com"},
		{"@ が2つ", "alice@bob@example.com"},
		{"ローカル部が空", "@example.com"},
		{"ローカル部が65文字", strings.Repeat("a", 65) + "@example.com"},
		{"ローカル部に、使えない記号", "al(ice@example.com"},
		{"ローカル部に、空白", "al ice@example.com"},
		{"ローカル部に、引用符", "\"alice\"@example.com"},
		{"ドメインが空", "alice@"},
		{"ラベルが1つ", "alice@localhost"},
		{"空のラベル(. が続く)", "alice@example..com"},
		{"空のラベル(. で始まる)", "alice@.example.com"},
		{"空のラベル(. で終わる)", "alice@example.com."},
		{"ラベルが64文字", "a@" + strings.Repeat("a", 64) + ".com"},
		{"ラベルが、ハイフンで始まる", "alice@-example.com"},
		{"ラベルが、ハイフンで終わる", "alice@example-.com"},
		{"ラベルに、アンダースコア", "alice@exa_mple.com"},
		{"255文字", over255},
		{"ASCII でない文字(ローカル部)", "アリス@example.com"},
		{"ASCII でない文字(ドメイン)", "alice@例え.jp"},
		{"中の改行", "alice@example.com\r\nBcc: x@example.com"},
		{"制御文字", "alice\x00@example.com"},
		{"正しくない UTF-8 の並び", "alice\xff@example.com"},
		{"IP アドレスの形", "alice@[127.0.0.1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewContact(tt.raw); !errors.Is(err, ErrInvalidContact) {
				t.Fatalf("ErrInvalidContact を返さなかった: %v", err)
			}
		})
	}
}

func TestRestoreContact_DoesNotValidate(t *testing.T) {
	if got := RestoreContact("alice@example.com").String(); got != "alice@example.com" {
		t.Fatalf("連絡先が違う: %q", got)
	}
}
