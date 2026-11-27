import type {
  AuthApi,
  CommandResult,
  CompleteSignupRequest,
  LoginResult,
  SessionCheck,
} from '../application/port/authApi'
import type { components } from './generated/openapi'
import { isRecord, isWireSession, sessionIdHeader, toRejection, toSessionInfo } from './wire'

type SignupRequest = components['schemas']['SignupRequest']
type WireCompleteSignupRequest = components['schemas']['CompleteSignupRequest']
type LoginRequest = components['schemas']['LoginRequest']

const sessionUrl = '/api/session'
const jsonHeaders = { 'Content-Type': 'application/json' }

/**
 * 登録、ログイン、ログイン状態の操作を、HTTP で行う(ADR-002)。
 *
 * ログイン状態を示す資格は、ログインの応答が、Cookie として与え、ブラウザが保つ。このコードは、資格に
 * 触れない(ADR-014)。fetch は、既定で、同じオリジンの要求に、Cookie を付ける。
 * 入力(連絡先、識別名、表示名、秘密)は、検証せず、整えずに送る。検証は、サーバーが行う。
 * 秘密と資格を、コンソールに出力しない。
 *
 * 結果は、完了、拒否(4xx で、ErrorResponse を得た)、完了できなかった(要求が失敗した、5xx、形式に合わない応答)に
 * 分けて返す。
 */
export class HttpAuthApi implements AuthApi {
  async currentSession(): Promise<SessionCheck> {
    try {
      const response = await fetch(sessionUrl)
      if (response.status !== 200) {
        return { kind: 'failed' }
      }
      const body: unknown = await response.json()
      if (!isRecord(body) || typeof body.authenticated !== 'boolean') {
        return { kind: 'failed' }
      }
      if (!body.authenticated) {
        return { kind: 'signedOut' }
      }
      return isWireSession(body.session) ? { kind: 'signedIn', session: toSessionInfo(body.session) } : { kind: 'failed' }
    } catch {
      return { kind: 'failed' }
    }
  }

  async login(contact: string, secret: string): Promise<LoginResult> {
    const payload: LoginRequest = { contact, secret }
    try {
      const response = await fetch(sessionUrl, { method: 'POST', headers: jsonHeaders, body: JSON.stringify(payload) })
      if (response.status === 200) {
        const body: unknown = await response.json()
        return isWireSession(body) ? { kind: 'completed', session: toSessionInfo(body) } : { kind: 'failed' }
      }
      const rejection = await toRejection(response)
      return rejection ? { kind: 'rejected', ...rejection } : { kind: 'failed' }
    } catch {
      return { kind: 'failed' }
    }
  }

  logout(sessionId: string): Promise<CommandResult> {
    return command(sessionUrl, { method: 'DELETE', headers: { [sessionIdHeader]: sessionId } })
  }

  requestSignup(contact: string): Promise<CommandResult> {
    const payload: SignupRequest = { contact }
    return command('/api/signups', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(payload) })
  }

  /** 確認のための資格は、要求の本文で送る。パスとクエリには、載せない(RL-33)。 */
  completeSignup(request: CompleteSignupRequest): Promise<CommandResult> {
    const payload: WireCompleteSignupRequest = {
      signupCredential: request.credential,
      handle: request.handle,
      displayName: request.displayName,
      secret: request.secret,
    }
    return command('/api/users', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(payload) })
  }
}

/** 本文のない完了(204)を返す操作を行う。 */
async function command(url: string, init: RequestInit): Promise<CommandResult> {
  try {
    const response = await fetch(url, init)
    if (response.status === 204) {
      return { kind: 'completed' }
    }
    const rejection = await toRejection(response)
    return rejection ? { kind: 'rejected', ...rejection } : { kind: 'failed' }
  } catch {
    return { kind: 'failed' }
  }
}
