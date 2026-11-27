// テスト用の、ポートの実装。application と ui のテストが使う。
import type { Message, UserProfile } from './model'
import type {
  AuthApi,
  CommandResult,
  CompleteSignupRequest,
  LoginResult,
  SessionCheck,
  SessionInfo,
} from './port/authApi'
import type { ChatApi, FindPeerResult, HistoryResult, SendRequest, SendResult } from './port/chatApi'
import type { EntryLink } from './port/entryLink'
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

function pend<Request, Result>(calls: Pending<Request, Result>[], request: Request): Promise<Result> {
  return new Promise((resolve, reject) => {
    calls.push({ request, resolve, reject })
  })
}

export class FakeAuthApi implements AuthApi {
  readonly sessionCalls: Pending<null, SessionCheck>[] = []
  readonly loginCalls: Pending<{ contact: string; secret: string }, LoginResult>[] = []
  readonly logoutCalls: Pending<{ sessionId: string }, CommandResult>[] = []
  readonly signupCalls: Pending<{ contact: string }, CommandResult>[] = []
  readonly completeCalls: Pending<CompleteSignupRequest, CommandResult>[] = []

  currentSession(): Promise<SessionCheck> {
    return pend(this.sessionCalls, null)
  }

  login(contact: string, secret: string): Promise<LoginResult> {
    return pend(this.loginCalls, { contact, secret })
  }

  logout(sessionId: string): Promise<CommandResult> {
    return pend(this.logoutCalls, { sessionId })
  }

  requestSignup(contact: string): Promise<CommandResult> {
    return pend(this.signupCalls, { contact })
  }

  completeSignup(request: CompleteSignupRequest): Promise<CommandResult> {
    return pend(this.completeCalls, request)
  }
}

export class FakeChatApi implements ChatApi {
  readonly peerCalls: Pending<{ sessionId: string; handle: string }, FindPeerResult>[] = []
  readonly historyCalls: Pending<{ sessionId: string; peer: string }, HistoryResult>[] = []
  readonly sendCalls: Pending<SendRequest, SendResult>[] = []

  findPeer(sessionId: string, handle: string): Promise<FindPeerResult> {
    return pend(this.peerCalls, { sessionId, handle })
  }

  listMessages(sessionId: string, peer: string): Promise<HistoryResult> {
    return pend(this.historyCalls, { sessionId, peer })
  }

  sendMessage(request: SendRequest): Promise<SendResult> {
    return pend(this.sendCalls, request)
  }
}

export type FakeConnection = {
  sessionId: string
  listener: RealtimeListener
  closed: boolean
}

export class FakeRealtimeChannel implements RealtimeChannel {
  readonly connections: FakeConnection[] = []

  connect(sessionId: string, listener: RealtimeListener): RealtimeConnection {
    const connection: FakeConnection = { sessionId, listener, closed: false }
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

/** 案内の資格を、テストから与える。読み取ると、除く。 */
export class FakeEntryLink implements EntryLink {
  credential: string | null
  taken = 0

  constructor(credential: string | null = null) {
    this.credential = credential
  }

  takeSignupCredential(): string | null {
    this.taken++
    const credential = this.credential
    this.credential = null
    return credential
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

export const alice: UserProfile = { handle: 'alice', displayName: 'Alice' }
export const bob: UserProfile = { handle: 'bob', displayName: 'Bob' }
export const carol: UserProfile = { handle: 'carol', displayName: 'Carol' }

/** テストの利用者の、ログイン状態。識別子は、利用者ごとに定まる。 */
export function sessionOf(user: UserProfile): SessionInfo {
  return { sessionId: `session-${user.handle}`, user }
}

export function message(seq: number, sender: UserProfile, body = `message ${seq}`): Message {
  return {
    seq,
    sender: { handle: sender.handle, displayName: sender.displayName },
    body,
    sentAt: `2026-11-21T03:04:${String(seq % 60).padStart(2, '0')}.000000Z`,
  }
}

export function delivery(participants: [UserProfile, UserProfile], msg: Message, sendId = `other-${msg.seq}`): DeliveredMessage {
  return { participants: participants.map((p) => p.handle).sort(), sendId, message: msg }
}

/** 約束(Promise)の結果が、application に届くまで待つ。 */
export async function settled(): Promise<void> {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
  }
}
