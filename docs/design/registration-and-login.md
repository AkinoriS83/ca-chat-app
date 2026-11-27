# 「登録とログイン」詳細設計

本書は、[開発方針](../development-policy.md)「設計フェーズの進め方」が定める、使い捨ての詳細設計です。[「登録とログイン」の仕様](../specs/registration-and-login.md)と、ADR-014〜021([docs/adr/](../adr/))を、実装できる形に具体化します。

本書の前提は次の通りです。

- 本書は、これからどう実装するかを定める。採用の理由は、ADRに従い、繰り返さない。規則と値は、仕様を参照し、書き写さない。
- 本書は、実装の完了後に削除する。実装の中で、異なる形が要る場合は、本書を直してから実装する。アーキテクチャ上の判断に当たる場合は、ADRを改めるか、新しいADRを起こす。
- 現在の姿(既存のパッケージ、ポート、経路)は、[アーキテクチャ概要](../architecture.md)にある。本書は、加えるものと、変えるものを示す。
- DD の番号は、本書の中のものである。ウォーキングスケルトンの詳細設計(コミット 0575ce9)の番号とは、対応しない。
- 「確認済み」は、2026-11-27 に、開発者の端末(Go 1.26.8)で確かめたことを指す。確かめていないことは、「実装の順序」の手順0に挙げる。

## 1. 本書で定めた事項

### 仕様とADRが、詳細設計に送った事項

| ID | 事項 | 送り元 | 定めた内容 | 節 |
|---|---|---|---|---|
| DD-01 | 画面の構成と操作の手段 | 仕様 | 1つの画面を、ログイン状態によって切り替える(ログイン、登録の申し込み、登録の完了、チャット)。クライアントは、送信の前に、入力を検証しない | 5.4 |
| DD-02 | 案内の文面 | 仕様、ADR-018 | 件名と本文を、4.5.7 の通りとする | 4.5.7 |
| DD-03 | 資格の形。確認のための資格を、案内に載せる形 | 仕様、ADR-014 | 32バイトの乱数を、base64url(パディングなし。43文字)で表す。案内には、`<アプリのオリジン>/#signup=<資格>` として載せる。`#` より後は、サーバーへ送られない。クライアントは、読み取った後、アドレスから除く | 4.5.9、5.3 |
| DD-04 | Cookie、要求ヘッダー、クエリの名前。終了コードの値 | ADR-014 | Cookie は `chat_session`、要求ヘッダーは `X-Session-Id`、クエリは `session`、終了コードは 4401 | 3.2、3.5 |
| DD-05 | 時刻の源。時間の経過を、自動テストで確かめる手段 | 仕様、ADR-014、ADR-016 | サーバーの時刻に揃える。usecase が、時刻のポート(`Clock`)から得た時刻を、ほかのポートに渡す。保管先の時刻は、期限の判定に使わない。テストは、`Clock` を差し替えて、時刻を進める | 4.3、7.1 |
| DD-06 | 期限に達した接続を閉じる仕組み。期限を過ぎた申し込みとログイン状態を消す時点と仕組み | 仕様、ADR-014、ADR-016 | 片付けのユースケース(`Housekeeper.Sweep`)を、起動の直後と、以後1分ごとに呼ぶ。配信は、配信の時点で期限を過ぎた接続に、届けずに、閉じる | 4.4.7、4.5.3 |
| DD-07 | 上限の数え方の実現。ポートの形。計数をメモリから消す仕組み | 仕様、ADR-016 | 数える単位ごとに、数えた時刻を持つ。確かめることと数えることは、1つの操作(`Admit`)で行う | 4.3、4.5.8 |
| DD-08 | 上限の値を設定する手段と名前 | 仕様 | 環境変数 `LIMIT_…`(6つ)。値は、`回数/時間`(例: `10/1h`)で書く | 4.6 |
| DD-09 | ログイン状態の確認を、usecase の中でまとめる形 | ADR-019 | usecase の型 `SessionGate` にまとめ、各ユースケースが、最初に呼ぶ。確かめる操作は、公開しない | 4.4.2 |
| DD-10 | 資格の要約を計算する場所 | ADR-019 | 保管先の adapter の中の、実現の手段とする。ポートは、資格を受ける。要約は、application に現れない | 4.5.4 |
| DD-11 | 同時に行う、秘密の計算の数の上限 | ADR-015 | 4(メモリは、約76MiB まで) | 4.5.5 |
| DD-12 | よく使われる秘密の一覧の置き場所、取り出すスクリプト、ポートの形 | ADR-017 | 一覧は `server/internal/adapter/commonsecrets/list.txt`、スクリプトは `scripts/extract-common-secrets.sh` | 4.5.6、6.2 |
| DD-13 | SMTP の設定の名前、接続先の判定の形、送信を待つ時間の上限、Mailpit のポートの番号を変える手段 | ADR-018 | `SMTP_ADDR`、`MAIL_FROM`。判定は、待ち受け先と同じ関数で行う。10秒。`MAILPIT_SMTP_PORT`、`MAILPIT_UI_PORT` | 4.6、6.1 |
| DD-14 | テストごとの、案内を読む手段 | ADR-018 | サーバーの振る舞いのテストは、案内のポートを差し替える。SMTP を通るのは、`smtpmail` のテストと、E2Eテストに限る | 7.1 |
| DD-15 | 取得する個人データの一覧の、置き場所 | 仕様 | `docs/personal-data.md` | 10 |
| DD-39 | ユースケースの公開する操作が通る、共通の1箇所の形 | ADR-021 | usecase の中の、公開しない型(`attempt`)が、操作の名前、送信元、特定できた利用者を持ち、出来事を記録する。公開するメソッドは、戻るときに、必ず、`attempt` の終わりの手順を通る | 4.4.8 |

仕様が送った「ログイン状態が無効になったこと(RL-19)と、別の利用者としてログインし直されたこと(RL-20)を、端末が知る手段」は、ADR-014 が定めています。クライアントの扱いは、5.2 の C-14 です。

### 具体化する中で判断した事項

| ID | 判断 | 理由 |
|---|---|---|
| DD-16 | 出来事の記録(R-6)は、usecase が、記録のポート(`EventRecorder`)を呼んで行う。ユースケースを呼ぶ adapter は、ユースケースが返したエラーを、ログに出力しなくなる | レビュー(2026-11-27)で、判断を、ADR-021 に移した。理由は、ADR-021 にある。記録を取りこぼさない形は、DD-39 が定める |
| DD-17 | 送信元は、要求の接続の相手のアドレスとする。中継が付けるヘッダー(`X-Forwarded-For` など)は、読まない | 中継を置く場合の定め方は、設置環境を扱うスライスに送られている(仕様「後続に送る事項」)。ローカルの開発環境では、すべての要求が、127.0.0.1 になる(RL-D5 の帰結) |
| DD-18 | 認証を要する操作では、ログイン状態の確認を、要求の形式の確認より先に行う。形式に合わない送信の要求と、WebSocket の接続を確立する要求でない要求は、adapter が拒否せず、印(`Malformed`)を付けて、ユースケースを呼ぶ。ユースケースは、確認のすぐ後に、ほかの手順より先に、`InvalidInput` で拒否する | 仕様「ログイン状態」は、確認を、ほかの検証より先に行うと定め、RL-15 は、すべての操作で、同じ応答を求める。ウォーキングスケルトンの順(形式 → 利用者)では、資格がなく、形式にも合わない要求に、`invalid_input` を返す。確かめるのは usecase であるため(ADR-019)、順を保つには、形式に合わない要求でも、ユースケースを呼ぶことになる。接続の受け付けも、同じ形に揃える。接続を登録した後に、adapter が拒否する形では、形式に合わない要求が、一時、配信の宛先に入る |
| DD-19 | 拒否の応答に、正しくない項目の一覧(`fields`)を加える | UC-01 E1 は、正しくない項目を、すべて示すと定める。`code` だけでは、表せない。値は、項目の名前の列挙に限り、文言と詳細は、含めない(WS-30) |
| DD-20 | 連絡先、識別名、表示名、秘密を、domain の、規則を通してしか作れない型にする。`User` の識別名と表示名も、この型に改める | 検証していない値を、保管と照合に渡す誤りを、型で防ぐ(本文と、送信の識別子と、同じ形である) |
| DD-21 | 秘密の型は、公開しない項目を持つ構造体とし、文字列にする操作(`String`、`GoString`)で、伏せた文字列を返す。値は、専用の操作(`Expose`)でだけ取り出す | 誤って、ログやエラーの文言に差し込んでも、値が現れない(SEC-1)。文字列を基にした型では、`String` を置いても、`log/slog` の JSON の出力に、値が、そのまま現れる。`GoString` がないと、`%#v` の書式に、値が現れる(どちらも、確認済み)。資格は、ユースケースの入力が文字列であるため(ADR-005)、型にしない。規約と、RL-33 のテストで確かめる(ADR-019) |
| DD-22 | 登録の完了のトランザクションは、既存のポート(`Transactor`)に、関数を1つ加えて行う。関数に渡す操作は、別のインターフェイス(`SignupWriter`)にする | ADR-016 は、ADR-012 のポートを使うと定める。会話のための操作(`ConversationWriter`)に加えると、送信のユースケースから、登録の操作を呼べるようになる |
| DD-23 | 登録の完了では、秘密の計算を、トランザクションの前に行う | 計算は、順番を待つことがある(DD-11)。その間、申し込みの行のロックを保たない |
| DD-24 | ログインの失敗は、照合の前に数え、照合に成功したときに、戻す | 確かめることと数えることを分けると、同時の要求が、上限を超えて照合に進む(ADR-016)。先に数えれば、上限の最後の1回を、同時の要求が分け合わない |
| DD-25 | ログインでは、秘密を、規則で検証しない。入力された通りに、照合に渡す | 規則を満たさない秘密は、保管した値と一致しない。照合の失敗として、同じ応答になる(仕様「ログインする」)。照合の計算も、同じく1回行う(RL-13)。入力の大きさは、DD-29 が限る |
| DD-26 | 接続を受け付けるユースケースは、接続を登録した後に、ログイン状態を、もう一度確かめる | 確かめてから登録するまでの間に、ログアウトが行われると、登録が、接続を閉じる処理の後になり、無効なログイン状態の接続が、期限まで残る |
| DD-27 | ログアウトと、ログインし直しは、ログイン状態を無効にしてから、接続を閉じる | 逆の順では、閉じた後、無効にする前に、同じ資格で、接続を登録できる。DD-26 の確認も、通る |
| DD-28 | 書き込み口に、ログイン状態が無効になったことを伝えて閉じる操作(`CloseSessionEnded`)を加える。閉じる理由を、引数で渡す形にしない | 受け付けの adapter は、`application/port` を import できない(ADR-005)。ポートが定義する型を、引数に取る操作は、実装できない |
| DD-29 | 登録とログインの要求の本文の大きさの上限を、16 KiB とする | 正しい要求は、最大でも約3KBである(128文字の秘密と、50文字の表示名が、すべてエスケープされた場合)。照合の計算に渡す入力の大きさも、これで限られる |
| DD-30 | 相手の指定を扱う手順(識別名の検証、自分自身、上限、居るかどうか)を、usecase の中の1つの関数にまとめ、相手の特定、履歴、送信の3つが使う | 仕様「上限」は、3つの操作に、同じ数え方と、同じ拒否の順を求める(RL-31) |
| DD-31 | 相手を特定した後、会話を開くのは、利用者の操作とする。特定と同時には、開かない | UC-08 基本フロー 2〜3 の通りにする。特定の結果(識別名と表示名)を、会話を開く前に示す(RL-24) |
| DD-32 | 片付けのユースケースを、一定の間隔で呼ぶ adapter(`adapter/schedule`)を加える。役割は、ユースケースを呼ぶ、である | main は、組み立てだけを行う(ADR-005)。時間の経過を相手とする adapter とすれば、既存の2つの役割に収まる |
| DD-33 | 時刻と、資格の生成は、`adapter/system` に、出来事の記録は、`adapter/eventlog` に置く | どちらも、ポートを実装する adapter であり、標準ライブラリだけを使う。相手(OS が与える時刻と乱数、ログの出力先)ごとに分ける |
| DD-34 | 案内の資格を持って開いた端末は、ログイン状態に依らず、登録を続ける画面を示す | RL-D9 が、案内から登録を続ける訪問者に、登録を続ける画面を示すと定める。そのブラウザに、別の利用者のログイン状態が残っていても、登録の後のログインで、置き換わる(RL-14) |
| DD-35 | 入力欄の内容(連絡先、識別名、表示名、秘密)は、ui の部品が持ち、送信のときに、application に渡す。送信の状態と、拒否の理由は、application が持つ | 入力欄の内容は、振る舞いを左右しない。拒否の後も、部品が、内容を保つ(UC-01 E1)。秘密を、application の状態として、画面の全体に配らない |
| DD-36 | クライアントの開発用のサーバーの記録から、`/api/` で始まるパスを、伏せる。記録の文言の形には、依らない | Vite は、中継に失敗したとき、要求のパスを記録する(Vite 8.3.8 のコードで、確認済み)。パスは、識別名を含む(`/api/users/{handle}` など)。RL-33 は、識別名が、このログに含まれないことを求める。記録の文言は、Vite の版によって変わりうるため、特定の文言だけを対象にしない |
| DD-37 | 実装は、加えるものを先に作り、既存の操作を、後から切り替える | 既存のユースケースの入力と、ポートを変えると、すべての層が、同時に変わる。内側の層と、ポートの実装を、先に、テストとともに加えれば、切り替えの前の各手順を、`make check` が通る状態で終えられる(9) |
| DD-38 | 登録の完了で、連絡先が登録済みであった場合は、確認のための資格が無効な場合として、拒否する(`InvalidSignupCredential`) | 申し込みが、登録済みでないことを確かめてから、申し込みを置くまでの間に、同じ連絡先の完了が確定すると、登録済みの連絡先に、申し込みが残る。その資格での完了は、連絡先の一意性の制約に当たる。扱いを定めないと、期限まで、完了できなかったこと(`internal`)を返し続ける。訪問者には、申し込み直しを求める(UC-01 E3)。申し込み直すと、登録済みであることを伝える案内が届く(UC-01 A1) |

