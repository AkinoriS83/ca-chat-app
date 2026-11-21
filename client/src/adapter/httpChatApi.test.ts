import { beforeEach, describe, expect, it, vi } from 'vitest'

import { HttpChatApi } from './httpChatApi'

const wireMessage = {
  participants: ['alice', 'bob'],
  seq: 1,
  sender: { handle: 'alice', displayName: 'Alice' },
  body: 'hello',
  sentAt: '2026-11-21T03:04:05.123456Z',
}
const message = {
  seq: 1,
  sender: { handle: 'alice', displayName: 'Alice' },
  body: 'hello',
  sentAt: '2026-11-21T03:04:05.123456Z',
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('HttpChatApi', () => {
  const fetchMock = vi.fn<typeof fetch>()
  const api = new HttpChatApi()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  describe('listMessages', () => {
    it('利用者を、要求ヘッダーで添えて、履歴を取得する', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { messages: [wireMessage] }))

      const result = await api.listMessages('alice', 'bob')

      expect(fetchMock).toHaveBeenCalledWith('/api/direct-conversations/bob/messages', {
        headers: { 'X-Fixed-User': 'alice' },
      })
      // 通信の形式の Message を、application の Message に詰め替える。
      expect(result).toEqual({ kind: 'loaded', messages: [message] })
    })

    it('相手の識別名を、パスの1つの要素として符号化する', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { messages: [] }))

      await api.listMessages('alice', 'a/b?c#d e')

      expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/direct-conversations/a%2Fb%3Fc%23d%20e/messages')
    })

    it('空の一覧を、取得できる', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { messages: [] }))

      expect(await api.listMessages('alice', 'bob')).toEqual({ kind: 'loaded', messages: [] })
    })

    it('200 のほかは、すべて「取得できなかった」として返す', async () => {
      for (const status of [400, 401, 403, 404, 500, 502]) {
        fetchMock.mockResolvedValue(jsonResponse(status, { code: 'not_found' }))

        expect(await api.listMessages('alice', 'bob')).toEqual({ kind: 'failed' })
      }
    })

    it('要求が失敗した場合と、応答が形式に合わない場合は、「取得できなかった」として返す', async () => {
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      expect(await api.listMessages('alice', 'bob')).toEqual({ kind: 'failed' })

      const malformed: unknown[] = [
        null,
        [],
        {},
        { messages: null },
        { messages: [{ ...wireMessage, seq: '1' }] },
        { messages: [{ ...wireMessage, sender: { handle: 'alice' } }] },
        { messages: [{ ...wireMessage, participants: 'alice,bob' }] },
        { messages: [wireMessage, null] },
      ]
      for (const body of malformed) {
        fetchMock.mockResolvedValue(jsonResponse(200, body))
        expect(await api.listMessages('alice', 'bob')).toEqual({ kind: 'failed' })
      }

      fetchMock.mockResolvedValue(new Response('<html>proxy error</html>', { status: 200 }))
      expect(await api.listMessages('alice', 'bob')).toEqual({ kind: 'failed' })
    })
  })

  describe('sendMessage', () => {
    const request = { actor: 'alice', peer: 'bob', sendId: '0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70', body: 'hello' }

    it('利用者を、要求ヘッダーで添えて、送信の識別子と本文を送る', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { message: wireMessage }))

      const result = await api.sendMessage(request)

      expect(fetchMock).toHaveBeenCalledWith('/api/direct-conversations/bob/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Fixed-User': 'alice' },
        body: JSON.stringify({ sendId: '0b6f2d3e-8a51-4c0e-9f1d-2a7c4e5b6d70', body: 'hello' }),
      })
      expect(result).toEqual({ kind: 'completed', message })
    })

    it('本文を、検証せず、整えずに送る。サーバーが拒否する本文も、そのまま届く', async () => {
      fetchMock.mockResolvedValue(jsonResponse(400, { code: 'invalid_body' }))
      // 空白文字だけの本文、4,001文字の本文、制御文字を含む本文、対にならないサロゲートを含む本文。
      const loneSurrogate = String.fromCharCode(0xd83d)
      const bodies = ['   ', 'a'.repeat(4001), `a${String.fromCharCode(0)}b`, '  前後の空白\r\n', `a${loneSurrogate}b`]

      for (const body of bodies) {
        await api.sendMessage({ ...request, body })
      }

      const sent = fetchMock.mock.calls.map(([, init]) => (JSON.parse(String(init?.body)) as { body: string }).body)
      expect(sent).toEqual(bodies)
      // 対にならないサロゲートは、JSON のエスケープとして送られる(U+FFFD に、置き換わらない)。
      const lastBody = String(fetchMock.mock.calls.at(-1)?.[1]?.body)
      expect(lastBody).toContain(`${String.fromCharCode(0x5c)}ud83d`)
    })

    it('4xx で、ErrorResponse を得た場合は、「拒否」として、理由を返す', async () => {
      const cases = [
        { status: 400, code: 'invalid_body', want: 'invalid_body' },
        { status: 400, code: 'invalid_input', want: 'invalid_input' },
        { status: 400, code: 'invalid_peer', want: 'invalid_peer' },
        { status: 404, code: 'not_found', want: 'not_found' },
        { status: 401, code: 'unauthenticated', want: 'unauthenticated' },
        { status: 403, code: 'forbidden_origin', want: 'other' },
        { status: 400, code: 'some_new_code', want: 'other' },
      ]
      for (const { status, code, want } of cases) {
        fetchMock.mockResolvedValue(jsonResponse(status, { code }))

        expect(await api.sendMessage(request)).toEqual({ kind: 'rejected', code: want })
      }
    })

    it('5xx を得た場合は、「送信できなかった」として返す(サーバーを停止している間の、中継の応答を含む)', async () => {
      fetchMock.mockResolvedValue(jsonResponse(500, { code: 'internal' }))
      expect(await api.sendMessage(request)).toEqual({ kind: 'failed' })

      fetchMock.mockResolvedValue(new Response('', { status: 502 }))
      expect(await api.sendMessage(request)).toEqual({ kind: 'failed' })

      fetchMock.mockResolvedValue(new Response('', { status: 500 }))
      expect(await api.sendMessage(request)).toEqual({ kind: 'failed' })
    })

    it('要求が失敗した場合は、「送信できなかった」として返す', async () => {
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

      expect(await api.sendMessage(request)).toEqual({ kind: 'failed' })
    })

    it('応答が、形式に合わない場合は、「送信できなかった」として返す', async () => {
      const responses = [
        jsonResponse(200, {}),
        jsonResponse(200, { message: { ...wireMessage, seq: 1.5 } }),
        jsonResponse(200, { message: { ...wireMessage, body: null } }),
        new Response('not json', { status: 200 }),
        new Response('not json', { status: 400 }),
        jsonResponse(400, {}),
        jsonResponse(400, { code: 1 }),
        jsonResponse(400, ['invalid_body']),
        jsonResponse(204 + 96, { message: wireMessage }),
      ]
      for (const response of responses) {
        fetchMock.mockResolvedValue(response)

        expect(await api.sendMessage(request)).toEqual({ kind: 'failed' })
      }
    })
  })
})
