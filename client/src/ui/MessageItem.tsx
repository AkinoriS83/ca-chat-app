import type { Message, Outgoing } from '../application/model'

const dateTimeFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'medium' })

/** 送信の日時を、端末の地域の時刻で表す。 */
function formatSentAt(sentAt: string): string {
  const date = new Date(sentAt)
  return Number.isNaN(date.getTime()) ? sentAt : dateTimeFormat.format(date)
}

type MessageItemProps = {
  message: Message
  /** 自分が送ったメッセージであるか。 */
  own: boolean
}

/**
 * 完了したメッセージ。送信者の表示名、送信の日時、本文を示す。
 *
 * 本文は、React の文字列として描画する。HTML として解釈させない。リンクへの変換など、
 * 本文の解釈は、しない(WS-27)。改行とタブは、CSS(white-space: pre-wrap)で、そのまま示す(WS-11)。
 */
export function MessageItem({ message, own }: MessageItemProps) {
  return (
    <li className={own ? 'message message-own' : 'message'} data-testid="message">
      <p className="message-meta">
        <span className="message-sender">{message.sender.displayName}</span>
        <time dateTime={message.sentAt}>{formatSentAt(message.sentAt)}</time>
      </p>
      <p className="message-body">{message.body}</p>
    </li>
  )
}

type OutgoingItemProps = {
  outgoing: Outgoing
  onResend: (sendId: string) => void
}

/**
 * 送信中のメッセージと、送信できなかったメッセージ。
 * 完了したメッセージと、見分けられるように示す(WS-17)。
 */
export function OutgoingItem({ outgoing, onResend }: OutgoingItemProps) {
  const failed = outgoing.status === 'failed'
  return (
    <li
      className={failed ? 'message message-own message-failed' : 'message message-own message-sending'}
      data-testid="outgoing"
    >
      <p className="message-body">{outgoing.body}</p>
      {failed ? (
        <p className="message-status" role="alert">
          <span>送信できませんでした</span>
          <button type="button" onClick={() => onResend(outgoing.sendId)}>
            送り直す
          </button>
        </p>
      ) : (
        <p className="message-status">送信中</p>
      )}
    </li>
  )
}
