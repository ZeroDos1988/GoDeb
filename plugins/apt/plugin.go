package apt

import (
	"net/http"

	"godeb/backend/core"
)

func init() {
	core.Register("apt", Mount)
}

// Mount registers the APT routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/apt/upgradable", handleUpgradable)
}

type statusResponse struct {
	core.CommandEnvelope
	Packages []Package `json:"packages"`
}

func handleUpgradable(w http.ResponseWriter, r *http.Request) {
	stdout, env := core.ReadCommand(r.Context(), sampleUpgradable, "apt", "list", "--upgradable")
	core.WriteJSON(w, http.StatusOK, statusResponse{
		CommandEnvelope: env,
		Packages:        ParseUpgradable(stdout),
	})
}
