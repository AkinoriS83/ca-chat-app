#!/usr/bin/env bash
# よく使われる秘密の一覧を、取り出し直す(ADR-017)。make common-secrets が呼ぶ。
#
# SecLists の、固定したコミットのファイルを取得し、scripts/extract-common-secrets.go の条件で取り出して、
# server/internal/adapter/commonsecrets/list.txt に書く。外部(raw.githubusercontent.com)への到達を要する。
# 一覧を更新するときは、コミットを指定し直し、同じ置き場所の NOTICE.md と、テストが持つ件数と要約を直す。
set -euo pipefail

commit="01bd31a91d11025e37c776b01edfe61c58dba784"
file="Passwords/Common-Credentials/Pwdb_top-1000000.txt"
url="https://raw.githubusercontent.com/danielmiessler/SecLists/$commit/$file"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="$root/server/internal/adapter/commonsecrets/list.txt"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "extract-common-secrets: 取得: $url"
curl --fail --silent --show-error --location --max-time 300 --output "$work/source.txt" "$url"

mkdir -p "$(dirname "$out")"
go run "$root/scripts/extract-common-secrets.go" "$work/source.txt" "$out"
echo "extract-common-secrets: 書き出した: $out"
sha256sum "$out"
