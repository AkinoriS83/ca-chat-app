package wsaccept

import (
	"sync"
	"testing"
)

func TestSink_QueuesInOrder(t *testing.T) {
	s := newSink()

	for _, data := range []string{"1", "2", "3"} {
		if !s.Send([]byte(data)) {
			t.Fatalf("列に加えられなかった: %s", data)
		}
	}

	for _, want := range []string{"1", "2", "3"} {
		if got := string(<-s.queue); got != want {
			t.Fatalf("加えた順に取り出せない: got %s, want %s", got, want)
		}
	}
}

func TestSink_FullQueueReturnsFalseWithoutBlocking(t *testing.T) {
	s := newSink()
	for i := 0; i < queueLength; i++ {
		if !s.Send([]byte("x")) {
			t.Fatalf("%d 通目を、列に加えられなかった", i+1)
		}
	}

	if s.Send([]byte("overflow")) {
		t.Fatal("一杯の列に、加えられた")
	}
	if len(s.queue) != queueLength {
		t.Fatalf("列の長さが違う: %d", len(s.queue))
	}
}

func TestSink_SendAfterCloseReturnsFalse(t *testing.T) {
	s := newSink()
	s.Close()
	s.Close() // 何度呼んでもよい

	if s.Send([]byte("x")) {
		t.Fatal("閉じた書き込み口に、加えられた")
	}
	if len(s.queue) != 0 {
		t.Fatal("閉じた書き込み口の列に、加わった")
	}
	select {
	case <-s.closed:
	default:
		t.Fatal("閉じたことが、印されていない")
	}
}

func TestSink_ConcurrentSendAndClose(t *testing.T) {
	s := newSink()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.Send([]byte("x"))
			}
		}()
		go func() {
			defer wg.Done()
			s.Close()
		}()
	}
	wg.Wait()
}

// 閉じた理由を、印として持つ。最初に閉じた理由が、残る。
func TestSink_CloseReason(t *testing.T) {
	t.Run("ログイン状態が無効になった", func(t *testing.T) {
		s := newSink()
		s.CloseSessionEnded()
		s.CloseSessionEnded() // 何度呼んでもよい
		s.Close()

		select {
		case <-s.closed:
		default:
			t.Fatal("閉じたことが、印されていない")
		}
		if !s.sessionEnded.Load() {
			t.Fatal("閉じた理由が、ログイン状態が無効になったことでない")
		}
		if s.Send([]byte("x")) {
			t.Fatal("閉じた書き込み口に、加えられた")
		}
	})

	t.Run("そのほかの理由で閉じた後", func(t *testing.T) {
		s := newSink()
		s.Close()
		s.CloseSessionEnded()

		if s.sessionEnded.Load() {
			t.Fatal("先に閉じた理由が、変わった")
		}
	})
}
