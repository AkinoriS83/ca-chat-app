// Package argon2id は、認証に用いる秘密を、保管する形に変え、照合する adapter である。
// port.SecretHasher を実装する(ADR-015)。golang.org/x/crypto を import できるのは、このパッケージだけである。
//
// 計算に渡すのは、秘密の、UTF-8 のバイト列そのものである。切り詰めず、正規化せず、別の要約にも通さない。
// 返すエラーには、秘密と、保管した値を、含めない。
package argon2id

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// パラメータである(ADR-015)。OWASP が、最小の構成として挙げる値である。
const (
	memoryKiB   = 19 * 1024 // 19 MiB
	iterations  = 2
	parallelism = 1
	saltLength  = 16
	keyLength   = 32
)

// MaxConcurrent は、同時に行う計算の数の上限である。1回の計算が、19 MiB を使うため、
// 試行を重ねる要求が使うメモリを、約76 MiB に限る。超える分は、順番を待つ。
const MaxConcurrent = 4

var (
	// ErrMalformedHash は、保管した値が、読めない形であることを示す。
	ErrMalformedHash = errors.New("argon2id: malformed stored hash")
	errRandom        = errors.New("argon2id: cannot read random bytes")
)

// Hasher は、Argon2id で、秘密を計算する。
type Hasher struct {
	slots chan struct{}
}

// NewHasher は、Hasher を作る。
func NewHasher() *Hasher {
	return &Hasher{slots: make(chan struct{}, MaxConcurrent)}
}

var _ port.SecretHasher = (*Hasher)(nil)

// params は、計算のパラメータである。保管する値が、持つ。
type params struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
}

var currentParams = params{memoryKiB: memoryKiB, iterations: iterations, parallelism: parallelism}

// Hash は、秘密を、保管する形に変える。ソルトは、呼び出しのたびに、新しく作る。
func (h *Hasher) Hash(ctx context.Context, secret domain.Secret) (domain.SecretHash, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", errRandom
	}
	key, err := h.compute(ctx, []byte(secret.Expose()), salt, currentParams, keyLength)
	if err != nil {
		return "", err
	}
	return domain.SecretHash(encode(currentParams, salt, key)), nil
}

// Verify は、入力された秘密を、保管した値が持つパラメータで計算し、所要時間が内容に依らない比較で比べる。
func (h *Hasher) Verify(ctx context.Context, secret string, stored domain.SecretHash) (bool, error) {
	p, salt, want, err := decode(string(stored))
	if err != nil {
		return false, err
	}
	got, err := h.compute(ctx, []byte(secret), salt, p, uint32(len(want)))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummy は、照合する相手がないログインで、計算に渡す、固定の入力である。
var (
	dummySecret = []byte("dummy secret for timing equalization")
	dummySalt   = make([]byte, saltLength)
)

// VerifyDummy は、固定の入力に対して、現在のパラメータの計算を、1回行う(RL-13)。
func (h *Hasher) VerifyDummy(ctx context.Context) error {
	_, err := h.compute(ctx, dummySecret, dummySalt, currentParams, keyLength)
	return err
}

// compute は、順番を待ってから、計算を行う。待つ間に、ctx が取り消されたら、やめる。
func (h *Hasher) compute(ctx context.Context, secret, salt []byte, p params, length uint32) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-h.slots }()

	return argon2.IDKey(secret, salt, p.iterations, p.memoryKiB, p.parallelism, length), nil
}

// 保管する値は、PHC の文字列の形式である。
//
//	$argon2id$v=19$m=19456,t=2,p=1$<ソルト>$<出力>
//
// ソルトと出力は、パディングのない、標準の Base64 で表す。
const prefix = "$argon2id$v=19$"

func encode(p params, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return prefix +
		"m=" + strconv.FormatUint(uint64(p.memoryKiB), 10) +
		",t=" + strconv.FormatUint(uint64(p.iterations), 10) +
		",p=" + strconv.FormatUint(uint64(p.parallelism), 10) +
		"$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key)
}

// 読める値の範囲である。保管した値が壊れていた場合に、際限のないメモリと時間を使わないために限る。
const (
	maxMemoryKiB  = 1024 * 1024 // 1 GiB
	maxIterations = 16
	maxKeyLength  = 128
)

func decode(stored string) (p params, salt, key []byte, err error) {
	rest, found := strings.CutPrefix(stored, prefix)
	if !found {
		return params{}, nil, nil, ErrMalformedHash
	}
	parts := strings.Split(rest, "$")
	if len(parts) != 3 {
		return params{}, nil, nil, ErrMalformedHash
	}

	fields := strings.Split(parts[0], ",")
	if len(fields) != 3 {
		return params{}, nil, nil, ErrMalformedHash
	}
	memory, ok1 := parseField(fields[0], "m=", maxMemoryKiB)
	iter, ok2 := parseField(fields[1], "t=", maxIterations)
	par, ok3 := parseField(fields[2], "p=", 255)
	if !ok1 || !ok2 || !ok3 {
		return params{}, nil, nil, ErrMalformedHash
	}
	// Argon2 は、並列度の8倍以上のメモリを要する。
	if memory < 8*par {
		return params{}, nil, nil, ErrMalformedHash
	}

	b64 := base64.RawStdEncoding
	salt, err = b64.DecodeString(parts[1])
	if err != nil || len(salt) == 0 {
		return params{}, nil, nil, ErrMalformedHash
	}
	key, err = b64.DecodeString(parts[2])
	if err != nil || len(key) == 0 || len(key) > maxKeyLength {
		return params{}, nil, nil, ErrMalformedHash
	}
	return params{memoryKiB: uint32(memory), iterations: uint32(iter), parallelism: uint8(par)}, salt, key, nil
}

// parseField は、「名前=値」の値を、1以上、max 以下の整数として読む。
func parseField(field, name string, max uint64) (uint64, bool) {
	text, found := strings.CutPrefix(field, name)
	if !found {
		return 0, false
	}
	value, err := strconv.ParseUint(text, 10, 32)
	if err != nil || value < 1 || value > max {
		return 0, false
	}
	return value, true
}
