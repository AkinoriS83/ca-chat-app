# ウォーキングスケルトン 詳細設計

本書は、[開発方針](../development-policy.md)「設計フェーズの進め方」が定める、使い捨ての詳細設計です。[ウォーキングスケルトンの仕様](../specs/walking-skeleton.md)と、ADR-001〜013([docs/adr/](../adr/))を、実装できる形に具体化します。

本書の前提は次の通りです。

- 本書は、これからどう実装するかを定める。採用の理由は、ADRに従い、繰り返さない。本書で新たに定めた事項は、「本書で定めた事項」に、理由とともに挙げる。
- 本書は、実装の完了後に削除する。残す内容は、アーキテクチャ概要(現在の姿)と、ADR(判断)が受け持つ。
- 実装の中で、本書と異なる形が必要になった場合は、本書を直してから実装する。アーキテクチャ上の判断に当たる場合は、ADRを改めるか、新しいADRを起こす。
- 本書の「確認済み」は、2026-11-20 に、開発者の端末(Go 1.26.8、Node.js 24.16.0)で、小さな試作によって確かめたことを指す。

## 1. 本書で定めた事項

仕様とADRが「設計で定める」「詳細設計で定める」とした事項と、具体化する中で判断が要った事項です。

### 仕様とADRが、詳細設計に送った事項

| ID | 事項 | 送り元 | 定めた内容 | 節 |
|---|---|---|---|---|
| DD-01 | 画面の構成と操作の手段 | 仕様「後続に送る事項」 | 1つの画面に、利用者の選択、相手の選択、会話、入力欄を置く | 5.4 |
| DD-02 | 仮実装の箇所の一覧の置き場所 | 仕様「仮実装」 | `docs/temporary-implementations.md`。コードには、`仮実装(WS-Pn)` のコメントを付ける | 6.6 |
| DD-03 | 動作検証の手順8、手順10の具体化 | 仕様「動作検証」 | 手順8は、開き直さずに送り直す。手順10は、画面からそのまま送る(DD-12) | 8 |
| DD-04 | 本文を、置き換えずに復号する手段 | ADR-001、ADR-003 | 本文の項目を `json.RawMessage` で受け、JSON の文字列を、自前の関数で復号する | 4.5.1 |
| DD-05 | 端末ごとの書き込みの時間の上限 | ADR-002 | 1通につき5秒。端末ごとの送信待ちの列は、256通まで。超えた端末の接続は、閉じる | 4.5.2 |
| DD-06 | 仮実装(WS-P1、WS-P2)の置き場所 | ADR-005 | サーバーは `adapter/fixeduser` と、利用者を要求から取り出す箇所。クライアントは `adapter/staticFixedUsers.ts` | 4.5.5、5.3 |
| DD-07 | E2Eテストで、データを空に戻す手段 | ADR-006 | テストごとに、E2E用のデータベースの表を TRUNCATE する | 7.4 |
| DD-08 | 「配信されない」ことの判定の方法 | ADR-006 | 待ち時間ではなく、後から送る「番兵」のメッセージが先に届くことで判定する | 7.2 |
| DD-09 | AIへの常設の指示の置き場所 | ADR-008 | リポジトリの直下の `CLAUDE.md` | 6.5 |
| DD-10 | 検査の記録の置き場所 | ADR-008 | `docs/dependency-check-log.md` | 6.4 |
| DD-11 | 利用者を添えるヘッダーとクエリの名前。再読み込みの後も選択を保つか | ADR-013 | ヘッダーは `X-Fixed-User`、クエリは `user`。選択は、保たない | 3.2、5.2 |

### 具体化する中で判断した事項

| ID | 判断 | 理由 |
|---|---|---|
| DD-12 | クライアントは、送信の前に、本文を検証しない。送信ボタンは、入力欄が空の文字列のときだけ、無効にする | 仕様は、クライアントでの検証を任意としている(WS-D3「検証する場合も」)。検証すると、本文の規則を TypeScript でも持つことになり、Go と同じ見本でのテストが要る(ADR-001)。検証しなければ、拒否の表示(WS-40)が通常の経路になり、動作検証の手順10を、画面からそのまま行える。往復は、ローカルでは短い。入力中の文字数の表示などは、フェーズ1の仕様で判断する |
| DD-13 | 操作は、会話を、相手の識別名で指定する。会話の識別子で指定する操作は、置かない | 本スライスは、会話を、相手を選んで開く(WS-P3)。成立していない会話には、識別子がない。会話の一覧(UC-10)は対象外であり、識別子で指定する操作を使う画面がない |
| DD-14 | 利用者の識別子(`UserID`)を、識別名とは別の値にする。保管するのは、識別子とする | 識別名は、フェーズ1で、本人が変更でき、他の人が再利用できる(D-1)。識別名を保管の鍵にすると、仮のログインの置き換えが、保管先と domain に及ぶ。別の値にしておけば、置き換えは、ADR-013 の通り、adapter とポートの実装に閉じる |
| DD-15 | メッセージの送信者は、識別名と表示名の組で、サーバーが返す。クライアントの固定ユーザーの表は、利用者と相手の選択にだけ使う | 送信者の表示(WS-20)が、サーバーから画面まで通る。クライアントの仮実装が、選択の2箇所に限られる |
| DD-16 | 拒否の種類に、「利用者を特定できない」を加える(計5種類) | 固定ユーザーであることは、usecase がポートを通して確かめる(ADR-013)。その結果を、adapter に伝える種類が要る(WS-16) |
| DD-17 | 接続への書き込み口は、受け付けの adapter が持つ「送信待ちの列」とする。受け付けの adapter は、接続を確立する前に、書き込み口を作ってユースケースを呼ぶ | 利用者を特定できない要求は、接続を確立せずに拒否する(ADR-002)。利用者を確かめるのは usecase であり、書き込み口は、そのユースケースの入力に含めて渡す(ADR-005)。両方を満たすには、書き込み口が、接続の確立より前に要る(4.5.2) |
| DD-18 | サーバーの振る舞いのテストは、`server/cmd/server`(main)の中に置く | すべての層を import できるのは、main だけである(ADR-005)。組み立てたサーバーを使うテストを、別のパッケージに置くと、規則の例外が要る |
| DD-19 | 開発に使う Go の道具は、版を Makefile に記録し、リポジトリの中の `bin/` に導入する。サーバーの go.mod には、加えない | サーバーの依存物を、少なく保つ(ADR-001)。道具ごとに、公開された通りの依存物で、ビルドされる |
| DD-20 | テスト用のデータベースは、マイグレーションを適用した雛形のデータベースから、複製して作る | テストごとに独立したデータベース(ADR-006)を、速く作れる。goose を、サーバーの依存物に加えずに済む |
| DD-21 | TypeScript は、5.x 系に固定する | openapi-typescript 7.13.0 が、TypeScript の 5.x 系を求める(確認済み。npm の最新は 7.0.2 である) |
| DD-22 | 要求の本文の大きさの上限を、1 MiB とする | 正しい送信の要求は、最大でも約48KBである(4,000文字が、すべてエスケープされた場合)。上限を超える要求は、読み切らずに拒否する(WS-30) |

DD-13 の帰結として、仕様の拒否の表にある「利用者(送信者)が参加者でない会話を指定した」は、本スライスの操作では、表せません。該当する要求の形がないためです。参加者でない利用者に対する保護(WS-23、WS-24)は、「どの操作によっても取得・送信できない」ことで確かめます(7.3)。「見つからない」応答は1種類だけであり、WS-15、WS-38 の「同じ応答」は、これで成り立ちます。

## 2. 全体の構成

### ディレクトリ

```
api/openapi.yaml                 通信の形式の定義(ADR-003)
server/                          Go のモジュール
  cmd/server/                    main。設定、組み立て、起動。サーバーの振る舞いのテスト
  internal/domain/
  internal/application/port/
  internal/application/usecase/
  internal/adapter/httpapi/      HTTP の操作を受ける(ユースケースを呼ぶ)
  internal/adapter/wsaccept/     接続の受け付け(ユースケースを呼ぶ)
  internal/adapter/wsdelivery/   配信(ポートを実装する)
  internal/adapter/postgres/     保管先(ポートを実装する)
    queries/                     sqlc に与える SQL
    sqlcgen/                     sqlc が生成したコード
  internal/adapter/fixeduser/    固定ユーザー(ポートを実装する。仮実装)
  internal/openapi/              oapi-codegen が生成したコード
  migrations/                    goose のマイグレーション
  .golangci.yml  sqlc.yaml  oapi-codegen.yaml
client/                          npm のパッケージ
  src/application/  src/application/port/
  src/adapter/  src/adapter/generated/   openapi-typescript が生成した型
  src/ui/
  src/main.tsx
  e2e/                           Playwright のテスト
  vite.config.ts  eslint.config.js  .dependency-cruiser.cjs  playwright.config.ts
docs/
  architecture.md                アーキテクチャ概要(実装とともに作成)
  temporary-implementations.md   仮実装の箇所の一覧(実装とともに作成)
  dependency-check-log.md        検査の記録
scripts/                         Makefile から呼ぶ手順
compose.yaml  Makefile  .env.example  .gitignore  README.md  CLAUDE.md
```

Go のモジュールのパスは、公開先のリポジトリのパスに合わせます。リモートが未設定のため、実装の着手時に、開発者に確認します(10.2)。本書では、`<module>` と書きます。

