// Package schedule は、片付けのユースケースを、一定の間隔で呼ぶ adapter である。ユースケースを呼ぶ(ADR-005)。
// 相手は、時間の経過である。
package schedule

import (
	"context"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
)

// Interval は、片付けの間隔である。設定には、しない。
const Interval = time.Minute

// Runner は、片付けを、起動の直後と、以後、一定の間隔で呼ぶ。
type Runner struct {
	sweep    func(ctx context.Context) error
	interval time.Duration
}

// NewRunner は、Runner を作る。受け取るのは、ユースケースだけである。
func NewRunner(housekeeper *usecase.Housekeeper) *Runner {
	return &Runner{sweep: housekeeper.Sweep, interval: Interval}
}

// Run は、片付けを、すぐに1回呼び、以後、間隔ごとに呼ぶ。ctx が取り消されると、戻る。
// 片付けが返したエラーは、ユースケースが記録する(ADR-021)。ここでは、次の回を待つ。
func (r *Runner) Run(ctx context.Context) {
	_ = r.sweep(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.sweep(ctx)
		}
	}
}
