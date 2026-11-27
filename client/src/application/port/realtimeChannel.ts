import type { Message } from '../model'

/** 配信されたメッセージ。 */
export type DeliveredMessage = {
  /** 会話の参加者2人の識別名。 */
  participants: string[]
  /** 送信の識別子(ADR-011)。 */
  sendId: string
  message: Message
}

/** 接続からの通知を受ける。 */
export type RealtimeListener = {
  /** 受信を開始できた(サーバーが、この接続を、配信の宛先として登録した)。 */
  onReady(): void
  onMessage(delivered: DeliveredMessage): void
  /**
   * 接続の確立に失敗した、または、接続が切れた。
   * sessionEnded は、サーバーが、ログイン状態が無効になったことを示して、閉じたかを表す(ADR-014)。
   */
  onClosed(sessionEnded: boolean): void
}

export interface RealtimeConnection {
  /** 接続を閉じる。閉じた後は、通知しない。 */
  close(): void
}

/** リアルタイムの接続。 */
export interface RealtimeChannel {
  /** sessionId は、ログイン状態の識別子である。資格は、ブラウザが付ける。 */
  connect(sessionId: string, listener: RealtimeListener): RealtimeConnection
}
