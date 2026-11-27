-- +goose Up
-- 会話とメッセージを、利用者の表に結び付ける。
--
-- 固定ユーザー(ウォーキングスケルトンの仮実装)の会話とメッセージは、引き継がない
-- (「登録とログイン」の仕様「対象とするフローの範囲」)。利用者の表に居ない利用者の会話と、
-- そのメッセージを除いてから、外部キーを加える。
DELETE FROM messages
WHERE conversation_id IN (
    SELECT c.id
    FROM conversations c
    WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.user_low)
       OR NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.user_high)
);

DELETE FROM conversations c
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.user_low)
   OR NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.user_high);

ALTER TABLE conversations
    ADD CONSTRAINT conversations_user_low_fkey FOREIGN KEY (user_low) REFERENCES users (id),
    ADD CONSTRAINT conversations_user_high_fkey FOREIGN KEY (user_high) REFERENCES users (id);

ALTER TABLE messages
    ADD CONSTRAINT messages_sender_id_fkey FOREIGN KEY (sender_id) REFERENCES users (id);

-- +goose Down
-- 除いた会話とメッセージは、戻らない。
ALTER TABLE messages DROP CONSTRAINT messages_sender_id_fkey;
ALTER TABLE conversations
    DROP CONSTRAINT conversations_user_high_fkey,
    DROP CONSTRAINT conversations_user_low_fkey;
