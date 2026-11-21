# アーキテクチャ概要

本書は、リポジトリ全体の「現在の姿」(層の構成、ディレクトリ、依存の向き、実行環境の構成)を説明する文書です([開発方針](development-policy.md)「設計フェーズの進め方」)。採用の理由は、ADR([docs/adr/](adr/))が受け持ちます。本書は、実装と一致するように、保ち続けます。

- 最終更新: 2026-11-21(ウォーキングスケルトンの完了)
- 現在の範囲: テキスト1on1チャットを、画面から保管まで薄く貫通させたもの([仕様](specs/walking-skeleton.md))。仮実装の箇所は、[仮実装の箇所の一覧](temporary-implementations.md)にあります。

## 全体

```
ブラウザ ──HTTP(操作)──────┐
(React)  ──WebSocket(配信)─┤
                            ▼
              クライアントの開発用のサーバー(Vite)127.0.0.1:5173
                            │ /api で始まる要求と、WebSocket の接続を中継する
                            ▼
                   サーバー(Go)127.0.0.1:8080
                            │ pgx
                            ▼
              PostgreSQL 18(Docker のコンテナ)127.0.0.1:5432
```

- クライアントからの操作(送信、履歴の取得)は、HTTP の要求と応答で行います。サーバーから端末への配信は、WebSocket で行います(ADR-002)。
- 通信の形式は、`api/openapi.yaml` の1つの文書で定義し、サーバーの型と、クライアントの型を、そこから生成します(ADR-003)。サーバーとクライアントは、ソースコードを共有しません。
- サーバーは、メッセージと会話を、メモリに保持しません。保持する状態は、接続中の端末と、配信の宛先だけです(後述)。

## ディレクトリ

```
api/openapi.yaml                 通信の形式の定義
server/                          Go のモジュール(github.com/AkinoriS83/ca-chat-app/server)
  cmd/server/                    main。設定、組み立て、起動。サーバーの振る舞いのテスト
  internal/domain/               利用者、会話、メッセージと、その規則
  internal/application/port/     ポート(application が定義し、adapter が実装するインターフェイス)
  internal/application/usecase/  ユースケースと、その入力・出力のデータ構造、拒否の種類
  internal/adapter/httpapi/      HTTP の操作を受ける(ユースケースを呼ぶ)
  internal/adapter/wsaccept/     WebSocket の接続の受け付け(ユースケースを呼ぶ)
  internal/adapter/wsdelivery/   配信(ポートを実装する)
  internal/adapter/postgres/     保管先(ポートを実装する)
    queries/                     sqlc に与える SQL
    sqlcgen/                     sqlc が生成したコード
    pgtest/                      保管先を使うテストのための、データベースの用意
  internal/adapter/fixeduser/    固定ユーザー(ポートを実装する。仮実装)
  internal/openapi/              oapi-codegen が生成したコード
  internal/depchecksample/       規則に違反する見本だけを置く(通常のビルドには、含まれない)
  migrations/                    goose のマイグレーション
client/                          npm のパッケージ
  src/application/               クライアントの振る舞いと状態(chatApp.ts、messages.ts、model.ts)
  src/application/port/          ポート
  src/adapter/                   ポートの実装。generated/ に、openapi-typescript が生成した型
  src/ui/                        React の部品と、スタイル
  src/main.tsx                   組み立てと起動
  e2e/                           Playwright のテスト
docs/                            文書
scripts/                         Makefile から呼ぶ手順
compose.yaml  Makefile  .env.example  CLAUDE.md  README.md
```

## サーバーの層

層は、4つです(ADR-005)。依存は、外側から内側へだけ向きます。

| 層 | 置き場所 | 置くもの |
|---|---|---|
| domain | `internal/domain` | `User`、`DirectPair`、`Conversation`、`Message`、`Body`(本文の規則)、`SendID`(送信の識別子の形式) |
| application | `internal/application/usecase`、`internal/application/port` | ユースケース(`MessageSender`、`HistoryReader`、`ConnectionKeeper`)と、ポート |
| adapter | `internal/adapter/` の下の各パッケージ | 外部の形式と、ユースケースの形式の間の変換 |
| main | `cmd/server` | 設定の読み込み、各層の組み立て、サーバーの起動 |

