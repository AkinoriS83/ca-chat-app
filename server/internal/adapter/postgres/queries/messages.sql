-- name: AppendMessage :one
-- 連番は、会話の行のロック(LockConversation)を得た後に、その会話の最大の連番に1を足して付ける。
-- 送信の日時は、同じ文の clock_timestamp() とする(ADR-010)。
INSERT INTO messages (conversation_id, seq, sender_id, send_id, body, sent_at)
SELECT
    sqlc.arg(conversation_id)::uuid,
    COALESCE(MAX(seq), 0) + 1,
    sqlc.arg(sender_id)::uuid,
    sqlc.arg(send_id)::uuid,
    sqlc.arg(body)::text,
    clock_timestamp()
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id)::uuid
RETURNING seq, sent_at;

-- name: ListMessages :many
-- 仮実装(WS-P4): 会話のすべてのメッセージを、一度に取得する。
SELECT conversation_id, seq, sender_id, send_id, body, sent_at
FROM messages
WHERE conversation_id = $1
ORDER BY seq;

-- name: FindMessageBySend :one
SELECT conversation_id, seq, sender_id, send_id, body, sent_at
FROM messages
WHERE sender_id = $1 AND send_id = $2;
