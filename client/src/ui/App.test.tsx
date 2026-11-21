// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ChatApp, SEND_TIMEOUT_MS } from '../application/chatApp'
import {
  FakeChatApi,
  FakeFixedUserDirectory,
  FakeRealtimeChannel,
  FakeSendIdSource,
  FakeTimer,
  alice,
  bob,
  carol,
  delivery,
  message,
  settled,
} from '../application/testPorts'
import { App } from './App'

afterEach(cleanup)

function renderApp() {
  const api = new FakeChatApi()
  const realtime = new FakeRealtimeChannel()
  const timer = new FakeTimer()
  const app = new ChatApp({
    api,
    realtime,
    timer,
    sendIds: new FakeSendIdSource(),
    users: new FakeFixedUserDirectory(),
  })
  const view = render(<App app={app} />)
  return { app, api, realtime, timer, ...view }
}

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

function composer(): HTMLTextAreaElement {
  return screen.getByRole<HTMLTextAreaElement>('textbox', { name: 'メッセージ' })
}

function type(text: string): void {
  fireEvent.change(composer(), { target: { value: text } })
}

/** alice として、bob との会話を開き、履歴を取得し終えた状態にする。 */
async function openConversation(history = [] as ReturnType<typeof message>[]) {
  const context = renderApp()
  click('Alice')
  await happen(() => context.realtime.latest.listener.onReady())
  click('Bob')
  await happen(() => context.api.historyCalls[0]?.resolve({ kind: 'loaded', messages: history }))
  return context
}

describe('利用者の選択', () => {
  it('利用者を選んでいないとき、固定ユーザー3人のボタンを示す', () => {
    renderApp()

    const group = screen.getByRole('group', { name: '利用者を選んでください。' })
    expect(within(group).getAllByRole('button').map((b) => b.textContent)).toEqual(['Alice', 'Bob', 'Carol'])
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('選ぶと、選んだ利用者の表示名と、自分以外の相手を示す', () => {
    const { realtime } = renderApp()

    click('Alice')

    expect(screen.getByTestId('current-user').textContent).toBe('Alice')
    const peers = within(screen.getByRole('navigation', { name: '相手' })).getAllByRole('button')
    expect(peers.map((b) => b.textContent)).toEqual(['Bob', 'Carol'])
    expect(realtime.latest.actor).toBe('alice')
    expect(screen.getByText('相手を選んでください。')).toBeTruthy()
  })

  it('「利用者を選び直す」で、利用者の選択に戻り、それまでの接続を閉じる(WS-41)', async () => {
    const { realtime } = await openConversation([message(1, bob, 'alice と bob の会話')])
    const asAlice = realtime.latest

    click('利用者を選び直す')

    expect(asAlice.closed).toBe(true)
    expect(screen.getByRole('group', { name: '利用者を選んでください。' })).toBeTruthy()
    expect(screen.queryByText('alice と bob の会話')).toBeNull()

    // carol として選び直した後、alice と bob の会話のメッセージは、示されない。
    click('Carol')
    await happen(() => realtime.latest.listener.onReady())
    click('Alice')
    await happen(() => {
      asAlice.listener.onMessage(delivery([alice, bob], message(2, bob, '前の接続に届いた')))
      realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob, '宛先でない配信')))
    })

    expect(screen.getByTestId('current-user').textContent).toBe('Carol')
    expect(screen.queryByText('前の接続に届いた')).toBeNull()
    expect(screen.queryByText('宛先でない配信')).toBeNull()
  })
})

describe('会話', () => {
  it('選んでいる相手を、見分けられるように示す', async () => {
    await openConversation()

    expect(screen.getByRole('button', { name: 'Bob' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: 'Carol' }).getAttribute('aria-pressed')).toBe('false')
    expect(screen.getByRole('region', { name: 'Bob との会話' })).toBeTruthy()
  })

  it('取得中であることを示す', async () => {
    const { realtime } = renderApp()
    click('Alice')
    await happen(() => realtime.latest.listener.onReady())

    click('Bob')

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
    const { api, realtime } = renderApp()
    click('Alice')
    await happen(() => realtime.latest.listener.onReady())
    click('Bob')
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

    expect(api.sendCalls[0]?.request).toEqual({ actor: 'alice', peer: 'bob', sendId: 'send-1', body: 'こんにちは' })
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
      { code: 'unauthenticated', text: '利用者を確認できないため、送信できませんでした。利用者を選び直してください。' },
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
      { actor: 'alice', peer: 'bob', sendId: 'send-1', body: '届かない本文' },
      { actor: 'alice', peer: 'bob', sendId: 'send-1', body: '届かない本文' },
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
