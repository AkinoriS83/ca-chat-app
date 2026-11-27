// クライアントの開発用のサーバー(Vite)の記録から、/api/ で始まるパスを伏せる。
//
// Vite は、サーバーへの中継に失敗したとき、要求のパスを記録する。パスは、識別名を含む
// (/api/users/<識別名>、/api/direct-conversations/<識別名>/messages)。仕様「記録」は、識別名を、
// 記録しないと定めている(RL-33)。記録の文言の形には、依らない。Vite の版を上げても、働き続けるように
// するためである。
//
// vite.config.ts だけが使う。開発の道具の設定であり、src を import しない。
import type { Logger } from 'vite'

/** 伏せた後に、パスの代わりに残す文字列。 */
export const REDACTED_API_PATH = '/api/…'

// /api/ から、パスに続かない文字(空白、制御文字、" ` < > [ ] { })の手前までを、1つのパスとして扱う。
// クエリ(? より後)と、# より後も、含める。
//
// ' ( ) では、打ち切らない。ブラウザは、この3つを、パスの中で、そのまま送る。打ち切ると、
// 入力の後ろが、記録に残る。代わりに、パスの直後の ' ( ) は、パスとともに伏せることになる。
// 制御文字で打ち切るのは、パスの直後に、色を戻すための並び(ESC で始まる)が続くためである。
const apiPath = /\/api\/[^\s\p{Cc}"`<>[\]{}]*/gu

/** 文言の中の、/api/ で始まるパス(クエリを含む)を、伏せる。 */
export function redactApiPaths(message: string): string {
  return message.replace(apiPath, REDACTED_API_PATH)
}

/** 記録の出力を包み、文言と、添えられたエラーの文言から、/api/ で始まるパスを伏せる。 */
export function redactingLogger(logger: Logger): Logger {
  return {
    // hasWarned は、元の出力先が、書き換える。写しを持たずに、元の値を返す。
    get hasWarned() {
      return logger.hasWarned
    },
    set hasWarned(value: boolean) {
      logger.hasWarned = value
    },
    info: (message, options) => logger.info(redactApiPaths(message), options),
    warn: (message, options) => logger.warn(redactApiPaths(message), options),
    warnOnce: (message, options) => logger.warnOnce(redactApiPaths(message), options),
    error: (message, options) => {
      const error = options?.error
      if (error) {
        error.message = redactApiPaths(error.message)
        if (error.stack) {
          error.stack = redactApiPaths(error.stack)
        }
      }
      logger.error(redactApiPaths(message), options)
    },
    hasErrorLogged: (error) => logger.hasErrorLogged(error),
    clearScreen: (type) => logger.clearScreen(type),
  }
}
