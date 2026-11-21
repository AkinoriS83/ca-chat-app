package httpapi

import (
	"unicode/utf16"
	"unicode/utf8"
)

// decodeJSONString は、JSON の文字列(引用符を含む)を、置き換えずに復号する。
//
// encoding/json は、正しくない UTF-8 の並びと、対にならないサロゲートのエスケープを、
// U+FFFD に置き換えて復号する。置き換えた後では、「文字列として正しい」(仕様 WS-D3)を
// 判定できない。この関数は、正しくない並びを、正しくないまま返す。判定は、domain が行う(ADR-003)。
//
//   - エスケープを解く。対になったサロゲートのエスケープは、1つのコードポイントにする。
//   - 対にならないサロゲートのエスケープは、そのサロゲートを UTF-8 の規則でそのまま符号化した
//     3バイトにする。これは、正しくない UTF-8 の並びである。
//   - エスケープでないバイトは、正しくない UTF-8 の並びを含め、そのまま写す。
//
// raw が、JSON の文字列の形でない場合は、ok に false を返す。
func decodeJSONString(raw []byte) (decoded string, ok bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	in := raw[1 : len(raw)-1]
	out := make([]byte, 0, len(in))

	for i := 0; i < len(in); {
		c := in[i]
		if c == '"' {
			return "", false
		}
		if c != '\\' {
			out = append(out, c)
			i++
			continue
		}

		if i+1 >= len(in) {
			return "", false
		}
		switch in[i+1] {
		case '"', '\\', '/':
			out = append(out, in[i+1])
			i += 2
		case 'b':
			out = append(out, '\b')
			i += 2
		case 'f':
			out = append(out, '\f')
			i += 2
		case 'n':
			out = append(out, '\n')
			i += 2
		case 'r':
			out = append(out, '\r')
			i += 2
		case 't':
			out = append(out, '\t')
			i += 2
		case 'u':
			first, ok := readHex4(in, i+2)
			if !ok {
				return "", false
			}
			i += 6
			if !utf16.IsSurrogate(first) {
				out = utf8.AppendRune(out, first)
				continue
			}
			// 上位サロゲートの直後に、下位サロゲートのエスケープが続く場合は、1つのコードポイントにする。
			if i+1 < len(in) && in[i] == '\\' && in[i+1] == 'u' {
				if second, ok := readHex4(in, i+2); ok {
					if combined := utf16.DecodeRune(first, second); combined != utf8.RuneError {
						out = utf8.AppendRune(out, combined)
						i += 6
						continue
					}
				}
			}
			out = appendLoneSurrogate(out, first)
		default:
			return "", false
		}
	}
	return string(out), true
}

// readHex4 は、in の pos から、16進数4桁を読む。
func readHex4(in []byte, pos int) (rune, bool) {
	if pos+4 > len(in) {
		return 0, false
	}
	var r rune
	for _, c := range in[pos : pos+4] {
		switch {
		case '0' <= c && c <= '9':
			r = r<<4 | rune(c-'0')
		case 'a' <= c && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case 'A' <= c && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

// appendLoneSurrogate は、サロゲート(U+D800〜U+DFFF)を、UTF-8 の規則でそのまま符号化して加える。
// 結果は、正しくない UTF-8 の並びである。
func appendLoneSurrogate(out []byte, r rune) []byte {
	return append(out, 0xE0|byte(r>>12), 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
}
