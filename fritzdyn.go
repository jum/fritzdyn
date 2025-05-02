package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/libdns/cloudflare"
	"github.com/libdns/libdns"
	_ "modernc.org/sqlite"
)

type Host struct {
	Token    string
	Name     string
	Domain   string
	Zone     string
	Ip4addr  *string
	Ip6addr  *string
	Modified time.Time
	Created  time.Time
}

type Update struct {
	Id       int64
	ApiKey   string `db:"api_key"`
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
	ctx := r.Context()
	err := r.ParseForm()
	if err != nil {
		slog.ErrorContext(ctx, "ParseForm", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	slog.DebugContext(ctx, "req", "url", r.URL, "header", r.Header, "form", r.Form)
	token := r.FormValue("token")
	ipaddr := r.FormValue("ipaddr")
	ip6addr := r.FormValue("ip6addr")
	ip6lanprefix := r.FormValue("ip6lanprefix")
	ether := r.FormValue("ether")
	domain := r.FormValue("domain")
	if len(ip6addr) == 0 && len(ip6lanprefix) > 0 && len(ether) > 0 {
		prefix, err := netip.ParsePrefix(ip6lanprefix)
		if err != nil {
			slog.ErrorContext(ctx, "ParsePrefix", "ip6lanprefix", ip6lanprefix, "err", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if !prefix.Addr().Is6() {
			slog.ErrorContext(ctx, "is not ip6", "prefix", prefix.String())
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		mac, err := net.ParseMAC(ether)
		if err != nil {
			slog.ErrorContext(ctx, "ParseMAC", "err", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		// make ip6addr the EUI ipv6 from prefix and ether
		if prefix.Bits() == -1 || prefix.Bits() > 64 {
			slog.ErrorContext(ctx, "bad prefix", "prefix", prefix.String())
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		// MAC must be in EUI-48 or EUI64 form.
		if len(mac) != 6 && len(mac) != 8 {
			slog.ErrorContext(ctx, "is not EUI-48 or EUI64", "mac", mac.String())
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		pbytes := prefix.Addr().As16()
		var ip [16]byte
		copy(ip[0:8], pbytes[0:8])

		// Flip 7th bit from left on the first byte of the MAC address, the
		// "universal/local (U/L)" bit.  See RFC 4291, Section 2.5.1 for more
		// information.

		// If MAC is in EUI-64 form, directly copy it into output IP address.
		if len(mac) == 8 {
			copy(ip[8:16], mac)
			ip[8] ^= 0x02
		} else {
			// If MAC is in EUI-48 form, split first three bytes and last three bytes,
			// and inject 0xff and 0xfe between them.
			copy(ip[8:11], mac[0:3])
			ip[8] ^= 0x02
			ip[11] = 0xff
			ip[12] = 0xfe
			copy(ip[13:16], mac[3:6])
		}
		ip6addr = netip.AddrFrom16(ip).String()
	}
	var host Host
	tx, err := fh.DB.Beginx()
	if err != nil {
		slog.ErrorContext(ctx, "Beginx", "err", err)
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
		slog.ErrorContext(ctx, "Get", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.DebugContext(ctx, "Updating", "host", host)
	if domain != host.Domain {
		slog.ErrorContext(ctx, "domain does not match", "domain_request", domain, "domain_update", host.Domain)
		http.Error(w, "Configured domain does not match", http.StatusForbidden)
		return
	}

	modified := false
	if ipaddr != "" && (host.Ip4addr == nil || ipaddr != *host.Ip4addr) {
		modified = true
		host.Ip4addr = &ipaddr
		_, err = tx.Exec("UPDATE hosts SET ip4addr = ? WHERE token = ?", host.Ip4addr, host.Token)
	}
	if err != nil {
		slog.ErrorContext(ctx, "Exec", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ip6addr != "" && (host.Ip6addr == nil || ip6addr != *host.Ip6addr) {
		modified = true
		host.Ip6addr = &ip6addr
		_, err = tx.Exec("UPDATE hosts SET ip6addr = ? WHERE token = ?", host.Ip6addr, host.Token)
	}
	if err != nil {
		slog.ErrorContext(ctx, "Exec", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.DebugContext(ctx, "Updating", "host", host, "modified", modified)
	if modified {
		var data = make(map[string]any)
		data["Req"] = r
		data["Host"] = &host
		var updates []Update
		err = tx.Select(&updates, "SELECT * FROM updates WHERE token = ?", host.Token)
		if err != nil {
			slog.ErrorContext(ctx, "Select", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, u := range updates {
			data["Upd"] = &u
			//slog.Debug("update", "data", data)
			argTempl, err := template.New("args").Parse(u.Args)
			if err != nil {
				slog.ErrorContext(ctx, "template.New", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var argStr strings.Builder
			err = argTempl.Execute(&argStr, data)
			if err != nil {
				slog.ErrorContext(ctx, "Execute", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			switch u.Cmd {
			case "GET":
				res, err := http.Get(argStr.String())
				if err != nil {
					slog.ErrorContext(ctx, "Get", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				defer res.Body.Close()
				if res.StatusCode/100 != 2 {
					slog.ErrorContext(ctx, "Get", "status", res.Status, "code", res.StatusCode)
					http.Error(w, res.Status, http.StatusInternalServerError)
					return
				}
				buf, err := io.ReadAll(res.Body)
				if err != nil {
					slog.ErrorContext(ctx, "ReadAll", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				slog.InfoContext(ctx, "Get", "url", argStr.String(), "resp", string(buf))
			case "cloudflare":
				apiKey := os.Getenv(u.ApiKey)
				if len(apiKey) == 0 {
					slog.ErrorContext(ctx, "api_key not set")
					continue
				}
				clfupdate := &cloudflare.Provider{APIToken: apiKey}
				sub := libdns.RelativeName(host.Domain, host.Zone)
				recs := []libdns.Record{
					libdns.Address{
						Name: sub,
						IP:   netip.MustParseAddr(*host.Ip4addr),
					},
				}
				if host.Ip6addr != nil {
					recs = append(recs, libdns.Address{
						Name: sub,
						IP:   netip.MustParseAddr(*host.Ip6addr),
					})
				}
				slog.DebugContext(ctx, "cloudflare SetRecords", "recs", recs)
				newRecs, err := clfupdate.SetRecords(ctx, host.Zone, recs)
				if err != nil {
					slog.ErrorContext(ctx, "cloudflare SetRecords", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				slog.InfoContext(ctx, "SetRecords", "zone", argStr.String(), "newRecs", newRecs)
			default:
				cmdTempl, err := template.New("cmd").Parse(u.Cmd)
				if err != nil {
					slog.ErrorContext(ctx, "template.New", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				var cmdStr strings.Builder
				err = cmdTempl.Execute(&cmdStr, data)
				if err != nil {
					slog.ErrorContext(ctx, "Execute", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				cmd := exec.Command("sh", "-c", cmdStr.String()+" \""+argStr.String()+"\"")
				stdoutStderr, err := cmd.CombinedOutput()
				if err != nil {
					slog.ErrorContext(ctx, "cmd", "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				slog.DebugContext(ctx, "exec", "cmd", cmdStr.String(), "args", argStr.String(), "outerr", string(stdoutStderr))
			}
		}
		err = tx.Commit()
		if err != nil {
			slog.ErrorContext(ctx, "Commit", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "OK modified\n")
	} else {
		fmt.Fprintf(w, "OK\n")
	}
}
