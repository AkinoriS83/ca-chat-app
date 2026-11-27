package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// テスト用の利用者。識別子の順は、識別名の順と、わざと変えてある(alice が最も大きい)。
var (
	alice = testUser("00000000-0000-4000-8000-000000000003", "alice", "Alice")
	bob   = testUser("00000000-0000-4000-8000-000000000001", "bob", "Bob")
	carol = testUser("00000000-0000-4000-8000-000000000002", "carol", "Carol")
)

// testUser は、テスト用の利用者を組み立てる。テスト用のポートの実装が返す値であるため、復元用の関数を使う。
func testUser(id, handle, displayName string) domain.User {
	return domain.User{ID: domain.UserID(id), Handle: domain.RestoreHandle(handle), DisplayName: domain.RestoreDisplayName(displayName)}
}

// as は、テストの中で、操作を行う利用者(Actor)を返す。alice、bob、carol は、world.loginAll が置く
// ログイン状態のものである。そのほかの名前は、当たるログイン状態がない資格になる。空は、何も添えない。
func as(name string) Actor {
	if name == "" {
		return Actor{}
	}
	return Actor{Credential: name + "-credential", SessionID: "session-of-" + name}
}

const (
	sendID1 = "11111111-1111-4111-8111-111111111111"
	sendID2 = "22222222-2222-4222-8222-222222222222"
)

const (
	aliceContact = "alice@example.com"
	aliceSecret  = "alice passphrase for tests"
	bobContact   = "bob@example.com"
	bobSecret    = "bob passphrase for tests"

	source = "192.0.2.10"
)

var errBoom = errors.New("boom")

// testNow は、テストの時刻の始まりである。
var testNow = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

// fakeUsers は、port.UserDirectory の、テスト用の実装である。
type fakeUsers struct {
	users    []domain.User
	accounts map[string]fakeAccount // 連絡先から、利用者と、保管した秘密
	err      error

	handleLookups int // FindByHandle が呼ばれた回数
}

type fakeAccount struct {
	user   domain.User
	secret domain.SecretHash
}

func (f *fakeUsers) FindByContact(_ context.Context, contact domain.Contact) (domain.User, domain.SecretHash, bool, error) {
	if f.err != nil {
		return domain.User{}, "", false, f.err
	}
	account, found := f.accounts[contact.String()]
	return account.user, account.secret, found, nil
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{
		users: []domain.User{alice, bob, carol},
		accounts: map[string]fakeAccount{
			aliceContact: {user: alice, secret: fakeHashOf(aliceSecret)},
			bobContact:   {user: bob, secret: fakeHashOf(bobSecret)},
		},
	}
}

func (f *fakeUsers) FindByHandle(_ context.Context, handle domain.Handle) (domain.User, bool, error) {
	f.handleLookups++
	if f.err != nil {
		return domain.User{}, false, f.err
	}
	for _, u := range f.users {
		if u.Handle.Key() == handle.Key() {
			return u, true, nil
		}
	}
	return domain.User{}, false, nil
}

// fakeStore は、port.ConversationReader、port.Transactor、port.ConversationWriter の、
// テスト用の実装である。メモリの上に保持する。取り消しは、再現しない(ADR-012)。
type fakeStore struct {
	conversations []domain.Conversation
	messages      []domain.Message
	now           time.Time

	findDirectErr error // FindDirect が返すエラー
	listErr       error // ListMessages が返すエラー
	appendErr     error // AppendMessage が返すエラー
	commitErr     error // fn が成功した後に、InTransaction が返すエラー(確定の失敗)
	findErr       error // FindMessageBySend が返すエラー

	inTx  bool
	calls []string // トランザクションの中で呼ばれた操作

	// 登録の完了のトランザクションが使う。
	accounts      *fakeUsers
	signups       *fakeSignups
	consumeErr    error // ConsumeSignup が返すエラー
	createUserErr error // CreateUser が返すエラー
}

func newFakeStore() *fakeStore {
	return &fakeStore{now: time.Date(2026, 11, 21, 3, 4, 5, 123456000, time.UTC)}
}

func (f *fakeStore) FindDirect(_ context.Context, pair domain.DirectPair) (domain.Conversation, bool, error) {
	if f.findDirectErr != nil {
		return domain.Conversation{}, false, f.findDirectErr
	}
	for _, c := range f.conversations {
		if c.Pair == pair {
			return c, true, nil
		}
	}
	return domain.Conversation{}, false, nil
}

