#!/usr/bin/env bash
# 規則に違反する見本のそれぞれが、違反として検出されることを確かめる(ADR-005「検査の手段」)。
# make check-deps-selftest が呼ぶ。
#
# 設定の誤りによって、検査が常に成功する状態を防ぐためである。
# 検査の記録(docs/dependency-check-log.md)には、追記しない。
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
failed=0

fail() {
  failed=1
  echo "check-deps-selftest: $*" >&2
}

# ---- サーバー: ビルドタグ depcheck_sample を付けて、depguard を実行する ----

mapfile -t server_samples < <(cd "$root/server" && grep -rl --include='*.go' '^//go:build depcheck_sample$' . | sed 's|^\./||' | sort)
if [[ ${#server_samples[@]} -eq 0 ]]; then
  fail "サーバーの見本が、見つかりません。"
fi

server_output="$(cd "$root/server" && "$root/bin/golangci-lint" run -c .golangci.deps.yml --build-tags depcheck_sample ./... 2>&1)"

# 型検査の失敗があると、検査は、他の見本の違反を、報告しなくなる。
if grep -q '(typecheck)' <<< "$server_output"; then
  fail "サーバーの検査の出力に、型検査の失敗があります。"
  echo "$server_output" >&2
fi
for sample in "${server_samples[@]}"; do
  if grep -F "$sample:" <<< "$server_output" | grep -q '(depguard)'; then
    echo "check-deps-selftest: 検出: server/$sample"
  else
    fail "検出されません: server/$sample"
  fi
done

# ---- クライアント: 見本(__depcheck__)を、対象に含めて実行する ----

mapfile -t cruise_samples < <(cd "$root/client" && find src -path '*/__depcheck__/*.depcruise.ts' | sort)
mapfile -t eslint_samples < <(cd "$root/client" && find src -path '*/__depcheck__/*.eslint.ts' | sort)
mapfile -t all_samples < <(cd "$root/client" && find src -path '*/__depcheck__/*' -type f | sort)
if [[ ${#cruise_samples[@]} -eq 0 || ${#eslint_samples[@]} -eq 0 ]]; then
  fail "クライアントの見本が、見つかりません。"
fi
if [[ ${#all_samples[@]} -ne $(( ${#cruise_samples[@]} + ${#eslint_samples[@]} )) ]]; then
  fail "クライアントの見本に、名前が .depcruise.ts でも .eslint.ts でもないファイルがあります。"
fi

cruise_output="$(cd "$root/client" && DEPCHECK_SAMPLES=1 npx --no-install depcruise src --config .dependency-cruiser.cjs 2>&1)"
for sample in "${cruise_samples[@]}"; do
  if grep -E '^\s*error ' <<< "$cruise_output" | grep -qF ": $sample →"; then
    echo "check-deps-selftest: 検出: client/$sample"
  else
    fail "検出されません: client/$sample"
  fi
done

for sample in "${eslint_samples[@]}"; do
  eslint_output="$(cd "$root/client" && DEPCHECK_SAMPLES=1 npx --no-install eslint -c eslint.deps.config.js "$sample" 2>&1)"
  if grep -qE 'error .* no-restricted-(globals|imports)$' <<< "$eslint_output"; then
    echo "check-deps-selftest: 検出: client/$sample"
  else
    fail "検出されません: client/$sample"
    echo "$eslint_output" >&2
  fi
done

if [[ $failed -ne 0 ]]; then
  echo "check-deps-selftest: 失敗しました。検査の設定を、確かめてください。" >&2
  exit 1
fi
echo "check-deps-selftest: 見本は、すべて検出されました(サーバー ${#server_samples[@]} 件、クライアント ${#all_samples[@]} 件)。"
