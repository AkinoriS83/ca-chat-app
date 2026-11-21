# 仮実装の箇所の一覧

[ウォーキングスケルトンの仕様](specs/walking-skeleton.md)「仮実装」の ID(WS-P1〜WS-P4)ごとに、実装の中で仮実装となっている箇所と、正式実装との違いを挙げます。MVPの完了までに、すべて正式実装に置き換えます(企画書「マイルストーンごとの完了条件」)。

- コードの該当する箇所には、`仮実装(WS-Pn)` のコメントを付けています。
- 本書とコメントが対応していることは、`grep -rn '仮実装(WS-P' api server client/src` で確かめます(生成したコードと、テストを除く)。
- 置き換えたときは、本書から行を除き、[観察記録](observations.md)に、変更が及んだ層とファイルの数を記録します([ADR-008](adr/ADR-008-observation-records.md))。

## WS-P1 固定ユーザー

代替している対象: UC-01、UC-05、UC-06。利用者は、あらかじめ定めた3人(alice、bob、carol)に限ります。登録、プロフィールの編集、連絡先を持ちません。

| 箇所 | 内容 | 正式実装で行うこと |
|---|---|---|
| `server/internal/adapter/fixeduser/directory.go` | `port.UserDirectory` の実装。3人を、コードの中に持つ | 保管先を使う実装に置き換える |
| `server/cmd/server/main.go` | 組み立てで、`fixeduser.NewDirectory()` を渡している | 置き換えた実装を渡す |
| `server/internal/application/port/port.go` の `UserDirectory` | ポートの実装が、固定ユーザーである旨のコメント | コメントを除く。ポートは、変えない見込みである |
| `server/migrations/00001_create_conversations_and_messages.sql` | 利用者の表がない。`conversations.user_low`、`user_high` と、`messages.sender_id` に、外部キーがない | 利用者の表と、外部キーを加える |
| `client/src/adapter/staticFixedUsers.ts` | `FixedUserDirectory` の実装。サーバーと同じ3人を、コードの中に持つ | 取り除く(WS-P2、WS-P3 の置き換えとともに) |
| `client/src/application/port/fixedUserDirectory.ts`、`client/src/application/model.ts` の `FixedUser`、`ChatState.users` | 固定ユーザーの一覧を、利用者と相手の選択肢として持つ | ログイン中の利用者と、相手の指定の形に改める |
| `client/src/main.tsx` | 組み立てで、`StaticFixedUsers` を渡している | 置き換えた実装を渡す |

## WS-P2 仮のログイン

代替している対象: UC-02、UC-03、UC-04、UC-07、R-1。固定ユーザーから、自分として振る舞う1人を選ぶだけで、ログインした状態になります。認証に用いる秘密、有効期限、ログアウトを持ちません。**なりすましを防ぎません**。添え方は、[ADR-013](adr/ADR-013-temporary-login-identity.md) の通りです。

| 箇所 | 内容 | 正式実装で行うこと |
|---|---|---|
| `api/openapi.yaml` の `securitySchemes.fixedUser` と、文書の説明 | 利用者を、要求ヘッダー `X-Fixed-User` と、クエリ `user` で添える | 認証の方式(フェーズ1のADR)に置き換える |
| `server/internal/adapter/httpapi/handler.go` の `fixedUserHeader`、`actorOf` | 要求ヘッダーの識別名を、そのまま、操作を行う利用者として渡す | ログイン状態の資格から、利用者を特定する |
| `server/internal/adapter/wsaccept/handler.go` の `fixedUserQuery`、`ServeHTTP` | クエリの識別名を、そのまま、操作を行う利用者として渡す | 同上。資格を、接続先のアドレスに載せない(SEC-1) |
| `server/cmd/server/config.go` の `validateLoopback` | 待ち受け先を、ループバックのアドレスに限る。仮のログインが、ローカルの開発環境の外から到達できないようにするためである(WS-D6) | 仮のログインとともに取り除く。待ち受け先は、設置環境の構成(OPS-1)で定める |
| `client/src/adapter/httpChatApi.ts` の `fixedUserHeader` | 選んだ固定ユーザーの識別名を、要求ヘッダーで添える | 認証の方式に従う |
| `client/src/adapter/webSocketRealtime.ts` の `fixedUserQuery`、`realtimeUrl` | 選んだ固定ユーザーの識別名を、クエリで添える | 同上 |
| `client/src/ui/UserPicker.tsx`(と、`Header.tsx` の「利用者を選び直す」) | 利用者を、一覧から選ぶ。選択は、メモリの上にだけ持ち、再読み込みすると、選択からやり直す | ログインと、ログアウトの画面に置き換える |

置き換えは、ユースケースを呼ぶ adapter と、利用者を確かめるポートの実装に閉じる見込みです(ADR-013)。ユースケースの入力の「操作を行う利用者」(`Actor`)は、変わりません。

## WS-P3 相手の選択

代替している対象: UC-08。相手を、識別名の入力ではなく、固定ユーザーの一覧から選びます。全利用者を列挙できる一覧を提供しないこと(AC-7)を、満たしません。

| 箇所 | 内容 | 正式実装で行うこと |
|---|---|---|
| `client/src/ui/PeerPicker.tsx` | 相手を、自分以外の固定ユーザーのボタンから選ぶ | 識別名の入力による指定(UC-08)と、会話の一覧(UC-10)に置き換える |
| `client/src/adapter/staticFixedUsers.ts`、`client/src/application/port/fixedUserDirectory.ts`、`client/src/application/model.ts` の `ChatState.users` | 相手の選択肢として、固定ユーザーの一覧を持つ(WS-P1 と同じ箇所) | 取り除く |

サーバーの操作は、相手を、識別名で指定する形(`/api/direct-conversations/{peer}/messages`)です。UC-08 E1、E2 に当たる拒否(`not_found`、`invalid_peer`)は、サーバーへ直接送られた要求に対して、既に適用しています。

## WS-P4 履歴の一括取得

代替している対象: UC-11 基本フロー 2〜3、R-5。会話のすべてのメッセージを、一度に取得します(仕様 WS-D4)。

| 箇所 | 内容 | 正式実装で行うこと |
|---|---|---|
| `api/openapi.yaml` の `listDirectMessages` | 応答(`MessageList`)が、会話のすべてのメッセージを含む | 位置と件数を指定する形にする(PERF-2) |
| `server/internal/application/port/port.go` の `ConversationReader.ListMessages` | 会話の識別子だけを受け、すべてのメッセージを返す | 位置と件数を受ける形にする |
| `server/internal/adapter/postgres/store.go` の `ListMessages`、`server/internal/adapter/postgres/queries/messages.sql` の `ListMessages` | 件数を限らずに、問い合わせる | 主キー(会話、連番)の索引を使って、指定した位置から、一定の件数だけを取得する |
| `server/internal/adapter/httpapi/handler.go` の `ListDirectMessages` | すべてのメッセージを、1つの応答で返す | 分割して返す |

クライアントは、履歴を、1回の取得で読み込む前提です(`client/src/application/chatApp.ts` の `fetchHistory`)。分割しての取得に合わせて、改めます。
