import type { FixedUser } from '../model'

/** 固定ユーザーの一覧。仮実装(WS-P1、WS-P3): 利用者と相手を、この一覧から選ぶ。 */
export interface FixedUserDirectory {
  list(): FixedUser[]
}
