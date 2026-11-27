import type { EntryLink } from '../application/port/entryLink'

/**
 * 登録を続けるためのアドレスの、# より後の、資格の前に置く部分。
 * サーバー(smtpmail)が、案内に載せる形と、同じ値を持つ。
 */
const signupFragment = '#signup='

/**
 * 案内の資格を、表示中のページのアドレスから読み取る。
 *
 * 資格は、アドレスの # より後に載っている。# より後は、サーバーへ送られない(RL-33)。
 * 読み取った後、アドレスから除く。ブラウザの履歴と、アドレスの欄に、資格を残さないためである。
 */
export class LocationEntryLink implements EntryLink {
  takeSignupCredential(): string | null {
    const hash = location.hash
    if (!hash.startsWith(signupFragment)) {
      return null
    }
    const credential = hash.slice(signupFragment.length)
    // 履歴の項目を、増やさずに、置き換える。
    history.replaceState(history.state, '', location.pathname + location.search)
    return credential === '' ? null : credential
  }
}
