# アーキテクチャ概要

本書は、リポジトリ全体の「現在の姿」(層の構成、ディレクトリ、依存の向き、実行環境の構成)を説明する文書です([開発方針](development-policy.md)「設計フェーズの進め方」)。採用の理由は、ADR([docs/adr/](adr/))が受け持ちます。本書は、実装と一致するように、保ち続けます。

- 最終更新: 2026-11-27(「登録とログイン」の完了)
- 現在の範囲: テキスト1on1チャット([ウォーキングスケルトンの仕様](specs/walking-skeleton.md))に、登録、ログインとログアウト、識別名による相手の特定を加えたもの([「登録とログイン」の仕様](specs/registration-and-login.md))。仮実装の箇所(履歴の一括取得)は、[仮実装の箇所の一覧](temporary-implementations.md)にあります。

## 全体

```
ブラウザ ──HTTP(操作)──────┐
(React)  ──WebSocket(配信)─┤
                            ▼
              クライアントの開発用のサーバー(Vite)127.0.0.1:5173
                            │ /api で始まる要求と、WebSocket の接続を中継する
                            ▼
                   サーバー(Go)127.0.0.1:8080
                            │ pgx                         │ SMTP(案内)
                            ▼                             ▼
   PostgreSQL 18(コンテナ)127.0.0.1:5432      Mailpit(コンテナ)127.0.0.1:1025
```

- クライアントからの操作(登録、ログインとログアウト、相手の特定、送信、履歴の取得)は、HTTP の要求と応答で行います。サーバーから端末への配信は、WebSocket で行います(ADR-002)。
- 通信の形式は、`api/openapi.yaml` の1つの文書で定義し、サーバーの型と、クライアントの型を、そこから生成します(ADR-003)。サーバーとクライアントは、ソースコードを共有しません。
- 認証は、Cookie で保つ資格と、端末がメモリに保持する、ログイン状態の識別子で行います(ADR-014)。ログイン状態が有効であることは、操作のたびに、usecase が確かめます(ADR-019)。
- 登録の案内は、SMTP で送ります。ローカルの開発環境では、Mailpit が受け取り、外へは送りません(ADR-018)。
- サーバーは、利用者、ログイン状態、登録の申し込み、メッセージと会話を、メモリに保持しません。メモリに保持する状態は、接続中の端末(配信の宛先と、ログイン状態との対応)と、上限の計数だけです(後述)。

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
  internal/adapter/argon2id/     秘密の計算と照合(ポートを実装する)
  internal/adapter/commonsecrets/ よく使われる秘密の一覧(ポートを実装する)。list.txt、NOTICE.md
  internal/adapter/smtpmail/     案内の送信(ポートを実装する)
  internal/adapter/ratelimit/    上限の計数(ポートを実装する)
  internal/adapter/system/       時刻、資格の生成(ポートを実装する)
  internal/adapter/eventlog/     出来事の記録(ポートを実装する)
  internal/adapter/schedule/     片付けを、一定の間隔で呼ぶ(ユースケースを呼ぶ)
  internal/openapi/              oapi-codegen が生成したコード
  internal/depchecksample/       規則に違反する見本だけを置く(通常のビルドには、含まれない)
  migrations/                    goose のマイグレーション
client/                          npm のパッケージ
  src/application/               クライアントの振る舞いと状態(chatApp.ts、messages.ts、model.ts)。テスト用のポートの実装(testPorts.ts)
  src/application/port/          ポート
  src/adapter/                   ポートの実装。generated/ に、openapi-typescript が生成した型
  src/ui/                        React の部品と、スタイル
  src/main.tsx                   組み立てと起動
  e2e/                           Playwright のテスト
  tools/                         開発用のサーバーの設定(vite.config.ts)が使う関数。src を import しない