### パッケージと、import してよい先

依存規則と飛ばし禁止は、パッケージごとの「import してよい先」として定めています。設定は、`server/.golangci.deps.yml`(depguard)です。列挙にない import は、違反です。規則は、テストのファイルと、生成したコードにも、同じく適用します。

| パッケージ | import してよい先 |
|---|---|
| `domain` | 入出力を行わない標準ライブラリ(下記)だけ |
| `application/port` | `domain`。入出力を行わない標準ライブラリ |
| `application/usecase` | `application/port`、`domain`。入出力を行わない標準ライブラリ |
| `adapter/httpapi` | `application/usecase`、`openapi`。標準ライブラリ |
| `adapter/wsaccept` | `application/usecase`、`openapi`、coder/websocket。標準ライブラリ |
| `adapter/wsdelivery` | `application/port`、`domain`、`openapi`。標準ライブラリ |
| `adapter/postgres` | `application/port`、`domain`、`adapter/postgres/sqlcgen`、`adapter/postgres/pgtest`、pgx。標準ライブラリ |
| `adapter/postgres/sqlcgen` | pgx。標準ライブラリ |
| `adapter/postgres/pgtest` | pgx。標準ライブラリ |
| `adapter/fixeduser` | `application/port`、`domain`。標準ライブラリ |
| `openapi` | oapi-codegen/runtime。標準ライブラリ |
| `cmd/server` | すべて |
| 上のどれにも当たらない、`internal` の下のパッケージ | なし(すべての import が、違反になる。パッケージを加えるときに、規則を加える) |

- `domain` と `application` が import してよい標準ライブラリは、入出力を行わないものに限り、設定に列挙しています(`cmp`、`context`、`errors`、`maps`、`math`、`reflect`、`slices`、`sort`、`strconv`、`strings`、`sync`、`testing`、`time`、`unicode`、`unicode/utf16`、`unicode/utf8`)。`net/http`、`database/sql`、`os`、`log/slog`、`fmt` などは、import できません。ログは、adapter と main が出力します。
- adapter のパッケージ同士は、import しません。1つの adapter のパッケージは、ユースケースを呼ぶことと、ポートを実装することの、一方だけを行います。
- この帰結として、ユースケースを呼ぶ adapter(`httpapi`、`wsaccept`)は、ポート、domain、他の adapter に、到達できません。「ハンドラーが、ユースケースを経由せずに、保管先に到達する」経路は、import の違反として検出されます。

### 型参照の範囲

| 境界 | 越えるデータの型 |
|---|---|
| ユースケースを呼ぶ境界(adapter → usecase) | usecase が定義する、ユースケースごとの入力・出力のデータ構造(`SendMessageInput` など)と、拒否の種類(`Rejection`、`RejectedError`)。domain の型を含めない |
| ポートの境界(usecase → port ← adapter) | domain の型を、そのまま使う |
| システムの外との境界 | 通信の形式(`openapi` の型)と、保管先の表現(`sqlcgen` の型)。adapter の中で詰め替える |

例外は、1つです。接続を受け付けるユースケースの入力(`AcceptConnectionInput`)は、`application/port` が定義する、接続への書き込み口(`ConnectionSink`)を含みます(ADR-005)。

### ポート

`internal/application/port/port.go` に、7つのインターフェイスがあります。

