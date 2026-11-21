#!/usr/bin/env bash
# リポジトリの直下の .env を読み込んで、与えられたコマンドを実行する。
#
# Docker Compose と同じく、既に環境にある変数は、上書きしない。
# .env は、シェルとしては実行しない。「名前=値」の行だけを読む。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="$root/.env"

if [[ ! -f "$env_file" ]]; then
  echo "with-env: $env_file がありません。.env.example をコピーし、PGPASSWORD を決めてください(README.md)。" >&2
  exit 1
fi

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%$'\r'}"
  [[ -z "$line" || "$line" == \#* ]] && continue
  if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
    name="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    if [[ -z "${!name+set}" ]]; then
      export "$name=$value"
    fi
  else
    echo "with-env: .env の行を読めません(名前=値 の形で書いてください)。" >&2
    exit 1
  fi
done < "$env_file"

exec "$@"