### レビューで確かめたい事項

上位の文書、ADR、規約の文言と、そのままでは合わない点です。本書は、次の読み方で設計しています。「レビューの結果」は、2026-11-27 のレビューで定めたことです。同じレビューで、ADR-020 と ADR-021 を起こし、DD-38 と DD-39 を加えました。

| # | 点 | 本書の読み方 | レビューの結果 |
|---|---|---|---|
| 1 | ADR-019 は、ユースケースの出力に、資格を含めないと定める。ログインは、新しく与えた資格を、Cookie に載せるために、adapter へ返す必要がある | 「受け取った資格を、返さない」と読み、ログインの出力だけが、新しく与えた資格を含む(4.4.4)。ADR-019 の文言を補うかどうかは、開発者が決める | ADR-019 の Decision を改めた。新しく与えた資格を、ログインの出力に含めることと、資格を渡してよいポートを、定めている |
| 2 | `.claude/rules/contract-and-codegen.md` と `server.md` は、拒否の応答を、`code` だけと定める | DD-19 で、`fields` を加える。実装のときに、指示を直す | 読み方の通りとする。指示は、手順6で直す(9) |
| 3 | 識別名は、操作のパスに載る(ウォーキングスケルトンから続く形)。仕様「記録」は、識別名を、記録しないと定める | サーバーは、要求のパスを記録しない。クライアントの開発用のサーバーは、DD-36 で除く。設置環境で、中継が記録するパスの扱いは、設置環境を扱うスライスで定める(仕様「後続に送る事項」の、記録の保持期間とともに) | 読み方の通りとする。DD-36 を、記録の文言の形に依らないものに改めた |
| 4 | DD-27 の順では、無効にしてから、接続を閉じるまでの間(続く2つの呼び出しの間)に確定した配信が、その接続に届きうる | 逆の順より、残る害が小さいと判断した。RL-18 のテストは、ログアウトの応答の後の送信で確かめる | 読み方の通りとする。帰結を、ADR-014 の Consequences に記載した |
| 5 | RL-09 の「24時間以内に消える」は、片付けが動いている間に成り立つ。サーバーを止めている間は、消えない | 起動の直後に、片付けを1回行う(DD-06) | 読み方の通りとする。帰結を、ADR-016 の Consequences に記載した |
| 6 | マイグレーション 00001 は、仮実装(WS-P1)の注釈を持つ。RL-42 は、この注釈を除くことを求める。`.claude/rules/contract-and-codegen.md` は、既存のマイグレーションのファイルを書き換える場合は、開発者に確かめると定める | 注釈だけを除き、構造を定める文は、変えない(8)。書き換えてよいかは、開発者が決める | 注釈だけを書き換える。適用済みのデータベースと、生成したコードに、影響しないことは、確かめていない。手順6で、`make db-migrate` と `make generate-check` によって確かめる |

## 2. 全体の構成

### 加える置き場所と、除く置き場所

```
server/internal/adapter/argon2id/       秘密の計算と照合(ポートを実装する)
server/internal/adapter/commonsecrets/  よく使われる秘密の一覧(ポートを実装する)。list.txt、NOTICE.md
server/internal/adapter/smtpmail/       案内の送信(ポートを実装する)
server/internal/adapter/ratelimit/      上限の計数(ポートを実装する)
server/internal/adapter/system/         時刻、資格の生成(ポートを実装する)
server/internal/adapter/eventlog/       出来事の記録(ポートを実装する)
server/internal/adapter/schedule/       片付けを、一定の間隔で呼ぶ(ユースケースを呼ぶ)
server/internal/adapter/fixeduser/      除く(仮実装 WS-P1)
server/migrations/00002_…、00003_…     利用者、ログイン状態、登録の申し込み。外部キー
client/tools/                           開発用のサーバーの設定が使う関数(DD-36)
scripts/extract-common-secrets.sh、scripts/extract-common-secrets.go
docs/personal-data.md                   取得する個人データの一覧(DD-15)
```

### 値の一覧

| 値 | 内容 | 出どころ |
|---|---|---|
| ログイン状態の有効期限 | ログインから30日 | 仕様「ログイン状態」 |
| 確認のための資格の有効期限 | 申し込みから24時間 | 仕様「登録する」 |
| 上限の既定値 | 仕様「上限」の表の通り | 仕様 |
| 片付けの間隔 | 1分 | DD-06 |
| 同時に行う、秘密の計算の数 | 4 | DD-11 |
| 案内の送信を待つ時間の上限 | 10秒 | DD-13 |
| 要求の本文の大きさの上限 | 登録とログインは 16 KiB。送信は 1 MiB(これまで通り) | DD-29 |

## 3. 通信の形式

`api/openapi.yaml` に、次を加え、仮のログイン(`securitySchemes.fixedUser`)を除きます。

### 3.1 操作

| 操作 | 方法とパス | 認証 | 要求 | 完了の応答 |
|---|---|---|---|---|
| 登録を申し込む(`requestSignup`) | `POST /api/signups` | 要さない | `SignupRequest` | 204 |
| 登録を完了する(`completeSignup`) | `POST /api/users` | 要さない | `CompleteSignupRequest` | 204 |
| ログインする(`login`) | `POST /api/session` | 要さない | `LoginRequest` | 200 `Session`。Cookie を与える |
| 現在のログイン状態を確かめる(`getSession`) | `GET /api/session` | Cookie だけを見る | なし | 200 `SessionStatus` |
| ログアウトする(`logout`) | `DELETE /api/session` | 要する | なし | 204。Cookie を消す |
| 相手を特定する(`findUser`) | `GET /api/users/{handle}` | 要する | なし | 200 `UserProfile` |
| 会話の履歴を読む、メッセージを送る | これまで通り | 要する | これまで通り | これまで通り |

- 確認のための資格は、要求の本文で送る。パスとクエリには、載せない(RL-33)。
- 登録の申し込みの完了の応答は、連絡先が登録済みかどうかに依らず、同じである(RL-03)。
- 登録の完了は、ログイン状態を与えない(仕様「登録する」)。
- 利用者の一覧を取得する操作と、部分一致で探す操作は、置かない(RL-27)。

### 3.2 ログイン状態の添え方

| 経路 | 添え方 |
|---|---|
| 資格 | Cookie `chat_session`。属性は、ADR-014 の表の通り。ブラウザが付ける |
| ログイン状態の識別子(HTTP の操作) | 要求ヘッダー `X-Session-Id` |
| ログイン状態の識別子(WebSocket の接続を確立する要求) | クエリ `session` |

OpenAPI の文書では、`securitySchemes` に、2つ(Cookie と、要求ヘッダー)を記述し、認証を要する操作に、両方を求めます。生成したコードは、これを取り出しません。adapter が、要求から取り出します。Cookie の名前は、`httpapi` と `wsaccept` に、それぞれ定数として持ちます。食い違いは、接続のテスト(7.2 の RL-15)が検出します。

### 3.3 スキーマ

| スキーマ | 項目 |
|---|---|
| `SignupRequest` | `contact` |
| `CompleteSignupRequest` | `signupCredential`、`handle`、`displayName`、`secret` |
| `LoginRequest` | `contact`、`secret` |
| `Session` | `sessionId`(ログイン状態の識別子)、`user`(`UserProfile`) |
| `SessionStatus` | `authenticated`(真偽)、`session`(`Session`。ログインしている場合だけ) |
| `InputField` | `contact`、`handle`、`displayName`、`secret` の列挙 |
| `ErrorResponse` | `code` に、`fields`(`InputField` の配列。`invalid_fields` の場合だけ)を加える |
| `ErrorCode` | 3.4 の値を加える |

- 項目は、上に断ったものを除き、必須とする。文字列は、文書の上では、制約を付けない。検証は、内側の層で行う。
- `UserProfile` は、識別名と表示名だけを持つ(RL-27)。連絡先を、どの応答にも含めない。
- 資格は、どのスキーマにも含めない。応答の本文には、現れない(ADR-014)。

### 3.4 拒否とエラーの応答

加える `code` と、既存の `code` に加わる場合は、次の通りです。

| `code` | 状態 | 場合 | 判定する層 |
|---|---|---|---|
| `unauthenticated` | 401 | ログイン状態が有効でない(RL-15)。理由を、区別しない | usecase |
| `invalid_input` | 400 | 認証を要さない操作の要求が、形式に合わない | adapter |
| `invalid_input` | 400 | 認証を要する操作の要求が、形式に合わない(DD-18)。相手の特定で、入力が、識別名の規則を満たさない | usecase |
| `invalid_fields` | 400 | 連絡先、識別名、表示名、秘密が、入力の規則を満たさない。`fields` に、正しくない項目を、すべて挙げる | usecase |
| `rate_limited` | 429 | 上限を超えた | usecase |
| `invalid_signup_credential` | 400 | 確認のための資格が、無効である。理由を、区別しない | usecase |
| `handle_taken` | 409 | 識別名が、使われている | usecase |
| `login_failed` | 401 | 照合に失敗した。理由を、区別しない | usecase |

