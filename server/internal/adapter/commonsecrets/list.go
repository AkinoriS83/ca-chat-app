// Package commonsecrets は、よく使われる秘密の一覧である。port.CommonSecretList を実装する(ADR-017)。
//
// 一覧(list.txt)は、ビルドの時点で、実行ファイルに含め、作るときに、メモリに読み込む。
// 実行時に、外部から取得しない。出所とライセンスは、同じ置き場所の NOTICE.md にある。
// list.txt は、手で書き換えない。取り出し直すには、make common-secrets を使う。
package commonsecrets

import (
	_ "embed"
	"strings"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

//go:embed list.txt
var listText string

// List は、よく使われる秘密の集合である。作った後は、変えない。
type List struct {
	secrets map[string]struct{}
}

var _ port.CommonSecretList = (*List)(nil)

// NewList は、実行ファイルに含めた一覧を読み込んで、List を作る。
func NewList() *List {
	lines := strings.Split(strings.TrimSuffix(listText, "\n"), "\n")
	secrets := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		secrets[line] = struct{}{}
	}
	return &List{secrets: secrets}
}

// Contains は、秘密が、一覧にあるかを返す。照合は、バイト列の完全な一致で行い、正規化しない(RL-D3)。
func (l *List) Contains(secret domain.Secret) bool {
	_, found := l.secrets[secret.Expose()]
	return found
}

// Len は、一覧の件数を返す。
func (l *List) Len() int { return len(l.secrets) }
