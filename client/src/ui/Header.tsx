import type { UserProfile } from '../application/model'

type Props = {
  user: UserProfile
  logout: 'idle' | 'pending' | 'failed'
  onLogout: () => void
}

/**
 * ログイン中の利用者の表示名と識別名、ログアウトの操作(UC-03)。
 * ログアウトを完了できなかった場合は、できなかったことを示す。ログアウトしたものとしては、示さない(RL-22)。
 */
export function Header({ user, logout, onLogout }: Props) {
  return (
    <header className="app-header">
      <p>
        <span className="current-user" data-testid="current-user">
          {user.displayName}
        </span>
        <span className="user-handle" data-testid="current-handle">
          {`@${user.handle}`}
        </span>
        {' としてログイン中'}
      </p>
      <div className="header-actions">
        {logout === 'failed' && (
          <p className="header-error" role="alert">
            ログアウトできませんでした。ログインしたままです。もう一度、試してください。
          </p>
        )}
        <button type="button" onClick={onLogout} disabled={logout === 'pending'}>
          ログアウト
        </button>
      </div>
    </header>
  )
}
