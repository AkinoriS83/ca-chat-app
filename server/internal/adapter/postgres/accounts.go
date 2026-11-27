package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres/sqlcgen"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// 利用者、ログイン状態、登録の申し込みを、保管先に置く(ADR-016)。
// Store は、port.UserDirectory を実装する。ログイン状態と、登録の申し込みは、
// 操作の名前が重なるため(Find、DeleteExpired)、Store から得る、別の型が実装する。

var (
	_ port.UserDirectory = (*Store)(nil)
	_ port.SessionStore  = (*SessionStore)(nil)
	_ port.SignupStore   = (*SignupStore)(nil)
	_ port.SignupWriter  = (*signupWriter)(nil)
)

// credentialDigest は、資格の要約(SHA-256)を返す(ADR-014)。
// 保管するのも、引くのも、要約である。資格そのものは、保管しない。
// ログイン状態と、登録の申し込みの、両方が使う。要約は、application に現れない。
func credentialDigest(credential string) []byte {
	sum := sha256.Sum256([]byte(credential))
	return sum[:]
}

// toUser は、保管先の行から、domain の利用者を組み立てる。連絡先と秘密は、含めない。
// 保管した値は、保管の時点で検証を通っているため、復元用の関数を使う(規則は、呼ばない)。
func toUser(id pgtype.UUID, handle, displayName string) domain.User {
	return domain.User{
		ID:          domain.UserID(id.String()),
		Handle:      domain.RestoreHandle(handle),
		DisplayName: domain.RestoreDisplayName(displayName),
	}
}

// FindByHandle は、識別名が一致する利用者を探す。大文字と小文字を、区別しない。
func (s *Store) FindByHandle(ctx context.Context, handle domain.Handle) (domain.User, bool, error) {
	row, err := sqlcgen.New(s.pool).FindUserByHandle(ctx, handle.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, newOpError("find user by handle", err)
	}
	return toUser(row.ID, row.Handle, row.DisplayName), true, nil
}

// FindByContact は、連絡先が一致する利用者と、保管した秘密を探す。
func (s *Store) FindByContact(ctx context.Context, contact domain.Contact) (domain.User, domain.SecretHash, bool, error) {
	row, err := sqlcgen.New(s.pool).FindUserByContact(ctx, contact.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, "", false, nil
	}
	if err != nil {
		return domain.User{}, "", false, newOpError("find user by contact", err)
	}
	return toUser(row.ID, row.Handle, row.DisplayName), domain.SecretHash(row.SecretHash), true, nil
}

// SessionStore は、ログイン状態を、保管先に置く。port.SessionStore を実装する。
type SessionStore struct {
	queries *sqlcgen.Queries
}

// Sessions は、同じ接続プールを使う、SessionStore を返す。
func (s *Store) Sessions() *SessionStore {
	return &SessionStore{queries: sqlcgen.New(s.pool)}
}

