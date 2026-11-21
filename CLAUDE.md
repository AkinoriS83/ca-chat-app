# AIへの常設の指示

このリポジトリで作業するAIへの指示です。背景は、[開発方針](docs/development-policy.md)、[仮説](docs/hypothesis.md)、[アーキテクチャ概要](docs/architecture.md)、ADR([docs/adr/](docs/adr/))を参照してください。手順は、[README.md](README.md) にあります。

## 観察記録を、その都度、追記する

本プロジェクトは、Clean Architecture の構造の効果と携行コストを、観察記録に残しています([ADR-008](docs/adr/ADR-008-observation-records.md))。出来事は、後から復元できません。次に当たることが起きた時点で、作業を続ける前に、[docs/observations.md](docs/observations.md) の「都度の記録」に追記してください。迷う場合は、記録します。

- 依存規則と飛ばし禁止の検査が、違反を検出した(近道を取ろうとした)。
- 1つの変更が、構造のために、複数の層やファイルに及んだ(項目を1つ加えるために、詰め替えを何箇所も変えた、など)。
- 変更が、1つの層の中に閉じた。層の境界で、実装を差し替えられた(テストでの差し替えを含む)。
- 境界要素(ポート、ユースケースの入力・出力のデータ構造、拒否の種類)を、加えた、変えた、削った。その理由。
- 置き場所や、規則の解釈に迷った。規則の例外を設けた、または、設けたくなった。
- AIに読ませる範囲や、指示の範囲を、層の単位で絞れた、または、絞れなかった。
- 仮実装(仕様 WS-P1〜WS-P4)を、正式実装に置き換えた。変更が及んだ層と、ファイルの数を、併せて記録する。

記録の項目は、次の通りです。既存の記録と、同じ表の形で書きます。

| 項目 | 内容 |
|---|---|
| 番号 | OBS-001 からの連番。振り直さない |
| 日付 | 出来事が起きた日 |
| スライスと工程 | 例: ウォーキングスケルトン、実装 |
| 区分 | 妨げ / 助け / どちらとも言えない |
| 出来事 | 何をしようとして、何が起きたか。事実だけを書く |
| 関わる構造 | 関わった層、境界要素、規則(依存規則、飛ばし禁止、型参照の範囲) |
| 手がかり | コミット、ファイル、検査の出力など、後から確かめられるもの |
| 所見 | 構造が、その出来事にどう働いたかの解釈。比較対象がないため、構造がなかった場合との差は推測であることを、明示する |
| 気付いた者 | 開発者 / AI |

仮説を支持する所見と、支持しない所見の、両方を残します。都合のよい出来事だけを、記録しないでください。

## 層と、import してよい先

層の構成と、パッケージごとの「import してよい先」は、[アーキテクチャ概要](docs/architecture.md)と [ADR-005](docs/adr/ADR-005-layer-structure.md) にあります。設定は、`server/.golangci.deps.yml`、`client/.dependency-cruiser.cjs`、`client/eslint.deps.config.js` です。

- 検査が失敗したとき、規則の側を緩めて通さない。import の向きを直すか、置き場所を改める。規則を変える必要があると考えた場合は、作業を止めて、開発者に相談する(ADRの改訂が要る)。
- パッケージや置き場所を加えるときは、規則を加え、アーキテクチャ概要を更新する。規則のない置き場所は、検査が違反として扱う。
- 規則は、テストのファイルにも、同じく適用する。

## 依存規則の検査は、`make check-deps` で実行する

依存規則と飛ばし禁止の検査は、`make check-deps` で実行してください。検査の道具(golangci-lint、depcruise、eslint)を、専用の設定を指定して、直接には実行しないでください。`make check-deps` は、違反を検出したとき、出力を [docs/dependency-check-log.md](docs/dependency-check-log.md) に追記します。直接に実行すると、違反が、記録に残らないまま直されてしまいます。

- 検査の記録は、手で書き換えない。違反を直した後も、消さない。
- `make lint` は、依存規則を検査しない(検査の記録に残すため、わざと分けてある)。

## 規約として守ること

次は、import の検査では、検出できません。コードを書くときと、レビューするときに、確かめてください。

- ポートを実装する adapter(`postgres`、`wsdelivery`、`fixeduser`)から、domain の規則(`NewBody`、`ParseSendID`、`NewDirectPair` など)を呼ばない。domain を使うのは、型の受け渡しと、保管した値からの組み立てに限る。
  - 例外は、1つである。adapter のテストが、ポートに渡す入力を用意するために、`NewDirectPair` を呼ぶことは、許す(`DirectPair` は、ほかに作る手段がない。ADR-005)。本文と送信の識別子は、テストでも、復元用の関数で作る。
  - 確かめるには、`grep -rn 'domain\.\(NewBody\|ParseSendID\|NewDirectPair\)' server/internal/adapter` を使う。出力は、テストのファイルの `NewDirectPair` だけになる。
- 復元用の関数(`RestoreBody`、`RestoreSendID`)を、usecase から呼ばない。呼ぶのは、ポートを実装する adapter と、テスト用のポートの実装だけとする。確かめるには、`grep -rn 'Restore\(Body\|SendID\)' server/internal` を使う。
- トランザクションの関数(`Transactor.InTransaction` に渡す関数)の中で、渡された操作(`ConversationWriter`)のほかに、ポートを呼ばない。配信は、トランザクションのポートから戻った後に呼ぶ。
- main(`server/cmd/server`)は、組み立てだけを行う。adapter には、そのパッケージが import してよい層のものだけを渡す。ユースケースを呼ぶ adapter(`httpapi`、`wsaccept`)には、ユースケースだけを渡し、ポートの実装を渡さない。adapter が自分でインターフェイスを定義して、他の adapter を受け取る形を、作らない。
- `httpapi` と `wsaccept` のパッケージの中のテストで、ユースケースを組み立てない。ユースケースを通る振る舞いは、`server/cmd/server` のテストで確かめる。
- クライアントの application は、React と、ブラウザの入出力のAPIを使わない。ui は、application を経由せずに、通信しない。

## ログに出力しないもの

- メッセージの本文と、要求の本文を、ログに出力しない(PRV-4、WS-33)。
- 保管先が返したエラーの文言と詳細を、ログに出力しない。行の内容(本文)が、含まれることがある。保管先の adapter は、操作の名前と SQLSTATE だけを持つエラー(`postgres.OpError`)に、作り直して返す。この形を、保つ。
- 拒否の応答とエラー応答は、`code` だけを返す。文言、詳細、内部の情報を、含めない(WS-30)。

## 仮実装

仮実装の箇所には、`仮実装(WS-Pn)` のコメントを付け、[docs/temporary-implementations.md](docs/temporary-implementations.md) の一覧を更新してください。一覧とコメントが対応していることは、`grep -rn '仮実装(WS-P' api server client/src` で確かめます。

## 生成したコード

`server/internal/openapi/`、`server/internal/adapter/postgres/sqlcgen/`、`client/src/adapter/generated/` は、生成したコードです。手で編集しないでください。定義(`api/openapi.yaml`、`server/migrations/`、`server/internal/adapter/postgres/queries/`)を変えてから、`make generate` で生成し直します。

## 変更の後に

変更の後に、`make check` を実行してください(保管先が起動している必要があります。`make db-up`)。画面と、サーバーとクライアントのつなぎに関わる変更では、`make e2e` も実行します。

文書の文体は、既存の文書(ADR、仕様)に合わせます。コミットは、開発者が求めたときにだけ行います。
