import { isBetween, mergeMessage, mergeMessages } from './messages'
import type { ChatState, Conversation, FormError, Outgoing, PeerSearch, SessionState, SignedOutNotice, UserProfile } from './model'
import type { AuthApi, CommandResult, Rejected, SessionInfo } from './port/authApi'
import type { ChatApi, SendResult } from './port/chatApi'
import type { EntryLink } from './port/entryLink'
import type { DeliveredMessage, RealtimeChannel, RealtimeConnection } from './port/realtimeChannel'
import type { SendIdSource } from './port/sendIdSource'
import type { Timer } from './port/timer'

/** 送信の応答を待つ時間(仕様「メッセージを送る」)。 */
export const SEND_TIMEOUT_MS = 10_000

export type ChatAppPorts = {
  auth: AuthApi
  api: ChatApi
  realtime: RealtimeChannel
  timer: Timer
  sendIds: SendIdSource
  entry: EntryLink
}

/** リアルタイムの接続の状態。 */
type Link = 'none' | 'connecting' | 'ready' | 'closed'

/** 送信の試行。送り直すたびに、新しくなる。 */
type Attempt = { id: number; cancelTimer: () => void }

const noPeerSearch: PeerSearch = { pending: false, found: null, error: null }

/**
 * クライアントの振る舞いと状態。React と、ブラウザの入出力のAPIを、使わない(ADR-005)。
 * ui は、subscribe と getState を、React の useSyncExternalStore につなぐ。
 *
 * ログイン状態を示す資格は、ブラウザが、Cookie として保つ。このクラスは、触れない(ADR-014)。
 * ログイン状態の識別子と、案内の資格は、状態の外に持ち、ui には、渡さない。
 * 入力欄の内容(連絡先、識別名、表示名、秘密)は、ui の部品が持ち、操作の引数で受ける。状態には、保たない。
 * クライアントは、送信の前に、入力を検証しない。検証と拒否は、サーバーが行う。
 *
 * 振る舞いの規則は、次の通りである。コメントとテストは、この番号(C-1 など)で参照する。
 * C-2〜C-12 の「利用者」は、ログイン中の利用者を指す。
 *
 * - C-1(RL-19、RL-41): ログイン状態の終わりでは、接続を閉じ、会話、相手の特定の結果、入力中の本文、
 *   ログイン状態の識別子を捨て、ログインを求める画面に切り替える。それより前の、接続、履歴の取得、送信、
 *   相手の特定に対して、後から届いた結果は、すべて捨てる。
 * - C-2(ADR-010、WS-22): 会話を開くと、会話の状態を作り直す。接続が ready を受けた後であれば、
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
 *   draft に別の入力がある場合は、その前に、改行で区切って戻す。認証の拒否は、C-14 による。
 * - C-8(WS-17): 送信が届かない場合、完了できなかったことを示す応答の場合、10秒が過ぎた場合に、
 *   その outgoing を failed にする。要求は、取り消さない。
 * - C-9(WS-17、WS-18): resend は、failed の outgoing を、同じ送信の識別子と本文で、sending に戻して、
 *   送信する。10秒を、改めて待ち始める。
 * - C-10(ADR-011): failed にした後に、完了の応答、配信、拒否の応答が届いた場合も、C-6、C-7 を行う。
 *   その outgoing が既にない場合は、応答を捨てる。送り直しの前の試行の、失敗と時間切れは、捨てる。
 * - C-11: 別の会話を開いたとき、それまでの会話の状態(outgoing と draft を含む)を捨てる。
 *   前の会話に対して、後から届いた結果は、捨てる。
 * - C-12(WS-17): 開いている会話と同じ相手を、もう一度開いた場合(同じ相手を特定し直して、openFoundPeer を
 *   呼んだ場合)と、retryHistory では、会話の状態を保つ。history が failed であれば、loading に戻して、
 *   C-2 の通りに取得し直す。そのほかの場合は、何もしない。
 * - C-13(RL-21、RL-23、RL-D9): start は、案内の資格を読み取る。あれば、ログイン状態に依らず、
 *   登録を続ける画面を示す。なければ、ログイン状態を確かめ、ログイン中であれば、利用者と識別子を保持して、
 *   接続を確立する。ログインしていなければ、ログインの画面を示す。確かめられなければ、そのことを示し、
 *   やり直せるようにする(ログインしていないとは、示さない)。
 * - C-14(RL-19、RL-20): どの操作でも、認証の拒否の応答を受けたときと、接続が、ログイン状態が無効に
 *   なったことを示して閉じられたときに、C-1 を行い、ログインし直しが要ることを示す。
 * - C-15(RL-12、RL-13): login は、完了で、利用者と識別子を保持し、接続を確立する。拒否(照合の失敗、上限)と、
 *   完了できなかった場合は、理由を示し、ログインの画面に留まる。
 * - C-16(RL-17、RL-22): logout は、完了で、C-1 を行う。認証の拒否は、C-14 による。完了できなかった場合は、
 *   できなかったことを示し、ログイン中のままとする。
 * - C-17(RL-01、RL-05〜RL-07): requestSignup は、完了で、案内を送ったことを示す。completeSignup は、
 *   完了で、案内の資格を捨て、ログインの画面に、登録を完了したことを示す。拒否では、理由と、正しくない項目を
 *   示し、同じ画面に留まる。資格が無効である拒否でも、同じ画面に留まり、申し込み直しを求める。
 * - C-18(RL-24〜RL-26、RL-31): findPeer は、結果(識別名と表示名)か、拒否の理由を示す。openFoundPeer は、
 *   特定した相手との会話を、C-2 の通りに開く。履歴の取得が拒否された場合は、理由を示す。
 * - C-19: login、requestSignup、completeSignup の結果を待つ間は、showLogin と showSignupRequest は、
 *   何もしない(画面を切り替えない)。切り替えると、サーバーで完了した操作の結果(ログイン、登録の完了)を、
 *   示せなくなるためである。
 */
