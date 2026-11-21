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
  /** 接続の確立に失敗した、または、接続が切れた。 */
  onClosed(): void
}

export interface RealtimeConnection {
  /** 接続を閉じる。閉じた後は、通知しない。 */
  close(): void
}

/** リアルタイムの接続。 */
export interface RealtimeChannel {
  connect(actor: string, listener: RealtimeListener): RealtimeConnection
}
