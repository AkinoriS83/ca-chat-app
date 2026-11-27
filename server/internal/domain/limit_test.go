package domain

import (
	"testing"
	"time"
)

// RL-28: 既定値は、仕様「上限」の表の通りである。
func TestDefaultLimitRules(t *testing.T) {
	want := LimitRules{
		SignupRequestPerSource:    LimitRule{Count: 10, Window: time.Hour},
		SignupRequestPerContact:   LimitRule{Count: 3, Window: time.Hour},
		SignupCompletionPerSource: LimitRule{Count: 20, Window: time.Hour},
		LoginFailurePerContact:    LimitRule{Count: 10, Window: 15 * time.Minute},
		LoginFailurePerSource:     LimitRule{Count: 30, Window: 15 * time.Minute},
		PeerLookupPerUser:         LimitRule{Count: 30, Window: time.Hour},
	}
	if got := DefaultLimitRules(); got != want {
		t.Fatalf("既定値が違う: %+v", got)
	}
}

// 数え方は、ログインの、連絡先ごとの上限だけが、「連続して N 回」である。
func TestLimitRules_BuildLimits(t *testing.T) {
	rules := DefaultLimitRules()
	contact := RestoreContact("alice@example.com")

	tests := []struct {
		limit   Limit
		kind    LimitKind
		subject string
		rule    LimitRule
		mode    LimitMode
	}{
		{rules.SignupRequestBySource("127.0.0.1"), LimitSignupRequestPerSource, "127.0.0.1", rules.SignupRequestPerSource, LimitWindow},
		{rules.SignupRequestByContact(contact), LimitSignupRequestPerContact, "alice@example.com", rules.SignupRequestPerContact, LimitWindow},
		{rules.SignupCompletionBySource("127.0.0.1"), LimitSignupCompletionPerSource, "127.0.0.1", rules.SignupCompletionPerSource, LimitWindow},
		{rules.LoginFailureByContact(contact), LimitLoginFailurePerContact, "alice@example.com", rules.LoginFailurePerContact, LimitLockout},
		{rules.LoginFailureBySource("127.0.0.1"), LimitLoginFailurePerSource, "127.0.0.1", rules.LoginFailurePerSource, LimitWindow},
		{rules.PeerLookupByUser("user-1"), LimitPeerLookupPerUser, "user-1", rules.PeerLookupPerUser, LimitWindow},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			want := Limit{Kind: tt.kind, Subject: tt.subject, Rule: tt.rule, Mode: tt.mode}
			if tt.limit != want {
				t.Fatalf("上限が違う: got %+v, want %+v", tt.limit, want)
			}
		})
	}
}

func TestLimitRule_Valid(t *testing.T) {
	tests := []struct {
		rule LimitRule
		want bool
	}{
		{LimitRule{Count: 1, Window: time.Nanosecond}, true},
		{LimitRule{Count: 0, Window: time.Hour}, false},
		{LimitRule{Count: -1, Window: time.Hour}, false},
		{LimitRule{Count: 1, Window: 0}, false},
		{LimitRule{Count: 1, Window: -time.Second}, false},
	}
	for _, tt := range tests {
		if got := tt.rule.Valid(); got != tt.want {
			t.Fatalf("%+v: Valid = %v, want %v", tt.rule, got, tt.want)
		}
	}
}
