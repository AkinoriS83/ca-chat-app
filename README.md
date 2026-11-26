# ca-chat-app

Clean Architecture を実践して開発する、チャットアプリの雛形です。背景と方針は、[開発方針](docs/development-policy.md)にあります。

現在は、最初の機能スライスである、ウォーキングスケルトン(テキスト1on1チャットを、画面から保管まで薄く貫通させたもの)を実装しています。利用者とログインは、仮実装です([仮実装の箇所の一覧](docs/temporary-implementations.md))。

> **注意**: 仮のログインは、なりすましを防ぎません。誰でも、任意の固定ユーザーとして振る舞えます。**ローカルの開発環境の外に、置かないでください**([仕様](docs/specs/walking-skeleton.md) WS-D6)。サーバーは、ループバックのアドレス以外では、待ち受けません。

## 文書

| 文書 | 内容 |
|---|---|
| [docs/development-policy.md](docs/development-policy.md) | 開発方針(目的、フェーズ、非機能要件の方針、開発の流れ、文書の役割分担) |
| [docs/hypothesis.md](docs/hypothesis.md) | 仮説(構造を先に採用する理由、適用範囲、観察方法) |
| [docs/project-plan.md](docs/project-plan.md) | 企画書(機能スコープ、非機能要件の目標水準、完了条件、リスク) |
| [docs/use-cases.md](docs/use-cases.md) | ユースケース記述 |
| [docs/specs/walking-skeleton.md](docs/specs/walking-skeleton.md) | ウォーキングスケルトンの仕様と、受け入れ基準 |
| [docs/adr/](docs/adr/) | ADR(設計の判断) |
| [docs/architecture.md](docs/architecture.md) | アーキテクチャ概要(現在の姿。層、ディレクトリ、import してよい先、実行環境) |
| [docs/temporary-implementations.md](docs/temporary-implementations.md) | 仮実装の箇所の一覧 |
| [docs/observations.md](docs/observations.md) | 観察記録 |
| [docs/dependency-check-log.md](docs/dependency-check-log.md) | 依存規則の検査が、違反を検出したときの記録 |
| [api/openapi.yaml](api/openapi.yaml) | 通信の形式の定義 |
| [CLAUDE.md](CLAUDE.md)、[.claude/rules/](.claude/rules/) | AIへの常設の指示(どの作業にも要るもの)と、置き場所ごとの指示(サーバー、クライアント、通信の形式と生成、文書) |

## 開発者の端末に要るもの

| 道具 | 版 | 備考 |
|---|---|---|
| Go | 1.26 系(`server/go.mod` の `toolchain` の行) | |
| Node.js | 24 系(`client/.nvmrc`) | npm を含む |
| Docker | Docker Compose を含む | 保管先(PostgreSQL)を、コンテナで動かす |
| make | | |
| C のコンパイラ(gcc など) | | sqlc のビルドと、`go test -race` に要る |

開発者の端末は、Windows の上の WSL2 です。Docker は、Windows の側の Docker Desktop を、WSL との連携によって使います([ADR-007](docs/adr/ADR-007-local-development-environment.md))。`docker` を使う手順(`make db-up`、`make test-server`、`make e2e` など)の前に、Windows の側で、Docker Desktop を起動しておきます。

WSL2 のネットワークは、nat(既定)で使います。mirrored では、待ち受けのない 127.0.0.1 のポートへの接続が、拒否されずに、時間切れ(約2分)まで待たされることがありました(2026-11-21 に確かめた。原因は、特定していない)。nat では、すぐに拒否されます。Windows の側のブラウザは、どちらでも、`http://localhost:5173` で届きます。

PostgreSQL のクライアント(`psql`)は、端末に要りません。保管先のコンテナの中のものを使います。

## 初回の準備

```sh
# 1. 秘密を与える。.env は、リポジトリが追跡しない。
cp .env.example .env
#    .env を開き、PGPASSWORD を決める(例: openssl rand -hex 16 の出力)。
#    値は、引用符を付けずに書く。空白と # は、使わない。

# 2. 道具と依存物を導入する(Go の道具を bin/ に、npm の依存物を client/node_modules に)。
make setup

# 3. 保管先を起動し、開発用のデータベースに、マイグレーションを適用する。
make db-up
make db-migrate
```

