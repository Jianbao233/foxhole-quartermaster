package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Jianbao233/foxhole-quartermaster/internal/config"
	"github.com/Jianbao233/foxhole-quartermaster/internal/server"
	"github.com/Jianbao233/foxhole-quartermaster/internal/store"
)

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err, "path", *cfgPath)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		slog.Error("mkdir db dir", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(cfg.AssetsDir, "icons"), 0o755); err != nil {
		slog.Error("mkdir icons", "err", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	if err := st.Seed(); err != nil {
		slog.Error("seed", "err", err)
		os.Exit(1)
	}
	if err := st.EnsureOwner(cfg.OwnerSteam64ID); err != nil {
		slog.Error("ensure owner", "err", err)
		os.Exit(1)
	}

	h := server.New(cfg, st, webFS())

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		slog.Info("listening", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	_ = srv.Shutdown(shutdownCtx)
}