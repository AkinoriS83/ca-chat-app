/** 送信の識別子を作る(ADR-009)。 */
export interface SendIdSource {
  next(): string
}
