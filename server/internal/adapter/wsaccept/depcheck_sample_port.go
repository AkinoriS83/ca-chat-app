//go:build depcheck_sample

// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import が違反として検出されることを確かめる。
// 通常のビルドと検査には、含まれない。1つのファイルには、違反する import を、1つだけ書く。
package wsaccept

// 違反: ユースケースを呼ぶ adapter から、ポートへ(ユースケースの飛ばし)
import _ "github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
