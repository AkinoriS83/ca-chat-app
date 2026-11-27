---
paths:
  - "server/**"
---

# サーバー(Go)の指示

現在の姿(パッケージ、ポート、経路)は、[アーキテクチャ概要](../../docs/architecture.md)「サーバーの層」にあります。理由は、[ADR-005](../../docs/adr/ADR-005-layer-structure.md)(層と規則)、[ADR-012](../../docs/adr/ADR-012-transaction-boundary.md)(トランザクション)、[ADR-019](../../docs/adr/ADR-019-login-state-verification-layer.md)(ログイン状態を確かめる層)、[ADR-020](../../docs/adr/ADR-020-domain-judgment-in-port-adapter.md)(adapter が呼んでよい domain の判定)、[ADR-021](../../docs/adr/ADR-021-event-recording-layer.md)(出来事の記録を行う層)にあります。本書は、コードを書くときと、レビューするときの判断と、import の検査では検出できない規約を挙げます。

## 層と、置くもの

依存は、外側から内側へだけ向きます。パッケージごとの「import してよい先」は、アーキテクチャ概要「パッケージと、import してよい先」の表と、`server/.golangci.deps.yml` にあります(本書には、写しません)。列挙にない import は、違反です。

| 層 | 置き場所 | 置くもの |
|---|---|---|
| domain | `internal/domain` | 利用者、会話、メッセージなどの型と、その規則(値の検証、判定) |
| application | `internal/application/port` | ポート(内側が定義し、adapter が実装するインターフェイス) |
| application | `internal/application/usecase` | ユースケース(操作の手順、規則を適用する順、拒否の判断)。入力・出力のデータ構造。拒否の種類 |
| adapter | `internal/adapter/<相手と役割>` | 外部の形式と、内側の形式の間の変換。入出力 |
| main | `cmd/server` | 設定の読み込み、組み立て、起動。サーバーの振る舞いのテスト |

置き場所は、次の順に考えます。

1. 外部の相手(HTTP、WebSocket、保管先、メールなど)の形式や、ライブラリを知っている必要があるか。あれば、adapter。
2. 1つの操作の手順(確かめる順、複数のポートの呼び出し、拒否の種類への置き換え)か。そうであれば、usecase。
3. 操作に依らず成り立つ、値や関係の規則か。そうであれば、domain。
4. 迷ったとき、どこにも収まらないときは、置く前に、開発者に相談する。観察記録にも当たる。

## domain

- 規則を通してしか作れない型(`Body`、`SendID`、`DirectPair`、`Contact`、`Handle`、`DisplayName`、`Secret`)は、中の値を、パッケージの外から直接には作れない形にする。
- 作る関数は、2種類ある。規則を確かめる関数(`NewBody`、`ParseSendID`、`NewDirectPair`、`NewContact`、`NewHandle`、`NewDisplayName`、`NewSecret`)と、保管した値から、検証せずに組み立てる復元用の関数(`RestoreBody`、`RestoreSendID`、`RestoreContact`、`RestoreHandle`、`RestoreDisplayName`)である。復元用の関数は、ポートを実装する adapter が、保管した値から組み立てる必要が、現にある型にだけ置く。テストのためだけには、加えない(OBS-016)。`Secret` には、置かない(保管するのは、`SecretHash` である)。
- 秘密の型(`Secret`)は、文字列にする操作(`String`、`GoString`)で、伏せた文字列を返す。値は、`Expose` でだけ取り出す。呼ぶのは、秘密を計算に渡す箇所と、一覧と照合する箇所に限る。
- 使える標準ライブラリは、設定に列挙したものに限る。`fmt`、`log/slog`、`os`、`net/http`、`database/sql` などは、import できない(application も同じ)。列挙にないものが要る場合は、開発者に確かめてから、設定に加える(入出力を行わないものに限る)。

## application

