package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/exp/slog"
)

type Host struct {
	Token    string
	Name     string
	Ip4addr  *string
	Ip6addr  *string
	Modified time.Time
	Created  time.Time
}

type Update struct {
	Id       int64
	Token    string
	Cmd      string
	Args     string
	Modified time.Time
	Created  time.Time
}

type FritzHandler struct {
	DB *sqlx.DB
}

func NewFritzHandler() (fh *FritzHandler, err error) {
	slog.Debug("NewFritzHandler", "driver", os.Getenv("SQL_DRIVER"), "dsn", os.Getenv("SQL_DSN"))
	db, err := sqlx.Connect(os.Getenv("SQL_DRIVER"), os.Getenv("SQL_DSN"))
	if err != nil {
		return nil, err
	}
	return &FritzHandler{DB: db}, nil
}

func (fh *FritzHandler) Close() error {
	return fh.DB.Close()
}

func (fh *FritzHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		slog.Error("ParseForm", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	slog.Debug("req", "url", r.URL, "header", r.Header, "form", r.Form)
	token := r.FormValue("token")
	ipaddr := r.FormValue("ipaddr")
	ip6addr := r.FormValue("ip6addr")
	//ip6lanprefix := r.FormValue("ip6lanprefix")
	var host Host
	tx, err := fh.DB.Beginx()
	if err != nil {
		slog.Error("Beginx", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	err = tx.Get(&host, "select * FROM hosts WHERE token = ?", token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		slog.Error("Get", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("Updating", "host", host)
	modified := false
	if ipaddr != "" && (host.Ip4addr == nil || ipaddr != *host.Ip4addr) {
		modified = true
		host.Ip4addr = &ipaddr
		_, err = tx.Exec("UPDATE hosts SET ip4addr = ? WHERE token = ?", host.Ip4addr, host.Token)
	}
	if ip6addr != "" && (host.Ip6addr == nil || ip6addr != *host.Ip6addr) {
		modified = true
		host.Ip6addr = &ip6addr
		_, err = tx.Exec("UPDATE hosts SET ip6addr = ? WHERE token = ?", host.Ip6addr, host.Token)
	}
	if err != nil {
		slog.Error("Exec", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("Updating", "host", host, "modified", modified)
	if modified {
		err = tx.Commit()
		if err != nil {
			slog.Error("Get", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var data = make(map[string]any)
		data["Req"] = r
		data["Host"] = &host
		var updates []Update
		err = fh.DB.Select(&updates, "SELECT * FROM updates WHERE token = ?", host.Token)
		if err != nil {
			slog.Error("Select", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, u := range updates {
			data["Upd"] = &u
			slog.Debug("update", "data", data)
			argTempl, err := template.New("args").Parse(u.Args)
			if err != nil {
				slog.Error("template.New", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var argStr strings.Builder
			err = argTempl.Execute(&argStr, data)
			if err != nil {
				slog.Error("Execute", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			switch u.Cmd {
			case "GET":
				res, err := http.Get(argStr.String())
				if err != nil {
					slog.Error("Get", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				defer res.Body.Close()
				if res.StatusCode/100 != 2 {
					slog.Error("Get", "status", res.Status, "code", res.StatusCode)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				buf, err := io.ReadAll(res.Body)
				if err != nil {
					slog.Error("ReadAll", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				slog.Debug("Get", "url", argStr.String(), "resp", string(buf))
			case "godaddy":
			default:
				cmdTempl, err := template.New("cmd").Parse(u.Cmd)
				if err != nil {
					slog.Error("template.New", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				var cmdStr strings.Builder
				err = cmdTempl.Execute(&cmdStr, data)
				if err != nil {
					slog.Error("Execute", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				cmd := exec.Command("sh", "-c", cmdStr.String()+" \""+argStr.String()+"\"")
				stdoutStderr, err := cmd.CombinedOutput()
				if err != nil {
					slog.Error("cmd", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				slog.Debug("exec", "cmd", cmdStr.String(), "args", argStr.String(), "outerr", string(stdoutStderr))
			}
		}
		fmt.Fprintf(w, "OK modified\n")
	} else {
		fmt.Fprintf(w, "OK\n")
	}
}
