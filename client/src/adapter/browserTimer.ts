import type { Timer } from '../application/port/timer'

export class BrowserTimer implements Timer {
  start(ms: number, onElapsed: () => void): () => void {
    const id = setTimeout(onElapsed, ms)
    return () => {
      clearTimeout(id)
    }
  }
}
