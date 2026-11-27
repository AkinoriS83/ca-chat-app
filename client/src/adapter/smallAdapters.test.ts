// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BrowserTimer } from './browserTimer'
import { CryptoSendIdSource } from './cryptoSendIdSource'
import { LocationEntryLink } from './locationEntryLink'

describe('LocationEntryLink(RL-33)', () => {
  afterEach(() => {
    history.replaceState(null, '', '/')
  })

  it('案内の資格を、アドレスの # より後から読み取り、アドレスから除く', () => {
    history.replaceState({ kept: true }, '', '/?x=1#signup=AbC-_123')
    const entries = history.length

    expect(new LocationEntryLink().takeSignupCredential()).toBe('AbC-_123')

    expect(location.hash).toBe('')
    expect(location.pathname + location.search).toBe('/?x=1')
    // 履歴の項目を、増やさない。
    expect(history.length).toBe(entries)
    expect(history.state).toEqual({ kept: true })
    // もう一度読み取っても、資格は、ない。
    expect(new LocationEntryLink().takeSignupCredential()).toBeNull()
  })

  it('資格がないアドレスでは、null を返し、アドレスを変えない', () => {
    for (const url of ['/', '/#', '/#other=1', '/?signup=abc', '/#xsignup=abc']) {
      history.replaceState(null, '', url)

      expect(new LocationEntryLink().takeSignupCredential()).toBeNull()
      expect(location.pathname + location.search + location.hash).toBe(url === '/#' ? '/' : url)
    }
  })

  it('資格が空の場合は、null を返し、アドレスからは、除く', () => {
    history.replaceState(null, '', '/#signup=')

    expect(new LocationEntryLink().takeSignupCredential()).toBeNull()
    expect(location.hash).toBe('')
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
