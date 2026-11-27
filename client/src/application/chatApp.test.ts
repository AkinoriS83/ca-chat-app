import { describe, expect, it } from 'vitest'

import { ChatApp, SEND_TIMEOUT_MS } from './chatApp'
import type { ChatState, Conversation, FormError, SessionState } from './model'
import type { LoginResult } from './port/authApi'
import {
  FakeAuthApi,
  FakeChatApi,
  FakeEntryLink,
  FakeRealtimeChannel,
  FakeSendIdSource,
  FakeTimer,
  alice,
  bob,
  carol,
  delivery,
  message,
  sessionOf,
  settled,
} from './testPorts'

// テストの名前の C-1 などは、振る舞いの規則の番号である(chatApp.ts の先頭)。

const users = { alice, bob, carol }
type Name = keyof typeof users

function setup(signupCredential: string | null = null) {
  const auth = new FakeAuthApi()
  const api = new FakeChatApi()
  const realtime = new FakeRealtimeChannel()
  const timer = new FakeTimer()
  const entry = new FakeEntryLink(signupCredential)
  const app = new ChatApp({ auth, api, realtime, timer, sendIds: new FakeSendIdSource(), entry })

  const conversation = (): Conversation => {
    const current = app.getState().conversation
    if (!current) {
      throw new Error('会話を、開いていない')
    }
    return current
  }
  const seqs = (): number[] => conversation().messages.map((m) => m.seq)
  const session = (): SessionState => app.getState().session

  context = { app, auth, api, realtime, timer, entry, conversation, seqs, session }
  return context
}

/** 最後に setup が作ったもの。下の補助が使う。 */
let context: {
  app: ChatApp
  auth: FakeAuthApi
  api: FakeChatApi
  realtime: FakeRealtimeChannel
  timer: FakeTimer
  entry: FakeEntryLink
  conversation: () => Conversation
  seqs: () => number[]
  session: () => SessionState
}

/** ログインしていない状態(ログインの画面)にする。 */
async function signedOutAtLogin(): Promise<void> {
  context.app.start()
  context.auth.sessionCalls.at(-1)?.resolve({ kind: 'signedOut' })
  await settled()
}

/** 利用者として、ログインした状態にする。ログイン中であれば、ログアウトしてから、ログインする。 */
async function signIn(name: Name): Promise<void> {
  const { app, auth } = context
  if (app.getState().session.kind === 'checking') {
    await signedOutAtLogin()
  }
  if (app.getState().session.kind === 'signedIn') {
    app.logout()
    auth.logoutCalls.at(-1)?.resolve({ kind: 'completed' })
    await settled()
  }
  app.login(`${name}@example.com`, `${name}-secret`)
  auth.loginCalls.at(-1)?.resolve({ kind: 'completed', session: sessionOf(users[name]) })
  await settled()
}

/** 相手を特定して、会話を開く。 */
async function openPeer(name: Name): Promise<void> {
  const { app, api } = context
  app.findPeer(name)
  api.peerCalls.at(-1)?.resolve({ kind: 'found', peer: users[name] })
  await settled()
  app.openFoundPeer()
}

/** alice として、bob との会話を開き、履歴(空)を取得し終えた状態にする。 */
async function setupOpenConversation() {
  const created = setup()
  await signIn('alice')
  created.realtime.latest.listener.onReady()
  await openPeer('bob')
  created.api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [] })
  await settled()
  return created
}

const signedOutLogin: ChatState = {
  session: { kind: 'signedOut', screen: 'login', pending: false, error: null, notice: null },
  peerSearch: { pending: false, found: null, error: null },
  conversation: null,
}

describe('アプリを開く(C-13、RL-21、RL-23)', () => {
  it('最初は、ログイン状態を確かめている。ログインしているとも、していないとも、示さない', () => {
    const { app, auth, realtime } = setup()

    expect(app.getState().session).toEqual({ kind: 'checking' })
    expect(auth.sessionCalls).toHaveLength(0)

    app.start()

    expect(app.getState().session).toEqual({ kind: 'checking' })
    expect(auth.sessionCalls).toHaveLength(1)
    expect(realtime.connections).toHaveLength(0)
  })

  it('ログイン中であれば、利用者を示し、ログイン状態の識別子で、接続を確立する(再読み込みの後)', async () => {
    const { app, auth, realtime, session } = setup()

    app.start()
    auth.sessionCalls[0]?.resolve({ kind: 'signedIn', session: sessionOf(alice) })
    await settled()

    expect(session()).toEqual({ kind: 'signedIn', user: alice, logout: 'idle' })
    expect(realtime.connections.map((c) => c.sessionId)).toEqual(['session-alice'])
  })

  it('ログインしていなければ、ログインの画面を示す。接続は、確立しない', async () => {
    const { app, auth, realtime } = setup()

    app.start()
    auth.sessionCalls[0]?.resolve({ kind: 'signedOut' })
    await settled()

    expect(app.getState()).toEqual(signedOutLogin)
    expect(realtime.connections).toHaveLength(0)
  })

  it('確かめられなければ、そのことを示す。ログインしていないとは、示さない。やり直せる', async () => {
    const { app, auth, session } = setup()

    app.start()
    auth.sessionCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(session()).toEqual({ kind: 'unknown' })

    app.start()
    expect(session()).toEqual({ kind: 'checking' })
    auth.sessionCalls[1]?.reject(new Error('network'))
    await settled()
    expect(session()).toEqual({ kind: 'unknown' })

    app.start()
    auth.sessionCalls[2]?.resolve({ kind: 'signedIn', session: sessionOf(alice) })
    await settled()
    expect(session()).toEqual({ kind: 'signedIn', user: alice, logout: 'idle' })
  })

  it('案内の資格があれば、ログイン状態を確かめずに、登録を続ける画面を示す(DD-34)', () => {
    const { app, auth, entry, session } = setup('signup-credential')

    app.start()

    expect(session()).toEqual({ kind: 'signedOut', screen: 'signupComplete', pending: false, error: null, notice: null })
    expect(auth.sessionCalls).toHaveLength(0)
    // 資格は、アドレスから除かれている。状態には、現れない。
    expect(entry.credential).toBeNull()
    expect(JSON.stringify(app.getState())).not.toContain('signup-credential')
  })

  it('ログインの画面か、ログイン中に、start を呼んでも、何もしない', async () => {
    const { app, auth, entry } = setup()
    await signIn('alice')
    const before = app.getState()

    app.start()

    expect(app.getState()).toBe(before)
    expect(auth.sessionCalls).toHaveLength(1)
    expect(entry.taken).toBe(1)
  })
})

