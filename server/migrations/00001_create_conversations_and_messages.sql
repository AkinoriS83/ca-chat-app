-- +goose Up
-- 利用者の表は、00002 で加える。user_low、user_high、sender_id の外部キーは、00003 で加える。
CREATE TABLE conversations (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low  uuid NOT NULL,
    user_high uuid NOT NULL,
    CONSTRAINT conversations_pair_order CHECK (user_low < user_high),
    CONSTRAINT conversations_pair_key UNIQUE (user_low, user_high)
);

CREATE TABLE messages (
    conversation_id uuid        NOT NULL REFERENCES conversations (id),
    seq             bigint      NOT NULL,
    sender_id       uuid        NOT NULL,
    send_id         uuid        NOT NULL,
    body            text        NOT NULL,
    sent_at         timestamptz NOT NULL,
    CONSTRAINT messages_pkey PRIMARY KEY (conversation_id, seq),
    CONSTRAINT messages_sender_send_key UNIQUE (sender_id, send_id)
);

-- +goose Down
DROP TABLE messages;
DROP TABLE conversations;