func (f *fakeStore) ListMessages(_ context.Context, id domain.ConversationID) ([]domain.Message, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []domain.Message
	for _, m := range f.messages {
		if m.ConversationID == id {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeStore) FindMessageBySend(_ context.Context, sender domain.UserID, sendID domain.SendID) (domain.Message, bool, error) {
	if f.findErr != nil {
		return domain.Message{}, false, f.findErr
	}
	for _, m := range f.messages {
		if m.SenderID == sender && m.SendID == sendID {
			return m, true, nil
		}
	}
	return domain.Message{}, false, nil
}

func (f *fakeStore) InTransaction(ctx context.Context, fn func(ctx context.Context, w port.ConversationWriter) error) error {
	f.inTx = true
	err := fn(ctx, f)
	f.inTx = false
	if err != nil {
		return err
	}
	return f.commitErr
}

func (f *fakeStore) EstablishDirect(ctx context.Context, pair domain.DirectPair) (domain.Conversation, error) {
	f.calls = append(f.calls, "EstablishDirect")
	if conv, found, _ := f.FindDirect(ctx, pair); found {
		return conv, nil
	}
	id := domain.ConversationID("c0000000-0000-4000-8000-00000000000" + string(rune('1'+len(f.conversations))))
	conv := domain.Conversation{ID: id, Pair: pair}
	f.conversations = append(f.conversations, conv)
	return conv, nil
}

func (f *fakeStore) SerializeAppends(_ context.Context, _ domain.ConversationID) error {
	f.calls = append(f.calls, "SerializeAppends")
	return nil
}

func (f *fakeStore) AppendMessage(_ context.Context, id domain.ConversationID, sender domain.UserID, sendID domain.SendID, body domain.Body) (domain.Message, error) {
	f.calls = append(f.calls, "AppendMessage")
	if f.appendErr != nil {
		return domain.Message{}, f.appendErr
	}
	seq := int64(1)
	for _, m := range f.messages {
		if m.SenderID == sender && m.SendID == sendID {
			return domain.Message{}, port.ErrDuplicateSend
		}
		if m.ConversationID == id {
			seq++
		}
	}
	msg := domain.Message{ConversationID: id, Seq: seq, SenderID: sender, SendID: sendID, Body: body, SentAt: f.now}
	f.messages = append(f.messages, msg)
	return msg, nil
}

// fakeDelivery は、port.MessageDelivery の、テスト用の実装である。
type fakeDelivery struct {
	store      *fakeStore
	deliveries []delivered
}

type delivered struct {
	now          time.Time
	participants []domain.User
	msg          domain.Message
	duringTx     bool
}

func (f *fakeDelivery) Deliver(_ context.Context, now time.Time, participants []domain.User, msg domain.Message) {
	f.deliveries = append(f.deliveries, delivered{now: now, participants: participants, msg: msg, duringTx: f.store.inTx})
}

// fakeRegistry は、port.ConnectionRegistry の、テスト用の実装である。
type fakeRegistry struct {
	registered   map[port.ConnectionSink]domain.Session
	unregistered []port.ConnectionSink
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{registered: map[port.ConnectionSink]domain.Session{}}
}

func (f *fakeRegistry) Register(_ context.Context, session domain.Session, sink port.ConnectionSink) {
	f.registered[sink] = session
}

func (f *fakeRegistry) Unregister(_ context.Context, sink port.ConnectionSink) {
	delete(f.registered, sink)
	f.unregistered = append(f.unregistered, sink)
}

// fakeSink は、port.ConnectionSink の、テスト用の実装である。
type fakeSink struct{}

func (*fakeSink) Send([]byte) bool   { return true }
func (*fakeSink) Close()             {}
func (*fakeSink) CloseSessionEnded() {}

// requireRejection は、err が、指定した種類の拒否であることを確かめる。
func requireRejection(t *testing.T, err error, want Rejection) {
	t.Helper()
	var rejectedErr *RejectedError
	if !errors.As(err, &rejectedErr) {
		t.Fatalf("拒否ではない: %v", err)
	}
	if rejectedErr.Kind != want {
		t.Fatalf("拒否の種類が違う: got %v, want %v", rejectedErr.Kind, want)
	}
}

// requireFailure は、err が、拒否ではないエラー(完了できなかったこと)であることを確かめる。
func requireFailure(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("エラーを返さなかった")
	}
	var rejectedErr *RejectedError
	if errors.As(err, &rejectedErr) {
		t.Fatalf("拒否として返した: %v", err)
	}
}

// InSignupTransaction は、登録の完了のトランザクションを、メモリの上で行う。
// fn がエラーを返した場合は、申し込みと利用者を、始める前の状態に戻す(取り消し)。
func (f *fakeStore) InSignupTransaction(ctx context.Context, fn func(ctx context.Context, w port.SignupWriter) error) error {
	signupsBefore := map[string]fakeSignup{}
	for k, v := range f.signups.byContact {
		signupsBefore[k] = v
	}
	accountsBefore := map[string]fakeAccount{}
	for k, v := range f.accounts.accounts {
		accountsBefore[k] = v
	}
	usersBefore := append([]domain.User(nil), f.accounts.users...)

	f.inTx = true
	err := fn(ctx, f)
	f.inTx = false
	if err == nil {
		err = f.commitErr
	}
	if err != nil {
		f.signups.byContact = signupsBefore
		f.accounts.accounts = accountsBefore
		f.accounts.users = usersBefore
	}
	return err
}

func (f *fakeStore) ConsumeSignup(_ context.Context, credential string, now time.Time) (domain.Signup, bool, error) {
	f.calls = append(f.calls, "ConsumeSignup")
	if f.consumeErr != nil {
		return domain.Signup{}, false, f.consumeErr
	}
	for contact, s := range f.signups.byContact {
		if s.credential == credential && now.Before(s.expiresAt) {
			delete(f.signups.byContact, contact)
			return domain.Signup{Contact: domain.RestoreContact(contact), ExpiresAt: s.expiresAt}, true, nil
		}
	}
	return domain.Signup{}, false, nil
}

func (f *fakeStore) CreateUser(_ context.Context, contact domain.Contact, handle domain.Handle, name domain.DisplayName, secret domain.SecretHash) (domain.User, error) {
	f.calls = append(f.calls, "CreateUser")
	if f.createUserErr != nil {
		return domain.User{}, f.createUserErr
	}
	if _, taken := f.accounts.accounts[contact.String()]; taken {
		return domain.User{}, port.ErrContactTaken
	}
	for _, u := range f.accounts.users {
		if u.Handle.Key() == handle.Key() {
			return domain.User{}, port.ErrHandleTaken
		}
	}
	user := domain.User{
		ID:          domain.UserID("00000000-0000-4000-8000-00000000010" + string(rune('0'+len(f.accounts.users)))),
		Handle:      handle,
		DisplayName: name,
	}
	f.accounts.users = append(f.accounts.users, user)
	f.accounts.accounts[contact.String()] = fakeAccount{user: user, secret: secret}
	return user, nil
}

// callLog は、複数のテスト用の実装にまたがって、呼ばれた順を覚える。
type callLog struct {
	calls []string
}

func (l *callLog) add(call string) {
	if l != nil {
		l.calls = append(l.calls, call)
	}
}

// fakeClock は、port.Clock の、テスト用の実装である。時刻は、テストから進める。
type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: testNow} }

