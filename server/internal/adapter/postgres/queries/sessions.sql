-- name: InsertSession :one
INSERT INTO sessions (credential_digest, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING id;

-- name: FindSession :one
-- 有効かどうかは、「now が、期限より前」で判定する(domain.Session.ValidAt と同じである)。
SELECT s.id, s.user_id, s.expires_at, u.handle, u.display_name
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.credential_digest = sqlc.arg(credential_digest) AND s.expires_at > sqlc.arg(now)::timestamptz;

-- name: DeleteSession :one
-- 無効にすることは、行の削除で表す。無効にした行を、残さない。
DELETE FROM sessions
WHERE credential_digest = $1
RETURNING id;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= sqlc.arg(now)::timestamptz;
