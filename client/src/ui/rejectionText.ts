import type { FormError, InputField, RejectionCode, SignedOutNotice } from '../application/model'

// サーバーが拒否した理由(code)と、完了できなかったことを、利用者に示す文言にする。
// 拒否の判定は、サーバーが行う。クライアントは、文言にするだけである。

const rateLimited = '回数の上限を超えました。時間をおいて、やり直してください。'
const failed = '完了できませんでした。時間をおいて、やり直してください。'

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
    case 'rate_limited':
      return `新しい相手への操作の、${rateLimited}`
    default:
      return '送信が拒否されました。'
  }
}

/** 相手の特定が、完了しなかった理由(RL-25、RL-26、RL-31)。 */
export function peerSearchErrorText(code: RejectionCode | 'failed'): string {
  switch (code) {
    case 'not_found':
      return '見つかりませんでした。'
    case 'invalid_peer':
      return '自分自身は、相手にできません。'
    case 'invalid_input':
      return '識別名が正しくありません。識別名は、3文字以上、20文字以下の、英字、数字、アンダースコア(_)です。'
    case 'rate_limited':
      return `新しい相手を探す操作の、${rateLimited}`
    case 'failed':
      return `相手を探す操作を、${failed}`
    default:
      return '相手を探す操作が、拒否されました。'
  }
}

/** 履歴の取得が拒否された理由。 */
export function historyRejectionText(code: RejectionCode): string {
  switch (code) {
    case 'rate_limited':
      return `新しい相手との会話を開く操作の、${rateLimited}`
    case 'not_found':
      return '相手が見つからないため、履歴を取得できませんでした。'
    case 'invalid_peer':
      return 'この相手との会話は、開けません。'
    default:
      return '履歴の取得が、拒否されました。'
  }
}

/**
 * ログインが、完了しなかった理由。照合の失敗は、理由(連絡先が登録されていない、秘密が違うなど)を、
 * 区別せずに示す(RL-13)。
 */
export function loginErrorText(error: FormError): string {
  switch (error.code) {
    case 'login_failed':
      return 'ログインできませんでした。連絡先と、秘密を確かめてください。'
    case 'rate_limited':
      return `ログインの試行の、${rateLimited}`
    case 'failed':
      return `ログインを、${failed}`
    default:
      return 'ログインが、拒否されました。'
  }
}

/** 登録の申し込みと、登録の完了が、完了しなかった理由。 */
export function signupErrorText(error: FormError): string {
  switch (error.code) {
    case 'invalid_fields':
      return '入力が正しくありません。次の項目を、直してください。'
    case 'handle_taken':
      return 'この識別名は、使われています。別の識別名を入力してください。'
    case 'invalid_signup_credential':
      return 'この案内では、登録を続けられません(有効期限が切れているか、使用済みです)。登録を、申し込み直してください。'
    case 'rate_limited':
      return rateLimited
    case 'failed':
      return failed
    default:
      return '登録が、拒否されました。'
  }
}

/** 入力の項目の名前。 */
export function fieldLabel(field: InputField): string {
  switch (field) {
    case 'contact':
      return '連絡先(メールアドレス)'
    case 'handle':
      return '識別名'
    case 'displayName':
      return '表示名'
    case 'secret':
      return '秘密(パスワード)'
  }
}

/**
 * 入力の項目の規則(仕様「入力の規則」)。項目ごとに、入力欄の近くに示す。
 * 秘密は、条件の全体を示す。どの条件に当たったかは、区別しない。
 */
export function fieldRule(field: InputField): string {
  switch (field) {
    case 'contact':
      return 'メールアドレスを、半角の英数字と記号で入力してください。'
    case 'handle':
      return '3文字以上、20文字以下。英字、数字、アンダースコア(_)が使えます。他の利用者が、あなたを探すときに使います。'
    case 'displayName':
      return '1文字以上、50文字以下。改行は、使えません。'
    case 'secret':
      return '15文字以上、128文字以下。よく使われるもの、連絡先や識別名と同じものは、使えません。'
  }
}

/** ログインしていないときに示す知らせ。 */
export function noticeText(notice: SignedOutNotice): string {
  switch (notice) {
    case 'sessionEnded':
      return 'ログインし直してください(ログイン状態が、無効になりました)。'
    case 'signupRequested':
      return '案内を送りました。届いた案内に従って、登録を続けてください。'
    case 'signupCompleted':
      return '登録を完了しました。登録した連絡先と、秘密で、ログインしてください。'
  }
}
