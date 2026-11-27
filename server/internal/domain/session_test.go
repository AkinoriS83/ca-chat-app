package domain

import (
	"testing"
	"time"
)

// RL-16: 有効かどうかは、「now が、期限より前」で判定する。期限ちょうどは、有効でない。
func TestSession_ValidAt(t *testing.T) {
	expiresAt := time.Date(2026, 12, 27, 9, 0, 0, 0, time.UTC)
	session := Session{ID: "s", UserID: "u", ExpiresAt: expiresAt}

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"期限の直前", expiresAt.Add(-time.Microsecond), true},
		{"期限ちょうど", expiresAt, false},
		{"期限の後", expiresAt.Add(time.Microsecond), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := session.ValidAt(tt.now); got != tt.want {
				t.Fatalf("ValidAt = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLifetimes(t *testing.T) {
	if SessionLifetime != 30*24*time.Hour {
		t.Fatalf("ログイン状態の有効期限が違う: %v", SessionLifetime)
	}
	if SignupLifetime != 24*time.Hour {
		t.Fatalf("確認のための資格の有効期限が違う: %v", SignupLifetime)
	}
}
