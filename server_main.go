//go:build server

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/exp/slog"

	"github.com/gorilla/handlers"
)

const (
	shutdownTimeout = 30 * time.Second
)

func main() {
	isDevelopment := os.Getenv("NODE_ENV") == "development"
	level := new(slog.LevelVar) // Info by default
	if isDevelopment {
		level.Set(slog.LevelDebug)
	}
	logger := slog.New(slog.HandlerOptions{Level: level}.NewJSONHandler(os.Stdout))
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
	var handler http.Handler = mux
	if isDevelopment {
		handler = handlers.CombinedLoggingHandler(os.Stdout, mux)
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
