package wsaccept

import "sync"

// queueLength は、端末ごとの、送信待ちの列の長さである。
// 超えた端末の接続は、配信の adapter が閉じる。
const queueLength = 256

// sink は、接続への書き込み口である。application/port の ConnectionSink を満たす。
//
// 接続そのものではなく、送信待ちの列である。接続を確立する前に作り、ユースケースに渡す。
// 利用者を特定できない要求を、接続を確立せずに拒否するためである(ADR-002)。
// このパッケージは、application/port を import しない。Go のインターフェイスは、import なしに満たせる。
type sink struct {
	queue     chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newSink() *sink {
	return &sink{
		queue:  make(chan []byte, queueLength),
		closed: make(chan struct{}),
	}
}

// Send は、1通を、送信待ちの列に加える。待たずに戻る。
// 列が一杯の場合と、Close の後に呼ばれた場合は、加えずに、false を返す。
// Close から、登録の解除までの間に、配信が Send を呼びうるためである。列そのものは、閉じない。
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
