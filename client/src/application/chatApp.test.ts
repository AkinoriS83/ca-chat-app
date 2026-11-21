import { describe, expect, it } from 'vitest'

import { ChatApp, SEND_TIMEOUT_MS } from './chatApp'
import type { Conversation } from './model'
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
} from './testPorts'

// テストの名前の C-1 などは、振る舞いの規則の番号である(chatApp.ts の先頭)。

function setup() {
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

  const conversation = (): Conversation => {
    const current = app.getState().conversation
    if (!current) {
      throw new Error('会話を、開いていない')
    }
    return current
  }
  const seqs = (): number[] => conversation().messages.map((m) => m.seq)

  return { app, api, realtime, timer, conversation, seqs }
}

/** alice として、bob との会話を開き、履歴(空)を取得し終えた状態にする。 */
async function setupOpenConversation() {
  const context = setup()
  context.app.selectUser('alice')
  context.realtime.latest.listener.onReady()
  context.app.openConversation('bob')
  context.api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [] })
  await settled()
  return context
}

describe('利用者の選択(C-1、WS-41)', () => {
  it('最初は、利用者を選んでいない。選択肢は、固定ユーザーである', () => {
    const { app, realtime } = setup()

    expect(app.getState()).toEqual({ users: [alice, bob, carol], currentUser: null, conversation: null })
    expect(realtime.connections).toHaveLength(0)
  })

  it('selectUser は、選んだ利用者として、接続を確立する', () => {
    const { app, realtime } = setup()

    app.selectUser('alice')

    expect(app.getState().currentUser).toEqual(alice)
    expect(realtime.connections.map((c) => c.actor)).toEqual(['alice'])
  })

  it('固定ユーザーでない識別名は、選べない', () => {
    const { app, realtime } = setup()

    app.selectUser('mallory')

    expect(app.getState().currentUser).toBeNull()
    expect(realtime.connections).toHaveLength(0)
  })

  it('selectUser は、それまでの接続を閉じ、会話の状態を捨ててから、新しい利用者として接続する', async () => {
    const { app, realtime, conversation } = await setupOpenConversation()
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))
    app.setDraft('書きかけ')
    expect(conversation().messages).toHaveLength(1)
    const asAlice = realtime.latest

    app.selectUser('carol')

    expect(asAlice.closed).toBe(true)
    expect(app.getState().currentUser).toEqual(carol)
    expect(app.getState().conversation).toBeNull()
    expect(realtime.connections.map((c) => c.actor)).toEqual(['alice', 'carol'])
    expect(realtime.latest.closed).toBe(false)
  })

  it('leaveUser は、接続を閉じ、会話の状態を捨てて、利用者を選んでいない状態に戻す', async () => {
    const { app, realtime, timer } = await setupOpenConversation()
    app.setDraft('hello')
    app.send()
    expect(timer.pending).toBe(1)

    app.leaveUser()

    expect(realtime.latest.closed).toBe(true)
    expect(app.getState().currentUser).toBeNull()
    expect(app.getState().conversation).toBeNull()
    expect(timer.pending).toBe(0)
  })

  it('選び直した後、前の利用者の接続に届いた配信は、捨てる', async () => {
    const { app, realtime, seqs } = await setupOpenConversation()
    const asAlice = realtime.latest

    app.selectUser('carol')
    realtime.latest.listener.onReady()
    app.openConversation('alice')
    // 前の接続(alice)に、alice と bob の会話のメッセージと、alice と carol の会話のメッセージが届く。
    asAlice.listener.onMessage(delivery([alice, bob], message(1, bob)))
    asAlice.listener.onMessage(delivery([alice, carol], message(1, alice)))

    expect(seqs()).toEqual([])
  })

  it('選び直した後、前の利用者としての、履歴の取得と送信の結果は、捨てる', async () => {
    const { app, api, realtime, conversation, seqs } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')
    const historyAsAlice = api.historyCalls[0]

    app.selectUser('carol')
    realtime.latest.listener.onReady()
    app.openConversation('bob')
    historyAsAlice?.resolve({ kind: 'loaded', messages: [message(1, alice, 'alice と bob の会話')] })
    await settled()

    expect(seqs()).toEqual([])
    expect(conversation().history).toBe('loading')

    // carol としての送信の後に、alice に選び直す。carol としての結果は、捨てる。
    api.historyCalls[1]?.resolve({ kind: 'loaded', messages: [] })
    await settled()
    app.setDraft('carol から')
    app.send()
    const sendAsCarol = api.sendCalls[0]
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')
    sendAsCarol?.resolve({ kind: 'completed', message: message(1, carol, 'carol から') })
    await settled()

    expect(seqs()).toEqual([])
    expect(conversation().outgoing).toEqual([])
  })

  it('選び直した後、前の接続の ready と切断は、新しい接続の扱いを変えない', () => {
    const { app, api, realtime } = setup()
    app.selectUser('alice')
    const asAlice = realtime.latest
    app.selectUser('carol')
    app.openConversation('alice')

    asAlice.listener.onReady()
    asAlice.listener.onClosed()

    // carol としての接続は、まだ ready を受けていない。履歴は、取得しない。
    expect(api.historyCalls).toHaveLength(0)
  })
})

