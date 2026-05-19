// Command stigman is the STIG Manager HTTP API server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Exonical/stig-manager-react/api/internal/config"
	"github.com/Exonical/stig-manager-react/api/internal/migrations"
	"github.com/Exonical/stig-manager-react/api/internal/server"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// version, commit, and buildDate are stamped by the build via -ldflags.
var (
	version   = "0.0.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger.Info("starting stigman",
		"version", version,
		"commit", commit,
		"build_date", buildDate,
		"addr", cfg.HTTPAddr,
		"oidc_issuer", cfg.OIDC.Issuer,
	)

	authProvider, err := server.BuildAuthProvider(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("auth provider: %w", err)
	}
	if authProvider == nil {
		logger.Warn("running without OIDC; protected endpoints will 401")
	}

	var (
		pool             *store.Pool
		migrationVersion int64
	)
	if cfg.DatabaseURL != "" {
		bootCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pool, err = store.Open(bootCtx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer pool.Close()
		logger.Info("database connected")
		if err := migrations.Migrate(bootCtx, pool); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
		v, err := migrations.Version(bootCtx, pool)
		if err != nil {
			return fmt.Errorf("read migration version: %w", err)
		}
		migrationVersion = v
		logger.Info("database migrated", "version", v)
	} else {
		logger.Warn("running without STIGMAN_DATABASE_URL; database-backed handlers will degrade")
	}

	srv := server.New(server.Options{
		Logger:           logger,
		Version:          version,
		Commit:           commit,
		BuildDate:        buildDate,
		Config:           cfg,
		AuthProvider:     authProvider,
		Pool:             pool,
		MigrationVersion: migrationVersion,
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, stopping http server")
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("stopped cleanly")
	return nil
}
