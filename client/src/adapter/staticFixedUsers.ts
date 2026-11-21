import type { FixedUser } from '../application/model'
import type { FixedUserDirectory } from '../application/port/fixedUserDirectory'

// 仮実装(WS-P1、WS-P3): 利用者は、あらかじめ定めた3人に限る。相手は、この一覧から選ぶ。
// サーバーの固定ユーザー(server/internal/adapter/fixeduser)と、同じ識別名と表示名を持つ。
// フェーズ1で、登録と、識別名による相手の指定に置き換える。
const fixedUsers: readonly FixedUser[] = [
  { handle: 'alice', displayName: 'Alice' },
  { handle: 'bob', displayName: 'Bob' },
  { handle: 'carol', displayName: 'Carol' },
]

export class StaticFixedUsers implements FixedUserDirectory {
  list(): FixedUser[] {
    return fixedUsers.map((user) => ({ ...user }))
  }
}