- `.env` の `PGUSER`、`PGPASSWORD`、`PGDATABASE` は、保管先のコンテナを最初に起動したとき(ボリュームが空のとき)にだけ、保管先に設定されます。後から値を変えても、保管先には反映されません。変える場合は、ボリュームを削除して作り直します(`docker compose down -v`。**開発用のデータは、失われます**)。
- `make setup` は、E2Eテスト用のブラウザ(Chromium)も取得します。Chromium が要する OS のライブラリが端末にない場合、E2Eテストは、ブラウザを起動できずに失敗します(`error while loading shared libraries` と出ます)。その場合は、次のどちらかを行います。
  - ライブラリを導入する: `cd client && sudo npx playwright install-deps chromium`。Playwright が取得した Chromium を使うため、ブラウザの版は、`@playwright/test` の版で決まります。
  - 端末に導入済みの Chromium を使う: `.env` に、`E2E_CHROMIUM_PATH=/snap/bin/chromium` のように、実行ファイルの場所を書きます(`sudo snap install chromium` で導入した場合)。この場合、ブラウザの版は、リポジトリでは固定されません。

道具と依存物の版は、リポジトリに記録して、固定しています(WS-32)。

| 対象 | 版を記録している場所 |
|---|---|
| Go と、サーバーの依存物 | `server/go.mod`、`server/go.sum` |
| クライアントの依存物と、Node.js の道具 | `client/package.json`(版を、1つに指定する)、`client/package-lock.json` |
| Go の道具(oapi-codegen、sqlc、goose、golangci-lint、gitleaks) | `Makefile` の先頭の変数 |
| PostgreSQL | `compose.yaml` のイメージのタグ |

## 起動

端末を3つ使います。

```sh
make db-up        # 保管先(起動していなければ)
make dev-server   # サーバー(127.0.0.1:8080)
make dev-client   # クライアントの開発用のサーバー(127.0.0.1:5173)
```

ブラウザで、`http://localhost:5173` を開きます。アドレスは、`localhost` を使ってください。端末のアドレス(IPアドレス)を指定すると、ブラウザが、保護された文脈として扱わず、送信の識別子を作れません(ADR-007)。

`make dev-client` は、起動したときに、`Local: http://127.0.0.1:5173/` と表示します。このアドレスでは、開かないでください。サーバーは、Origin ヘッダーが、`APP_ORIGIN`(`http://localhost:5173`)と一致しない要求を拒否します。`http://127.0.0.1:5173` で開くと、画面と履歴は示されますが(ブラウザは、同一オリジンの GET に、Origin を付けない)、送信は「送信が拒否されました。」となり、リアルタイムの配信も届きません(2026-11-21 に確かめた)。

1. 利用者(Alice、Bob、Carol)から、自分として振る舞う1人を選ぶ。
2. 相手を選ぶ。
3. メッセージを入力し、Enter、または「送信」で送る(Shift+Enter で改行)。

別のブラウザ(または、別のプロファイル、別のタブ)で、相手の利用者を選ぶと、やり取りできます。

停止は、次の通りです。`make db-down` は、コンテナを削除しますが、データ(ボリューム)は残します。

```sh
# make dev-server と make dev-client は、Ctrl+C で止める。
make db-down
```

### 設定(環境変数)

`.env` で与えます。Docker Compose と、`scripts/with-env.sh`(Makefile が使う)が読みます。既に環境にある変数は、上書きしません。

| 変数 | 内容 |
|---|---|
| `PGHOST`、`PGPORT`、`PGUSER`、`PGPASSWORD`、`PGDATABASE` | 保管先への接続情報。`PGUSER`、`PGPASSWORD`、`PGDATABASE` がない場合、サーバーは、起動に失敗する |
| `APP_ORIGIN` | アプリのオリジン(`http://localhost:5173`)。サーバーは、Origin ヘッダーが、これと一致しない要求を拒否する。Host ヘッダーが、このオリジンのホスト(`localhost:5173`)でも、ループバックのアドレスの表記(`127.0.0.1:8080` など)でもない要求も、拒否する。ない場合、サーバーは、起動に失敗する |
| `LISTEN_ADDR` | サーバーの待ち受け先。既定は、`127.0.0.1:8080`。ループバックのアドレス(127.0.0.0/8、::1)に限る |
| `CLIENT_PORT`、`API_TARGET` | クライアントの開発用のサーバーの、待ち受けるポート(既定は、5173)と、中継先(既定は、`http://127.0.0.1:8080`)。E2Eテストが使う |
| `E2E_CHROMIUM_PATH` | E2Eテストで使う Chromium の実行ファイルの場所。与えない場合は、Playwright が取得した Chromium を使う(「初回の準備」) |