func (f *fakeClock) Now() time.Time { return f.now }

// fakeEvents は、port.EventRecorder の、テスト用の実装である。
type fakeEvents struct {
	events []domain.Event
}

func (f *fakeEvents) Record(_ context.Context, event domain.Event) {
	f.events = append(f.events, event)
}

// kinds は、記録された出来事の種類を、記録された順に返す。
func (f *fakeEvents) kinds() []domain.EventKind {
	kinds := make([]domain.EventKind, 0, len(f.events))
	for _, e := range f.events {
		kinds = append(kinds, e.Kind)
	}
	return kinds
}

// count は、指定した種類の出来事が、記録された回数を返す。
func (f *fakeEvents) count(kind domain.EventKind) int {
	n := 0
	for _, e := range f.events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// fakeLimiter は、port.AttemptLimiter の、テスト用の実装である。
// 結果は、テストが指定する。数え方は、ratelimit のテストで確かめる。
type fakeLimiter struct {
	exceeded map[domain.LimitKind]bool // 上限に達している種類
	err      error

	admitted []domain.Limit // 数えたもの
	forgiven []domain.Limit
	reset    []domain.Limit
	admitAt  []time.Time
}

func newFakeLimiter() *fakeLimiter {
	return &fakeLimiter{exceeded: map[domain.LimitKind]bool{}}
}

func (f *fakeLimiter) Admit(_ context.Context, now time.Time, limits ...domain.Limit) (domain.Limit, bool, error) {
	if f.err != nil {
		return domain.Limit{}, false, f.err
	}
	for _, l := range limits {
		if f.exceeded[l.Kind] {
			return l, false, nil
		}
	}
	f.admitted = append(f.admitted, limits...)
	f.admitAt = append(f.admitAt, now)
	return domain.Limit{}, true, nil
}

func (f *fakeLimiter) Forgive(_ context.Context, limit domain.Limit) error {
	if f.err != nil {
		return f.err
	}
	f.forgiven = append(f.forgiven, limit)
	return nil
}

func (f *fakeLimiter) Reset(_ context.Context, limit domain.Limit) error {
	if f.err != nil {
		return f.err
	}
	f.reset = append(f.reset, limit)
	return nil
}

// fakeCredentials は、port.CredentialSource の、テスト用の実装である。credential-1 から、順に返す。
type fakeCredentials struct {
	issued []string
	err    error
}

func (f *fakeCredentials) NewCredential() (string, error) {
	if f.err != nil {
		return "", f.err
	}
	credential := "credential-" + string(rune('1'+len(f.issued)))
	f.issued = append(f.issued, credential)
	return credential, nil
}

// fakeHashOf は、fakeHasher が保管する形である。
func fakeHashOf(secret string) domain.SecretHash {
	return domain.SecretHash("hashed:" + secret)
}

// fakeHasher は、port.SecretHasher の、テスト用の実装である。計算の回数を数える。
type fakeHasher struct {
	store *fakeStore // トランザクションの中で呼ばれたかを、確かめるため
	err   error

	hashes       int
	verifies     int
	dummies      int
	hashedDuring bool
}

func (f *fakeHasher) Hash(_ context.Context, secret domain.Secret) (domain.SecretHash, error) {
	f.hashes++
	if f.store != nil && f.store.inTx {
		f.hashedDuring = true
	}
	if f.err != nil {
		return "", f.err
	}
	return fakeHashOf(secret.Expose()), nil
}

func (f *fakeHasher) Verify(_ context.Context, secret string, stored domain.SecretHash) (bool, error) {
	f.verifies++
	if f.err != nil {
		return false, f.err
	}
	return fakeHashOf(secret) == stored, nil
}

func (f *fakeHasher) VerifyDummy(context.Context) error {
	f.dummies++
	return f.err
}

// calculations は、秘密の計算を行った回数を返す。
func (f *fakeHasher) calculations() int { return f.hashes + f.verifies + f.dummies }

// fakeCommonSecrets は、port.CommonSecretList の、テスト用の実装である。
type fakeCommonSecrets map[string]bool

func (f fakeCommonSecrets) Contains(secret domain.Secret) bool { return f[secret.Expose()] }

// fakeNotices は、port.NoticeSender の、テスト用の実装である。送った案内を、覚える。
type fakeNotices struct {
	err  error
	sent []fakeNotice
}

type fakeNotice struct {
	registered bool // 登録済みであることを伝える案内
	to         string
	credential string
	expiresAt  time.Time
}

func (f *fakeNotices) SendSignupNotice(_ context.Context, to domain.Contact, credential string, expiresAt time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, fakeNotice{to: to.String(), credential: credential, expiresAt: expiresAt})
	return nil
}

