//go:build cgi

package main

import (
	"fmt"
	"log/slog"
	"log/syslog"
	"net/http/cgi"
	"os"

	slogsyslog "github.com/samber/slog-syslog"
)

func main() {
	isDevelopment := os.Getenv("NODE_ENV") == "development"
	level := new(slog.LevelVar) // Info by default
	if isDevelopment {
		level.Set(slog.LevelDebug)
	}
	syslogger, err := syslog.New(syslog.LOG_USER|syslog.LOG_DEBUG, "fritzdyn")
	if err != nil {
		fmt.Fprintf(os.Stderr, "syslog.New: %v\n", err)
		os.Exit(1)
	}
	logger := slog.New(slogsyslog.Option{Level: level, Writer: syslogger}.NewSyslogHandler())
	slog.SetDefault(logger)
	fh, err := NewFritzHandler()
	if err != nil {
		slog.Error("NewFritzHandler", "err", err)
		os.Exit(1)
	}
	defer fh.Close()
	err = cgi.Serve(fh)
	if err != nil {
		slog.Error("cgi.Serve", "err", err)
		os.Exit(1)
	}
}
