package domain

import "time"

// Message は、保管したメッセージである。
type Message struct {
	ConversationID ConversationID
	Seq            int64 // 会話ごとの連番。メッセージの順序(ADR-010)
	SenderID       UserID
	SendID         SendID
	Body           Body
	SentAt         time.Time // 保管先が付けた、送信の日時(UTC)
}
