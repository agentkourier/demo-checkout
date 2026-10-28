// Command checkout is the checkout service: it confirms orders once payments
// has authorized them.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/agentkourier/demo-checkout/client"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "checkout", "version", version)
	if err := run(logger); err != nil {
		logger.Error("checkout exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	paymentsURL := os.Getenv("PAYMENTS_URL")
	if paymentsURL == "" {
		return errors.New("PAYMENTS_URL is required")
	}
	payments := client.NewPayments(paymentsURL, &http.Client{Timeout: 5 * time.Second})

	var ready atomic.Bool
	srv := &http.Server{
		Addr:              addr,
		Handler:           newRouter(logger, payments, &ready),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr, "payments_url", paymentsURL)
		errc <- srv.ListenAndServe()
	}()
	ready.Store(true)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newRouter(logger *slog.Logger, payments authorizer, ready *atomic.Bool) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/checkout", &checkoutHandler{logger: logger, payments: payments})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
