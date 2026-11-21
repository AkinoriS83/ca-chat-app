import { describe, expect, it } from 'vitest'

import { isBetween, mergeMessage, mergeMessages } from './messages'
import type { Message } from './model'

function message(seq: number, body = `m${seq}`): Message {
  return { seq, sender: { handle: 'alice', displayName: 'Alice' }, body, sentAt: '2026-11-21T03:04:05.123456Z' }
}

function seqs(messages: Message[]): number[] {
  return messages.map((m) => m.seq)
}

describe('mergeMessage(C-3)', () => {
  it('連番の順に加える。届いた順には、依らない', () => {
    const orders = [
      [1, 2, 3, 4],
      [4, 3, 2, 1],
      [2, 4, 1, 3],
      [3, 1, 4, 2],
    ]
    for (const order of orders) {
      const merged = order.reduce<Message[]>((acc, seq) => mergeMessage(acc, message(seq)), [])
      expect(seqs(merged)).toEqual([1, 2, 3, 4])
    }
  })

  it('同じ連番のものが既にあれば、加えない', () => {
    const first = [message(1, 'first'), message(2)]

    const merged = mergeMessage(first, message(1, 'second'))

    expect(merged).toBe(first)
    expect(merged[0]?.body).toBe('first')
  })

  it('元の配列を、変えない', () => {
    const original = [message(1), message(3)]

    const merged = mergeMessage(original, message(2))

    expect(seqs(original)).toEqual([1, 3])
    expect(seqs(merged)).toEqual([1, 2, 3])
  })
})

describe('mergeMessages(C-3)', () => {
  it('履歴と、先に届いた配信を、欠けず、重複せずに統合する', () => {
    const delivered = [message(3), message(4)]
    const history = [message(1), message(2), message(3)]

    expect(seqs(mergeMessages(delivered, history))).toEqual([1, 2, 3, 4])
    expect(seqs(mergeMessages(history, delivered))).toEqual([1, 2, 3, 4])
  })

  it('空の履歴は、何も変えない', () => {
    const delivered = [message(1)]

    expect(mergeMessages(delivered, [])).toBe(delivered)
  })
})

describe('isBetween(C-4)', () => {
  it('自分と相手の組に、一致する。順は、問わない', () => {
    expect(isBetween(['alice', 'bob'], 'alice', 'bob')).toBe(true)
    expect(isBetween(['alice', 'bob'], 'bob', 'alice')).toBe(true)
  })

  it('別の組には、一致しない', () => {
    expect(isBetween(['alice', 'carol'], 'alice', 'bob')).toBe(false)
    expect(isBetween(['bob', 'carol'], 'alice', 'bob')).toBe(false)
    expect(isBetween(['alice'], 'alice', 'bob')).toBe(false)
    expect(isBetween(['alice', 'bob', 'carol'], 'alice', 'bob')).toBe(false)
    expect(isBetween(['alice', 'alice'], 'alice', 'bob')).toBe(false)
    expect(isBetween([], 'alice', 'bob')).toBe(false)
  })
})