### プロセスと待ち受け先

| プロセス | 待ち受け先 | 備考 |
|---|---|---|
| クライアントの開発用のサーバー(Vite) | 127.0.0.1:5173 | ブラウザは、`http://localhost:5173` にアクセスする。`/api` で始まる要求と、WebSocket の接続を、サーバーへ中継する |
| サーバー | 127.0.0.1:8080 | |
| PostgreSQL(コンテナ) | 127.0.0.1:5432 | Docker Desktop が、WSL2 の中と、Windows の側の、127.0.0.1 に公開する(6.2) |
| E2Eテスト用の、サーバーと、クライアントの開発用のサーバー | 127.0.0.1:18080、127.0.0.1:15173 | E2Eテストの手順が起動する(7.4) |

### 値の一覧

| 値 | 内容 | 出どころ |
|---|---|---|
| 本文の長さの上限 | 4,000 コードポイント | 仕様 WS-D3 |
| 送信の応答を待つ時間(クライアント) | 10秒 | 仕様「メッセージを送る」 |
| 要求の本文の大きさの上限 | 1 MiB | DD-22 |
| 端末への1通の書き込みの時間の上限 | 5秒 | DD-05 |
| 端末ごとの送信待ちの列の長さ | 256通 | DD-05 |

## 3. 通信の形式

`api/openapi.yaml`(OpenAPI 3.0.3)に定義します。試作で、下記の形の文書から、サーバーのコードを生成できることを確かめました(確認済み)。

### 3.1 操作

| 操作 | 方法とパス | 要求 | 完了の応答 |
|---|---|---|---|
| 会話の履歴を読む(`listDirectMessages`) | `GET /api/direct-conversations/{peer}/messages` | なし | 200 `MessageList` |
| メッセージを送る(`sendDirectMessage`) | `POST /api/direct-conversations/{peer}/messages` | `SendMessageRequest` | 200 `SendMessageResponse` |

- `{peer}` は、相手の識別名である。
- 成立していない会話の履歴は、200 で、空の一覧を返す(WS-21)。
- 送り直しで、保管済みのメッセージについて完了を示す場合も、最初の送信と同じ 200 と、保管済みのメッセージを返す(WS-18)。
- どちらの操作も、拒否とエラーは、`ErrorResponse` を返す(3.4)。

### 3.2 利用者の添え方(仮実装 WS-P2)

| 経路 | 添え方 |
|---|---|
| HTTP の操作 | 要求ヘッダー `X-Fixed-User: <識別名>` |
| WebSocket の接続を確立する要求 | クエリ `user=<識別名>` |

OpenAPI の文書では、操作のパラメータではなく、`securitySchemes`(`apiKey`、`in: header`)として記述します。生成したコードは、これを取り出しません。adapter が、要求から取り出します。フェーズ1の認証で置き換える箇所を、ここに限るためです。

### 3.3 スキーマ

| スキーマ | 項目 |
|---|---|
| `UserProfile` | `handle`(識別名)、`displayName`(表示名) |
| `Message` | `participants`(会話の参加者2人の識別名。昇順)、`seq`(連番。int64)、`sender`(`UserProfile`)、`body`、`sentAt`(date-time。UTC) |
| `MessageList` | `messages`(`Message` の配列。連番の順) |
| `SendMessageRequest` | `sendId`(制約のない文字列)、`body`(制約のない文字列) |
| `SendMessageResponse` | `message`(`Message`) |
| `ErrorCode` | 3.4 の値の列挙 |
| `ErrorResponse` | `code`(`ErrorCode`) |
| `ReadyEvent` | `type`(`ready`) |
| `MessageEvent` | `type`(`message`)、`message`(`Message`)、`sendId` |
| `ServerEvent` | `ReadyEvent` と `MessageEvent` の `oneOf`。`type` で判別する |

- 項目は、すべて必須とする。
- 「会話を判別できる情報」(ADR-010)は、`participants` とする。完了の応答、履歴、配信のどの経路でも、`Message` に含まれる。
- 送信の識別子は、`MessageEvent` にだけ含め、`Message` には含めない(ADR-011)。
- `sentAt` は、保管先が返した値(マイクロ秒まで)を、UTC にして、そのまま表す。3つの経路で、同じ文字列になる。

### 3.4 拒否とエラーの応答

応答は、`{"code": "<値>"}` だけを返します。文言、詳細、内部の情報は、含めません(WS-30)。クライアントは、`code` によって、利用者に示す文言を選びます(WS-40)。

| `code` | 状態 | 場合 | 判定する層 |
|---|---|---|---|
| `forbidden_origin` | 403 | Origin があり、アプリのオリジンと一致しない(WS-29) | adapter |
| `invalid_input` | 400 | 要求が、形式に合わない(JSON として壊れている、必須の項目がない、項目の型が違う、大きさの上限を超える) | adapter |
| `unauthenticated` | 401 | 利用者が添えられていない。固定ユーザーの識別名でない(WS-16) | usecase |
| `invalid_body` | 400 | 本文が WS-D3 の規則を満たさない | usecase |
| `invalid_input` | 400 | 送信の識別子の形式が正しくない。送信の識別子が、同じ送信者の、別の会話の保管済みのメッセージと一致する | usecase |
| `invalid_peer` | 400 | 相手として自分自身を指定した | usecase |
| `not_found` | 404 | 相手が固定ユーザーの中に居ない。定義されていないパスと方法 | usecase、adapter |
| `internal` | 500 | システムが操作を完了できなかった(WS-31) | adapter |

1つの要求が、複数の場合に当たるときは、次の順で、最初に当たったものを返します。Origin → 形式 → 利用者 → 仕様の拒否の表の順(本文、送信の識別子の形式、自分自身、相手が居ない、別の会話との一致)。

形式を、利用者より先に確かめるのは、利用者を確かめるのが usecase であり(ADR-013)、usecase を呼ぶには、要求を復号している必要があるためです。

### 3.5 WebSocket

- 接続先は、`GET /api/realtime?user=<識別名>` とする。OpenAPI の「操作」としては記述せず、文書の説明に書く(ADR-003)。パスとクエリの名前は、サーバーとクライアントに、それぞれ定数として持つ。
- 接続を確立する前の拒否(`forbidden_origin`、`unauthenticated`)は、HTTP の応答として、`ErrorResponse` を返す。
- サーバーは、接続を確立した後、最初に `ReadyEvent` を1通届ける。以後、`MessageEvent` を届ける。どちらも、テキストのメッセージ(JSON)とする。
- 端末からは、何も送らない。データのメッセージを受けた場合、サーバーは、接続を閉じる。

### 3.6 生成

| 生成するもの | 道具 | 出力先 | 設定 |
|---|---|---|---|
| サーバーの型と、操作を受けるインターフェイス | oapi-codegen | `server/internal/openapi/` | `std-http-server` と `models` を生成する。どの操作からも参照されないスキーマ(イベント)を残すため、`skip-prune` を有効にする |
| クライアントの型 | openapi-typescript | `client/src/adapter/generated/openapi.d.ts` | |

- 生成したインターフェイス(`ServerInterface`)は、`(w, r, peer string)` の形であり、要求の本文は、復号しない(確認済み)。復号は、adapter が行う(4.5.1)。
- 生成したコードは、パスのパラメータの復号に、oapi-codegen/runtime を import する(確認済み)。サーバーの実行時の依存物になる。
- 生成した `HandlerWithOptions` の既定のエラー処理は、エラーの文言を、そのまま応答に書く。`ErrorHandlerFunc` を必ず与え、`invalid_input` の応答に置き換える(WS-30)。
- `ServerEvent` は、Go では、扱いにくい型になる。サーバーは、`ReadyEvent` と `MessageEvent` を、直接に使う。

## 4. サーバー

### 4.1 パッケージと、import してよい先

ADR-005 の規則を、パッケージに当てはめたものです。depguard に、パッケージごとに、この通りに列挙します。列挙にない import は、違反です。

| パッケージ | import してよい先(標準ライブラリのほか) |
|---|---|
| `domain` | なし |
| `application/port` | `domain` |
| `application/usecase` | `application/port`、`domain` |
| `adapter/httpapi` | `application/usecase`、`openapi` |
| `adapter/wsaccept` | `application/usecase`、`openapi`、coder/websocket |
| `adapter/wsdelivery` | `application/port`、`domain`、`openapi` |
| `adapter/postgres` | `application/port`、`domain`、`adapter/postgres/sqlcgen`、pgx |
| `adapter/postgres/sqlcgen` | pgx |
| `adapter/fixeduser` | `application/port`、`domain` |
| `openapi` | oapi-codegen/runtime |
| `cmd/server` | すべて |

- `domain` と `application` は、入出力を行う標準ライブラリのパッケージ(`net/http`、`database/sql`、`os`、`log/slog` など)を、import しない。これも、depguard の規則に含める。
- ログは、adapter と main が出力する。`domain` と `application` は、ログを出力しない。

### 4.2 domain

