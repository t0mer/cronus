package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/cronus/internal/stats"
	"github.com/t0mer/cronus/internal/store"
)

// maxRawPoints bounds how many measurements are read for a chart before the
// caller is expected to supply a coarser step.
const maxRawPoints = 50000

type point struct {
	TS        time.Time `json:"ts"`
	Reachable bool      `json:"reachable"`
	Offset    float64   `json:"offset_seconds"`
	RTT       float64   `json:"rtt_seconds"`
	Jitter    float64   `json:"jitter_seconds"`
	Stratum   uint8     `json:"stratum"`
}

type measurementsResponse struct {
	ServerID string     `json:"server_id"`
	From     *time.Time `json:"from,omitempty"`
	To       *time.Time `json:"to,omitempty"`
	Step     string     `json:"step,omitempty"`
	Count    int        `json:"count"`
	Points   []point    `json:"points"`
}

func (a *API) handleMeasurements(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := a.deps.Store.GetServer(r.Context(), id); err != nil {
		notFoundOrError(w, err, "server")
		return
	}

	from, err := parseTimeParam(r, "from")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from': "+err.Error())
		return
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to': "+err.Error())
		return
	}
	var step time.Duration
	if s := r.URL.Query().Get("step"); s != "" {
		step, err = time.ParseDuration(s)
		if err != nil || step <= 0 {
			writeError(w, http.StatusBadRequest, "invalid 'step': must be a positive duration")
			return
		}
	}

	ms, err := a.deps.Store.Measurements(r.Context(), id, from, to, maxRawPoints)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read measurements")
		return
	}

	var points []point
	if step > 0 {
		points = downsample(ms, step)
	} else {
		points = make([]point, len(ms))
		for i, m := range ms {
			points[i] = toPoint(m)
		}
	}
	// Always return an array, never null — clients iterate this field, and a
	// null crashes the monitoring overlay for a server with no data yet.
	if points == nil {
		points = []point{}
	}

	resp := measurementsResponse{ServerID: id, Count: len(points), Points: points}
	if !from.IsZero() {
		resp.From = &from
	}
	if !to.IsZero() {
		resp.To = &to
	}
	if step > 0 {
		resp.Step = step.String()
	}
	writeJSON(w, http.StatusOK, resp)
}

func toPoint(m store.Measurement) point {
	return point{
		TS:        m.TS,
		Reachable: m.Reachable,
		Offset:    m.Offset.Seconds(),
		RTT:       m.RTT.Seconds(),
		Jitter:    m.Jitter.Seconds(),
		Stratum:   m.Stratum,
	}
}

// downsample averages measurements into fixed-width time buckets. A bucket is
// reachable if any of its measurements were reachable; only reachable samples
// contribute to the averaged offset/RTT/jitter.
func downsample(ms []store.Measurement, step time.Duration) []point {
	if len(ms) == 0 {
		return nil
	}
	var out []point
	var (
		bucketStart time.Time
		sumOff      float64
		sumRTT      float64
		sumJit      float64
		nReach      int
		reachable   bool
		lastStratum uint8
		have        bool
	)
	flush := func() {
		if !have {
			return
		}
		p := point{TS: bucketStart, Reachable: reachable, Stratum: lastStratum}
		if nReach > 0 {
			p.Offset = sumOff / float64(nReach)
			p.RTT = sumRTT / float64(nReach)
			p.Jitter = sumJit / float64(nReach)
		}
		out = append(out, p)
	}
	for _, m := range ms {
		b := m.TS.Truncate(step)
		if !have || !b.Equal(bucketStart) {
			flush()
			bucketStart = b
			sumOff, sumRTT, sumJit, nReach, reachable, have = 0, 0, 0, 0, false, true
		}
		if m.Reachable {
			sumOff += m.Offset.Seconds()
			sumRTT += m.RTT.Seconds()
			sumJit += m.Jitter.Seconds()
			nReach++
			reachable = true
			lastStratum = m.Stratum
		}
	}
	flush()
	return out
}

type driftResponse struct {
	ServerID    string     `json:"server_id"`
	DriftPPM    *float64   `json:"drift_ppm"`
	R2          *float64   `json:"r2,omitempty"`
	SamplesUsed int        `json:"samples_used"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`
	Message     string     `json:"message,omitempty"`
}

func (a *API) handleDrift(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := a.deps.Store.GetServer(r.Context(), id); err != nil {
		notFoundOrError(w, err, "server")
		return
	}

	window := 24 * time.Hour
	if s := r.URL.Query().Get("window"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "invalid 'window': must be a positive duration")
			return
		}
		window = d
	}
	end := time.Now().UTC()
	start := end.Add(-window)

	ms, err := a.deps.Store.Measurements(r.Context(), id, start, end, maxRawPoints)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read measurements")
		return
	}

	var times, offsets []float64
	var first time.Time
	for _, m := range ms {
		if !m.Reachable {
			continue
		}
		if first.IsZero() {
			first = m.TS
		}
		times = append(times, m.TS.Sub(first).Seconds())
		offsets = append(offsets, m.Offset.Seconds())
	}

	resp := driftResponse{ServerID: id, SamplesUsed: len(offsets), WindowStart: &start, WindowEnd: &end}
	ppm, reg, ok := stats.DriftPPM(times, offsets)
	if !ok {
		resp.Message = "not enough reachable measurements to compute drift"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.DriftPPM = &ppm
	resp.R2 = &reg.R2
	writeJSON(w, http.StatusOK, resp)
}

func parseTimeParam(r *http.Request, key string) (time.Time, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, v)
}

func notFoundOrError(w http.ResponseWriter, err error, what string) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, what+" not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "failed to load "+what)
}
