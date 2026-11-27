package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

var (
	ctx   = context.Background()
	start = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
)

func windowLimit(subject string, count int, window time.Duration) domain.Limit {
	return domain.Limit{Kind: domain.LimitSignupRequestPerSource, Subject: subject, Rule: domain.LimitRule{Count: count, Window: window}, Mode: domain.LimitWindow}
}

func lockoutLimit(subject string, count int, window time.Duration) domain.Limit {
	return domain.Limit{Kind: domain.LimitLoginFailurePerContact, Subject: subject, Rule: domain.LimitRule{Count: count, Window: window}, Mode: domain.LimitLockout}
}

func requireAdmit(t *testing.T, l *Limiter, now time.Time, want bool, limits ...domain.Limit) domain.Limit {
	t.Helper()
	exceeded, admitted, err := l.Admit(ctx, now, limits...)
	if err != nil {
		t.Fatal(err)
	}
	if admitted != want {
		t.Fatalf("admitted = %v, want %v(%s)", admitted, want, now.Sub(start))
	}
	return exceeded
}

// RL-28:「T に N 回」。N 回までは受け付け、達した後は拒否する。拒否した要求は、数えない。
// 数えた時刻から T が過ぎると、再び受け付ける。
func TestWindow_Boundaries(t *testing.T) {
	l := NewLimiter()
	limit := windowLimit("192.0.2.1", 3, time.Hour)

	requireAdmit(t, l, start, true, limit)
	requireAdmit(t, l, start.Add(10*time.Minute), true, limit)
	requireAdmit(t, l, start.Add(20*time.Minute), true, limit)

	if exceeded := requireAdmit(t, l, start.Add(30*time.Minute), false, limit); exceeded != limit {
		t.Fatalf("当たった上限が違う: %+v", exceeded)
	}
	// 最初の1回から、T の直前までは、拒否する。拒否した要求を数えていれば、ここより後も、拒否が続く。
	requireAdmit(t, l, start.Add(time.Hour-time.Microsecond), false, limit)
	// 最初の1回から、ちょうど T で、1回分が空く。
	requireAdmit(t, l, start.Add(time.Hour), true, limit)
	requireAdmit(t, l, start.Add(time.Hour), false, limit)
	// 2回目から T で、もう1回分が空く。
	requireAdmit(t, l, start.Add(time.Hour+10*time.Minute), true, limit)
}

// 数える単位(種類と、数える対象)ごとに、別に数える。
func TestWindow_CountsPerKindAndSubject(t *testing.T) {
	l := NewLimiter()
	a := windowLimit("192.0.2.1", 1, time.Hour)
	b := windowLimit("192.0.2.2", 1, time.Hour)
	otherKind := a
	otherKind.Kind = domain.LimitSignupCompletionPerSource

	requireAdmit(t, l, start, true, a)
	requireAdmit(t, l, start, false, a)
	requireAdmit(t, l, start, true, b)
	requireAdmit(t, l, start, true, otherKind)
}

// 複数の上限を渡した場合、どれかが達していれば、どれも数えずに、最初に当たったものを返す。
func TestAdmit_AllOrNothing(t *testing.T) {
	l := NewLimiter()
	bySource := windowLimit("192.0.2.1", 5, time.Hour)
	byContact := domain.Limit{Kind: domain.LimitSignupRequestPerContact, Subject: "a@example.com", Rule: domain.LimitRule{Count: 1, Window: time.Hour}, Mode: domain.LimitWindow}

	requireAdmit(t, l, start, true, bySource, byContact)
	for i := 0; i < 10; i++ {
		if exceeded := requireAdmit(t, l, start, false, bySource, byContact); exceeded != byContact {
			t.Fatalf("当たった上限が違う: %+v", exceeded)
		}
	}
	// 送信元ごとの計数は、1回のままである。あと4回、受け付ける。
	for i := 0; i < 4; i++ {
		requireAdmit(t, l, start, true, bySource)
	}
	requireAdmit(t, l, start, false, bySource)
}

// RL-28、RL-29:「連続して N 回で、T の間、拒否する」。N 回目までは受け付け、N 回目から T の間、拒否する。
// 拒否の間が過ぎると、0から数える。
func TestLockout_Boundaries(t *testing.T) {
	l := NewLimiter()
	limit := lockoutLimit("alice@example.com", 3, 15*time.Minute)

	requireAdmit(t, l, start, true, limit)
	requireAdmit(t, l, start.Add(time.Minute), true, limit)
	last := start.Add(2 * time.Minute)
	requireAdmit(t, l, last, true, limit) // 3回目。ここから、拒否の間が始まる

	requireAdmit(t, l, last, false, limit)
	requireAdmit(t, l, last.Add(15*time.Minute-time.Microsecond), false, limit)

	// 最後に数えた時点から、ちょうど T で、解ける。0から数えるため、N 回目まで、受け付ける。
	unlocked := last.Add(15 * time.Minute)
	requireAdmit(t, l, unlocked, true, limit)
	requireAdmit(t, l, unlocked, true, limit)
	requireAdmit(t, l, unlocked, true, limit)
	requireAdmit(t, l, unlocked, false, limit)
}

