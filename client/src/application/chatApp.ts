import { isBetween, mergeMessage, mergeMessages } from './messages'
import type { ChatState, Conversation, FixedUser, Outgoing } from './model'
import type { ChatApi, SendResult } from './port/chatApi'
import type { FixedUserDirectory } from './port/fixedUserDirectory'
import type { DeliveredMessage, RealtimeChannel, RealtimeConnection } from './port/realtimeChannel'
import type { SendIdSource } from './port/sendIdSource'
import type { Timer } from './port/timer'

/** 送信の応答を待つ時間(仕様「メッセージを送る」)。 */
export const SEND_TIMEOUT_MS = 10_000

export type ChatAppPorts = {
  api: ChatApi
  realtime: RealtimeChannel
  timer: Timer
  sendIds: SendIdSource
  users: FixedUserDirectory
}

/** リアルタイムの接続の状態。 */
type Link = 'none' | 'connecting' | 'ready' | 'closed'

/** 送信の試行。送り直すたびに、新しくなる。 */
type Attempt = { id: number; cancelTimer: () => void }

/**
 * クライアントの振る舞いと状態。React と、ブラウザの入出力のAPIを、使わない(ADR-005)。
 * ui は、subscribe と getState を、React の useSyncExternalStore につなぐ。
 *
 * 振る舞いの規則は、次の通りである。コメントとテストは、この番号(C-1 など)で参照する。
 *
 * - C-1(WS-41): selectUser は、それまでの接続を閉じ、会話の状態を捨ててから、新しい利用者として、
 *   接続を確立する。leaveUser は、接続を閉じ、会話の状態を捨てて、利用者を選んでいない状態に戻す。
 *   どちらの場合も、それより前の接続、履歴の取得、送信に対して、後から届いた結果は、すべて捨てる。
 * - C-2(ADR-010、WS-22): openConversation は、会話の状態を作り直す。接続が ready を受けた後であれば、
 *   すぐに履歴を取得する。接続を確立している途中であれば、ready を待ってから取得する。待つ間に、
 *   接続の確立に失敗した場合と、接続が切れた場合は、その時点で取得する。接続が切れている場合は、
 *   待たずに取得する。
 * - C-3(WS-07、WS-22、WS-37): 履歴、完了の応答、配信のどれで届いたメッセージも、連番をキーとして、
 *   messages に加える。同じ連番のものが既にあれば、加えない。並びは、連番の順とする。
 * - C-4(WS-22): 配信は、participants が、自分と、開いている会話の相手の組に一致するものだけを、加える。
 *   一致しないものは、捨てる。履歴の取得中に届いた配信も、同じ規則で、すぐに加える。
 * - C-5: send は、draft が空の文字列であれば、何もしない。そうでなければ、送信の識別子を作り、
 *   outgoing に sending として加え、draft と rejection を空にして、送信する。同時に、10秒の時間の
 *   経過を待ち始める。送信の前に、本文は検証しない。
 * - C-6(WS-37、ADR-011): 完了の応答、または、同じ送信の識別子を持ち、送信者が自分である配信の、
 *   先に届いた方で、その outgoing を除き、メッセージを C-3 で加える。後から届いた方は、C-3 によって、
 *   加わらない。送信者が自分でない配信は、送信の識別子が一致しても、outgoing を除かない。
 * - C-7(WS-40): 拒否の応答で、その outgoing を除き、rejection に理由を入れ、本文を draft に戻す。
 *   draft に別の入力がある場合は、その前に、改行で区切って戻す。
 * - C-8(WS-17): 送信が届かない場合、完了できなかったことを示す応答の場合、10秒が過ぎた場合に、
 *   その outgoing を failed にする。要求は、取り消さない。
 * - C-9(WS-17、WS-18): resend は、failed の outgoing を、同じ送信の識別子と本文で、sending に戻して、
 *   送信する。10秒を、改めて待ち始める。
 * - C-10(ADR-011): failed にした後に、完了の応答、配信、拒否の応答が届いた場合も、C-6、C-7 を行う。
 *   その outgoing が既にない場合は、応答を捨てる。送り直しの前の試行の、失敗と時間切れは、捨てる。
 * - C-11: 別の会話を開いたとき、それまでの会話の状態(outgoing と draft を含む)を捨てる。
 *   前の会話に対して、後から届いた結果は、捨てる。
 * - C-12(WS-17): 開いている会話と同じ相手を、openConversation で指定された場合、会話の状態を保つ。
 *   history が failed であれば、loading に戻して、C-2 の通りに取得し直す。そのほかの場合は、何もしない。
 */
