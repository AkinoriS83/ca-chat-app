import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// クライアントの開発用のサーバーの、待ち受け先と中継先。E2Eテストは、環境変数で変える。
const clientPort = Number(process.env.CLIENT_PORT ?? '5173')
const apiTarget = process.env.API_TARGET ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    // 端末自身からの接続だけを受け付ける(ADR-007)。ブラウザは、http://localhost:<ポート> にアクセスする。
    host: '127.0.0.1',
    port: clientPort,
    strictPort: true,
    // HTTP の操作と、WebSocket の接続を、サーバーへ中継する。ブラウザから見たオリジンは、1つになる。
    proxy: {
      '/api': { target: apiTarget, ws: true },
    },
  },
  test: {
    // application のテストは、ブラウザのAPIのない環境で実行する。
    // adapter と ui のテストは、ファイルの先頭の注釈で、jsdom を指定する。
    environment: 'node',
    include: ['src/**/*.test.{ts,tsx}'],
    restoreMocks: true,
    unstubGlobals: true,
  },
})
