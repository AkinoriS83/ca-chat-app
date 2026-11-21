// application の型。ui と、ポート(adapter が実装する)が参照する。

/** 固定ユーザー。仮実装(WS-P1): 利用者は、あらかじめ定めた3人に限る。 */
export type FixedUser = {
  /** 識別名 */
  handle: string
  /** 表示名 */
  displayName: string
}

/** 完了したメッセージ。 */
export type Message = {
  /** 会話ごとの連番。表示の並びは、この順とする(ADR-010)。 */
  seq: number
  sender: { handle: string; displayName: string }
  body: string
  /** 送信の日時(サーバーが保管した日時。UTC の文字列)。 */
  sentAt: string
}

/** 送信中のメッセージと、送信できなかったメッセージ。送信の識別子をキーとして保持する(ADR-011)。 */
export type Outgoing = {
  sendId: string
  body: string
  status: 'sending' | 'failed'
}

/** サーバーが送信を拒否した理由。 */
export type RejectionCode =
  | 'invalid_body'
  | 'invalid_input'
  | 'invalid_peer'
  | 'not_found'
  | 'unauthenticated'
  | 'other'

/** 開いている会話の状態。 */
export type Conversation = {
  peer: FixedUser
  history: 'loading' | 'loaded' | 'failed'
  /** 完了したメッセージ。連番の順。 */
  messages: Message[]
  /** 送信中と、送信できなかったメッセージ。送信した順。 */
  outgoing: Outgoing[]
  /** 入力欄の内容。 */
  draft: string
  /** 直前の送信が拒否された理由。 */
  rejection: RejectionCode | null
}

export type ChatState = {
  /** 利用者と相手の選択肢。仮実装(WS-P1、WS-P3): 固定ユーザーの一覧である。 */
  users: FixedUser[]
  currentUser: FixedUser | null
  conversation: Conversation | null
}