## コード生成

通信の形式の型と、保管先への問い合わせのコードは、定義から生成し、リポジトリに含めています(ADR-003、ADR-004)。生成したコードは、手で編集しません。

| 定義 | 生成するもの | 道具 |
|---|---|---|
| `api/openapi.yaml` | `server/internal/openapi/` | oapi-codegen |
| `api/openapi.yaml` | `client/src/adapter/generated/openapi.d.ts` | openapi-typescript |
| `server/migrations/`、`server/internal/adapter/postgres/queries/` | `server/internal/adapter/postgres/sqlcgen/` | sqlc |

```sh
make generate         # 定義を変えた後に、生成し直す
make generate-check   # 生成し直した結果が、リポジトリの内容と一致することを確かめる
```

マイグレーションを加えた場合は、`make db-migrate` で、開発用のデータベースに適用します。

## 検査とテスト

```sh
make check   # 下の表の、e2e を除くすべてを、順に実行する
make e2e     # E2Eテスト
```

| 手順 | 内容 | 保管先 |
|---|---|---|
| `make generate-check` | 生成したコードが、定義と一致すること | 使わない |
| `make lint` | golangci-lint、ESLint、TypeScript の型の検査 | 使わない |
| `make check-deps` | 依存規則と飛ばし禁止の検査(下記) | 使わない |
| `make check-deps-selftest` | 規則に違反する見本が、すべて、違反として検出されること | 使わない |
| `make test-server` | サーバーのテスト(`go test -race -count=1 ./...`) | 使う |
| `make test-client` | クライアントのテスト(Vitest) | 使わない |
| `make secrets-scan` | リポジトリの履歴の全体に、秘密が含まれないこと(gitleaks)。コミットした内容が、対象である | 使わない |
| `make e2e` | E2Eテスト(Playwright。Chromium) | 使う |

### 依存規則と飛ばし禁止の検査

層の規則(パッケージごとの、import してよい先)は、[アーキテクチャ概要](docs/architecture.md)にあります。検査は、`make check-deps` で実行します。

- 違反を検出したとき、`make check-deps` は、日時と出力を、[docs/dependency-check-log.md](docs/dependency-check-log.md) に追記して、失敗します。観察記録のためです([ADR-008](docs/adr/ADR-008-observation-records.md))。違反を直した後も、記録は消しません。[観察記録](docs/observations.md)に、都度の記録を追記します。
- 検査の道具を、専用の設定(`server/.golangci.deps.yml`、`client/.dependency-cruiser.cjs`、`client/eslint.deps.config.js`)を指定して、直接には実行しないでください。記録に、残らなくなります。`make lint` は、これらの規則を、わざと含めていません。
- パッケージや置き場所を加えるときは、設定に規則を加え、アーキテクチャ概要を更新します。規則に違反する見本も、併せて加えます。

### サーバーのテストと、保管先

`make test-server` は、雛形のデータベース(`chat_test_template`)を作り直し、マイグレーションを適用してから、テストを実行します。保管先を使うテストは、テストごとに、雛形を複製したデータベースを作り、終わったら削除します。開発用のデータには、影響しません。

`go test` を直接に実行する場合は、雛形が要ります。雛形がない場合、テストは、`make test-server` を使うよう示して、失敗します。

```sh
cd server && ../scripts/with-env.sh go test -race -count=1 ./cmd/server/ -run TestSend
```

### E2Eテスト

`make e2e` は、E2E用のデータベース(`chat_e2e`)を作り直してから、Playwright を実行します。Playwright が、このデータベースにつないだサーバー(127.0.0.1:18080)と、クライアントの開発用のサーバー(127.0.0.1:15173)を起動します。開発用に起動しているサーバーとは、別の待ち受け先を使うため、`make dev-server` を動かしたままで、実行できます。

