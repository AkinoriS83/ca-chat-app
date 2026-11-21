package domain

import (
	"errors"
	"strings"
)

// ErrInvalidSendID は、送信の識別子の形式が、正しくないことを示す。
var ErrInvalidSendID = errors.New("domain: invalid send identifier")

// SendID は、送信の識別子である(ADR-009)。UUID の文字列を、小文字に揃えて保持する。
// 中の値は、パッケージの外から直接には作れない。ParseSendID か RestoreSendID で作る。
type SendID struct {
	value string
}

// ParseSendID は、入力から、送信の識別子を作る。
// 36文字で、ハイフンが 8-4-4-4-12 の位置にあり、ほかが16進数であることだけを確かめる。
// UUID の版は、問わない。形式が正しくない場合は、ErrInvalidSendID を返す。
func ParseSendID(raw string) (SendID, error) {
	if len(raw) != 36 {
		return SendID{}, ErrInvalidSendID
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return SendID{}, ErrInvalidSendID
			}
		default:
			if !isHexDigit(c) {
				return SendID{}, ErrInvalidSendID
			}
		}
	}
	return SendID{value: strings.ToLower(raw)}, nil
}

func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

// RestoreSendID は、保管した値(小文字の36文字の文字列)から、送信の識別子を組み立てる。
// 形式は、確かめない。呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする(規約)。
func RestoreSendID(stored string) SendID {
	return SendID{value: stored}
}

// String は、送信の識別子の文字列(小文字の36文字)を返す。
func (s SendID) String() string { return s.value }
