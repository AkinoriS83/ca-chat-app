package argon2id

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// secretOf は、ポートに渡す入力を用意する。adapter から domain の規則を呼ばない規約の、例外である。
// Secret は、中の値をパッケージの外から作れず、復元用の関数もないため、テストの入力の用意に限って、
// NewSecret を呼ぶ(DirectPair と同じ扱い)。
func secretOf(t *testing.T, raw string) domain.Secret {
	t.Helper()
	secret, err := domain.NewSecret(raw, domain.RestoreContact("someone@example.com"), "someone")
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

// RL-34: 保管する値は、方式、版、パラメータ、ソルト、出力を持つ、PHC の文字列の形式である。秘密を、含まない。
func TestHash_Format(t *testing.T) {
	const raw = "correct horse battery staple"
	stored, err := NewHasher().Hash(context.Background(), secretOf(t, raw))
	if err != nil {
		t.Fatal(err)
	}

	// ソルトは16バイト(22文字)、出力は32バイト(43文字)。
	pattern := regexp.MustCompile(`^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if !pattern.MatchString(string(stored)) {
		t.Fatalf("保管する値の形が違う: %s", stored)
	}
	if strings.Contains(string(stored), raw) {
		t.Fatal("保管する値が、秘密を含む")
	}
}

// RL-34: 同じ秘密の、2つの保管する値は、異なる(ソルトを、値ごとに作る)。どちらも、照合できる。
func TestHash_SameSecretGivesDifferentValues(t *testing.T) {
	h := NewHasher()
	ctx := context.Background()
	const raw = "correct horse battery staple"

	first, err := h.Hash(ctx, secretOf(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Hash(ctx, secretOf(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("同じ秘密の、保管する値が、同じである")
	}
	for _, stored := range []domain.SecretHash{first, second} {
		if matched, err := h.Verify(ctx, raw, stored); err != nil || !matched {
			t.Fatalf("照合できない: %v, %v", matched, err)
		}
	}
}

// RL-12: 秘密の全体が、照合に働く。128文字(512バイト)の秘密と、末尾の1文字だけが異なる秘密は、一致しない。
func TestVerify_WholeSecretMatters(t *testing.T) {
	h := NewHasher()
	ctx := context.Background()
	long := strings.Repeat("\U0001F600", 127) + "a"
	stored, err := h.Hash(ctx, secretOf(t, long))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		secret string
		want   bool
	}{
		{"同じ", long, true},
		{"末尾の1文字が異なる", strings.Repeat("\U0001F600", 127) + "b", false},
		{"末尾の1文字がない", strings.Repeat("\U0001F600", 127), false},
		{"先頭の72バイトだけ", long[:72], false},
		{"空", "", false},
		{"正しくない UTF-8 の並び", "\xff\xfe", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := h.Verify(ctx, tt.secret, stored)
			if err != nil {
				t.Fatal(err)
			}
			if matched != tt.want {
				t.Fatalf("matched = %v, want %v", matched, tt.want)
			}
		})
	}
}

// 保管した値が持つパラメータで、照合する(ADR-015)。パラメータを変える前に保管した値も、照合できる。
func TestVerify_UsesStoredParameters(t *testing.T) {
	h := NewHasher()
	ctx := context.Background()
	old := params{memoryKiB: 8 * 1024, iterations: 1, parallelism: 2}
	salt := []byte("0123456789abcdef")
	key, err := h.compute(ctx, []byte("an older secret value"), salt, old, 24)
	if err != nil {
		t.Fatal(err)
	}
	stored := domain.SecretHash(encode(old, salt, key))

	if matched, err := h.Verify(ctx, "an older secret value", stored); err != nil || !matched {
		t.Fatalf("照合できない: %v, %v", matched, err)
	}
	if matched, _ := h.Verify(ctx, "another secret value", stored); matched {
		t.Fatal("違う秘密が、一致した")
	}
}

// 読めない形の保管した値は、エラーにする。エラーは、秘密と、保管した値を、含まない(RL-33)。
func TestVerify_MalformedStoredHash(t *testing.T) {
	h := NewHasher()
	const secret = "marker-secret-value"
	valid, err := h.Hash(context.Background(), secretOf(t, "correct horse battery staple"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(valid), "$")

	malformed := []string{
		"",
		"plain-text-secret",
		"$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz012345",
		"$argon2i$v=19$m=19456,t=2,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=16$m=19456,t=2,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=19456,t=2$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=0,t=2,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=19456,t=0,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=19456,t=2,p=0$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=4294967295,t=2,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=19456,t=1000,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=4,t=2,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=19456,t=2,p=1$$" + parts[5],
		"$argon2id$v=19$m=19456,t=2,p=1$" + parts[4] + "$",
		"$argon2id$v=19$m=19456,t=2,p=1$!!!$" + parts[5],
		"$argon2id$v=19$m=19456,t=2,p=1$" + parts[4] + "$" + parts[5] + "$extra",
	}
	for _, stored := range malformed {
		matched, err := h.Verify(context.Background(), secret, domain.SecretHash(stored))
		if !errors.Is(err, ErrMalformedHash) || matched {
			t.Fatalf("%q: ErrMalformedHash を返さなかった: %v, %v", stored, matched, err)
		}
		if strings.Contains(err.Error(), secret) || (stored != "" && strings.Contains(err.Error(), stored)) {
			t.Fatalf("エラーが、秘密か、保管した値を含む: %v", err)
		}
	}
}

// DD-11: 同時に行う計算は、MaxConcurrent までである。超える分は、順番を待ち、待つ間に取り消されたら、やめる。
func TestCompute_LimitsConcurrency(t *testing.T) {
	h := NewHasher()
	// 枠を、すべて埋める。
	for i := 0; i < MaxConcurrent; i++ {
		h.slots <- struct{}{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := h.VerifyDummy(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("待つ間に取り消された計算が、やめなかった: %v", err)
	}
	if _, err := h.Hash(ctx, secretOf(t, "correct horse battery staple")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("待つ間に取り消された計算が、やめなかった: %v", err)
	}
	if _, err := h.Verify(ctx, "x", domain.SecretHash(encode(currentParams, dummySalt, make([]byte, keyLength)))); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("待つ間に取り消された計算が、やめなかった: %v", err)
	}

	// 枠が空くと、待っていた計算が進む。枠は、計算の後に、返される。
	for i := 0; i < MaxConcurrent; i++ {
		<-h.slots
	}
	var wg sync.WaitGroup
	for i := 0; i < MaxConcurrent*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.VerifyDummy(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(h.slots) != 0 {
		t.Fatalf("枠が、返されていない: %d", len(h.slots))
	}
}