func (f *fakeNotices) SendAlreadyRegisteredNotice(_ context.Context, to domain.Contact) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, fakeNotice{registered: true, to: to.String()})
	return nil
}

// fakeSessions は、port.SessionStore の、テスト用の実装である。
type fakeSessions struct {
	users *fakeUsers
	log   *callLog

	byCredential map[string]domain.Session
	created      int
	findErr      error
	createErr    error
	revokeErr    error
	deleteErr    error
	finds        int
	// failFindFrom が正の値のとき、その回数目からの Find が、エラーを返す。
	failFindFrom int
	deletedAt    []time.Time
	// afterFind は、Find が戻る直前に呼ばれる。確かめた後に、状態が変わる場合を作るために使う。
	afterFind func()
}

func newFakeSessions(users *fakeUsers) *fakeSessions {
	return &fakeSessions{users: users, byCredential: map[string]domain.Session{}}
}

func (f *fakeSessions) Create(_ context.Context, credential string, user domain.UserID, expiresAt time.Time) (domain.Session, error) {
	f.log.add("sessions.Create")
	if f.createErr != nil {
		return domain.Session{}, f.createErr
	}
	f.created++
	session := domain.Session{
		ID:        domain.SessionID("session-" + string(rune('0'+f.created))),
		UserID:    user,
		ExpiresAt: expiresAt,
	}
	f.byCredential[credential] = session
	return session, nil
}

