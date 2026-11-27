// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ChatApp, SEND_TIMEOUT_MS } from '../application/chatApp'
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
} from '../application/testPorts'
import { App } from './App'

afterEach(cleanup)

const users = { alice, bob, carol }

function renderApp(signupCredential: string | null = null) {
  const auth = new FakeAuthApi()
  const api = new FakeChatApi()
  const realtime = new FakeRealtimeChannel()
  const timer = new FakeTimer()
  const entry = new FakeEntryLink(signupCredential)
  const app = new ChatApp({ auth, api, realtime, timer, sendIds: new FakeSendIdSource(), entry })
  const view = render(<App app={app} />)
  return { app, auth, api, realtime, timer, entry, ...view }
}

type Context = ReturnType<typeof renderApp>

/** application の外(ポート)で起きた出来事を、画面に反映する。 */
async function happen(event: () => void): Promise<void> {
  await act(async () => {
    event()
    await settled()
  })
}

function click(name: string): void {
  fireEvent.click(screen.getByRole('button', { name }))
}

function fill(label: string, value: string): void {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

function composer(): HTMLTextAreaElement {
  return screen.getByRole<HTMLTextAreaElement>('textbox', { name: 'メッセージ' })
}

function type(text: string): void {
  fireEvent.change(composer(), { target: { value: text } })
}

const contactLabel = '連絡先(メールアドレス)'
const secretLabel = '秘密(パスワード)'

/** ログインしていない状態(ログインの画面)にする。 */
async function atLogin(signupCredential: string | null = null): Promise<Context> {
  const context = renderApp(signupCredential)
  await happen(() => context.auth.sessionCalls[0]?.resolve({ kind: 'signedOut' }))
  return context
}

/** 画面から、利用者としてログインする。 */
async function loginAs(context: Context, name: keyof typeof users): Promise<void> {
  fill(contactLabel, `${name}@example.com`)
  fill(secretLabel, `${name}-secret`)
  click('ログイン')
  await happen(() => context.auth.loginCalls.at(-1)?.resolve({ kind: 'completed', session: sessionOf(users[name]) }))
}

/** alice として、ログインした状態にする。 */
async function signedIn(): Promise<Context> {
  const context = await atLogin()
  await loginAs(context, 'alice')
  return context
}

/** 相手を特定して、会話を開く。 */
async function findAndOpen(context: Context, name: keyof typeof users): Promise<void> {
  fill('相手の識別名', name)
  click('探す')
  await happen(() => context.api.peerCalls.at(-1)?.resolve({ kind: 'found', peer: users[name] }))
  click('会話を開く')
}

/** alice として、bob との会話を開き、履歴を取得し終えた状態にする。 */
async function openConversation(history = [] as ReturnType<typeof message>[]) {
  const context = await signedIn()
  await happen(() => context.realtime.latest.listener.onReady())
  await findAndOpen(context, 'bob')
  await happen(() => context.api.historyCalls[0]?.resolve({ kind: 'loaded', messages: history }))
  return context
}

describe('アプリを開く(RL-21、RL-23)', () => {
  it('ログイン状態を確かめている間は、ログインの画面も、チャットも、示さない', () => {
    const { auth } = renderApp()

    expect(screen.getByRole('status').textContent).toBe('ログイン状態を確かめています…')
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(auth.sessionCalls).toHaveLength(1)
  })

  it('ログイン中であれば、再読み込みの後も、ログインした利用者を示す', async () => {
    const { auth, realtime } = renderApp()

    await happen(() => auth.sessionCalls[0]?.resolve({ kind: 'signedIn', session: sessionOf(alice) }))

    expect(screen.getByTestId('current-user').textContent).toBe('Alice')
    expect(screen.getByTestId('current-handle').textContent).toBe('@alice')
    expect(realtime.latest.sessionId).toBe('session-alice')
  })

  it('確かめられなければ、そのことを示す。ログインの画面は、示さない。確かめ直せる', async () => {
    const { auth } = renderApp()

    await happen(() => auth.sessionCalls[0]?.resolve({ kind: 'failed' }))

    expect(screen.getByRole('alert').textContent).toBe('ログイン状態を確かめられませんでした。')
    expect(screen.queryByRole('button', { name: 'ログイン' })).toBeNull()

    click('確かめ直す')
    await happen(() => auth.sessionCalls[1]?.resolve({ kind: 'signedOut' }))

    expect(screen.getByRole('button', { name: 'ログイン' })).toBeTruthy()
  })
})

describe('ログイン(RL-12、RL-13)', () => {
  it('連絡先と秘密を入力して、ログインする。秘密の入力欄は、内容を伏せる', async () => {
    const context = await atLogin()

    const secret = screen.getByLabelText<HTMLInputElement>(secretLabel)
    expect(secret.type).toBe('password')
    expect(secret.autocomplete).toBe('current-password')
    // 入力欄が空の間だけ、ボタンを無効にする。入力は、検証しない。
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'ログイン' }).disabled).toBe(true)

    fill(contactLabel, 'Alice@Example.com')
    fill(secretLabel, 'x')
    click('ログイン')

    expect(context.auth.loginCalls[0]?.request).toEqual({ contact: 'Alice@Example.com', secret: 'x' })
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'ログイン' }).disabled).toBe(true)

    await happen(() => context.auth.loginCalls[0]?.resolve({ kind: 'completed', session: sessionOf(alice) }))

    expect(screen.getByTestId('current-user').textContent).toBe('Alice')
    expect(screen.queryByLabelText(secretLabel)).toBeNull()
    expect(screen.getByText('相手の識別名を入力して、探してください。')).toBeTruthy()
  })

  it('失敗の理由は、区別せずに示す。入力は、保つ', async () => {
    const { auth } = await atLogin()
    fill(contactLabel, 'alice@example.com')
    fill(secretLabel, 'wrong-secret')
    click('ログイン')

    await happen(() => auth.loginCalls[0]?.resolve({ kind: 'rejected', code: 'login_failed', fields: [] }))

    expect(screen.getByRole('alert').textContent).toBe('ログインできませんでした。連絡先と、秘密を確かめてください。')
    expect(screen.getByLabelText<HTMLInputElement>(contactLabel).value).toBe('alice@example.com')
    expect(screen.getByLabelText<HTMLInputElement>(secretLabel).value).toBe('wrong-secret')
  })

  it('上限と、完了できなかった場合の、理由を示す(RL-29)', async () => {
    const { auth } = await atLogin()
    fill(contactLabel, 'alice@example.com')
    fill(secretLabel, 'secret')

    click('ログイン')
    await happen(() => auth.loginCalls[0]?.resolve({ kind: 'rejected', code: 'rate_limited', fields: [] }))
    expect(screen.getByRole('alert').textContent).toBe('ログインの試行の、回数の上限を超えました。時間をおいて、やり直してください。')

    click('ログイン')
    await happen(() => auth.loginCalls[1]?.resolve({ kind: 'failed' }))
    expect(screen.getByRole('alert').textContent).toBe('ログインを、完了できませんでした。時間をおいて、やり直してください。')
  })
})