| 型 | 内容と規則 |
|---|---|
| `UserID` | 利用者の識別子。UUID の文字列(小文字)。識別名とは別の値(DD-14) |
| `User` | `ID`、`Handle`(識別名)、`DisplayName`(表示名) |
| `DirectPair` | 1on1会話の2人の `UserID`。`NewDirectPair(a, b)` で作る。2人が同じ場合は、`ErrSelfConversation` を返す。2人を、文字列の昇順に並べて保持する(どちらが相手を選んでも、同じ値になる) |
| `ConversationID` | 会話の識別子。UUID の文字列 |
| `Conversation` | `ID`、`Pair` |
| `Body` | 本文。`NewBody(raw)` で作る。WS-D3 の規則を満たさない場合は、`ErrInvalidBody` を返す |
| `SendID` | 送信の識別子。`ParseSendID(raw)` で作る。形式が正しくない場合は、`ErrInvalidSendID` を返す。小文字に揃えて保持する |
| `Message` | `ConversationID`、`Seq`、`SenderID`、`SendID`、`Body`、`SentAt` |

`NewBody` は、次の順で判定します。

1. 文字列として正しいこと(`utf8.ValidString`)。adapter は、対にならないサロゲートを、正しくない UTF-8 の並びとして渡す(4.5.1)。
2. 改行の正規化(CRLF と、単独の CR を、LF に置き換える)。以後は、正規化した後の値を扱う。
3. 制御文字(`unicode.Cc`)を含まないこと。U+000A と U+0009 は、除く。
4. 空白文字(`unicode.White_Space`)以外の文字を、1文字以上含むこと。
5. コードポイントの数(`utf8.RuneCountInString`)が、4,000 以下であること。

`ParseSendID` は、36文字で、ハイフンが 8-4-4-4-12 の位置にあり、ほかが16進数であることだけを確かめます(ADR-009)。UUID の版は、問いません。外部のライブラリは、使いません。

`DirectPair` の並びは、保管先の `uuid` の型の大小と一致します。どちらも、小文字の16進数の、先頭からの比較になるためです。

### 4.3 application/port

境界要素として数える、型の宣言です(ADR-008)。インターフェイスは、7つです。

```go
// 利用者を調べる。本スライスの実装は、固定ユーザー(仮実装 WS-P1)。
type UserDirectory interface {
    FindByHandle(ctx context.Context, handle string) (user domain.User, found bool, err error)
}

// 保管先の読み取り。トランザクションの外で使う。
type ConversationReader interface {
    FindDirect(ctx context.Context, pair domain.DirectPair) (conv domain.Conversation, found bool, err error)
    ListMessages(ctx context.Context, id domain.ConversationID) ([]domain.Message, error) // 連番の順
    FindMessageBySend(ctx context.Context, sender domain.UserID, sendID domain.SendID) (msg domain.Message, found bool, err error)
}

// 一体として行う操作を、1つの関数として受ける。fn がエラーを返すと、すべて取り消す(ADR-012)。
type Transactor interface {
    InTransaction(ctx context.Context, fn func(ctx context.Context, w ConversationWriter) error) error
}

// トランザクションの中でだけ行える操作。InTransaction が、fn に渡す。
type ConversationWriter interface {
    // 会話を成立させる。既に成立していれば、その会話を返す。
    EstablishDirect(ctx context.Context, pair domain.DirectPair) (domain.Conversation, error)
    // その会話への追記を、直列にする。トランザクションが終わるまで、他の追記を待たせる。
    SerializeAppends(ctx context.Context, id domain.ConversationID) error
    // メッセージを追記する。連番と送信の日時を付けて返す。
    // 同じ送信者の、同じ送信の識別子のメッセージが保管済みであれば、ErrDuplicateSend を返す。
    AppendMessage(ctx context.Context, id domain.ConversationID, sender domain.UserID, sendID domain.SendID, body domain.Body) (domain.Message, error)
}

var ErrDuplicateSend = errors.New("duplicate send")

// 配信。保管が確定したメッセージを、participants がアプリを開いている端末のすべてに届ける。
// 送信待ちの列に加えるまでを、呼び出しの中で終える。端末への書き込みは、待たない。
type MessageDelivery interface {
    Deliver(ctx context.Context, participants []domain.User, msg domain.Message)
}

// 接続への書き込み口。受け付けの adapter が実装する。
type ConnectionSink interface {
    // 1通を、送信待ちの列に加える。待たずに戻る。列が一杯で、加えられなかった場合は、false を返す。
    Send(data []byte) (queued bool)
    // 接続を閉じる。何度呼んでもよい。
    Close()
}

// 接続中の端末の登録と解除。配信の adapter が実装する。
type ConnectionRegistry interface {
    Register(ctx context.Context, user domain.UserID, sink ConnectionSink)
    Unregister(ctx context.Context, sink ConnectionSink)
}
```

- `Deliver` は、エラーを返さない。配信が届かなくても、保管と完了の応答は、取り消さないためである(仕様「リアルタイムの配信」)。
- `Deliver` に、宛先ではなく、参加者を渡すのは、配信の adapter が、`Message` の通信の形式(`participants`、`sender`)を組み立てるのに、参加者の識別名と表示名が要るためである。宛先は、参加者の全員である。
- 接続は、書き込み口の値そのもので識別する。サーバーは、接続の識別子を持たない(ADR-011)。
- OBS-002 は、接続の登録と解除を、2つのポートと数えた。型の宣言としては、`ConnectionRegistry` の1つにまとめる。

### 4.4 application/usecase

#### 入力・出力のデータ構造と、拒否の種類

境界要素として数える、型の宣言です。domain の型を、含めません。

```go
type MessageView struct {
    Participants      []string // 2人の識別名。昇順
    Seq               int64
    SenderHandle      string
    SenderDisplayName string
    Body              string
    SentAt            time.Time
}

type SendMessageInput  struct{ Actor, Peer, SendID, Body string } // Actor、Peer は識別名
type SendMessageOutput struct{ Message MessageView }

type ReadHistoryInput  struct{ Actor, Peer string }
type ReadHistoryOutput struct{ Messages []MessageView }

type AcceptConnectionInput struct {
    Actor      string
    Connection port.ConnectionSink // ADR-005 が定める例外
}
type CloseConnectionInput struct{ Connection port.ConnectionSink }

type Rejection int // Unauthenticated、InvalidBody、InvalidInput、InvalidPeer、NotFound
type RejectedError struct{ Kind Rejection } // error を実装する
```

- ユースケースは、拒否を `*RejectedError` で、完了できなかったことを、そのほかの `error` で返す。adapter は、`errors.As` で見分ける。
- ユースケースは、3つの型に分ける。`MessageSender`(`Send`)、`HistoryReader`(`Read`)、`ConnectionKeeper`(`Accept`、`Close`)。main は、それぞれに、使うポートだけを渡す。

#### メッセージを送る(`MessageSender.Send`)

1. `UserDirectory.FindByHandle(Actor)`。居なければ、`Unauthenticated`。
2. `domain.NewBody(Body)`。エラーなら、`InvalidBody`。
3. `domain.ParseSendID(SendID)`。エラーなら、`InvalidInput`。
4. `UserDirectory.FindByHandle(Peer)`。居なければ、`NotFound`。
5. `domain.NewDirectPair(actor.ID, peer.ID)`。`ErrSelfConversation` なら、`InvalidPeer`。
6. `Transactor.InTransaction` の中で、`EstablishDirect` → `SerializeAppends` → `AppendMessage` を、この順に呼ぶ。エラーは、そのまま関数から返す(取り消される)。関数の中では、渡された操作のほかに、ポートを呼ばない。
7. 手順6が `ErrDuplicateSend` を返した場合(重複した送信)。
   1. `ConversationReader.FindMessageBySend(actor.ID, sendID)` で、保管済みのメッセージを得る。見つからなければ、完了できなかったものとして、エラーを返す。
   2. `ConversationReader.FindDirect(pair)` で、指定した会話を得る。会話がない、または、保管済みのメッセージの会話と異なる場合は、`InvalidInput`。
   3. 同じ会話であれば、配信せずに、保管済みのメッセージを出力として返す。
8. 手順6が、そのほかのエラーを返した場合は、そのエラーを返す。配信しない(WS-31)。
9. 手順6が確定した場合、`MessageDelivery.Deliver([actor, peer], msg)` を呼び、出力を返す。

自分自身を相手に指定した場合、相手は必ず見つかるため、手順4と5の順は、仕様の拒否の表の順(自分自身 → 相手が居ない)と、同じ結果になります。

#### 会話の履歴を読む(`HistoryReader.Read`)

1. `FindByHandle(Actor)`。居なければ、`Unauthenticated`。
2. `FindByHandle(Peer)`。居なければ、`NotFound`。
3. `NewDirectPair`。`ErrSelfConversation` なら、`InvalidPeer`。
4. `FindDirect(pair)`。会話がなければ、空の一覧を返す。会話は、成立させない(WS-06)。
5. `ListMessages(conv.ID)` を、出力に詰め替えて返す。送信者は、参加者2人のうち、`SenderID` が一致する方である。

#### リアルタイムの接続を受け付ける、接続の終わりを受ける(`ConnectionKeeper`)

- `Accept`: `FindByHandle(Actor)`。居なければ、`Unauthenticated`。居れば、`ConnectionRegistry.Register(user.ID, Connection)`。
- `Close`: `ConnectionRegistry.Unregister(Connection)`。ポートを呼ぶだけのユースケースである(ADR-008 の計数の対象)。

### 4.5 adapter

#### 4.5.1 httpapi(HTTP の操作を受ける)

