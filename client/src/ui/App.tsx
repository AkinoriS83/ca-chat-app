import { useSyncExternalStore } from 'react'

import type { ChatApp } from '../application/chatApp'
import { Composer } from './Composer'
import { ConversationView } from './ConversationView'
import { Header } from './Header'
import { PeerPicker } from './PeerPicker'
import { UserPicker } from './UserPicker'

type Props = {
  app: ChatApp
}

/**
 * 画面。application の状態を表示し、利用者の操作を application に伝える。
 * 1つの画面に、利用者の選択、相手の選択、会話、入力欄を置く。
 */
export function App({ app }: Props) {
  const state = useSyncExternalStore(app.subscribe, app.getState)
  const { users, currentUser, conversation } = state

  if (!currentUser) {
    return <UserPicker users={users} onSelect={app.selectUser} />
  }

  return (
    <div className="app">
      <Header user={currentUser} onLeave={app.leaveUser} />
      <PeerPicker
        peers={users.filter((user) => user.handle !== currentUser.handle)}
        selected={conversation?.peer.handle ?? null}
        onSelect={app.openConversation}
      />
      {conversation ? (
        <main className="chat">
          <ConversationView
            self={currentUser}
            conversation={conversation}
            onResend={app.resend}
            onRetryHistory={() => app.openConversation(conversation.peer.handle)}
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
          <p className="conversation-note">相手を選んでください。</p>
        </main>
      )}
    </div>
  )
}
