# よく使われる秘密の一覧(`list.txt`)の出所

判断と理由は、[ADR-017](../../../../docs/adr/ADR-017-common-secret-list.md) にあります。

| 項目 | 内容 |
|---|---|
| 出所 | SecLists(<https://github.com/danielmiessler/SecLists>)の `Passwords/Common-Credentials/Pwdb_top-1000000.txt` |
| コミット | `01bd31a91d11025e37c776b01edfe61c58dba784` |
| 上流 | Pwdb-Public(<https://github.com/ignis-sec/Pwdb-Public>) |
| 取り出した条件 | 15文字以上、128文字以下の行(Unicode のコードポイントで数える)から、`@` を含む行を除いたもの。行の順は、出所のまま |
| 件数 | 5,392件(長さの条件を満たす 5,406件から、`@` を含む 14件を除いた) |
| SHA-256 | `7ea526caa6921340fccd0548617d4f7a3a33e66207ffd61472efd9684fad0919` |
| 取り出した日 | 2026-11-27 |
| 取り出す手順 | `make common-secrets`(`scripts/extract-common-secrets.sh`、`scripts/extract-common-secrets.go`) |

## ライセンス

SecLists は、リポジトリの全体を、MITライセンスで公開しています。上流の Pwdb-Public も、MITライセンスです(ADR-017 が、2026-11-27 に確かめた)。元の漏えいしたデータそのものには、ライセンスの表明がありません。

```
MIT License

Copyright (c) 2018 Daniel Miessler

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## 注意

- 一覧は、漏えいした、実際の秘密です。個人を指しうる値(氏名や、生まれた年に見える値)が、残っています。いずれも、出所で、既に公開されている値です。
- `list.txt` は、手で書き換えません。パッケージのテストが、件数、各行の長さ、`@` を含まないこと、ファイルの要約を確かめます。取り出し直したときは、本書と、テストが持つ件数と要約を、併せて直します。
