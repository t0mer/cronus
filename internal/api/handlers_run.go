package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/t0mer/cronus/internal/ntp"
)

type testRequest struct {
	Servers []string `json:"servers"`
	Samples int      `json:"samples"`
}

type testResponse struct {
	Results    []ntp.ServerResult `json:"results"`
	Comparison ntp.Comparison     `json:"comparison"`
}

func (a *API) handleTest(w http.ResponseWriter, r *http.Request) {
	var req testRequest
	if err := decodeJSON(w, r, &req, maxBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if len(req.Servers) == 0 {
		writeError(w, http.StatusBadRequest, "at least one server is required")
		return
	}
	if len(req.Servers) > maxTestServers {
		writeError(w, http.StatusBadRequest, "too many servers (max 20 per request)")
		return
	}
	targets := make([]string, 0, len(req.Servers))
	for _, s := range req.Servers {
		s = strings.TrimSpace(s)
		if _, _, err := ntp.SplitTarget(s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid server address "+strconv.Quote(s)+": "+err.Error())
			return
		}
		targets = append(targets, s)
	}
	if req.Samples != 0 && (req.Samples < 1 || req.Samples > 10) {
		writeError(w, http.StatusBadRequest, "samples must be between 1 and 10")
		return
	}

	results := a.deps.Engine.RunWithSamples(r.Context(), targets, req.Samples)
	comp := ntp.BuildComparison(results, a.outlierThreshold())
	writeJSON(w, http.StatusOK, testResponse{Results: results, Comparison: comp})
}

// outlierThreshold returns the live threshold from settings when available,
// falling back to the static configured value.
func (a *API) outlierThreshold() time.Duration {
	if a.deps.Settings != nil {
		return a.deps.Settings.Get().OutlierThreshold
	}
	return a.deps.OutlierThreshold
}
