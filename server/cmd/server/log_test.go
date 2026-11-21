package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
)

// WS-33: メッセージの本文が、サーバーが出力するログに含まれない。
// 送信が完了した場合、拒否された場合、エラーとなった場合のいずれでも同じである。
func TestLogs_DoNotContainMessageBody(t *testing.T) {
	const marker = "BODY-MARKER-7f3a9c"

	t.Run("完了した場合と、拒否された場合", func(t *testing.T) {
		s := startServer(t)
		bob := s.connect("bob")

		bob.requireNext(s.mustSend("alice", "bob", "completed "+marker))
		s.mustHistory("bob", "alice")
		requireError(t, s.send("alice", "bob", newSendID(t), marker+strings.Repeat("a", 4000)), http.StatusBadRequest, "invalid_body")
		requireError(t, s.send("alice", "bob", newSendID(t), marker+"\x00"), http.StatusBadRequest, "invalid_body")
		requireError(t, s.send("alice", "bob", "bad-id-"+marker, marker), http.StatusBadRequest, "invalid_input")
		requireError(t, s.send("alice", "dave", newSendID(t), marker), http.StatusNotFound, "not_found")
		requireError(t, s.send("mallory", "bob", newSendID(t), marker), http.StatusUnauthorized, "unauthenticated")
		requireError(t, s.sendRaw("alice", "bob", []byte(`{"sendId":"x","body":"`+marker)), http.StatusBadRequest, "invalid_input")

		if logs := s.logs.String(); strings.Contains(logs, marker) {
			t.Fatalf("ログに、本文が含まれる:\n%s", logs)
		}
	})

	t.Run("保管先がエラーを返した場合", func(t *testing.T) {
		s := startServer(t)
		// 違反したときの詳細(DETAIL)に、行の内容が現れる制約を、このテストのデータベースにだけ加える。
		if _, err := s.pool.Exec(context.Background(),
			"ALTER TABLE messages ADD CONSTRAINT test_reject_marker CHECK (body NOT LIKE '%"+marker+"%')"); err != nil {
			t.Fatal(err)
		}

		resp := s.send("alice", "bob", newSendID(t), "rejected by store "+marker)

		requireError(t, resp, http.StatusInternalServerError, "internal")
		requireNoInternals(t, resp)
		logs := s.logs.String()
		if strings.Contains(logs, marker) || strings.Contains(logs, "Failing row") {
			t.Fatalf("ログに、本文か、保管先のエラーの詳細が含まれる:\n%s", logs)
		}
		// 完了できなかった操作は、操作の名前と、SQLSTATE とともに、記録されている。
		for _, want := range []string{"operation failed", "send direct message", "append message", "23514"} {
			if !strings.Contains(logs, want) {
				t.Fatalf("ログに、%q がない:\n%s", want, logs)
			}
		}
		s.requireStored(0, 0)
	})

	t.Run("確定の直前に失敗した場合", func(t *testing.T) {
		s := startServer(t, withTransactor(func(inner port.Transactor) port.Transactor {
			return &failBeforeCommit{inner: inner, err: errors.New("injected")}
		}))

		requireError(t, s.send("alice", "bob", newSendID(t), "will fail "+marker), http.StatusInternalServerError, "internal")

		logs := s.logs.String()
		if strings.Contains(logs, marker) {
			t.Fatalf("ログに、本文が含まれる:\n%s", logs)
		}
		if !strings.Contains(logs, "operation failed") {
			t.Fatalf("完了できなかった操作が、記録されていない:\n%s", logs)
		}
	})
}

// 要求ごとの記録は、出力しない。完了した操作と、拒否は、ログに現れない。
func TestLogs_NoPerRequestRecords(t *testing.T) {
	s := startServer(t)
	bob := s.connect("bob")

	bob.requireNext(s.mustSend("alice", "bob", "hello"))
	s.mustHistory("bob", "alice")
	requireError(t, s.send("alice", "dave", newSendID(t), "hello"), http.StatusNotFound, "not_found")

	if logs := s.logs.String(); logs != "" {
		t.Fatalf("完了した操作か、拒否が、ログに出力された:\n%s", logs)
	}
}