describe('登録(RL-01、RL-03、RL-06、RL-07)', () => {
  it('連絡先を入力して、申し込む。完了で、案内を送ったことを示す', async () => {
    const { auth } = await atLogin()

    click('登録を申し込む')
    fill(contactLabel, 'dave@example.com')
    click('申し込む')
    expect(auth.signupCalls[0]?.request).toEqual({ contact: 'dave@example.com' })
    await happen(() => auth.signupCalls[0]?.resolve({ kind: 'completed' }))

    expect(screen.getByRole('status').textContent).toBe('案内を送りました。届いた案内に従って、登録を続けてください。')
  })

  it('申し込みが拒否された場合と、完了できなかった場合は、理由を示し、案内を送ったとは示さない(RL-10)', async () => {
    const { auth } = await atLogin()
    click('登録を申し込む')
    fill(contactLabel, 'not-a-contact')

    click('申し込む')
    await happen(() => auth.signupCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_fields', fields: ['contact'] }))
    expect(screen.getByRole('alert').textContent).toBe('入力が正しくありません。次の項目を、直してください。')
    expect(screen.getByLabelText(contactLabel).getAttribute('aria-invalid')).toBe('true')
    expect(screen.queryByRole('status')).toBeNull()

    click('申し込む')
    await happen(() => auth.signupCalls[1]?.resolve({ kind: 'failed' }))
    expect(screen.getByRole('alert').textContent).toBe('完了できませんでした。時間をおいて、やり直してください。')
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('案内から開くと、登録を続ける画面を示す。項目ごとに、規則を示す。完了で、ログインの画面に、完了したことを示す', async () => {
    const { auth } = renderApp('signup-credential')

    expect(screen.getByRole('heading', { name: '登録' })).toBeTruthy()
    expect(auth.sessionCalls).toHaveLength(0)
    const secret = screen.getByLabelText<HTMLInputElement>(secretLabel)
    expect(secret.type).toBe('password')
    expect(secret.autocomplete).toBe('new-password')
    expect(screen.getByText(/^3文字以上、20文字以下。英字、数字、アンダースコア/)).toBeTruthy()
    expect(screen.getByText(/^15文字以上、128文字以下。よく使われるもの、連絡先や識別名と同じもの/)).toBeTruthy()

    fill('識別名', 'Dave_01')
    fill('表示名', 'Dave')
    fill(secretLabel, 'dave passphrase for tests')
    click('登録する')

    expect(auth.completeCalls[0]?.request).toEqual({
      credential: 'signup-credential',
      handle: 'Dave_01',
      displayName: 'Dave',
      secret: 'dave passphrase for tests',
    })
    await happen(() => auth.completeCalls[0]?.resolve({ kind: 'completed' }))

    expect(screen.getByRole('status').textContent).toBe('登録を完了しました。登録した連絡先と、秘密で、ログインしてください。')
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeTruthy()
  })

  it('拒否では、正しくない項目を、すべて示し、入力を保つ(UC-01 E1)', async () => {
    const { auth } = renderApp('signup-credential')
    fill('識別名', 'da ve')
    fill('表示名', 'Dave')
    fill(secretLabel, 'short')
    click('登録する')

    await happen(() => auth.completeCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_fields', fields: ['handle', 'secret'] }))

    const alert = screen.getByRole('alert')
    expect(within(alert).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['識別名', secretLabel])
    expect(screen.getByLabelText('識別名').getAttribute('aria-invalid')).toBe('true')
    expect(screen.getByLabelText('表示名').getAttribute('aria-invalid')).toBeNull()
    expect(screen.getByLabelText(secretLabel).getAttribute('aria-invalid')).toBe('true')
    expect(screen.getByLabelText<HTMLInputElement>('識別名').value).toBe('da ve')
    expect(screen.getByLabelText<HTMLInputElement>('表示名').value).toBe('Dave')
    expect(screen.getByLabelText<HTMLInputElement>(secretLabel).value).toBe('short')
  })

  it('識別名が使われている場合と、案内が無効な場合の、理由を示す', async () => {
    const { auth } = renderApp('signup-credential')
    fill('識別名', 'alice')
    fill('表示名', 'Dave')
    fill(secretLabel, 'dave passphrase for tests')

    click('登録する')
    await happen(() => auth.completeCalls[0]?.resolve({ kind: 'rejected', code: 'handle_taken', fields: [] }))
    expect(screen.getByRole('alert').textContent).toBe('この識別名は、使われています。別の識別名を入力してください。')

    click('登録する')
    await happen(() => auth.completeCalls[1]?.resolve({ kind: 'rejected', code: 'invalid_signup_credential', fields: [] }))
    expect(screen.getByRole('alert').textContent).toContain('登録を、申し込み直してください。')

    click('登録を申し込み直す')
    expect(screen.getByRole('heading', { name: '登録の申し込み' })).toBeTruthy()
  })
})

describe('ログアウトと、ログイン状態の終わり(RL-17、RL-19、RL-20、RL-22)', () => {
  it('ログアウトすると、ログインの画面に切り替わり、会話の内容と、利用者の情報を、示し続けない', async () => {
    const { auth, realtime } = await openConversation([message(1, bob, 'alice と bob の会話')])
    type('入力中の本文')

    click('ログアウト')
    await happen(() => auth.logoutCalls[0]?.resolve({ kind: 'completed' }))

    expect(realtime.latest.closed).toBe(true)
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeTruthy()
    expect(screen.queryByText('alice と bob の会話')).toBeNull()
    expect(screen.queryByTestId('current-user')).toBeNull()
    expect(screen.queryByDisplayValue('入力中の本文')).toBeNull()
    // 入力欄は、空から始まる。
    expect(screen.getByLabelText<HTMLInputElement>(contactLabel).value).toBe('')
    expect(screen.getByLabelText<HTMLInputElement>(secretLabel).value).toBe('')
  })

  it('ログアウトを完了できなかった場合は、できなかったことを示し、ログイン中のままとする(RL-22)', async () => {
    const { auth } = await openConversation([message(1, bob, 'alice と bob の会話')])

    click('ログアウト')
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'ログアウト' }).disabled).toBe(true)
    await happen(() => auth.logoutCalls[0]?.resolve({ kind: 'failed' }))

    expect(screen.getByRole('alert').textContent).toBe('ログアウトできませんでした。ログインしたままです。もう一度、試してください。')
    expect(screen.getByTestId('current-user').textContent).toBe('Alice')
    expect(screen.getByText('alice と bob の会話')).toBeTruthy()
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'ログアウト' }).disabled).toBe(false)
  })

  it('接続が、ログイン状態が無効になったことを示して閉じられると、操作なしに、ログインを求める画面に切り替わる(RL-19)', async () => {
    const { realtime } = await openConversation([message(1, bob, 'alice と bob の会話')])
    type('入力中の本文')

    await happen(() => realtime.latest.listener.onClosed(true))

    expect(screen.getByRole('status').textContent).toBe('ログインし直してください(ログイン状態が、無効になりました)。')
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeTruthy()
    expect(screen.queryByText('alice と bob の会話')).toBeNull()
    expect(screen.queryByTestId('current-user')).toBeNull()
    expect(screen.queryByDisplayValue('入力中の本文')).toBeNull()
  })

  it('認証の拒否を受けると、ログインを求める画面に切り替わる。本文は、保たない(RL-19、RL-20)', async () => {
    const { api } = await openConversation([message(1, bob, 'alice と bob の会話')])
    type('alice を示したまま、入力した本文')
    click('送信')

    await happen(() => api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'unauthenticated' }))

    expect(screen.getByRole('status').textContent).toBe('ログインし直してください(ログイン状態が、無効になりました)。')
    expect(screen.queryByText('alice と bob の会話')).toBeNull()
    expect(screen.queryByText('alice を示したまま、入力した本文')).toBeNull()
  })

  it('別の利用者としてログインし直した後、前の利用者の会話と、前の接続の配信を、示さない(RL-20)', async () => {
    const context = await openConversation([message(1, bob, 'alice と bob の会話')])
    const asAlice = context.realtime.latest
    await happen(() => asAlice.listener.onClosed(true))

    await loginAs(context, 'carol')
    await happen(() => context.realtime.latest.listener.onReady())
    await findAndOpen(context, 'bob')
    await happen(() => {
      asAlice.listener.onMessage(delivery([alice, bob], message(2, bob, '前の接続に届いた')))
      context.realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob, '宛先でない配信')))
    })

    expect(screen.getByTestId('current-user').textContent).toBe('Carol')
    expect(screen.queryByText('alice と bob の会話')).toBeNull()
    expect(screen.queryByText('前の接続に届いた')).toBeNull()
    expect(screen.queryByText('宛先でない配信')).toBeNull()
  })
})

