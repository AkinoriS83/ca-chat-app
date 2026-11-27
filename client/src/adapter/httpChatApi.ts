import type { ChatApi, FindPeerResult, HistoryResult, SendRequest, SendResult } from '../application/port/chatApi'
import type { components } from './generated/openapi'
import { isRecord, isWireMessage, isWireUserProfile, sessionIdHeader, toMessage, toRejection, toUserProfile } from './wire'

type SendMessageRequest = components['schemas']['SendMessageRequest']

function messagesUrl(peer: string): string {
  return `/api/direct-conversations/${encodeURIComponent(peer)}/messages`
}

/**
 * サーバーへの、認証を要する操作を、HTTP で行う(ADR-002)。
 *
 * ログイン状態の識別子を、要求ヘッダーで添える。ログイン状態を示す資格は、ブラウザが、Cookie として保ち、
 * 同じオリジンの要求に付ける。このコードは、資格に触れない(ADR-014)。
 *
 * 結果は、3つに分けて返す。
 * - 完了: 200 で、定めた形式の応答を得た。
 * - 拒否: 4xx で、ErrorResponse を得た(認証の拒否を含む)。
 * - 完了できなかった: 要求が失敗した。5xx を得た。応答が、形式に合わない。
 */
export class HttpChatApi implements ChatApi {
  /** 入力は、検証せず、整えずに送る。検証は、サーバーが行う。 */
  async findPeer(sessionId: string, handle: string): Promise<FindPeerResult> {
    try {
      const response = await fetch(`/api/users/${encodeURIComponent(handle)}`, {
        headers: { [sessionIdHeader]: sessionId },
      })
      if (response.status === 200) {
        const body: unknown = await response.json()
        return isWireUserProfile(body) ? { kind: 'found', peer: toUserProfile(body) } : { kind: 'failed' }
      }
      const rejection = await toRejection(response)
      return rejection ? { kind: 'rejected', code: rejection.code } : { kind: 'failed' }
    } catch {
      return { kind: 'failed' }
    }
  }

  /** 仮実装(WS-P4): 会話のすべてのメッセージを、一度に取得する。 */
  async listMessages(sessionId: string, peer: string): Promise<HistoryResult> {
    try {
      const response = await fetch(messagesUrl(peer), {
        headers: { [sessionIdHeader]: sessionId },
      })
      if (response.status === 200) {
        const body: unknown = await response.json()
        if (!isRecord(body) || !Array.isArray(body.messages) || !body.messages.every(isWireMessage)) {
          return { kind: 'failed' }
        }
        return { kind: 'loaded', messages: body.messages.map(toMessage) }
      }
      const rejection = await toRejection(response)
      return rejection ? { kind: 'rejected', code: rejection.code } : { kind: 'failed' }
    } catch {
      return { kind: 'failed' }
    }
  }

  /** 本文は、検証せず、整えずに送る。検証は、サーバーが行う。 */
  async sendMessage(request: SendRequest): Promise<SendResult> {
    const payload: SendMessageRequest = { sendId: request.sendId, body: request.body }
    try {
      const response = await fetch(messagesUrl(request.peer), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          [sessionIdHeader]: request.sessionId,
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
      const rejection = await toRejection(response)
      return rejection ? { kind: 'rejected', code: rejection.code } : { kind: 'failed' }
    } catch {
      return { kind: 'failed' }
    }
  }
}
