import { useState } from 'react'

import type { PeerSearch } from '../application/model'
import { peerSearchErrorText } from './rejectionText'

type Props = {
  search: PeerSearch
  /** 会話を開いている相手の識別名。 */
  opened: string | null
  onFind: (handle: string) => void
  onOpen: () => void
}

/**
 * 相手の特定。相手の識別名を入力して探す(UC-08)。
 *
 * 結果として、相手の識別名(相手が登録した通り)と表示名を示す。表示名は、重複しうるため、
 * 識別名を併せて示す(RL-D6)。一覧と、候補は、示さない(AC-7)。入力は、検証せずに、そのまま渡す。
 */
export function PeerFinder({ search, opened, onFind, onOpen }: Props) {
  const [handle, setHandle] = useState('')
  const found = search.found

  return (
    <nav className="peer-finder" aria-label="相手を探す">
      <form
        className="peer-finder-form"
        onSubmit={(event) => {
          event.preventDefault()
          onFind(handle)
        }}
      >
        <label>
          <span>相手の識別名</span>
          <input
            type="text"
            name="peer"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            value={handle}
            onChange={(event) => setHandle(event.target.value)}
          />
        </label>
        <button type="submit" disabled={search.pending || handle === ''}>
          探す
        </button>
      </form>
      {search.error && (
        <p className="peer-finder-error" role="alert">
          {peerSearchErrorText(search.error)}
        </p>
      )}
      {found && (
        <p className="peer-finder-result" data-testid="found-peer">
          <span className="found-name">{found.displayName}</span>
          <span className="user-handle">{`@${found.handle}`}</span>
          <button type="button" onClick={onOpen} aria-pressed={found.handle === opened}>
            会話を開く
          </button>
        </p>
      )}
    </nav>
  )
}