1つの要求が、複数の場合に当たるときの順は、次の通りです。文書の説明も、この通りに直します。

- どの操作でも、Host、Origin が先である(これまで通り)。
- 認証を要する操作は、続いて、ログイン状態、形式、操作ごとの順とする(DD-18)。
- 認証を要さない操作は、続いて、形式、操作ごとの順とする。

| 操作 | 操作ごとの順 |
|---|---|
| 登録の申し込み | 連絡先の規則 → 上限 |
| 登録の完了 | 上限 → 資格が無効 → 入力の規則 → 識別名が使われている |
| ログイン | 上限 → 照合の失敗 |
| 相手の特定 | 識別名の規則 → 自分自身 → 上限 → 居ない |
| 履歴 | 自分自身 → 上限 → 相手が居ない |
| 送信 | 本文 → 送信の識別子の形式 → 自分自身 → 上限 → 相手が居ない → 別の会話との一致 |

履歴と送信で、相手の指定が、識別名の規則を満たさない場合は、相手が居ない場合として扱います(`not_found`)。上限には、数えません(仕様「上限」)。

### 3.5 WebSocket

- 接続先は、`GET /api/realtime?session=<ログイン状態の識別子>` とする。資格は、Cookie で届く。
- 接続を確立する前の拒否は、ログイン状態(`unauthenticated`)、形式(`invalid_input`)の順とする。
- サーバーは、ログイン状態が無効になった接続(ログアウト、同じブラウザでのログインし直し、有効期限)を、終了コード 4401 で閉じる。送信待ちの列が一杯になった接続は、これまで通り、1008 で閉じる。
- 4401 は、アプリが使える範囲(4000〜4999)の値である。coder/websocket v1.8.15 で、サーバーがこの値で閉じ、相手が読めることを、試作で確かめた(確認済み)。

## 4. サーバー

### 4.1 パッケージと、import してよい先

加えるパッケージの規則です。`server/.golangci.deps.yml` に加え、規則に違反する見本を置きます(`.claude/rules/server.md`「パッケージを加えるとき」)。`adapter/fixeduser` の規則と見本は、除きます。

| パッケージ | 役割 | import してよい先(標準ライブラリのほか) |
|---|---|---|
| `adapter/argon2id` | ポートを実装する | `application/port`、`domain`、golang.org/x/crypto/argon2 |
| `adapter/commonsecrets` | ポートを実装する | `application/port`、`domain` |
| `adapter/smtpmail` | ポートを実装する | `application/port`、`domain` |
| `adapter/ratelimit` | ポートを実装する | `application/port`、`domain` |
| `adapter/system` | ポートを実装する | `application/port`、`domain` |
| `adapter/eventlog` | ポートを実装する | `application/port`、`domain` |
| `adapter/schedule` | ユースケースを呼ぶ | `application/usecase` |

- golang.org/x/crypto を import できるのは、`adapter/argon2id` だけとする(ADR-015)。
- `domain` と `application` が import してよい標準ライブラリの列挙は、変えない。乱数、要約、ログは、adapter が受け持つ。
- 見本は、少なくとも次を加える。ポートを実装する6つのパッケージから `application/usecase` へ。`schedule` から `application/port` へ。`application/usecase` から golang.org/x/crypto/argon2 と、`crypto/rand` へ。

### 4.2 domain

| 型 | 内容と規則 |
|---|---|
| `Contact` | 連絡先。`NewContact(raw)` で作る。前後の空白文字を除き、英字を小文字に揃えてから、仕様「入力の規則」を確かめる。保管した値からは、`RestoreContact` で組み立てる |
| `Handle` | 識別名。`NewHandle(raw)` で作る。入力された通りに保持する。`Key()` は、小文字に揃えた値を返す(重複と、照合の判定に使う)。`RestoreHandle` を置く |
| `DisplayName` | 表示名。`NewDisplayName(raw)` で作る。前後の空白文字を除いてから、確かめる。`RestoreDisplayName` を置く |
| `Secret` | 秘密。`NewSecret(raw, contact, rawHandle)` で作る。長さ、文字列として正しいこと、連絡先・識別名と同じでないこと(大文字と小文字を区別しない)を確かめる。一覧との照合は、ポートで行う(4.4.3)。公開しない項目を持つ構造体とし、`Expose()` で値を取り出す(DD-21) |
| `SecretHash` | 保管する形の秘密。規則を持たない、文字列の型 |
| `User` | `ID`、`Handle`(`Handle` に改める)、`DisplayName`(`DisplayName` に改める)。連絡先と秘密は、持たない |
| `SessionID`、`Session` | ログイン状態の識別子(UUID の文字列)。`Session` は、`ID`、`UserID`、`ExpiresAt`。`ValidAt(now)` は、`now` が `ExpiresAt` より前であるかを返す。配信の adapter も、これを呼ぶ(ADR-020) |
| `Signup` | 登録の申し込み。`Contact`、`ExpiresAt` |
| `LimitKind`、`LimitRule`、`LimitRules`、`Limit` | 上限。`LimitKind` は、仕様「上限」の表の6行。`LimitRule` は、回数と時間。`LimitRules` は、6行の値で、`DefaultLimitRules()` が既定値を返す。`Limit` は、種類、数える対象(送信元、連絡先、利用者の識別子)、値、数え方(下記) |
| `EventKind`、`Event` | 出来事の記録(4.4.8) |
| 定数 | `SessionLifetime`(30日)、`SignupLifetime`(24時間) |

- 入力の規則を満たさない場合、作る関数は、項目ごとのエラー(`ErrInvalidContact` など)を返す。usecase が、拒否の種類と、項目の一覧に置き換える。
- 復元用の関数は、保管先の adapter が、行から組み立てる型(`Contact`、`Handle`、`DisplayName`)にだけ置く。`Secret` には、置かない。
- 有効かどうかの判定は、「`now` が、期限より前」に揃える。保管先の問い合わせ(`expires_at > now`)も、同じにする。
- 上限の数え方は、2つである。「T に N 回」(直近の T の間に数えた回数が、N に達している間、拒否する)と、「連続して N 回で、T の間、拒否する」(ログインの、連絡先ごとの上限)。`Limit` は、どちらであるかを持つ。

### 4.3 application/port

境界要素として数える、型の宣言です(ADR-008)。加えるものが11、変えるものが5、変えないものが2(`ConversationReader`、`ConversationWriter`)で、計18になります。

```go
// 変える。実装は、保管先(仮実装 WS-P1 を置き換える)。
type UserDirectory interface {
    // 識別名が一致する利用者を探す。大文字と小文字を、区別しない。
    FindByHandle(ctx context.Context, handle domain.Handle) (user domain.User, found bool, err error)
    // 連絡先が一致する利用者と、保管した秘密を探す。
    FindByContact(ctx context.Context, contact domain.Contact) (user domain.User, secret domain.SecretHash, found bool, err error)
}

// ログイン状態を保持する(ADR-016)。資格は、要約で保管し、要約で引く(DD-10)。
type SessionStore interface {
    Create(ctx context.Context, credential string, user domain.UserID, expiresAt time.Time) (domain.Session, error)
    // 資格に当たる、now の時点で有効なログイン状態と、その利用者を探す。
    Find(ctx context.Context, credential string, now time.Time) (session domain.Session, user domain.User, found bool, err error)
    // 資格に当たるログイン状態を、無効にする。
    Revoke(ctx context.Context, credential string) (id domain.SessionID, found bool, err error)
    DeleteExpired(ctx context.Context, now time.Time) error
}

// 登録の申し込みを保持する(ADR-016)。
type SignupStore interface {
    // 申し込みを置く。同じ連絡先の、以前の申し込みを置き換える(RL-04)。
    Put(ctx context.Context, contact domain.Contact, credential string, expiresAt time.Time) error
    // 資格に当たる、now の時点で有効な申し込みを探す。
    Find(ctx context.Context, credential string, now time.Time) (signup domain.Signup, found bool, err error)
    DeleteExpired(ctx context.Context, now time.Time) error
}

// 変える。登録の完了を、一体として行う関数を加える(DD-22)。InTransaction は、変えない。
type Transactor interface {
    InTransaction(ctx context.Context, fn func(ctx context.Context, w ConversationWriter) error) error
    InSignupTransaction(ctx context.Context, fn func(ctx context.Context, w SignupWriter) error) error
}

// 登録の完了の、トランザクションの中でだけ行える操作。
type SignupWriter interface {
    // 資格に当たる、now の時点で有効な申し込みを、使用済みにして、返す。
    ConsumeSignup(ctx context.Context, credential string, now time.Time) (signup domain.Signup, found bool, err error)
    // 利用者を作る。識別名が使われていれば(大文字と小文字を区別しない)、ErrHandleTaken を返す。
    // 連絡先が登録済みであれば、ErrContactTaken を返す(DD-38)。
    CreateUser(ctx context.Context, contact domain.Contact, handle domain.Handle, name domain.DisplayName, secret domain.SecretHash) (domain.User, error)
}

var (
    ErrHandleTaken  = errors.New("port: handle taken")
    ErrContactTaken = errors.New("port: contact taken")
)

// 変える。now の時点で、期限を過ぎたログイン状態による接続には、届けずに、閉じる(DD-06)。
type MessageDelivery interface {
    Deliver(ctx context.Context, now time.Time, participants []domain.User, msg domain.Message)
}

// 変える。CloseSessionEnded を加える(DD-28)。
type ConnectionSink interface {
    Send(data []byte) (queued bool)
    Close()
    // ログイン状態が無効になったことを伝えて、接続を閉じる。何度呼んでもよい。
    CloseSessionEnded()
}

// 変える。接続を、利用者に加えて、ログイン状態と対応付ける(ADR-014)。
type ConnectionRegistry interface {
    Register(ctx context.Context, session domain.Session, sink ConnectionSink)
    Unregister(ctx context.Context, sink ConnectionSink)
}

// ログイン状態による接続を閉じる。配信の adapter が実装する。
type SessionConnections interface {
    CloseBySession(ctx context.Context, id domain.SessionID)
    CloseExpired(ctx context.Context, now time.Time)
}

// 資格を作る(ADR-019)。
type CredentialSource interface {
    NewCredential() (string, error)
}

// 秘密を、保管する形に変え、照合する(ADR-015)。
type SecretHasher interface {
    Hash(ctx context.Context, secret domain.Secret) (domain.SecretHash, error)
    Verify(ctx context.Context, secret string, stored domain.SecretHash) (matched bool, err error)
    // 照合する相手がないログインで、Verify と同じ重さの計算を、1回行う(RL-13)。
    VerifyDummy(ctx context.Context) error
}

// 秘密が、よく使われる秘密の一覧にあるかを確かめる(ADR-017)。
type CommonSecretList interface {
    Contains(secret domain.Secret) bool
}

// 案内を送る(ADR-018)。
type NoticeSender interface {
    SendSignupNotice(ctx context.Context, to domain.Contact, credential string, expiresAt time.Time) error
    SendAlreadyRegisteredNotice(ctx context.Context, to domain.Contact) error
}

// 上限を数える(ADR-016)。
type AttemptLimiter interface {
    // limits のすべてが、上限に達していないことを確かめ、達していなければ、それぞれに、1回と数える。
    // 達しているものがあれば、どれも数えずに、最初に当たったものを返す。
    Admit(ctx context.Context, now time.Time, limits ...domain.Limit) (exceeded domain.Limit, admitted bool, err error)
    // Admit が数えた1回を、戻す。
    Forgive(ctx context.Context, limit domain.Limit) error
    // 計数を、0に戻す。
    Reset(ctx context.Context, limit domain.Limit) error
}

type Clock interface {
    Now() time.Time
}

// 出来事を記録する(R-6、DD-16)。
type EventRecorder interface {
    Record(ctx context.Context, event domain.Event)
}
```

