package httpapi

import (
	"testing"
	"unicode/utf8"
)

// esc は、JSON の、コードポイントのエスケープ(バックスラッシュ、u、16進数4桁)を作る。
func esc(hex string) string {
	return string(rune(0x5C)) + "u" + hex
}

func quote(s string) []byte {
	return []byte(`"` + s + `"`)
}

func TestDecodeJSONString_Valid(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{"空の文字列", quote(""), ""},
		{"エスケープなし", quote("hello"), "hello"},
		{"複数バイトの文字を、そのまま写す", quote("こんにちは\U0001F600"), "こんにちは\U0001F600"},
		{"1文字のエスケープ", quote(`a\"b\\c\/d\b\f\n\r\t`), "a\"b\\c/d\b\f\n\r\t"},
		{"コードポイントのエスケープ", quote(esc("0041") + esc("3042")), "Aあ"},
		{"16進数の大文字と小文字", quote(esc("00e9") + esc("00E9")), "éé"},
		{"対になったサロゲートは、1つのコードポイントにする", quote(esc("d83d") + esc("de00")), "\U0001F600"},
		{"対になったサロゲート(大文字)", quote(esc("D83D") + esc("DE00")), "\U0001F600"},
		{"U+0000 のエスケープ", quote("a" + esc("0000") + "b"), "a\x00b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decodeJSONString(tt.raw)
			if !ok {
				t.Fatal("復号できなかった")
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// 正しくない並びは、U+FFFD に置き換えずに、正しくないまま返す。
func TestDecodeJSONString_KeepsInvalidSequences(t *testing.T) {
	const (
		loneHigh = "\xED\xA0\xBD" // U+D83D を、UTF-8 の規則でそのまま符号化した3バイト
		loneLow  = "\xED\xB8\x80" // U+DE00
	)
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{"対にならない上位サロゲート", quote("a" + esc("d83d") + "b"), "a" + loneHigh + "b"},
		{"対にならない下位サロゲート", quote("a" + esc("de00") + "b"), "a" + loneLow + "b"},
		{"末尾の上位サロゲート", quote(esc("d83d")), loneHigh},
		{"上位サロゲートの後に、サロゲートでないエスケープ", quote(esc("d83d") + esc("0041")), loneHigh + "A"},
		{"上位サロゲートの後に、1文字のエスケープ", quote(esc("d83d") + `\n`), loneHigh + "\n"},
		{"上位が2つ続いた後に、下位", quote(esc("d83d") + esc("d83d") + esc("de00")), loneHigh + "\U0001F600"},
		{"下位、上位の順", quote(esc("de00") + esc("d83d")), loneLow + loneHigh},
		{"エスケープでない、正しくないバイト", quote("a\xFFb"), "a\xFFb"},
		{"エスケープでない、途中で切れた並び", quote("a\xE3\x81"), "a\xE3\x81"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decodeJSONString(tt.raw)
			if !ok {
				t.Fatal("復号できなかった")
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if utf8.ValidString(got) {
				t.Fatal("正しい UTF-8 の並びになっている(domain が、拒否できない)")
			}
		})
	}
}

func TestDecodeJSONString_NotAString(t *testing.T) {
	backslash := string(rune(0x5C))
	tests := []struct {
		name string
		raw  []byte
	}{
		{"空", []byte("")},
		{"引用符が1つ", []byte(`"`)},
		{"数値", []byte("123")},
		{"null", []byte("null")},
		{"オブジェクト", []byte(`{"a":"b"}`)},
		{"閉じていない", []byte(`"abc`)},
		{"開いていない", []byte(`abc"`)},
		{"途中に、エスケープされていない引用符", []byte(`"a"b"`)},
		{"定義されていないエスケープ", quote(`\x41`)},
		{"末尾のバックスラッシュ", quote("abc" + backslash)},
		{"桁が足りないコードポイントのエスケープ", quote(backslash + "u12")},
		{"16進数でないコードポイントのエスケープ", quote(backslash + "u12G4")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := decodeJSONString(tt.raw); ok {
				t.Fatalf("復号できた: %q", got)
			}
		})
	}
}
