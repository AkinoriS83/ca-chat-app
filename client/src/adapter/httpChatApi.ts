import type { ChatApi, HistoryResult, SendRequest, SendResult } from '../application/port/chatApi'
import type { components } from './generated/openapi'
import { isRecord, isWireMessage, toMessage, toRejectionCode } from './wire'

type SendMessageRequest = components['schemas']['SendMessageRequest']

// 仮実装(WS-P2): 仮のログイン。利用者を添える、要求ヘッダーの名前。
const fixedUserHeader = 'X-Fixed-User'

function messagesUrl(peer: string): string {
  return `/api/direct-conversations/${encodeURIComponent(peer)}/messages`
}

/** サーバーへの操作を、HTTP で行う(ADR-002)。 */
export class HttpChatApi implements ChatApi {
  /** 履歴の取得は、200 のほかを、すべて「取得できなかった」として返す。 */
  async listMessages(actor: string, peer: string): Promise<HistoryResult> {
    try {
      const response = await fetch(messagesUrl(peer), {
        // 仮実装(WS-P2): 選んだ固定ユーザーの識別名を、そのまま添える。
        headers: { [fixedUserHeader]: actor },
      })
      if (response.status !== 200) {
        return { kind: 'failed' }
      }
      const body: unknown = await response.json()
      if (!isRecord(body) || !Array.isArray(body.messages) || !body.messages.every(isWireMessage)) {
        return { kind: 'failed' }
      }
      return { kind: 'loaded', messages: body.messages.map(toMessage) }
    } catch {
      return { kind: 'failed' }
    }
  }

  /**
   * 送信の結果を、3つに分けて返す。
   * - 完了: 200 で、SendMessageResponse を得た。
   * - 拒否: 4xx で、ErrorResponse を得た。
   * - 送信できなかった: 要求が失敗した。5xx を得た。応答が、形式に合わない。
   *
   * 本文は、検証せず、整えずに送る。検証は、サーバーが行う。
   */
  async sendMessage(request: SendRequest): Promise<SendResult> {
    const payload: SendMessageRequest = { sendId: request.sendId, body: request.body }
    try {
      const response = await fetch(messagesUrl(request.peer), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          // 仮実装(WS-P2): 選んだ固定ユーザーの識別名を、そのまま添える。
          [fixedUserHeader]: request.actor,
        },
        body: JSON.stringify(payload),
      })

      if (response.status === 200) {
        const body: unknown = await response.json()
        if (!isRecord(body) || !isWireMessage(body.message)) {
          return { kind: 'failed' }
        }
        return { kind: 'completed', message: toMessage(body.message) }
      }

      if (response.status >= 400 && response.status < 500) {
        const code = toRejectionCode(await response.json())
        return code ? { kind: 'rejected', code } : { kind: 'failed' }
      }
      return { kind: 'failed' }
    } catch {
      return { kind: 'failed' }
    }
  }
}