- **ポート**は、現に存在するユースケースが通過するものだけを置く。ポートの境界では、domain の型を、そのまま使う。専用のデータ構造は、置かない。
- **ユースケースの入力と出力**は、usecase が定義する、ユースケースごとの専用のデータ構造とする。domain の型を、含めない。認証を要するユースケースの入力には、操作を行う利用者(`Actor`。要求から取り出した、資格と、ログイン状態の識別子)を含める。例外は、接続を受け付けるユースケースの入力が含む、`port.ConnectionSink` の1つである(ADR-005)。
- **拒否**は、`*RejectedError`(種類は、`Rejection`)で返す。完了できなかったことは、そのほかの `error` で返す。domain が返した規則の違反は、usecase が、拒否の種類に置き換える。
- ユースケースを呼ぶ側には、インターフェイスを置かない。adapter は、usecase の具体的な型を呼ぶ。
- **ログイン状態の確認**は、usecase が行う(ADR-019)。認証を要するユースケースの、公開するメソッドは、最初の手順として、`SessionGate` の確認を呼び、ほかの検証と認可より先に行う。確認の手順を持たないメソッドは、認証なしに、操作を受け付ける。検査では検出できないため、操作を加えたときは、`cmd/server` の `TestInvalidSession_IsRejectedByEveryOperation` に加える。
- **資格**(ログイン状態を示す資格、確認のための資格)は、ログイン状態と、登録の申し込みを保持するポートと、案内を送るポートに渡すほかには、使わない。ユースケースの出力(新しく与えた資格を含む、`LoginOutput` を除く)、拒否、エラー、出来事の記録に、資格と秘密を含めない(ADR-019)。
- **出来事の記録**は、usecase が、記録のポート(`EventRecorder`)を呼んで行う(ADR-021)。出来事は、すべて、`attempt`(`usecase/attempt.go`)を通して記録する。エラーを返す、公開するメソッドは、最初に `begin` で `attempt` を作り、戻るときに、`finish` を `defer` で呼ぶ。`finish` を通さないメソッドが返したエラーは、どこにも記録されない。公開するメソッドを加えたときは、`usecase/recording_test.go` の一覧に加える。
- 時刻は、`Clock` のポートから得て、ほかのポートに、引数で渡す。期限と上限の判定を、保管先の時刻や、adapter が読んだ時刻に、頼らない。
- 復元用の関数(`RestoreBody`、`RestoreSendID`、`RestoreContact`、`RestoreHandle`、`RestoreDisplayName`)を、usecase から呼ばない。呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする。確かめるには、`grep -rn 'Restore' server/internal/application/usecase/*.go | grep -v _test` を使う(出力は、ない)。
- トランザクションの関数(`Transactor.InTransaction`、`InSignupTransaction` に渡す関数)の中で、渡された操作(`ConversationWriter`、`SignupWriter`)のほかに、ポートを呼ばない。配信は、トランザクションのポートから戻った後に呼ぶ。
- ログの標準ライブラリは、使わない。記録は、上の通り、ポートを通す。
- 存在の秘匿(R-2、AC-5、AC-9)は、usecase が、存在しない場合と、認可されない場合を、同じ拒否の種類(`NotFound`)にすることで保つ。区別できる種類を、加えない。

## adapter

1つの adapter のパッケージは、次の2つの役割の、一方だけを行います。adapter のパッケージ同士は、import しません。両方が要る相手(WebSocket)は、パッケージを分けています。

| 役割 | 今あるパッケージ |
|---|---|
| ユースケースを呼ぶ | `httpapi`、`wsaccept`、`schedule` |
| ポートを実装する | `postgres`、`wsdelivery`、`argon2id`、`commonsecrets`、`smtpmail`、`ratelimit`、`system`、`eventlog` |

パッケージごとの「import してよい先」は、アーキテクチャ概要の表にあります。

- ポートを実装する adapter から、domain の規則(`NewBody`、`ParseSendID`、`NewDirectPair` など)を呼ばない。domain を使うのは、型の受け渡しと、保管した値からの組み立てに限る。
  - 例外は、2つである(ADR-020)。
    1. adapter のテストが、ポートに渡す入力を用意するために、ほかに作る手段がない値を作る関数を呼ぶこと。当たるのは、`NewDirectPair`(`postgres` のテスト)と、`NewSecret`(`argon2id`、`commonsecrets` のテスト)である。復元用の関数がある型(本文、送信の識別子、連絡先、識別名、表示名)は、テストでも、復元用の関数で作る。
    2. ポートを実装する adapter が、保持する状態の要素ごとに、既にある domain の値についての判定を呼ぶこと。当たるのは、`wsdelivery` が、保持したログイン状態について呼ぶ、`Session.ValidAt` の1つである。加える場合は、ADR-020 の条件を、レビューで確かめる。
  - 確かめるには、`grep -rn 'domain\.\(New\|Parse\)' server/internal/adapter` を使う。出力は、テストのファイルの、上の3箇所だけになる。規則を確かめる関数を domain に加えたときは、本書の列挙に加える。
- adapter が自分でインターフェイスを定義して、他の adapter を受け取る形を、作らない(import を伴わずに、層を飛ばす経路になる)。
- 同じ変換が、複数の adapter に要ることがある(メッセージを通信の形式にする変換は、`httpapi` と `wsdelivery` にある。拒否の応答を書く処理、資格を載せる Cookie の名前、送信元の取り出しは、`httpapi` と `wsaccept` にある)。これは、規則の帰結であり、他の adapter の関数を使う近道を、取らない。それぞれに書き、食い違いは、`cmd/server` のテストで検出する。共有のためのパッケージを加えたくなった場合は、開発者に相談する。
- ポートを実装する adapter は、外部の相手が返したエラーを、そのまま内側へ返さない。行の内容や、宛先を含まない形に、作り直して返す。保管先は、操作の名前と SQLSTATE だけを持つエラー(`postgres.OpError`)、案内の送信は、操作の名前と、SMTP の応答の番号だけを持つエラー(`smtpmail.SendError`)である。返したエラーの文言は、usecase が、出来事として記録する(ADR-021)。この形を、保つ。
- ユースケースを呼ぶ adapter は、ユースケースが返した拒否とエラーを、ログに出力しない。応答に置き換えるだけとする(ADR-021)。資格と、ログイン状態の識別子は、要求から取り出して、そのまま渡す。自分では、確かめない。
- ポートの実装は、自分では、時刻を読まない。時刻は、ポートの引数で受ける(期限と上限の判定に使わない、メールの作成の日時は、除く)。
- 1つのサーバーの中だけで保持する状態(接続中の端末、配信の宛先、上限の計数、プレゼンスなど)は、ポートを実装する adapter の中に置く。usecase と domain に、持たせない(PERF-3)。

