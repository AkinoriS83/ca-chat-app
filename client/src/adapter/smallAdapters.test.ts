import { afterEach, describe, expect, it, vi } from 'vitest'

import { BrowserTimer } from './browserTimer'
import { CryptoSendIdSource } from './cryptoSendIdSource'
import { StaticFixedUsers } from './staticFixedUsers'

describe('StaticFixedUsers', () => {
  it('固定ユーザーの3人を返す。サーバーの固定ユーザーと、同じ識別名と表示名を持つ', () => {
    expect(new StaticFixedUsers().list()).toEqual([
      { handle: 'alice', displayName: 'Alice' },
      { handle: 'bob', displayName: 'Bob' },
      { handle: 'carol', displayName: 'Carol' },
    ])
  })

  it('返した一覧を変えても、次に返す一覧は、変わらない', () => {
    const directory = new StaticFixedUsers()
    const first = directory.list()
    first[0]!.displayName = 'changed'
    first.pop()

    expect(directory.list()).toHaveLength(3)
    expect(directory.list()[0]?.displayName).toBe('Alice')
  })
})

describe('CryptoSendIdSource', () => {
  it('UUID の形の、呼ぶたびに異なる識別子を作る(ADR-009)', () => {
    const source = new CryptoSendIdSource()

    const ids = new Set(Array.from({ length: 100 }, () => source.next()))

    expect(ids.size).toBe(100)
    for (const id of ids) {
      expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/)
    }
  })
})

describe('BrowserTimer', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('指定した時間が過ぎたときに、1回だけ呼ぶ', () => {
    vi.useFakeTimers()
    const onElapsed = vi.fn()

    new BrowserTimer().start(10_000, onElapsed)

    vi.advanceTimersByTime(9_999)
    expect(onElapsed).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onElapsed).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(60_000)
    expect(onElapsed).toHaveBeenCalledTimes(1)
  })

  it('待つのをやめると、呼ばない', () => {
    vi.useFakeTimers()
    const onElapsed = vi.fn()

    const cancel = new BrowserTimer().start(10_000, onElapsed)
    cancel()
    vi.advanceTimersByTime(60_000)

    expect(onElapsed).not.toHaveBeenCalled()
  })
})
