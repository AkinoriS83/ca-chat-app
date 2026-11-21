#!/usr/bin/env bash
# 依存規則と飛ばし禁止の検査(ADR-005)。make check-deps が呼ぶ。
#
# 違反を検出したとき、日時と、検査の出力を、docs/dependency-check-log.md に追記して、失敗する。
# AIは、検査の失敗を、作業の中で直してしまうことがあり、自己申告だけでは、違反の件数が
# 残らないためである(ADR-008「取り漏らしを防ぐ手段」)。
#
# 検査の道具を、専用の設定を指定して、直接には実行しない。記録に、残らないためである。
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
log="$root/docs/dependency-check-log.md"
failed=0

# check <検査の名前> <ディレクトリ> <コマンド...>
check() {
  local name="$1" dir="$2"
  shift 2

  local output status
  output="$(cd "$root/$dir" && "$@" 2>&1)"
  status=$?
  if [[ $status -eq 0 ]]; then
    echo "check-deps: $name: 違反なし"
    return
  fi

  failed=1
  echo "check-deps: $name: 失敗(終了コード $status)"
  echo "$output"
  {
    echo
    echo "## $(date '+%Y-%m-%dT%H:%M:%S%:z') $name"
    echo
    echo '```'
    echo "$output"
    echo '```'
  } >> "$log"
}

check "サーバー(depguard)" server \
  "$root/bin/golangci-lint" run -c .golangci.deps.yml ./...
check "クライアント(dependency-cruiser)" client \
  npx --no-install depcruise src --config .dependency-cruiser.cjs
check "クライアント(ESLint。application の規則)" client \
  npx --no-install eslint -c eslint.deps.config.js src

if [[ $failed -ne 0 ]]; then
  echo "check-deps: 違反を検出しました。出力を、docs/dependency-check-log.md に追記しました。" >&2
  echo "check-deps: 規則の側を緩めて通さないでください。観察記録(docs/observations.md)に、都度の記録を追記してください。" >&2
  exit 1
fi