- 生成した `ServerInterface` を実装する。受け取るのは、ユースケース(`*usecase.MessageSender`、`*usecase.HistoryReader`)と、ログの出力先だけである。
- 利用者は、`X-Fixed-User` ヘッダーの値を、そのまま、入力の `Actor` に渡す。ヘッダーがない場合は、空の文字列を渡す(usecase が、`Unauthenticated` にする)。この箇所は、仮実装(WS-P2)である。
- 出力(`MessageView`)を、生成した型(`openapi.Message`)に詰め替えて、JSON で返す。
- 拒否の種類を、3.4 の `code` と状態に対応させる。拒否でないエラーは、`internal`(500)を返し、ログに出力する(4.6)。

**要求の本文の復号(DD-04)**

1. `http.MaxBytesReader` で、1 MiB までを読む。超えた場合は、`invalid_input`。
2. 読んだ全体を、`encoding/json` で、`sendId` を文字列、`body` を `json.RawMessage` とする構造体に復号する。失敗した場合と、JSON の値の後ろに余分な内容がある場合は、`invalid_input`。
3. `sendId` と `body` のどちらかがない場合、または、`body` が JSON の文字列でない場合は、`invalid_input`。
4. `body` の JSON の文字列を、自前の関数で復号する。この関数は、次の通りに働く。
   - エスケープ(`\"`、`\\`、`\/`、`\b`、`\f`、`\n`、`\r`、`\t`、`\uXXXX`)を解く。対になったサロゲートのエスケープは、1つのコードポイントにする。
   - 対にならないサロゲートのエスケープは、置き換えずに、そのサロゲートを UTF-8 の規則でそのまま符号化した3バイトにする。これは、正しくない UTF-8 の並びであり、`NewBody` が拒否する。
   - エスケープでないバイトは、正しくない UTF-8 の並びを含め、そのまま写す。
5. 復号した文字列を、入力の `Body` に渡す。adapter は、本文を拒否しない。

`json.RawMessage` が、正しくない UTF-8 の並びと、対にならないサロゲートのエスケープを、そのまま保つことは、確かめました(確認済み)。文字列の型で受けると、どちらも U+FFFD に置き換わります。未知の項目は、無視します。

**Origin の検証と、共通の応答**

- `httpapi` は、Origin を検証するミドルウェアを公開する。Origin ヘッダーがあり、設定で与えたアプリのオリジンと一致しない要求に、`forbidden_origin`(403)を返す。Origin がない要求は、通す(ADR-002)。
- main は、このミドルウェアで、HTTP の操作と、WebSocket の接続先の両方を包む。
- CORS のヘッダーは、どの応答にも付けない。`OPTIONS` は、定義されていない方法として、`not_found` を返す。
- 定義されていないパスと方法には、`not_found` を返す。Go の既定の応答(文言)を、使わない。

#### 4.5.2 wsaccept(接続の受け付け)

受け取るのは、ユースケース(`*usecase.ConnectionKeeper`)、アプリのオリジン、ログの出力先です。

書き込み口(`port.ConnectionSink` を満たす型)は、長さ256の送信待ちの列と、閉じたことを示す印を持ちます。

- `Send` は、列に加えて、待たずに戻る。列が一杯であれば、加えずに、`false` を返す。
- `Close` は、閉じたことを印す。何度呼んでもよい。

要求の処理は、次の順です(DD-17)。

1. WebSocket の接続を確立する要求でない場合(`Upgrade` ヘッダーが `websocket` でない)は、`invalid_input`(400)を返す。クエリの `user` を取り出す(仮実装 WS-P2)。
2. 書き込み口を作り、`ConnectionKeeper.Accept` を呼ぶ。`Unauthenticated` であれば、接続を確立せずに、`unauthenticated`(401)を返す(WS-16)。
3. 以後、どの経路で終わる場合も、`ConnectionKeeper.Close` を呼ぶ(登録を解く)。
4. `websocket.Accept` で、接続を確立する。`OriginPatterns` に、アプリのオリジンを与える。失敗した場合は、終わる。
5. `CloseRead` を呼ぶ。端末がデータのメッセージを送った場合と、接続が切れた場合に、それを知るためである。
6. `ReadyEvent` を書き込む。登録(手順2)の後であり、ADR-010 の順を満たす。
7. 列から1通ずつ取り出して、書き込む。1通の書き込みは、5秒までとする。書き込みの失敗、時間切れ、書き込み口が閉じられたこと、接続が切れたことの、いずれかで終わり、接続を閉じる。

- 手順2から手順4の間に配信されたメッセージは、列に溜まり、手順7で届く。
- 手順4で、ライブラリが接続の確立を拒否した場合の応答は、ライブラリが書く。この応答も、内部の構造が分かる情報を含まないことを、テストで確かめる(WS-30)。
- 接続が切れたことを、書き込みを伴わずに確かめる仕組み(定期的な確認)は、作らない。接続が切れたことの扱いは、フェーズ1の仕様で定める(仕様「後続に送る事項」)。
- coder/websocket は、Origin がない要求を通し、Origin のホストが要求の Host と一致するか、`OriginPatterns` に当たる要求を通す(確認済み)。4.5.1 のミドルウェアが先に検証するため、二重の検証になる。

#### 4.5.3 wsdelivery(配信)

- `port.MessageDelivery` と `port.ConnectionRegistry` を実装する。接続中の端末と、配信の宛先を保持する状態は、このパッケージだけが持つ(PERF-3、WS-35)。
- 状態は、`UserID` から、書き込み口の集合への対応である。排他の制御のもとで、読み書きする。
- `Deliver` は、`MessageEvent` の JSON を、1回だけ組み立てる。参加者のそれぞれについて、登録されている書き込み口のすべてに、`Send` で渡す。`Send` が `false` を返した書き込み口は、`Close` で閉じる(DD-05)。登録は、受け付けの adapter が、接続の終わりとして解く。排他の制御を保ったまま、`Send` と `Close` を呼んでよい(どちらも、待たない)。
- `Message` の通信の形式への変換は、`httpapi` と、ここの、2箇所にある(ADR-005 の Consequences)。食い違いは、7.3 のテストで検出する。
- WebSocket のライブラリは、import しない。

#### 4.5.4 postgres(保管先)

`port.ConversationReader` と `port.Transactor` を実装します。接続は、pgx の接続プールを使います。

**構造(マイグレーション 00001)**

```sql
CREATE TABLE conversations (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low  uuid NOT NULL,
    user_high uuid NOT NULL,
    CONSTRAINT conversations_pair_order CHECK (user_low < user_high),
    CONSTRAINT conversations_pair_key UNIQUE (user_low, user_high)
);

CREATE TABLE messages (
    conversation_id uuid        NOT NULL REFERENCES conversations (id),
    seq             bigint      NOT NULL,
    sender_id       uuid        NOT NULL,
    send_id         uuid        NOT NULL,
    body            text        NOT NULL,
    sent_at         timestamptz NOT NULL,
    CONSTRAINT messages_pkey PRIMARY KEY (conversation_id, seq),
    CONSTRAINT messages_sender_send_key UNIQUE (sender_id, send_id)
);
```

- 利用者の表は、置かない。利用者は、仮実装(WS-P1)であり、フェーズ1で加える。`user_low`、`user_high`、`sender_id` の外部キーも、そのときに加える。
- 履歴の取得は、主キー(会話、連番)の索引を使う。フェーズ1の、分割しての取得(PERF-2)も、この索引を使える。

**問い合わせ(sqlc)**

| ポートの操作 | 問い合わせ |
|---|---|
| `EstablishDirect` | `INSERT INTO conversations (user_low, user_high) VALUES (…) ON CONFLICT (user_low, user_high) DO NOTHING`。続けて、`SELECT id … WHERE user_low = … AND user_high = …` |
| `SerializeAppends` | `SELECT id FROM conversations WHERE id = … FOR UPDATE` |
| `AppendMessage` | `INSERT INTO messages (…) SELECT …, COALESCE(MAX(seq), 0) + 1, …, clock_timestamp() FROM messages WHERE conversation_id = … RETURNING seq, sent_at` |
| `FindDirect` | `SELECT id … WHERE user_low = … AND user_high = …` |
| `ListMessages` | `SELECT … FROM messages WHERE conversation_id = … ORDER BY seq` |
| `FindMessageBySend` | `SELECT … FROM messages WHERE sender_id = … AND send_id = …` |

- 連番は、会話の行のロック(`SerializeAppends`)を得た後に、その会話の最大の連番に1を足して付ける。送信の日時は、同じ文の `clock_timestamp()` である(ADR-010)。
- 2人がほぼ同時に最初のメッセージを送った場合、後の `INSERT … ON CONFLICT DO NOTHING` は、先のトランザクションの確定を待ち、何もしない。続く `SELECT` が、確定した会話を得る(WS-05)。分離の水準は、既定(READ COMMITTED)とする。
- `AppendMessage` は、一意性の制約 `messages_sender_send_key` への違反(SQLSTATE 23505 と、制約の名前)を、`port.ErrDuplicateSend` に置き換える。
- `InTransaction` は、トランザクションを始め、`ConversationWriter` の実装を `fn` に渡す。`fn` がエラーを返すか、パニックした場合は取り消し、そうでなければ確定する。
- `sent_at` は、UTC にして、`domain.Message` に入れる。

**エラーの扱い(WS-33、ADR-004)**

保管先が返したエラーを、そのまま返しません。操作の名前と、SQLSTATE だけを持つエラーに、作り直して返します。元のエラーの文言と詳細には、行の内容(本文)が含まれることがあるためです。これにより、上の層がエラーをログに出力しても、本文は現れません。

