// Package pgtest は、保管先を使うテストのための、データベースを用意する。
// テストだけが import する。
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// templateDatabase は、マイグレーションを適用した、雛形のデータベースである。
// make test-server が、作り直す。
const templateDatabase = "chat_test_template"

// NewDatabase は、テストごとに独立したデータベースを、雛形から複製して作り、
// その接続プールを返す。テストが終わると、データベースを削除する。
//
// 接続情報は、環境変数(PGHOST、PGPORT、PGUSER、PGPASSWORD)から読む。
func NewDatabase(t testing.TB) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, "dbname=postgres")
	if err != nil {
		t.Fatalf("保管先に接続できません。make db-up で起動し、make test-server を使ってください: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	name := "chat_test_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+templateDatabase); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "3D000" {
			t.Fatalf("雛形のデータベース(%s)がありません。make test-server を使ってください", templateDatabase)
		}
		t.Fatalf("テスト用のデータベースを作れません: %v", err)
	}

	pool, err := pgxpool.New(ctx, "dbname="+name)
	if err != nil {
		t.Fatalf("テスト用のデータベースに接続できません: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, "dbname=postgres")
		if err != nil {
			t.Logf("テスト用のデータベース(%s)を削除できません: %v", name, err)
			return
		}
		defer func() { _ = admin.Close(context.Background()) }()
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("テスト用のデータベース(%s)を削除できません: %v", name, err)
		}
	})
	return pool
}

func randomSuffix(t testing.TB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
