-- +goose Up
-- 利用者、ログイン状態、登録の申し込み(ADR-016)。
-- 登録の日時など、機能に要らない項目は、持たない(PRV-1)。

-- 連絡先は、揃えた後の値(前後の空白文字を除き、英字を小文字にした値)を保管する。
-- 識別名は、入力された通りに保管し、一意性は、小文字にした値で保つ(RL-07)。
CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    contact      text NOT NULL,
    handle       text NOT NULL,
    display_name text NOT NULL,
    secret_hash  text NOT NULL,
    CONSTRAINT users_contact_key UNIQUE (contact)
);
CREATE UNIQUE INDEX users_handle_key ON users (lower(handle));

-- 資格そのものは、保管しない。要約(SHA-256)だけを保管し、要約で引く(ADR-014)。
CREATE TABLE sessions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_digest bytea       NOT NULL,
    user_id           uuid        NOT NULL REFERENCES users (id),
    expires_at        timestamptz NOT NULL,
    CONSTRAINT sessions_credential_digest_key UNIQUE (credential_digest)
);

-- 登録の申し込みは、連絡先ごとに、1つだけを持つ(RL-04)。
CREATE TABLE signups (
    contact           text PRIMARY KEY,
    credential_digest bytea       NOT NULL,
    expires_at        timestamptz NOT NULL,
    CONSTRAINT signups_credential_digest_key UNIQUE (credential_digest)
);

-- +goose Down
DROP TABLE signups;
DROP TABLE sessions;
DROP TABLE users;
