-- name: FindUserByHandle :one
-- 識別名の照合では、大文字と小文字を区別しない(RL-D6)。索引 users_handle_key を使う。
SELECT id, handle, display_name
FROM users
WHERE lower(handle) = lower(sqlc.arg(handle)::text);

-- name: FindUserByContact :one
SELECT id, handle, display_name, secret_hash
FROM users
WHERE contact = $1;

-- name: InsertUser :one
INSERT INTO users (contact, handle, display_name, secret_hash)
VALUES ($1, $2, $3, $4)
RETURNING id;
