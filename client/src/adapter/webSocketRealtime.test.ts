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

  serverClose(code = 1006): void {
    this.dispatchEvent(new CloseEvent('close', { code }))
  }
}

function recordingListener() {
  const events: unknown[] = []
  const listener: RealtimeListener = {
    onReady: () => events.push('ready'),
    onMessage: (delivered) => events.push(delivered),
    onClosed: (sessionEnded) => events.push(sessionEnded ? 'session-ended' : 'closed'),
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

  function connect(sessionId = 'session-1') {
    const { events, listener } = recordingListener()
    const connection = new WebSocketRealtime().connect(sessionId, listener)
    const socket = FakeWebSocket.instances.at(-1)
    if (!socket) {
      throw new Error('WebSocket が、作られていない')
    }
    return { events, connection, socket }
  }

  it('接続先を、表示中のページのオリジンから作り、ログイン状態の識別子を、クエリで添える', () => {
    const { socket } = connect()

    expect(socket.url).toBe(`ws://${location.host}/api/realtime?session=session-1`)
    expect(location.host).not.toBe('')
  })

  it('ログイン状態の識別子を、クエリの値として符号化する', () => {
    const { socket } = connect('a&session=b c')

    expect(socket.url).toBe(`ws://${location.host}/api/realtime?session=a%26session%3Db%20c`)
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

  it('終了コード 4401 で閉じられた場合は、ログイン状態が無効になったこととして、通知する(RL-19)', () => {
    const { events, socket } = connect()
    socket.receive(JSON.stringify({ type: 'ready' }))

    socket.serverClose(4401)

    expect(events).toEqual(['ready', 'session-ended'])
  })

  it('そのほかの終了コードは、ログイン状態が無効になったこととしては、通知しない', () => {
    for (const code of [1000, 1006, 1008, 4400, 4402]) {
      const { events, socket } = connect()
      socket.serverClose(code)
      expect(events).toEqual(['closed'])
    }
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
