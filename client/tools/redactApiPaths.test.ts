import type { Logger } from 'vite'
import { describe, expect, it, vi } from 'vitest'

import { REDACTED_API_PATH, redactApiPaths, redactingLogger } from './redactApiPaths'

// RL-33、DD-36: クライアントの開発用のサーバーの記録に、識別名(要求のパスの一部)が、含まれない。

describe('redactApiPaths', () => {
  it.each([
    ['http proxy error: /api/users/Alice_01', `http proxy error: ${REDACTED_API_PATH}`],
    ['http proxy error: /api/direct-conversations/bob/messages', `http proxy error: ${REDACTED_API_PATH}`],
    ['ws proxy error: /api/realtime?session=0b6f2d3e-8a51', `ws proxy error: ${REDACTED_API_PATH}`],
    // 文言の形に、依らない。
    ['failed (GET "/api/users/carol") after 3ms', `failed (GET "${REDACTED_API_PATH}") after 3ms`],
    ['[vite] proxied http://localhost:5173/api/users/dave -> 502', `[vite] proxied http://localhost:5173${REDACTED_API_PATH} -> 502`],
    ['/api/users/a then /api/users/b', `${REDACTED_API_PATH} then ${REDACTED_API_PATH}`],
    ['/api/users/%E3%81%82?x=1#frag end', `${REDACTED_API_PATH} end`],
  ])('%s', (message, expected) => {
    expect(redactApiPaths(message)).toBe(expected)
  })

  it('/api/ で始まらないパスと、ほかの文言は、変えない', () => {
    for (const message of ['page reload src/ui/App.tsx', 'hmr update /src/ui/style.css', 'GET /apiary/1', 'Local: http://127.0.0.1:5173/']) {
      expect(redactApiPaths(message)).toBe(message)
    }
  })
})

describe('redactingLogger', () => {
  function fakeLogger() {
    const logger: Logger = {
      info: vi.fn(),
      warn: vi.fn(),
      warnOnce: vi.fn(),
      error: vi.fn(),
      clearScreen: vi.fn(),
      hasErrorLogged: vi.fn(() => false),
      hasWarned: false,
    }
    return logger
  }

  it('どの水準の記録からも、/api/ で始まるパスを伏せる', () => {
    const inner = fakeLogger()
    const logger = redactingLogger(inner)

    logger.info('info /api/users/Alice_01')
    logger.warn('warn /api/users/Alice_01')
    logger.warnOnce('once /api/users/Alice_01')
    logger.error('error /api/users/Alice_01', { timestamp: true })

    for (const write of [inner.info, inner.warn, inner.warnOnce, inner.error]) {
      const [message] = vi.mocked(write).mock.calls[0] ?? []
      expect(message).toContain(REDACTED_API_PATH)
      expect(message).not.toContain('Alice_01')
    }
    expect(vi.mocked(inner.error).mock.calls[0]?.[1]).toEqual({ timestamp: true })
  })

  it('添えられたエラーの文言と、呼び出しの経路からも、伏せる', () => {
    const inner = fakeLogger()
    const error = new Error('connect ECONNREFUSED while proxying /api/users/Alice_01')
    error.stack = `Error: ${error.message}\n    at proxy (/api/users/Alice_01)`

    redactingLogger(inner).error('http proxy error: /api/users/Alice_01', { error })

    const [message, options] = vi.mocked(inner.error).mock.calls[0] ?? []
    expect(`${message} ${options?.error?.message} ${options?.error?.stack}`).not.toContain('Alice_01')
  })

  it('そのほかの操作は、そのまま、元の出力先に渡す', () => {
    const inner = fakeLogger()
    const logger = redactingLogger(inner)

    logger.clearScreen('error')
    logger.hasErrorLogged(new Error('x'))

    expect(inner.clearScreen).toHaveBeenCalledWith('error')
    expect(inner.hasErrorLogged).toHaveBeenCalled()
  })
})
