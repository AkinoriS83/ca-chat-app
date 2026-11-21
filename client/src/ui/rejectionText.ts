import type { RejectionCode } from '../application/model'

/** サーバーが送信を拒否した理由を、利用者に示す文言にする(WS-40)。 */
export function rejectionText(code: RejectionCode): string {
  switch (code) {
    case 'invalid_body':
      return '本文が正しくないため、送信できませんでした。本文は、空でないこと、4,000文字以下であること、制御文字を含まないことが必要です。'
    case 'invalid_input':
      return '入力が正しくないため、送信できませんでした。'
    case 'invalid_peer':
      return 'この相手には、送信できません。'
    case 'not_found':
      return '相手が見つからないため、送信できませんでした。'
    case 'unauthenticated':
      return '利用者を確認できないため、送信できませんでした。利用者を選び直してください。'
    case 'other':
      return '送信が拒否されました。'
  }
}
