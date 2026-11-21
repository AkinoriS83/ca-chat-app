import type {
  RealtimeChannel,
  RealtimeConnection,
  RealtimeListener,
} from '../application/port/realtimeChannel'
import type { components } from './generated/openapi'
import { isRecord, isWireMessage, toMessage } from './wire'

type ReadyEvent = components['schemas']['ReadyEvent']
type MessageEvent = components['schemas']['MessageEvent']

// 接続先のパスと、利用者を添えるクエリの名前。サーバー(wsaccept)と、同じ値を持つ。
const realtimePath = '/api/realtime'
// 仮実装(WS-P2): 仮のログイン。利用者を添える、クエリの名前。
const fixedUserQuery = 'user'

const readyType: ReadyEvent['type'] = 'ready'
const messageType: MessageEvent['type'] = 'message'

/** 接続先を、表示中のページのオリジンから作る。 */
function realtimeUrl(actor: string): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  // 仮実装(WS-P2): 選んだ固定ユーザーの識別名を、そのまま添える。
  return `${scheme}//${location.host}${realtimePath}?${fixedUserQuery}=${encodeURIComponent(actor)}`
}

/**
 * リアルタイムの接続を、ブラウザの WebSocket で行う(ADR-002)。
 * 端末からは、何も送らない。接続が切れても、再接続しない。
 */
export class WebSocketRealtime implements RealtimeChannel {
  connect(actor: string, listener: RealtimeListener): RealtimeConnection {
    let finished = false
    const finish = (notify: boolean): void => {
      if (finished) {
        return
      }
      finished = true
      if (notify) {
        listener.onClosed()
      }
    }

    let socket: WebSocket
    try {
      socket = new WebSocket(realtimeUrl(actor))
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
    socket.addEventListener('close', () => finish(true))

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
