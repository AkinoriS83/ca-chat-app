# ローカルの開発環境の手順(ADR-007)。使い方は、README.md を参照。
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# Go の道具の版(WS-32)。bin/ に導入する。サーバーの go.mod には、加えない。
OAPI_CODEGEN_VERSION  := v2.8.2
SQLC_VERSION          := v1.31.1
GOOSE_VERSION         := v3.30.1
GOLANGCI_LINT_VERSION := v2.15.0
GITLEAKS_VERSION      := v8.30.1

ROOT     := $(CURDIR)
BIN      := $(ROOT)/bin
WITH_ENV := $(ROOT)/scripts/with-env.sh

# goose は、PostgreSQL のほかの保管先のドライバーを除いてビルドする。
GOOSE_TAGS := no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb

.PHONY: help setup tools db-up db-down db-migrate dev-server dev-client \
	generate generate-check lint check-deps check-deps-selftest \
	test-server test-client e2e secrets-scan check

help:
	@echo "手順は、README.md を参照してください。主な手順:"
	@echo "  make setup       道具と依存物を導入する"
	@echo "  make db-up       保管先を起動する"
	@echo "  make db-migrate  開発用のデータベースに、マイグレーションを適用する"
	@echo "  make dev-server  サーバーを起動する"
	@echo "  make dev-client  クライアントの開発用のサーバーを起動する"
	@echo "  make check       検査とテストを、まとめて実行する"
	@echo "  make e2e         E2Eテストを実行する"

## 導入

setup: tools
	cd client && npm ci
	cd client && npx playwright install chromium

tools:
	GOBIN=$(BIN) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
	GOBIN=$(BIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	GOBIN=$(BIN) go install -tags='$(GOOSE_TAGS)' github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(BIN) go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

## 保管先

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

db-migrate:
	$(WITH_ENV) bash -c '$(ROOT)/scripts/migrate.sh "$$PGDATABASE"'

## 起動

dev-server:
	cd server && $(WITH_ENV) go run ./cmd/server

dev-client:
	cd client && npm run dev

## コード生成(ADR-003、ADR-004)

generate:
	cd server && $(BIN)/oapi-codegen -config oapi-codegen.yaml ../api/openapi.yaml
	cd server && $(BIN)/sqlc generate
	cd client && npm run --silent generate

generate-check:
	scripts/generate-check.sh

## 検査

# 依存規則と飛ばし禁止の規則は、含めない。check-deps だけが実行する(検査の記録に残すため)。
lint:
	cd server && $(BIN)/golangci-lint run ./...
	cd client && npm run --silent lint
	cd client && npm run --silent typecheck

check-deps:
	scripts/check-deps.sh

check-deps-selftest:
	scripts/check-deps-selftest.sh

secrets-scan:
	$(BIN)/gitleaks git --no-banner --redact .

## テスト

test-server:
	$(WITH_ENV) scripts/recreate-db.sh chat_test_template
	cd server && $(WITH_ENV) go test -race -count=1 ./...

test-client:
	cd client && npm run --silent test

e2e:
	$(WITH_ENV) scripts/recreate-db.sh chat_e2e
	cd client && $(WITH_ENV) npx playwright test

## まとめて実行する

check: generate-check lint check-deps check-deps-selftest test-server test-client secrets-scan
