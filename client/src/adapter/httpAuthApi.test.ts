import { beforeEach, describe, expect, it, vi } from 'vitest'

import { HttpAuthApi } from './httpAuthApi'

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

const wireSession = { sessionId: 'session-1', user: { handle: 'Alice_01', displayName: 'Alice' } }
const session = { sessionId: 'session-1', user: { handle: 'Alice_01', displayName: 'Alice' } }
const json = { 'Content-Type': 'application/json' }

describe('HttpAuthApi', () => {
  const fetchMock = vi.fn<typeof fetch>()
  const api = new HttpAuthApi()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  describe('currentSession(RL-21、RL-23)', () => {
    it('ログイン中であれば、利用者と、ログイン状態の識別子を返す。何も添えない(資格は、ブラウザが付ける)', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { authenticated: true, session: wireSession }))

      expect(await api.currentSession()).toEqual({ kind: 'signedIn', session })
      expect(fetchMock).toHaveBeenCalledWith('/api/session')
    })

    it('ログインしていなければ、そのことを返す', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { authenticated: false }))

      expect(await api.currentSession()).toEqual({ kind: 'signedOut' })
    })

    it('確かめられなかった場合は、ログインしていないとは、返さない', async () => {
      const responses = [
        jsonResponse(500, { code: 'internal' }),
        jsonResponse(403, { code: 'forbidden_origin' }),
        jsonResponse(200, {}),
        jsonResponse(200, { authenticated: 'no' }),
        jsonResponse(200, { authenticated: true }),
        jsonResponse(200, { authenticated: true, session: { sessionId: '', user: wireSession.user } }),
        jsonResponse(200, { authenticated: true, session: { sessionId: 's', user: { handle: 'a' } } }),
        new Response('<html>proxy error</html>', { status: 200 }),
      ]
      for (const response of responses) {
        fetchMock.mockResolvedValue(response)
        expect(await api.currentSession()).toEqual({ kind: 'failed' })
      }
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      expect(await api.currentSession()).toEqual({ kind: 'failed' })
    })
  })

  describe('login(RL-12、RL-13)', () => {
    it('連絡先と秘密を、検証せず、整えずに送る。完了で、利用者と識別子を返す', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, wireSession))

      const result = await api.login('  Alice@Example.com ', ' secret  ')

      expect(fetchMock).toHaveBeenCalledWith('/api/session', {
        method: 'POST',
        headers: json,
        body: JSON.stringify({ contact: '  Alice@Example.com ', secret: ' secret  ' }),
      })
      expect(result).toEqual({ kind: 'completed', session })
    })

    it('拒否(照合の失敗、上限)と、完了できなかった場合を、分けて返す', async () => {
      fetchMock.mockResolvedValue(jsonResponse(401, { code: 'login_failed' }))
      expect(await api.login('a@example.com', 'x')).toEqual({ kind: 'rejected', code: 'login_failed', fields: [] })

      fetchMock.mockResolvedValue(jsonResponse(429, { code: 'rate_limited' }))
      expect(await api.login('a@example.com', 'x')).toEqual({ kind: 'rejected', code: 'rate_limited', fields: [] })

      for (const response of [jsonResponse(500, { code: 'internal' }), jsonResponse(200, { sessionId: 's' }), new Response('oops', { status: 401 })]) {
        fetchMock.mockResolvedValue(response)
        expect(await api.login('a@example.com', 'x')).toEqual({ kind: 'failed' })
      }
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      expect(await api.login('a@example.com', 'x')).toEqual({ kind: 'failed' })
    })
  })

  describe('logout(RL-17、RL-22)', () => {
    it('ログイン状態の識別子を、要求ヘッダーで添える。204 で、完了とする', async () => {
      fetchMock.mockResolvedValue(new Response(null, { status: 204 }))

      expect(await api.logout('session-1')).toEqual({ kind: 'completed' })
      expect(fetchMock).toHaveBeenCalledWith('/api/session', { method: 'DELETE', headers: { 'X-Session-Id': 'session-1' } })
    })

    it('認証の拒否と、完了できなかった場合を、分けて返す', async () => {
      fetchMock.mockResolvedValue(jsonResponse(401, { code: 'unauthenticated' }))
      expect(await api.logout('session-1')).toEqual({ kind: 'rejected', code: 'unauthenticated', fields: [] })

      fetchMock.mockResolvedValue(jsonResponse(500, { code: 'internal' }))
      expect(await api.logout('session-1')).toEqual({ kind: 'failed' })
      // 204 のほかの、完了の状態は、完了として扱わない。
      fetchMock.mockResolvedValue(jsonResponse(200, {}))
      expect(await api.logout('session-1')).toEqual({ kind: 'failed' })
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      expect(await api.logout('session-1')).toEqual({ kind: 'failed' })
    })
  })

  describe('requestSignup と completeSignup(RL-01、RL-06)', () => {
    it('申し込みは、連絡先を、そのまま送る', async () => {
      fetchMock.mockResolvedValue(new Response(null, { status: 204 }))

      expect(await api.requestSignup(' Dave@Example.com')).toEqual({ kind: 'completed' })
      expect(fetchMock).toHaveBeenCalledWith('/api/signups', {
        method: 'POST',
        headers: json,
        body: JSON.stringify({ contact: ' Dave@Example.com' }),
      })
    })

    it('完了は、確認のための資格を、要求の本文で送る。パスとクエリには、載せない(RL-33)', async () => {
      fetchMock.mockResolvedValue(new Response(null, { status: 204 }))

      const result = await api.completeSignup({ credential: 'signup-credential', handle: 'Dave_01', displayName: ' Dave ', secret: 'dave secret' })

      expect(result).toEqual({ kind: 'completed' })
      expect(fetchMock).toHaveBeenCalledWith('/api/users', {
        method: 'POST',
        headers: json,
        body: JSON.stringify({ signupCredential: 'signup-credential', handle: 'Dave_01', displayName: ' Dave ', secret: 'dave secret' }),
      })
    })

    it('拒否では、理由と、正しくない項目を返す。知らない項目は、除く', async () => {
      fetchMock.mockResolvedValue(jsonResponse(400, { code: 'invalid_fields', fields: ['handle', 'secret', 'unknown', 1] }))
      expect(await api.completeSignup({ credential: 'c', handle: '!', displayName: 'D', secret: 'x' })).toEqual({
        kind: 'rejected',
        code: 'invalid_fields',
        fields: ['handle', 'secret'],
      })

      fetchMock.mockResolvedValue(jsonResponse(400, { code: 'invalid_fields', fields: ['contact'] }))
      expect(await api.requestSignup('x')).toEqual({ kind: 'rejected', code: 'invalid_fields', fields: ['contact'] })

      for (const [status, code] of [[409, 'handle_taken'], [400, 'invalid_signup_credential'], [429, 'rate_limited']] as const) {
        fetchMock.mockResolvedValue(jsonResponse(status, { code }))
        expect(await api.completeSignup({ credential: 'c', handle: 'h', displayName: 'D', secret: 'x' })).toEqual({ kind: 'rejected', code, fields: [] })
      }
    })

    it('案内を送れなかった場合(5xx)と、要求が失敗した場合は、完了できなかったこととして返す(RL-10)', async () => {
      fetchMock.mockResolvedValue(jsonResponse(500, { code: 'internal' }))
      expect(await api.requestSignup('dave@example.com')).toEqual({ kind: 'failed' })

      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      expect(await api.requestSignup('dave@example.com')).toEqual({ kind: 'failed' })
      expect(await api.completeSignup({ credential: 'c', handle: 'h', displayName: 'D', secret: 'x' })).toEqual({ kind: 'failed' })
    })
  })
})
