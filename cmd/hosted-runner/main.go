// Command hosted-runner is the private Cloud Run execution service for hosted
// v1 (#63). It exposes POST /v1/execute and GET /healthz only.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openclaw/gogcli/internal/controlplane"
	"github.com/openclaw/gogcli/internal/hosted/runner"
)

const (
	defaultPort            = "8080"
	defaultTimeout         = 20 * time.Second
	defaultMaxRequestBytes = 64 * 1024
	defaultMaxResponse     = 256 * 1024
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	secret := os.Getenv("GOG_RUNNER_INVOCATION_TOKEN")
	if len(secret) < 32 {
		logger.Error("GOG_RUNNER_INVOCATION_TOKEN must be at least 32 bytes")
		os.Exit(1)
	}

	engine := runner.Engine{
		Discoverer:       controlplane.EngineDiscoverer{},
		Secret:           secret,
		Counter:          nil,
		Timeout:          timeoutFromEnv(logger),
		MaxRequestBytes:  int64FromEnv("GOG_RUNNER_MAX_REQUEST_BYTES", defaultMaxRequestBytes),
		MaxResponseBytes: intFromEnv("GOG_RUNNER_MAX_RESPONSE_BYTES", defaultMaxResponse),
		Logger:           logger,
	}
	server := &runner.Server{Engine: &engine}

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      timeoutFromEnv(logger) + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	logger.Info("hosted runner listening", "port", port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err.Error())
		os.Exit(1)
	}
}

func timeoutFromEnv(logger *slog.Logger) time.Duration {
	raw := os.Getenv("GOG_RUNNER_TIMEOUT")
	if raw == "" {
		return defaultTimeout
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 || duration > time.Minute {
		logger.Error("invalid GOG_RUNNER_TIMEOUT")
		os.Exit(1)
	}

	return duration
}

func int64FromEnv(name string, fallback int64) int64 {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		os.Exit(1)
	}

	return value
}

func intFromEnv(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		os.Exit(1)
	}

	return value
}
