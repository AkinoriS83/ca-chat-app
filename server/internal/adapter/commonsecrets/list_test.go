package commonsecrets

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

const (
	wantCount  = 5392
	wantSHA256 = "7ea526caa6921340fccd0548617d4f7a3a33e66207ffd61472efd9684fad0919"
)

// ADR-017: 一覧は、取り出した通りである。手での書き換えを、検出する。
func TestList_FileIsAsExtracted(t *testing.T) {
	sum := sha256.Sum256([]byte(listText))
	if got := hex.EncodeToString(sum[:]); got != wantSHA256 {
		t.Fatalf("list.txt の要約が違う: %s。取り出し直した場合は、NOTICE.md と、このテストを直す", got)
	}

	lines := strings.Split(strings.TrimSuffix(listText, "\n"), "\n")
	if len(lines) != wantCount {
		t.Fatalf("件数が違う: %d", len(lines))
	}
	for i, line := range lines {
		if n := utf8.RuneCountInString(line); n < domain.MinSecretLength || n > domain.MaxSecretLength {
			t.Fatalf("%d 行目の長さが、秘密の長さの条件の外である: %d 文字", i+1, n)
		}
		if strings.Contains(line, "@") {
			t.Fatalf("%d 行目が、@ を含む", i+1)
		}
	}
	// 重複する行は、ない。
	if got := NewList().Len(); got != wantCount {
		t.Fatalf("読み込んだ件数が違う: %d", got)
	}
}

// secretOf は、ポートに渡す入力を用意する。adapter から domain の規則を呼ばない規約の、例外である。
// Secret は、中の値をパッケージの外から作れず、復元用の関数もないため、テストの入力の用意に限って、
// NewSecret を呼ぶ(DirectPair と同じ扱い)。
func secretOf(t *testing.T, raw string) domain.Secret {
	t.Helper()
	secret, err := domain.NewSecret(raw, domain.RestoreContact("someone@example.com"), "someone")
	if err != nil {
		t.Fatalf("秘密を作れない(%d 文字): %v", utf8.RuneCountInString(raw), err)
	}
	return secret
}

// RL-06: 一覧にある値は、当たる。照合は、完全な一致である(ADR-017)。
func TestList_Contains(t *testing.T) {
	list := NewList()
	lines := strings.Split(strings.TrimSuffix(listText, "\n"), "\n")
	first, last := lines[0], lines[len(lines)-1]

	for _, raw := range []string{first, last, "123456789012345", "qwertyuiopasdfghjkl"} {
		if !list.Contains(secretOf(t, raw)) {
			t.Fatalf("一覧にある値が、当たらない: %q", raw)
		}
	}
	notListed := []string{
		"correct horse battery staple",
		first + "x",            // 末尾に1文字を足したもの
		strings.ToUpper(first), // 大文字と小文字だけが異なるもの
		" " + first,            // 前に空白を足したもの
		strings.Repeat("\U0001F600", 15),
	}
	for _, raw := range notListed {
		if list.Contains(secretOf(t, raw)) {
			t.Fatalf("一覧にない値が、当たる: %q", raw)
		}
	}
}
