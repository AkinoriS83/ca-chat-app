import { defineConfig, devices } from '@playwright/test'

// E2Eテストの設定(ADR-006)。make e2e が、E2E用のデータベース(chat_e2e)を作り直してから、実行する。
//
// Playwright が、E2E用のデータベースにつないだサーバーと、クライアントの開発用のサーバーを起動する。
// 開発用に起動しているサーバーとは、別の待ち受け先を使う。
const serverPort = 18080
const clientPort = 15173
const appOrigin = `http://localhost:${clientPort}`

// 既定では、Playwright が取得した Chromium(make setup)を使う。版は、@playwright/test の版で決まる。
// E2E_CHROMIUM_PATH を与えると、端末に導入済みの Chromium を、代わりに使う。
// Playwright の Chromium が要する OS のライブラリを、端末に導入できない場合のためである。
// この場合、ブラウザの版は、リポジトリでは固定されない。
const chromiumPath = process.env.E2E_CHROMIUM_PATH

export default defineConfig({
  testDir: './e2e',
  // テストは、直列に実行する。固定ユーザーは3人で、会話は3つしかないためである。
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: true,
  reporter: 'list',
  // 正常な場合、テストは数秒で終わる。メモリの少ない端末では、ブラウザの起動が遅くなるため、長めにする。
  timeout: 120_000,
  use: {
    baseURL: appOrigin,
    // トレースは、既定では記録しない。記録は、メモリを使う。調べるときは、--trace on を付ける。
    trace: 'off',
  },
  // ブラウザは、Chromium だけとする。対応するブラウザの範囲は、フェーズ1の仕様で定める。
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        ...(chromiumPath ? { launchOptions: { executablePath: chromiumPath } } : {}),
      },
    },
  ],
  // 起動の確認には、url(待ち受け先への接続)ではなく、プロセスの出力を使う。
  // url を指定すると、Playwright は、起動の前に、待ち受け先が空いていることを、接続して確かめる。
  // WSL2 の mirrored ネットワークでは、待ち受けのない 127.0.0.1 のポートへの接続が、拒否されずに、
  // 時間切れ(約2分)まで待たされることがあった(2026-11-21。その後、開発者の端末は、nat に変えた)。
  // 待ち受け先が使われている場合、サーバーと Vite(strictPort)は、起動に失敗し、テストは始まらない。
  webServer: [
    {
      // ビルドしてから、起動する。終了のときに、サーバーのプロセスが残らないようにするためである。
      command: 'go build -o ../bin/chat-server-e2e ./cmd/server && exec ../bin/chat-server-e2e',
      cwd: '../server',
      env: {
        PGDATABASE: 'chat_e2e',
        LISTEN_ADDR: `127.0.0.1:${serverPort}`,
        APP_ORIGIN: appOrigin,
      },
      // 起動は、サーバーが標準出力に出す、起動の記録で確かめる(下記)。
      wait: { stdout: /"msg":"server started"/ },
      timeout: 180_000,
    },
    {
      command: 'npm run dev',
      env: {
        CLIENT_PORT: String(clientPort),
        API_TARGET: `http://127.0.0.1:${serverPort}`,
      },
      wait: { stdout: /Local:\s+http/ },
      timeout: 60_000,
    },
  ],
})
