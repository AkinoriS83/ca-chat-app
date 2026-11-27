package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

func record(t *testing.T, event domain.Event) map[string]any {
	t.Helper()
	var out bytes.Buffer
	NewRecorder(slog.New(slog.NewJSONHandler(&out, nil))).Record(context.Background(), event)

	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		t.Fatalf("ログの行を、読めない: %v(%s)", err, out.String())
	}
	if _, ok := line["time"]; !ok {
		t.Fatal("発生時刻が、ない")
	}
	delete(line, "time")
	return line
}

// RL-32: 出来事は、種類、送信元、利用者の識別子(特定できた場合)とともに、出力する。
// 上限の超過には、数える単位の種類を、エラーには、操作の名前とエラーを、添える。値を持たない項目は、出力しない。
func TestRecord(t *testing.T) {
	tests := []struct {
		name  string
		event domain.Event
		want  map[string]any
	}{
		{
			"登録の申し込み",
			domain.Event{Kind: domain.EventSignupRequested, Source: "192.0.2.1"},
			map[string]any{"level": "INFO", "msg": "event", "event": "signup_requested", "source": "192.0.2.1"},
		},
		{
			"ログインの成功",
			domain.Event{Kind: domain.EventLoginSucceeded, Source: "192.0.2.1", UserID: "user-1"},
			map[string]any{"level": "INFO", "msg": "event", "event": "login_succeeded", "source": "192.0.2.1", "user_id": "user-1"},
		},
		{
			"ログインの失敗",
			domain.Event{Kind: domain.EventLoginFailed, Source: "192.0.2.1"},
			map[string]any{"level": "WARN", "msg": "event", "event": "login_failed", "source": "192.0.2.1"},
		},
		{
			"認証の拒否",
			domain.Event{Kind: domain.EventAuthenticationRejected, Source: "192.0.2.1"},
			map[string]any{"level": "WARN", "msg": "event", "event": "authentication_rejected", "source": "192.0.2.1"},
		},
		{
			"上限の超過",
			domain.Event{Kind: domain.EventLimitExceeded, Source: "192.0.2.1", UserID: "user-1", Limit: domain.LimitPeerLookupPerUser},
			map[string]any{"level": "WARN", "msg": "event", "event": "limit_exceeded", "source": "192.0.2.1", "user_id": "user-1", "limit": "peer_lookup_per_user"},
		},
		{
			"案内を送れなかった",
			domain.Event{Kind: domain.EventSignupNoticeFailed, Source: "192.0.2.1"},
			map[string]any{"level": "ERROR", "msg": "event", "event": "signup_notice_failed", "source": "192.0.2.1"},
		},
		{
			"エラー",
			domain.Event{Kind: domain.EventOperationFailed, Source: "192.0.2.1", UserID: "user-1", Operation: "send message", Error: "postgres: append message failed"},
			map[string]any{"level": "ERROR", "msg": "event", "event": "operation_failed", "source": "192.0.2.1", "user_id": "user-1", "operation": "send message", "error": "postgres: append message failed"},
		},
		{
			"片付けのエラー(送信元がない)",
			domain.Event{Kind: domain.EventOperationFailed, Operation: "sweep", Error: "boom"},
			map[string]any{"level": "ERROR", "msg": "event", "event": "operation_failed", "operation": "sweep", "error": "boom"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := record(t, tt.event); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("出力が違う:\n got %v\nwant %v", got, tt.want)
			}
		})
	}
}

// 出来事の種類は、すべて、名前を持つ。
func TestRecord_AllKindsHaveNames(t *testing.T) {
	for kind := domain.EventSignupRequested; kind <= domain.EventOperationFailed; kind++ {
		if got := record(t, domain.Event{Kind: kind})["event"]; got == "unknown" || got == "" {
			t.Fatalf("種類 %d に、名前がない", kind)
		}
	}
}