describe('ログイン(C-15、RL-12、RL-13)', () => {
  it('完了で、利用者と識別子を保持し、接続を確立する。入力は、検証せずに、そのまま渡す', async () => {
    const { app, auth, realtime, session } = setup()
    await signedOutAtLogin()

    app.login('  Alice@Example.com ', ' x ')

    expect(session()).toMatchObject({ kind: 'signedOut', screen: 'login', pending: true, error: null })
    expect(auth.loginCalls[0]?.request).toEqual({ contact: '  Alice@Example.com ', secret: ' x ' })

    auth.loginCalls[0]?.resolve({ kind: 'completed', session: sessionOf(alice) })
    await settled()

    expect(session()).toEqual({ kind: 'signedIn', user: alice, logout: 'idle' })
    expect(realtime.connections.map((c) => c.sessionId)).toEqual(['session-alice'])
    // 秘密と、連絡先は、状態に保たない。
    expect(JSON.stringify(app.getState())).not.toContain('Alice@Example.com')
  })

  const loginFailures: [string, LoginResult, FormError][] = [
    ['照合の失敗', { kind: 'rejected', code: 'login_failed', fields: [] }, { code: 'login_failed', fields: [] }],
    ['上限', { kind: 'rejected', code: 'rate_limited', fields: [] }, { code: 'rate_limited', fields: [] }],
    ['完了できなかった', { kind: 'failed' }, { code: 'failed', fields: [] }],
  ]
  it.each(loginFailures)('%s では、理由を示し、ログインの画面に留まる', async (_name, result, error) => {
    const { app, auth, realtime, session } = setup()
    await signedOutAtLogin()

    app.login('alice@example.com', 'wrong')
    auth.loginCalls[0]?.resolve(result)
    await settled()

    expect(session()).toEqual({ kind: 'signedOut', screen: 'login', pending: false, error, notice: null })
    expect(realtime.connections).toHaveLength(0)
  })

  it('要求が失敗した場合も、完了できなかったことを示す。やり直すと、理由は消える', async () => {
    const { app, auth, session } = setup()
    await signedOutAtLogin()

    app.login('alice@example.com', 'secret')
    auth.loginCalls[0]?.reject(new Error('network'))
    await settled()
    expect(session()).toMatchObject({ pending: false, error: { code: 'failed' } })

    app.login('alice@example.com', 'secret')
    expect(session()).toMatchObject({ pending: true, error: null })
  })

  it('結果を待っている間は、もう一度、送らない', async () => {
    const { app, auth } = setup()
    await signedOutAtLogin()

    app.login('alice@example.com', 'secret')
    app.login('alice@example.com', 'secret')

    expect(auth.loginCalls).toHaveLength(1)
  })

  it('結果を待つ間は、画面を切り替えない。その間に完了したログインを、捨てない(C-19)', async () => {
    const { app, auth, session } = setup()
    await signedOutAtLogin()

    app.login('alice@example.com', 'secret')
    app.showSignupRequest()
    expect(session()).toMatchObject({ screen: 'login', pending: true })

    auth.loginCalls[0]?.resolve({ kind: 'completed', session: sessionOf(alice) })
    await settled()
    expect(session()).toEqual({ kind: 'signedIn', user: alice, logout: 'idle' })
  })

  it('ログインの画面を示していなければ、ログインしない', () => {
    const { app, auth } = setup()

    app.login('alice@example.com', 'secret')

    expect(auth.loginCalls).toHaveLength(0)
  })
})

