// Package fixeduser は、固定ユーザーを返す adapter である。port.UserDirectory を実装する。
//
// 仮実装(WS-P1): 利用者は、あらかじめ定めた3人に限る。登録、プロフィールの編集を持たない。
// フェーズ1で、保管先を使う実装に置き換える。
package fixeduser

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// 仮実装(WS-P1): 固定ユーザーの3人。
//
// 識別子の順は、識別名の順と、わざと変えてある。通信の形式の participants は、識別名の昇順であり、
// domain.DirectPair は、識別子の昇順である。2つの順が同じだと、participants を DirectPair の順のまま
// 返す誤りを、テストで検出できない。
var users = []domain.User{
	{ID: "00000000-0000-4000-8000-000000000003", Handle: "alice", DisplayName: "Alice"},
	{ID: "00000000-0000-4000-8000-000000000001", Handle: "bob", DisplayName: "Bob"},
	{ID: "00000000-0000-4000-8000-000000000002", Handle: "carol", DisplayName: "Carol"},
}

// Directory は、固定ユーザーを調べる。
type Directory struct{}

// NewDirectory は、Directory を作る。
func NewDirectory() *Directory {
	return &Directory{}
}

var _ port.UserDirectory = (*Directory)(nil)

// FindByHandle は、識別名が一致する固定ユーザーを返す。
func (*Directory) FindByHandle(_ context.Context, handle string) (domain.User, bool, error) {
	for _, u := range users {
		if u.Handle == handle {
			return u, true, nil
		}
	}
	return domain.User{}, false, nil
}
