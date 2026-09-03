package api

import (
	"net/http"
	"time"

	"github.com/t0mer/cronus/internal/settings"
)

type settingsBody struct {
	MonitorInterval  string `json:"monitor_interval"`
	Retention        string `json:"retention"`
	OutlierThreshold string `json:"outlier_threshold"`
}

func toBody(v settings.Values) settingsBody {
	return settingsBody{
		MonitorInterval:  v.MonitorInterval.String(),
		Retention:        v.Retention.String(),
		OutlierThreshold: v.OutlierThreshold.String(),
	}
}

func (a *API) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, toBody(a.deps.Settings.Get()))
}

func (a *API) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body settingsBody
	if err := decodeJSON(w, r, &body, maxBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	interval, err := time.ParseDuration(body.MonitorInterval)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid monitor_interval: "+err.Error())
		return
	}
	retention, err := time.ParseDuration(body.Retention)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid retention: "+err.Error())
		return
	}
	threshold, err := time.ParseDuration(body.OutlierThreshold)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid outlier_threshold: "+err.Error())
		return
	}
	v := settings.Values{MonitorInterval: interval, Retention: retention, OutlierThreshold: threshold}
	if err := a.deps.Settings.Update(r.Context(), v); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toBody(a.deps.Settings.Get()))
}