- 時刻を要する操作は、`now` を、引数で受ける。ポートの実装は、自分では、時刻を読まない(DD-05)。メッセージの送信の日時は、これまで通り、保管先が付ける(ADR-010)。
- `AttemptLimiter` は、`ctx` と `error` を持つ。メモリの上の実装は、使わないが、共有の保管先を使う実装に差し替えるとき、内側の層を変えないためである(ADR-016 の確認)。計数を確かめられなかった操作は、受け付けない。
- `EventRecorder` と `MessageDelivery` は、エラーを返さない。記録と配信の失敗は、操作の結果を変えない。

### 4.4 application/usecase

#### 4.4.1 入力・出力のデータ構造と、拒否の種類

境界要素として数える、型の宣言です。domain の型を、含めません。

```go
// 操作を行う利用者。adapter が、要求から取り出して、そのまま渡す(ADR-019)。添えられていなければ、空。
type Actor struct{ Credential, SessionID string }

type UserView struct{ Handle, DisplayName string }

type RequestSignupInput  struct{ Contact, Source string }
type CompleteSignupInput struct{ Credential, Handle, DisplayName, Secret, Source string }

type LoginInput struct{ Credential, Contact, Secret, Source string } // Credential は、要求に付いていた資格
type LoginOutput struct {
    Credential string    // 新しく与えた資格。adapter は、Cookie にだけ載せる
    SessionID  string
    ExpiresAt  time.Time
    User       UserView
}

type CurrentSessionInput  struct{ Credential, Source string }
type CurrentSessionOutput struct {
    Authenticated bool
    SessionID     string
    User          UserView
}

type LogoutInput struct{ Actor Actor; Source string }

type FindPeerInput  struct{ Actor Actor; Source, Handle string }
type FindPeerOutput struct{ Peer UserView }

// 変える。Actor は、識別名から Actor に、Source を加える。
type SendMessageInput struct {
    Actor     Actor
    Source    string
    Peer      string
    SendID    string
    Body      string
    Malformed bool // 要求が、形式に合わない(DD-18)。SendID と Body は、空
}
type ReadHistoryInput      struct{ Actor Actor; Source, Peer string }
type AcceptConnectionInput struct {
    Actor      Actor
    Source     string
    Malformed  bool // WebSocket の接続を確立する要求でない(DD-18)
    Connection port.ConnectionSink
}

type InputField int // FieldContact、FieldHandle、FieldDisplayName、FieldSecret

// 加える種類: InvalidFields、RateLimited、InvalidSignupCredential、HandleTaken、LoginFailed
type RejectedError struct {
    Kind   Rejection
    Fields []InputField // InvalidFields の場合だけ
}
```

- `Source` は、送信元である(DD-17)。
- 資格を含む出力は、`LoginOutput` だけである(ADR-019)。拒否とエラーには、資格と秘密を含めない。
- `CloseConnectionInput` と、出力(`MessageView` など)は、変えない。

ユースケースの型と、使うポートは、次の通りです。main は、それぞれに、使うものだけを渡します。

| 型 | メソッド | 使うポート(`SessionGate` を通すものを除く) |
|---|---|---|
| `SessionGate` | (確認の操作は、公開しない) | `SessionStore`、`Clock`(記録は、呼んだユースケースの `attempt` を通すため、`EventRecorder` は、受け取らない) |
| `SignupRequester` | `Request` | `UserDirectory`、`SignupStore`、`CredentialSource`、`NoticeSender`、`AttemptLimiter`、`Clock`、`EventRecorder` |
| `SignupCompleter` | `Complete` | `SignupStore`、`CommonSecretList`、`SecretHasher`、`Transactor`、`AttemptLimiter`、`Clock`、`EventRecorder` |
| `Authenticator` | `Login` | `UserDirectory`、`SecretHasher`、`SessionStore`、`SessionConnections`、`CredentialSource`、`AttemptLimiter`、`Clock`、`EventRecorder` |
| `SessionKeeper` | `Current`、`Logout` | `SessionStore`、`SessionConnections`、`Clock`、`EventRecorder` |
| `PeerFinder` | `Find` | `UserDirectory`、`ConversationReader`、`AttemptLimiter`、`Clock`、`EventRecorder` |
| `MessageSender`、`HistoryReader`(変える) | `Send`、`Read` | これまでのものに、`AttemptLimiter`、`Clock`、`EventRecorder` を加える |
| `ConnectionKeeper`(変える) | `Accept`、`Close` | `ConnectionRegistry`、`EventRecorder` |
| `Housekeeper` | `Sweep` | `SessionConnections`、`SessionStore`、`SignupStore`、`Clock`、`EventRecorder` |

#### 4.4.2 ログイン状態の確認(`SessionGate`。DD-09)

main が1つ作り、認証を要するユースケース(`SessionKeeper.Logout`、`PeerFinder`、`MessageSender`、`HistoryReader`、`ConnectionKeeper.Accept`)に渡します。各ユースケースは、最初の手順として、確認を呼びます。確認の操作は、公開しないため、adapter からは呼べません。

1. `Actor` の資格か、識別子が空であれば、拒否する。
2. `SessionStore.Find(資格, now)`。当たるものがなければ、拒否する。
3. ログイン状態の識別子が、`Actor` の識別子と異なれば、拒否する(ADR-014)。
4. ログイン状態と、利用者を返す。

- 拒否は、どの理由でも、`Unauthenticated` とし、認証の拒否を記録する(4.4.8)。
- `Find` がエラーを返した場合は、エラーを返す。操作は、受け付けない(RL-23)。記録は、呼んだユースケースの `attempt` が行う(4.4.8)。
- 確認できた利用者は、`attempt` に入れる。以後の記録に、添えられる。
- 保管先への照会は、操作につき、1回である(ログイン状態と利用者を、1つの問い合わせで得る)。接続の受け付けだけは、登録の後に、もう一度確かめるため、2回である(DD-26)。

#### 4.4.3 登録(`SignupRequester.Request`、`SignupCompleter.Complete`)

**申し込み**

1. `domain.NewContact`。エラーなら、`InvalidFields`(連絡先)。
2. `AttemptLimiter.Admit`(送信元ごと、宛先の連絡先ごと)。達していれば、`RateLimited`。
3. 申し込みを受け付けたことを、記録する。
4. `UserDirectory.FindByContact`。
   - 登録済みであれば、`NoticeSender.SendAlreadyRegisteredNotice`。申し込みは、置かない。アカウントとログイン状態は、変えない(RL-03)。
   - 登録されていなければ、資格を作り、`SignupStore.Put(連絡先, 資格, now + 24時間)`、`NoticeSender.SendSignupNotice` の順に行う。
5. 案内を送れなかった場合は、その旨(`signup_notice_failed`)を記録し、エラーを返す(RL-10)。エラーの経路に、例外を作らないため、`operation_failed` も、続けて記録される(4.4.8)。手順2の計数は、戻さない(仕様「登録する」)。

申し込みを置いてから、案内を送ります。逆の順では、置くことに失敗したとき、使えない資格の案内が届きます。

**完了**

1. `AttemptLimiter.Admit`(送信元ごと)。達していれば、`RateLimited`。
2. `SignupStore.Find(資格, now)`。当たるものがなければ、`InvalidSignupCredential`。識別名は、まだ見ない(RL-08)。
3. `NewHandle`、`NewDisplayName`、`NewSecret`、`CommonSecretList.Contains` を、すべて行う。正しくない項目が1つでもあれば、`InvalidFields` に、すべてを挙げる。
4. `SecretHasher.Hash`(DD-23)。
5. `Transactor.InSignupTransaction` の中で、`ConsumeSignup(資格, now)`、`CreateUser` の順に呼ぶ。
   - `ConsumeSignup` が、当たるものがないと返した場合は、取り消して、`InvalidSignupCredential`(同時の完了、手順2の後の再度の申し込み)。
   - `CreateUser` が `ErrHandleTaken` を返した場合は、取り消して、`HandleTaken`。申し込みは、使用済みにならない(UC-01 E2)。
   - `CreateUser` が `ErrContactTaken` を返した場合は、取り消して、`InvalidSignupCredential`(DD-38)。申し込みは、期限まで残り、片付けが消す。
6. 登録の完了を、記録する。

手順5は、ADR-012 と同じく、制約に当たったトランザクションの全体を取り消します。関数の中では、渡された操作のほかに、ポートを呼びません。

#### 4.4.4 ログインとログアウト(`Authenticator.Login`、`SessionKeeper`)

**ログイン**

1. `domain.NewContact`。結果は、手順3で使う。
2. `AttemptLimiter.Admit`(送信元ごと。連絡先が正しければ、連絡先ごとも)。達していれば、`RateLimited`。照合は、行わない。ここで数えた1回は、失敗として残る(DD-24)。
3. 連絡先が正しくない場合と、`FindByContact` が、居ないと返した場合は、`VerifyDummy` を呼び、手順5へ。
4. `SecretHasher.Verify(入力された秘密, 保管した秘密)`(DD-25)。一致すれば、手順6へ。
5. ログインの失敗を記録し、`LoginFailed`。それまでのログイン状態は、変えない。
6. 送信元ごとの計数を戻し(`Forgive`)、連絡先ごとの計数を、0に戻す(`Reset`)。
7. 要求に付いていた資格があれば、`SessionStore.Revoke`。当たるものがあれば、`SessionConnections.CloseBySession`(DD-27、RL-14)。
8. 資格を作り、`SessionStore.Create(資格, 利用者, now + 30日)`。ログインの成功を記録し、出力を返す。

失敗の経路は、どれも、秘密の計算を、ちょうど1回行います(RL-13)。上限による拒否は、除きます(ADR-015)。

**現在のログイン状態を確かめる(`Current`)**

`SessionGate` を通しません。資格が空の場合と、`SessionStore.Find` が、当たるものがないと返した場合は、`Authenticated: false` を返します。拒否とせず、記録しません(RL-D9)。`Find` がエラーを返した場合は、エラーを返します(RL-23)。

**ログアウト(`Logout`)**

`SessionGate` の確認、`SessionStore.Revoke`、`SessionConnections.CloseBySession`、記録の順に行います。

#### 4.4.5 相手の指定(`peerResolver`。DD-30)と、相手の特定(`PeerFinder.Find`)

`peerResolver` は、usecase の中の、公開しない関数です。確認済みの利用者と、相手の指定(文字列)を受けて、次の順に行います。

1. `domain.NewHandle`。エラーなら、相手の特定では `InvalidInput`、履歴と送信では `NotFound`。
2. 識別名の `Key()` が、自分のものと同じであれば、`InvalidPeer`。
3. `UserDirectory.FindByHandle`。居れば、`NewDirectPair` と、`ConversationReader.FindDirect` で、成立した会話があるかを確かめる。
4. 成立した会話がなければ(相手が居ない場合を含む)、`AttemptLimiter.Admit`(利用者ごと)。達していれば、`RateLimited`。
5. 相手が居なければ、`NotFound`。
6. 相手、2人の組、会話(あれば)を返す。

- 手順4は、相手が居るかどうかを、応答に出す前に行う。上限に達している間、居る場合と、居ない場合が、同じ応答になる(RL-31)。
- 成立した会話がある相手は、手順4を通らない。上限に達している間も、受け付ける。

