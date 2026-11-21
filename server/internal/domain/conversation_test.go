package domain

import (
	"errors"
	"testing"
)

func TestNewDirectPair_OrdersByID(t *testing.T) {
	const (
		a UserID = "00000000-0000-4000-8000-000000000001"
		b UserID = "00000000-0000-4000-8000-000000000003"
	)

	ab, err := NewDirectPair(a, b)
	if err != nil {
		t.Fatal(err)
	}
	ba, err := NewDirectPair(b, a)
	if err != nil {
		t.Fatal(err)
	}

	if ab != ba {
		t.Fatalf("どちらが相手を選んでも、同じ値になるはず: %v, %v", ab, ba)
	}
	if ab.Low() != a || ab.High() != b {
		t.Fatalf("識別子の昇順に並んでいない: low %q, high %q", ab.Low(), ab.High())
	}
}

func TestNewDirectPair_RejectsSelf(t *testing.T) {
	const a UserID = "00000000-0000-4000-8000-000000000001"
	if _, err := NewDirectPair(a, a); !errors.Is(err, ErrSelfConversation) {
		t.Fatalf("ErrSelfConversation を返さなかった: %v", err)
	}
}
