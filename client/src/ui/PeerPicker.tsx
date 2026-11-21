import type { FixedUser } from '../application/model'

type Props = {
  /** 自分以外の固定ユーザー。 */
  peers: FixedUser[]
  /** 選んでいる相手の識別名。 */
  selected: string | null
  onSelect: (handle: string) => void
}

/**
 * 相手の選択。
 *
 * 仮実装(WS-P3): 相手を、識別名の入力ではなく、固定ユーザーの一覧から選ぶ。
 * 全利用者を列挙できる一覧を提供しないこと(AC-7)を、満たさない。
 */
export function PeerPicker({ peers, selected, onSelect }: Props) {
  return (
    <nav className="peer-picker" aria-labelledby="peer-picker-label">
      <p id="peer-picker-label">相手</p>
      <div className="picker-buttons">
        {peers.map((peer) => (
          <button
            key={peer.handle}
            type="button"
            aria-pressed={peer.handle === selected}
            onClick={() => onSelect(peer.handle)}
          >
            {peer.displayName}
          </button>
        ))}
      </div>
    </nav>
  )
}
