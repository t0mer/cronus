// Package metrics exposes Cronus's Prometheus metrics: per-monitored-server
// gauges (offset, RTT, jitter, stratum, reachability) plus scheduler internals.
// It uses a private registry so /metrics reports only Cronus and standard
// Go/process collectors.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/t0mer/cronus/internal/ntp"
)

// Metrics holds Cronus's Prometheus collectors.
type Metrics struct {
	reg *prometheus.Registry

	offset    *prometheus.GaugeVec
	rtt       *prometheus.GaugeVec
	jitter    *prometheus.GaugeVec
	stratum   *prometheus.GaugeVec
	reachable *prometheus.GaugeVec

	pollsTotal  prometheus.Counter
	prunedTotal prometheus.Counter
	lastPoll    prometheus.Gauge
	monitored   prometheus.Gauge
}

var serverLabels = []string{"id", "server"}

// New builds and registers the metrics on a private registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := &Metrics{
		reg: reg,
		offset: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cronus_offset_seconds",
			Help: "Clock offset of a monitored server relative to the local clock, in seconds.",
		}, serverLabels),
		rtt: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cronus_rtt_seconds",
			Help: "Round-trip delay to a monitored server, in seconds.",
		}, serverLabels),
		jitter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cronus_jitter_seconds",
			Help: "Offset jitter (stddev across samples) for a monitored server, in seconds.",
		}, serverLabels),
		stratum: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cronus_stratum",
			Help: "Reported stratum of a monitored server.",
		}, serverLabels),
		reachable: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cronus_reachable",
			Help: "Whether a monitored server was reachable at the last poll (1) or not (0).",
		}, serverLabels),
		pollsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cronus_polls_total",
			Help: "Total number of monitoring poll cycles executed.",
		}),
		prunedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cronus_measurements_pruned_total",
			Help: "Total number of measurements deleted by housekeeping.",
		}),
		lastPoll: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cronus_last_poll_timestamp_seconds",
			Help: "Unix timestamp of the last completed monitoring poll cycle.",
		}),
		monitored: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cronus_monitored_servers",
			Help: "Number of enabled monitored servers at the last poll cycle.",
		}),
	}
	reg.MustRegister(m.offset, m.rtt, m.jitter, m.stratum, m.reachable,
		m.pollsTotal, m.prunedTotal, m.lastPoll, m.monitored)
	return m
}

// Handler returns the /metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// ObserveServer records the latest result for a monitored server.
func (m *Metrics) ObserveServer(id string, r ntp.ServerResult) {
	lbl := prometheus.Labels{"id": id, "server": r.Target}
	if r.Reachable {
		m.reachable.With(lbl).Set(1)
		m.offset.With(lbl).Set(r.Offset.Seconds())
		m.rtt.With(lbl).Set(r.RTT.Seconds())
		m.jitter.With(lbl).Set(r.Jitter.Seconds())
		m.stratum.With(lbl).Set(float64(r.Stratum))
	} else {
		m.reachable.With(lbl).Set(0)
	}
}

// ForgetServer removes all series for a deleted/disabled server.
func (m *Metrics) ForgetServer(id, target string) {
	lbl := prometheus.Labels{"id": id, "server": target}
	m.offset.Delete(lbl)
	m.rtt.Delete(lbl)
	m.jitter.Delete(lbl)
	m.stratum.Delete(lbl)
	m.reachable.Delete(lbl)
}

// PollCompleted updates cycle-level metrics.
func (m *Metrics) PollCompleted(monitored int, unixSeconds float64) {
	m.pollsTotal.Inc()
	m.monitored.Set(float64(monitored))
	m.lastPoll.Set(unixSeconds)
}

// Pruned records the number of measurements deleted by housekeeping.
func (m *Metrics) Pruned(n int) {
	if n > 0 {
		m.prunedTotal.Add(float64(n))
	}
}