#### 4.5.5 fixeduser(固定ユーザー。仮実装 WS-P1)

`port.UserDirectory` を実装します。3人を、コードの中に持ちます。

| `ID` | `Handle` | `DisplayName` |
|---|---|---|
| `00000000-0000-4000-8000-000000000001` | alice | Alice |
| `00000000-0000-4000-8000-000000000002` | bob | Bob |
| `00000000-0000-4000-8000-000000000003` | carol | Carol |

### 4.6 main(`cmd/server`)

**設定(環境変数)**

| 変数 | 内容 |
|---|---|
| `PGHOST`、`PGPORT`、`PGUSER`、`PGPASSWORD`、`PGDATABASE` | 保管先への接続情報。pgx が、そのまま読む。接続文字列には、組み立てない |
| `APP_ORIGIN` | アプリのオリジン(ローカルでは、`http://localhost:5173`) |
| `LISTEN_ADDR` | 待ち受け先。既定は、`127.0.0.1:8080` |

`PGUSER`、`PGPASSWORD`、`PGDATABASE`、`APP_ORIGIN` のいずれかがない場合、サーバーは、既定の値で動かず、起動に失敗します(ADR-007)。

**組み立て**

- 組み立ては、ポートの実装とログの出力先を受け取って、`http.Handler` を返す関数にまとめる。テストは、この関数に、差し替えた実装を渡す(WS-31、WS-33)。
- adapter には、そのパッケージが import してよい層のものだけを渡す(ADR-005)。`httpapi` と `wsaccept` には、ユースケースだけを渡し、ポートの実装を渡さない。

**ログ(WS-33)**

- `log/slog` で、標準出力に出力する。
- 出力するのは、起動と終了、完了できなかった操作(操作の名前と、4.5.4 のエラー)、端末への書き込みの失敗(理由の区分だけ)とする。
- 要求の本文、メッセージの本文、保管先のエラーの文言は、出力しない。要求ごとの記録は、出力しない(出来事の記録は、フェーズ1。仕様「実装しないもの」)。

## 5. クライアント

### 5.1 構成と、import してよい先

| 置き場所 | 置くもの | import してよい先 |
|---|---|---|
| `src/application/` | `chatApp.ts`(状態と操作)、`model.ts`(型)、`messages.ts`(統合の関数) | application の中だけ |
| `src/application/port/` | `chatApi.ts`、`realtimeChannel.ts`、`timer.ts`、`sendIdSource.ts`、`fixedUserDirectory.ts` | application の中だけ |
| `src/adapter/` | ポートの実装。`generated/` に、生成した型 | `application/port` と、そこから参照される application の型 |
| `src/ui/` | React の部品 | application(port を除く) |
| `src/main.tsx` | 組み立てと起動 | すべて |

- 生成した型(`adapter/generated/`)は、adapter だけが import する。
- application は、React と、ブラウザの入出力のAPI(`fetch`、`WebSocket`、`setTimeout`、`crypto`、`window`、`document`、`localStorage` など)を使わない。ESLint の規則(`no-restricted-globals`、`no-restricted-imports`)で検査する。
- 実行時の依存物は、react と react-dom だけとする。HTTP の要求は、`fetch` を直接に使う。

### 5.2 application

`chatApp.ts` は、状態を持ち、変化を通知します。React には、依りません。ui は、`subscribe` と `getState` を、React の `useSyncExternalStore` につなぎます。

**状態**

```ts
type ChatState = {
  users: FixedUser[]                 // 選択肢(仮実装 WS-P1)
  currentUser: FixedUser | null
  conversation: null | {
    peer: FixedUser
    history: 'loading' | 'loaded' | 'failed'
    messages: Message[]              // 完了したメッセージ。連番の順
    outgoing: Outgoing[]             // 送信中と、送信できなかったメッセージ。送信した順
    draft: string                    // 入力欄の内容
    rejection: RejectionCode | null  // 直前の送信が拒否された理由
  }
}
type Message  = { seq: number; sender: { handle: string; displayName: string }; body: string; sentAt: string }
type Outgoing = { sendId: string; body: string; status: 'sending' | 'failed' }
```

**操作**: `selectUser(handle)`、`openConversation(peerHandle)`、`setDraft(text)`、`send()`、`resend(sendId)`。

**規則**

テストは、この規則の番号と対応させます。

| # | 規則 | 基準 |
|---|---|---|
| C-1 | `selectUser` は、それまでの接続を閉じ、会話の状態を捨ててから、新しい利用者として、接続を確立する。選び直す前の接続、履歴の取得、送信に対して、後から届いた結果は、すべて捨てる | WS-41 |
| C-2 | `openConversation` は、会話の状態を作り直す(`history: 'loading'`)。接続が、受信を開始できたことの知らせ(`ready`)を受けた後であれば、すぐに履歴を取得する。接続を確立している途中であれば、`ready` を待ってから取得する。接続が切れている場合は、待たずに取得する | ADR-010、WS-22 |
| C-3 | 履歴、完了の応答、配信のどれで届いたメッセージも、連番をキーとして、`messages` に加える。同じ連番のものが既にあれば、加えない。並びは、連番の順とする | WS-07、WS-22、WS-37 |
| C-4 | 配信は、`participants` が、自分と、開いている会話の相手の組に一致するものだけを、加える。一致しないものは、捨てる。履歴の取得中に届いた配信も、同じ規則で、すぐに加える(履歴が届いたとき、C-3 で統合される) | 仕様「リアルタイムの配信」、WS-22 |
| C-5 | `send` は、`draft` が空の文字列であれば、何もしない。そうでなければ、送信の識別子を作り、`outgoing` に `sending` として加え、`draft` と `rejection` を空にして、送信する。同時に、10秒の時間の経過を待ち始める | UC-09 基本フロー |
| C-6 | 完了の応答、または、同じ送信の識別子を持つ配信の、先に届いた方で、その `outgoing` を除き、メッセージを C-3 で加える。後から届いた方は、C-3 によって、加わらない | WS-37、ADR-011 |
| C-7 | 拒否の応答で、その `outgoing` を除き、`rejection` に理由を入れ、本文を `draft` に戻す。`draft` に別の入力がある場合は、その前に、改行で区切って戻す | WS-40 |
| C-8 | 送信が届かない場合、完了できなかったことを示す応答の場合、10秒が過ぎた場合に、その `outgoing` を `failed` にする。要求は、取り消さない | WS-17 |
| C-9 | `resend` は、`failed` の `outgoing` を、同じ送信の識別子と本文で、`sending` に戻して、送信する。10秒を、改めて待ち始める | WS-17、WS-18 |
| C-10 | `failed` にした後に、完了の応答、配信、拒否の応答が届いた場合も、C-6、C-7 を行う。その `outgoing` が既にない場合は、応答を捨てる。送り直しの前の試行の、失敗と時間切れは、捨てる | ADR-011 |
| C-11 | 別の会話を開いたとき、それまでの会話の状態(`outgoing` と `draft` を含む)を捨てる。前の会話に対して、後から届いた結果は、捨てる | 仕様「メッセージを送る」(送信できなかったメッセージを保つ範囲) |

- C-7 の「前に戻す」は、拒否された本文と、その後の入力の、どちらも失わないための決めである。DD-12 により、拒否は通常の経路になるが、往復の間に、次の入力を始めていることは、まれである。
- 固定ユーザーの選択は、メモリの上にだけ持つ。再読み込みすると、選択からやり直す(DD-11)。保つには、保存のためのポートが要り、仮のログインは、フェーズ1で置き換えるためである。

### 5.3 ポートと adapter

| ポート | 役割 | 実装(adapter) |
|---|---|---|
| `ChatApi` | 履歴の取得、送信 | `httpChatApi.ts`。`fetch` と、生成した型を使う。`X-Fixed-User` を添える(仮実装 WS-P2) |
| `RealtimeChannel` | 接続の確立と終了。`ready`、メッセージ、切断の通知 | `webSocketRealtime.ts`。ブラウザの `WebSocket` を使う。接続先は、表示中のページのオリジンから作る。`user` を添える(仮実装 WS-P2) |
| `Timer` | 時間の経過 | `browserTimer.ts` |
| `SendIdSource` | 送信の識別子を作る | `cryptoSendIdSource.ts`(`crypto.randomUUID`) |
| `FixedUserDirectory` | 固定ユーザーの一覧 | `staticFixedUsers.ts`(仮実装 WS-P1、WS-P3)。サーバーの 4.5.5 と、同じ識別名と表示名を持つ |

`httpChatApi.ts` は、送信の結果を、次の3つに分けて返します。

| 結果 | 場合 |
|---|---|
| 完了 | 200 で、`SendMessageResponse` を得た |
| 拒否 | 4xx で、`ErrorResponse` を得た。`code` を、理由として返す |
| 送信できなかった | 要求が失敗した。5xx を得た。応答が、形式に合わない |

サーバーを停止している間、開発用のサーバー(Vite)の中継は、5xx を返します。これは、「送信できなかった」に当たります(動作検証の手順8)。

履歴の取得は、200 のほかを、すべて「取得できなかった」として返します。

### 5.4 ui(画面の構成と操作の手段。DD-01)

1つの画面に、次を置きます。文言は、日本語とします。

