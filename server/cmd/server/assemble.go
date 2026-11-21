package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/httpapi"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/wsaccept"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/application/usecase"
)

// ports は、組み立てに使う、ポートの実装である。
// テストは、差し替えた実装を渡す(WS-31、WS-33)。
type ports struct {
	Users    port.UserDirectory
	Reader   port.ConversationReader
	Tx       port.Transactor
	Delivery port.MessageDelivery
	Registry port.ConnectionRegistry
}

// newHandler は、ポートの実装と、ログの出力先を受け取って、サーバーの http.Handler を組み立てる。
//
// main は、組み立てだけを行う。adapter には、そのパッケージが import してよい層のものだけを渡す(ADR-005)。
// ユースケースを呼ぶ adapter(httpapi、wsaccept)には、ユースケースだけを渡し、ポートの実装を渡さない。
func newHandler(p ports, appOrigin string, logger *slog.Logger) (http.Handler, error) {
	sender := usecase.NewMessageSender(p.Users, p.Reader, p.Tx, p.Delivery)
	history := usecase.NewHistoryReader(p.Users, p.Reader)
	keeper := usecase.NewConnectionKeeper(p.Users, p.Registry)

	realtime, err := wsaccept.NewHandler(keeper, appOrigin, logger)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	httpapi.NewHandler(sender, history, logger).Register(mux)
	mux.Handle(http.MethodGet+" "+wsaccept.Path, realtime)
	// 定義されていないパスと方法の、受け皿。
	mux.Handle("/", httpapi.NotFoundHandler())

	// Origin の検証で、HTTP の操作と、WebSocket の接続先の両方を包む。
	// 正規の形でないパスは、ServeMux より前で拒否する。
	return httpapi.RequireAppOrigin(appOrigin)(httpapi.RequireCanonicalPath(mux)), nil
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
