//go:build depcheck_sample

// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import が違反として検出されることを確かめる。
// 通常のビルドと検査には、含まれない。1つのファイルには、違反する import を、1つだけ書く。
package usecase

// 違反: application から、乱数の標準ライブラリへ(資格の生成は、adapter が受け持つ。ADR-019)
import _ "crypto/rand"