describe('登録(C-17、RL-01、RL-05〜RL-07)', () => {
  it('申し込みの完了で、案内を送ったことを示す', async () => {
    const { app, auth, session } = setup()
    await signedOutAtLogin()

    app.showSignupRequest()
    expect(session()).toEqual({ kind: 'signedOut', screen: 'signupRequest', pending: false, error: null, notice: null })

    app.requestSignup('dave@example.com')
    expect(session()).toMatchObject({ screen: 'signupRequest', pending: true })
    expect(auth.signupCalls[0]?.request).toEqual({ contact: 'dave@example.com' })

    auth.signupCalls[0]?.resolve({ kind: 'completed' })
    await settled()

    expect(session()).toEqual({
      kind: 'signedOut',
      screen: 'signupRequest',
      pending: false,
      error: null,
      notice: 'signupRequested',
    })
  })

  it('申し込みの拒否と、完了できなかった場合は、理由を示し、案内を送ったとは示さない(RL-10)', async () => {
    const { app, auth, session } = setup()
    await signedOutAtLogin()
    app.showSignupRequest()

    app.requestSignup('not-a-contact')
    auth.signupCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_fields', fields: ['contact'] })
    await settled()
    expect(session()).toMatchObject({ screen: 'signupRequest', error: { code: 'invalid_fields', fields: ['contact'] }, notice: null })

    app.requestSignup('dave@example.com')
    auth.signupCalls[1]?.resolve({ kind: 'failed' })
    await settled()
    expect(session()).toMatchObject({ screen: 'signupRequest', error: { code: 'failed', fields: [] }, notice: null })

    app.requestSignup('dave@example.com')
    auth.signupCalls[2]?.resolve({ kind: 'rejected', code: 'rate_limited', fields: [] })
    await settled()
    expect(session()).toMatchObject({ error: { code: 'rate_limited' }, notice: null })
  })

  it('登録の完了は、案内の資格とともに送る。完了で、資格を捨て、ログインの画面に、完了したことを示す', async () => {
    const { app, auth, session } = setup('signup-credential')
    app.start()

    app.completeSignup('Dave_01', 'Dave', 'dave-secret')

    expect(auth.completeCalls[0]?.request).toEqual({
      credential: 'signup-credential',
      handle: 'Dave_01',
      displayName: 'Dave',
      secret: 'dave-secret',
    })
    auth.completeCalls[0]?.resolve({ kind: 'completed' })
    await settled()

    expect(session()).toEqual({ kind: 'signedOut', screen: 'login', pending: false, error: null, notice: 'signupCompleted' })
    // 資格は、捨てている。もう一度は、送らない。
    app.completeSignup('Dave_01', 'Dave', 'dave-secret')
    expect(auth.completeCalls).toHaveLength(1)
  })

  it('登録の完了の拒否では、理由と、正しくない項目を示し、同じ画面に留まる。同じ資格で、やり直せる', async () => {
    const { app, auth, session } = setup('signup-credential')
    app.start()

    app.completeSignup('!', '', 'x')
    auth.completeCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_fields', fields: ['handle', 'displayName', 'secret'] })
    await settled()
    expect(session()).toEqual({
      kind: 'signedOut',
      screen: 'signupComplete',
      pending: false,
      error: { code: 'invalid_fields', fields: ['handle', 'displayName', 'secret'] },
      notice: null,
    })

    app.completeSignup('alice', 'Dave', 'dave-secret')
    auth.completeCalls[1]?.resolve({ kind: 'rejected', code: 'handle_taken', fields: [] })
    await settled()
    expect(session()).toMatchObject({ screen: 'signupComplete', error: { code: 'handle_taken' } })

    app.completeSignup('Dave_01', 'Dave', 'dave-secret')
    expect(auth.completeCalls[2]?.request.credential).toBe('signup-credential')
  })

  it('資格が無効である拒否でも、同じ画面に留まり、理由を示す(申し込み直しを求める)', async () => {
    const { app, auth, session } = setup('signup-credential')
    app.start()

    app.completeSignup('Dave_01', 'Dave', 'dave-secret')
    auth.completeCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_signup_credential', fields: [] })
    await settled()

    expect(session()).toMatchObject({ screen: 'signupComplete', pending: false, error: { code: 'invalid_signup_credential' } })

    // 申し込み直しの画面へ移ると、資格を捨てる。
    app.showSignupRequest()
    expect(session()).toMatchObject({ screen: 'signupRequest', error: null })
    app.completeSignup('Dave_01', 'Dave', 'dave-secret')
    expect(auth.completeCalls).toHaveLength(1)
  })

  it('申し込みの結果を待つ間は、画面を切り替えない。結果は、捨てずに示す(C-19)', async () => {
    const { app, auth, session } = setup()
    await signedOutAtLogin()
    app.showSignupRequest()
    app.requestSignup('dave@example.com')

    app.showLogin()
    expect(session()).toMatchObject({ screen: 'signupRequest', pending: true })

    auth.signupCalls[0]?.resolve({ kind: 'completed' })
    await settled()
    expect(session()).toMatchObject({ screen: 'signupRequest', pending: false, notice: 'signupRequested' })

    // 結果を受けた後は、切り替えられる。
    app.showLogin()
    expect(session()).toEqual({ kind: 'signedOut', screen: 'login', pending: false, error: null, notice: null })
  })

  it('登録の完了の結果を待つ間は、画面を切り替えず、案内の資格も捨てない(C-19)', async () => {
    const { app, auth, session } = setup('signup-credential')
    app.start()
    app.completeSignup('Dave_01', 'Dave', 'dave-secret')

    app.showLogin()
    app.showSignupRequest()
    expect(session()).toMatchObject({ screen: 'signupComplete', pending: true })

    // 拒否の後、同じ資格で、やり直せる。
    auth.completeCalls[0]?.resolve({ kind: 'rejected', code: 'handle_taken', fields: [] })
    await settled()
    app.completeSignup('Dave_02', 'Dave', 'dave-secret')
    expect(auth.completeCalls[1]?.request.credential).toBe('signup-credential')

    // 完了は、切り替えを試みた後でも、ログインの画面に、完了したこととして示す。
    app.showSignupRequest()
    auth.completeCalls[1]?.resolve({ kind: 'completed' })
    await settled()
    expect(session()).toEqual({ kind: 'signedOut', screen: 'login', pending: false, error: null, notice: 'signupCompleted' })
  })

  it('ログイン中は、画面を切り替えない', async () => {
    const { app, session } = setup()
    await signIn('alice')

    app.showSignupRequest()
    app.showLogin()

    expect(session()).toMatchObject({ kind: 'signedIn' })
  })
})

describe('ログアウト(C-16、RL-17、RL-22)', () => {
  it('完了で、接続を閉じ、会話と、相手の特定の結果と、入力中の本文を捨てて、ログインの画面に切り替える(C-1)', async () => {
    const { app, auth, realtime, session } = await setupOpenConversation()
    app.setDraft('typing')

    app.logout()

    expect(session()).toEqual({ kind: 'signedIn', user: alice, logout: 'pending' })
    expect(auth.logoutCalls[0]?.request).toEqual({ sessionId: 'session-alice' })
    // 完了するまでは、ログアウトしたものとして、示さない。
    expect(app.getState().conversation).not.toBeNull()

    auth.logoutCalls[0]?.resolve({ kind: 'completed' })
    await settled()

    expect(app.getState()).toEqual(signedOutLogin)
    expect(realtime.latest.closed).toBe(true)
  })

  it('完了できなかった場合は、できなかったことを示し、ログイン中のままとする(RL-22)', async () => {
    const { app, auth, realtime, session, conversation } = await setupOpenConversation()
    app.setDraft('typing')

    app.logout()
    auth.logoutCalls[0]?.resolve({ kind: 'failed' })
    await settled()

    expect(session()).toEqual({ kind: 'signedIn', user: alice, logout: 'failed' })
    expect(conversation().draft).toBe('typing')
    expect(realtime.latest.closed).toBe(false)

    // やり直せる。要求が失敗した場合も、同じである。
    app.logout()
    expect(session()).toMatchObject({ logout: 'pending' })
    auth.logoutCalls[1]?.reject(new Error('network'))
    await settled()
    expect(session()).toMatchObject({ kind: 'signedIn', logout: 'failed' })
  })

  it('認証の拒否では、ログインし直しが要ることを示して、ログインの画面に切り替える(C-14)', async () => {
    const { app, auth, session } = await setupOpenConversation()

    app.logout()
    auth.logoutCalls[0]?.resolve({ kind: 'rejected', code: 'unauthenticated', fields: [] })
    await settled()

    expect(session()).toEqual({ kind: 'signedOut', screen: 'login', pending: false, error: null, notice: 'sessionEnded' })
    expect(app.getState().conversation).toBeNull()
  })

  it('結果を待っている間は、もう一度、送らない。ログインしていなければ、何もしない', async () => {
    const { app, auth } = setup()
    app.logout()
    expect(auth.logoutCalls).toHaveLength(0)

    await signIn('alice')
    app.logout()
    app.logout()
    expect(auth.logoutCalls).toHaveLength(1)
  })
})

