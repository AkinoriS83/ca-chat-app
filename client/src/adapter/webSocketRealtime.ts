import type {
  RealtimeChannel,
  RealtimeConnection,
  RealtimeListener,
} from '../application/port/realtimeChannel'
import type { components } from './generated/openapi'
import { isRecord, isWireMessage, toMessage } from './wire'

type ReadyEvent = components['schemas']['ReadyEvent']
type MessageEvent = components['schemas']['MessageEvent']

// 接続先のパス、ログイン状態の識別子を添えるクエリの名前、ログイン状態が無効になったことを示す終了コード。
// サーバー(wsaccept)と、同じ値を持つ(ADR-014)。
const realtimePath = '/api/realtime'
const sessionQuery = 'session'
const sessionEndedCode = 4401

const readyType: ReadyEvent['type'] = 'ready'
const messageType: MessageEvent['type'] = 'message'

/**
 * 接続先を、表示中のページのオリジンから作る。
 * アドレスに載せるのは、ログイン状態の識別子だけである。資格は、ブラウザが、Cookie で付ける。
 */
function realtimeUrl(sessionId: string): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${location.host}${realtimePath}?${sessionQuery}=${encodeURIComponent(sessionId)}`
}

/**
 * リアルタイムの接続を、ブラウザの WebSocket で行う(ADR-002)。
 * 端末からは、何も送らない。接続が切れても、再接続しない。
 */
export class WebSocketRealtime implements RealtimeChannel {
  connect(sessionId: string, listener: RealtimeListener): RealtimeConnection {
    let finished = false
    const finish = (notify: boolean, sessionEnded = false): void => {
      if (finished) {
        return
      }
      finished = true
      if (notify) {
        listener.onClosed(sessionEnded)
      }
    }

    let socket: WebSocket
    try {
      socket = new WebSocket(realtimeUrl(sessionId))
    } catch {
      finish(true)
      return { close: () => undefined }
    }

    socket.addEventListener('message', (event) => {
      if (finished) {
        return
      }
      const data = parse(event.data)
      if (!isRecord(data)) {
        return
      }
      if (data.type === readyType) {
        listener.onReady()
        return
      }
      if (data.type === messageType && typeof data.sendId === 'string' && isWireMessage(data.message)) {
        listener.onMessage({
          participants: [...data.message.participants],
          sendId: data.sendId,
          message: toMessage(data.message),
        })
      }
    })
    // 接続の確立に失敗した場合も、ブラウザは、close を通知する。
    // サーバーが、ログイン状態が無効になったことを示して閉じた場合を、終了コードで見分ける。
    socket.addEventListener('close', (event) => finish(true, event.code === sessionEndedCode))

    return {
      close: () => {
        // 閉じた後は、通知しない。
        finish(false)
        socket.close()
      },
    }
  }
}

function parse(data: unknown): unknown {
  if (typeof data !== 'string') {
    return undefined
  }
  try {
    return JSON.parse(data) as unknown
  } catch {
    return undefined
  }
}
