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
	"github.com/felixge/httpsnoop"
	slogtp "github.com/jum/slog-traceparent"
	"github.com/jum/traceparent"
	"github.com/jussi-kalliokoski/slogdriver"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	shutdownTimeout = 30 * time.Second
)

func main() {
	otelShutdown, prop, err := setupOTEL(context.Background())
	if err != nil {
		slog.Error("setupOTEL", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := otelShutdown(context.Background()); err != nil {
			slog.Error("otel shutdown", "err", err)
		}
	}()
	var traceMiddleware func(http.Handler) http.Handler
	debug := os.Getenv("NODE_ENV") == "development"
	access_log := os.Getenv("ACCESS_LOG") == "true"
	level := new(slog.LevelVar) // Info by default
	if debug {
		level.Set(slog.LevelDebug)
	}
	var shandler slog.Handler
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if len(projectID) > 0 {
		traceMiddleware = traceparent.New
		shandler = slogdriver.NewHandler(os.Stderr, slogdriver.Config{
			Level:     level,
			ProjectID: projectID,
		})
	} else {
		traceMiddleware = slogtp.New
		shandler = slogctx.NewHandler(
			slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
				Level: level,
			}),
			&slogctx.HandlerOptions{
				// Prependers stays as default (leaving as nil would accomplish the same)
				Prependers: []slogctx.AttrExtractor{
					slogctx.ExtractPrepended,
				},
				// Appenders first appends anything added with slogctx.Append,
				// then appends our custom ctx value
				Appenders: []slogctx.AttrExtractor{
					slogctx.ExtractAppended,
					slogtp.TraceParentExtractor,
				},
			},
		)
	}
	logger := slog.New(shandler)
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
	ah := NewAdminHandler(fh.DB)
	mux.Handle("/admin/", ah)
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
		h := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := httpsnoop.CaptureMetrics(h, w, r)
			slog.InfoContext(r.Context(), "handled request", "method", r.Method, "URL", r.URL.String(), "status", m.Code, "duration", float64(m.Duration)/float64(time.Second), "size", m.Written)
		})
	}
	handler = traceMiddleware(handler)
	if os.Getenv("ENABLE_OTEL") == "true" {
		handler = otelhttp.NewHandler(handler, "server",
			otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
				return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			}),
			otelhttp.WithPropagators(prop),
		)
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
