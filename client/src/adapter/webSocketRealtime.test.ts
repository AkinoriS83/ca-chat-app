// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { RealtimeListener } from '../application/port/realtimeChannel'
import { WebSocketRealtime } from './webSocketRealtime'

/** ブラウザの WebSocket の、テスト用の代わり。 */
class FakeWebSocket extends EventTarget {
  static instances: FakeWebSocket[] = []
  static failOnConstruct = false

  readonly url: string
  closeCalls = 0
  sent: unknown[] = []

  constructor(url: string) {
    super()
    if (FakeWebSocket.failOnConstruct) {
      throw new DOMException('invalid url', 'SyntaxError')
    }
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  send(data: unknown): void {
    this.sent.push(data)
  }

  close(): void {
    this.closeCalls++
  }

  receive(data: unknown): void {
    this.dispatchEvent(new MessageEvent('message', { data }))
  }

  serverClose(): void {
    this.dispatchEvent(new Event('close'))
  }
}

function recordingListener() {
  const events: unknown[] = []
  const listener: RealtimeListener = {
    onReady: () => events.push('ready'),
    onMessage: (delivered) => events.push(delivered),
    onClosed: () => events.push('closed'),
  }
  return { events, listener }
}

const wireMessage = {
  participants: ['alice', 'bob'],
  seq: 3,
  sender: { handle: 'bob', displayName: 'Bob' },
  body: 'hello',
  sentAt: '2026-11-21T03:04:05.123456Z',
}

describe('WebSocketRealtime', () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    FakeWebSocket.failOnConstruct = false
    vi.stubGlobal('WebSocket', FakeWebSocket)
  })

  function connect(actor = 'alice') {
    const { events, listener } = recordingListener()
    const connection = new WebSocketRealtime().connect(actor, listener)
    const socket = FakeWebSocket.instances.at(-1)
    if (!socket) {
      throw new Error('WebSocket が、作られていない')
    }
    return { events, connection, socket }
  }

  it('接続先を、表示中のページのオリジンから作り、利用者を、クエリで添える', () => {
    const { socket } = connect('alice')

    expect(socket.url).toBe(`ws://${location.host}/api/realtime?user=alice`)
    expect(location.host).not.toBe('')
  })

  it('利用者の識別名を、クエリの値として符号化する', () => {
    const { socket } = connect('a&user=b c')

    expect(socket.url).toBe(`ws://${location.host}/api/realtime?user=a%26user%3Db%20c`)
  })

  it('ready と、メッセージの配信を、通知する', () => {
    const { events, socket } = connect()

    socket.receive(JSON.stringify({ type: 'ready' }))
    socket.receive(JSON.stringify({ type: 'message', sendId: 'send-1', message: wireMessage }))

    expect(events).toEqual([
      'ready',
      {
        participants: ['alice', 'bob'],
        sendId: 'send-1',
        message: {
          seq: 3,
          sender: { handle: 'bob', displayName: 'Bob' },
          body: 'hello',
          sentAt: '2026-11-21T03:04:05.123456Z',
        },
      },
    ])
  })

  it('形式に合わないイベントは、捨てる', () => {
    const { events, socket } = connect()

    socket.receive('not json')
    socket.receive(JSON.stringify(null))
    socket.receive(JSON.stringify({ type: 'unknown' }))
    socket.receive(JSON.stringify({ type: 'message', message: wireMessage }))
    socket.receive(JSON.stringify({ type: 'message', sendId: 'send-1', message: { ...wireMessage, seq: '3' } }))
    socket.receive(new ArrayBuffer(4))

    expect(events).toEqual([])
  })

  it('接続が切れたことを、1回だけ通知する', () => {
    const { events, socket } = connect()
    socket.receive(JSON.stringify({ type: 'ready' }))

    socket.serverClose()
    socket.serverClose()

    expect(events).toEqual(['ready', 'closed'])
  })

  it('接続の確立に失敗した場合も、切れたことを通知する', () => {
    const { events, socket } = connect()

    // ブラウザは、確立に失敗した場合、error の後に、close を通知する。
    socket.dispatchEvent(new Event('error'))
    socket.serverClose()

    expect(events).toEqual(['closed'])
  })

  it('WebSocket を作れなかった場合も、切れたことを通知する', () => {
    FakeWebSocket.failOnConstruct = true
    const { events, listener } = recordingListener()

    const connection = new WebSocketRealtime().connect('alice', listener)

    expect(events).toEqual(['closed'])
    expect(() => connection.close()).not.toThrow()
  })

  it('close は、接続を閉じる。閉じた後は、通知しない(WS-41)', () => {
    const { events, connection, socket } = connect()
    socket.receive(JSON.stringify({ type: 'ready' }))

    connection.close()
    socket.receive(JSON.stringify({ type: 'message', sendId: 'send-1', message: wireMessage }))
    socket.serverClose()

    expect(socket.closeCalls).toBe(1)
    expect(events).toEqual(['ready'])
  })

  it('端末からは、何も送らない', () => {
    const { socket } = connect()
    socket.receive(JSON.stringify({ type: 'ready' }))
    socket.receive(JSON.stringify({ type: 'message', sendId: 'send-1', message: wireMessage }))

    expect(socket.sent).toEqual([])
  })
})
