package schedule

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// DD-06: 片付けを、起動の直後と、以後、間隔ごとに呼ぶ。エラーが返っても、続ける。終了の合図で、止まる。
func TestRun(t *testing.T) {
	var calls atomic.Int32
	third := make(chan struct{})
	runner := &Runner{
		interval: 5 * time.Millisecond,
		sweep: func(context.Context) error {
			if calls.Add(1) == 3 {
				close(third)
			}
			return errors.New("boom")
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()

	select {
	case <-third:
	case <-time.After(5 * time.Second):
		t.Fatalf("片付けが、繰り返し呼ばれない: %d 回", calls.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("終了の合図で、止まらない")
	}
}

// 起動の直後に、間隔を待たずに、1回呼ぶ。
func TestRun_SweepsImmediately(t *testing.T) {
	first := make(chan struct{}, 1)
	runner := &Runner{
		interval: time.Hour,
		sweep: func(context.Context) error {
			first <- struct{}{}
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	select {
	case <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("起動の直後に、呼ばれない")
	}
}

func TestInterval(t *testing.T) {
	if Interval != time.Minute {
		t.Fatalf("片付けの間隔が違う: %v", Interval)
	}
}