describe('相手の特定(RL-24〜RL-26、RL-31)', () => {
  it('識別名を入力して探すと、相手の識別名(登録した通り)と表示名を示す。会話は、開くまで、示さない', async () => {
    const { api, realtime } = await signedIn()
    await happen(() => realtime.latest.listener.onReady())

    fill('相手の識別名', 'BOB_SMITH')
    click('探す')
    expect(api.peerCalls[0]?.request).toEqual({ sessionId: 'session-alice', handle: 'BOB_SMITH' })
    await happen(() => api.peerCalls[0]?.resolve({ kind: 'found', peer: { handle: 'Bob_Smith', displayName: 'Bob S.' } }))

    expect(screen.getByTestId('found-peer').textContent).toContain('Bob S.')
    expect(screen.getByTestId('found-peer').textContent).toContain('@Bob_Smith')
    expect(screen.queryByRole('region')).toBeNull()

    click('会話を開く')
    expect(screen.getByRole('region', { name: 'Bob S. との会話' })).toBeTruthy()
    expect(api.historyCalls[0]?.request).toEqual({ sessionId: 'session-alice', peer: 'Bob_Smith' })
  })

  it('見つからない場合、自分自身の場合、上限の場合の、理由を示す。候補は、示さない', async () => {
    const cases = [
      { code: 'not_found', text: '見つかりませんでした。' },
      { code: 'invalid_peer', text: '自分自身は、相手にできません。' },
      { code: 'invalid_input', text: '識別名が正しくありません。識別名は、3文字以上、20文字以下の、英字、数字、アンダースコア(_)です。' },
      { code: 'rate_limited', text: '新しい相手を探す操作の、回数の上限を超えました。時間をおいて、やり直してください。' },
    ] as const
    const { api } = await signedIn()
    fill('相手の識別名', 'someone')

    for (const [index, { code, text }] of cases.entries()) {
      click('探す')
      await happen(() => api.peerCalls[index]?.resolve({ kind: 'rejected', code }))

      expect(screen.getByRole('alert').textContent).toBe(text)
      expect(screen.queryByTestId('found-peer')).toBeNull()
    }
  })

  it('履歴の取得が、上限によって拒否された場合は、理由を示す(RL-31)', async () => {
    const context = await signedIn()
    await happen(() => context.realtime.latest.listener.onReady())
    await findAndOpen(context, 'bob')

    await happen(() => context.api.historyCalls[0]?.resolve({ kind: 'rejected', code: 'rate_limited' }))

    expect(screen.getByRole('alert').textContent).toContain('新しい相手との会話を開く操作の、回数の上限を超えました。')
  })
})

