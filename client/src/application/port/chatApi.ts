import type { Message, RejectionCode, UserProfile } from '../model'

/** 履歴の取得の結果。 */
export type HistoryResult =
  | { kind: 'loaded'; messages: Message[] }
  /** サーバーが拒否した(認証の拒否を含む)。 */
  | { kind: 'rejected'; code: RejectionCode }
  /** 取得できなかった(要求が失敗した、サーバーが完了できなかった、応答が形式に合わない)。 */
  | { kind: 'failed' }

/** 送信の結果。 */
export type SendResult =
  /** 完了した。 */
  | { kind: 'completed'; message: Message }
  /** サーバーが拒否した(認証の拒否を含む)。 */
  | { kind: 'rejected'; code: RejectionCode }
  /** 送信できなかった(要求が失敗した、サーバーが完了できなかった、応答が形式に合わない)。 */
  | { kind: 'failed' }

/** 相手の特定の結果。 */
export type FindPeerResult =
  | { kind: 'found'; peer: UserProfile }
  | { kind: 'rejected'; code: RejectionCode }
  | { kind: 'failed' }

export type SendRequest = {
  /** ログイン状態の識別子 */
  sessionId: string
  /** 相手の識別名 */
  peer: string
  sendId: string
  body: string
}

/**
 * サーバーへの操作(相手の特定、履歴の取得、送信)。認証を要する。
 * ログイン状態の識別子を、受け取って添える。資格は、ブラウザが付ける(ADR-014)。
 */
export interface ChatApi {
  findPeer(sessionId: string, handle: string): Promise<FindPeerResult>
  /** 仮実装(WS-P4): 会話のすべてのメッセージを、一度に取得する。フェーズ1で、分割しての取得に置き換える。 */
  listMessages(sessionId: string, peer: string): Promise<HistoryResult>
  sendMessage(request: SendRequest): Promise<SendResult>
}