describe('ログイン状態が無効になった(C-1、C-14、RL-19、RL-20)', () => {
  const ended: ChatState = {
    session: { kind: 'signedOut', screen: 'login', pending: false, error: null, notice: 'sessionEnded' },
    peerSearch: { pending: false, found: null, error: null },
    conversation: null,
  }

  it('接続が、ログイン状態が無効になったことを示して閉じられると、操作なしに、ログインの画面に切り替える', async () => {
    const { app, realtime, conversation } = await setupOpenConversation()
    app.setDraft('typing')
    expect(conversation().peer).toEqual(bob)

    realtime.latest.listener.onClosed(true)

    // 会話の内容、利用者の情報、入力中の本文を、示し続けない。
    expect(app.getState()).toEqual(ended)
    expect(realtime.latest.closed).toBe(true)
  })

  it('そのほかの理由で、接続が切れた場合は、ログイン中のままとする', async () => {
    const { realtime, session, conversation } = await setupOpenConversation()

    realtime.latest.listener.onClosed(false)

    expect(session()).toMatchObject({ kind: 'signedIn' })
    expect(conversation().peer).toEqual(bob)
  })

  it('履歴の取得が、認証の拒否を受けると、ログインの画面に切り替える', async () => {
    const { app, api, realtime } = setup()
    await signIn('alice')
    realtime.latest.listener.onClosed(false)
    await openPeer('bob')

    api.historyCalls[0]?.resolve({ kind: 'rejected', code: 'unauthenticated' })
    await settled()

    expect(app.getState()).toEqual(ended)
  })

  it('送信が、認証の拒否を受けると、ログインの画面に切り替える。本文は、入力欄に戻さない', async () => {
    const { app, api, timer } = await setupOpenConversation()
    app.setDraft('typed while showing alice')
    app.send()

    api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'unauthenticated' })
    await settled()

    expect(app.getState()).toEqual(ended)
    expect(timer.pending).toBe(0)
    // 識別子を捨てている。以後、送信しない。
    app.setDraft('again')
    app.send()
    expect(api.sendCalls).toHaveLength(1)
  })

  it('相手の特定が、認証の拒否を受けると、ログインの画面に切り替える', async () => {
    const { app, api } = await setupOpenConversation()

    app.findPeer('carol')
    api.peerCalls[1]?.resolve({ kind: 'rejected', code: 'unauthenticated' })
    await settled()

    expect(app.getState()).toEqual(ended)
  })

  it('切り替えた後に届いた、前のログイン状態の、接続、履歴の取得、送信、相手の特定の結果は、捨てる', async () => {
    const { app, api, realtime, timer } = setup()
    await signIn('alice')
    const asAlice = realtime.latest
    asAlice.listener.onReady()
    await openPeer('bob')
    app.findPeer('carol')
    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [] })
    await settled()
    app.setDraft('hello')
    app.send()

    asAlice.listener.onClosed(true)
    asAlice.listener.onMessage(delivery([alice, bob], message(1, bob)))
    asAlice.listener.onReady()
    asAlice.listener.onClosed(false)
    api.peerCalls[1]?.resolve({ kind: 'found', peer: carol })
    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(2, alice, 'hello') })
    timer.advance(SEND_TIMEOUT_MS)
    await settled()

    expect(app.getState()).toEqual(ended)
  })

  it('別の利用者としてログインし直した後、前の利用者の会話を示さず、前の接続の配信を捨てる(RL-20)', async () => {
    const { app, api, realtime, session, conversation, seqs } = await setupOpenConversation()
    const asAlice = realtime.latest
    asAlice.listener.onClosed(true)

    await signIn('carol')

    expect(session()).toEqual({ kind: 'signedIn', user: carol, logout: 'idle' })
    expect(app.getState().conversation).toBeNull()
    expect(realtime.connections.map((c) => c.sessionId)).toEqual(['session-alice', 'session-carol'])

    await openPeer('bob')
    asAlice.listener.onMessage(delivery([alice, bob], message(1, bob)))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(2, bob)))
    expect(seqs()).toEqual([])
    expect(conversation().peer).toEqual(bob)
    // 操作には、新しいログイン状態の識別子を添える。
    expect(api.peerCalls.at(-1)?.request.sessionId).toBe('session-carol')
  })
})

