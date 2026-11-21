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

	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/fixeduser"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/postgres"
	"github.com/AkinoriS83/ca-chat-app/server/internal/adapter/wsdelivery"
)

func main() {
	// ログは、標準出力に出力する。要求の本文、メッセージの本文、保管先のエラーの文言は、出力しない(WS-33)。
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
	handler, err := newHandler(ports{
		Users:    fixeduser.NewDirectory(), // 仮実装(WS-P1)
		Reader:   store,
		Tx:       store,
		Delivery: hub,
		Registry: hub,
	}, cfg.AppOrigin, logger)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("%s で、待ち受けられません", cfg.ListenAddr)
	}
	server := newHTTPServer(cfg.ListenAddr, handler)

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
