// Package eventlog は、出来事を、ログに出力する adapter である。port.EventRecorder を実装する(ADR-021)。
//
// 出力するのは、domain.Event が持つ項目だけである。連絡先、識別名、表示名、秘密、資格、本文は、
// domain.Event に項目がなく、出力しない(仕様「記録」)。発生時刻は、ログの出力の時刻である。
package eventlog

import (
	"context"
	"log/slog"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// Message は、出来事の記録の、ログの文言である。出来事の種類は、event の項目に出力する。
const Message = "event"

// Recorder は、出来事を、ログに出力する。
type Recorder struct {
	logger *slog.Logger
}

// NewRecorder は、Recorder を作る。
func NewRecorder(logger *slog.Logger) *Recorder {
	return &Recorder{logger: logger}
}

var _ port.EventRecorder = (*Recorder)(nil)

// Record は、出来事を出力する。値を持たない項目は、出力しない。
func (r *Recorder) Record(ctx context.Context, event domain.Event) {
	attrs := []slog.Attr{slog.String("event", event.Kind.String())}
	if event.Source != "" {
		attrs = append(attrs, slog.String("source", event.Source))
	}
	if event.UserID != "" {
		attrs = append(attrs, slog.String("user_id", string(event.UserID)))
	}
	if event.Kind == domain.EventLimitExceeded {
		attrs = append(attrs, slog.String("limit", event.Limit.String()))
	}
	if event.Kind == domain.EventOperationFailed {
		attrs = append(attrs, slog.String("operation", event.Operation), slog.String("error", event.Error))
	}
	r.logger.LogAttrs(ctx, levelOf(event.Kind), Message, attrs...)
}

// levelOf は、出来事の種類に対応する、ログの水準を返す。
func levelOf(kind domain.EventKind) slog.Level {
	switch kind {
	case domain.EventOperationFailed, domain.EventSignupNoticeFailed:
		return slog.LevelError
	case domain.EventLoginFailed, domain.EventAuthenticationRejected, domain.EventLimitExceeded:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
