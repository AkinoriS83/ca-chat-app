package system

import (
	"encoding/base64"
	"regexp"
	"testing"
	"time"
)

// RL-35: 資格は、256ビットの無作為な値を、base64url(パディングなし。43文字)で表す。呼ぶたびに、異なる。
func TestNewCredential(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		credential, err := CredentialSource{}.NewCredential()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(credential) {
			t.Fatalf("資格の形が違う(%d 文字)", len(credential))
		}
		decoded, err := base64.RawURLEncoding.DecodeString(credential)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("資格が、32バイトでない: %d, %v", len(decoded), err)
		}
		if seen[credential] {
			t.Fatal("同じ資格を、2回作った")
		}
		seen[credential] = true
	}
}

// DD-05: 時刻は、UTC で、保管先の精度(マイクロ秒)に丸める。
func TestClock_Now(t *testing.T) {
	before := time.Now().Add(-time.Microsecond)
	now := Clock{}.Now()
	after := time.Now()

	if now.Location() != time.UTC {
		t.Fatalf("UTC でない: %v", now.Location())
	}
	if now.Nanosecond()%1000 != 0 {
		t.Fatalf("マイクロ秒に、丸めていない: %d", now.Nanosecond())
	}
	if now.Before(before) || now.After(after) {
		t.Fatalf("現在の時刻でない: %v", now)
	}
}
