import { useState } from 'react'

import type { FormError, InputField } from '../application/model'
import { fieldLabel, fieldRule, signupErrorText } from './rejectionText'

type Props = {
  pending: boolean
  error: FormError | null
  onComplete: (handle: string, displayName: string, secret: string) => void
  onShowSignupRequest: () => void
  onShowLogin: () => void
}

/**
 * 登録の完了。案内から開いた訪問者が、識別名、表示名、秘密を入力する(UC-01 基本フロー 4〜6)。
 *
 * 入力欄の内容は、この部品が持つ。拒否の後も、内容を保つ(UC-01 E1)。送信の前に、入力を検証しない。
 * 拒否では、正しくない項目を、すべて示す。
 */
export function SignupCompleteForm({ pending, error, onComplete, onShowSignupRequest, onShowLogin }: Props) {
  const [handle, setHandle] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [secret, setSecret] = useState('')
  const invalidFields = error?.fields ?? []
  const isInvalid = (field: InputField): true | undefined => (invalidFields.includes(field) ? true : undefined)

  return (
    <main className="auth">
      <h1>登録</h1>
      <p>識別名、表示名、秘密を決めて、登録を完了してください。</p>
      <form
        className="auth-form"
        // ブラウザによる入力の検証を、行わない。検証と拒否は、サーバーが行う。
        noValidate
        onSubmit={(event) => {
          event.preventDefault()
          onComplete(handle, displayName, secret)
        }}
      >
        <label>
          <span>{fieldLabel('handle')}</span>
          <input
            type="text"
            name="handle"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            aria-invalid={isInvalid('handle')}
            aria-describedby="signup-handle-rule"
            value={handle}
            onChange={(event) => setHandle(event.target.value)}
          />
        </label>
        <p className="auth-rule" id="signup-handle-rule">
          {fieldRule('handle')}
        </p>
        <label>
          <span>{fieldLabel('displayName')}</span>
          <input
            type="text"
            name="displayName"
            autoComplete="nickname"
            aria-invalid={isInvalid('displayName')}
            aria-describedby="signup-display-name-rule"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
        </label>
        <p className="auth-rule" id="signup-display-name-rule">
          {fieldRule('displayName')}
        </p>
        <label>
          <span>{fieldLabel('secret')}</span>
          <input
            type="password"
            name="secret"
            autoComplete="new-password"
            aria-invalid={isInvalid('secret')}
            aria-describedby="signup-secret-rule"
            value={secret}
            onChange={(event) => setSecret(event.target.value)}
          />
        </label>
        <p className="auth-rule" id="signup-secret-rule">
          {fieldRule('secret')}
        </p>
        {error && (
          <div className="auth-error" role="alert">
            <p>{signupErrorText(error)}</p>
            {invalidFields.length > 0 && (
              <ul aria-label="正しくない項目">
                {invalidFields.map((field) => (
                  <li key={field}>{fieldLabel(field)}</li>
                ))}
              </ul>
            )}
          </div>
        )}
        <button type="submit" disabled={pending || handle === '' || displayName === '' || secret === ''}>
          登録する
        </button>
      </form>
      <p className="auth-switch">
        {/* 結果を待つ間は、画面を切り替えない(C-19)。 */}
        <button type="button" className="link-button" disabled={pending} onClick={onShowSignupRequest}>
          登録を申し込み直す
        </button>
        <button type="button" className="link-button" disabled={pending} onClick={onShowLogin}>
          ログインの画面へ
        </button>
      </p>
    </main>
  )
}