func (f *fakeSessions) Find(_ context.Context, credential string, now time.Time) (domain.Session, domain.User, bool, error) {
	f.finds++
	if f.afterFind != nil {
		defer f.afterFind()
	}
	if f.findErr != nil {
		return domain.Session{}, domain.User{}, false, f.findErr
	}
	if f.failFindFrom > 0 && f.finds >= f.failFindFrom {
		return domain.Session{}, domain.User{}, false, errBoom
	}
	session, found := f.byCredential[credential]
	if !found || !session.ValidAt(now) {
		return domain.Session{}, domain.User{}, false, nil
	}
	for _, u := range f.users.users {
		if u.ID == session.UserID {
			return session, u, true, nil
		}
	}
	return domain.Session{}, domain.User{}, false, nil
}

func (f *fakeSessions) Revoke(_ context.Context, credential string) (domain.SessionID, bool, error) {
	f.log.add("sessions.Revoke")
	if f.revokeErr != nil {
		return "", false, f.revokeErr
	}
	session, found := f.byCredential[credential]
	delete(f.byCredential, credential)
	return session.ID, found, nil
}

func (f *fakeSessions) DeleteExpired(_ context.Context, now time.Time) error {
	f.log.add("sessions.DeleteExpired")
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedAt = append(f.deletedAt, now)
	return nil
}

// login は、テストの準備として、利用者のログイン状態を置き、操作を行う利用者(Actor)を返す。
func (f *fakeSessions) login(user domain.User, credential string) Actor {
	session, _ := f.Create(context.Background(), credential, user.ID, testNow.Add(domain.SessionLifetime))
	return Actor{Credential: credential, SessionID: string(session.ID)}
}

// fakeSignups は、port.SignupStore の、テスト用の実装である。
type fakeSignups struct {
	log *callLog

	byContact map[string]fakeSignup
	putErr    error
	findErr   error
	deleteErr error
	deletedAt []time.Time
}

type fakeSignup struct {
	credential string
	expiresAt  time.Time
}

func newFakeSignups() *fakeSignups {
	return &fakeSignups{byContact: map[string]fakeSignup{}}
}

func (f *fakeSignups) Put(_ context.Context, contact domain.Contact, credential string, expiresAt time.Time) error {
	f.log.add("signups.Put")
	if f.putErr != nil {
		return f.putErr
	}
	f.byContact[contact.String()] = fakeSignup{credential: credential, expiresAt: expiresAt}
	return nil
}

func (f *fakeSignups) Find(_ context.Context, credential string, now time.Time) (domain.Signup, bool, error) {
	if f.findErr != nil {
		return domain.Signup{}, false, f.findErr
	}
	for contact, s := range f.byContact {
		if s.credential == credential && now.Before(s.expiresAt) {
			return domain.Signup{Contact: domain.RestoreContact(contact), ExpiresAt: s.expiresAt}, true, nil
		}
	}
	return domain.Signup{}, false, nil
}

func (f *fakeSignups) DeleteExpired(_ context.Context, now time.Time) error {
	f.log.add("signups.DeleteExpired")
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedAt = append(f.deletedAt, now)
	return nil
}

// fakeConnections は、port.SessionConnections の、テスト用の実装である。
type fakeConnections struct {
	log *callLog

	closedSessions  []domain.SessionID
	closedExpiredAt []time.Time
}

func (f *fakeConnections) CloseBySession(_ context.Context, id domain.SessionID) {
	f.log.add("connections.CloseBySession")
	f.closedSessions = append(f.closedSessions, id)
}

func (f *fakeConnections) CloseExpired(_ context.Context, now time.Time) {
	f.log.add("connections.CloseExpired")
	f.closedExpiredAt = append(f.closedExpiredAt, now)
}

