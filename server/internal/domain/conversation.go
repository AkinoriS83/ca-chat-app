package domain

import "errors"

// ErrSelfConversation は、自分自身を相手とする1on1会話を、作ろうとしたことを示す。
var ErrSelfConversation = errors.New("domain: conversation with oneself")

// DirectPair は、1on1会話の2人である。
// 2人を、識別子の文字列の昇順に並べて保持する。どちらが相手を選んでも、同じ値になる。
// この並びは、保管先の uuid の型の大小と一致する(どちらも、小文字の16進数の、先頭からの比較になる)。
type DirectPair struct {
	low  UserID
	high UserID
}

// NewDirectPair は、2人の組を作る。2人が同じ場合は、ErrSelfConversation を返す。
func NewDirectPair(a, b UserID) (DirectPair, error) {
	if a == b {
		return DirectPair{}, ErrSelfConversation
	}
	if a < b {
		return DirectPair{low: a, high: b}, nil
	}
	return DirectPair{low: b, high: a}, nil
}

// Low は、2人のうち、識別子が小さい方を返す。
func (p DirectPair) Low() UserID { return p.low }

// High は、2人のうち、識別子が大きい方を返す。
func (p DirectPair) High() UserID { return p.high }

// ConversationID は、会話の識別子である。UUID の文字列(小文字)で表す。
type ConversationID string

// Conversation は、成立した1on1会話である。
type Conversation struct {
	ID   ConversationID
	Pair DirectPair
}
