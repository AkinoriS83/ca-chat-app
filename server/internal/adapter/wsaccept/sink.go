package wsaccept

import (
	"sync"
	"sync/atomic"
)

// queueLength は、端末ごとの、送信待ちの列の長さである。
// 超えた端末の接続は、配信の adapter が閉じる。
const queueLength = 256

// sink は、接続への書き込み口である。application/port の ConnectionSink を満たす。
//
// 接続そのものではなく、送信待ちの列である。接続を確立する前に作り、ユースケースに渡す。
// ログイン状態が有効でない要求を、接続を確立せずに拒否するためである(ADR-002)。
// このパッケージは、application/port を import しない。Go のインターフェイスは、import なしに満たせる。
type sink struct {
	queue     chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	// sessionEnded は、閉じた理由が、ログイン状態が無効になったことであるかを示す。
	// closed を閉じる前に、定める。
	sessionEnded atomic.Bool
}

func newSink() *sink {
	return &sink{
		queue:  make(chan []byte, queueLength),
		closed: make(chan struct{}),
	}
}

// Send は、1通を、送信待ちの列に加える。待たずに戻る。
// 列が一杯の場合と、閉じた後に呼ばれた場合は、加えずに、false を返す。
// 閉じてから、登録の解除までの間に、配信が Send を呼びうるためである。列そのものは、閉じない。
func (s *sink) Send(data []byte) bool {
	select {
	case <-s.closed:
		return false
	default:
	}
	select {
	case s.queue <- data:
		return true
	default:
		return false
	}
}

// Close は、閉じたことを印す。何度呼んでもよい。
func (s *sink) Close() {
	s.closeOnce.Do(func() { close(s.closed) })
}

// CloseSessionEnded は、ログイン状態が無効になったことを理由として、閉じたことを印す。何度呼んでもよい。
// 既に閉じていれば、理由は、変えない。
func (s *sink) CloseSessionEnded() {
	s.closeOnce.Do(func() {
		s.sessionEnded.Store(true)
		close(s.closed)
	})
}
