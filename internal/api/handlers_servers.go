package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/cronus/internal/ntp"
	"github.com/t0mer/cronus/internal/store"
)

type serverRequest struct {
	Address string `json:"address"`
	Label   string `json:"label"`
	Enabled *bool  `json:"enabled"`
}

func (a *API) handleListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := a.deps.Store.ListServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}
	if servers == nil {
		servers = []store.Server{}
	}
	writeJSON(w, http.StatusOK, servers)
}

func (a *API) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var req serverRequest
	if err := decodeJSON(w, r, &req, maxBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	addr := strings.TrimSpace(req.Address)
	if _, _, err := ntp.SplitTarget(addr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid server address: "+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	created, err := a.deps.Store.CreateServer(r.Context(), store.Server{
		Address: addr,
		Label:   strings.TrimSpace(req.Label),
		Enabled: enabled,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create server")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) handleGetServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := a.deps.Store.GetServer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get server")
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (a *API) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req serverRequest
	if err := decodeJSON(w, r, &req, maxBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	addr := strings.TrimSpace(req.Address)
	if _, _, err := ntp.SplitTarget(addr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid server address: "+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updated, err := a.deps.Store.UpdateServer(r.Context(), store.Server{
		ID:      id,
		Address: addr,
		Label:   strings.TrimSpace(req.Label),
		Enabled: enabled,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update server")
		return
	}
	// Forget metrics series if the address changed or the server was disabled.
	if a.deps.Metrics != nil && !enabled {
		a.deps.Metrics.ForgetServer(id, addr)
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Capture the address first so we can drop its metric series.
	srv, _ := a.deps.Store.GetServer(r.Context(), id)
	err := a.deps.Store.DeleteServer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete server")
		return
	}
	if a.deps.Metrics != nil {
		a.deps.Metrics.ForgetServer(id, srv.Address)
	}
	w.WriteHeader(http.StatusNoContent)
}
