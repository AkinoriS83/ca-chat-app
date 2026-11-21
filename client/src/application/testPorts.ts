// テスト用の、ポートの実装。application のテストが使う。
import type { FixedUser, Message } from './model'
import type { ChatApi, HistoryResult, SendRequest, SendResult } from './port/chatApi'
import type { FixedUserDirectory } from './port/fixedUserDirectory'
import type {
  DeliveredMessage,
  RealtimeChannel,
  RealtimeConnection,
  RealtimeListener,
} from './port/realtimeChannel'
import type { SendIdSource } from './port/sendIdSource'
import type { Timer } from './port/timer'

/** 結果を、テストから、後で与えられる要求。 */
export type Pending<Request, Result> = {
  request: Request
  resolve(result: Result): void
  reject(reason: unknown): void
}

export class FakeChatApi implements ChatApi {
  readonly historyCalls: Pending<{ actor: string; peer: string }, HistoryResult>[] = []
  readonly sendCalls: Pending<SendRequest, SendResult>[] = []

  listMessages(actor: string, peer: string): Promise<HistoryResult> {
    return new Promise((resolve, reject) => {
      this.historyCalls.push({ request: { actor, peer }, resolve, reject })
    })
  }

  sendMessage(request: SendRequest): Promise<SendResult> {
    return new Promise((resolve, reject) => {
      this.sendCalls.push({ request, resolve, reject })
    })
  }
}

export type FakeConnection = {
  actor: string
  listener: RealtimeListener
  closed: boolean
}

export class FakeRealtimeChannel implements RealtimeChannel {
  readonly connections: FakeConnection[] = []

  connect(actor: string, listener: RealtimeListener): RealtimeConnection {
    const connection: FakeConnection = { actor, listener, closed: false }
    this.connections.push(connection)
    return {
      close: () => {
        connection.closed = true
      },
    }
  }

  /** 最後に確立した接続。 */
  get latest(): FakeConnection {
    const connection = this.connections.at(-1)
    if (!connection) {
      throw new Error('接続が、確立されていない')
    }
    return connection
  }
}

/** 時間を、テストから進める。 */
export class FakeTimer implements Timer {
  private now = 0
  private timers: { due: number; onElapsed: () => void }[] = []

  start(ms: number, onElapsed: () => void): () => void {
    const timer = { due: this.now + ms, onElapsed }
    this.timers.push(timer)
    return () => {
      this.timers = this.timers.filter((t) => t !== timer)
    }
  }

  advance(ms: number): void {
    this.now += ms
    const due = this.timers.filter((t) => t.due <= this.now)
    this.timers = this.timers.filter((t) => t.due > this.now)
    for (const timer of due) {
      timer.onElapsed()
    }
  }

  /** 待っている数。 */
  get pending(): number {
    return this.timers.length
  }
}

export class FakeSendIdSource implements SendIdSource {
  private count = 0

  next(): string {
    this.count++
    return `send-${this.count}`
  }
}

export const alice: FixedUser = { handle: 'alice', displayName: 'Alice' }
export const bob: FixedUser = { handle: 'bob', displayName: 'Bob' }
export const carol: FixedUser = { handle: 'carol', displayName: 'Carol' }

export class FakeFixedUserDirectory implements FixedUserDirectory {
  list(): FixedUser[] {
    return [alice, bob, carol]
  }
}

export function message(seq: number, sender: FixedUser, body = `message ${seq}`): Message {
  return {
    seq,
    sender: { handle: sender.handle, displayName: sender.displayName },
    body,
    sentAt: `2026-11-21T03:04:${String(seq % 60).padStart(2, '0')}.000000Z`,
  }
}

export function delivery(participants: [FixedUser, FixedUser], msg: Message, sendId = `other-${msg.seq}`): DeliveredMessage {
  return { participants: participants.map((p) => p.handle).sort(), sendId, message: msg }
}

/** 約束(Promise)の結果が、application に届くまで待つ。 */
export async function settled(): Promise<void> {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
  }
}