## main(`cmd/server`)

- 組み立てだけを行う。業務の判断を、書かない。
- adapter には、そのパッケージが import してよい層のものだけを渡す。ユースケースを呼ぶ adapter(`httpapi`、`wsaccept`、`schedule`)には、ユースケースだけを渡し、ポートの実装を渡さない。
- 組み立ては、ポートの実装を受け取る関数(`assemble.go` の `assemble`)にまとめてある。テストは、この関数に、差し替えた実装を渡す(時刻の源、案内の送信など)。

## テストの置き場所

| 確かめること | 置き場所 | 保管先 |
|---|---|---|
| domain の規則 | `internal/domain` | 使わない |
| ユースケースの手順と分岐。ポートは、手で書いたテスト用の実装(`usecase/fakes_test.go`)に差し替える | `internal/application/usecase` | 使わない |
| 保管先の adapter。制約とトランザクションに依る振る舞い | `internal/adapter/postgres` | 使う(`pgtest` が、テストごとのデータベースを用意する) |
| ユースケースを呼ぶ adapter のうち、ユースケースを通らないもの(本文の復号、ミドルウェア、共通の応答、書き込み口、呼ぶ間隔) | `internal/adapter/httpapi`、`internal/adapter/wsaccept`、`internal/adapter/schedule` | 使わない |
| そのほかの、ポートを実装する adapter | `internal/adapter/<パッケージ>` | 使わない(`smtpmail` は、Mailpit を使う) |
| サーバーの振る舞い(受け入れ基準。拒否の種類と `code` の対応、認証と認可、配信、ログ、保管の失敗) | `cmd/server` | 使う |

- サーバーの振る舞いのテスト(`cmd/server/harness_test.go`)は、実際の adapter で組み立て、時刻の源(テストから進める)と、案内の送信(送った案内を覚える)だけを差し替える。秘密の計算は、実際に行う。上限は、既定では、当たらない値にする。
- `httpapi`、`wsaccept`、`schedule` のパッケージの中のテストで、ユースケースを組み立てない(port と domain を import できない)。ユースケースを通る振る舞いは、`cmd/server` のテストで確かめる。
- テストの手段は、Go の標準の仕組み(testing)とする。比較や、代替実装の生成のためのライブラリを、加えない(ADR-006)。
- サーバーのテストは、`make test-server` で実行する(雛形のデータベースを作り直す)。`go test` を直接に実行する場合の手順は、README「サーバーのテストと、保管先」にある。
- 受け入れ基準を確かめるテストは、どの基準のものかが分かるように、基準のIDを、コメントに書く。

## ログと応答に、出力しないもの

ログには、次を出力しません。

- メッセージの本文と、要求の本文(PRV-4、WS-33)。
- 保管先が返したエラーの文言と詳細。行の内容(本文)が、含まれることがある。
- 認証に用いる秘密、ログイン状態を示す資格、連絡先の確認・再設定のための一時的な資格(SEC-1。エラー応答にも、出力しない)。
- 連絡先、識別名、表示名(仕様「記録」、RL-D8。利用者は、システムが割り当てた識別子で記録する)。要求のパスは、識別名を含むため、記録しない。

拒否の応答とエラー応答は、`code` を返します。`invalid_fields` の応答だけは、正しくない項目の名前の一覧(`fields`)を加えます。文言、詳細、内部の情報を、含めません(WS-30)。資格は、Cookie にだけ載せ、応答の本文には、含めません(ADR-014)。

## パッケージを加えるとき

1. 層と、adapter であれば役割(ユースケースを呼ぶ、ポートを実装する)を決める。既存の形に収まらない場合は、開発者に相談する(ADRが要る)。
2. `server/.golangci.deps.yml` に、3箇所を加える。そのパッケージの規則。import する側の、許可の一覧。`unlisted-package` からの除外。規則のない置き場所は、検査が、違反として扱う。
3. 規則に違反する見本(`depcheck_sample_*.go`。ビルドタグ `depcheck_sample`。1つのファイルに、違反する import を、1つ)を加え、`make check-deps-selftest` で、検出されることを確かめる。
4. アーキテクチャ概要の、ディレクトリと、「パッケージと、import してよい先」の表を更新する。
5. 本書のうち、パッケージの名前を挙げている箇所(adapter の役割の表、規約、テストの置き場所)を直す。パッケージを除いたときも、同じである。
6. 観察記録に、追記する(置き場所の判断、境界要素の追加に当たる)。