describe('画面の構造やプログラムとして解釈されうる表示名と識別名(RL-41 の WS-27)', () => {
  const markup = '<img src=x onerror="window.hacked = true"><b>bold</b>'

  it('表示名を、文字として示し、要素を作らない', async () => {
    const evil = { handle: 'evil_user', displayName: markup }
    const { auth, api, realtime, container } = renderApp()
    await happen(() => auth.sessionCalls[0]?.resolve({ kind: 'signedIn', session: { sessionId: 's', user: evil } }))
    await happen(() => realtime.latest.listener.onReady())

    expect(screen.getByTestId('current-user').textContent).toBe(markup)

    fill('相手の識別名', 'peer')
    click('探す')
    await happen(() => api.peerCalls[0]?.resolve({ kind: 'found', peer: { handle: 'peer', displayName: markup } }))
    expect(screen.getByTestId('found-peer').textContent).toContain(markup)

    click('会話を開く')
    await happen(() =>
      api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, { handle: 'peer', displayName: markup }, 'hello')] }),
    )

    expect(screen.getByTestId('conversation-peer').textContent).toContain(markup)
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('b')).toBeNull()
    expect((window as unknown as { hacked?: boolean }).hacked).toBeUndefined()
  })
})

describe('会話', () => {
  it('選んでいる相手を、見分けられるように示す', async () => {
    await openConversation()

    expect(screen.getByRole('button', { name: '会話を開く' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('region', { name: 'Bob との会話' })).toBeTruthy()
    // 相手は、表示名に、識別名を併せて示す(RL-D6)。
    expect(screen.getByTestId('conversation-peer').textContent).toBe('Bob@bob')
  })

  it('取得中であることを示す', async () => {
    const context = await signedIn()
    await happen(() => context.realtime.latest.listener.onReady())

    await findAndOpen(context, 'bob')

    expect(screen.getByText('履歴を取得しています…')).toBeTruthy()
    expect(screen.queryByText('メッセージはありません')).toBeNull()
  })

  it('メッセージがなければ、「メッセージはありません」と示す(WS-21)', async () => {
    await openConversation()

    expect(screen.getByText('メッセージはありません')).toBeTruthy()
    expect(screen.queryByText('履歴を取得しています…')).toBeNull()
    expect(screen.queryByRole('list', { name: 'メッセージ' })).toBeNull()
  })

  it('メッセージを、連番の順に、送信者の表示名と、送信の日時とともに示す(WS-20)', async () => {
    await openConversation([message(1, alice, 'one'), message(2, bob, 'two'), message(3, alice, 'three')])

    const items = within(screen.getByRole('list', { name: 'メッセージ' })).getAllByRole('listitem')
    expect(items.map((item) => item.querySelector('.message-body')?.textContent)).toEqual(['one', 'two', 'three'])
    expect(items.map((item) => item.querySelector('.message-sender')?.textContent)).toEqual(['Alice', 'Bob', 'Alice'])
    for (const [index, item] of items.entries()) {
      const time = item.querySelector('time')
      expect(time?.getAttribute('datetime')).toBe(message(index + 1, alice).sentAt)
      // 端末の地域の時刻で、表す。
      expect(time?.textContent).not.toBe('')
      expect(time?.textContent).not.toBe(message(index + 1, alice).sentAt)
    }
    expect(screen.queryByText('メッセージはありません')).toBeNull()
  })

  it('配信されたメッセージを、操作なしに示す(WS-02)', async () => {
    const { realtime } = await openConversation()

    await happen(() => realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob, 'bob から'))))

    expect(screen.getByText('bob から')).toBeTruthy()
    expect(screen.queryByText('メッセージはありません')).toBeNull()
  })

  it('別の会話の配信は、開いている会話の中には示さない', async () => {
    const { realtime } = await openConversation()

    await happen(() => realtime.latest.listener.onMessage(delivery([alice, carol], message(1, carol, 'carol から'))))

    expect(screen.queryByText('carol から')).toBeNull()
  })

  it('履歴を取得できなかった場合は、そのことと、「取得し直す」ボタンを示す', async () => {
    const context = await signedIn()
    const { api, realtime } = context
    await happen(() => realtime.latest.listener.onReady())
    await findAndOpen(context, 'bob')
    await happen(() => api.historyCalls[0]?.resolve({ kind: 'failed' }))

    expect(screen.getByText('履歴を取得できませんでした。')).toBeTruthy()
    expect(screen.queryByText('メッセージはありません')).toBeNull()

    click('取得し直す')
    expect(screen.getByText('履歴を取得しています…')).toBeTruthy()
    await happen(() => api.historyCalls[1]?.resolve({ kind: 'loaded', messages: [message(1, bob, '取得できた')] }))

    expect(screen.getByText('取得できた')).toBeTruthy()
    expect(screen.queryByText('履歴を取得できませんでした。')).toBeNull()
  })

  it('改行とタブを、保管した通りに示す(WS-11)', async () => {
    const body = '1行目\n2行目\n\n\t字下げ  空白2つ '
    await openConversation([message(1, bob, body)])

    const element = screen.getByTestId('message').querySelector('.message-body')
    // 本文は、1つの文字列として描画する。改行は、CSS(white-space: pre-wrap)で、改行として示す。
    expect(element?.textContent).toBe(body)
    expect(element?.childNodes).toHaveLength(1)
  })
})

