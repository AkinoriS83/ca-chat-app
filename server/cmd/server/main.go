// Command server は、チャットのサーバーである。
// 設定の読み込み、各層の組み立て、サーバーの起動を行う(ADR-005 の main)。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/argon2id"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/commonsecrets"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/eventlog"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/ratelimit"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/schedule"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/smtpmail"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/system"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/wsdelivery"
)

func main() {
	// ログは、標準出力に出力する。要求の本文、メッセージの本文、保管先のエラーの文言は、出力しない(WS-33)。
	// 秘密、資格、連絡先、識別名、表示名も、出力しない(RL-33)。
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server stopped with error", "error", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 保管先への接続情報は、pgx が、環境変数(PGHOST など)から読む。
	pool, err := pgxpool.New(ctx, "")
	if err != nil {
		return errors.New("保管先への接続の設定を、読めません")
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		// 保管先が返したエラーの文言は、出力しない。
		return errors.New("保管先に接続できません。保管先が起動していること(make db-up)と、.env の接続情報を確かめてください")
	}

	store := postgres.NewStore(pool)
	hub := wsdelivery.NewHub(logger)
	app, err := assemble(ports{
		Users:         store,
		Reader:        store,
		Tx:            store,
		Sessions:      store.Sessions(),
		Signups:       store.Signups(),
		Delivery:      hub,
		Registry:      hub,
		Connections:   hub,
		Credentials:   system.CredentialSource{},
		Hasher:        argon2id.NewHasher(),
		CommonSecrets: commonsecrets.NewList(),
		Notices:       smtpmail.NewSender(cfg.SMTPAddr, cfg.MailFrom, cfg.AppOrigin),
		Limiter:       ratelimit.NewLimiter(),
		Clock:         system.Clock{},
		Events:        eventlog.NewRecorder(logger),
	}, cfg.Limits, cfg.AppOrigin, logger)
	if err != nil {
		return err
	}

	// 片付けを、起動の直後と、以後、一定の間隔で行う。終了の合図で、止まる。
	go schedule.NewRunner(app.Housekeeper).Run(ctx)

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("%s で、待ち受けられません", cfg.ListenAddr)
	}
	server := newHTTPServer(cfg.ListenAddr, app.Handler)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	logger.Info("server started", "listen_addr", cfg.ListenAddr, "app_origin", cfg.AppOrigin)

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	logger.Info("server stopped")
	return nil
}
