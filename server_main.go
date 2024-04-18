//go:build server

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alexliesenfeld/health"
	"github.com/gorilla/handlers"
)

const (
	shutdownTimeout = 30 * time.Second
)

func main() {
	debug := os.Getenv("NODE_ENV") == "development"
	access_log := os.Getenv("ACCESS_LOG") == "true"
	level := new(slog.LevelVar) // Info by default
	if debug {
		level.Set(slog.LevelDebug)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3050"
		slog.Debug("Defaulting", "port", port)
	}
	var network string
	var addr string
	if strings.HasPrefix(port, "/") {
		network = "unix"
		addr = port
		err := os.Remove(addr)
		if err != nil && !os.IsNotExist(err) {
			slog.Error("remove unix socket", "err", err)
		}
		defer os.Remove(addr)
		slog.Info("Listening", "addr", addr)
	} else {
		network = "tcp"
		addr = fmt.Sprintf(":%s", port)
		slog.Info("Listening", "port", port, "url", fmt.Sprintf("http://localhost:%s/", port))
	}
	mux := http.NewServeMux()
	fh, err := NewFritzHandler()
	if err != nil {
		slog.Error("NewFritzHandler", "err", err)
		os.Exit(1)
	}
	defer fh.Close()
	mux.Handle("/", fh)
	checker := health.NewChecker(
		health.WithCheck(health.Check{
			Name:    "database",      // A unique check name.
			Timeout: 2 * time.Second, // A check specific timeout.
			Check:   fh.DB.PingContext,
		}),
	)
	mux.Handle("/health", health.NewHandler(checker))
	var handler http.Handler = mux
	if access_log {
		handler = handlers.CombinedLoggingHandler(os.Stderr, mux)
	}
	srv := http.Server{
		Addr:    addr,
		Handler: handler,
	}
	listener, err := net.Listen(network, addr)
	if err != nil {
		slog.Error("Listen", "err", err)
		os.Exit(1)
	}
	if network == "unix" {
		err := os.Chmod(addr, 0666)
		if err != nil {
			slog.Error("chmod", "addr", addr, "err", err)
			os.Exit(1)
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := srv.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				slog.Error("Serve", "err", err)
				os.Exit(1)
			}
		}
	}()
	<-stop
	slog.Debug("Shutdown")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Shutdown", "err", err)
		os.Exit(1)
	}
}