`PeerFinder.Find` は、`SessionGate` の確認の後に、`peerResolver` を呼び、相手の識別名(登録した通り)と表示名を返します。`MessageSender.Send` は、本文と、送信の識別子の検証の後に、`HistoryReader.Read` は、確認のすぐ後に、`peerResolver` を呼びます。以後は、これまで通りです。利用者を、識別名で確かめる手順(仮のログイン)は、なくなります。`Send` は、`Malformed` の要求を、確認のすぐ後に、`InvalidInput` で拒否します。配信には、`Clock` から得た時刻を渡します。

#### 4.4.6 リアルタイムの接続を受け付ける(`ConnectionKeeper.Accept`)

1. `SessionGate` の確認。
2. `Malformed` であれば、`InvalidInput`。接続は、登録しない(DD-18)。
3. `ConnectionRegistry.Register(ログイン状態, 書き込み口)`。
4. もう一度、ログイン状態を確かめる(DD-26)。有効でなくなっていれば、`Unregister` を呼び、`Unauthenticated`。

#### 4.4.7 片付け(`Housekeeper.Sweep`。DD-06)

`Clock` から時刻を得て、`SessionConnections.CloseExpired`、`SessionStore.DeleteExpired`、`SignupStore.DeleteExpired` を、この順に呼びます。入力は、ありません。操作を行う利用者が居ない点は、「接続の終わりを受ける」と同じです。

- 接続は、期限から、最長で1分遅れて、閉じられる。その間に保管されたメッセージは、配信の時点の確認によって、届かない(RL-18)。
- 複数サーバー構成では、各サーバーが、片付けを行う。削除は、何度行っても、同じ結果になる。
- どれかがエラーを返した場合は、そこで戻り、`attempt` が記録する(4.4.8)。残りは、次の間隔で、やり直す。

#### 4.4.8 出来事の記録(DD-16)

| 出来事(`event`) | 記録する場合 | 利用者の識別子 |
|---|---|---|
| `signup_requested` | 登録の申し込みを、受け付けた(UC-01 A1、A2 を含む) | なし |
| `signup_notice_failed` | 案内を、送れなかった | なし |
| `signup_completed` | 登録を完了した | 作った利用者 |
| `login_succeeded`、`login_failed` | 照合に成功した、失敗した | 失敗では、連絡先が登録済みの場合に、その利用者 |
| `logged_out` | ログアウトを完了した | その利用者 |
| `authentication_rejected` | ログイン状態が有効でない要求を、拒否した | なし |
| `limit_exceeded` | 上限によって、拒否した。数える単位の種類(`limit`)を添える | 利用者ごとの上限の場合に、その利用者 |
| `operation_failed` | 操作を完了できなかった。操作の名前と、エラーを添える | 特定できていれば、その利用者 |

- 要求による操作の出来事には、送信元を添える。片付けは、要求によらないため、添えない。発生時刻は、記録の時刻である。
- 数える対象の値(連絡先など)は、添えない。連絡先、識別名、表示名、秘密、資格、本文は、`Event` に、項目がない(仕様「記録」、RL-D8)。
- エラーの文言は、adapter が、行の内容や宛先を含まない形に作り直したものだけが、ここへ届く(`postgres.OpError`、4.5.7)。

**記録の通り道(DD-39、ADR-021)**

エラーの記録を、取りこぼさないための形です。

- usecase の中の、公開しない型 `attempt` が、操作の名前、送信元、特定できた利用者を持つ。出来事は、すべて、`attempt` を通して記録する。送信元と利用者は、`attempt` が添える。
- ユースケースの公開するメソッドは、最初に `attempt` を作り、戻るときに、終わりの手順(`finish`)を、`defer` で呼ぶ。`finish` は、戻り値のエラーが、拒否(`*RejectedError`)でない場合に、`operation_failed` を記録する。途中の手順は、エラーを、そのまま返すだけでよい。
- `SessionGate` は、`authentication_rejected` を、`attempt` を通して記録する。確認を完了できなかった場合の記録は、`finish` が行う。
- 対象は、4.4.1 の表のメソッドのうち、エラーを返すもの、すべてである。`ConnectionKeeper.Close` は、エラーを返さないため、除く。
- usecase のテストは、メソッドごとに、ポートを失敗させて、`operation_failed` が、1回だけ記録されることを確かめる(7.2 の RL-32)。`attempt` を通さないメソッドを加えると、このテストが、検出する。

### 4.5 adapter

#### 4.5.1 httpapi

- 受け取るのは、ユースケースだけである。ログの出力先は、受け取らなくなる(DD-16)。
- `Actor` は、Cookie `chat_session` の値と、要求ヘッダー `X-Session-Id` の値を、そのまま渡す。なければ、空の文字列を渡す。送信元は、`RemoteAddr` のホストとする(DD-17)。
- 要求の本文の文字列は、どの項目も、置き換えずに復号する(既存の `decodeJSONString`。ADR-015)。大きさの上限は、DD-29 の通りとする。
- 送信の要求が、形式に合わない場合は、`Malformed` を立てて、ユースケースを呼ぶ(DD-18)。登録とログインの要求が、形式に合わない場合は、ユースケースを呼ばずに、`invalid_input` を返す。
- ログインの完了では、`LoginOutput.Credential` を、Cookie に載せる(属性は ADR-014。有効期限は、`ExpiresAt`)。応答の本文には、識別子と利用者だけを載せる。ログアウトの完了では、同じ名前と属性で、Cookie を消す。拒否とエラーの応答では、Cookie を変えない。
- 拒否の種類と `code` の対応に、3.4 を加える。`InvalidFields` は、`fields` に詰め替える。
- 本文のない応答(204)にも、`Cache-Control: no-store` を付ける。

#### 4.5.2 wsaccept

要求の処理の順を、次の通りに改めます。

1. 書き込み口を作り、`ConnectionKeeper.Accept` を呼ぶ。`Actor` は、Cookie の値と、クエリ `session` の値である。WebSocket の接続を確立する要求でなければ、拒否せずに、`Malformed` を立てる(DD-18)。`Unauthenticated` であれば、`unauthenticated`(401)を、`InvalidInput` であれば、`invalid_input`(400)を、接続を確立せずに返す。
2. 以後、どの経路で終わる場合も、`ConnectionKeeper.Close` を呼ぶ。
3. 以後は、これまで通りである。書き込み口が閉じられたとき、`CloseSessionEnded` によるものであれば、4401 で、そうでなければ、これまで通り 1008 で閉じる。

書き込み口は、`CloseSessionEnded` を加えます。閉じた理由を、印として持ちます。

ユースケースが返したエラーは、ログに出力しなくなります(ADR-021)。接続への書き込みの失敗と、閉じた理由は、これまで通り、出力します。ログの出力先は、受け取り続けます。

#### 4.5.3 wsdelivery

- 書き込み口ごとに、利用者、ログイン状態の識別子、期限を保持する。利用者からと、ログイン状態の識別子からの、2つの対応を持つ。状態は、これまで通り、このパッケージだけが持つ(RL-39)。
- `CloseBySession` は、そのログイン状態による書き込み口のすべてに、`CloseSessionEnded` を呼び、登録を解く。
- `CloseExpired` は、`ValidAt(now)` でない書き込み口に、同じことを行う。
- `Deliver` は、宛先のうち、`ValidAt(now)` でない書き込み口に、届けずに、同じことを行う。
- 有効かどうかは、保持したログイン状態の `ValidAt` で判定する。期限を、自分では比べない。ポートを実装する adapter が、domain の判定を呼ぶ例外である(ADR-020)。

#### 4.5.4 postgres

`UserDirectory`、`SessionStore`、`SignupStore` と、`Transactor.InSignupTransaction` を実装します。`SessionStore` と `SignupStore` は、同じ名前の操作(`Find`、`DeleteExpired`)を持つため、`Store` とは別の型(`Store.Sessions()`、`Store.Signups()` で得る)に実装します。

**構造**

```sql
-- 00002
CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    contact      text NOT NULL,
    handle       text NOT NULL,
    display_name text NOT NULL,
    secret_hash  text NOT NULL,
    CONSTRAINT users_contact_key UNIQUE (contact)
);
CREATE UNIQUE INDEX users_handle_key ON users (lower(handle));

CREATE TABLE sessions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_digest bytea       NOT NULL,
    user_id           uuid        NOT NULL REFERENCES users (id),
    expires_at        timestamptz NOT NULL,
    CONSTRAINT sessions_credential_digest_key UNIQUE (credential_digest)
);

CREATE TABLE signups (
    contact           text PRIMARY KEY,
    credential_digest bytea       NOT NULL,
    expires_at        timestamptz NOT NULL,
    CONSTRAINT signups_credential_digest_key UNIQUE (credential_digest)
);

-- 00003(切り替えの手順で加える。9 の手順6)
-- 固定ユーザーの会話とメッセージを除いてから(仕様「対象とするフローの範囲」)、
-- conversations.user_low、user_high と、messages.sender_id に、users (id) への外部キーを加える。
```

- 連絡先は、揃えた後の値を保管する。識別名は、入力された通りに保管し、一意性は、小文字にした値で保つ(RL-07)。
- 登録の日時など、機能に要らない項目は、持たない(PRV-1)。期限と、利用者からログイン状態を引くための索引は、想定規模では要らないため、置かない。

**ポートの操作と、問い合わせ**

| ポートの操作 | 問い合わせ |
|---|---|
| `FindByHandle` | `… FROM users WHERE lower(handle) = lower($1)` |
| `FindByContact` | `… FROM users WHERE contact = $1` |
| `SessionStore.Create` | `INSERT INTO sessions (credential_digest, user_id, expires_at) … RETURNING id` |
| `SessionStore.Find` | `… FROM sessions JOIN users … WHERE credential_digest = $1 AND expires_at > $2` |
| `SessionStore.Revoke` | `DELETE FROM sessions WHERE credential_digest = $1 RETURNING id` |
| `SignupStore.Put` | `INSERT INTO signups … ON CONFLICT (contact) DO UPDATE SET credential_digest = …, expires_at = …` |
| `SignupStore.Find` | `… FROM signups WHERE credential_digest = $1 AND expires_at > $2` |
| `DeleteExpired`(2つ) | `DELETE FROM … WHERE expires_at <= $1` |
| `ConsumeSignup` | `DELETE FROM signups WHERE credential_digest = $1 AND expires_at > $2 RETURNING contact, expires_at` |
| `CreateUser` | `INSERT INTO users … RETURNING id`。`users_handle_key` への違反を、`port.ErrHandleTaken` に、`users_contact_key` への違反を、`port.ErrContactTaken` に置き換える |

- 資格の要約は、資格の文字列の SHA-256 とする(crypto/sha256)。計算は、このパッケージの1つの関数にまとめ、ログイン状態と、登録の申し込みの、両方が使う(DD-10)。
- 無効にすることと、使用済みにすることは、行の削除で表す。無効にした行を、残さない。
- 同じ資格での完了が、同時に行われた場合、後の `DELETE` は、先のトランザクションの確定を待ち、行がないことを得る(RL-05)。同じ識別名での完了は、後の `INSERT` が、一意性の違反になる(RL-07)。分離の水準は、既定(READ COMMITTED)とする。
- エラーの作り直し(`OpError`)と、復元用の関数だけを使うことは、これまで通りとする。

#### 4.5.5 argon2id

