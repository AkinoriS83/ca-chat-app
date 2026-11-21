package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// errInvalidUUID は、識別子が、UUID の形式でないことを示す。
var errInvalidUUID = errors.New("invalid uuid")

// OpError は、保管先への操作が、完了できなかったことを示す。
//
// 保管先が返したエラーを、そのまま返さない。元のエラーの文言と詳細には、行の内容(本文)が
// 含まれることがあるためである。持つのは、操作の名前と、SQLSTATE だけとする(WS-33)。
// 元のエラーは、保持しない(Unwrap で取り出せない)。
type OpError struct {
	Op       string // 操作の名前
	SQLState string // 保管先が返した SQLSTATE。保管先が返したエラーでない場合は、空
}

func (e *OpError) Error() string {
	if e.SQLState == "" {
		return "postgres: " + e.Op + " failed"
	}
	return "postgres: " + e.Op + " failed (SQLSTATE " + e.SQLState + ")"
}

func newOpError(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return &OpError{Op: op, SQLState: pgErr.Code}
	}
	return &OpError{Op: op}
}

// isUniqueViolation は、err が、指定した一意性の制約への違反であるかを返す。
func isUniqueViolation(err error, constraint string) bool {
	const uniqueViolation = "23505"

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraint
}
