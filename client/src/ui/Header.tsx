import type { FixedUser } from '../application/model'

type Props = {
  user: FixedUser
  onLeave: () => void
}

/**
 * 選んだ利用者の表示名と、利用者の選択に戻る操作。
 *
 * 仮実装(WS-P2): 仮のログイン。ログアウトの代わりに、利用者を選び直す。
 */
export function Header({ user, onLeave }: Props) {
  return (
    <header className="app-header">
      <p>
        <span className="current-user" data-testid="current-user">
          {user.displayName}
        </span>
        {' として利用中'}
      </p>
      <button type="button" onClick={onLeave}>
        利用者を選び直す
      </button>
    </header>
  )
}