- `SecretHasher` を実装する。パラメータ、ソルト、保管する値の形、比較の手段は、ADR-015 の通りとする。
- 同時に行う計算は、4つまでとする(DD-11)。超える分は、順番を待つ。待つ間に、要求が取り消されたら、やめる。
- `VerifyDummy` は、固定の入力に対して、現在のパラメータの計算を、1回行う。
- 4は、想定規模のログインの重なりに足り(1回が約25ミリ秒であるため、1秒に約160回)、試行を重ねる要求が使うメモリを、約76MiB に限る値である。

#### 4.5.6 commonsecrets

- `CommonSecretList` を実装する。`list.txt` を、実行ファイルに含め(embed)、作るときに、集合に読み込む。照合は、完全な一致で行う(ADR-017)。
- 同じ置き場所の `NOTICE.md` に、出所(SecLists のコミットとファイル)、取り出した条件、ライセンスを記載する。
- パッケージのテストが、一覧の件数(5,392)、各行の長さ(15文字以上、128文字以下)、`@` を含まないこと、ファイルの要約を確かめる。手での書き換えを、検出する。

#### 4.5.7 smtpmail

- `NoticeSender` を実装する。受け取るのは、接続先、送信元のアドレス、アプリのオリジンである。
- 接続から、引き渡しの完了までを、10秒までとする(DD-13)。暗号化と認証は、行わない(ADR-018)。
- 本文は、UTF-8 の、書式を持たないテキストとする。件名と本文は、標準ライブラリで符号化する。
- 返すエラーは、操作の名前と、SMTP の応答の番号だけを持つ形に、作り直す。相手が返した文言は、宛先を含みうるためである(RL-33)。

| 案内 | 件名 | 本文 |
|---|---|---|
| 登録を続けるための案内 | 登録の確認 | 登録の申し込みを受け付けたこと。登録を続けるためのアドレス(`<アプリのオリジン>/#signup=<資格>`)。有効期限(申し込みから24時間であることと、UTC の日時)。心当たりがない場合は、何もしなくてよいこと |
| 登録済みであることを伝える案内 | 登録の申し込みについて | この連絡先で、登録の申し込みがあったこと。この連絡先は、登録済みであること。心当たりがない場合は、何もしなくてよいこと |

訪問者が入力した内容は、宛先のほかに、含めません(RL-11)。宛先は、`Contact`(揃えた後の値)です。

#### 4.5.8 ratelimit

`AttemptLimiter` を実装します。状態は、種類と、数える対象の組ごとの、数えた時刻の並びと、拒否を解く時刻です。排他の制御のもとで、読み書きします。

- `Admit` は、まず、すべての `Limit` について、`now` から、その時間より前の時刻を捨て、達しているかを確かめる。「T に N 回」は、残った数が N 以上であること、「連続して N 回」は、`now` が、拒否を解く時刻より前であることで、判定する。
- どれも達していなければ、それぞれに、`now` を加える。「連続して N 回」で、数が N になった場合は、拒否を解く時刻を、`now` + T とし、時刻の並びを空にする(拒否の間が過ぎると、0から数える)。
- `Forgive` は、最も新しい1つを除く。`Reset` は、その組の状態を除く。
- `Admit` は、前に掃いてから1分が過ぎていれば、すべての組を見て、時刻の並びが空で、拒否の間も過ぎた組を除く。別の goroutine は、動かさない。
- 計数は、サーバーごとであり、再起動で失われる(ADR-016)。

#### 4.5.9 system、eventlog、schedule

- `system` は、`Clock`(サーバーの時刻。保管先の精度に合わせて、マイクロ秒に丸める)と、`CredentialSource`(crypto/rand の32バイトを、base64url にする。RL-35)を実装する。
- `eventlog` は、`EventRecorder` を実装する。4.4.8 の項目を、`log/slog` で出力する。
- `schedule` は、`Housekeeper.Sweep` を、起動の直後と、以後1分ごとに呼ぶ。終了の合図で、止まる。間隔は、設定にしない。

### 4.6 main(`cmd/server`)

**設定(環境変数)**

| 変数 | 内容 |
|---|---|
| `SMTP_ADDR` | SMTP のサーバーの接続先(`ホスト:ポート`)。ない場合と、ホストが、ループバックのアドレスの表記でない場合、起動に失敗する。判定は、待ち受け先と同じ関数(`validateLoopback`)で行う |
| `MAIL_FROM` | 案内の送信元のアドレス。ない場合と、アドレスとして読めない場合、起動に失敗する |
| `LIMIT_SIGNUP_REQUEST_PER_SOURCE`、`LIMIT_SIGNUP_REQUEST_PER_CONTACT`、`LIMIT_SIGNUP_COMPLETION_PER_SOURCE`、`LIMIT_LOGIN_FAILURE_PER_CONTACT`、`LIMIT_LOGIN_FAILURE_PER_SOURCE`、`LIMIT_PEER_LOOKUP_PER_USER` | 上限(仕様「上限」の表の、上から順)。値は、`回数/時間`(時間は、`15m`、`1h` などの形)で書く。ない場合は、既定値で働く。読めない場合と、回数か時間が、正の値でない場合、起動に失敗する(RL-28) |

`LISTEN_ADDR` の制限は、残します。`validateLoopback` の、仮実装のコメントだけを除き、理由を、SEC-2 のためと書き直します(RL-D1、RL-42)。

**組み立て**

- 組み立ての関数(`newHandler`)が受け取る、ポートの実装に、加えたポートを加える。`Housekeeper` は、同じ実装から組み立て、`schedule` に渡す。テストは、`Sweep` を、直接に呼ぶ。
- ユースケースを呼ぶ adapter(`httpapi`、`wsaccept`、`schedule`)には、ユースケースだけを渡す。
- `wsdelivery` の `Hub` は、`MessageDelivery`、`ConnectionRegistry`、`SessionConnections` の実装として渡す。

## 5. クライアント

### 5.1 状態と操作

```ts
type UserProfile = { handle: string; displayName: string }   // FixedUser を置き換える
type InputField = 'contact' | 'handle' | 'displayName' | 'secret'
type FormError = { code: RejectionCode | 'failed'; fields: InputField[] }

type SessionState =
  | { kind: 'checking' }                                     // ログイン状態を確かめている
  | { kind: 'unknown' }                                      // 確かめられなかった。やり直せる
  | { kind: 'signedOut'
      screen: 'login' | 'signupRequest' | 'signupComplete'
      pending: boolean
      error: FormError | null
      notice: 'sessionEnded' | 'signupRequested' | 'signupCompleted' | null }
  | { kind: 'signedIn'; user: UserProfile; logout: 'idle' | 'pending' | 'failed' }

type ChatState = {
  session: SessionState
  peerSearch: { pending: boolean; found: UserProfile | null; error: RejectionCode | 'failed' | null }
  conversation: Conversation | null   // peer は UserProfile。履歴の取得が拒否された理由を加える
}
```

- `RejectionCode` に、`invalid_fields`、`handle_taken`、`invalid_signup_credential`、`login_failed`、`rate_limited` を加える。
- ログイン状態の識別子と、案内の資格は、`ChatApp` が、状態の外に持つ。ui には、渡さない。
- `ChatState.users`、`FixedUser`、`selectUser`、`leaveUser` は、除く。

**操作**: `start()`、`showLogin()`、`showSignupRequest()`、`requestSignup(contact)`、`completeSignup(handle, displayName, secret)`、`login(contact, secret)`、`logout()`、`findPeer(handle)`、`openFoundPeer()`、`retryHistory()`、`setDraft(text)`、`send()`、`resend(sendId)`。

### 5.2 規則

`chatApp.ts` の先頭の一覧を、次の通りに改めます。C-2〜C-12 は、「利用者」を、ログイン中の利用者と読み替えて、そのまま残します。

| # | 規則 | 基準 |
|---|---|---|
| C-1(改める) | ログイン状態の終わりでは、接続を閉じ、会話、相手の特定の結果、入力中の本文、ログイン状態の識別子を捨て、ログインを求める画面に切り替える。それより前の、接続、履歴の取得、送信、相手の特定に対して、後から届いた結果は、すべて捨てる | RL-19、RL-41 |
| C-13 | `start` は、案内の資格を読み取る。あれば、登録を続ける画面を示す(DD-34)。なければ、ログイン状態を確かめ、ログイン中であれば、利用者と識別子を保持して、接続を確立する。ログインしていなければ、ログインの画面を示す。確かめられなければ、そのことを示し、やり直せるようにする(ログインしていないとは、示さない) | RL-21、RL-23、RL-D9 |
| C-14 | どの操作でも、認証の拒否の応答を受けたときと、接続が、ログイン状態が無効になったことを示して閉じられたときに、C-1 を行い、ログインし直しが要ることを示す | RL-19、RL-20 |
| C-15 | `login` は、完了で、利用者と識別子を保持し、接続を確立する。拒否(照合の失敗、上限)と、完了できなかった場合は、理由を示し、ログインの画面に留まる | RL-12、RL-13 |
| C-16 | `logout` は、完了で、C-1 を行う。認証の拒否は、C-14 による。完了できなかった場合は、できなかったことを示し、ログイン中のままとする | RL-17、RL-22 |
| C-17 | `requestSignup` は、完了で、案内を送ったことを示す。`completeSignup` は、完了で、案内の資格を捨て、ログインの画面に、登録を完了したことを示す。拒否では、理由と、正しくない項目を示し、同じ画面に留まる。資格が無効である拒否では、申し込み直しを求める | RL-01、RL-05〜RL-07 |
| C-18 | `findPeer` は、結果(識別名と表示名)か、拒否の理由を示す。`openFoundPeer` は、特定した相手との会話を、C-2 の通りに開く。履歴の取得が拒否された場合は、理由を示す | RL-24〜RL-26、RL-31 |
| C-19 | `login`、`requestSignup`、`completeSignup` の結果を待つ間は、`showLogin` と `showSignupRequest` は、何もしない(画面を切り替えない)。切り替えると、サーバーで完了した操作の結果(ログイン、登録の完了)を、示せなくなるためである。ui は、待つ間、画面を切り替えるボタンを、無効にする(実装のレビューで加えた) | ― |

- C-12 の「同じ相手を、もう一度指定された場合」は、`retryHistory` と、同じ相手を特定し直して開いた場合に当たる。
- クライアントは、送信の前に、入力を検証しない(DD-01)。送信のボタンは、入力欄が空のときだけ、無効にする。

### 5.3 ポートと adapter

| ポート | 変更 | 実装 |
|---|---|---|
| `AuthApi`(加える) | ログイン状態を確かめる、ログイン、ログアウト、登録の申し込み、登録の完了 | `httpAuthApi.ts`(`fetch`) |
| `ChatApi` | 相手の特定を加える。利用者の識別名の代わりに、ログイン状態の識別子を受ける。結果に、認証の拒否と、拒否の理由を加える | `httpChatApi.ts`。識別子を、`X-Session-Id` で添える |
| `RealtimeChannel` | 識別子を受ける。切断の通知に、ログイン状態が無効になったことによるものかどうかを加える | `webSocketRealtime.ts`。識別子を、クエリ `session` で添える。終了コード 4401 を見分ける |
| `EntryLink`(加える) | 案内の資格を、アドレスから読み取り、アドレスから除く | `locationEntryLink.ts`(`location.hash`、`history.replaceState`) |
| `FixedUserDirectory` | 除く | `staticFixedUsers.ts` を除く |

- 資格は、ブラウザが、Cookie として保ち、要求に付ける。クライアントのコードは、触れない(ADR-014)。`fetch` は、既定で、同じオリジンの要求に、Cookie を付ける。
- `httpAuthApi.ts` は、結果を、完了、拒否(`code` と `fields`)、完了できなかった(要求が失敗した、5xx、形式に合わない応答)に分けて返す。

