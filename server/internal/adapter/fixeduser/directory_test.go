package fixeduser

import (
	"context"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

func TestFindByHandle(t *testing.T) {
	directory := NewDirectory()
	ctx := context.Background()

	want := map[string]string{"alice": "Alice", "bob": "Bob", "carol": "Carol"}
	ids := map[domain.UserID]bool{}
	for handle, displayName := range want {
		user, found, err := directory.FindByHandle(ctx, handle)
		if err != nil || !found {
			t.Fatalf("%s が見つからない: found %v, err %v", handle, found, err)
		}
		if user.Handle != handle || user.DisplayName != displayName {
			t.Fatalf("利用者が違う: %+v", user)
		}
		ids[user.ID] = true
	}
	if len(ids) != 3 {
		t.Fatalf("識別子が、利用者ごとに異なっていない: %v", ids)
	}

	for _, handle := range []string{"", "mallory", "Alice", "alice ", string(users[0].ID)} {
		if _, found, err := directory.FindByHandle(ctx, handle); err != nil || found {
			t.Fatalf("固定ユーザーでない識別名(%q)が見つかった", handle)
		}
	}
}

// 識別子の順と、識別名の順が異なる組があること。
// participants(識別名の昇順)を、識別子の順のまま返す誤りを、テストで検出するための前提である。
func TestIDOrderDiffersFromHandleOrder(t *testing.T) {
	byHandle := map[string]domain.UserID{}
	for _, u := range users {
		byHandle[u.Handle] = u.ID
	}
	if byHandle["alice"] <= byHandle["bob"] || byHandle["alice"] <= byHandle["carol"] {
		t.Fatalf("alice の識別子が、bob、carol より大きくない: %v", byHandle)
	}
}
