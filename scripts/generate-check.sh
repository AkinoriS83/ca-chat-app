#!/usr/bin/env bash
# 生成し直した結果が、リポジトリの内容と一致することを確かめる(ADR-003、ADR-004)。
# make generate-check が呼ぶ。
#
# 生成したコードを、一時的な場所に写してから、生成し直し、比べる。
# 一致しない場合、生成し直した結果は、そのまま残る(内容を確かめて、コミットする)。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

generated=(
  server/internal/openapi
  server/internal/adapter/postgres/sqlcgen
  client/src/adapter/generated
)

snapshot="$(mktemp -d)"
trap 'rm -rf "$snapshot"' EXIT

for dir in "${generated[@]}"; do
  mkdir -p "$snapshot/$dir"
  if [[ -d "$dir" ]]; then
    cp -R "$dir/." "$snapshot/$dir/"
  fi
done

make --no-print-directory generate > /dev/null

status=0
for dir in "${generated[@]}"; do
  if ! diff -r "$snapshot/$dir" "$dir"; then
    status=1
  fi
done

if [[ $status -ne 0 ]]; then
  echo "generate-check: 生成したコードが、定義と一致しません。make generate の結果を確かめてください。" >&2
  exit 1
fi
echo "generate-check: 生成したコードは、定義と一致しています。"
