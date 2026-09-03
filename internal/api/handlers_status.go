package api

import (
	"net/http"
	"time"

	"github.com/t0mer/cronus/internal/store"
	"github.com/t0mer/cronus/internal/version"
)

type statusResponse struct {
	Version          string        `json:"version"`
	UptimeSeconds    float64       `json:"uptime_seconds"`
	SchedulerRunning bool          `json:"scheduler_running"`
	DB               store.DBStats `json:"db"`
	Now              time.Time     `json:"now"`
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	dbStats, err := a.deps.Store.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read status")
		return
	}
	uptime := time.Duration(0)
	if !a.deps.StartTime.IsZero() {
		uptime = time.Since(a.deps.StartTime)
	}
	writeJSON(w, http.StatusOK, statusResponse{
		Version:          version.String(),
		UptimeSeconds:    uptime.Seconds(),
		SchedulerRunning: a.deps.SchedulerRunning(),
		DB:               dbStats,
		Now:              time.Now().UTC(),
	})
}