// RL-29: 数える期間より前の失敗は、数えない。
func TestLockout_OldFailuresAreNotCounted(t *testing.T) {
	l := NewLimiter()
	limit := lockoutLimit("alice@example.com", 3, 15*time.Minute)

	requireAdmit(t, l, start, true, limit)
	requireAdmit(t, l, start.Add(time.Minute), true, limit)
	// 最初の2回から、15分が過ぎた後の失敗は、1回目として数える。
	later := start.Add(16 * time.Minute)
	requireAdmit(t, l, later, true, limit)
	requireAdmit(t, l, later, true, limit)
	requireAdmit(t, l, later, true, limit)
	requireAdmit(t, l, later, false, limit)
}

// DD-24: Forgive は、数えた1回を戻し、Reset は、0に戻す(拒否の間も、解く)。
func TestForgiveAndReset(t *testing.T) {
	l := NewLimiter()
	bySource := windowLimit("192.0.2.1", 2, time.Hour)
	byContact := lockoutLimit("alice@example.com", 2, time.Hour)

	// 成功したログインを、何度繰り返しても、上限に達しない。
	for i := 0; i < 10; i++ {
		requireAdmit(t, l, start, true, bySource, byContact)
		if err := l.Forgive(ctx, bySource); err != nil {
			t.Fatal(err)
		}
		if err := l.Reset(ctx, byContact); err != nil {
			t.Fatal(err)
		}
	}

	// 失敗は、残る。
	requireAdmit(t, l, start, true, bySource, byContact)
	requireAdmit(t, l, start, true, bySource, byContact)
	requireAdmit(t, l, start, false, bySource)
	requireAdmit(t, l, start, false, byContact)

	// Reset は、拒否の間も、解く。
	if err := l.Reset(ctx, byContact); err != nil {
		t.Fatal(err)
	}
	requireAdmit(t, l, start, true, byContact)

	// 数えていない単位への Forgive と Reset は、何もしない。
	unknown := windowLimit("192.0.2.99", 1, time.Hour)
	if err := l.Forgive(ctx, unknown); err != nil {
		t.Fatal(err)
	}
	if err := l.Reset(ctx, unknown); err != nil {
		t.Fatal(err)
	}
}

// ADR-016: 確かめることと数えることは、1つの操作である。同時の要求が、上限を超えて受け付けられない。
func TestAdmit_ConcurrentRequestsDoNotExceedLimit(t *testing.T) {
	l := NewLimiter()
	limit := windowLimit("192.0.2.1", 10, time.Hour)

	var wg sync.WaitGroup
	var mu sync.Mutex
	admittedCount := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, admitted, _ := l.Admit(ctx, start, limit)
			if admitted {
				mu.Lock()
				admittedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if admittedCount != 10 {
		t.Fatalf("受け付けた数が違う: %d", admittedCount)
	}
}

// DD-07: 数える時間を過ぎた計数は、メモリから消す。拒否の間の計数は、残す。
func TestSweep_RemovesIdleCounters(t *testing.T) {
	l := NewLimiter()
	for _, subject := range []string{"a", "b", "c"} {
		requireAdmit(t, l, start, true, windowLimit(subject, 5, 10*time.Minute))
	}
	locked := lockoutLimit("alice@example.com", 1, time.Hour)
	requireAdmit(t, l, start, true, locked)
	if len(l.counters) != 4 {
		t.Fatalf("計数の数が違う: %d", len(l.counters))
	}

	// 10分が過ぎた後の Admit が、使われなくなった3つを除く。拒否の間の1つと、今回の1つが残る。
	requireAdmit(t, l, start.Add(10*time.Minute), true, windowLimit("d", 5, 10*time.Minute))
	if len(l.counters) != 2 {
		t.Fatalf("計数の数が違う: %d", len(l.counters))
	}
	requireAdmit(t, l, start.Add(10*time.Minute), false, locked)

	// 拒否の間が過ぎると、それも除く。
	requireAdmit(t, l, start.Add(time.Hour), true, windowLimit("e", 5, 10*time.Minute))
	if len(l.counters) != 1 {
		t.Fatalf("計数の数が違う: %d", len(l.counters))
	}
}

// 掃くのは、前に掃いてから、1分が過ぎた後である。
func TestSweep_Interval(t *testing.T) {
	l := NewLimiter()
	requireAdmit(t, l, start, true, windowLimit("a", 5, time.Second))

	requireAdmit(t, l, start.Add(30*time.Second), true, windowLimit("b", 5, time.Hour))
	if len(l.counters) != 2 {
		t.Fatalf("1分が過ぎる前に、掃いた: %d", len(l.counters))
	}
	requireAdmit(t, l, start.Add(time.Minute), true, windowLimit("b", 5, time.Hour))
	if len(l.counters) != 1 {
		t.Fatalf("1分が過ぎた後に、掃いていない: %d", len(l.counters))
	}
}