| ポート | 役割 | 実装 |
|---|---|---|
| `UserDirectory` | 利用者を調べる | `adapter/fixeduser`(仮実装 WS-P1) |
| `ConversationReader` | 保管先の読み取り(トランザクションの外) | `adapter/postgres` の `Store` |
| `Transactor` | 一体として行う操作を、1つの関数として受ける(ADR-012) | `adapter/postgres` の `Store` |
| `ConversationWriter` | トランザクションの中でだけ行える操作。`Transactor` が、関数に渡す | `adapter/postgres` |
| `MessageDelivery` | 保管が確定したメッセージを、参加者の端末に届ける | `adapter/wsdelivery` の `Hub` |
| `ConnectionRegistry` | 接続中の端末の登録と解除 | `adapter/wsdelivery` の `Hub` |
| `ConnectionSink` | 接続への書き込み口(送信待ちの列) | `adapter/wsaccept`(import なしに満たす) |

### 3つの経路

**送信**(`POST /api/direct-conversations/{peer}/messages`)

1. `httpapi` が、要求の本文を復号し、ユースケースの入力に詰め替える。本文は、置き換えずに復号し、正しくない並びを、正しくないまま渡す。
2. `usecase.MessageSender` が、利用者の確認、本文の検証(`domain.NewBody`)、送信の識別子の検証、相手の確認を、この順に行う。
3. `Transactor.InTransaction` の中で、会話の成立、追記の直列化(会話の行のロック)、メッセージの追記を、一体として行う。連番と送信の日時は、保管先が付ける(ADR-010)。
4. 追記が、重複した送信(`port.ErrDuplicateSend`)を返した場合、トランザクションを取り消した後に、保管済みのメッセージを取得する。同じ会話のものであれば、配信せずに、完了を示す(ADR-009、ADR-012)。
5. 保管が確定した後に、`MessageDelivery.Deliver` を呼び、完了の応答を返す。

**配信**(`GET /api/realtime?user=<識別名>` の WebSocket)

1. `wsaccept` が、接続を確立する前に、書き込み口(送信待ちの列)を作り、`usecase.ConnectionKeeper.Accept` を呼ぶ。ユースケースが、利用者を確かめた後に、`ConnectionRegistry.Register` を呼ぶ。
2. `wsaccept` が、接続を確立し、受信を開始できたことの知らせ(`ReadyEvent`)を届ける。以後、列から取り出して、書き込む。
3. `wsdelivery` の `Deliver` は、`MessageEvent` を組み立て、参加者の、登録されている書き込み口のすべてに、渡す。端末への書き込みは、待たない。
4. 接続が終わると、`wsaccept` が、`ConnectionKeeper.Close` を呼び、ユースケースが、`ConnectionRegistry.Unregister` を呼ぶ。

**取得**(`GET /api/direct-conversations/{peer}/messages`)

`usecase.HistoryReader` が、利用者と相手を確かめ、会話を探し、メッセージを連番の順に返します。成立していない会話は、空の一覧を返します。会話は、成立させません。

### 状態の置き場所(PERF-3、WS-35)

接続中の端末と、配信の宛先を保持する状態は、`adapter/wsdelivery` の `Hub` だけが持ちます。`application/port` が定義する `MessageDelivery` と `ConnectionRegistry` の背後にあり、内側の層は、この状態を知りません。複数サーバー構成で要る、サーバーの間の中継は、この2つのポートの実装として加えられます(ADR-002)。本スライスは、単一のサーバーで動かし、その仕組みは作っていません。

操作(HTTP)の間に、サーバーが保持する、端末ごとの状態は、ありません。会話の一意性、送信の識別子の一意性、メッセージの順序は、保管先の制約とトランザクションで保ちます(ADR-004)。

### 通信の形式への変換が、2箇所にあること

同じメッセージを、通信の形式(`openapi.Message`)へ変換する箇所が、2つあります。`httpapi` は、ユースケースの出力から変換し、`wsdelivery` は、domain の型から変換します。2つの adapter は、互いに import できないためです(ADR-005 の Consequences)。食い違いは、`cmd/server` のテスト(`TestDelivery_ReachesAllDevicesOfParticipants`)が、完了の応答、履歴、配信の内容を比べて、検出します。

拒否の応答(`{"code": …}`)を書く処理も、同じ理由で、`httpapi` と `wsaccept` の2箇所にあります。

