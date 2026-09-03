// Package ntp is Cronus's NTP query engine. It wraps github.com/beevik/ntp to
// sample one or more servers, aggregates the samples of each server (offset,
// round-trip delay, jitter, stratum, reference id, ...), and builds the
// cross-server comparison (median consensus, falseticker outliers, and the
// pairwise offset-delta matrix).
//
// The engine never hand-rolls the NTP packet exchange; it delegates to
// beevik/ntp and surfaces kiss-of-death codes rather than retrying. DNS is
// resolved at query time and the answering IP is recorded with every result,
// so the caller can see which pool member replied.
package ntp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	beevik "github.com/beevik/ntp"

	"github.com/t0mer/cronus/internal/stats"
)

// DefaultPort is the standard NTP UDP port used when a target omits one.
const DefaultPort = "123"

// minMonitorInterval is the hard floor between polls of a single server in
// monitoring mode, enforced regardless of configuration to be a good citizen.
const MinMonitorInterval = 15 * time.Second

// Sample is a single query/response exchange with one server.
type Sample struct {
	OK             bool          `json:"ok"`
	Offset         time.Duration `json:"offset"`
	RTT            time.Duration `json:"rtt"`
	Stratum        uint8         `json:"stratum"`
	Leap           uint8         `json:"leap"`
	Precision      time.Duration `json:"precision"`
	ReferenceID    string        `json:"reference_id"`
	RootDelay      time.Duration `json:"root_delay"`
	RootDispersion time.Duration `json:"root_dispersion"`
	KissCode       string        `json:"kiss_code,omitempty"`
	Err            string        `json:"error,omitempty"`
	At             time.Time     `json:"at"`
}

// ServerResult holds every sample for one target plus the aggregates Cronus
// reports. Aggregates are computed only over successful samples; when no sample
// succeeds Reachable is false and Error explains why.
type ServerResult struct {
	Target         string        `json:"target"`
	Host           string        `json:"host"`
	Port           string        `json:"port"`
	ResolvedIP     string        `json:"resolved_ip"`
	Reachable      bool          `json:"reachable"`
	Samples        []Sample      `json:"samples"`
	Offset         time.Duration `json:"offset"`
	RTT            time.Duration `json:"rtt"`
	Jitter         time.Duration `json:"jitter"`
	Stratum        uint8         `json:"stratum"`
	Leap           uint8         `json:"leap"`
	ReferenceID    string        `json:"reference_id"`
	Precision      time.Duration `json:"precision"`
	RootDelay      time.Duration `json:"root_delay"`
	RootDispersion time.Duration `json:"root_dispersion"`
	Error          string        `json:"error,omitempty"`
}

// Comparison is the cross-server analysis over a set of results.
type Comparison struct {
	Labels       []string          `json:"labels"`        // reachable targets, in result order
	MedianOffset time.Duration     `json:"median_offset"` // consensus across reachable servers
	Outliers     []string          `json:"outliers"`      // suspected falsetickers
	Pairwise     [][]time.Duration `json:"pairwise"`      // Pairwise[i][j] = offset[i]-offset[j]
}

// QueryFunc performs a single NTP query against an address ("ip:port"). It is
// injectable so the aggregation and error paths can be tested without real UDP.
type QueryFunc func(ctx context.Context, address string, timeout time.Duration) (*beevik.Response, error)

// Config configures an Engine.
type Config struct {
	Samples       int           // samples per server per run (clamped to >=1)
	Timeout       time.Duration // per-query timeout
	Workers       int           // max servers queried in parallel (clamped to >=1)
	SampleSpacing time.Duration // delay between samples of one server
}

// Engine runs NTP tests. The zero value is not usable; use NewEngine.
type Engine struct {
	cfg     Config
	query   QueryFunc
	resolve func(ctx context.Context, host string) (string, error)
	sleep   func(ctx context.Context, d time.Duration)
	now     func() time.Time
}

// NewEngine builds an Engine with sensible defaults filled in for any zero
// config field.
func NewEngine(cfg Config) *Engine {
	if cfg.Samples < 1 {
		cfg.Samples = 4
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Workers < 1 {
		cfg.Workers = 8
	}
	return &Engine{
		cfg:     cfg,
		query:   defaultQuery,
		resolve: defaultResolve,
		sleep:   sleepCtx,
		now:     time.Now,
	}
}

func defaultQuery(ctx context.Context, address string, timeout time.Duration) (*beevik.Response, error) {
	// beevik/ntp is blocking; run it and honour ctx cancellation.
	type res struct {
		r   *beevik.Response
		err error
	}
	ch := make(chan res, 1)
	go func() {
		r, err := beevik.QueryWithOptions(address, beevik.QueryOptions{Timeout: timeout})
		ch <- res{r, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		return out.r, out.err
	}
}

func defaultResolve(ctx context.Context, host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no addresses for %q", host)
	}
	return addrs[0], nil
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// SplitTarget splits a "host", "host:port", "ipv6", or "[ipv6]:port" target
// into host and port, defaulting the port to 123.
func SplitTarget(target string) (host, port string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", errors.New("empty target")
	}
	if h, p, e := net.SplitHostPort(target); e == nil {
		if h == "" {
			return "", "", fmt.Errorf("invalid target %q: empty host", target)
		}
		return h, p, nil
	}
	// No port present. A bare IPv6 literal parses here; anything with a stray
	// colon that isn't valid host:port was already rejected above only when it
	// had multiple colons — guard bare IPv6 explicitly.
	if ip := net.ParseIP(target); ip != nil {
		return target, DefaultPort, nil
	}
	if strings.Contains(target, " ") {
		return "", "", fmt.Errorf("invalid target %q", target)
	}
	return target, DefaultPort, nil
}

// Run queries every target with the configured number of samples, in parallel
// across targets (bounded by Workers) and sequentially per target. It always
// returns one ServerResult per target, in input order.
func (e *Engine) Run(ctx context.Context, targets []string) []ServerResult {
	results := make([]ServerResult, len(targets))
	sem := make(chan struct{}, e.cfg.Workers)
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, target string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = e.runOne(ctx, target)
		}(i, target)
	}
	wg.Wait()
	return results
}