describe('画面の構造やプログラムとして解釈されうる本文(WS-27)', () => {
  const bodies = [
    '<script>window.__xss = true</script>',
    '<img src="x" onerror="window.__xss = true">',
    '<b>太字</b><a href="javascript:window.__xss = true">link</a>',
    '&lt;b&gt; &amp; &quot; &#x3C;script&#x3E;',
    'https://example.com/ へのリンク',
    '{{constructor.constructor("window.__xss = true")()}}',
  ]

  it('受け取った本文を、文字として示し、要素を作らない', async () => {
    await openConversation(bodies.map((body, index) => message(index + 1, bob, body)))

    const shown = screen.getAllByTestId('message').map((item) => item.querySelector('.message-body'))
    expect(shown.map((element) => element?.textContent)).toEqual(bodies)
    for (const element of shown) {
      expect(element?.children).toHaveLength(0)
    }
    const list = screen.getByRole('list', { name: 'メッセージ' })
    expect(list.querySelector('script, img, b, a, iframe')).toBeNull()
    expect((window as unknown as { __xss?: boolean }).__xss).toBeUndefined()
  })

  it('送信者の画面でも、送信中の本文と、完了した本文を、文字として示す', async () => {
    const { api } = await openConversation()
    const body = bodies[1] ?? ''
    type(body)
    click('送信')

    const outgoing = screen.getByTestId('outgoing').querySelector('.message-body')
    expect(outgoing?.textContent).toBe(body)
    expect(outgoing?.children).toHaveLength(0)

    await happen(() => api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, body) }))

    const completed = screen.getByTestId('message').querySelector('.message-body')
    expect(completed?.textContent).toBe(body)
    expect(document.querySelector('img')).toBeNull()
    expect((window as unknown as { __xss?: boolean }).__xss).toBeUndefined()
  })
})

