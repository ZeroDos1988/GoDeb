package postgres

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"godeb/backend/core"
)

func init() {
	core.Register("postgres", Mount)
}

const base = "/api/plugins/postgres"

// Mount registers the PostgreSQL routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+base+"/status", handleStatus)
	mux.HandleFunc("POST "+base+"/databases", handleCreateDB)
	mux.HandleFunc("POST "+base+"/databases/{name}/delete", handleDropDB)
	mux.HandleFunc("POST "+base+"/roles", handleCreateRole)
	mux.HandleFunc("POST "+base+"/roles/{name}/delete", handleDropRole)
	mux.HandleFunc("GET "+base+"/databases/{name}/tables", handleTables)
	mux.HandleFunc("GET "+base+"/databases/{name}/tables/{table}", handleBrowse)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	active := strings.TrimSpace(core.RunQuiet(r.Context(), "systemctl", "is-active", "postgresql").Stdout)
	dbs, errDB := listDatabases(r.Context())
	roles, errRoles := listRoles(r.Context())
	mocked := errDB != nil || errRoles != nil
	detail := ""
	if mocked {
		dbs = []string{"app", "postgres"}
		roles = []string{"app", "postgres"}
		if errDB != nil {
			detail = errDB.Error()
		} else if errRoles != nil {
			detail = errRoles.Error()
		}
	}
	accounts, _ := listAccounts()
	known := map[string]bool{}
	for _, a := range accounts {
		known[a.Name] = true
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"active":   active,
		"databases": dbs,
		"roles":    roles,
		"managed":  known,
		"mocked":   mocked,
		"detail":   detail,
		"listen":   "localhost (Ports nie öffentlich)",
	})
}

func handleCreateDB(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Owner string `json:"owner"`
	}
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	if err := createDatabase(r.Context(), req.Name, req.Owner); err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "PostgreSQL-Datenbank angelegt", "name", req.Name, "owner", req.Owner)
	core.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func handleDropDB(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := dropDatabase(r.Context(), name); err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "PostgreSQL-Datenbank gelöscht", "name", name)
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	if err := createRole(r.Context(), req.Name, req.Password); err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, _ := cleanIdent(req.Name, "Benutzer")
	_ = saveAccount(n, req.Password, time.Now().UTC().Format(time.RFC3339))
	slog.InfoContext(r.Context(), "PostgreSQL-Rolle angelegt", "name", n)
	core.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func handleDropRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := dropRole(r.Context(), name); err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, _ := cleanIdent(name, "Benutzer")
	_ = deleteAccount(n)
	slog.InfoContext(r.Context(), "PostgreSQL-Rolle gelöscht", "name", n)
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleTables(w http.ResponseWriter, r *http.Request) {
	tables, err := listTables(r.Context(), r.PathValue("name"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

func handleBrowse(w http.ResponseWriter, r *http.Request) {
	cols, rows, err := browseTable(r.Context(), r.PathValue("name"), r.PathValue("table"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"columns": cols, "rows": rows})
}