describe('相手の特定(C-18、RL-24〜RL-26、RL-31)', () => {
  it('結果(識別名と表示名)を示す。会話は、開かない。入力は、検証せずに、そのまま渡す', async () => {
    const { app, api } = setup()
    await signIn('alice')

    app.findPeer(' BOB ')

    expect(app.getState().peerSearch).toEqual({ pending: true, found: null, error: null })
    expect(api.peerCalls[0]?.request).toEqual({ sessionId: 'session-alice', handle: ' BOB ' })

    api.peerCalls[0]?.resolve({ kind: 'found', peer: bob })
    await settled()

    expect(app.getState().peerSearch).toEqual({ pending: false, found: bob, error: null })
    expect(app.getState().conversation).toBeNull()
    expect(api.historyCalls).toHaveLength(0)
  })

  it.each([
    ['見つからない', { kind: 'rejected', code: 'not_found' }, 'not_found'],
    ['自分自身', { kind: 'rejected', code: 'invalid_peer' }, 'invalid_peer'],
    ['入力が正しくない', { kind: 'rejected', code: 'invalid_input' }, 'invalid_input'],
    ['上限', { kind: 'rejected', code: 'rate_limited' }, 'rate_limited'],
    ['完了できなかった', { kind: 'failed' }, 'failed'],
  ] as const)('%s 場合は、理由を示す', async (_name, result, error) => {
    const { app, api } = setup()
    await signIn('alice')

    app.findPeer('someone')
    api.peerCalls[0]?.resolve(result)
    await settled()

    expect(app.getState().peerSearch).toEqual({ pending: false, found: null, error })
  })

  it('要求が失敗した場合も、完了できなかったことを示す', async () => {
    const { app, api } = setup()
    await signIn('alice')

    app.findPeer('bob')
    api.peerCalls[0]?.reject(new Error('network'))
    await settled()

    expect(app.getState().peerSearch).toEqual({ pending: false, found: null, error: 'failed' })
  })

  it('特定し直すと、前の特定の結果は、捨てる', async () => {
    const { app, api } = setup()
    await signIn('alice')

    app.findPeer('bob')
    app.findPeer('carol')
    api.peerCalls[0]?.resolve({ kind: 'found', peer: bob })
    await settled()
    expect(app.getState().peerSearch).toEqual({ pending: true, found: null, error: null })

    api.peerCalls[1]?.resolve({ kind: 'found', peer: carol })
    await settled()
    expect(app.getState().peerSearch.found).toEqual(carol)
  })

  it('特定の拒否は、開いている会話を変えない', async () => {
    const { app, api, conversation } = await setupOpenConversation()

    app.findPeer('nobody')
    api.peerCalls[1]?.resolve({ kind: 'rejected', code: 'not_found' })
    await settled()
    app.openFoundPeer()

    expect(conversation().peer).toEqual(bob)
    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴の取得が拒否された場合は、理由を示す。取得し直せる', async () => {
    const { app, api, realtime, conversation } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')

    api.historyCalls[0]?.resolve({ kind: 'rejected', code: 'rate_limited' })
    await settled()
    expect(conversation()).toMatchObject({ history: 'failed', historyRejection: 'rate_limited' })

    app.retryHistory()
    expect(conversation()).toMatchObject({ history: 'loading', historyRejection: null })
    api.historyCalls[1]?.resolve({ kind: 'failed' })
    await settled()
    expect(conversation()).toMatchObject({ history: 'failed', historyRejection: null })
  })
})

describe('会話を開く(C-2、ADR-010)', () => {
  it('ログインしていなければ、特定も、会話を開くことも、行わない。特定していなければ、開かない', async () => {
    const { app, api } = setup()

    app.findPeer('bob')
    app.openFoundPeer()
    expect(api.peerCalls).toHaveLength(0)
    expect(app.getState().conversation).toBeNull()

    await signIn('alice')
    app.openFoundPeer()
    expect(app.getState().conversation).toBeNull()
  })

  it('ready を受けた後であれば、すぐに履歴を取得する', async () => {
    const { api, realtime, conversation } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()

    await openPeer('bob')

    expect(conversation()).toEqual({
      peer: bob,
      history: 'loading',
      historyRejection: null,
      messages: [],
      outgoing: [],
      draft: '',
      rejection: null,
    })
    expect(api.historyCalls.map((c) => c.request)).toEqual([{ sessionId: 'session-alice', peer: 'bob' }])
  })

  it('接続を確立している途中であれば、ready を待ってから、履歴を取得する', async () => {
    const { api, realtime, conversation } = setup()
    await signIn('alice')

    await openPeer('bob')

    expect(conversation().history).toBe('loading')
    expect(api.historyCalls).toHaveLength(0)

    realtime.latest.listener.onReady()

    expect(api.historyCalls).toHaveLength(1)
  })

  it('ready を待つ間に、接続の確立に失敗した場合は、その時点で取得する', async () => {
    const { api, realtime } = setup()
    await signIn('alice')
    await openPeer('bob')

    realtime.latest.listener.onClosed(false)

    expect(api.historyCalls).toHaveLength(1)
  })

  it('接続が切れている場合は、待たずに取得する', async () => {
    const { api, realtime } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    realtime.latest.listener.onClosed(false)

    await openPeer('bob')

    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴を取得すると、連番の順に示す(WS-20)。メッセージがなければ、空のままである(WS-21)', async () => {
    const { api, realtime, conversation, seqs } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')

    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, alice), message(2, bob), message(3, alice)] })
    await settled()

    expect(conversation().history).toBe('loaded')
    expect(seqs()).toEqual([1, 2, 3])

    await openPeer('carol')
    api.historyCalls[1]?.resolve({ kind: 'loaded', messages: [] })
    await settled()

    expect(conversation().history).toBe('loaded')
    expect(seqs()).toEqual([])
  })

  it('履歴を取得できなかった場合は、failed にする', async () => {
    const first = setup()
    await signIn('alice')
    first.realtime.latest.listener.onReady()
    await openPeer('bob')
    first.api.historyCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(first.conversation().history).toBe('failed')

    const second = setup()
    await signIn('alice')
    second.realtime.latest.listener.onReady()
    await openPeer('bob')
    second.api.historyCalls[0]?.reject(new Error('network'))
    await settled()
    expect(second.conversation().history).toBe('failed')
  })
})