describe('送信', () => {
  it('送信ボタンは、入力欄が空の文字列のときだけ、無効にする', async () => {
    await openConversation()
    const button = screen.getByRole<HTMLButtonElement>('button', { name: '送信' })

    expect(button.disabled).toBe(true)
    type(' ')
    expect(button.disabled).toBe(false)
    type('')
    expect(button.disabled).toBe(true)
  })

  it('送信すると、送信中として示し、入力欄を空にする。完了すると、送信者と日時とともに示す(WS-01)', async () => {
    const { api } = await openConversation()
    type('こんにちは')

    click('送信')

    expect(api.sendCalls[0]?.request).toEqual({ sessionId: 'session-alice', peer: 'bob', sendId: 'send-1', body: 'こんにちは' })
    expect(composer().value).toBe('')
    const outgoing = screen.getByTestId('outgoing')
    expect(within(outgoing).getByText('こんにちは')).toBeTruthy()
    expect(within(outgoing).getByText('送信中')).toBeTruthy()
    expect(screen.queryByTestId('message')).toBeNull()

    await happen(() => api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'こんにちは') }))

    expect(screen.queryByTestId('outgoing')).toBeNull()
    const completed = screen.getByTestId('message')
    expect(completed.querySelector('.message-sender')?.textContent).toBe('Alice')
    expect(completed.querySelector('time')?.getAttribute('datetime')).toBe(message(1, alice).sentAt)
    expect(within(completed).getByText('こんにちは')).toBeTruthy()
  })

  it('完了の応答と配信の両方が届いても、1通だけを示す(WS-37)', async () => {
    const { api, realtime } = await openConversation()
    type('hello')
    click('送信')

    await happen(() => {
      realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, 'hello'), 'send-1'))
      api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'hello') })
    })

    expect(screen.getAllByTestId('message')).toHaveLength(1)
    expect(screen.queryByTestId('outgoing')).toBeNull()
  })

  it('Enter で送信し、Shift+Enter では送信しない', async () => {
    const { api } = await openConversation()
    type('1行目')

    fireEvent.keyDown(composer(), { key: 'Enter', shiftKey: true })
    expect(api.sendCalls).toHaveLength(0)

    fireEvent.keyDown(composer(), { key: 'Enter' })
    expect(api.sendCalls).toHaveLength(1)
    expect(api.sendCalls[0]?.request.body).toBe('1行目')
  })

  it('かな漢字変換の確定のための Enter では、送信しない', async () => {
    const { api } = await openConversation()
    type('へんかんちゅう')

    fireEvent.keyDown(composer(), { key: 'Enter', isComposing: true })
    fireEvent.keyDown(composer(), { key: 'Enter', keyCode: 229 })

    expect(api.sendCalls).toHaveLength(0)
    expect(composer().value).toBe('へんかんちゅう')
  })

  it('空の入力欄での Enter では、送信しない', async () => {
    const { api } = await openConversation()

    fireEvent.keyDown(composer(), { key: 'Enter' })

    expect(api.sendCalls).toHaveLength(0)
  })
})