| 部品 | 内容 |
|---|---|
| 利用者の選択(`UserPicker`) | 利用者を選んでいないとき、固定ユーザー3人のボタンを示す |
| ヘッダー | 選んだ利用者の表示名と、「利用者を選び直す」ボタン。押すと、利用者の選択に戻る |
| 相手の選択(`PeerPicker`) | 自分以外の固定ユーザーのボタン。選んでいる相手を、見分けられるようにする |
| 会話(`ConversationView`) | メッセージを、連番の順に示す。その後ろに、送信中と、送信できなかったメッセージを示す。メッセージがなければ、「メッセージはありません」と示す。取得中と、取得できなかったことを示す |
| メッセージ(`MessageItem`) | 送信者の表示名、送信の日時(端末の地域の時刻)、本文。送信中は「送信中」、送信できなかったものは「送信できませんでした」と「送り直す」ボタンを示す |
| 入力欄(`Composer`) | 複数行の入力欄と、「送信」ボタン。拒否の理由を、入力欄の近くに示す |

- 本文は、React の文字列として描画する。`dangerouslySetInnerHTML` は、使わない(ESLint の規則で禁止する)。リンクへの変換など、本文の解釈は、しない(WS-27)。改行とタブは、CSS の `white-space: pre-wrap` で、そのまま示す(WS-11)。
- 入力欄は、Enter で送信し、Shift+Enter で改行する。かな漢字変換の確定のための Enter では、送信しない。
- 拒否の理由は、`code` ごとの文言で示す(`invalid_body`: 本文が正しくないこと。空でないこと、4,000文字以下であること、制御文字を含まないことを添える。`invalid_input`、`invalid_peer`、`not_found`、`unauthenticated`: それぞれの文言。そのほか: 拒否されたこと)。
- 新しいメッセージが加わったとき、会話の表示を、末尾まで送る。
- スタイルは、CSS のファイル1つとする。外部から、スタイルとフォントを読み込まない(ADR-001)。
- テストと E2Eテストが部品を特定できるように、ボタンと入力欄に、読み上げられる名前を付ける。

## 6. ローカルの開発環境

### 6.1 開発者の端末に要るもの

Go、Node.js(24 系。`client/.nvmrc` に記録する)、Docker(Docker Compose を含む)、make、C のコンパイラ(sqlc のビルドに要る)。

Docker は、Windows の側の Docker Desktop を、WSL との連携によって使います(ADR-007)。連携は、2026-11-21 に有効にしました(Docker Engine 29.9.3、Docker Compose v5.7.0)。WSL2 の中に、Docker Engine は導入しません。`docker` を使う手順(`make db-up`、`make test-server`、`make e2e` など)の前に、Windows の側で、Docker Desktop を起動しておきます。

PostgreSQL のクライアント(`psql`)は、端末に導入しません。保管先のコンテナの中のものを、`docker compose exec` で使います(7.1、7.4)。

### 6.2 保管先と秘密

**保管先の定義(`compose.yaml`)**

- プロジェクトの名前を、定義の中で定める(`name: ca-chat-app`)。コンテナとボリュームの名前が、リポジトリを置いたディレクトリの名前に依らなくなる。
- イメージは、Docker の公式イメージ `postgres` を、マイナー版まで指定して固定する(着手の時点の 18 系の最新。2026-11-21 の時点では、`postgres:18.7`)。
- データは、名前を付けたボリュームに置く。マウントする先は、`/var/lib/postgresql` とする。18 のイメージは、データを置く場所が、17 までの `/var/lib/postgresql/data` から変わっている。
- 公開するポートは、`127.0.0.1:${PGPORT:?}:5432` に限る。`PGPORT` の見本の値は、5432 とする。
- 起動の確認(healthcheck)を定める。`pg_isready -h 127.0.0.1` で、TCP での接続を確かめる。初回の起動では、イメージが、初期化のために、TCP で待ち受けないサーバーを一時的に動かす。ソケットで確かめると、その間に、起動したと判定してしまう。
- ログの設定(ADR-004)は、起動の引数で与える。`log_error_verbosity=terse`、`log_statement=none`、`log_min_duration_statement=-1`、`log_parameter_max_length_on_error=0`。

**秘密**

- 秘密は、リポジトリの直下の `.env`(追跡しない)で与える。Docker Compose は、`compose.yaml` と同じ場所の `.env` を、自動で読む。Makefile は、同じファイルを読んで、サーバーと道具に渡す。
- `.env.example` には、`PGHOST`、`PGPORT`、`PGUSER`、`PGPASSWORD`、`PGDATABASE`、`APP_ORIGIN` の名前と、秘密でない値だけを書く。`PGHOST` は、`127.0.0.1` とする。`PGPASSWORD` は、空にしておく。
- `compose.yaml` は、`${PGUSER:?}`、`${PGPASSWORD:?}`、`${PGDATABASE:?}` の形で参照し、イメージの `POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB` に渡す。与えられていなければ、起動に失敗する。
- イメージが、利用者とパスワードを設定するのは、ボリュームが空のとき(初回の起動)だけである。`.env` の値を後から変えても、保管先には反映されない。変える場合は、ボリュームを削除して作り直す(`docker compose down -v`。開発用のデータは、失われる)。手順書に記載する。
- `.gitignore` に、`.env`、`bin/`、`node_modules/`、ビルドとテストの出力を挙げる。

**確かめたこと(2026-11-21)**

上の定義に当たる、使い捨ての定義で、次を確かめました。

- 秘密を与えない場合、定義の読み込みが、失敗する。
- `docker compose up -d --wait` は、起動の確認が通ってから戻り、直後に接続できる(初回の起動で、約5秒)。
- 公開したポートは、WSL2 の中の 127.0.0.1 と、Windows の側の 127.0.0.1 から届く。WSL2 の、他のアドレスからは、届かない。
- `docker compose down` と起動を挟んでも、データが残る。
- ログの設定が効き、制約への違反の記録に、詳細(DETAIL)が現れない。
- コンテナの中の `psql` で、雛形からのデータベースの複製(`CREATE DATABASE … TEMPLATE`)ができる。

### 6.3 道具の版の固定(WS-32)

| 対象 | 固定の手段 |
|---|---|
| Go と、サーバーの依存物 | `server/go.mod`(`go` と `toolchain` の行)、`server/go.sum` |
| クライアントの依存物と、Node.js の道具(Vite、Vitest、Playwright、ESLint、dependency-cruiser、openapi-typescript、TypeScript 5.x) | `client/package.json`(版を、範囲ではなく、1つに指定する)、`client/package-lock.json`。導入は、`npm ci` で行う |
| Go の道具(oapi-codegen、sqlc、goose、golangci-lint、gitleaks) | Makefile に、版を変数として記録する。`go install <パッケージ>@<版>` で、`bin/` に導入する(DD-19) |
| PostgreSQL | `compose.yaml` のイメージのタグ |

版は、着手の時点の最新の安定版を、導入の時点で確かめて決めます。導入する版のライセンスも、その時点で確かめます(ADR-001)。

### 6.4 Makefile の手順

| 手順 | 内容 |
|---|---|
| `make setup` | Go の道具を `bin/` に導入する。`npm ci`。Playwright のブラウザ(Chromium)を取得する |
| `make db-up` / `make db-down` | 保管先を起動し、接続を受け付けるまで待つ(`docker compose up -d --wait`) / 保管先を停止し、コンテナを削除する(`docker compose down`)。データ(ボリューム)は、残す |
| `make db-migrate` | 開発用のデータベースに、マイグレーションを適用する |
| `make dev-server` / `make dev-client` | サーバー / クライアントの開発用のサーバーを起動する |
| `make generate` | oapi-codegen、sqlc、openapi-typescript を実行する |
| `make generate-check` | 生成し直した結果が、リポジトリの内容と一致することを確かめる |
| `make lint` | golangci-lint、ESLint、TypeScript の型の検査 |
| `make check-deps` | 依存規則と飛ばし禁止の検査(depguard、dependency-cruiser、application に対する ESLint の規則)。違反を検出したとき、日時と出力を、`docs/dependency-check-log.md` に追記して、失敗する |
| `make check-deps-selftest` | 規則に違反する見本に対して、検査が失敗することを確かめる。検査の記録には、追記しない |
| `make test-server` | テスト用の雛形のデータベースを作り直してから、`go test ./...` を実行する |
| `make test-client` | Vitest を実行する |
| `make e2e` | E2Eテストを実行する(7.4) |
| `make secrets-scan` | gitleaks を、リポジトリの履歴の全体に対して実行する(WS-34) |
| `make check` | `generate-check`、`lint`、`check-deps`、`check-deps-selftest`、`test-server`、`test-client`、`secrets-scan` |

**規則に違反する見本(ADR-005「検査の手段」)**

- サーバーは、見本を、対象のパッケージの中に、ビルドタグ(`depcheck_sample`)を付けたファイルとして置く。通常のビルドと検査には、含まれない。`check-deps-selftest` は、このタグを付けて depguard を実行し、すべての見本が検出されることを確かめる。
- 見本は、少なくとも次を含める。`httpapi` → `application/port`、`httpapi` → `domain`、`httpapi` → `adapter/postgres`、`wsaccept` → `application/port`、`wsdelivery` → `application/usecase`、`application/usecase` → adapter、`domain` → `application/port`、`domain` → `net/http`。
- クライアントは、見本を、各層の `__depcheck__/` に置く。通常の検査とビルドからは、除く。見本は、`ui` → `adapter`、`adapter` → `ui`、`ui` → `application/port`、`application` → `adapter`、`application` → `react`、`application` での `fetch` の使用を含める。

