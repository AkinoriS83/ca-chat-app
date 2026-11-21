import type { FixedUser } from '../application/model'

type Props = {
  users: FixedUser[]
  onSelect: (handle: string) => void
}

/**
 * 利用者の選択。
 *
 * 仮実装(WS-P2): 仮のログイン。固定ユーザーから、自分として振る舞う1人を選ぶだけで、
 * ログインした状態になる。認証に用いる秘密を持たず、なりすましを防がない。
 */
export function UserPicker({ users, onSelect }: Props) {
  return (
    <main className="user-picker">
      <h1>ca-chat-app</h1>
      <p id="user-picker-label">利用者を選んでください。</p>
      <div className="picker-buttons" role="group" aria-labelledby="user-picker-label">
        {users.map((user) => (
          <button key={user.handle} type="button" onClick={() => onSelect(user.handle)}>
            {user.displayName}
          </button>
        ))}
      </div>
    </main>
  )
}
