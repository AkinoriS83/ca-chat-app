package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/httpapi"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/wsaccept"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// ports は、組み立てに使う、ポートの実装である。
// テストは、差し替えた実装を渡す(WS-31、WS-33、RL-10、RL-23)。
type ports struct {
	Users         port.UserDirectory
	Reader        port.ConversationReader
	Tx            port.Transactor
	Sessions      port.SessionStore
	Signups       port.SignupStore
	Delivery      port.MessageDelivery
	Registry      port.ConnectionRegistry
	Connections   port.SessionConnections
	Credentials   port.CredentialSource
	Hasher        port.SecretHasher
	CommonSecrets port.CommonSecretList
	Notices       port.NoticeSender
	Limiter       port.AttemptLimiter
	Clock         port.Clock
	Events        port.EventRecorder
}

// assembled は、組み立てた結果である。
type assembled struct {
	// Handler は、サーバーの http.Handler である。
	Handler http.Handler
	// Housekeeper は、片付けのユースケースである。main は、一定の間隔で呼ぶ adapter に渡す。
	// テストは、直接に呼ぶ。
	Housekeeper *usecase.Housekeeper
}

// assemble は、ポートの実装、上限の値、ログの出力先を受け取って、ユースケースと、サーバーの http.Handler を組み立てる。
//
// main は、組み立てだけを行う。adapter には、そのパッケージが import してよい層のものだけを渡す(ADR-005)。
// ユースケースを呼ぶ adapter(httpapi、wsaccept、schedule)には、ユースケースだけを渡し、ポートの実装を渡さない。
// logger は、接続への書き込みの失敗を出力する、接続の受け付けの adapter だけが受け取る(ADR-021)。
func assemble(p ports, limits domain.LimitRules, appOrigin string, logger *slog.Logger) (assembled, error) {
	// ログイン状態の確認は、1つにまとめ、認証を要するユースケースに渡す(ADR-019)。
	gate := usecase.NewSessionGate(p.Sessions, p.Clock)

	usecases := httpapi.Usecases{
		SignupRequester: usecase.NewSignupRequester(p.Users, p.Signups, p.Credentials, p.Notices, p.Limiter, limits, p.Clock, p.Events),
		SignupCompleter: usecase.NewSignupCompleter(p.Signups, p.CommonSecrets, p.Hasher, p.Tx, p.Limiter, limits, p.Clock, p.Events),
		Authenticator:   usecase.NewAuthenticator(p.Users, p.Hasher, p.Sessions, p.Connections, p.Credentials, p.Limiter, limits, p.Clock, p.Events),
		SessionKeeper:   usecase.NewSessionKeeper(gate, p.Sessions, p.Connections, p.Clock, p.Events),
		PeerFinder:      usecase.NewPeerFinder(gate, p.Users, p.Reader, p.Limiter, limits, p.Clock, p.Events),
		MessageSender:   usecase.NewMessageSender(gate, p.Users, p.Reader, p.Tx, p.Delivery, p.Limiter, limits, p.Clock, p.Events),
		HistoryReader:   usecase.NewHistoryReader(gate, p.Users, p.Reader, p.Limiter, limits, p.Clock, p.Events),
	}
	keeper := usecase.NewConnectionKeeper(gate, p.Registry, p.Events)
	housekeeper := usecase.NewHousekeeper(p.Connections, p.Sessions, p.Signups, p.Clock, p.Events)

	realtime, err := wsaccept.NewHandler(keeper, appOrigin, logger)
	if err != nil {
		return assembled{}, err
	}

	mux := http.NewServeMux()
	httpapi.NewHandler(usecases).Register(mux)
	mux.Handle(http.MethodGet+" "+wsaccept.Path, realtime)
	// 定義されていないパスと方法の、受け皿。
	mux.Handle("/", httpapi.NotFoundHandler())

	// Host と Origin の検証で、HTTP の操作と、WebSocket の接続先の両方を包む。
	// 正規の形でないパスは、ServeMux より前で拒否する。
	requireAppHost, err := httpapi.RequireAppHost(appOrigin)
	if err != nil {
		return assembled{}, err
	}
	handler := requireAppHost(httpapi.RequireAppOrigin(appOrigin)(httpapi.RequireCanonicalPath(mux)))
	return assembled{Handler: handler, Housekeeper: housekeeper}, nil
}

// newHTTPServer は、handler を使う http.Server を作る。
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// "OPTIONS *" にも、Go の既定の応答ではなく、handler の応答を返す。
		DisableGeneralOptionsHandler: true,
	}
}
