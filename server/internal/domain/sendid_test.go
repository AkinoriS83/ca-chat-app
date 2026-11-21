package domain

import (
	"errors"
	"testing"
)

func TestParseSendID_Accepts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"小文字", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70"},
		{"大文字は、小文字に揃える", "0B6F2D3E-8A51-4C0E-9F1D-2A7C4E5B6D70", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70"},
		{"UUID の版は、問わない", "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000"},
		{"すべて f", "ffffffff-ffff-ffff-ffff-ffffffffffff", "ffffffff-ffff-ffff-ffff-ffffffffffff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ParseSendID(tt.raw)
			if err != nil {
				t.Fatalf("ParseSendID がエラーを返した: %v", err)
			}
			if got := id.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSendID_Rejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"空の文字列", ""},
		{"35文字", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d7"},
		{"37文字", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d700"},
		{"ハイフンがない(32文字)", "0b6f2d3e8a514c0e9f1d2a7c4e5b6d70"},
		{"ハイフンの位置が違う", "0b6f2d3e8-a51-4c0e-9f1d-2a7c4e5b6d70"},
		{"ハイフンの位置に16進数がある", "0b6f2d3e08a51-4c0e-9f1d-2a7c4e5b6d70"},
		{"16進数でない文字", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d7g"},
		{"波括弧で囲んだ形", "{0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d}"},
		{"前後の空白", " 0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d7"},
		{"全角の数字を含む(36バイト)", "0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6\U0000FF10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseSendID(tt.raw); !errors.Is(err, ErrInvalidSendID) {
				t.Fatalf("ErrInvalidSendID を返さなかった: %v", err)
			}
		})
	}
}
