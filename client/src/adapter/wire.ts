// 通信の形式(生成した型)の値を、検査して、application の型に詰め替える。
// 応答とイベントは、形式に合わないことがありうる(中継の応答、版の食い違いなど)ため、形を確かめる。
import type { InputField, Message, RejectionCode, UserProfile } from '../application/model'
import type { SessionInfo } from '../application/port/authApi'
import type { components } from './generated/openapi'

type WireMessage = components['schemas']['Message']
type WireErrorCode = components['schemas']['ErrorCode']
type WireInputField = components['schemas']['InputField']
type WireUserProfile = components['schemas']['UserProfile']
type WireSession = components['schemas']['Session']

/** ログイン状態の識別子を添える、要求ヘッダーの名前(ADR-014)。サーバー(httpapi)と、同じ値を持つ。 */
export const sessionIdHeader = 'X-Session-Id'

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

/** 値が、通信の形式の UserProfile であるかを返す。 */
export function isWireUserProfile(value: unknown): value is WireUserProfile {
  return isRecord(value) && typeof value.handle === 'string' && typeof value.displayName === 'string'
}

/** 通信の形式の UserProfile を、application の UserProfile に詰め替える。識別名と表示名だけを写す。 */
export function toUserProfile(wire: WireUserProfile): UserProfile {
  return { handle: wire.handle, displayName: wire.displayName }
}

/** 値が、通信の形式の Session であるかを返す。 */
export function isWireSession(value: unknown): value is WireSession {
  return isRecord(value) && typeof value.sessionId === 'string' && value.sessionId !== '' && isWireUserProfile(value.user)
}

/** 通信の形式の Session を、application の SessionInfo に詰め替える。 */
export function toSessionInfo(wire: WireSession): SessionInfo {
  return { sessionId: wire.sessionId, user: toUserProfile(wire.user) }
}

// 利用者に、理由を示し分ける code。ほかは、'other' とする。
const rejectionCodes: Partial<Record<WireErrorCode, RejectionCode>> = {
  invalid_body: 'invalid_body',
  invalid_input: 'invalid_input',
  invalid_peer: 'invalid_peer',
  not_found: 'not_found',
  unauthenticated: 'unauthenticated',
  invalid_fields: 'invalid_fields',
  handle_taken: 'handle_taken',
  invalid_signup_credential: 'invalid_signup_credential',
  login_failed: 'login_failed',
  rate_limited: 'rate_limited',
}

const inputFields: Record<WireInputField, InputField> = {
  contact: 'contact',
  handle: 'handle',
  displayName: 'displayName',
  secret: 'secret',
}

/**
 * 拒否の応答(ErrorResponse)の fields を、正しくない項目の一覧に詰め替える。
 * 項目がない場合と、知らない値は、除く。
 */
export function toInputFields(value: unknown): InputField[] {
  if (!isRecord(value) || !Array.isArray(value.fields)) {
    return []
  }
  const fields: InputField[] = []
  for (const field of value.fields) {
    if (typeof field === 'string' && Object.hasOwn(inputFields, field)) {
      fields.push(inputFields[field as WireInputField])
    }
  }
  return fields
}

/** 拒否の応答(ErrorResponse)の code を、拒否の理由に詰め替える。形式に合わなければ、undefined を返す。 */
export function toRejectionCode(value: unknown): RejectionCode | undefined {
  if (!isRecord(value) || typeof value.code !== 'string') {
    return undefined
  }
  // 一覧が持つ code だけを、詰め替える。Object から継承した名前(constructor など)は、一覧にないものとして扱う。
  return Object.hasOwn(rejectionCodes, value.code) ? rejectionCodes[value.code as WireErrorCode] : 'other'
}

/**
 * 4xx の応答を、拒否(code と fields)に詰め替える。形式に合わなければ、undefined を返す。
 * 応答の本文を読めない場合も、undefined を返す。
 */
export async function toRejection(response: Response): Promise<{ code: RejectionCode; fields: InputField[] } | undefined> {
  if (response.status < 400 || response.status >= 500) {
    return undefined
  }
  const body: unknown = await response.json()
  const code = toRejectionCode(body)
  return code ? { code, fields: toInputFields(body) } : undefined
}

export { isRecord }