### 5.4 ui(DD-01)

| 部品 | 内容 |
|---|---|
| ログイン(`LoginForm`) | 連絡先と秘密の入力欄、「ログイン」ボタン、登録の申し込みへの切り替え。失敗の理由は、区別せずに示す(RL-13)。ログインし直しが要ることと、登録を完了したことの知らせを示す |
| 登録の申し込み(`SignupRequestForm`) | 連絡先の入力欄、「申し込む」ボタン。完了で、案内を送ったことを示す |
| 登録の完了(`SignupCompleteForm`) | 識別名、表示名、秘密の入力欄、「登録する」ボタン。項目ごとに、規則を示す。拒否では、正しくない項目を示し、入力を保つ |
| ヘッダー(`Header`) | ログイン中の利用者の表示名と識別名、「ログアウト」ボタン。ログアウトできなかったことを示す |
| 相手の特定(`PeerFinder`) | 識別名の入力欄と「探す」ボタン。結果(識別名と表示名)と、「会話を開く」ボタン。`PeerPicker` を置き換える |
| 会話、メッセージ、入力欄 | これまで通り。会話に、相手の識別名を併せて示す(RL-D6) |

- `UserPicker` と `PeerPicker` は、除く。
- 秘密の入力欄は、内容を伏せる形(`type="password"`)とし、貼り付けと、ブラウザの保存の仕組みを、妨げない(`autocomplete` を、ログインでは `current-password`、登録では `new-password` とする)。
- 表示名と識別名は、React の文字列として描画する(RL-41 の WS-27)。
- 拒否の理由の文言(`rejectionText.ts`)に、加えた `code` を加える。秘密が正しくない場合は、条件の全体(長さ、よく使われるもの、連絡先・識別名と同じもの)を示す。どの条件に当たったかは、区別しない(DD-19)。

### 5.5 開発用のサーバーのログ(DD-36)

`vite.config.ts` に、ログの出力を包む関数を与え、記録の中の、`/api/` で始まるパス(クエリを含む)を、`/api/…` に置き換えます。中継のエラーの記録(`http proxy error: <パス>`)の文言には、依りません。Vite の版を上げても、働き続けるようにするためです。

関数は、`client/tools/` に置き、テストします(Vitest と、型の検査の対象に、この置き場所を加えます)。`client/tools/` は、`vite.config.ts` と同じく、開発の道具の設定であり、`src` を import しません。層の規則の検査(dependency-cruiser と ESLint。対象は、`src`)には、当たりません。WebSocket の中継のエラーの記録は、パスを含みません(Vite 8.3.8 のコードで、確認済み)。

## 6. ローカルの開発環境

### 6.1 Mailpit と設定

- `compose.yaml` に、Mailpit を加える。イメージ、公開するアドレス、無効にする機能は、ADR-018 の通りとする。公開するポートは、`127.0.0.1:${MAILPIT_SMTP_PORT:?}:1025` と、`127.0.0.1:${MAILPIT_UI_PORT:?}:8025` とする。
- `make db-up` と `make db-down` が、保管先とともに、Mailpit を起動し、止める。手順の名前は、変えない。
- `.env.example` に、次を加える。どれも、秘密ではない。`SMTP_ADDR=127.0.0.1:1025`、`MAIL_FROM`、`MAILPIT_SMTP_PORT=1025`、`MAILPIT_UI_PORT=8025`。`LIMIT_…` は、注釈として挙げる。
- 手順書に、Mailpit の画面を `http://127.0.0.1:8025` で開くこと(ADR-018)と、加えた設定を記載する。

### 6.2 よく使われる秘密の一覧を取り出す手順(DD-12)

`scripts/extract-common-secrets.sh` が、固定したコミットのファイルを取得し、`scripts/extract-common-secrets.go`(標準ライブラリだけを使う)が、ADR-017 の条件で取り出して、`list.txt` に書きます。`make common-secrets` で実行します。`make check` には、含めません(外部への到達を要するため)。取り出した結果が、ADR-017 の件数と一致することを、実装の中で確かめます。

## 7. テスト

### 7.1 道具立て

| 対象 | 手段 |
|---|---|
| usecase のテスト | 加えたポートに、テスト用の実装を加える(`fakes_test.go`)。時刻は、テストから進める。上限は、結果を指定できる実装にする(数え方は、`ratelimit` のテストで確かめる) |
| サーバーの振る舞いのテスト | 実際の adapter で組み立てる。差し替えるのは、`Clock`(テストから進める)と、`NoticeSender`(送った案内を、覚える)である。秘密の計算は、実際に行う(ADR-015)。上限は、既定では、当たらない値にし、上限のテストだけが、小さな値を与える |
| 同上の補助 | 利用者を登録する(申し込み、覚えた案内の資格での完了)。ログインして、Cookie と識別子を持つ端末を得る。片付けを呼ぶ。識別子を指定して、利用者の行を直接に作る(RL-41 の WS-37 が、識別子の順と、識別名の順が異なる組を要するため) |
| SMTP を通るテスト | `smtpmail` のテストが、Mailpit へ送り、Mailpit の API で、宛先を指定して読む。宛先は、テストごとに変える(ADR-018) |
| クライアントのテスト | `testPorts.ts` に、加えたポートの、テスト用の実装を加える |

- 時刻は、マイクロ秒の精度で進める(保管先の精度)。
- 「配信されない」ことは、これまで通り、番兵のメッセージで判定する。
- 秘密の計算によって、サーバーのテストの所要時間とメモリが、どう変わるかは、実装の中で確かめる(ADR-015 の Consequences)。

### 7.2 受け入れ基準との対応

「振る舞い」は、サーバーの振る舞いのテスト(`cmd/server`)を指します。

| 基準 | 確かめ方 |
|---|---|
| RL-01 | 振る舞い: 申し込み、案内の資格での完了、ログイン。`smtpmail`: 案内が、Mailpit に1通届く。E2E |
| RL-02 | 振る舞い: 完了の前は、ログインできず、特定されない。入力の規則で拒否された識別名で、別の人が完了できる |
| RL-03 | 振る舞い: 2つの申し込みの応答(状態と本文)が一致する。利用者とログイン状態の行が変わらない。覚えた案内が、資格を含まない |
| RL-04 | 振る舞い: 再度の申し込みの後、以前の資格は拒否され、新しい資格で完了できる |
| RL-05 | 振る舞い: 期限の直前と、期限。使用済み。正しくない資格。応答が一致し、利用者の行が増えない。同じ資格での、並行の完了が、1つだけ完了する。`postgres`: 連絡先の一意性の違反の置き換え。usecase: 連絡先が登録済みであった完了を、資格が無効な場合として拒否する(DD-38) |
| RL-06 | domain: 各規則の、境界の値。`commonsecrets`: 一覧にある値と、ない値。振る舞い: 代表の値と、`fields` が、正しくない項目を、すべて挙げること。拒否の後、同じ資格で完了できる |
| RL-07 | 振る舞い: 大文字と小文字だけが異なる識別名。同じ資格で、やり直せる。同じ識別名での、並行の完了。`postgres`: 一意性の違反の置き換え |
| RL-08 | 振る舞い: 無効な資格での完了の応答が、使われている識別名と、使われていない識別名で、一致する |
| RL-09 | 振る舞い: 期限まで時刻を進め、片付けを呼ぶと、申し込みの行がない(保管先を、直接に調べる)。期限の前は、残る |
| RL-10 | 振る舞い: 案内のポートを、失敗する実装にする。応答は `internal`。`signup_notice_failed` が記録される。登録済みの連絡先でも、同じ応答になる |
| RL-11 | `smtpmail`: 2つの案内の本文が、4.5.7 の内容である。大文字と空白を含めて入力した連絡先が、宛先のほかに現れない(振る舞いで、ポートに渡る値も確かめる) |
| RL-12 | 振る舞い: 大文字と空白を違えた連絡先でログインできる。128文字の秘密と、末尾の1文字が異なる秘密 |
| RL-13 | 振る舞い: 4つの場合の応答が一致する。usecase: どの失敗の経路も、秘密の計算を、ちょうど1回呼ぶ。設計の確認: 4.4.4 |
| RL-14 | 振る舞い: 別の Cookie でのログインの後、元の Cookie が有効である。資格が、ログインのたびに異なる。Cookie を付けたログインの後、前の資格が使えない |
| RL-15 | 振る舞い: 資格がない、正しくない、期限が切れた、ログアウトした、識別子が合わない、の要求が、ログアウト、相手の特定、履歴、送信、接続の確立で、同じ応答になる(送信と、接続の確立は、形式に合わない要求を含む)。`getSession` は、どれにも、同じ応答で、ログインしていないと示し、`authentication_rejected` を記録しない |
| RL-16 | 振る舞い: 期限の直前と、期限。操作を続けても、延びない |
| RL-17 | 振る舞い: ログアウトの後、同じ Cookie の操作と接続が拒否され、別のログイン状態は残る。動作検証 |
| RL-18 | 振る舞い: ログアウトの後と、期限の後(片付けの前を含む)に保管されたメッセージが、その接続に届かない(番兵、または、4401 での終了)。`wsdelivery`: `CloseBySession`、`CloseExpired`、期限を過ぎた接続への `Deliver`(期限の直前と、期限) |
| RL-19 | 振る舞い: ログアウトと、期限の後の片付けで、接続が 4401 で閉じられる。application: C-1、C-14。ui: ログインの画面に切り替わり、会話が示されない |
| RL-20 | 振る舞い: 同じ Cookie でログインし直すと、前のログイン状態の接続が、4401 で閉じられる。前の識別子と、新しい Cookie の組の操作が、拒否される。application: C-14。E2E |
| RL-21 | 振る舞い: 同じ保管先で、サーバーを組み立て直した後、同じ Cookie で操作でき、申し込みを完了できる。application: C-13。動作検証 |
| RL-22 | application: C-16。ui: できなかったことが示され、ログイン中のままである |
| RL-23 | 振る舞い: ログイン状態を保持するポートを、失敗する実装で包む。各操作が `internal` になり、行われない。`getSession` も `internal` になる。application: C-13 |
| RL-24 | 振る舞い: 大文字と小文字を違えた特定が、登録した通りの識別名を返す。ui、E2E |
| RL-25 | 振る舞い: 居ない識別名、識別名の一部、完了していない人が入力した識別名の応答が、一致する |
| RL-26 | 振る舞い: 自分の識別名(大文字と小文字を違えたものを含む) |
| RL-27 | 振る舞い: 特定の応答と、`Message` が、識別名と表示名のほかの、利用者の項目を持たない。通信の形式の確認: 3.1、3.3 |
| RL-28 | `ratelimit`: 2つの数え方の境界と、時間の経過。振る舞い: 仕様「上限」の表の行ごとに、小さな値を与えて確かめる。`cmd/server` の設定のテスト: 値の読み取り、既定値、正の値でない設定 |
| RL-29 | 振る舞い: 登録されていない連絡先でも、同じ応答になる。拒否の間は、正しい秘密でも拒否される。既に与えた Cookie は、使える。数える期間より前の失敗。規則を満たさない連絡先は、連絡先ごとに数えない(usecase)。動作検証 |
| RL-30 | 振る舞い: 登録済みの連絡先と、登録されていない連絡先 |
| RL-31 | usecase: `peerResolver` の順と、数えるもの。振る舞い: 3つの操作が、同じ計数を使う。上限の間、居る相手と、居ない相手の応答が一致する。会話がある相手は、受け付けられる |
| RL-32 | 振る舞い: 4.4.8 の出来事ごとに、ログの出力に、種類、送信元、利用者の識別子(定めた場合)がある。usecase: 公開するメソッドごとに、ポートを失敗させ、`operation_failed` が、1回だけ記録される(DD-39) |
| RL-33 | 振る舞い: 目印の文字列を、秘密、連絡先、識別名、表示名に含め、資格も併せて、完了、拒否、エラーのそれぞれで、ログの出力に現れないことを確かめる。拒否とエラーの応答と、ログインの応答の本文に、秘密と資格が現れない。`smtpmail`: 返すエラーが、宛先を含まない。`client/tools`: 5.5 の関数。秘密の型を、`%v`、`%#v`、`log/slog` の JSON で出力しても、値が現れない(DD-21。`cmd/server` に置く。domain のテストは、書式とログの標準ライブラリを import できない) |
| RL-34 | `argon2id`: 同じ秘密の、2つの値が異なる。保管する値の形。振る舞い: 利用者の行が、秘密を含まない。ADRの確認: ADR-015 |
| RL-35 | 設計の確認: 4.5.9。`system`: 資格の長さと、2つの値が異なること |
| RL-36 | 文書の確認: `docs/personal-data.md` |
| RL-37 | ADRの確認: ADR-014〜018。設定の確認: `compose.yaml`、`.env.example` |
| RL-38 | 設定のテスト: 待ち受け先と、`SMTP_ADDR` が、ループバックのアドレスでない場合。設定の確認: `compose.yaml` の、公開するアドレス |
| RL-39 | `make check-deps`。状態が、`wsdelivery` と `ratelimit` だけにあることを、アーキテクチャ概要に記載する |
| RL-40 | ADRの確認: ADR-014〜018 の「選定の条件の確認」 |
| RL-41 | ウォーキングスケルトンのテストを、登録してログインした利用者に改めて、残す。表示名に、WS-27、WS-28 のテストを加える。加えた操作に、WS-29(他のオリジン)、WS-30(想定しない入力)のテストを加える。`go mod verify`(WS-32) |
| RL-42 | 検査: `grep -rn '仮実装(WS-P' api server client/src` が、WS-P4 だけを挙げる。振る舞い: `X-Fixed-User` と、クエリ `user` を添えただけの要求が、拒否される |