docs/                            文書
scripts/                         Makefile から呼ぶ手順。よく使われる秘密の一覧を取り出す手順(extract-common-secrets)
.claude/rules/                   AIへの、置き場所ごとの指示(該当するファイルを読み書きしたときに、読み込まれる)
compose.yaml  Makefile  .env.example  CLAUDE.md  README.md
```

## サーバーの層

層は、4つです(ADR-005)。依存は、外側から内側へだけ向きます。

| 層 | 置き場所 | 置くもの |
|---|---|---|
| domain | `internal/domain` | `User`、`DirectPair`、`Conversation`、`Message`、`Body`(本文の規則)、`SendID`(送信の識別子の形式)。規則を通してしか作れない、`Contact`(連絡先)、`Handle`(識別名)、`DisplayName`(表示名)、`Secret`(秘密。文字列にする操作では、伏せた文字列を返す)。`SecretHash`、`Session`(有効かどうかの判定 `ValidAt`)、`Signup`、有効期限の値、上限(`Limit`、`LimitRules` と既定値)、出来事の記録(`Event`) |
| application | `internal/application/usecase`、`internal/application/port` | ユースケース(下記)と、ポート |
| adapter | `internal/adapter/` の下の各パッケージ | 外部の形式と、ユースケースの形式の間の変換 |
| main | `cmd/server` | 設定の読み込み、各層の組み立て、サーバーの起動 |

ユースケースは、次の通りです。公開するメソッドは、戻るときに、記録の通り道(`attempt`)を通り、拒否でないエラーを、出来事として記録します(ADR-021)。

| 型 | メソッド | 内容 |
|---|---|---|
| `SignupRequester` | `Request` | 登録を申し込む(案内を送る) |
| `SignupCompleter` | `Complete` | 登録を完了する(申し込みを使用済みにし、利用者を作る) |
| `Authenticator` | `Login` | ログインする |
| `SessionKeeper` | `Current`、`Logout` | 現在のログイン状態を確かめる、ログアウトする |
| `PeerFinder` | `Find` | 相手を特定する |
| `MessageSender` | `Send` | メッセージを送る |
| `HistoryReader` | `Read` | 会話の履歴を読む |
| `ConnectionKeeper` | `Accept`、`Close` | リアルタイムの接続を受け付ける、接続の終わりを受ける |
| `Housekeeper` | `Sweep` | 片付け(期限に達した接続を閉じ、期限を過ぎたログイン状態と、申し込みを消す) |
| `SessionGate` | (公開しない) | ログイン状態の確認。認証を要するユースケースが、最初の手順として呼ぶ(ADR-019) |

相手の指定を扱う手順(識別名の規則、自分自身、上限、居るかどうか)は、usecase の中の `peerResolver` にまとめ、相手の特定、履歴、送信の3つが使います。

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
| `adapter/argon2id` | `application/port`、`domain`、golang.org/x/crypto/argon2。標準ライブラリ |
| `adapter/commonsecrets`、`adapter/smtpmail`、`adapter/ratelimit`、`adapter/system`、`adapter/eventlog` | `application/port`、`domain`。標準ライブラリ |
| `adapter/schedule` | `application/usecase`。標準ライブラリ |
| `openapi` | oapi-codegen/runtime。標準ライブラリ |
| `cmd/server` | すべて |
| 上のどれにも当たらない、`internal` の下のパッケージ | なし(すべての import が、違反になる。パッケージを加えるときに、規則を加える) |

- `domain` と `application` が import してよい標準ライブラリは、入出力を行わないものに限り、設定に列挙しています(`cmp`、`context`、`errors`、`maps`、`math`、`reflect`、`slices`、`sort`、`strconv`、`strings`、`sync`、`testing`、`time`、`unicode`、`unicode/utf16`、`unicode/utf8`)。`net/http`、`database/sql`、`os`、`log/slog`、`fmt`、`crypto/rand` などは、import できません。乱数、要約、秘密の計算、時刻、ログは、ポートの背後の adapter が受け持ちます。golang.org/x/crypto を import できるのは、`adapter/argon2id` だけです(ADR-015)。
- adapter のパッケージ同士は、import しません。1つの adapter のパッケージは、ユースケースを呼ぶことと、ポートを実装することの、一方だけを行います。
- この帰結として、ユースケースを呼ぶ adapter(`httpapi`、`wsaccept`、`schedule`)は、ポート、domain、他の adapter に、到達できません。「ハンドラーが、ユースケースを経由せずに、保管先に到達する」経路は、import の違反として検出されます。

### 型参照の範囲

| 境界 | 越えるデータの型 |
|---|---|
| ユースケースを呼ぶ境界(adapter → usecase) | usecase が定義する、ユースケースごとの入力・出力のデータ構造(`SendMessageInput` など)と、拒否の種類(`Rejection`、`RejectedError`。`InvalidFields` の拒否は、正しくない項目 `InputField` の一覧を持つ)。domain の型を含めない。操作を行う利用者(`Actor`)は、要求から取り出した、資格と、ログイン状態の識別子である。資格を含む出力は、`LoginOutput` だけである(ADR-019) |
| ポートの境界(usecase → port ← adapter) | domain の型を、そのまま使う |
| システムの外との境界 | 通信の形式(`openapi` の型)と、保管先の表現(`sqlcgen` の型)。adapter の中で詰め替える |

例外は、1つです。接続を受け付けるユースケースの入力(`AcceptConnectionInput`)は、`application/port` が定義する、接続への書き込み口(`ConnectionSink`)を含みます(ADR-005)。

### ポート

`internal/application/port/port.go` に、18のインターフェイスがあります。

| ポート | 役割 | 実装 |
|---|---|---|
| `UserDirectory` | 登録を完了した利用者を、識別名(大文字と小文字を区別しない)と、連絡先で調べる | `adapter/postgres` の `Store` |
| `ConversationReader` | 保管先の読み取り(トランザクションの外) | `adapter/postgres` の `Store` |
| `Transactor` | 一体として行う操作を、1つの関数として受ける(ADR-012)。送信と、登録の完了の、2つ | `adapter/postgres` の `Store` |
| `ConversationWriter` | 送信のトランザクションの中でだけ行える操作 | `adapter/postgres` |
| `SignupWriter` | 登録の完了のトランザクションの中でだけ行える操作(申し込みを使用済みにする、利用者を作る) | `adapter/postgres` |
| `SessionStore` | ログイン状態を保持する。資格は、要約で保管し、要約で引く | `adapter/postgres` の `SessionStore`(`Store.Sessions()`) |
| `SignupStore` | 登録の申し込みを保持する | `adapter/postgres` の `SignupStore`(`Store.Signups()`) |
| `MessageDelivery` | 保管が確定したメッセージを、参加者の端末に届ける。期限を過ぎたログイン状態による接続には、届けずに、閉じる | `adapter/wsdelivery` の `Hub` |
| `ConnectionRegistry` | 接続中の端末の登録(ログイン状態との対応付け)と解除 | `adapter/wsdelivery` の `Hub` |
| `SessionConnections` | ログイン状態による接続を閉じる(無効にしたもの、期限を過ぎたもの) | `adapter/wsdelivery` の `Hub` |
| `ConnectionSink` | 接続への書き込み口(送信待ちの列)。閉じる理由(ログイン状態が無効になった)を、操作で伝える | `adapter/wsaccept`(import なしに満たす) |
| `CredentialSource` | 資格(ログイン状態を示す資格、確認のための資格)を作る | `adapter/system` |
| `SecretHasher` | 秘密を、保管する形に変え、照合する(ADR-015) | `adapter/argon2id` |
| `CommonSecretList` | 秘密が、よく使われる秘密の一覧にあるかを確かめる(ADR-017) | `adapter/commonsecrets` |
| `NoticeSender` | 案内を送る(ADR-018) | `adapter/smtpmail` |
| `AttemptLimiter` | 上限を数える。確かめることと数えることを、1つの操作で行う(ADR-016) | `adapter/ratelimit` |
| `Clock` | 時刻の源。期限と上限の判定は、ここから得た時刻に揃える | `adapter/system` |
| `EventRecorder` | 出来事を記録する(ADR-021) | `adapter/eventlog` |

時刻を要するポートの操作は、時刻を、引数で受けます。ポートの実装は、自分では、時刻を読みません。テストは、`Clock` を差し替えて、時刻を進めます。

### 経路

操作と、ログイン状態の添え方は、`api/openapi.yaml` にあります。認証を要する操作では、どのユースケースも、最初に、`SessionGate` で、ログイン状態を確かめます。保管先への照会は、操作につき1回です。

**登録**(`POST /api/signups`、`POST /api/users`)

1. 申し込みでは、`usecase.SignupRequester` が、連絡先の検証、上限の確認を行い、連絡先が登録済みでなければ、資格を作って、申し込みを置き、案内を送る。登録済みであれば、資格を含まない案内を送る。応答は、どちらも同じである。
2. 完了では、`usecase.SignupCompleter` が、上限、資格、入力の規則(よく使われる秘密の一覧との照合を含む)を、この順に確かめ、秘密を計算してから、`Transactor.InSignupTransaction` の中で、申し込みを使用済みにし、利用者を作る。識別名と連絡先の一意性は、保管先の制約で保つ。

**ログインとログアウト**(`POST`、`GET`、`DELETE /api/session`)

1. ログインでは、`usecase.Authenticator` が、失敗として数えてから、秘密を照合する。照合する相手がない場合も、同じ重さの計算を、1回行う。成功すると、数えた分を戻し、要求に付いていた資格のログイン状態を無効にして、その接続を閉じ、新しいログイン状態を作る。`httpapi` が、資格を、Cookie にだけ載せる。
2. ログアウトでは、`usecase.SessionKeeper` が、ログイン状態を無効にしてから、そのログイン状態による接続を閉じる。
3. 「現在のログイン状態を確かめる」は、Cookie の資格だけを見る。ログインしていない場合も、拒否としない。

**相手の特定**(`GET /api/users/{handle}`)

`usecase.PeerFinder` が、`peerResolver` で相手を特定し、識別名(登録した通り)と表示名を返します。会話は、開きません。

**送信**(`POST /api/direct-conversations/{peer}/messages`)

1. `httpapi` が、要求の本文を復号し、ユースケースの入力に詰め替える。文字列は、置き換えずに復号し、正しくない並びを、正しくないまま渡す。形式に合わない要求は、拒否せずに、印(`Malformed`)を付けて渡す。ログイン状態の確認を、形式の確認より先に行うためである。
2. `usecase.MessageSender` が、ログイン状態の確認、形式、本文の検証(`domain.NewBody`)、送信の識別子の検証、相手の指定(`peerResolver`)を、この順に行う。
3. `Transactor.InTransaction` の中で、会話の成立、追記の直列化(会話の行のロック)、メッセージの追記を、一体として行う。連番と送信の日時は、保管先が付ける(ADR-010)。確定は、要求の取り消し(端末の切断)に従わず、時間の上限を設けて行う(ADR-012)。
4. 追記が、重複した送信(`port.ErrDuplicateSend`)を返した場合、トランザクションを取り消した後に、保管済みのメッセージを取得する。同じ会話のものであれば、配信せずに、完了を示す(ADR-009、ADR-012)。
5. 保管が確定した後に、`MessageDelivery.Deliver` を呼び、完了の応答を返す。

**配信**(`GET /api/realtime?session=<ログイン状態の識別子>` の WebSocket)

1. `wsaccept` が、接続を確立する前に、書き込み口(送信待ちの列)を作り、`usecase.ConnectionKeeper.Accept` を呼ぶ。ユースケースが、ログイン状態を確かめた後に、`ConnectionRegistry.Register` を呼び、登録の後に、もう一度確かめる。
2. `wsaccept` が、接続を確立し、受信を開始できたことの知らせ(`ReadyEvent`)を届ける。以後、列から取り出して、書き込む。
3. `wsdelivery` の `Deliver` は、`MessageEvent` を組み立て、参加者の、登録されている書き込み口のすべてに、渡す。端末への書き込みは、待たない。
4. ログイン状態が無効になると(ログアウト、同じブラウザでのログインし直し、有効期限)、`wsdelivery` が、書き込み口に、そのことを伝えて閉じ、`wsaccept` が、接続を、終了コード 4401 で閉じる。
5. 接続が終わると、`wsaccept` が、`ConnectionKeeper.Close` を呼び、ユースケースが、`ConnectionRegistry.Unregister` を呼ぶ。

**取得**(`GET /api/direct-conversations/{peer}/messages`)

`usecase.HistoryReader` が、ログイン状態と、相手の指定(`peerResolver`)を確かめ、メッセージを連番の順に返します。成立していない会話は、空の一覧を返します。会話は、成立させません。

**片付け**(要求によらない)

`adapter/schedule` が、起動の直後と、以後1分ごとに、`usecase.Housekeeper.Sweep` を呼びます。期限に達したログイン状態による接続を閉じ、期限を過ぎたログイン状態と、登録の申し込みを、保管先から消します。

### 出来事の記録

出来事の記録(仕様「記録」)は、usecase が、`EventRecorder` を呼んで行い、`adapter/eventlog` が、ログに出力します(ADR-021)。ユースケースを呼ぶ adapter は、ユースケースが返した拒否とエラーを、ログに出力しません。記録する項目は、`domain.Event` が持つものだけです。連絡先、識別名、表示名、秘密、資格、本文は、項目がありません。

### 状態の置き場所(PERF-3、WS-35、RL-39)

1つのサーバーの中だけで保持する状態は、2つです。どちらも、ポートを実装する adapter の中にあり、内側の層は、この状態を知りません。

| 状態 | 置き場所 | 背後にあるポート |
|---|---|---|
| 接続中の端末、配信の宛先、接続とログイン状態の対応 | `adapter/wsdelivery` の `Hub` | `MessageDelivery`、`ConnectionRegistry`、`SessionConnections` |
| 上限の計数 | `adapter/ratelimit` の `Limiter` | `AttemptLimiter` |

複数サーバー構成で要る、サーバーの間の中継と、計数の共有は、これらのポートの実装として加えられます(ADR-002、ADR-016)。本スライスは、単一のサーバーで動かし、その仕組みは作っていません。

配信の adapter は、保持したログイン状態が有効かどうかを、domain の判定(`Session.ValidAt`)で確かめます。ポートを実装する adapter が、domain の判定を呼ぶ、例外です(ADR-020)。

操作(HTTP)の間に、サーバーが保持する、端末ごとの状態は、ありません。ログイン状態と、登録の申し込みは、保管先に置きます。会話、識別名、連絡先、送信の識別子の一意性、メッセージの順序、同時の登録の完了は、保管先の制約とトランザクションで保ちます(ADR-004、ADR-016)。

### 通信の形式への変換が、2箇所にあること

同じメッセージを、通信の形式(`openapi.Message`)へ変換する箇所が、2つあります。`httpapi` は、ユースケースの出力から変換し、`wsdelivery` は、domain の型から変換します。2つの adapter は、互いに import できないためです(ADR-005 の Consequences)。食い違いは、`cmd/server` のテスト(`TestDelivery_ReachesAllDevicesOfParticipants`)が、完了の応答、履歴、配信の内容を比べて、検出します。

同じ理由で、次の処理も、`httpapi` と `wsaccept` の2箇所にあります。拒否の応答(`{"code": …}`)を書く処理。資格を載せる Cookie の名前。送信元(要求の接続の相手のアドレス)の取り出し。食い違いは、ログイン状態が有効でない要求に、すべての操作が同じ応答を返すことを確かめるテスト(`TestInvalidSession_IsRejectedByEveryOperation`)が、検出します。

### 保管先

表は、次の5つです(`server/migrations/`)。

| 表 | 内容 |
|---|---|
| `users` | 利用者。連絡先(揃えた後の値。一意)、識別名(入力された通り。小文字にした値で、一意)、表示名、保管する形の秘密 |
| `sessions` | ログイン状態。資格の要約(一意)、利用者、有効期限 |
| `signups` | 登録の申し込み。連絡先(主キー)、資格の要約(一意)、有効期限 |
| `conversations` | 会話。2人の組に、一意性の制約。2人は、`users` を参照する |
| `messages` | メッセージ。主キーは、会話と連番。送信者と送信の識別子の組に、一意性の制約。送信者は、`users` を参照する |

資格そのものは、保管しません。無効にしたログイン状態と、使用済みの申し込みは、行を削除します。有効かどうかは、usecase が渡した時刻と、有効期限を比べて、判定します。

保管先が返したエラーは、そのまま内側へ返しません。操作の名前と SQLSTATE だけを持つエラー(`postgres.OpError`)に、作り直します。元のエラーの詳細には、行の内容(本文、連絡先など)が含まれることがあるためです(WS-33、RL-33)。`smtpmail` も、同じく、操作の名前と、SMTP の応答の番号だけを持つエラー(`smtpmail.SendError`)に、作り直します。

## クライアントの層

| 層 | 置き場所 | 置くもの |
|---|---|---|
| application | `src/application`、`src/application/port` | 状態と操作(`chatApp.ts`)、履歴と配信の統合(`messages.ts`)、型(`model.ts`)、ポート |
| adapter | `src/adapter` | ポートの実装。通信の形式(生成した型)と、application の型の間の変換 |
| ui | `src/ui` | React の部品。application の状態を表示し、利用者の操作を application に伝える |
| main | `src/main.tsx` | 組み立てと、起動 |

domain の層は、置いていません。業務の規則は、サーバーが持ちます。クライアントは、送信の前に、入力(本文、連絡先、識別名、表示名、秘密)を検証しません。

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
| `AuthApi` | ログイン状態を確かめる、ログイン、ログアウト、登録の申し込み、登録の完了 | `httpAuthApi.ts`(`fetch`) |
| `ChatApi` | 相手の特定、履歴の取得、送信。ログイン状態の識別子を、要求ヘッダーで添える | `httpChatApi.ts`(`fetch`) |
| `RealtimeChannel` | 接続の確立と終了。受信の開始、メッセージ、切断(ログイン状態が無効になったことによるものかどうか)の通知 | `webSocketRealtime.ts`(ブラウザの `WebSocket`) |
| `EntryLink` | 案内の資格を、アドレスから読み取り、アドレスから除く | `locationEntryLink.ts`(`location.hash`、`history.replaceState`) |
| `Timer` | 時間の経過 | `browserTimer.ts` |
| `SendIdSource` | 送信の識別子を作る | `cryptoSendIdSource.ts`(`crypto.randomUUID`) |

ログイン状態を示す資格は、ブラウザが、Cookie として保ち、要求に付けます。クライアントのコードは、資格に触れません(ADR-014)。

### 振る舞い

`ChatApp`(`src/application/chatApp.ts`)が、状態を持ち、変化を通知します。React には、依りません。ui は、`subscribe` と `getState` を、React の `useSyncExternalStore` につなぎます。

振る舞いの規則(C-1〜C-19)は、`chatApp.ts` の先頭に挙げています。要点は、次の通りです。

- 開いた直後に、案内の資格を読み取る。あれば、登録を続ける画面を示す。なければ、ログイン状態を確かめ、ログイン中であれば、利用者と、ログイン状態の識別子を保持して、接続を確立する。確かめられなければ、ログインしていないとは示さず、やり直せるようにする。
- ログイン状態の識別子と、案内の資格は、状態の外に持ち、ui には渡さない。入力欄の内容(連絡先、識別名、表示名、秘密)は、ui の部品が持ち、状態には保たない。
- どの操作でも、認証の拒否の応答を受けたときと、接続が、ログイン状態が無効になったことを示して閉じられたときに、接続を閉じ、会話、相手の特定の結果、入力中の本文、識別子を捨てて、ログインを求める画面に切り替える。それより前の結果は、後から届いても、捨てる。
- ログアウトを完了できなかった場合は、できなかったことを示し、ログイン中のままとする。
- ログイン、登録の申し込み、登録の完了の結果を待つ間は、ログインしていないときの画面を、切り替えない。
- 相手は、識別名を入力して特定し、結果(識別名と表示名)を示してから、会話を開く。

- 履歴、完了の応答、配信のどれで届いたメッセージも、連番をキーとして統合し、連番の順に示す(ADR-010)。
- 会話を開くときは、WebSocket の接続が、受信を開始できたことの知らせを受けた後に、履歴を取得する。履歴の取得中に届いた配信は、捨てずに、統合する。
- 送信中のメッセージと、送信できなかったメッセージを、送信の識別子をキーとして保持する。完了の応答と、配信のうち、先に届いた方で、完了したメッセージに置き換える(ADR-011)。
- 拒否された本文は、入力欄に戻す。10秒以内に応答を得られない送信は、送信できなかったものとして示し、同じ送信の識別子で、送り直せる。

### 開発用のサーバーのログ

クライアントの開発用のサーバー(Vite)の記録から、`/api/` で始まるパスを伏せます(`client/tools/redactApiPaths.ts`)。パスは、識別名を含むためです(RL-33)。

## 実行環境の構成(ローカルの開発環境)

本スライスまでは、ローカルの開発環境でだけ動かします(仕様 RL-D1、ADR-007)。通信経路と、SMTP の経路を、暗号化しないためです(SEC-2)。

| プロセス | 待ち受け先 | 動かし方 |
|---|---|---|
| クライアントの開発用のサーバー(Vite) | 127.0.0.1:5173 | `make dev-client`。ブラウザは、`http://localhost:5173` にアクセスする |
| サーバー(Go) | 127.0.0.1:8080 | `make dev-server`。待ち受け先と、SMTP の接続先は、ループバックのアドレスに限る(それ以外では、起動に失敗する) |
| PostgreSQL 18(コンテナ) | 127.0.0.1:5432 | `make db-up`(Docker Compose)。データは、名前を付けたボリュームに置く |
| Mailpit(コンテナ) | 127.0.0.1:1025(SMTP)、127.0.0.1:8025(画面と API) | `make db-up`。受け取った案内を、外へ送らずに溜める。画面は、`http://127.0.0.1:8025` で開く(ADR-018) |
| E2Eテスト用の、サーバーと、クライアントの開発用のサーバー | 127.0.0.1:18080、127.0.0.1:15173 | `make e2e` の中で、Playwright が起動する |