describe('履歴と配信の統合(C-3、C-4、WS-22)', () => {
  it('開いている会話の配信を、操作なしに加える', async () => {
    const { realtime, seqs } = await setupOpenConversation()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(2, alice)))

    expect(seqs()).toEqual([1, 2])
  })

  it('participants が、自分と、開いている会話の相手の組に一致しない配信は、捨てる', async () => {
    const { realtime, seqs } = await setupOpenConversation()

    realtime.latest.listener.onMessage(delivery([alice, carol], message(1, carol)))
    realtime.latest.listener.onMessage(delivery([bob, carol], message(1, carol)))

    expect(seqs()).toEqual([])
  })

  it('会話を開いていないときの配信は、捨てる', async () => {
    const { app, realtime } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))
    await openPeer('bob')

    expect(app.getState().conversation?.messages).toEqual([])
  })

  it('履歴の取得中に届いた配信を、すぐに加え、履歴と統合する。欠けず、重複しない', async () => {
    const { api, realtime, seqs } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')

    // 履歴の読み取りと前後して、連番3と4が保管された。3は、履歴にも、配信にも現れる。
    realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob)))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(4, bob)))
    expect(seqs()).toEqual([3, 4])

    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, alice), message(2, bob), message(3, bob)] })
    await settled()

    expect(seqs()).toEqual([1, 2, 3, 4])
  })

  it('履歴の後に、同じメッセージが配信で届いても、重複しない', async () => {
    const { api, realtime, seqs } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')
    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, alice), message(2, bob)] })
    await settled()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(2, bob)))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob)))

    expect(seqs()).toEqual([1, 2, 3])
  })

  it('届く順を入れ替えても、連番の順に示す(WS-07)', async () => {
    const { realtime, seqs } = await setupOpenConversation()

    for (const seq of [3, 1, 4, 2]) {
      realtime.latest.listener.onMessage(delivery([alice, bob], message(seq, bob)))
    }

    expect(seqs()).toEqual([1, 2, 3, 4])
  })
})

describe('送信(C-5)', () => {
  it('draft が空の文字列であれば、何もしない', async () => {
    const { app, api, conversation } = await setupOpenConversation()

    app.send()

    expect(api.sendCalls).toHaveLength(0)
    expect(conversation().outgoing).toEqual([])
  })

  it('送信の識別子を作り、sending として加え、draft と rejection を空にして、送信する', async () => {
    const { app, api, timer, conversation } = await setupOpenConversation()
    app.setDraft('  こんにちは\n')

    app.send()

    // クライアントは、本文を検証せず、整えない。入力の通りに送る。
    expect(api.sendCalls.map((c) => c.request)).toEqual([
      { sessionId: 'session-alice', peer: 'bob', sendId: 'send-1', body: '  こんにちは\n' },
    ])
    expect(conversation().outgoing).toEqual([{ sendId: 'send-1', body: '  こんにちは\n', status: 'sending' }])
    expect(conversation().draft).toBe('')
    expect(timer.pending).toBe(1)
  })

  it('空白文字だけの draft も、送信する(検証は、サーバーが行う)', async () => {
    const { app, api } = await setupOpenConversation()
    app.setDraft('   ')

    app.send()

    expect(api.sendCalls).toHaveLength(1)
  })

  it('送信のたびに、新しい送信の識別子を添える', async () => {
    const { app, api } = await setupOpenConversation()

    app.setDraft('one')
    app.send()
    app.setDraft('two')
    app.send()

    expect(api.sendCalls.map((c) => c.request.sendId)).toEqual(['send-1', 'send-2'])
  })
})

describe('完了(C-6、WS-37)', () => {
  it('完了の応答で、outgoing を除き、メッセージを加える(WS-01)', async () => {
    const { app, api, timer, conversation } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'hello') })
    await settled()

    expect(conversation().outgoing).toEqual([])
    expect(conversation().messages).toEqual([message(1, alice, 'hello')])
    expect(timer.pending).toBe(0)
  })

  it('完了の応答が先に届き、配信が後から届いても、1通だけになる', async () => {
    const { app, api, realtime, conversation, seqs } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'hello') })
    await settled()
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, 'hello'), 'send-1'))

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })

  it('配信が先に届き、完了の応答が後から届いても、1通だけになる', async () => {
    const { app, api, realtime, timer, conversation, seqs } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, 'hello'), 'send-1'))

    // 配信で、送信中の表示が、完了したメッセージに置き換わる。
    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
    expect(timer.pending).toBe(0)

    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'hello') })
    await settled()

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })

  it('送信者が自分でない配信は、送信の識別子が一致しても、outgoing を除かない', async () => {
    const { app, realtime, conversation, seqs } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    // bob が、偶然、または、意図して、同じ送信の識別子で送った。
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob, 'from bob'), 'send-1'))

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([{ sendId: 'send-1', body: 'hello', status: 'sending' }])
  })

  it('保持している送信の識別子と一致しない、自分の配信(別の端末からの送信)は、通常の配信として加える(WS-03)', async () => {
    const { app, realtime, conversation, seqs } = await setupOpenConversation()
    app.setDraft('この端末から')
    app.send()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, '別の端末から'), 'send-on-another-device'))

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toHaveLength(1)
  })

  it('完了したメッセージは、連番の順に、送信中のメッセージとは別に保持する', async () => {
    const { app, api, realtime, conversation, seqs } = await setupOpenConversation()
    app.setDraft('first')
    app.send()
    app.setDraft('second')
    app.send()

    // 後の送信が、先に完了する。その間に、bob のメッセージも届く。
    api.sendCalls[1]?.resolve({ kind: 'completed', message: message(2, alice, 'second') })
    await settled()
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))

    expect(seqs()).toEqual([1, 2])
    expect(conversation().outgoing).toEqual([{ sendId: 'send-1', body: 'first', status: 'sending' }])
  })
})

describe('拒否(C-7、WS-40)', () => {
  it('拒否の応答で、outgoing を除き、理由を入れ、本文を draft に戻す', async () => {
    const { app, api, timer, conversation } = await setupOpenConversation()
    app.setDraft('拒否される本文')
    app.send()

    api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_body' })
    await settled()

    expect(conversation().outgoing).toEqual([])
    expect(conversation().messages).toEqual([])
    expect(conversation().rejection).toBe('invalid_body')
    expect(conversation().draft).toBe('拒否される本文')
    expect(timer.pending).toBe(0)
  })

  it('draft に別の入力がある場合は、その前に、改行で区切って戻す', async () => {
    const { app, api, conversation } = await setupOpenConversation()
    app.setDraft('拒否される本文')
    app.send()
    app.setDraft('次の入力')

    api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_body' })
    await settled()

    expect(conversation().draft).toBe('拒否される本文\n次の入力')
  })

  it('拒否の後の送信は、新しい送信として扱い、新しい送信の識別子を添える。理由は、消す', async () => {
    const { app, api, conversation } = await setupOpenConversation()
    app.setDraft('拒否される本文')
    app.send()
    api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_body' })
    await settled()

    app.setDraft('直した本文')
    app.send()

    expect(api.sendCalls[1]?.request).toEqual({ sessionId: 'session-alice', peer: 'bob', sendId: 'send-2', body: '直した本文' })
    expect(conversation().rejection).toBeNull()
    expect(conversation().draft).toBe('')
  })
})

