# 仮実装の箇所の一覧

[ウォーキングスケルトンの仕様](specs/walking-skeleton.md)「仮実装」の ID(WS-P1〜WS-P4)ごとに、実装の中で仮実装となっている箇所と、正式実装との違いを挙げます。MVPの完了までに、すべて正式実装に置き換えます(企画書「マイルストーンごとの完了条件」)。

- コードの該当する箇所には、`仮実装(WS-Pn)` のコメントを付けています。
- 本書とコメントが対応していることは、`grep -rn '仮実装(WS-P' api server client/src` で確かめます(生成したコードと、テストを除く)。
- 置き換えたときは、本書から行を除き、[観察記録](observations.md)に、変更が及んだ層とファイルの数を記録します([ADR-008](adr/ADR-008-observation-records.md))。

## WS-P1 固定ユーザー、WS-P2 仮のログイン、WS-P3 相手の選択

「登録とログイン」のスライスで、正式実装に置き換えました([仕様](specs/registration-and-login.md)の RL-42)。箇所は、コードと本書から、除いています。置き換えで、変更が及んだ層とファイルの数は、観察記録の OBS-029、OBS-030 にあります。

待ち受け先を、ループバックのアドレスに限る検査(`server/cmd/server/config.go` の `validateLoopback`)は、コードに残しています。仮のログインのためではなく、通信経路を暗号化しないため(SEC-2)のものです(仕様 RL-D1)。設置環境を扱うスライスで、解きます。

## WS-P4 履歴の一括取得

代替している対象: UC-11 基本フロー 2〜3、R-5。会話のすべてのメッセージを、一度に取得します(仕様 WS-D4)。

| 箇所 | 内容 | 正式実装で行うこと |
|---|---|---|
| `api/openapi.yaml` の `listDirectMessages` | 応答(`MessageList`)が、会話のすべてのメッセージを含む | 位置と件数を指定する形にする(PERF-2) |
| `server/internal/application/usecase/read_history.go` の `HistoryReader.Read`(入力と出力は、`io.go` の `ReadHistoryInput`、`ReadHistoryOutput`) | 相手だけを受け、会話のすべてのメッセージを返す | 入力に、位置と件数を加える。出力を、分割して返す形にする |
| `server/internal/application/port/port.go` の `ConversationReader.ListMessages` | 会話の識別子だけを受け、すべてのメッセージを返す | 位置と件数を受ける形にする |
| `server/internal/adapter/postgres/store.go` の `ListMessages`、`server/internal/adapter/postgres/queries/messages.sql` の `ListMessages` | 件数を限らずに、問い合わせる | 主キー(会話、連番)の索引を使って、指定した位置から、一定の件数だけを取得する |
| `server/internal/adapter/httpapi/handler.go` の `ListDirectMessages` | すべてのメッセージを、1つの応答で返す | 分割して返す |
| `client/src/application/port/chatApi.ts` の `ChatApi.listMessages`、`client/src/adapter/httpChatApi.ts` の `listMessages` | 相手だけを指定して、会話のすべてのメッセージを、1回の要求で取得する | 位置と件数を指定する形にする |
| `client/src/application/chatApp.ts` の `fetchHistory` | 履歴を、1回の取得で読み込み、それを会話の全体として扱う | 分割しての取得に合わせて、読み込みと、配信との統合を改める |
