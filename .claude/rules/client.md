---
paths:
  - "client/**"
---

# クライアント(TypeScript、React)の指示

現在の姿(置き場所、ポート、振る舞いの要点)は、[アーキテクチャ概要](../../docs/architecture.md)「クライアントの層」にあります。理由は、[ADR-005](../../docs/adr/ADR-005-layer-structure.md)「クライアントの層」にあります。

## 層と、置くもの

| 層 | 置き場所 | 置くもの |
|---|---|---|
| application | `src/application`、`src/application/port` | クライアントの振る舞いと状態(`chatApp.ts`)、型(`model.ts`)、ポート |
| adapter | `src/adapter` | ポートの実装。通信の形式(生成した型)と、application の型の間の変換(`wire.ts`) |
| ui | `src/ui` | React の部品。application の状態を表示し、利用者の操作を application に伝える |
| main | `src/main.tsx` | 組み立てと、起動 |

置き場所ごとの「import してよい先」は、アーキテクチャ概要「置き場所と、import してよい先」の表と、設定(`client/.dependency-cruiser.cjs`。型だけの import も、対象にする。`client/eslint.deps.config.js`)にあります(本書には、写しません)。列挙にない置き場所は、違反です。

置き場所は、次の順に考えます。

1. ブラウザの入出力のAPI(`fetch`、`WebSocket`、タイマー、`crypto`、`localStorage` など)か、通信の形式を、知っている必要があるか。あれば、adapter。application からは、ポートを通して使う。
2. 画面の構造と、表示のための文言か。そうであれば、ui。
3. 状態と、その遷移(何を受けたら、何を示すか)か。そうであれば、application。
4. 業務の規則(本文の検証など)は、クライアントに置かない。サーバーが持つ。domain の層は、置いていない。置くものが現れたと考えた場合は、開発者に相談する(ADR-005)。

## 規約

- application は、React と、ブラウザの入出力のAPIを使わない。新しいAPIが要る場合は、ポートを加え、adapter で実装する。ESLint の規則の一覧(`eslint.deps.config.js`)にないAPIを使うことになる場合は、一覧にも加える。
- ui は、application を経由せずに、通信しない。ui と adapter は、互いに import しない。
- 生成した型(`src/adapter/generated/`)は、adapter だけが import する。ui と application は、application の型(`model.ts`)を使う。
- 状態は、`ChatApp` が持ち、変化を通知する。ui は、`subscribe` と `getState` を、React の `useSyncExternalStore` につなぐ。振る舞いに関わる状態を、ui の部品の中に持たせない。
- 振る舞いの規則(C-1〜)は、`chatApp.ts` の先頭に挙げてある。コメントとテストは、この番号で参照する。振る舞いを加える・変えるときは、この一覧を、併せて直す。
- クライアントは、送信の前に、本文を検証しない。検証と拒否は、サーバーが行い、クライアントは、拒否の `code` を、ui で文言にして示す(`ui/rejectionText.ts`)。
- 利用者が入力した内容は、文字として示す。HTML として差し込まない(`dangerouslySetInnerHTML` を、使わない。SEC-4、WS-27)。
- 実行時の依存物は、react と react-dom だけである。実行時に、外部から、コード、スタイル、フォントを読み込まない(ADR-001)。依存物を加える場合は、開発者に相談する。版は、`package.json` に、1つに指定して書く(WS-32)。
- メッセージの本文と、認証に関わる秘密や資格を、コンソールに出力しない。

## テストの置き場所

| 確かめること | 置き場所 | 手段 |
|---|---|---|
| 振る舞いと、状態の遷移 | `src/application/*.test.ts` | Vitest。ブラウザのAPIのない環境。ポートは、テスト用の実装(`application/testPorts.ts`)に差し替え、時間は、テストから進める |
| ポートの実装と、通信の形式の変換 | `src/adapter/*.test.ts` | Vitest(jsdom) |
| 画面の表示 | `src/ui/*.test.tsx` | Vitest、jsdom、React Testing Library。同じテスト用のポートを使う |
| サーバーとクライアントをつないだ動作 | `e2e/` | Playwright。複数の端末にまたがる主要な経路に限る。受け入れ基準を、網羅しない(ADR-006) |

- 受け入れ基準ごとの確認は、application と ui のテストを主とする。E2Eテストを、増やしすぎない。
- E2Eテストは、直列に実行し、テストごとに、データを空の状態に戻す。`make e2e` で実行する。端末ごとの事情(Chromium の場所など)は、README「初回の準備」「E2Eテスト」にある。
- 受け入れ基準を確かめるテストは、基準のIDを、名前かコメントに書く。

## 置き場所を加えるとき

1. `client/.dependency-cruiser.cjs` に、規則を加える(`unlisted-location` からの除外を含む)。application に関わる場合は、`client/eslint.deps.config.js` も確かめる。
2. 規則に違反する見本を、その層の `__depcheck__/` に加える(名前は、`.depcruise.ts` か `.eslint.ts` で終える)。`make check-deps-selftest` で、検出されることを確かめる。
3. アーキテクチャ概要の、ディレクトリと、「置き場所と、import してよい先」の表を更新する。
4. 本書のうち、置き場所とファイルの名前を挙げている箇所(層の表、規約、テストの置き場所)を直す。
5. 観察記録に、追記する。
