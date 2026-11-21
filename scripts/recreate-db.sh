#!/usr/bin/env bash
# データベースを作り直し、マイグレーションを適用する(詳細は README.md)。
#
#   scripts/recreate-db.sh <データベースの名前>
#
# テスト用の雛形(chat_test_template)と、E2E用(chat_e2e)に使う。
# 作り直しは、保管先のコンテナの中の psql で行う。scripts/with-env.sh を通して呼ぶ。
set -euo pipefail

name="${1:?データベースの名前を指定してください}"
if [[ ! "$name" =~ ^[a-z][a-z0-9_]*$ ]]; then
  echo "recreate-db: データベースの名前は、英小文字、数字、_ で指定してください。" >&2
  exit 1
fi
if [[ "$name" == "${PGDATABASE:?}" || "$name" == "postgres" ]]; then
  echo "recreate-db: 開発用のデータベースは、作り直しません。" >&2
  exit 1
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

docker compose exec -T postgres psql -q -v ON_ERROR_STOP=1 -U "${PGUSER:?}" -d postgres \
  -c "DROP DATABASE IF EXISTS \"$name\" WITH (FORCE)" \
  -c "CREATE DATABASE \"$name\""

"$root/scripts/migrate.sh" "$name"
