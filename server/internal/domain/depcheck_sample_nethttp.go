//go:build depcheck_sample

// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import が違反として検出されることを確かめる。
// 通常のビルドと検査には、含まれない。1つのファイルには、違反する import を、1つだけ書く。
package domain

// 違反: domain から、入出力を行う標準ライブラリへ
import _ "net/http"