- テストは、直列に実行します。テストごとに、始める前に、データを空の状態に戻します。
- 失敗を調べるときは、`cd client && ../scripts/with-env.sh npx playwright test --trace on` で、トレースを記録します(既定では、記録しません)。

## 動作検証

[仕様](docs/specs/walking-skeleton.md)「動作検証」の手順1〜10を、`http://localhost:5173` に対して行います。互いに状態を共有しない、3つのブラウザ(別のブラウザ、または、別のプロファイル)を使います。動作検証は、E2Eテストとは別に、開発者がブラウザで行います(ADR-006)。

始める前に、開発用のデータベースを空にしておくと、手順1(メッセージがないこと)を、そのまま確かめられます。

```sh
docker compose exec -T postgres psql -U "$(grep '^PGUSER=' .env | cut -d= -f2)" -d "$(grep '^PGDATABASE=' .env | cut -d= -f2)" \
  -c 'TRUNCATE messages, conversations'
```

仕様の手順のうち、具体化が要るものは、次の通りに行います。

**手順7(サーバーと保管先を再起動する)**

1. `make dev-server` を止め、`make db-down`、`make db-up` の順に実行して、`make dev-server` を起動する。
2. 各ブラウザを再読み込みし、利用者と相手を選び直す。それまでのメッセージが、すべて履歴として示されることを確かめる。

**手順8(サーバーを停止した状態での送信)**

1. `make dev-server` を止める。保管先と、クライアントの開発用のサーバーは、止めない。
2. ブラウザ1から送信する。「送信できませんでした」と、本文が示されることを確かめる(応答を待つ時間は、最大で10秒)。
3. `make dev-server` を起動する。
4. ブラウザ1を開き直さずに、「送り直す」を押す。メッセージが、完了したものとして示されることを確かめる。
5. ブラウザ1を再読み込みし、alice として、bob との会話を開く。履歴に、そのメッセージが1通だけあることを確かめる。

サーバーを停止すると、各ブラウザの WebSocket の接続は切れ、再接続しません(ADR-002)。手順4の送信は完了しますが、他のブラウザは、再読み込みするまで、配信を受け取りません。

サーバーを停止している間、クライアントの開発用のサーバーの中継は、5xx を返します。画面は、これを受けて、「送信できませんでした」を示します。

WSL2 のネットワークが mirrored の場合は、停止したサーバーへの接続が、拒否されずに、時間切れ(約2分)まで待たされることがあります(「開発者の端末に要るもの」)。その場合も、画面は、送信から10秒で「送信できませんでした」を示します。停止している間に会話を開くと、履歴の取得は、中継の応答があるまで「履歴を取得しています…」のままになります。nat では、中継は、すぐに 5xx を返します。

**手順10(サーバーが拒否する本文)**

クライアントは、送信の前に、本文を検証しません。サーバーが拒否する本文を、画面から、そのまま送れます。

1. 4,001文字の本文を、入力欄に貼り付けて、送信する。本文は、ブラウザの開発者向けの機能(コンソール)で、`copy('a'.repeat(4001))` を実行して作る。
2. 拒否されたことと理由が示され、本文が入力欄に残り、会話に示されないことを確かめる。
3. 1文字を削って、送信する。完了することを確かめる。

## ライセンスの確認

依存物のライセンスは、導入の時点(2026-11-21)で確かめました(ADR-001)。版を上げるときは、確かめ直します。

| 対象 | ライセンス |
|---|---|
| サーバーの実行時の依存物 | pgx と、その依存物(MIT)、coder/websocket(ISC)、oapi-codegen/runtime、go-jsonmerge(Apache-2.0、MIT)、google/uuid、golang.org/x/sync、golang.org/x/text(BSD-3-Clause) |
| クライアントの実行時の依存物 | react、react-dom、scheduler(MIT) |
| 開発の時点で実行する道具 | MIT、ISC、BSD、Apache-2.0 のほか、golangci-lint(GPL-3.0)、lightningcss(MPL-2.0。Vite が使う)がある。いずれも、成果物には含まれない |