- ブラウザから見たオリジンは、1つ(`http://localhost:5173`)です。サーバーは、Origin ヘッダーがあり、このオリジンと一致しない要求を、拒否します。CORS の許可は、返しません。
- サーバーは、Host ヘッダーが、このオリジンのホスト(`localhost:5173`)でも、ループバックのアドレスの表記(`127.0.0.1:8080` など)でもない要求を、拒否します。第三者のサイトが、自分のホストの名前を、サーバーのアドレスに向け直す場合(DNS rebinding)への対策です(ADR-002)。クライアントの開発用のサーバーは、Host を書き換えずに中継します。
- 秘密(保管先への接続情報)と、設定(アプリのオリジン、SMTP の接続先、案内の送信元、上限の値)は、リポジトリが追跡しない `.env` で与えます。必須のものが与えられていない場合、保管先の定義とサーバーは、起動に失敗します。上限の値は、与えなければ、既定値で働きます。
- 資格の Cookie は、ローカルの開発環境では、Secure と、名前の `__Host-` 接頭辞を付けません。Cookie は、ポートを区別しないため、Mailpit の画面は、`localhost` ではなく、`127.0.0.1` で開きます(ADR-014、ADR-018)。
- データベースは、3種類を使い分けます。開発用(`.env` の `PGDATABASE`)、サーバーのテスト用(雛形の `chat_test_template` と、テストごとの複製)、E2E用(`chat_e2e`)です。
- 開発者の端末は、Windows の上の WSL2 です(ネットワークは、nat)。Docker は、Windows の側の Docker Desktop を、WSL との連携によって使います。保管先と Mailpit のポートは、WSL2 の中と、Windows の側の、127.0.0.1 にだけ現れます。Windows の側のブラウザは、`http://localhost:5173` で、クライアントの開発用のサーバーに届きます。