describe('拒否(WS-40)', () => {
  it('拒否されたことと理由を示し、本文を入力欄に残し、会話には示さない', async () => {
    const { api } = await openConversation()
    const body = 'a'.repeat(4001)
    type(body)
    click('送信')

    await happen(() => api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_body' }))

    const alert = screen.getByRole('alert')
    expect(alert.textContent).toContain('本文が正しくないため、送信できませんでした')
    expect(alert.textContent).toContain('4,000文字以下')
    expect(composer().value).toBe(body)
    expect(composer().getAttribute('aria-invalid')).toBe('true')
    expect(screen.queryByTestId('message')).toBeNull()
    expect(screen.queryByTestId('outgoing')).toBeNull()
    expect(screen.getByText('メッセージはありません')).toBeTruthy()
  })

  it('本文を直して、送信できる。理由は、消える', async () => {
    const { api } = await openConversation()
    type('a'.repeat(4001))
    click('送信')
    await happen(() => api.sendCalls[0]?.resolve({ kind: 'rejected', code: 'invalid_body' }))

    type('a'.repeat(4000))
    click('送信')

    expect(screen.queryByRole('alert')).toBeNull()
    expect(api.sendCalls[1]?.request.sendId).toBe('send-2')

    await happen(() => api.sendCalls[1]?.resolve({ kind: 'completed', message: message(1, alice, 'a'.repeat(4000)) }))
    expect(screen.getAllByTestId('message')).toHaveLength(1)
  })

  it('理由ごとの文言を示す', async () => {
    const cases = [
      { code: 'invalid_input', text: '入力が正しくないため、送信できませんでした。' },
      { code: 'invalid_peer', text: 'この相手には、送信できません。' },
      { code: 'not_found', text: '相手が見つからないため、送信できませんでした。' },
      { code: 'rate_limited', text: '新しい相手への操作の、回数の上限を超えました。時間をおいて、やり直してください。' },
      { code: 'other', text: '送信が拒否されました。' },
    ] as const
    const { api } = await openConversation()

    for (const [index, { code, text }] of cases.entries()) {
      type('hello')
      click('送信')
      await happen(() => api.sendCalls[index]?.resolve({ kind: 'rejected', code }))

      expect(screen.getByRole('alert').textContent).toBe(text)
    }
  })
})

describe('送信できなかった場合(WS-17)', () => {
  it('送信できなかったことを示す。本文は失われず、入力し直さずに、送り直せる', async () => {
    const { api, timer } = await openConversation()
    type('届かない本文')
    click('送信')

    await happen(() => timer.advance(SEND_TIMEOUT_MS))

    const failed = screen.getByTestId('outgoing')
    expect(within(failed).getByText('届かない本文')).toBeTruthy()
    expect(within(failed).getByText('送信できませんでした')).toBeTruthy()
    expect(screen.queryByText('送信中')).toBeNull()

    click('送り直す')

    expect(api.sendCalls.map((c) => c.request)).toEqual([
      { sessionId: 'session-alice', peer: 'bob', sendId: 'send-1', body: '届かない本文' },
      { sessionId: 'session-alice', peer: 'bob', sendId: 'send-1', body: '届かない本文' },
    ])
    expect(within(screen.getByTestId('outgoing')).getByText('送信中')).toBeTruthy()

    await happen(() => api.sendCalls[1]?.resolve({ kind: 'completed', message: message(1, alice, '届かない本文') }))

    expect(screen.queryByTestId('outgoing')).toBeNull()
    expect(screen.getAllByTestId('message')).toHaveLength(1)
  })

  it('完了したメッセージの後ろに、送信中と、送信できなかったメッセージを示す', async () => {
    const { api, realtime } = await openConversation([message(1, bob, '履歴')])
    type('失敗する')
    click('送信')
    await happen(() => api.sendCalls[0]?.resolve({ kind: 'failed' }))
    type('送信中のまま')
    click('送信')
    await happen(() => realtime.latest.listener.onMessage(delivery([alice, bob], message(2, bob, '後から届いた'))))

    const items = within(screen.getByRole('list', { name: 'メッセージ' })).getAllByRole('listitem')
    expect(items.map((item) => item.querySelector('.message-body')?.textContent)).toEqual([
      '履歴',
      '後から届いた',
      '失敗する',
      '送信中のまま',
    ])
    expect(items.map((item) => item.getAttribute('data-testid'))).toEqual(['message', 'message', 'outgoing', 'outgoing'])
  })
})
