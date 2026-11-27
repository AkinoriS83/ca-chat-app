//go:build depcheck_sample

// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import が違反として検出されることを確かめる。
// 通常のビルドと検査には、含まれない。1つのファイルには、違反する import を、1つだけ書く。
package eventlog

// 違反: ポートを実装する adapter から、ユースケースへ
import _ "github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