func (e *Engine) runOne(ctx context.Context, target string) ServerResult {
	res := ServerResult{Target: target}
	host, port, err := SplitTarget(target)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Host, res.Port = host, port

	ip, err := e.resolve(ctx, host)
	if err != nil {
		res.Error = fmt.Sprintf("resolve %q: %v", host, err)
		return res
	}
	res.ResolvedIP = ip
	address := net.JoinHostPort(ip, port)

	res.Samples = make([]Sample, 0, e.cfg.Samples)
	for i := 0; i < e.cfg.Samples; i++ {
		if i > 0 {
			e.sleep(ctx, e.cfg.SampleSpacing)
		}
		if ctx.Err() != nil {
			break
		}
		res.Samples = append(res.Samples, e.sampleOnce(ctx, address))
	}
	aggregate(&res)
	return res
}

func (e *Engine) sampleOnce(ctx context.Context, address string) Sample {
	s := Sample{At: e.now()}
	resp, err := e.query(ctx, address, e.cfg.Timeout)
	if err != nil {
		s.Err = err.Error()
		return s
	}
	if verr := resp.Validate(); verr != nil {
		s.Err = verr.Error()
		if resp.IsKissOfDeath() {
			s.KissCode = resp.KissCode
		}
		return s
	}
	s.OK = true
	s.Offset = resp.ClockOffset
	s.RTT = resp.RTT
	s.Stratum = resp.Stratum
	s.Leap = uint8(resp.Leap)
	s.Precision = resp.Precision
	s.ReferenceID = resp.ReferenceString()
	s.RootDelay = resp.RootDelay
	s.RootDispersion = resp.RootDispersion
	return s
}

// aggregate fills the ServerResult aggregate fields from its samples. The
// representative offset/RTT come from the lowest-delay successful sample (the
// standard NTP choice); jitter is the standard deviation of all successful
// offsets.
func aggregate(res *ServerResult) {
	var offsets []float64
	best := -1
	for i := range res.Samples {
		s := res.Samples[i]
		if !s.OK {
			continue
		}
		offsets = append(offsets, s.Offset.Seconds())
		if best < 0 || s.RTT < res.Samples[best].RTT {
			best = i
		}
	}
	if best < 0 {
		res.Reachable = false
		if res.Error == "" {
			res.Error = firstError(res.Samples)
		}
		return
	}
	res.Reachable = true
	b := res.Samples[best]
	res.Offset = b.Offset
	res.RTT = b.RTT
	res.Stratum = b.Stratum
	res.Leap = b.Leap
	res.ReferenceID = b.ReferenceID
	res.Precision = b.Precision
	res.RootDelay = b.RootDelay
	res.RootDispersion = b.RootDispersion
	res.Jitter = time.Duration(stats.StdDev(offsets) * float64(time.Second))
}

func firstError(samples []Sample) string {
	for _, s := range samples {
		if s.Err != "" {
			if s.KissCode != "" {
				return fmt.Sprintf("kiss-of-death %s: %s", s.KissCode, s.Err)
			}
			return s.Err
		}
	}
	return "unreachable"
}

// BuildComparison computes the cross-server consensus over the reachable
// results: the median offset, the targets whose offset deviates from that
// median by more than threshold (falsetickers), and the pairwise offset-delta
// matrix. Unreachable results are excluded.
func BuildComparison(results []ServerResult, threshold time.Duration) Comparison {
	var labels []string
	var offsets []float64
	var durs []time.Duration
	for _, r := range results {
		if !r.Reachable {
			continue
		}
		labels = append(labels, r.Target)
		offsets = append(offsets, r.Offset.Seconds())
		durs = append(durs, r.Offset)
	}
	comp := Comparison{Labels: labels}
	if len(offsets) == 0 {
		return comp
	}
	comp.MedianOffset = time.Duration(stats.Median(offsets) * float64(time.Second))
	for _, idx := range stats.OutlierIndices(offsets, threshold.Seconds()) {
		comp.Outliers = append(comp.Outliers, labels[idx])
	}
	comp.Pairwise = make([][]time.Duration, len(durs))
	for i := range durs {
		comp.Pairwise[i] = make([]time.Duration, len(durs))
		for j := range durs {
			comp.Pairwise[i][j] = durs[i] - durs[j]
		}
	}
	return comp
}

// SortableOffsets returns the successful sample offsets of a result, sorted.
// Exposed for callers that want the raw distribution.
func (r ServerResult) SortableOffsets() []time.Duration {
	var out []time.Duration
	for _, s := range r.Samples {
		if s.OK {
			out = append(out, s.Offset)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