AWS上の検証環境は、まだありません。設置環境の構成(OPS-1)、通信経路の暗号化(SEC-2)、メールを送信する手段は、フェーズ1の、設置環境を扱うスライスで定めます。

## 検査とテスト

| 手順 | 確かめること |
|---|---|
| `make check-deps` | 依存規則と飛ばし禁止(depguard、dependency-cruiser、ESLint)。違反を検出したとき、出力を、[検査の記録](dependency-check-log.md)に追記する |
| `make check-deps-selftest` | 規則に違反する見本(サーバーは、ビルドタグ `depcheck_sample` を付けたファイル。クライアントは、各層の `__depcheck__/`)が、すべて、違反として検出されること |
| `make generate-check` | 生成したコードが、定義と一致すること |
| `make lint` | golangci-lint、ESLint、TypeScript の型の検査。依存規則は、含めない |
| `make test-server` | domain、usecase、adapter のテストと、サーバーの振る舞いのテスト(`cmd/server`。実際の PostgreSQL を使い、秘密の計算も、実際に行う。時刻の源と、案内の送信は、差し替える)。`smtpmail` のテストは、Mailpit を使う |
| `make test-client` | application のテスト(ブラウザのAPIのない環境)と、adapter、ui、`tools/` のテスト(jsdom) |
| `make e2e` | サーバーとクライアントをつないだ動作(Playwright。Chromium)。案内は、Mailpit の API で読む |
| `make secrets-scan` | リポジトリの履歴に、秘密が含まれないこと(gitleaks) |

テストの置き場所は、層の規則から決まっています。`httpapi`、`wsaccept`、`schedule` は、ユースケースの具体的な型を受け取り、`application/port` を import できないため、パッケージの中のテストでは、ユースケースを組み立てません。ユースケースを通る振る舞いは、すべての層を import できる `cmd/server` のテストで確かめます。