### 保管先

表は、`conversations`(2人の組に、一意性の制約)と、`messages`(主キーは、会話と連番。送信者と送信の識別子の組に、一意性の制約)です(`server/migrations/`)。利用者の表は、ありません(仮実装 WS-P1)。

保管先が返したエラーは、そのまま内側へ返しません。操作の名前と SQLSTATE だけを持つエラー(`postgres.OpError`)に、作り直します。元のエラーの詳細には、行の内容(本文)が含まれることがあるためです(WS-33)。

## クライアントの層

| 層 | 置き場所 | 置くもの |
|---|---|---|
| application | `src/application`、`src/application/port` | 状態と操作(`chatApp.ts`)、履歴と配信の統合(`messages.ts`)、型(`model.ts`)、ポート |
| adapter | `src/adapter` | ポートの実装。通信の形式(生成した型)と、application の型の間の変換 |
| ui | `src/ui` | React の部品。application の状態を表示し、利用者の操作を application に伝える |
| main | `src/main.tsx` | 組み立てと、起動 |

domain の層は、置いていません。業務の規則は、サーバーが持ちます。クライアントは、送信の前に、本文を検証しません。

### 置き場所と、import してよい先

設定は、`client/.dependency-cruiser.cjs`(dependency-cruiser。型だけの import も、対象にする)と、`client/eslint.deps.config.js`(ESLint)です。

| 置き場所 | import してよい先 |
|---|---|
| `src/application/`(`port/` を含む) | application の中だけ。テストのファイルは、Vitest も |
| `src/adapter/` | `src/application/port/` と、`src/application/model.ts`(型だけの import)。adapter の中 |
| `src/ui/` | application(`port/` を除く)。React。ui の中 |
| `src/main.tsx` | すべて(生成した型を除く) |
| 上のどれにも当たらない、`src` の下の置き場所 | なし(違反になる。置き場所を加えるときに、規則を加える) |

- 生成した型(`src/adapter/generated/`)は、adapter だけが import します。
- application は、React と、ブラウザの入出力のAPI(`fetch`、`WebSocket`、`setTimeout`、`crypto`、`window`、`document`、`localStorage` など)を使いません。ESLint の規則で検査します。
- ui と adapter は、互いに import しません。画面の部品が、application を経由せずに通信する経路は、違反として検出されます。
- 実行時の依存物は、react と react-dom だけです。実行時に、外部から、コード、スタイル、フォントを読み込みません(ADR-001)。

### ポート

| ポート | 役割 | 実装 |
|---|---|---|
| `ChatApi` | 履歴の取得、送信 | `httpChatApi.ts`(`fetch`) |
| `RealtimeChannel` | 接続の確立と終了。受信の開始、メッセージ、切断の通知 | `webSocketRealtime.ts`(ブラウザの `WebSocket`) |
| `Timer` | 時間の経過 | `browserTimer.ts` |
| `SendIdSource` | 送信の識別子を作る | `cryptoSendIdSource.ts`(`crypto.randomUUID`) |
| `FixedUserDirectory` | 固定ユーザーの一覧 | `staticFixedUsers.ts`(仮実装 WS-P1、WS-P3) |

### 振る舞い

`ChatApp`(`src/application/chatApp.ts`)が、状態を持ち、変化を通知します。React には、依りません。ui は、`subscribe` と `getState` を、React の `useSyncExternalStore` につなぎます。

振る舞いの規則(C-1〜C-12)は、`chatApp.ts` の先頭に挙げています。要点は、次の通りです。

- 履歴、完了の応答、配信のどれで届いたメッセージも、連番をキーとして統合し、連番の順に示す(ADR-010)。
- 会話を開くときは、WebSocket の接続が、受信を開始できたことの知らせを受けた後に、履歴を取得する。履歴の取得中に届いた配信は、捨てずに、統合する。
- 送信中のメッセージと、送信できなかったメッセージを、送信の識別子をキーとして保持する。完了の応答と、配信のうち、先に届いた方で、完了したメッセージに置き換える(ADR-011)。
- 拒否された本文は、入力欄に戻す。10秒以内に応答を得られない送信は、送信できなかったものとして示し、同じ送信の識別子で、送り直せる。
- 利用者を選び直すと、それまでの接続を閉じ、会話の状態を捨てる。それより前の結果は、後から届いても、捨てる(ADR-013)。

