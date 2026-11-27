#!/usr/bin/env bash
# CIのランナーの上で、.env を作る(ADR-022)。.github/workflows/ci.yml が呼ぶ。
#
# .env.example を写し、PGPASSWORD に、実行のたびに生成した無作為な値を与える。
# 値は、ランナーとともに消える。リポジトリにも、GitHub の Secrets にも、秘密を置かない(SEC-6)。
#
# 開発者の端末では、使わない。既に .env がある場合は、上書きせずに、失敗する。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="$root/.env"

if [[ -e "$env_file" ]]; then
  echo "ci-env: $env_file が、既にあります。上書きしません。" >&2
  exit 1
fi

password="$(openssl rand -hex 24)"
# GitHub Actions のログに、値が現れた場合に、伏せさせる。
if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
  echo "::add-mask::$password"
fi

(
  umask 077
  sed "s/^PGPASSWORD=\$/PGPASSWORD=$password/" "$root/.env.example" > "$env_file"
)

if ! grep -q "^PGPASSWORD=$password\$" "$env_file"; then
  echo "ci-env: .env.example に、空の PGPASSWORD の行が、見つかりません。" >&2
  rm -f "$env_file"
  exit 1
fi
echo "ci-env: .env を作りました。"
