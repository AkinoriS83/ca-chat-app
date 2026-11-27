import { useState } from 'react'

import type { FormError, SignedOutNotice } from '../application/model'
import { fieldLabel, loginErrorText, noticeText } from './rejectionText'

type Props = {
  pending: boolean
  error: FormError | null
  notice: SignedOutNotice | null
  onLogin: (contact: string, secret: string) => void
  onShowSignupRequest: () => void
}

/**
 * ログイン。連絡先と秘密を入力する(UC-02)。
 *
 * 入力欄の内容は、この部品が持ち、送信のときに、application に渡す。拒否の後も、内容を保つ。
 * 送信の前に、入力を検証しない。失敗の理由は、区別せずに示す(RL-13)。
 */
export function LoginForm({ pending, error, notice, onLogin, onShowSignupRequest }: Props) {
  const [contact, setContact] = useState('')
  const [secret, setSecret] = useState('')

  return (
    <main className="auth">
      <h1>ログイン</h1>
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
          onLogin(contact, secret)
        }}
      >
        <label>
          <span>{fieldLabel('contact')}</span>
          <input
            type="email"
            name="contact"
            autoComplete="username"
            value={contact}
            onChange={(event) => setContact(event.target.value)}
          />
        </label>
        <label>
          <span>{fieldLabel('secret')}</span>
          <input
            type="password"
            name="secret"
            autoComplete="current-password"
            value={secret}
            onChange={(event) => setSecret(event.target.value)}
          />
        </label>
        {error && (
          <p className="auth-error" role="alert">
            {loginErrorText(error)}
          </p>
        )}
        <button type="submit" disabled={pending || contact === '' || secret === ''}>
          ログイン
        </button>
      </form>
      <p className="auth-switch">
        {/* 結果を待つ間は、画面を切り替えない(C-19)。 */}
        <button type="button" className="link-button" disabled={pending} onClick={onShowSignupRequest}>
          登録を申し込む
        </button>
      </p>
    </main>
  )
}
