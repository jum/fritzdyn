package main

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

//go:embed templates/*.html
var templateFS embed.FS

type AdminHandler struct {
	DB *sqlx.DB
}

func NewAdminHandler(db *sqlx.DB) *AdminHandler {
	return &AdminHandler{DB: db}
}

func (h *AdminHandler) render(w http.ResponseWriter, tmplName string, data any) {
	tmpl, err := template.ParseFS(templateFS, "templates/layout.html", "templates/"+tmplName)
	if err != nil {
		slog.Error("template parse error", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, data)
	if err != nil {
		slog.Error("template execute error", "err", err)
	}
}

func (h *AdminHandler) renderBlock(w http.ResponseWriter, tmplName string, blockName string, data any) {
	tmpl, err := template.ParseFS(templateFS, "templates/"+tmplName)
	if err != nil {
		slog.Error("template parse error", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	err = tmpl.ExecuteTemplate(w, blockName, data)
	if err != nil {
		slog.Error("template execute error", "err", err)
	}
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin")
	if path == "" || path == "/" {
		h.handleHosts(w, r)
		return
	}
	if strings.HasPrefix(path, "/host/new") {
		h.handleHostNew(w, r)
		return
	}
	if strings.HasPrefix(path, "/host/") {
		token := strings.TrimPrefix(path, "/host/")
		h.handleHostEdit(w, r, token)
		return
	}
	if strings.HasPrefix(path, "/updates") {
		h.handleUpdates(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *AdminHandler) handleHosts(w http.ResponseWriter, r *http.Request) {
	var hosts []Host
	err := h.DB.Select(&hosts, "SELECT * FROM hosts ORDER BY created DESC")
	if err != nil {
		slog.Error("Select hosts", "err", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "hosts.html", map[string]any{
		"Hosts": hosts,
	})
}

func (h *AdminHandler) handleHostNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		host := Host{
			Name:   r.FormValue("name"),
			Token:  r.FormValue("token"),
			Domain: r.FormValue("domain"),
			Zone:   r.FormValue("zone"),
		}
		if host.Token == "" {
			host.Token = uuid.NewString()
		}
		
		ip4 := r.FormValue("ip4addr")
		if ip4 != "" {
			host.Ip4addr = &ip4
		}
		ip6 := r.FormValue("ip6addr")
		if ip6 != "" {
			host.Ip6addr = &ip6
		}

		_, err = h.DB.Exec("INSERT INTO hosts (token, name, domain, zone, ip4addr, ip6addr) VALUES (?, ?, ?, ?, ?, ?)",
			host.Token, host.Name, host.Domain, host.Zone, host.Ip4addr, host.Ip6addr)
		
		if err != nil {
			slog.Error("Insert host", "err", err)
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}

	h.render(w, "host_edit.html", map[string]any{
		"IsNew": true,
		"Host":  Host{},
	})
}

func (h *AdminHandler) handleHostEdit(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method == "DELETE" {
		_, err := h.DB.Exec("DELETE FROM hosts WHERE token = ?", token)
		if err != nil {
			slog.Error("Delete host", "err", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("HX-Redirect", "/admin/")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		
		name := r.FormValue("name")
		domain := r.FormValue("domain")
		zone := r.FormValue("zone")
		ip4 := r.FormValue("ip4addr")
		ip6 := r.FormValue("ip6addr")
		
		var ip4ptr, ip6ptr *string
		if ip4 != "" { ip4ptr = &ip4 }
		if ip6 != "" { ip6ptr = &ip6 }

		_, err = h.DB.Exec("UPDATE hosts SET name=?, domain=?, zone=?, ip4addr=?, ip6addr=? WHERE token=?",
			name, domain, zone, ip4ptr, ip6ptr, token)
		
		if err != nil {
			slog.Error("Update host", "err", err)
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/host/"+token, http.StatusSeeOther)
		return
	}

	var host Host
	err := h.DB.Get(&host, "SELECT * FROM hosts WHERE token = ?", token)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var updates []Update
	err = h.DB.Select(&updates, "SELECT * FROM updates WHERE token = ?", token)
	if err != nil {
		slog.Error("Select updates", "err", err)
	}

	h.render(w, "host_edit.html", map[string]any{
		"IsNew":   false,
		"Host":    host,
		"Updates": updates,
	})
}

func (h *AdminHandler) handleUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		token := r.FormValue("token")
		cmd := r.FormValue("cmd")
		args := r.FormValue("args")
		apiKey := r.FormValue("api_key")
		
		var apiKeyPtr *string
		if apiKey != "" {
			apiKeyPtr = &apiKey
		}

		res, err := h.DB.Exec("INSERT INTO updates (token, cmd, args, api_key) VALUES (?, ?, ?, ?)",
			token, cmd, args, apiKeyPtr)
		if err != nil {
			slog.Error("Insert update", "err", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		
		// Return the new row
		update := Update{
			Id: id,
			Token: token,
			Cmd: cmd,
			Args: args,
			ApiKey: apiKeyPtr,
			Modified: time.Now(),
			Created: time.Now(),
		}
		h.renderBlock(w, "host_edit.html", "update_row", update)
		return
	}

	if r.Method == "DELETE" {
		idStr := strings.TrimPrefix(r.URL.Path, "/updates/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		_, err = h.DB.Exec("DELETE FROM updates WHERE id = ?", id)
		if err != nil {
			slog.Error("Delete update", "err", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK) // HTMX will remove the element
		return
	}
}
