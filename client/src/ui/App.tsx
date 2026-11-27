import { useEffect, useSyncExternalStore } from 'react'

import type { ChatApp } from '../application/chatApp'
import { Composer } from './Composer'
import { ConversationView } from './ConversationView'
import { Header } from './Header'
import { LoginForm } from './LoginForm'
import { PeerFinder } from './PeerFinder'
import { SignupCompleteForm } from './SignupCompleteForm'
import { SignupRequestForm } from './SignupRequestForm'

type Props = {
  app: ChatApp
}

/**
 * 画面。application の状態を表示し、利用者の操作を application に伝える。
 * 1つの画面を、ログイン状態によって切り替える(ログイン、登録の申し込み、登録の完了、チャット)。
 */
export function App({ app }: Props) {
  const state = useSyncExternalStore(app.subscribe, app.getState)
  const { session, peerSearch, conversation } = state

  // 開いた直後に、案内の資格を読み取るか、ログイン状態を確かめる(C-13)。
  useEffect(() => {
    app.start()
  }, [app])

  switch (session.kind) {
    case 'checking':
      return (
        <main className="auth">
          <p role="status">ログイン状態を確かめています…</p>
        </main>
      )
    case 'unknown':
      // 確かめられなかった。ログインしていないとは、示さない(RL-23)。
      return (
        <main className="auth">
          <p className="auth-error" role="alert">
            ログイン状態を確かめられませんでした。
          </p>
          <button type="button" onClick={app.start}>
            確かめ直す
          </button>
        </main>
      )
    case 'signedOut':
      switch (session.screen) {
        case 'login':
          return (
            <LoginForm
              pending={session.pending}
              error={session.error}
              notice={session.notice}
              onLogin={app.login}
              onShowSignupRequest={app.showSignupRequest}
            />
          )
        case 'signupRequest':
          return (
            <SignupRequestForm
              pending={session.pending}
              error={session.error}
              notice={session.notice}
              onRequest={app.requestSignup}
              onShowLogin={app.showLogin}
            />
          )
        case 'signupComplete':
          return (
            <SignupCompleteForm
              pending={session.pending}
              error={session.error}
              onComplete={app.completeSignup}
              onShowSignupRequest={app.showSignupRequest}
              onShowLogin={app.showLogin}
            />
          )
      }
  }

  return (
    <div className="app">
      <Header user={session.user} logout={session.logout} onLogout={app.logout} />
      <PeerFinder
        search={peerSearch}
        opened={conversation?.peer.handle ?? null}
        onFind={app.findPeer}
        onOpen={app.openFoundPeer}
      />
      {conversation ? (
        <main className="chat">
          <ConversationView
            self={session.user}
            conversation={conversation}
            onResend={app.resend}
            onRetryHistory={app.retryHistory}
          />
          <Composer
            draft={conversation.draft}
            rejection={conversation.rejection}
            onChange={app.setDraft}
            onSend={app.send}
          />
        </main>
      ) : (
        <main className="chat chat-empty">
          <p className="conversation-note">相手の識別名を入力して、探してください。</p>
        </main>
      )}
    </div>
  )
}
