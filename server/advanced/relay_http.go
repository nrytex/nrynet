package advanced

import (
	"encoding/json"
	"net/http"
	"strings"

	netx "github.com/nat-link/nat-link/internal/advanced"
)

type RelayAPI struct {
	registry *netx.RelayRegistry
}

type heartbeatRequest struct {
	Connections int `json:"connections"`
}

func NewRelayAPI(registry *netx.RelayRegistry) *RelayAPI {
	return &RelayAPI{registry: registry}
}

func (api *RelayAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/relays", api.handleRelays)
	mux.HandleFunc("/relays/", api.handleRelay)
	mux.HandleFunc("/assignments/", api.handleAssignment)
	return mux
}

func (api *RelayAPI) handleRelays(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var node netx.RelayNode
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		registered, err := api.registry.Register(node)
		writeJSONOrError(w, registered, err)
	case http.MethodGet:
		writeJSON(w, api.registry.Nodes())
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *RelayAPI) handleRelay(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[2] != "heartbeat" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	node, err := api.registry.Heartbeat(parts[1], request.Connections)
	writeJSONOrError(w, node, err)
}

func (api *RelayAPI) handleAssignment(w http.ResponseWriter, r *http.Request) {
	tunnelID := strings.TrimPrefix(r.URL.Path, "/assignments/")
	if tunnelID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPost:
		assignment, err := api.registry.AssignTunnel(tunnelID)
		writeJSONOrError(w, assignment, err)
	case http.MethodPatch:
		assignment, err := api.registry.ReassignTunnel(tunnelID)
		writeJSONOrError(w, assignment, err)
	case http.MethodDelete:
		api.registry.ReleaseTunnel(tunnelID)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeJSONOrError(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, value)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