export class ChatApp {
  private readonly ports: ChatAppPorts
  private state: ChatState
  private readonly listeners = new Set<() => void>()

  /** ログイン状態の識別子(ADR-014)。ログインしていなければ、null。ui には、渡さない。 */
  private sessionId: string | null = null
  /** 案内の資格(確認のための資格)。ui には、渡さない。 */
  private signupCredential: string | null = null

  /**
   * ログイン状態が始まる、または、終わるたびに増える。それより前の結果を、捨てるために使う(C-1)。
   * ログインしていないときの画面の切り替えでは、増やさない。結果を待つ間は、切り替えないためである(C-19)。
   */
  private session = 0
  /** 会話の状態を作り直す、または、捨てるたびに増える。前の会話への結果を、捨てるために使う(C-11)。 */
  private conversationEpoch = 0
  /** 相手の特定を始めるたびに増える。前の特定の結果を、捨てるために使う。 */
  private searchEpoch = 0

  private connection: RealtimeConnection | null = null
  private link: Link = 'none'
  /** 履歴の取得が、ready を待っている(C-2)。 */
  private historyWaiting = false

  /** 送信の識別子ごとの、現在の試行(C-8〜C-10)。 */
  private readonly attempts = new Map<string, Attempt>()
  private lastAttemptId = 0