// Create は、資格に当たるログイン状態を作る。保管するのは、資格の要約である。
func (s *SessionStore) Create(ctx context.Context, credential string, user domain.UserID, expiresAt time.Time) (domain.Session, error) {
	const op = "create session"

	userID, err := toUUID(string(user))
	if err != nil {
		return domain.Session{}, newOpError(op, err)
	}
	id, err := s.queries.InsertSession(ctx, sqlcgen.InsertSessionParams{
		CredentialDigest: credentialDigest(credential),
		UserID:           userID,
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		return domain.Session{}, newOpError(op, err)
	}
	return domain.Session{ID: domain.SessionID(id.String()), UserID: user, ExpiresAt: expiresAt.UTC()}, nil
}

// Find は、資格に当たる、now の時点で有効なログイン状態と、その利用者を探す。
func (s *SessionStore) Find(ctx context.Context, credential string, now time.Time) (domain.Session, domain.User, bool, error) {
	row, err := s.queries.FindSession(ctx, sqlcgen.FindSessionParams{CredentialDigest: credentialDigest(credential), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, domain.User{}, false, nil
	}
	if err != nil {
		return domain.Session{}, domain.User{}, false, newOpError("find session", err)
	}
	user := toUser(row.UserID, row.Handle, row.DisplayName)
	session := domain.Session{ID: domain.SessionID(row.ID.String()), UserID: user.ID, ExpiresAt: row.ExpiresAt.UTC()}
	return session, user, true, nil
}

// Revoke は、資格に当たるログイン状態を、無効にする(行を削除する)。
func (s *SessionStore) Revoke(ctx context.Context, credential string) (domain.SessionID, bool, error) {
	id, err := s.queries.DeleteSession(ctx, credentialDigest(credential))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, newOpError("revoke session", err)
	}
	return domain.SessionID(id.String()), true, nil
}

// DeleteExpired は、now の時点で、期限を過ぎたログイン状態を消す。
func (s *SessionStore) DeleteExpired(ctx context.Context, now time.Time) error {
	if err := s.queries.DeleteExpiredSessions(ctx, now); err != nil {
		return newOpError("delete expired sessions", err)
	}
	return nil
}

// SignupStore は、登録の申し込みを、保管先に置く。port.SignupStore を実装する。
type SignupStore struct {
	queries *sqlcgen.Queries
}

// Signups は、同じ接続プールを使う、SignupStore を返す。
func (s *Store) Signups() *SignupStore {
	return &SignupStore{queries: sqlcgen.New(s.pool)}
}

// Put は、申し込みを置く。同じ連絡先の、以前の申し込みを置き換える。
func (s *SignupStore) Put(ctx context.Context, contact domain.Contact, credential string, expiresAt time.Time) error {
	err := s.queries.UpsertSignup(ctx, sqlcgen.UpsertSignupParams{
		Contact:          contact.String(),
		CredentialDigest: credentialDigest(credential),
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		return newOpError("put signup", err)
	}
	return nil
}

// Find は、資格に当たる、now の時点で有効な申し込みを探す。
func (s *SignupStore) Find(ctx context.Context, credential string, now time.Time) (domain.Signup, bool, error) {
	row, err := s.queries.FindSignup(ctx, sqlcgen.FindSignupParams{CredentialDigest: credentialDigest(credential), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Signup{}, false, nil
	}
	if err != nil {
		return domain.Signup{}, false, newOpError("find signup", err)
	}
	return domain.Signup{Contact: domain.RestoreContact(row.Contact), ExpiresAt: row.ExpiresAt.UTC()}, true, nil
}

// DeleteExpired は、now の時点で、期限を過ぎた申し込みを、連絡先とともに消す(RL-09)。
func (s *SignupStore) DeleteExpired(ctx context.Context, now time.Time) error {
	if err := s.queries.DeleteExpiredSignups(ctx, now); err != nil {
		return newOpError("delete expired signups", err)
	}
	return nil
}

// InSignupTransaction は、fn を、1つのトランザクションの中で実行する。
// 取り消しと確定は、InTransaction と同じである(ADR-012)。
func (s *Store) InSignupTransaction(ctx context.Context, fn func(ctx context.Context, w port.SignupWriter) error) error {
	return s.inTx(ctx, func(queries *sqlcgen.Queries) error {
		return fn(ctx, &signupWriter{queries: queries})
	})
}

// signupWriter は、登録の完了の、トランザクションの中でだけ行える操作を実装する。
type signupWriter struct {
	queries *sqlcgen.Queries
}

// ConsumeSignup は、資格に当たる、now の時点で有効な申し込みを、使用済みにして(行を削除して)、返す。
func (w *signupWriter) ConsumeSignup(ctx context.Context, credential string, now time.Time) (domain.Signup, bool, error) {
	row, err := w.queries.ConsumeSignup(ctx, sqlcgen.ConsumeSignupParams{CredentialDigest: credentialDigest(credential), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Signup{}, false, nil
	}
	if err != nil {
		return domain.Signup{}, false, newOpError("consume signup", err)
	}
	return domain.Signup{Contact: domain.RestoreContact(row.Contact), ExpiresAt: row.ExpiresAt.UTC()}, true, nil
}

// CreateUser は、利用者を作る。一意性は、保管先の制約で保つ(RL-07)。
// 識別名の一意性への違反を、port.ErrHandleTaken に、連絡先の一意性への違反を、port.ErrContactTaken に置き換える。
func (w *signupWriter) CreateUser(ctx context.Context, contact domain.Contact, handle domain.Handle, name domain.DisplayName, secret domain.SecretHash) (domain.User, error) {
	id, err := w.queries.InsertUser(ctx, sqlcgen.InsertUserParams{
		Contact:     contact.String(),
		Handle:      handle.String(),
		DisplayName: name.String(),
		SecretHash:  string(secret),
	})
	switch {
	case isUniqueViolation(err, "users_handle_key"):
		return domain.User{}, port.ErrHandleTaken
	case isUniqueViolation(err, "users_contact_key"):
		return domain.User{}, port.ErrContactTaken
	case err != nil:
		return domain.User{}, newOpError("create user", err)
	}
	return toUser(id, handle.String(), name.String()), nil
}
