package usecase

import (
	"context"
	"errors"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// attempt は、ユースケースの公開する操作の、1回の呼び出しである(ADR-021)。
// 操作の名前、送信元、特定できた利用者を持ち、出来事を記録する。出来事は、すべて、ここを通して記録する。
//
// 公開するメソッドは、最初に begin で作り、戻るときに、finish を defer で呼ぶ(規約)。
// finish を通さないメソッドが返したエラーは、どこにも記録されない。
type attempt struct {
	events    port.EventRecorder
	operation string
	source    string
	user      domain.UserID
}

func begin(events port.EventRecorder, operation, source string) *attempt {
	return &attempt{events: events, operation: operation, source: source}
}

// identify は、特定できた利用者を入れる。以後の記録に、添えられる。
func (a *attempt) identify(user domain.UserID) {
	a.user = user
}

// record は、出来事を記録する。送信元と、特定できた利用者を添える。
func (a *attempt) record(ctx context.Context, kind domain.EventKind) {
	a.events.Record(ctx, domain.Event{Kind: kind, Source: a.source, UserID: a.user})
}

// limitExceeded は、上限によって拒否したことを記録する。数える単位の種類を添える。
// 数える対象の値(連絡先など)は、添えない。
func (a *attempt) limitExceeded(ctx context.Context, limit domain.Limit) {
	a.events.Record(ctx, domain.Event{Kind: domain.EventLimitExceeded, Source: a.source, UserID: a.user, Limit: limit.Kind})
}

// finish は、操作の終わりの手順である。戻り値のエラーが、拒否でない場合に、
// 操作を完了できなかったことを記録する。
func (a *attempt) finish(ctx context.Context, errp *error) {
	err := *errp
	if err == nil {
		return
	}
	var rejectedErr *RejectedError
	if errors.As(err, &rejectedErr) {
		return
	}
	a.events.Record(ctx, domain.Event{
		Kind:      domain.EventOperationFailed,
		Source:    a.source,
		UserID:    a.user,
		Operation: a.operation,
		Error:     err.Error(),
	})
}