### 6.5 AIへの常設の指示(`CLAUDE.md`。DD-09)

リポジトリの直下の `CLAUDE.md` に、次を書きます。

- 観察記録の規則(ADR-008「都度の記録」の、記録する出来事と項目)。当たることが起きた時点で、`docs/observations.md` に追記すること。
- 層と、import してよい先の規則の所在(アーキテクチャ概要と ADR-005)。検査が失敗したとき、規則の側を緩めて通さないこと。
- 規約として守ること(import の検査では、検出できないもの)。ポートを実装する adapter から、domain の規則を呼ばない。トランザクションの関数の中で、渡された操作のほかに、ポートを呼ばない。main は、adapter に、import してよい層のものだけを渡す。
- メッセージの本文と、保管先のエラーの文言を、ログに出力しないこと。
- 仮実装の箇所に、`仮実装(WS-Pn)` のコメントを付け、一覧を更新すること。
- 変更の後に、`make check` を実行すること。

### 6.6 仮実装の箇所の一覧(DD-02)

`docs/temporary-implementations.md` に、仕様の WS-P1〜WS-P4 ごとに、該当するファイルと、正式実装との違いを書きます。コードの該当する箇所には、`仮実装(WS-Pn)` のコメントを付けます。一覧とコメントが対応していることは、コメントを検索して確かめます。

実装の前の見込みは、次の通りです。

| ID | 箇所 |
|---|---|
| WS-P1 | `server/internal/adapter/fixeduser/`。`client/src/adapter/staticFixedUsers.ts`。保管先に、利用者の表と外部キーがないこと |
| WS-P2 | `httpapi` と `wsaccept` の、利用者を要求から取り出す箇所。`api/openapi.yaml` の `securitySchemes`。`httpChatApi.ts` と `webSocketRealtime.ts` の、利用者を添える箇所。ui の `UserPicker` |
| WS-P3 | ui の `PeerPicker`。`staticFixedUsers.ts` |
| WS-P4 | `ConversationReader.ListMessages` と、その問い合わせ。`listDirectMessages` の応答 |

## 7. テスト

### 7.1 テストの種類と置き場所

| 種類 | 置き場所 | 保管先 |
|---|---|---|
| domain のテスト | `server/internal/domain/` | 使わない |
| usecase のテスト | `server/internal/application/usecase/`。ポートは、手で書いたテスト用の実装に差し替える | 使わない |
| adapter のテスト | 各 adapter のパッケージ | `postgres` だけが使う |
| サーバーの振る舞いのテスト | `server/cmd/server/`(DD-18) | 使う |
| クライアントの application のテスト | `client/src/application/`。ポートは、テスト用の実装に差し替える。時間は、テストから進める | — |
| クライアントの adapter と ui のテスト | `client/src/adapter/`、`client/src/ui/`。jsdom の上で実行する | — |
| E2Eテスト | `client/e2e/` | 使う |

**テスト用のデータベース(DD-20)**

- `make test-server` は、雛形のデータベース(`chat_test_template`)を作り直し、goose でマイグレーションを適用する。作り直し(`DROP DATABASE` と `CREATE DATABASE`)は、保管先のコンテナの中の `psql` を、`docker compose exec` で実行する。goose と、テストは、端末から、127.0.0.1 に公開したポートへ接続する。
- 保管先を使うテストは、テストごとに、雛形を複製したデータベースを作る(`CREATE DATABASE … TEMPLATE chat_test_template`)。終わったら、削除する。
- 雛形がない場合、テストは、`make test-server` を使うよう示して、失敗する。

**サーバーの振る舞いのテストの道具立て**

- テストは、4.6 の組み立ての関数で、実際の adapter を使うサーバーを組み立て、`httptest.Server` で起動する。
- 利用者ごとの、要求の補助(送信、履歴の取得)と、WebSocket の接続の補助(次のイベントを待って返す)を用意する。接続の補助は、coder/websocket のクライアントを使う。
- 次のイベントを待つ時間には、上限(5秒)を設ける。上限は、失敗を検出するためのものであり、成功する場合の所要時間には、影響しない。

### 7.2 「配信されない」ことの判定(DD-08)

届かないことを、待ち時間で判定しません。次の性質を使います。

- `Deliver` は、送信待ちの列に加えるまでを、呼び出しの中で終える(4.3)。送信の完了の応答が返った時点で、そのメッセージの配信は、宛先のすべての列に、加え終わっている。
- 列は、加えた順に届く。

そこで、「端末Xに、メッセージMが届かない」ことは、次の手順で確かめます。

1. Mの送信(または、何も届かないはずの操作)を行い、応答を得る。
2. 端末Xに届くはずの、別のメッセージS(番兵)を送り、応答を得る。
3. 端末Xが次に受け取るイベントが、Sであることを確かめる。Mが届いていれば、Sより先に現れる。

例: WS-25 では、alice が bob に送った後、alice が carol に番兵を送り、carol の端末が次に受け取るのが、番兵であることを確かめます。

### 7.3 受け入れ基準との対応

「振る舞い」は、サーバーの振る舞いのテストを指します。

| 基準 | 確かめ方 |
|---|---|
| WS-01 | 振る舞い: 送信の応答が、連番、送信者、送信の日時を持つ。ui: 完了したメッセージが、送信者と日時とともに示される。E2E |
| WS-02、WS-03 | 振る舞い: 要求をしていない接続(bob の端末、alice の別の端末)が、イベントを受け取る。E2E |
| WS-04 | 振る舞い: 接続のない bob が、後から履歴で得る |
| WS-05 | 振る舞い: 最初の送信で、会話の行ができる。alice と bob が、同時に最初の送信をしても、会話の行は1つで、連番は1と2になる(保管先を、直接に調べる) |
| WS-06 | 振る舞い: 履歴の取得の後、会話の行がない。bob の端末に届かない(番兵) |
| WS-07 | 振る舞い: 2人が、並行して、複数通を送る。2人の履歴が一致し、連番に欠番と重複がない。配信で受け取った連番の集合が、履歴と一致する。application: 届く順を変えても、表示が連番の順になる(C-3) |
| WS-08 | 動作検証 |
| WS-09〜WS-11 | domain: 規則の境界の値(下記)。振る舞い: 代表の値を、HTTP を通して送る。対にならないサロゲートのエスケープ、正しくない UTF-8 のバイト、CRLF と CR(応答の本文が、LF になっている)、結合した絵文字(そのまま返る)。ui: 改行が、改行として示される |
| WS-12 | 振る舞い: 拒否のそれぞれについて、メッセージと会話の行が増えず、配信されない(番兵) |
| WS-13 | domain と振る舞い: 長さ、ハイフンの位置、16進数でない文字。大文字は、受け付ける |
| WS-14、WS-15、WS-38 | 振る舞い: `invalid_peer`、`not_found`。送信と履歴の `not_found` の応答が、状態と本文で一致する |
| WS-16 | 振る舞い: 利用者を添えない要求と、固定ユーザーでない識別名の要求が、送信、履歴、接続の確立のそれぞれで、401 になる。接続は、確立されない |
| WS-17 | application: 10秒を進めると、`failed` になる。本文が残り、`resend` が、同じ送信の識別子で送る(C-8、C-9)。届かない場合は、動作検証 |
| WS-18 | 振る舞い: 同じ送信の識別子の送信が、同じメッセージ(同じ連番)で完了する。メッセージの行は1つ。再び配信されない(番兵)。同じ識別子を、並行して複数回送っても、行は1つで、すべて、同じ連番で完了する。usecase: 重複した送信の分岐 |
| WS-19 | 振る舞い: bob が、alice と同じ送信の識別子で送ると、別のメッセージになる |
| WS-39 | 振る舞い: 同じ識別子で、本文が正しくない送信は `invalid_body`、別の相手への送信は `invalid_input`。保管済みのメッセージは変わらず、別の相手との会話は成立しない。usecase: 同じ分岐 |
| WS-20 | 振る舞い: 履歴が、連番の順で、送信者の表示名と日時を持つ。ui: 表示 |
| WS-21 | 振る舞い: 空の一覧。ui: 「メッセージはありません」 |
| WS-22 | application: 履歴の取得中に配信が届く場合、履歴と配信の両方に同じメッセージがある場合、届く順を入れ替えた場合に、欠けず、重複しない(C-2〜C-4)。`ready` の前に、履歴を取得しない |
| WS-23、WS-24 | 振る舞い: alice と bob がやり取りした後、carol が行える操作(alice、bob のそれぞれを相手とする履歴の取得と送信)のどれでも、alice と bob の会話のメッセージを得られず、その会話に、メッセージが増えない |
| WS-25 | 振る舞い: carol の端末に届かない(番兵) |
| WS-26 | WS-23〜WS-25 の振る舞いのテストは、すべて、サーバーへ直接送った要求と接続で行う |
| WS-27 | ui: `<script>`、イベントの属性を持つタグ、HTML の実体参照を含む本文が、文字として示され、要素が作られない。E2E |
| WS-28 | 振る舞い: 引用符、`;`、`--`、`%`、`$1`、`\` を含む本文が、そのまま返る |
| WS-29 | 振る舞い: 他のオリジンを Origin に持つ、送信、履歴の取得、接続の確立が、403 になり、保管されない。CORS のヘッダーが、どの応答にもない。`OPTIONS` に、許可を返さない。Origin も利用者もない GET が、401 になる |
| WS-30 | 振る舞い: 壊れた JSON、型の違う項目、大きすぎる要求、定義されていないパスと方法、保管の失敗に対する応答が、`code` だけを持ち、決めた文字列(パッケージの名前、SQL、ファイルのパス、`goroutine` など)を含まない。応答のヘッダーに、ソフトウェアの名前と版がない |
| WS-31 | 振る舞い: トランザクションのポートを、確定の直前に失敗する実装で包んで、組み立てる。応答は `internal`。履歴に現れず、配信されない(番兵)。usecase: 保管が失敗したとき、配信のポートを呼ばない |
| WS-32 | 検査: 6.3 の表の通りに、固定されていること。`go mod verify` と `npm ci` が成功すること |
| WS-33 | 振る舞い: ログの出力先を差し替え、目印の文字列を本文に含む送信を、完了、拒否、エラーのそれぞれで行う。出力に、目印が現れない。`postgres` のテスト: 制約への違反を起こし、返るエラーの文言に、行の内容が含まれない |
| WS-34 | `make secrets-scan` |
| WS-35 | `make check-deps`。状態が、`wsdelivery` だけにあることを、設計の確認として、アーキテクチャ概要に記載する |
| WS-36 | ADRの確認: ADR-001、ADR-002、ADR-004 の「選定の条件の確認」に、複数サーバー構成への移行の確認があること |
| WS-37 | 振る舞い: 送信した利用者の接続が、送信の識別子を持つイベントを受け取る。完了の応答、履歴、配信の `Message` が、一致する(ADR-005 の、変換が2箇所にあることの検出)。application: 応答が先の場合と、配信が先の場合のどちらでも、1通だけになる(C-6) |
| WS-40 | application: C-7。ui: 理由が示され、本文が入力欄に残り、会話に示されない。動作検証 |
| WS-41 | application: C-1。振る舞い: 閉じた接続が、登録から除かれる。E2E |

domain の本文のテストは、少なくとも次の値を含めます。

- 長さ: 4,000 と 4,001。基本多言語面の外の文字(1文字が、UTF-16 で2つ、UTF-8 で4バイトになる)だけの本文で、4,000 と 4,001。
- 空: 空の文字列。U+0020、U+3000、U+00A0、U+2028、改行、タブだけの本文。
- 空でない: U+200B だけの本文。U+FEFF だけの本文(どちらも、White_Space ではない)。
- 制御文字: U+0000、U+001F、U+007F、U+0085、U+009F を含む本文(拒否)。U+000A と U+0009 を含む本文(受け付ける)。
- 改行: CRLF と、単独の CR が、LF になる。正規化の後の長さで、判定される(CRLF を含む、正規化の前は 4,001 で、後は 4,000 の本文)。
- 文字列として正しくない並び。
- 複数のコードポイントを結合した絵文字(U+200D を含む)が、そのまま保たれる。

### 7.4 E2Eテスト

- 対象は、動作検証の手順1〜5と、手順9とする(ADR-006)。ブラウザは、Chromium だけとする。
- 1つのテストの中で、互いに状態を共有しない、複数のブラウザのコンテキストを開く。
- `make e2e` は、E2E用のデータベース(`chat_e2e`)を作り直して、マイグレーションを適用する(手段は、7.1 の雛形と同じ)。Playwright が、このデータベースにつないだサーバー(127.0.0.1:18080)と、クライアントの開発用のサーバー(127.0.0.1:15173)を起動する。`APP_ORIGIN` は、`http://localhost:15173` とする。
- テストは、直列に実行する。テストごとに、始める前に、`messages` と `conversations` を TRUNCATE する(DD-07)。保管先のコンテナの中の `psql` を、`docker compose exec` で実行する。サーバーは、メッセージと会話を、メモリに保持しないため、再起動は要らない。
- クライアントの開発用のサーバーの、待ち受け先と中継先は、環境変数で変えられるようにする。

