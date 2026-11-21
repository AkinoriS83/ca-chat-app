// Package domain は、利用者、会話、メッセージと、その規則を置く。
// 外部のライブラリと、入出力を行う標準ライブラリを、import しない(ADR-005)。
package domain

// UserID は、利用者の識別子である。UUID の文字列(小文字)で表す。
// 識別名(Handle)とは、別の値である。保管するのは、識別子である。
type UserID string

// User は、利用者である。
type User struct {
	ID          UserID
	Handle      string // 識別名
	DisplayName string // 表示名
}