## 実行環境の構成(ローカルの開発環境)

ウォーキングスケルトンは、ローカルの開発環境でだけ動かします(仕様 WS-D6、ADR-007)。仮のログインが、なりすましを防がないためです。

| プロセス | 待ち受け先 | 動かし方 |
|---|---|---|
| クライアントの開発用のサーバー(Vite) | 127.0.0.1:5173 | `make dev-client`。ブラウザは、`http://localhost:5173` にアクセスする |
| サーバー(Go) | 127.0.0.1:8080 | `make dev-server`。待ち受け先は、ループバックのアドレスに限る(それ以外では、起動に失敗する) |
| PostgreSQL 18(コンテナ) | 127.0.0.1:5432 | `make db-up`(Docker Compose)。データは、名前を付けたボリュームに置く |
| E2Eテスト用の、サーバーと、クライアントの開発用のサーバー | 127.0.0.1:18080、127.0.0.1:15173 | `make e2e` の中で、Playwright が起動する |

- ブラウザから見たオリジンは、1つ(`http://localhost:5173`)です。サーバーは、Origin ヘッダーがあり、このオリジンと一致しない要求を、拒否します。CORS の許可は、返しません。
- 秘密(保管先への接続情報)は、リポジトリが追跡しない `.env` で与えます。与えられていない場合、保管先の定義とサーバーは、起動に失敗します。
- データベースは、3種類を使い分けます。開発用(`.env` の `PGDATABASE`)、サーバーのテスト用(雛形の `chat_test_template` と、テストごとの複製)、E2E用(`chat_e2e`)です。
- 開発者の端末は、Windows の上の WSL2 です(ネットワークは、nat)。Docker は、Windows の側の Docker Desktop を、WSL との連携によって使います。保管先のポートは、WSL2 の中と、Windows の側の、127.0.0.1 にだけ現れます。Windows の側のブラウザは、`http://localhost:5173` で、クライアントの開発用のサーバーに届きます。

AWS上の検証環境は、まだありません。設置環境の構成(OPS-1)と、通信経路の暗号化(SEC-2)は、フェーズ1で定めます。

## 検査とテスト

| 手順 | 確かめること |
|---|---|
| `make check-deps` | 依存規則と飛ばし禁止(depguard、dependency-cruiser、ESLint)。違反を検出したとき、出力を、[検査の記録](dependency-check-log.md)に追記する |
| `make check-deps-selftest` | 規則に違反する見本(サーバーは、ビルドタグ `depcheck_sample` を付けたファイル。クライアントは、各層の `__depcheck__/`)が、すべて、違反として検出されること |
| `make generate-check` | 生成したコードが、定義と一致すること |
| `make lint` | golangci-lint、ESLint、TypeScript の型の検査。依存規則は、含めない |
| `make test-server` | domain、usecase、adapter のテストと、サーバーの振る舞いのテスト(`cmd/server`。実際の PostgreSQL を使う) |
| `make test-client` | application のテスト(ブラウザのAPIのない環境)と、adapter、ui のテスト(jsdom) |
| `make e2e` | サーバーとクライアントをつないだ動作(Playwright。Chromium) |
| `make secrets-scan` | リポジトリの履歴に、秘密が含まれないこと(gitleaks) |

テストの置き場所は、層の規則から決まっています。`httpapi` と `wsaccept` は、ユースケースの具体的な型を受け取り、`application/port` を import できないため、パッケージの中のテストでは、ユースケースを組み立てません。ユースケースを通る振る舞いは、すべての層を import できる `cmd/server` のテストで確かめます。
