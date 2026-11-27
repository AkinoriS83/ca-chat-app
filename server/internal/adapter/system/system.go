// Package system は、OS が与える時刻と乱数を使う adapter である。
// port.Clock と port.CredentialSource を実装する(ADR-019)。
package system

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// Clock は、サーバーの時刻を返す。
type Clock struct{}

var _ port.Clock = Clock{}

// Now は、現在の時刻(UTC)を返す。保管先の精度に合わせて、マイクロ秒に丸める。
// 丸めないと、保管した期限と、メモリの上の期限が、マイクロ秒より下の桁で食い違う。
func (Clock) Now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

// credentialBytes は、資格の無作為さである(256ビット。ADR-014、RL-35)。
const credentialBytes = 32

// CredentialSource は、資格(ログイン状態を示す資格、確認のための資格)を作る。
type CredentialSource struct{}

var _ port.CredentialSource = CredentialSource{}

var errRandom = errors.New("system: cannot read random bytes")

// NewCredential は、暗号に使える乱数の32バイトを、パディングのない base64url(43文字)で返す。
func (CredentialSource) NewCredential() (string, error) {
	b := make([]byte, credentialBytes)
	if _, err := rand.Read(b); err != nil {
		return "", errRandom
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
