import type { InputField, RejectionCode, UserProfile } from '../model'

/** ログイン状態。sessionId は、ログイン状態の識別子である(ADR-014)。資格は、含まない。 */
export type SessionInfo = {
  sessionId: string
  user: UserProfile
}

/** 「現在のログイン状態を確かめる」の結果。 */
export type SessionCheck =
  | { kind: 'signedIn'; session: SessionInfo }
  | { kind: 'signedOut' }
  /** 確かめられなかった(要求が失敗した、サーバーが完了できなかった、応答が形式に合わない)。 */
  | { kind: 'failed' }

/** サーバーが拒否した。fields は、正しくない項目のすべて(code が invalid_fields の場合だけ)。 */
export type Rejected = { kind: 'rejected'; code: RejectionCode; fields: InputField[] }

/** 完了できなかった(要求が失敗した、5xx、応答が形式に合わない)。 */
export type Failed = { kind: 'failed' }

export type LoginResult = { kind: 'completed'; session: SessionInfo } | Rejected | Failed

/** 本文のない完了を返す操作(登録の申し込み、登録の完了、ログアウト)の結果。 */
export type CommandResult = { kind: 'completed' } | Rejected | Failed

export type CompleteSignupRequest = {
  /** 案内に含まれる、確認のための資格。 */
  credential: string
  handle: string
  displayName: string
  secret: string
}

/**
 * 登録、ログイン、ログイン状態の操作。
 * ログイン状態を示す資格は、ブラウザが、Cookie として保ち、要求に付ける。このポートは、扱わない(ADR-014)。
 */
export interface AuthApi {
  currentSession(): Promise<SessionCheck>
  login(contact: string, secret: string): Promise<LoginResult>
  logout(sessionId: string): Promise<CommandResult>
  requestSignup(contact: string): Promise<CommandResult>
  completeSignup(request: CompleteSignupRequest): Promise<CommandResult>
}
