import type { KeyboardEvent } from 'react'

import type { RejectionCode } from '../application/model'
import { rejectionText } from './rejectionText'

type Props = {
  draft: string
  rejection: RejectionCode | null
  onChange: (text: string) => void
  onSend: () => void
}

/**
 * 入力欄。Enter で送信し、Shift+Enter で改行する。
 *
 * 送信の前に、本文を検証しない。検証は、サーバーが行い、拒否の理由を、入力欄の近くに示す(WS-40)。
 * 送信ボタンは、入力欄が空の文字列のときだけ、無効にする。
 */
export function Composer({ draft, rejection, onChange, onSend }: Props) {
  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
    if (event.key !== 'Enter' || event.shiftKey) {
      return
    }
    // かな漢字変換の確定のための Enter では、送信しない。
    if (event.nativeEvent.isComposing || event.keyCode === 229) {
      return
    }
    event.preventDefault()
    onSend()
  }

  return (
    <form
      className="composer"
      onSubmit={(event) => {
        event.preventDefault()
        onSend()
      }}
    >
      {rejection && (
        <p className="composer-rejection" role="alert" id="composer-rejection">
          {rejectionText(rejection)}
        </p>
      )}
      <div className="composer-row">
        <textarea
          aria-label="メッセージ"
          aria-describedby={rejection ? 'composer-rejection' : undefined}
          aria-invalid={rejection ? true : undefined}
          rows={3}
          value={draft}
          placeholder="メッセージを入力(Enter で送信、Shift+Enter で改行)"
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={handleKeyDown}
        />
        <button type="submit" disabled={draft === ''}>
          送信
        </button>
      </div>
    </form>
  )
}
