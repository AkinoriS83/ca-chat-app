import type { Message, RejectionCode } from '../model'

/** 履歴の取得の結果。 */
export type HistoryResult = { kind: 'loaded'; messages: Message[] } | { kind: 'failed' }

/** 送信の結果。 */
export type SendResult =
  /** 完了した。 */
  | { kind: 'completed'; message: Message }
  /** サーバーが拒否した。 */
  | { kind: 'rejected'; code: RejectionCode }
  /** 送信できなかった(要求が失敗した、サーバーが完了できなかった、応答が形式に合わない)。 */
  | { kind: 'failed' }

export type SendRequest = {
  /** 操作を行う利用者の識別名 */
  actor: string
  /** 相手の識別名 */
  peer: string
  sendId: string
  body: string
}

/** サーバーへの操作(履歴の取得、送信)。 */
export interface ChatApi {
  listMessages(actor: string, peer: string): Promise<HistoryResult>
  sendMessage(request: SendRequest): Promise<SendResult>
}
