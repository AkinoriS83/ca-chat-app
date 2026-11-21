import type { Message } from './model'

/**
 * メッセージを、連番をキーとして加える。同じ連番のものが既にあれば、加えない。
 * 並びは、連番の順とする。届いた順には、依らない(ADR-010)。
 *
 * 履歴、完了の応答、配信のどれで届いたメッセージも、この関数で統合する。
 */
export function mergeMessage(messages: Message[], incoming: Message): Message[] {
  // 多くの場合、末尾に加わる。末尾から探す。
  let index = messages.length
  while (index > 0) {
    const previous = messages[index - 1]
    if (previous === undefined || previous.seq < incoming.seq) {
      break
    }
    if (previous.seq === incoming.seq) {
      return messages
    }
    index--
  }
  return [...messages.slice(0, index), incoming, ...messages.slice(index)]
}

/** 複数のメッセージを、mergeMessage で加える。 */
export function mergeMessages(messages: Message[], incoming: Message[]): Message[] {
  return incoming.reduce(mergeMessage, messages)
}

/** 配信の participants が、自分と、開いている会話の相手の組に、一致するかを返す。 */
export function isBetween(participants: string[], self: string, peer: string): boolean {
  if (participants.length !== 2) {
    return false
  }
  const [first, second] = participants
  return (first === self && second === peer) || (first === peer && second === self)
}
