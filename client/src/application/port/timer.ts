/** 時間の経過。 */
export interface Timer {
  /**
   * ms ミリ秒が過ぎたときに、onElapsed を呼ぶ。
   * 戻り値は、待つのをやめる関数である。
   */
  start(ms: number, onElapsed: () => void): () => void
}