// requireInvalidFields は、err が、指定した項目を、この順に挙げる、InvalidFields の拒否であることを確かめる。
func requireInvalidFields(t *testing.T, err error, want ...InputField) {
	t.Helper()
	requireRejection(t, err, InvalidFields)
	var rejectedErr *RejectedError
	errors.As(err, &rejectedErr)
	if len(rejectedErr.Fields) != len(want) {
		t.Fatalf("正しくない項目が違う: got %v, want %v", rejectedErr.Fields, want)
	}
	for i := range want {
		if rejectedErr.Fields[i] != want[i] {
			t.Fatalf("正しくない項目が違う: got %v, want %v", rejectedErr.Fields, want)
		}
	}
}

// requireEvents は、記録された出来事の種類が、この順であることを確かめる。
func requireEvents(t *testing.T, events *fakeEvents, want ...domain.EventKind) {
	t.Helper()
	got := events.kinds()
	if len(got) != len(want) {
		t.Fatalf("記録された出来事が違う: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("記録された出来事が違う: got %v, want %v", got, want)
		}
	}
}

// world は、ユースケースのテストが使う、テスト用のポートの実装の一式である。
type world struct {
	log         *callLog
	delivery    *fakeDelivery
	registry    *fakeRegistry
	users       *fakeUsers
	store       *fakeStore
	sessions    *fakeSessions
	signups     *fakeSignups
	connections *fakeConnections
	credentials *fakeCredentials
	hasher      *fakeHasher
	common      fakeCommonSecrets
	notices     *fakeNotices
	limiter     *fakeLimiter
	clock       *fakeClock
	events      *fakeEvents
	rules       domain.LimitRules
	gate        *SessionGate
}

func newWorld() *world {
	log := &callLog{}
	users := newFakeUsers()
	store := newFakeStore()
	sessions := newFakeSessions(users)
	sessions.log = log
	signups := newFakeSignups()
	signups.log = log
	store.accounts = users
	store.signups = signups
	clock := newFakeClock()
	return &world{
		log:         log,
		delivery:    &fakeDelivery{store: store},
		registry:    newFakeRegistry(),
		users:       users,
		store:       store,
		sessions:    sessions,
		signups:     signups,
		connections: &fakeConnections{log: log},
		credentials: &fakeCredentials{},
		hasher:      &fakeHasher{store: store},
		common:      fakeCommonSecrets{},
		notices:     &fakeNotices{},
		limiter:     newFakeLimiter(),
		clock:       clock,
		events:      &fakeEvents{},
		rules:       domain.DefaultLimitRules(),
		gate:        NewSessionGate(sessions, clock),
	}
}

func (w *world) signupRequester() *SignupRequester {
	return NewSignupRequester(w.users, w.signups, w.credentials, w.notices, w.limiter, w.rules, w.clock, w.events)
}

func (w *world) signupCompleter() *SignupCompleter {
	return NewSignupCompleter(w.signups, w.common, w.hasher, w.store, w.limiter, w.rules, w.clock, w.events)
}

func (w *world) authenticator() *Authenticator {
	return NewAuthenticator(w.users, w.hasher, w.sessions, w.connections, w.credentials, w.limiter, w.rules, w.clock, w.events)
}

func (w *world) sessionKeeper() *SessionKeeper {
	return NewSessionKeeper(w.gate, w.sessions, w.connections, w.clock, w.events)
}

func (w *world) housekeeper() *Housekeeper {
	return NewHousekeeper(w.connections, w.sessions, w.signups, w.clock, w.events)
}

// loginAll は、alice、bob、carol の、ログイン状態を置く。as が返す Actor に当たる。
func (w *world) loginAll() *world {
	for _, u := range []domain.User{alice, bob, carol} {
		actor := as(u.Handle.String())
		w.sessions.byCredential[actor.Credential] = domain.Session{
			ID:        domain.SessionID(actor.SessionID),
			UserID:    u.ID,
			ExpiresAt: testNow.Add(domain.SessionLifetime),
		}
	}
	return w
}

func (w *world) messageSender() *MessageSender {
	return NewMessageSender(w.gate, w.users, w.store, w.store, w.delivery, w.limiter, w.rules, w.clock, w.events)
}

func (w *world) historyReader() *HistoryReader {
	return NewHistoryReader(w.gate, w.users, w.store, w.limiter, w.rules, w.clock, w.events)
}

func (w *world) peerFinder() *PeerFinder {
	return NewPeerFinder(w.gate, w.users, w.store, w.limiter, w.rules, w.clock, w.events)
}

func (w *world) connectionKeeper() *ConnectionKeeper {
	return NewConnectionKeeper(w.gate, w.registry, w.events)
}