### 7.3 E2Eテスト

- 対象は、動作検証の手順1〜4、7、9とする。案内は、Mailpit の API で読む。
- テストごとに、始める前に、利用者、ログイン状態、登録の申し込み、会話、メッセージの表を、空にする。
- E2E用のサーバーには、当たらない値の上限を、環境変数で与える。計数は、サーバーのメモリにあり、テストの間で、戻せないためである。
- 手順7と9の「同じブラウザのタブ」は、1つのコンテキストの、2つのページで表す。

## 8. 仮実装の置き換え(RL-42)

| ID | 除く・改める箇所 |
|---|---|
| WS-P1 | `adapter/fixeduser` を除き、`postgres` の実装に置き換える。`port.go` の注釈を除く。マイグレーション 00001 の注釈は、00003 で外部キーを加えたうえで、除く(「レビューで確かめたい事項」6)。クライアントの `staticFixedUsers.ts`、`fixedUserDirectory.ts`、`FixedUser`、`ChatState.users` を除く |
| WS-P2 | 通信の形式の `securitySchemes.fixedUser` と、説明。`httpapi` の `fixedUserHeader`、`actorOf`。`wsaccept` の `fixedUserQuery`。クライアントの `httpChatApi.ts`、`webSocketRealtime.ts` の、利用者を添える箇所。`UserPicker.tsx` と、`Header.tsx` の「利用者を選び直す」。`validateLoopback` は、コードを残し、注釈と、一覧の行だけを除く(RL-D1) |
| WS-P3 | `PeerPicker.tsx` を、`PeerFinder` に置き換える |

置き換えたときに、変更が及んだ層と、ファイルの数を、観察記録に残します(ADR-008)。WS-P4 は、残ります。

## 9. 実装の順序

各手順は、`make check` のうち、その時点で実行できるものが通る状態で終えます。手順1〜5は、既存の操作を変えずに、内側の層と、ポートの実装を加えます。手順6で、通信の形式と、既存の操作を切り替えます(DD-37)。観察記録に当たる出来事は、その都度、追記します。

| # | 手順 | 終えたときの状態 |
|---|---|---|
| 0 | 環境の確認(下記) | 成り立たない場合は、本書か、ADRを改める |
| 1 | 土台。`compose.yaml`、`.env.example`、設定の読み込み(`SMTP_ADDR`、`MAIL_FROM`、`LIMIT_…`)。golang.org/x/crypto の導入と、版の固定 | 設定のテストが通る。`make db-up` で、Mailpit が起動する |
| 2 | domain(4.2 の、加える型。`User` は、まだ変えない) | domain のテストが通る |
| 3 | ポート(加える11と、`UserDirectory.FindByContact`、`Transactor.InSignupTransaction`)と、加えるユースケース(登録の申し込みと完了、ログイン、ログイン状態、片付け、`SessionGate`)。既存の実装(`fixeduser`、`postgres`)は、ポートに加えた操作を、コンパイルが通る形で満たす | usecase のテストが通る。既存の操作は、変わらない |
| 4 | 保管先。マイグレーション 00002、問い合わせ、`postgres` の実装 | `postgres` のテストが通る |
| 5 | 加える adapter(`argon2id`、`commonsecrets` と一覧、`smtpmail`、`ratelimit`、`system`、`eventlog`、`schedule`)。検査の設定と、規則に違反する見本 | 各パッケージのテスト、`make check-deps`、`make check-deps-selftest` が通る |
| 6 | 切り替え。通信の形式(操作を加え、仮のログインを除く)。既存のユースケースの入力と、ポート(4.3 の、変えるもの)。相手の特定(`PeerFinder`、`peerResolver`)。`User` の型。`httpapi`、`wsaccept`、`wsdelivery`、main の組み立て。`fixeduser` を除く。マイグレーション 00003。振る舞いのテストの補助を改め、既存のテストを直し、加えた基準のテストを置く | サーバーの `make check` が通る。クライアントは、まだ、サーバーにつながらない |
| 7 | クライアントの application と、ポート。ui と adapter は、型の検査が通る形に留める | application のテストと、型の検査が通る |
| 8 | クライアントの adapter、ui、main。`client/tools/` | `make check` が通る。ブラウザで、登録からやり取りまで行える |
| 9 | E2Eテスト | `make e2e` が通る |
| 10 | 完了時に行うこと(10) | 完了条件を満たす |

手順6は、すべての層に及びます。既存のユースケースの入力と、ポートを変えると、それを呼ぶ adapter と、実装する adapter が、同時に変わるためです。手順3〜5で、加えるものを、テストとともに先に作り、手順6で変える量を、小さくします。

**同じ手順の中で直す、文書と指示**

アーキテクチャ概要、手順書、置き場所ごとの指示は、当たる手順の中で直します(`.claude/rules/docs.md`「文書の種類と、手の入れ方」、`server.md`「パッケージを加えるとき」)。完了時まで、送りません。送ると、実装の間、古い指示が読み込まれ、手順書の通りに、起動できなくなります(手順1で、`SMTP_ADDR`、`MAIL_FROM`、`MAILPIT_SMTP_PORT`、`MAILPIT_UI_PORT` が、必須になる)。

| # | 直すもの |
|---|---|
| 1 | 手順書(加えた設定、Mailpit の起動と画面、既存の `.env` に書き足す値)。アーキテクチャ概要「実行環境の構成」 |
| 2 | アーキテクチャ概要「サーバーの層」(domain に置くもの) |
| 3 | アーキテクチャ概要(ポートの表、ユースケース)。`server.md`(記録は、ポートを通す。公開するメソッドは、`attempt` を通す。ADR-021) |
| 4 | アーキテクチャ概要「保管先」 |
| 5 | アーキテクチャ概要(ディレクトリ、パッケージと import してよい先の表、状態の置き場所)。`server.md`(adapter の役割の表。エラーを作り直す規約を、加えた adapter にも適用すること)。手順書(一覧を取り出す手順、ライセンスの確認) |
| 6 | アーキテクチャ概要(経路、ポートの表、仮のログインについての記述)。`server.md`(`fixeduser` の記述、拒否の応答の `fields`、資格と秘密を、出力とエラーに含めない規約、規則を確かめる関数と復元用の関数の列挙、`ValidAt` を呼ぶ例外〈ADR-020〉)。`contract-and-codegen.md`(`ErrorResponse`、拒否の順)。仮実装の箇所の一覧(サーバーと、通信の形式の箇所)。手順書の冒頭の注意と、`CLAUDE.md`「変えてはならない前提」の1つ目(下記) |
| 7 | アーキテクチャ概要「クライアントの層」(ポート、振る舞い)。`client.md`(ポートの名前) |
| 8 | アーキテクチャ概要(クライアントのディレクトリ)。`client.md`(ファイルの名前、入力欄の内容を、ui の部品が持つこと〈DD-35〉、`client/tools/`)。仮実装の箇所の一覧(クライアントの箇所) |
| 9 | 手順書(E2Eテストが、Mailpit を要すること) |

`CLAUDE.md`「変えてはならない前提」の1つ目と、手順書の冒頭の注意は、仮のログイン(WS-P2)を、理由にしています。手順6で、仮のログインがなくなるため、次の内容に改めます(RL-D1、SEC-2)。通信経路の暗号化の構成ができるまで(設置環境を扱うスライス)、サーバーを、ローカルの開発環境の外に置かない。待ち受け先と、SMTP の接続先を、ループバックのアドレスの外に広げない。

**手順0で確かめること**(2026-11-27 の時点で、確かめていない)

- Mailpit(v1.34.0)を、ADR-018 の構成で動かせること。Go の net/smtp で引き渡せること。API で、宛先を指定して読めること。新しい版の確認を無効にする設定の名前。起動の確認(`docker compose up --wait`)が働くこと。
- ブラウザで、`http://localhost:5173` から、クライアントの開発用のサーバーの中継を通して、Cookie が与えられ、HTTP の操作と、WebSocket の接続を確立する要求の両方に付くこと。終了コード 4401 が、中継を通って、ブラウザに届くこと。
- 一覧のファイルを含めたリポジトリの履歴に対して、`make secrets-scan` が通ること(ADR-017 の Consequences)。

## 10. 完了時に行うこと

仕様「マイルストーンの完了条件との対応」に対応します。

- アーキテクチャ概要、手順書、置き場所ごとの指示、仮実装の箇所の一覧、`CLAUDE.md`「変えてはならない前提」が、9 の各手順で直した通りに、現状と一致していることを確かめる。アーキテクチャ概要の「最終更新」と「現在の範囲」を改める。
- 取得する個人データの一覧(`docs/personal-data.md`)を作る(RL-36)。項目、用途、保管先を載せる。保管先には、保管データのほか、サーバーのメモリ(上限の計数)、ログ(利用者の識別子、送信元)、ローカルの開発環境の Mailpit を含める。`CLAUDE.md` と手順書の、文書の表に加える。
- 仮実装の箇所の一覧と、コードのコメントが、WS-P4 だけを挙げることを確かめる(RL-42)。
- 手順書の、動作検証の手順を、仕様の手順1〜11に改める。
- 動作検証(仕様の手順1〜11)を行う。
- 観察記録に、仮実装の置き換えの記録と、スライスの記録を追記し、検査の記録と突き合わせる。
- ADR-014〜021 を、frozen にする。ADR-013 は、frozen のまま、ADR-014 が置き換える。
- `CLAUDE.md` の「現在地」を更新する。
- 本書を削除する。
