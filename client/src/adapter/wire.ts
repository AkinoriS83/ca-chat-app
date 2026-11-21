// 通信の形式(生成した型)の値を、検査して、application の型に詰め替える。
// 応答とイベントは、形式に合わないことがありうる(中継の応答、版の食い違いなど)ため、形を確かめる。
import type { Message, RejectionCode } from '../application/model'
import type { components } from './generated/openapi'

type WireMessage = components['schemas']['Message']
type WireErrorCode = components['schemas']['ErrorCode']

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** 値が、通信の形式の Message であるかを返す。 */
export function isWireMessage(value: unknown): value is WireMessage {
  if (!isRecord(value)) {
    return false
  }
  const { participants, seq, sender, body, sentAt } = value
  return (
    Array.isArray(participants) &&
    participants.every((p) => typeof p === 'string') &&
    typeof seq === 'number' &&
    Number.isSafeInteger(seq) &&
    isRecord(sender) &&
    typeof sender.handle === 'string' &&
    typeof sender.displayName === 'string' &&
    typeof body === 'string' &&
    typeof sentAt === 'string'
  )
}

/** 通信の形式の Message を、application の Message に詰め替える。 */
export function toMessage(wire: WireMessage): Message {
  return {
    seq: wire.seq,
    sender: { handle: wire.sender.handle, displayName: wire.sender.displayName },
    body: wire.body,
    sentAt: wire.sentAt,
  }
}

// 利用者に、理由を示し分ける code。ほかは、'other' とする。
const rejectionCodes: Partial<Record<WireErrorCode, RejectionCode>> = {
  invalid_body: 'invalid_body',
  invalid_input: 'invalid_input',
  invalid_peer: 'invalid_peer',
  not_found: 'not_found',
  unauthenticated: 'unauthenticated',
}

/** 拒否の応答(ErrorResponse)の code を、拒否の理由に詰め替える。形式に合わなければ、undefined を返す。 */
export function toRejectionCode(value: unknown): RejectionCode | undefined {
  if (!isRecord(value) || typeof value.code !== 'string') {
    return undefined
  }
  return rejectionCodes[value.code as WireErrorCode] ?? 'other'
}

export { isRecord }
