-- name: InsertConversationIfAbsent :exec
INSERT INTO conversations (user_low, user_high)
VALUES ($1, $2)
ON CONFLICT (user_low, user_high) DO NOTHING;

-- name: FindConversationID :one
SELECT id
FROM conversations
WHERE user_low = $1 AND user_high = $2;

-- name: LockConversation :one
SELECT id
FROM conversations
WHERE id = $1
FOR UPDATE;