describe('送信できなかった場合(C-8、WS-17)', () => {
  it('10秒以内に応答を得られない場合、failed にする。本文は、失われない', async () => {
    const { app, timer, conversation } = await setupOpenConversation()
    app.setDraft('届かない本文')
    app.send()

    timer.advance(SEND_TIMEOUT_MS - 1)
    expect(conversation().outgoing[0]?.status).toBe('sending')

    timer.advance(1)
    expect(conversation().outgoing).toEqual([{ sendId: 'send-1', body: '届かない本文', status: 'failed' }])
    expect(conversation().messages).toEqual([])
  })

  it('送信が届かない場合、failed にする', async () => {
    const { app, api, timer, conversation } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    api.sendCalls[0]?.resolve({ kind: 'failed' })
    await settled()

    expect(conversation().outgoing).toEqual([{ sendId: 'send-1', body: 'hello', status: 'failed' }])
    expect(timer.pending).toBe(0)
  })

  it('ポートの呼び出しが失敗した場合も、failed にする', async () => {
    const { app, api, conversation } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    api.sendCalls[0]?.reject(new Error('network'))
    await settled()

    expect(conversation().outgoing[0]?.status).toBe('failed')
  })

  it('リアルタイムの接続が切れていても、送信が届けば、完了する', async () => {
    const { app, api, realtime, conversation, seqs } = await setupOpenConversation()
    realtime.latest.listener.onClosed(false)
    app.setDraft('hello')
    app.send()

    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'hello') })
    await settled()

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })
})

describe('送り直し(C-9、WS-17、WS-18)', () => {
  async function setupFailed() {
    const context = await setupOpenConversation()
    context.app.setDraft('送り直す本文')
    context.app.send()
    context.timer.advance(SEND_TIMEOUT_MS)
    return context
  }

  it('failed の outgoing を、同じ送信の識別子と本文で、sending に戻して、送信する', async () => {
    const { app, api, timer, conversation } = await setupFailed()

    app.resend('send-1')

    expect(api.sendCalls.map((c) => c.request)).toEqual([
      { sessionId: 'session-alice', peer: 'bob', sendId: 'send-1', body: '送り直す本文' },
      { sessionId: 'session-alice', peer: 'bob', sendId: 'send-1', body: '送り直す本文' },
    ])
    expect(conversation().outgoing).toEqual([{ sendId: 'send-1', body: '送り直す本文', status: 'sending' }])
    expect(timer.pending).toBe(1)
  })

  it('送り直しが完了すると、完了したメッセージになる', async () => {
    const { app, api, conversation, seqs } = await setupFailed()
    app.resend('send-1')

    api.sendCalls[1]?.resolve({ kind: 'completed', message: message(1, alice, '送り直す本文') })
    await settled()

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })

  it('10秒を、改めて待ち始める', async () => {
    const { app, timer, conversation } = await setupFailed()
    app.resend('send-1')

    timer.advance(SEND_TIMEOUT_MS - 1)
    expect(conversation().outgoing[0]?.status).toBe('sending')
    timer.advance(1)
    expect(conversation().outgoing[0]?.status).toBe('failed')
  })

  it('sending の outgoing と、存在しない送信の識別子は、送り直さない', async () => {
    const { app, api } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()

    app.resend('send-1')
    app.resend('send-unknown')

    expect(api.sendCalls).toHaveLength(1)
  })
})

describe('failed にした後に届いた結果(C-10、ADR-011)', () => {
  async function setupFailed() {
    const context = await setupOpenConversation()
    context.app.setDraft('遅れて届く本文')
    context.app.send()
    context.timer.advance(SEND_TIMEOUT_MS)
    expect(context.conversation().outgoing[0]?.status).toBe('failed')
    return context
  }

  it('完了の応答が届いた場合、完了したメッセージに置き換える', async () => {
    const { api, conversation, seqs } = await setupFailed()

    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, '遅れて届く本文') })
    await settled()

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })

  it('配信が届いた場合、完了したメッセージに置き換える', async () => {
    const { realtime, conversation, seqs } = await setupFailed()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, '遅れて届く本文'), 'send-1'))

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })

  it('拒否の応答が届いた場合、outgoing を除き、理由を入れ、本文を draft に戻す', async () => {
    const { api, conversation } = await setupFailed()

    api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_body' })
    await settled()

    expect(conversation().outgoing).toEqual([])
    expect(conversation().rejection).toBe('invalid_body')
    expect(conversation().draft).toBe('遅れて届く本文')
  })

  it('送り直しの前の試行の、時間切れは、捨てる', async () => {
    const { app, api, timer, conversation } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()
    // 最初の試行は、5秒で失敗する。
    timer.advance(5_000)
    api.sendCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(conversation().outgoing[0]?.status).toBe('failed')

    app.resend('send-1')
    // 最初の送信から10秒が過ぎても、送り直しは、まだ5秒しか待っていない。
    timer.advance(5_000)
    expect(conversation().outgoing[0]?.status).toBe('sending')

    timer.advance(5_000)
    expect(conversation().outgoing[0]?.status).toBe('failed')
  })

  it('送り直しの前の試行の、失敗は、捨てる', async () => {
    // 最初の試行は、応答を得られないまま、10秒が過ぎている。
    const { app, api, conversation } = await setupFailed()
    app.resend('send-1')

    // 送り直しの後に、最初の試行の失敗が届く。
    api.sendCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(conversation().outgoing[0]?.status).toBe('sending')

    // 送り直しの失敗は、反映する。
    api.sendCalls[1]?.reject(new Error('network'))
    await settled()
    expect(conversation().outgoing[0]?.status).toBe('failed')
  })

  it('送り直しの後に、前の試行の完了の応答が届いた場合、完了にする。後の応答は、捨てる', async () => {
    const { app, api, conversation, seqs } = await setupFailed()
    app.resend('send-1')

    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, '遅れて届く本文') })
    await settled()
    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])

    // 送り直しは、保管済みのメッセージについて、完了する(WS-18)。outgoing は、既にない。
    api.sendCalls[1]?.resolve({ kind: 'completed', message: message(1, alice, '遅れて届く本文') })
    await settled()
    expect(seqs()).toEqual([1])

    // outgoing が既にない送信への、拒否の応答は、捨てる。
    api.sendCalls[1]?.resolve({ kind: 'rejected', code: 'invalid_input' })
    await settled()
    expect(conversation().rejection).toBeNull()
    expect(conversation().draft).toBe('')
  })
})

