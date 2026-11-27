// Package ratelimit は、上限の計数を、サーバーのメモリに持つ adapter である。
// port.AttemptLimiter を実装する(ADR-016)。
//
// 計数は、サーバーごとであり、再起動で失われる。1つのサーバーの中だけで保持する状態であり、
// このパッケージだけが持つ(PERF-3、RL-39)。複数サーバー構成では、共有の保管先を使う実装に差し替える。
// 時刻は、引数で受ける。自分では、読まない。
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// sweepInterval は、使われなくなった計数を、メモリから除く間隔である。
const sweepInterval = time.Minute

// key は、数える単位である(種類と、数える対象の組)。
type key struct {
	kind    domain.LimitKind
	subject string
}

// counter は、1つの数える単位の状態である。
type counter struct {
	window      time.Duration // 数える時間。掃くときに使う
	counted     []time.Time   // 数えた時刻。古い順
	lockedUntil time.Time     // 拒否を解く時刻(「連続して N 回」の数え方だけが使う)
}

// Limiter は、上限を数える。
type Limiter struct {
	mu        sync.Mutex
	counters  map[key]*counter
	lastSweep time.Time
}

// NewLimiter は、Limiter を作る。
func NewLimiter() *Limiter {
	return &Limiter{counters: map[key]*counter{}}
}

var _ port.AttemptLimiter = (*Limiter)(nil)

// Admit は、limits のすべてが、上限に達していないことを確かめ、達していなければ、それぞれに、1回と数える。
// 達しているものがあれば、どれも数えずに、最初に当たったものを返す。
// 確かめることと数えることを、1つの排他の制御のもとで行うため、同時の要求が、上限を超えて受け付けられない。
func (l *Limiter) Admit(_ context.Context, now time.Time, limits ...domain.Limit) (domain.Limit, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweepLocked(now)

	for _, limit := range limits {
		c, found := l.counters[keyOf(limit)]
		if !found {
			continue
		}
		c.prune(now)
		if c.reached(now, limit) {
			return limit, false, nil
		}
	}

	for _, limit := range limits {
		k := keyOf(limit)
		c, found := l.counters[k]
		if !found {
			c = &counter{}
			l.counters[k] = c
		}
		c.window = limit.Rule.Window
		c.counted = append(c.counted, now)
		// 「連続して N 回」では、N 回に達した時点から、T の間、拒否する。拒否の間が過ぎると、0から数える。
		if limit.Mode == domain.LimitLockout && len(c.counted) >= limit.Rule.Count {
			c.lockedUntil = now.Add(limit.Rule.Window)
			c.counted = nil
		}
	}
	return domain.Limit{}, true, nil
}

// Forgive は、Admit が数えた1回(最も新しい1つ)を、戻す。
func (l *Limiter) Forgive(_ context.Context, limit domain.Limit) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if c, found := l.counters[keyOf(limit)]; found && len(c.counted) > 0 {
		c.counted = c.counted[:len(c.counted)-1]
	}
	return nil
}

// Reset は、計数を、0に戻す。拒否の間も、解く。
func (l *Limiter) Reset(_ context.Context, limit domain.Limit) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.counters, keyOf(limit))
	return nil
}

func keyOf(limit domain.Limit) key {
	return key{kind: limit.Kind, subject: limit.Subject}
}

// prune は、now から、数える時間より前の時刻を捨てる。数えた時刻から、ちょうど数える時間が過ぎたものも、捨てる。
func (c *counter) prune(now time.Time) {
	cutoff := now.Add(-c.window)
	kept := 0
	for kept < len(c.counted) && !c.counted[kept].After(cutoff) {
		kept++
	}
	c.counted = c.counted[kept:]
}

// reached は、上限に達しているかを返す。prune の後に呼ぶ。
func (c *counter) reached(now time.Time, limit domain.Limit) bool {
	if limit.Mode == domain.LimitLockout {
		return now.Before(c.lockedUntil)
	}
	return len(c.counted) >= limit.Rule.Count
}

// sweepLocked は、前に掃いてから、sweepInterval が過ぎていれば、すべての数える単位を見て、
// 数えた時刻が残っておらず、拒否の間も過ぎたものを除く。別の goroutine は、動かさない。
func (l *Limiter) sweepLocked(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	for k, c := range l.counters {
		c.prune(now)
		if len(c.counted) == 0 && !now.Before(c.lockedUntil) {
			delete(l.counters, k)
		}
	}
}