## 8. 動作検証の具体化(DD-03)

仕様「動作検証」の手順を、`http://localhost:5173` に対して行います。3つのブラウザは、別のブラウザ、または、別のプロファイルとします。

**手順8(サーバーを停止した状態での送信)**

1. `make dev-server` を停止する。保管先と、クライアントの開発用のサーバーは、止めない。
2. ブラウザ1から送信する。「送信できませんでした」と、本文が示されることを確かめる。
3. `make dev-server` を起動する。
4. ブラウザ1を開き直さずに、「送り直す」を押す。メッセージが、完了したものとして示されることを確かめる。
5. ブラウザ1を再読み込みし、alice として、bob との会話を開く。履歴に、そのメッセージが1通だけあることを確かめる。

サーバーを停止すると、各ブラウザの WebSocket の接続は切れ、再接続しません(ADR-002)。手順4の送信は完了しますが、他のブラウザは、再読み込みするまで、配信を受け取りません。

**手順10(サーバーが拒否する本文)**

クライアントは、送信の前に本文を検証しないため(DD-12)、画面から、そのまま行えます。

1. 4,001文字の本文を、入力欄に貼り付けて、送信する。本文は、ブラウザの開発者向けの機能で、`'a'.repeat(4001)` を実行して作る。
2. 拒否されたことと理由が示され、本文が入力欄に残り、会話に示されないことを確かめる。
3. 1文字を削って、送信する。完了することを確かめる。

**そのほか**

- 手順7の「サーバーと保管先を再起動する」は、`make dev-server` の停止と起動、`make db-down` と `make db-up` で行う。`make db-down` は、コンテナを削除するが、ボリュームは残す(6.4)。
- 開発者の端末は、WSL2 である。ブラウザを Windows の側で動かす場合に、`localhost` で到達できることは、実装の最初に確かめる(ADR-007。9 の手順0)。

## 9. 実装の順序

各手順は、`make check` のうち、その時点で実行できるものが通る状態で終えます。観察記録に当たる出来事は、その都度、`docs/observations.md` に追記します(ADR-008)。

| # | 手順 | 終えたときの状態 |
|---|---|---|
| 0 | 環境の確認。Docker の、WSL との連携は、有効にし、保管先のコンテナに届くことを確かめた(6.1、6.2)。残りは、次の確認である。何も実装していない Go のサーバーと Vite を、127.0.0.1 で起動し、Windows の側のブラウザから、`localhost` で、HTTP と WebSocket の両方が、中継を通って届くことを確かめる | 成り立たない場合は、ADR-007 を改める |
| 1 | 土台。`CLAUDE.md`、`.gitignore`、`.env.example`、`compose.yaml`、Makefile、道具の導入。`server/` と `client/` の骨組み。検査の設定(depguard、dependency-cruiser、ESLint)と、規則に違反する見本。gitleaks | `make setup`、`make db-up`、`make lint`、`make check-deps`、`make check-deps-selftest`、`make secrets-scan` が通る |
| 2 | 通信の形式。`api/openapi.yaml` と、生成 | `make generate-check` が通る |
| 3 | domain と usecase(送信、履歴)。テスト用のポートの実装 | domain と usecase のテストが通る |
| 4 | 保管先。マイグレーション、問い合わせ、`postgres` の adapter | `postgres` のテストが通る |
| 5 | `fixeduser`、`httpapi`、main。HTTP だけの、振る舞いのテスト(送信、履歴、検証と拒否、送り直し、Origin、エラー応答、ログ) | 配信に関わらない基準の、振る舞いのテストが通る |
| 6 | リアルタイム。`ConnectionKeeper`、`wsaccept`、`wsdelivery`。配信に関わる振る舞いのテスト | サーバーの自動テストが、すべて通る |
| 7 | クライアントの application と、そのテスト | C-1〜C-11 のテストが通る |
| 8 | クライアントの adapter、ui、main | クライアントの自動テストが通る。ブラウザで、やり取りできる |
| 9 | E2Eテスト | `make e2e` が通る |
| 10 | 文書と動作検証(10.1) | 完了条件を満たす |

## 10. 完了時に行うことと、未確定の事項

### 10.1 完了時に行うこと

仕様「マイルストーンの完了条件との対応」に対応します。

- アーキテクチャ概要(`docs/architecture.md`)を作成する。層の構成、ディレクトリ、パッケージと規則の一覧(4.1、5.1)、実行環境の構成を、現状の通りに記載する。
- 仮実装の箇所の一覧(`docs/temporary-implementations.md`)を、コードのコメントと一致させる。
- 手順書(`README.md`)に、起動、コード生成、検査、テスト、動作検証の手順を記載する。
- 動作検証(仕様の手順1〜10)を行い、結果を記録する。
- 観察記録に、スライスの記録を追記する(ADR-008)。検査の記録(`docs/dependency-check-log.md`)と、都度の記録を突き合わせる。
- 実装を終えたADR(ADR-001〜013)を、frozen にする。
- 本書を削除する。

### 10.2 未確定の事項

| 事項 | 決める時点 |
|---|---|
| Go のモジュールのパス(公開先のリポジトリのパス) | 手順1の前に、開発者に確認する |
| 道具と依存物の版(6.3) | 手順1で、導入の時点の最新の安定版を確かめて決める。TypeScript は 5.x 系(DD-21) |
| WSL2 で、Windows の側のブラウザから `localhost` で到達できるか | 手順0 |
