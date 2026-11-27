-- name: UpsertSignup :exec
-- 同じ連絡先の、以前の申し込みを置き換える(RL-04)。
INSERT INTO signups (contact, credential_digest, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (contact) DO UPDATE
SET credential_digest = EXCLUDED.credential_digest, expires_at = EXCLUDED.expires_at;

-- name: FindSignup :one
SELECT contact, expires_at
FROM signups
WHERE credential_digest = sqlc.arg(credential_digest) AND expires_at > sqlc.arg(now)::timestamptz;

-- name: ConsumeSignup :one
-- 使用済みにすることは、行の削除で表す。同じ資格での完了が、同時に行われた場合、
-- 後の DELETE は、先のトランザクションの確定を待ち、行がないことを得る(RL-05)。
DELETE FROM signups
WHERE credential_digest = sqlc.arg(credential_digest) AND expires_at > sqlc.arg(now)::timestamptz
RETURNING contact, expires_at;

-- name: DeleteExpiredSignups :exec
DELETE FROM signups
WHERE expires_at <= sqlc.arg(now)::timestamptz;
