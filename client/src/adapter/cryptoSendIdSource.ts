import type { SendIdSource } from '../application/port/sendIdSource'

/**
 * 送信の識別子(UUID)を、ブラウザの標準のAPIで作る(ADR-009)。
 * crypto.randomUUID は、保護された文脈(localhost、暗号化した経路)でだけ使える。
 */
export class CryptoSendIdSource implements SendIdSource {
  next(): string {
    return crypto.randomUUID()
  }
}
