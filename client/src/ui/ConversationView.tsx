import { useEffect, useRef } from 'react'

import type { Conversation, FixedUser } from '../application/model'
import { MessageItem, OutgoingItem } from './MessageItem'

type Props = {
  self: FixedUser
  conversation: Conversation
  onResend: (sendId: string) => void
  /** 履歴を、取得し直す。 */
  onRetryHistory: () => void
}

/**
 * 会話。メッセージを、連番の順に示す。その後ろに、送信中と、送信できなかったメッセージを示す。
 */
export function ConversationView({ self, conversation, onResend, onRetryHistory }: Props) {
  const { peer, history, messages, outgoing } = conversation
  const listRef = useRef<HTMLDivElement>(null)

  // 新しいメッセージが加わったとき、会話の表示を、末尾まで送る。
  const itemCount = messages.length + outgoing.length
  useEffect(() => {
    const list = listRef.current
    if (list) {
      list.scrollTop = list.scrollHeight
    }
  }, [itemCount])

  const empty = itemCount === 0

  return (
    <section className="conversation" aria-label={`${peer.displayName} との会話`}>
      <div className="conversation-scroll" ref={listRef}>
        {history === 'loading' && <p className="conversation-note">履歴を取得しています…</p>}
        {history === 'failed' && (
          <p className="conversation-note conversation-error" role="alert">
            <span>履歴を取得できませんでした。</span>
            <button type="button" onClick={onRetryHistory}>
              取得し直す
            </button>
          </p>
        )}
        {history === 'loaded' && empty && <p className="conversation-note">メッセージはありません</p>}
        {!empty && (
          <ol className="message-list" aria-label="メッセージ">
            {messages.map((message) => (
              <MessageItem key={message.seq} message={message} own={message.sender.handle === self.handle} />
            ))}
            {outgoing.map((item) => (
              <OutgoingItem key={item.sendId} outgoing={item} onResend={onResend} />
            ))}
          </ol>
        )}
      </div>
    </section>
  )
}
