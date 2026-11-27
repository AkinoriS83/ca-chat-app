---
paths:
  - "api/**"
  - "server/migrations/**"
  - "server/internal/adapter/postgres/queries/**"
  - "server/internal/adapter/postgres/sqlcgen/**"
  - "server/internal/openapi/**"
  - "client/src/adapter/generated/**"
  - "server/sqlc.yaml"
  - "server/oapi-codegen.yaml"
---

# 通信の形式、保管先の定義、生成したコードの指示

理由は、[ADR-003](../../docs/adr/ADR-003-api-contract-definition.md)(通信の形式)と、[ADR-004](../../docs/adr/ADR-004-data-store.md)(保管先)にあります。

## 定義と、生成するもの

| 定義(手で書く) | 生成するもの(手で編集しない) | 道具 |
|---|---|---|
| `api/openapi.yaml` | `server/internal/openapi/` | oapi-codegen |
| `api/openapi.yaml` | `client/src/adapter/generated/openapi.d.ts` | openapi-typescript |
| `server/migrations/`、`server/internal/adapter/postgres/queries/` | `server/internal/adapter/postgres/sqlcgen/` | sqlc |

1. 定義を変える。
2. `make generate` で、生成し直す。生成したコードは、リポジトリに含める。
3. マイグレーションを加えた場合は、`make db-migrate` で、開発用のデータベースに適用する(テスト用と E2E用のデータベースは、`make test-server` と `make e2e` が、作り直す)。
4. `make check` を実行する。`make generate-check` が、生成し直した結果と、リポジトリの内容の一致を確かめる。

生成したコードにも、依存規則を適用します。生成した型を import できるのは、通信の形式や、保管先を扱う adapter だけです。内側の層へ、渡しません。

## 通信の形式(`api/openapi.yaml`)

- 通信の形式は、この文書1つで定義する。手で書いた型で、二重に持たない。
- 文書に記述するのは、形式(項目と、その型)までとする。値の検証は、内側の層で行う。本文と、送信の識別子は、文書の上では、制約のない文字列である。長さや形式の制約を、文書に加えない(復号の段階で拒否すると、仕様が定めた拒否の順序を、保てない)。
- 拒否の応答とエラー応答は、どの操作でも、`ErrorResponse` とする。持つのは、`code` と、`invalid_fields` の場合の、正しくない項目の名前の一覧(`fields`)だけである。文言、詳細、内部の情報を、含めない(WS-30)。
- 資格(ログイン状態を示す資格、確認のための資格)を、応答のスキーマ、パス、クエリに、含めない。ログイン状態を示す資格は、Cookie でだけ運ぶ。確認のための資格は、要求の本文で送る(ADR-014、RL-33)。利用者を表すスキーマ(`UserProfile`)は、識別名と表示名だけを持つ。連絡先を、どの応答にも含めない(RL-27)。
- 状態を変える操作を、GET と HEAD に割り当てない(ADR-014)。
- 仕様が、同じ応答で拒否すると定めた場合(存在しない場合と、認可されない場合。ログイン状態が有効でない理由。ログインの失敗の理由。確認のための資格が無効な理由。R-2、AC-5、AC-9)は、応答の種類と内容を、同じにする。区別できる `code` や、状態コードを、加えない。
- 1つの要求が、複数の拒否に当たるときの順は、文書の説明に書いてある(Host、Origin が先。認証を要する操作は、続いて、ログイン状態、形式、操作ごとの順。認証を要さない操作は、形式、操作ごとの順)。操作や拒否を加えるときは、この順の中の位置を、仕様で確かめ、説明を直す。
- WebSocket で届けるメッセージは、スキーマとしてだけ記述し、どのメッセージが、いつ届くかは、文書の説明に書く。
- OpenAPI の版は、3.0 とする。
- 仮実装の箇所には、`仮実装(WS-Pn)` と書く。この文言は、生成したコードにも写る。
- ログイン状態の添え方(`securitySchemes`)は、文書に記述するだけである。生成したコードは、取り出さない。adapter が、要求から取り出す。

## 保管先(マイグレーションと、問い合わせ)

- 構造を変えるときは、連番の続きの、新しいファイルを、`server/migrations/` に加える(goose)。既存のファイルを書き換える必要があると考えた場合は、開発者に確かめる。
- 問い合わせは、SQL で、`queries/` に書く。ORM は、使わない。値は、常にパラメータとして渡し、問い合わせの文に、埋め込まない(WS-28、PRV-4)。
- 一意性と順序は、保管先の制約とトランザクションで保つ。サーバーのメモリの上の状態で、保たない(PERF-3)。
- 一覧、履歴、検索結果は、分割して取得する形にする(PERF-2。フェーズ1から適用する。今の履歴の一括取得は、仮実装 WS-P4 である)。
- 生成した型(`sqlcgen`)は、保管先の表現である。domain の型へは、`adapter/postgres` の中で詰め替える。

## 項目を1つ加えるときに、変わる箇所

利用者と、認証の方式を変えたときの範囲は、OBS-029 にあります。メッセージに項目を1つ加える場合、生成したコードを除いて、次が変わります(OBS-009)。変え漏らしを防ぐために、挙げておきます。

- 通信の形式の定義(`api/openapi.yaml`)
- domain の型
- 保管先のマイグレーション、問い合わせ、`adapter/postgres` の詰め替え
- ユースケースの出力のデータ構造と、その詰め替え(`usecase/view.go`)
- 通信の形式への変換が、2箇所(`httpapi` と `wsdelivery`)。食い違いは、`cmd/server` のテストが検出する
- クライアントの `adapter/wire.ts` と、`application/model.ts`

このように、1つの変更が、構造のために、複数の層に及んだときは、観察記録に追記します(変えた層と、ファイルの数を、手がかりに残す)。