export class ChatApp {
  private readonly ports: ChatAppPorts
  private state: ChatState
  private readonly listeners = new Set<() => void>()

  /** 利用者を選ぶ、または、選択に戻るたびに増える。それより前の結果を、捨てるために使う(C-1)。 */
  private session = 0
  /** 会話の状態を作り直す、または、捨てるたびに増える。前の会話への結果を、捨てるために使う(C-11)。 */
  private conversationEpoch = 0

  private connection: RealtimeConnection | null = null
  private link: Link = 'none'
  /** 履歴の取得が、ready を待っている(C-2)。 */
  private historyWaiting = false

  /** 送信の識別子ごとの、現在の試行(C-8〜C-10)。 */
  private readonly attempts = new Map<string, Attempt>()
  private lastAttemptId = 0

  constructor(ports: ChatAppPorts) {
    this.ports = ports
    this.state = { users: ports.users.list(), currentUser: null, conversation: null }
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  readonly getState = (): ChatState => this.state

  /**
   * C-1: それまでの接続を閉じ、会話の状態を捨ててから、新しい利用者として、接続を確立する。
   * それより前の接続、履歴の取得、送信に対して、後から届いた結果は、すべて捨てる(WS-41)。
   */
  readonly selectUser = (handle: string): void => {
    const user = this.findUser(handle)
    if (!user) {
      return
    }
    this.endSession()
    this.setState({ ...this.state, currentUser: user, conversation: null })

    const session = this.session
    this.link = 'connecting'
    this.connection = this.ports.realtime.connect(user.handle, {
      onReady: () => {
        if (session !== this.session || this.link !== 'connecting') {
          return
        }
        this.link = 'ready'
        this.fetchHistoryIfWaiting()
      },
      onMessage: (delivered) => {
        if (session !== this.session) {
          return
        }
        this.receiveDelivery(delivered)
      },
      onClosed: () => {
        if (session !== this.session) {
          return
        }
        this.link = 'closed'
        // C-2: 待つ間に、接続の確立に失敗した場合と、接続が切れた場合は、その時点で取得する。
        this.fetchHistoryIfWaiting()
      },
    })
  }

  /** C-1: 接続を閉じ、会話の状態を捨てて、利用者を選んでいない状態に戻す。 */
  readonly leaveUser = (): void => {
    this.endSession()
    this.setState({ ...this.state, currentUser: null, conversation: null })
  }

  /**
   * C-2: 会話の状態を作り直し、履歴を取得する。
   * C-12: 開いている会話と同じ相手を指定された場合は、会話の状態を保つ。
   * 履歴を取得できなかったときだけ、取得し直す。
   */
  readonly openConversation = (peerHandle: string): void => {
    const self = this.state.currentUser
    const peer = this.findUser(peerHandle)
    if (!self || !peer || peer.handle === self.handle) {
      return
    }

    const current = this.state.conversation
    if (current && current.peer.handle === peer.handle) {
      if (current.history === 'failed') {
        this.updateConversation((c) => ({ ...c, history: 'loading' }))
        this.startHistory()
      }
      return
    }

    // C-11: それまでの会話の状態(outgoing と draft を含む)を捨てる。
    this.discardConversation()
    this.setState({
      ...this.state,
      conversation: { peer, history: 'loading', messages: [], outgoing: [], draft: '', rejection: null },
    })
    this.startHistory()
  }

  readonly setDraft = (text: string): void => {
    this.updateConversation((c) => ({ ...c, draft: text }))
  }

  /**
   * C-5: draft が空の文字列であれば、何もしない。そうでなければ、送信の識別子を作り、
   * outgoing に sending として加え、draft と rejection を空にして、送信する。
   * クライアントは、送信の前に、本文を検証しない。検証は、サーバーが行う。
   */
  readonly send = (): void => {
    const conversation = this.state.conversation
    if (!this.state.currentUser || !conversation || conversation.draft === '') {
      return
    }
    const sendId = this.ports.sendIds.next()
    const body = conversation.draft
    this.updateConversation((c) => ({
      ...c,
      outgoing: [...c.outgoing, { sendId, body, status: 'sending' }],
      draft: '',
      rejection: null,
    }))
    this.dispatch(sendId, body)
  }

  /** C-9: failed の outgoing を、同じ送信の識別子と本文で、sending に戻して、送信する。 */
  readonly resend = (sendId: string): void => {
    const outgoing = this.findOutgoing(sendId)
    if (!outgoing || outgoing.status !== 'failed') {
      return
    }
    this.updateConversation((c) => ({ ...c, outgoing: withStatus(c.outgoing, sendId, 'sending') }))
    this.dispatch(sendId, outgoing.body)
  }

  // ---- 利用者と接続 ----

  private findUser(handle: string): FixedUser | undefined {
    return this.state.users.find((user) => user.handle === handle)
  }

  private endSession(): void {
    this.session++
    this.connection?.close()
    this.connection = null
    this.link = 'none'
    this.discardConversation()
  }

  private discardConversation(): void {
    this.conversationEpoch++
    this.historyWaiting = false
    for (const attempt of this.attempts.values()) {
      attempt.cancelTimer()
    }
    this.attempts.clear()
  }

  // ---- 履歴 ----

  /**
   * C-2: 接続が ready を受けた後であれば、すぐに履歴を取得する。接続を確立している途中であれば、
   * ready を待ってから取得する。接続が切れている場合は、待たずに取得する。
   *
   * サーバーが、この接続を配信の宛先として登録した後に、履歴を読むためである。
   * こうすると、会話を開くのと前後して保管されたメッセージが、履歴と配信の、少なくとも
   * 一方で届く(ADR-010)。
   */
  private startHistory(): void {
    if (this.link === 'connecting') {
      this.historyWaiting = true
      return
    }
    this.fetchHistory()
  }

  private fetchHistoryIfWaiting(): void {
    if (this.historyWaiting) {
      this.fetchHistory()
    }
  }

  // 仮実装(WS-P4): 履歴を、1回の取得で読み込む。フェーズ1で、分割しての取得に置き換える。
  private fetchHistory(): void {
    this.historyWaiting = false
    const self = this.state.currentUser
    const conversation = this.state.conversation
    if (!self || !conversation) {
      return
    }

    const isCurrent = this.currentChecker()
    const failed = (): void => {
      if (isCurrent()) {
        this.updateConversation((c) => ({ ...c, history: 'failed' }))
      }
    }
    this.ports.api.listMessages(self.handle, conversation.peer.handle).then((result) => {
      if (!isCurrent()) {
        return
      }
      if (result.kind !== 'loaded') {
        failed()
        return
      }
      // C-3: 取得中に届いた配信と、統合する。
      this.updateConversation((c) => ({
        ...c,
        history: 'loaded',
        messages: mergeMessages(c.messages, result.messages),
      }))
    }, failed)
  }

  // ---- 配信 ----

  /**
   * C-4: participants が、自分と、開いている会話の相手の組に一致するものだけを加える。
   * 履歴の取得中に届いた配信も、すぐに加える。
   * C-6: 同じ送信の識別子を持ち、送信者が自分である配信で、その outgoing を除く。
   * 送信者が自分でない配信は、送信の識別子が一致しても、outgoing を除かない。
   * 一致は、送信者ごとに判定するためである。
   */
  private receiveDelivery(delivered: DeliveredMessage): void {
    const self = this.state.currentUser
    const conversation = this.state.conversation
    if (!self || !conversation || !isBetween(delivered.participants, self.handle, conversation.peer.handle)) {
      return
    }

    const fromSelf = delivered.message.sender.handle === self.handle
    if (fromSelf && this.findOutgoing(delivered.sendId)) {
      this.clearAttempt(delivered.sendId)
      this.updateConversation((c) => ({
        ...c,
        outgoing: without(c.outgoing, delivered.sendId),
        messages: mergeMessage(c.messages, delivered.message),
      }))
      return
    }
    this.updateConversation((c) => ({ ...c, messages: mergeMessage(c.messages, delivered.message) }))
  }

  // ---- 送信 ----

  private dispatch(sendId: string, body: string): void {
    const self = this.state.currentUser
    const conversation = this.state.conversation
    if (!self || !conversation) {
      return
    }

    // 送り直しの前の試行の、失敗と時間切れは、捨てる(C-10)。
    this.clearAttempt(sendId)
    const attemptId = ++this.lastAttemptId
    const isCurrent = this.currentChecker()

    // C-5、C-9: 10秒の時間の経過を、待ち始める。
    const cancelTimer = this.ports.timer.start(SEND_TIMEOUT_MS, () => {
      if (isCurrent()) {
        this.failAttempt(sendId, attemptId)
      }
    })
    this.attempts.set(sendId, { id: attemptId, cancelTimer })

    const settle = (result: SendResult): void => {
      if (isCurrent()) {
        this.settleSend(sendId, attemptId, result)
      }
    }
    this.ports.api
      .sendMessage({ actor: self.handle, peer: conversation.peer.handle, sendId, body })
      .then(settle, () => settle({ kind: 'failed' }))
  }

  private settleSend(sendId: string, attemptId: number, result: SendResult): void {
    // C-10: その outgoing が既にない場合は、応答を捨てる。
    const outgoing = this.findOutgoing(sendId)
    if (!outgoing) {
      return
    }

    switch (result.kind) {
      case 'completed':
        // C-6: 完了の応答で、その outgoing を除き、メッセージを加える。
        // C-10: failed にした後に届いた場合も、同じである。
        this.clearAttempt(sendId)
        this.updateConversation((c) => ({
          ...c,
          outgoing: without(c.outgoing, sendId),
          messages: mergeMessage(c.messages, result.message),
        }))
        return
      case 'rejected':
        // C-7: 拒否の応答で、その outgoing を除き、理由を入れ、本文を draft に戻す。
        // draft に別の入力がある場合は、その前に、改行で区切って戻す。
        this.clearAttempt(sendId)
        this.updateConversation((c) => ({
          ...c,
          outgoing: without(c.outgoing, sendId),
          rejection: result.code,
          draft: c.draft === '' ? outgoing.body : `${outgoing.body}\n${c.draft}`,
        }))
        return
      case 'failed':
        this.failAttempt(sendId, attemptId)
        return
    }
  }

  /**
   * C-8: 送信が届かない場合、完了できなかったことを示す応答の場合、10秒が過ぎた場合に、
   * その outgoing を failed にする。要求は、取り消さない。
   */
  private failAttempt(sendId: string, attemptId: number): void {
    const attempt = this.attempts.get(sendId)
    if (!attempt || attempt.id !== attemptId) {
      // C-10: 送り直しの前の試行の、失敗と時間切れは、捨てる。
      return
    }
    this.clearAttempt(sendId)
    this.updateConversation((c) => ({ ...c, outgoing: withStatus(c.outgoing, sendId, 'failed') }))
  }

  private clearAttempt(sendId: string): void {
    this.attempts.get(sendId)?.cancelTimer()
    this.attempts.delete(sendId)
  }

  private findOutgoing(sendId: string): Outgoing | undefined {
    return this.state.conversation?.outgoing.find((o) => o.sendId === sendId)
  }

  // ---- 状態 ----

  /** 呼んだ時点の利用者と会話が、今も同じであるかを返す関数を作る(C-1、C-11)。 */
  private currentChecker(): () => boolean {
    const session = this.session
    const epoch = this.conversationEpoch
    return () => session === this.session && epoch === this.conversationEpoch
  }

  private updateConversation(update: (conversation: Conversation) => Conversation): void {
    const conversation = this.state.conversation
    if (!conversation) {
      return
    }
    this.setState({ ...this.state, conversation: update(conversation) })
  }

  private setState(next: ChatState): void {
    this.state = next
    for (const listener of this.listeners) {
      listener()
    }
  }
}

function without(outgoing: Outgoing[], sendId: string): Outgoing[] {
  return outgoing.filter((o) => o.sendId !== sendId)
}

function withStatus(outgoing: Outgoing[], sendId: string, status: Outgoing['status']): Outgoing[] {
  return outgoing.map((o) => (o.sendId === sendId ? { ...o, status } : o))
}