describe('会話を開く(C-2、ADR-010)', () => {
  it('利用者を選んでいなければ、開かない。自分自身と、固定ユーザーでない相手は、開かない', () => {
    const { app, realtime } = setup()

    app.openConversation('bob')
    expect(app.getState().conversation).toBeNull()

    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('alice')
    app.openConversation('mallory')
    expect(app.getState().conversation).toBeNull()
  })

  it('ready を受けた後であれば、すぐに履歴を取得する', () => {
    const { app, api, realtime, conversation } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()

    app.openConversation('bob')

    expect(conversation()).toEqual({
      peer: bob,
      history: 'loading',
      messages: [],
      outgoing: [],
      draft: '',
      rejection: null,
    })
    expect(api.historyCalls.map((c) => c.request)).toEqual([{ actor: 'alice', peer: 'bob' }])
  })

  it('接続を確立している途中であれば、ready を待ってから、履歴を取得する', () => {
    const { app, api, realtime, conversation } = setup()
    app.selectUser('alice')

    app.openConversation('bob')

    expect(conversation().history).toBe('loading')
    expect(api.historyCalls).toHaveLength(0)

    realtime.latest.listener.onReady()

    expect(api.historyCalls).toHaveLength(1)
  })

  it('ready を待つ間に、接続の確立に失敗した場合は、その時点で取得する', () => {
    const { app, api, realtime } = setup()
    app.selectUser('alice')
    app.openConversation('bob')

    realtime.latest.listener.onClosed()

    expect(api.historyCalls).toHaveLength(1)
  })

  it('接続が切れている場合は、待たずに取得する', () => {
    const { app, api, realtime } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    realtime.latest.listener.onClosed()

    app.openConversation('bob')

    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴を取得すると、連番の順に示す(WS-20)。メッセージがなければ、空のままである(WS-21)', async () => {
    const { app, api, realtime, conversation, seqs } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')

    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, alice), message(2, bob), message(3, alice)] })
    await settled()

    expect(conversation().history).toBe('loaded')
    expect(seqs()).toEqual([1, 2, 3])

    app.openConversation('carol')
    api.historyCalls[1]?.resolve({ kind: 'loaded', messages: [] })
    await settled()

    expect(conversation().history).toBe('loaded')
    expect(seqs()).toEqual([])
  })

  it('履歴を取得できなかった場合は、failed にする', async () => {
    const first = setup()
    first.app.selectUser('alice')
    first.realtime.latest.listener.onReady()
    first.app.openConversation('bob')
    first.api.historyCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(first.conversation().history).toBe('failed')

    const second = setup()
    second.app.selectUser('alice')
    second.realtime.latest.listener.onReady()
    second.app.openConversation('bob')
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

  it('会話を開いていないときの配信は、捨てる', () => {
    const { app, realtime } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()

    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, bob)))
    app.openConversation('bob')

    expect(app.getState().conversation?.messages).toEqual([])
  })

  it('履歴の取得中に届いた配信を、すぐに加え、履歴と統合する。欠けず、重複しない', async () => {
    const { app, api, realtime, seqs } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')

    // 履歴の読み取りと前後して、連番3と4が保管された。3は、履歴にも、配信にも現れる。
    realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob)))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(4, bob)))
    expect(seqs()).toEqual([3, 4])

    api.historyCalls[0]?.resolve({ kind: 'loaded', messages: [message(1, alice), message(2, bob), message(3, bob)] })
    await settled()

    expect(seqs()).toEqual([1, 2, 3, 4])
  })

  it('履歴の後に、同じメッセージが配信で届いても、重複しない', async () => {
    const { app, api, realtime, seqs } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')
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
      { actor: 'alice', peer: 'bob', sendId: 'send-1', body: '  こんにちは\n' },
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

    expect(api.sendCalls[1]?.request).toEqual({ actor: 'alice', peer: 'bob', sendId: 'send-2', body: '直した本文' })
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
    realtime.latest.listener.onClosed()
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
      { actor: 'alice', peer: 'bob', sendId: 'send-1', body: '送り直す本文' },
      { actor: 'alice', peer: 'bob', sendId: 'send-1', body: '送り直す本文' },
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

    app.openConversation('carol')

    expect(conversation()).toEqual({
      peer: carol,
      history: 'loading',
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

    app.openConversation('carol')
    api.sendCalls[0]?.resolve({ kind: 'completed', message: message(1, alice, 'bob へ') })
    realtime.latest.listener.onMessage(delivery([alice, bob], message(1, alice, 'bob へ'), 'send-1'))
    realtime.latest.listener.onMessage(delivery([alice, bob], message(2, bob)))
    await settled()

    expect(seqs()).toEqual([])
    expect(conversation().outgoing).toEqual([])
    expect(conversation().rejection).toBeNull()
  })

  it('前の会話の履歴の取得の結果は、捨てる', async () => {
    const { app, api, realtime, conversation, seqs } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')
    app.openConversation('carol')

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

    app.openConversation('carol')
    app.openConversation('bob')
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

    app.openConversation('bob')

    expect(conversation()).toBe(before)
    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴の取得中は、何もしない', () => {
    const { app, api, realtime } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')

    app.openConversation('bob')

    expect(api.historyCalls).toHaveLength(1)
  })

  it('履歴を取得できなかった場合は、会話の状態を保ったまま、取得し直す', async () => {
    const { app, api, realtime, conversation, seqs } = setup()
    app.selectUser('alice')
    realtime.latest.listener.onReady()
    app.openConversation('bob')
    api.historyCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    // 取得できなかった間にも、配信は届き、入力はできる。
    realtime.latest.listener.onMessage(delivery([alice, bob], message(3, bob)))
    app.setDraft('書きかけ')

    app.openConversation('bob')

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
    const { app, api, realtime, conversation } = setup()
    app.selectUser('alice')
    app.openConversation('bob')
    realtime.latest.listener.onClosed()
    api.historyCalls[0]?.resolve({ kind: 'failed' })
    await settled()
    expect(conversation().history).toBe('failed')

    // 接続は、切れている。待たずに、取得し直す。
    app.openConversation('bob')

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

    app.selectUser('alice')
    expect(notified).toBe(1)
    expect(app.getState()).not.toBe(before)

    realtime.latest.listener.onReady()
    app.openConversation('bob')
    app.setDraft('hello')
    expect(notified).toBe(3)

    unsubscribe()
    app.setDraft('bye')
    expect(notified).toBe(3)
  })
})
