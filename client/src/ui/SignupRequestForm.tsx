import { useState } from 'react'

import type { FormError, SignedOutNotice } from '../application/model'
import { fieldLabel, fieldRule, noticeText, signupErrorText } from './rejectionText'

type Props = {
  pending: boolean
  error: FormError | null
  notice: SignedOutNotice | null
  onRequest: (contact: string) => void
  onShowLogin: () => void
}

/**
 * 登録の申し込み。連絡先を入力する(UC-01 基本フロー 1〜3)。
 * 完了で、案内を送ったことを示す。連絡先が登録済みかどうかは、示さない(RL-03)。
 */
export function SignupRequestForm({ pending, error, notice, onRequest, onShowLogin }: Props) {
  const [contact, setContact] = useState('')
  const invalid = error?.fields.includes('contact') ?? false

  return (
    <main className="auth">
      <h1>登録の申し込み</h1>
      <p>連絡先に、登録を続けるための案内を送ります。</p>
      {notice && (
        <p className="auth-notice" role="status">
          {noticeText(notice)}
        </p>
      )}
      <form
        className="auth-form"
        // ブラウザによる入力の検証を、行わない。検証と拒否は、サーバーが行う。
        noValidate
        onSubmit={(event) => {
          event.preventDefault()
          onRequest(contact)
        }}
      >
        <label>
          <span>{fieldLabel('contact')}</span>
          <input
            type="email"
            name="contact"
            autoComplete="email"
            aria-invalid={invalid ? true : undefined}
            aria-describedby="signup-contact-rule"
            value={contact}
            onChange={(event) => setContact(event.target.value)}
          />
        </label>
        <p className="auth-rule" id="signup-contact-rule">
          {fieldRule('contact')}
        </p>
        {error && (
          <p className="auth-error" role="alert">
            {signupErrorText(error)}
          </p>
        )}
        <button type="submit" disabled={pending || contact === ''}>
          申し込む
        </button>
      </form>
      <p className="auth-switch">
        {/* 結果を待つ間は、画面を切り替えない(C-19)。 */}
        <button type="button" className="link-button" disabled={pending} onClick={onShowLogin}>
          ログインの画面へ
        </button>
      </p>
    </main>
  )
}
