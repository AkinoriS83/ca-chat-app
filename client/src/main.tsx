// 組み立てと、起動(ADR-005 の main)。
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { BrowserTimer } from './adapter/browserTimer'
import { CryptoSendIdSource } from './adapter/cryptoSendIdSource'
import { HttpChatApi } from './adapter/httpChatApi'
import { StaticFixedUsers } from './adapter/staticFixedUsers'
import { WebSocketRealtime } from './adapter/webSocketRealtime'
import { ChatApp } from './application/chatApp'
import { App } from './ui/App'
import './ui/style.css'

const app = new ChatApp({
  api: new HttpChatApi(),
  realtime: new WebSocketRealtime(),
  timer: new BrowserTimer(),
  sendIds: new CryptoSendIdSource(),
  users: new StaticFixedUsers(), // 仮実装(WS-P1、WS-P3)
})

const root = document.getElementById('root')
if (!root) {
  throw new Error('root の要素が、ありません')
}
createRoot(root).render(
  <StrictMode>
    <App app={app} />
  </StrictMode>,
)