  constructor(ports: ChatAppPorts) {
    this.ports = ports
    this.state = { session: { kind: 'checking' }, peerSearch: noPeerSearch, conversation: null }
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  readonly getState = (): ChatState => this.state

  // ---- ログイン状態 ----

  /**
   * C-13: 案内の資格があれば、登録を続ける画面を示す。なければ、ログイン状態を確かめる。
   * 確かめられなかった後に、もう一度呼ぶと、確かめ直す。
   */
  readonly start = (): void => {
    const kind = this.state.session.kind
    if (kind !== 'checking' && kind !== 'unknown') {
      return
    }
    const credential = this.ports.entry.takeSignupCredential()
    if (credential !== null) {
      // 案内から登録を続ける訪問者には、ログイン状態に依らず、登録を続ける画面を示す(RL-D9)。
      this.signupCredential = credential
      this.session++
      this.setSession(signedOut('signupComplete'))
      return
    }

    const session = ++this.session
    this.setSession({ kind: 'checking' })
    const failed = (): void => {
      if (session === this.session) {
        this.setSession({ kind: 'unknown' })
      }
    }
    this.ports.auth.currentSession().then((result) => {
      if (session !== this.session) {
        return
      }
      switch (result.kind) {
        case 'signedIn':
          this.beginSession(result.session)
          return
        case 'signedOut':
          this.setSession(signedOut('login'))
          return
        case 'failed':
          failed()
          return
      }
    }, failed)
  }

  /** ログインの画面を示す。登録を続ける画面から移る場合は、案内の資格を捨てる。結果を待つ間は、何もしない(C-19)。 */
  readonly showLogin = (): void => {
    this.showScreen('login')
  }

  /** 登録の申し込みの画面を示す。結果を待つ間は、何もしない(C-19)。 */
  readonly showSignupRequest = (): void => {
    this.showScreen('signupRequest')
  }

  /**
   * C-15: 完了で、利用者と識別子を保持し、接続を確立する。
   * 拒否と、完了できなかった場合は、理由を示し、ログインの画面に留まる。
   */
  readonly login = (contact: string, secret: string): void => {
    const session = this.beginForm('login')
    if (session === null) {
      return
    }
    const failed = (): void => this.endForm(session, failure)
    this.ports.auth.login(contact, secret).then((result) => {
      if (session !== this.session) {
        return
      }
      if (result.kind === 'completed') {
        this.beginSession(result.session)
        return
      }
      this.endForm(session, toFormError(result))
    }, failed)
  }

  /**
   * C-16: 完了で、C-1 を行う。認証の拒否は、C-14 による。
   * 完了できなかった場合は、できなかったことを示し、ログイン中のままとする(RL-22)。
   */
  readonly logout = (): void => {
    const current = this.state.session
    if (current.kind !== 'signedIn' || current.logout === 'pending' || this.sessionId === null) {
      return
    }
    this.setSession({ ...current, logout: 'pending' })

    const session = this.session
    const failed = (): void => {
      const now = this.state.session
      if (session === this.session && now.kind === 'signedIn') {
        this.setSession({ ...now, logout: 'failed' })
      }
    }
    this.ports.auth.logout(this.sessionId).then((result) => {
      if (session !== this.session) {
        return
      }
      if (result.kind === 'completed') {
        this.endSession(null)
        return
      }
      if (result.kind === 'rejected' && result.code === 'unauthenticated') {
        this.endSession('sessionEnded')
        return
      }
      failed()
    }, failed)
  }

  /** C-17: 完了で、案内を送ったことを示す。拒否では、理由と、正しくない項目を示す。 */
  readonly requestSignup = (contact: string): void => {
    const session = this.beginForm('signupRequest')
    if (session === null) {
      return
    }
    this.settleForm(session, this.ports.auth.requestSignup(contact), () => {
      this.setSession({ ...signedOut('signupRequest'), notice: 'signupRequested' })
    })
  }

  /**
   * C-17: 完了で、案内の資格を捨て、ログインの画面に、登録を完了したことを示す。
   * 拒否では、理由と、正しくない項目を示し、同じ画面に留まる。
   */
  readonly completeSignup = (handle: string, displayName: string, secret: string): void => {
    const credential = this.signupCredential
    if (credential === null) {
      return
    }
    const session = this.beginForm('signupComplete')
    if (session === null) {
      return
    }
    this.settleForm(session, this.ports.auth.completeSignup({ credential, handle, displayName, secret }), () => {
      this.signupCredential = null
      this.setSession({ ...signedOut('login'), notice: 'signupCompleted' })
    })
  }

  // ---- 相手の特定と、会話 ----

  /** C-18: 相手を特定し、結果(識別名と表示名)か、拒否の理由を示す。会話は、開かない。 */
  readonly findPeer = (handle: string): void => {
    if (this.state.session.kind !== 'signedIn' || this.sessionId === null) {
      return
    }
    const search = ++this.searchEpoch
    const session = this.session
    const isCurrent = (): boolean => session === this.session && search === this.searchEpoch
    this.setState({ ...this.state, peerSearch: { pending: true, found: null, error: null } })

    const failed = (): void => {
      if (isCurrent()) {
        this.setState({ ...this.state, peerSearch: { pending: false, found: null, error: 'failed' } })
      }
    }
    this.ports.api.findPeer(this.sessionId, handle).then((result) => {
      if (!isCurrent()) {
        return
      }
      switch (result.kind) {
        case 'found':
          this.setState({ ...this.state, peerSearch: { pending: false, found: result.peer, error: null } })
          return
        case 'rejected':
          if (result.code === 'unauthenticated') {
            this.endSession('sessionEnded')
            return
          }
          this.setState({ ...this.state, peerSearch: { pending: false, found: null, error: result.code } })
          return
        case 'failed':
          failed()
          return
      }
    }, failed)
  }

  /**
   * C-18、C-2: 特定した相手との会話を開き、履歴を取得する。
   * C-12: 開いている会話と同じ相手であれば、会話の状態を保つ。履歴を取得できなかったときだけ、取得し直す。
   */
  readonly openFoundPeer = (): void => {
    const peer = this.state.peerSearch.found
    if (this.state.session.kind !== 'signedIn' || !peer) {
      return
    }

    const current = this.state.conversation
    if (current && current.peer.handle === peer.handle) {
      this.retryHistory()
      return
    }

    // C-11: それまでの会話の状態(outgoing と draft を含む)を捨てる。
    this.discardConversation()
    this.setState({
      ...this.state,
      conversation: {
        peer,
        history: 'loading',
        historyRejection: null,
        messages: [],
        outgoing: [],
        draft: '',
        rejection: null,
      },
    })
    this.startHistory()
  }

  /** C-12: 履歴を取得できなかった場合に、取得し直す。そのほかの場合は、何もしない。 */
  readonly retryHistory = (): void => {
    if (this.state.conversation?.history !== 'failed') {
      return
    }
    this.updateConversation((c) => ({ ...c, history: 'loading', historyRejection: null }))
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
    if (this.sessionId === null || !conversation || conversation.draft === '') {
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

  // ---- ログイン状態の始まりと終わり ----

  /** ログイン中の利用者。ログインしていなければ、null。 */
  private currentUser(): UserProfile | null {
    return this.state.session.kind === 'signedIn' ? this.state.session.user : null
  }

  /** C-13、C-15: 利用者と識別子を保持し、接続を確立する。 */
  private beginSession(info: SessionInfo): void {
    this.closeSession()
    this.sessionId = info.sessionId
    this.signupCredential = null
    this.setState({
      session: { kind: 'signedIn', user: info.user, logout: 'idle' },
      peerSearch: noPeerSearch,
      conversation: null,
    })

    const session = this.session
    this.link = 'connecting'
    this.connection = this.ports.realtime.connect(info.sessionId, {
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
      onClosed: (sessionEnded) => {
        if (session !== this.session) {
          return
        }
        if (sessionEnded) {
          // C-14: ログイン状態が無効になった(ログアウト、ログインし直し、有効期限)。
          this.endSession('sessionEnded')
          return
        }
        this.link = 'closed'
        // C-2: 待つ間に、接続の確立に失敗した場合と、接続が切れた場合は、その時点で取得する。
        this.fetchHistoryIfWaiting()
      },
    })
  }

  /**
   * C-1: 接続を閉じ、会話、相手の特定の結果、入力中の本文、ログイン状態の識別子を捨て、
   * ログインを求める画面に切り替える。notice は、併せて示す知らせである。
   */
  private endSession(notice: SignedOutNotice | null): void {
    this.closeSession()
    this.setState({
      session: { ...signedOut('login'), notice },
      peerSearch: noPeerSearch,
      conversation: null,
    })
  }

  /** それまでの接続を閉じ、保持していたものを捨てる。それより前の結果は、以後、捨てられる。 */
  private closeSession(): void {
    this.session++
    this.searchEpoch++
    this.sessionId = null
    this.connection?.close()
    this.connection = null
    this.link = 'none'
    this.discardConversation()
  }

  // ---- ログインしていないときの画面 ----

  private showScreen(screen: 'login' | 'signupRequest'): void {
    const current = this.state.session
    // C-19: 結果を待つ間は、切り替えない。
    if (current.kind !== 'signedOut' || current.pending) {
      return
    }
    // 案内の資格は、登録を続ける画面を離れると、捨てる。
    this.signupCredential = null
    this.setSession(signedOut(screen))
  }

  /**
   * 画面の操作を始める。結果を待つ状態にし、それまでの理由と知らせを消す。
   * 指定した画面を示していない場合と、結果を待っている場合は、始めずに、null を返す。
   */
  private beginForm(screen: 'login' | 'signupRequest' | 'signupComplete'): number | null {
    const current = this.state.session
    if (current.kind !== 'signedOut' || current.screen !== screen || current.pending) {
      return null
    }
    this.setSession({ ...current, pending: true, error: null, notice: null })
    return this.session
  }

  /** 画面の操作を、拒否か、完了できなかったこととして終える。同じ画面に留まる。 */
  private endForm(session: number, error: FormError): void {
    const current = this.state.session
    if (session !== this.session || current.kind !== 'signedOut') {
      return
    }
    this.setSession({ ...current, pending: false, error })
  }

  private settleForm(session: number, result: Promise<CommandResult>, onCompleted: () => void): void {
    result.then(
      (r) => {
        if (session !== this.session) {
          return
        }
        if (r.kind === 'completed') {
          onCompleted()
          return
        }
        this.endForm(session, toFormError(r))
      },
      () => this.endForm(session, failure),
    )
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
    const sessionId = this.sessionId
    const conversation = this.state.conversation
    if (sessionId === null || !conversation) {
      return
    }

    const isCurrent = this.currentChecker()
    const failed = (): void => {
      if (isCurrent()) {
        this.updateConversation((c) => ({ ...c, history: 'failed', historyRejection: null }))
      }
    }
    this.ports.api.listMessages(sessionId, conversation.peer.handle).then((result) => {
      if (!isCurrent()) {
        return
      }
      if (result.kind === 'rejected') {
        if (result.code === 'unauthenticated') {
          // C-14: 認証の拒否。
          this.endSession('sessionEnded')
          return
        }
        // C-18: 履歴の取得が拒否された場合は、理由を示す。
        this.updateConversation((c) => ({ ...c, history: 'failed', historyRejection: result.code }))
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
    const self = this.currentUser()
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
    const sessionId = this.sessionId
    const conversation = this.state.conversation
    if (sessionId === null || !conversation) {
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
      .sendMessage({ sessionId, peer: conversation.peer.handle, sendId, body })
      .then(settle, () => settle({ kind: 'failed' }))
  }

  private settleSend(sendId: string, attemptId: number, result: SendResult): void {
    // C-14: 認証の拒否は、ほかの拒否と異なり、本文を入力欄に戻さない。入力中の本文も、保たない(RL-19)。
    if (result.kind === 'rejected' && result.code === 'unauthenticated') {
      this.endSession('sessionEnded')
      return
    }
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

  /** 呼んだ時点のログイン状態と会話が、今も同じであるかを返す関数を作る(C-1、C-11)。 */
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

  private setSession(session: SessionState): void {
    this.setState({ ...this.state, session })
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

function signedOut(screen: 'login' | 'signupRequest' | 'signupComplete'): Extract<SessionState, { kind: 'signedOut' }> {
  return { kind: 'signedOut', screen, pending: false, error: null, notice: null }
}

/** 完了できなかったこと(拒否ではない)。 */
const failure: FormError = { code: 'failed', fields: [] }

function toFormError(result: Rejected | { kind: 'failed' }): FormError {
  return result.kind === 'rejected' ? { code: result.code, fields: result.fields } : failure
}
