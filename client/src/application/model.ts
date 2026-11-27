// application の型。ui と、ポート(adapter が実装する)が参照する。

/** 利用者。他の利用者に見せるのは、識別名と表示名だけである(RL-27)。 */
export type UserProfile = {
  /** 識別名。登録した通りの、大文字と小文字で持つ。 */
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

/** サーバーが操作を拒否した理由。 */
export type RejectionCode =
  | 'invalid_body'
  | 'invalid_input'
  | 'invalid_peer'
  | 'not_found'
  | 'unauthenticated'
  | 'invalid_fields'
  | 'handle_taken'
  | 'invalid_signup_credential'
  | 'login_failed'
  | 'rate_limited'
  | 'other'

/** 入力の項目。拒否が、正しくない項目を挙げるために使う。 */
export type InputField = 'contact' | 'handle' | 'displayName' | 'secret'

/** 登録とログインの操作が、完了しなかった理由。'failed' は、完了できなかったこと(拒否ではない)を表す。 */
export type FormError = {
  code: RejectionCode | 'failed'
  /** 正しくない項目のすべて。code が invalid_fields の場合だけ。 */
  fields: InputField[]
}

/** ログインしていないときに示す画面。 */
export type SignedOutScreen = 'login' | 'signupRequest' | 'signupComplete'

/** ログインしていないときに示す知らせ。 */
export type SignedOutNotice = 'sessionEnded' | 'signupRequested' | 'signupCompleted'

/** ログイン状態。 */
export type SessionState =
  /** ログイン状態を、確かめている。 */
  | { kind: 'checking' }
  /** 確かめられなかった。ログインしていないとは、示さない。やり直せる(RL-23)。 */
  | { kind: 'unknown' }
  | {
      kind: 'signedOut'
      screen: SignedOutScreen
      /** 操作の結果を、待っている。 */
      pending: boolean
      error: FormError | null
      notice: SignedOutNotice | null
    }
  | { kind: 'signedIn'; user: UserProfile; logout: 'idle' | 'pending' | 'failed' }

/** 相手の特定の状態。 */
export type PeerSearch = {
  pending: boolean
  /** 特定した相手。 */
  found: UserProfile | null
  error: RejectionCode | 'failed' | null
}

/** 開いている会話の状態。 */
export type Conversation = {
  peer: UserProfile
  history: 'loading' | 'loaded' | 'failed'
  /** 履歴の取得が拒否された理由。拒否でなく、取得できなかった場合は、null。 */
  historyRejection: RejectionCode | null
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
  session: SessionState
  peerSearch: PeerSearch
  conversation: Conversation | null
}
