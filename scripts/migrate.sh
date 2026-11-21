#!/usr/bin/env bash
# 指定したデータベースに、マイグレーションを適用する。
#
#   scripts/migrate.sh <データベースの名前>
#
# 接続情報は、環境変数(PGHOST、PGPORT、PGUSER、PGPASSWORD)から読む。
# scripts/with-env.sh を通して呼ぶ。
set -euo pipefail

name="${1:?データベースの名前を指定してください}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PGDATABASE="$name" "$root/bin/goose" -dir "$root/server/migrations" postgres "dbname=$name" up