describe('別の会話を開く(C-11)', () => {
  it('それまでの会話の状態(outgoing と draft を含む)を捨てる', async () => {
    const { app, realtime, timer, conversation } = await setupOpenConversation()
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))
    app.setDraft('送信中')
    app.send()
    app.setDraft('書きかけ')

    await openPeer('carol')

    expect(conversation()).toEqual({
      peer: carol,
      history: 'loading',
      historyRejection: null,
      messages: [],
      outgoing: [],
      draft: '',
      rejection: null,
    })
    expect(timer.pending).toBe(0)
  })

  it('前の会話に対して、後から届いた結果は、捨てる', async () => {
    const { app, api, realtime, conversation, seqs } = await setupOpenConversation()
    app.setDraft('bob へ')
    app.send()

    await openPeer('carol')
    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'bob へ') })
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, 'bob へ'), 'send-1'))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(2, bob)))
    await settled()

    expect(seqs()).toEqual([])
    expect(conversation().outgoing).toEqual([])
    expect(conversation().rejection).toBeNull()
  })

  it('前の会話の履歴の取得の結果は、捨てる', async () => {
    const { api, realtime, conversation, seqs } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')
    await openPeer('carol')

    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, bob, 'bob との会話')] })
    await settled()
    expect(seqs()).toEqual([])
    expect(conversation().history).toBe('loading')

    api.historyCalls[1]?.resolve({ kind: 'loaded', messages: [message(1, carol, 'carol との会話')] })
    await settled()
    expect(conversation().messages.map((m) => m.body)).toEqual(['carol との会話'])
  })

  it('開き直した会話は、履歴が示す内容に従う', async () => {
    const { app, api, conversation, seqs } = await setupOpenConversation()
    app.setDraft('完了の応答が届かない送信')
    app.send()

    await openPeer('carol')
    await openPeer('bob')
    // サーバーは、その送信を保管していた。
    api.historyCalls[2]?.resolve({ kind: 'loaded', messages: [message(1, alice, '完了の応答が届かない送信')] })
    await settled()

    expect(seqs()).toEqual([1])
    expect(conversation().outgoing).toEqual([])
  })
})

describe('同じ相手を、もう一度指定する(C-12)', () => {
  it('会話の状態(messages、outgoing、draft、rejection)を保ち、履歴を取得し直さない', async () => {
    const { app, api, realtime, timer, conversation } = await setupOpenConversation()
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))
    app.setDraft('送信できなかった本文')
    app.send()
    timer.advance(SEND_TIMEOUT_MS)
    app.setDraft('書きかけ')
    const before = conversation()

    await openPeer('bob')

    expect(conversation()).toBe(before)
    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴の取得中は、何もしない', async () => {
    const { api, realtime } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')

    await openPeer('bob')

    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴を取得できなかった場合は、会話の状態を保ったまま、取得し直す', async () => {
    const { app, api, realtime, conversation, seqs } = setup()
    await signIn('alice')
    realtime.latest.listener.onReady()
    await openPeer('bob')
    api.historyCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    // 取得できなかった間にも、配信は届き、入力はできる。
    realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob)))
    app.setDraft('書きかけ')

    await openPeer('bob')

    expect(conversation().history).toBe('loading')
    expect(conversation().draft).toBe('書きかけ')
    expect(seqs()).toEqual([3])
    expect(api.historyCalls).toHaveLength(2)

    api.historyCalls[1]?.resolve({ kind: 'loaded', messages: [message(1, alice), message(2, bob), message(3, bob)] })
    await settled()

    expect(conversation().history).toBe('loaded')
    expect(seqs()).toEqual([1, 2, 3])
  })

  it('取得し直すときも、接続を確立している途中であれば、ready を待つ', async () => {
    const { api, realtime, conversation } = setup()
    await signIn('alice')
    await openPeer('bob')
    realtime.latest.listener.onClosed(false)
    api.historyCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(conversation().history).toBe('failed')

    // 接続は、切れている。待たずに、取得し直す。
    await openPeer('bob')

    expect(api.historyCalls).toHaveLength(2)
  })
})

describe('変化の通知', () => {
  it('状態が変わるたびに、購読している相手に通知する。getState は、変化がなければ、同じ値を返す', async () => {
    const { app, realtime } = setup()
    let notified = 0
    const unsubscribe = app.subscribe(() => {
      notified++
    })

    const before = app.getState()
    expect(app.getState()).toBe(before)

    await signIn('alice')
    const afterSignIn = notified
    expect(afterSignIn).toBeGreaterThan(0)
    expect(app.getState()).not.toBe(before)

    realtime.latest.listener.onReady()
    expect(notified).toBe(afterSignIn)
    app.setDraft('会話を開いていなければ、変わらない')
    expect(notified).toBe(afterSignIn)

    await openPeer('bob')
    const afterOpen = notified
    app.setDraft('hello')
    expect(notified).toBe(afterOpen + 1)
    const last = notified

    unsubscribe()
    app.setDraft('bye')
    expect(notified).toBe(last)
  })
})
