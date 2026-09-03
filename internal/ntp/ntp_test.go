package ntp

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	beevik "github.com/beevik/ntp"
)

func TestSplitTarget(t *testing.T) {
	tests := []struct {
		in      string
		host    string
		port    string
		wantErr bool
	}{
		{"time.cloudflare.com", "time.cloudflare.com", "123", false},
		{"time.google.com:123", "time.google.com", "123", false},
		{"10.0.0.1", "10.0.0.1", "123", false},
		{"10.0.0.1:5123", "10.0.0.1", "5123", false},
		{"2001:db8::1", "2001:db8::1", "123", false},
		{"[2001:db8::1]:123", "2001:db8::1", "123", false},
		{"  pool.ntp.org  ", "pool.ntp.org", "123", false},
		{"", "", "", true},
		{"host name", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			host, port, err := SplitTarget(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SplitTarget(%q) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("SplitTarget(%q) unexpected error: %v", tt.in, err)
			}
			if host != tt.host || port != tt.port {
				t.Fatalf("SplitTarget(%q) = %q,%q want %q,%q", tt.in, host, port, tt.host, tt.port)
			}
		})
	}
}

// validResp constructs a *beevik.Response that passes Validate() with the given
// offset, RTT and stratum.
func validResp(offset, rtt time.Duration, stratum uint8) *beevik.Response {
	now := time.Now()
	return &beevik.Response{
		ClockOffset:    offset,
		RTT:            rtt,
		Stratum:        stratum,
		Leap:           0,
		Precision:      -20,
		Time:           now,
		ReferenceTime:  now.Add(-time.Hour),
		RootDelay:      time.Millisecond,
		RootDispersion: time.Millisecond,
	}
}

func TestAggregatePicksLowestRTTAndComputesJitter(t *testing.T) {
	res := ServerResult{Samples: []Sample{
		{OK: true, Offset: 10 * time.Millisecond, RTT: 40 * time.Millisecond, Stratum: 2},
		{OK: true, Offset: 12 * time.Millisecond, RTT: 20 * time.Millisecond, Stratum: 2}, // best (lowest RTT)
		{OK: true, Offset: 14 * time.Millisecond, RTT: 60 * time.Millisecond, Stratum: 2},
	}}
	aggregate(&res)
	if !res.Reachable {
		t.Fatal("expected reachable")
	}
	if res.Offset != 12*time.Millisecond {
		t.Fatalf("Offset = %v, want 12ms (lowest-RTT sample)", res.Offset)
	}
	if res.RTT != 20*time.Millisecond {
		t.Fatalf("RTT = %v, want 20ms", res.RTT)
	}
	// Offsets 10,12,14 ms -> stddev = 2ms/sqrt? population stddev of {10,12,14} = sqrt(8/3)*... check:
	// mean=12, devs -2,0,2 -> sumsq=8 -> /3 -> sqrt(2.6667)=1.63299ms
	if d := res.Jitter - 1632993*time.Nanosecond; d < -50*time.Microsecond || d > 50*time.Microsecond {
		t.Fatalf("Jitter = %v, want ~1.633ms", res.Jitter)
	}
}

func TestAggregateUnreachable(t *testing.T) {
	res := ServerResult{Samples: []Sample{
		{OK: false, Err: "i/o timeout"},
		{OK: false, Err: "i/o timeout"},
	}}
	aggregate(&res)
	if res.Reachable {
		t.Fatal("expected unreachable")
	}
	if res.Error == "" {
		t.Fatal("expected an error message on unreachable result")
	}
}

func TestBuildComparison(t *testing.T) {
	results := []ServerResult{
		{Target: "a", Reachable: true, Offset: 10 * time.Millisecond},
		{Target: "b", Reachable: true, Offset: 12 * time.Millisecond},
		{Target: "c", Reachable: true, Offset: 11 * time.Millisecond},
		{Target: "d", Reachable: true, Offset: 500 * time.Millisecond}, // falseticker
		{Target: "e", Reachable: false},                                // excluded
	}
	comp := BuildComparison(results, 100*time.Millisecond)
	if len(comp.Labels) != 4 {
		t.Fatalf("labels = %v, want 4 reachable", comp.Labels)
	}
	// median of 10,11,12,500 = (11+12)/2 = 11.5ms
	if comp.MedianOffset != 11500*time.Microsecond {
		t.Fatalf("median = %v, want 11.5ms", comp.MedianOffset)
	}
	if len(comp.Outliers) != 1 || comp.Outliers[0] != "d" {
		t.Fatalf("outliers = %v, want [d]", comp.Outliers)
	}
	// pairwise[0][1] = 10 - 12 = -2ms
	if comp.Pairwise[0][1] != -2*time.Millisecond {
		t.Fatalf("pairwise[0][1] = %v, want -2ms", comp.Pairwise[0][1])
	}
}

func TestBuildComparisonAllUnreachable(t *testing.T) {
	comp := BuildComparison([]ServerResult{{Reachable: false}}, 100*time.Millisecond)
	if len(comp.Labels) != 0 || comp.MedianOffset != 0 || comp.Outliers != nil {
		t.Fatalf("expected empty comparison, got %+v", comp)
	}
}

// newTestEngine returns an engine whose query/resolve/sleep are injected.
func newTestEngine(cfg Config, q QueryFunc) *Engine {
	e := NewEngine(cfg)
	e.query = q
	e.resolve = func(ctx context.Context, host string) (string, error) { return "203.0.113.1", nil }
	e.sleep = func(ctx context.Context, d time.Duration) {}
	return e
}

func TestRunHappyPath(t *testing.T) {
	e := newTestEngine(Config{Samples: 3, Workers: 4}, func(ctx context.Context, addr string, to time.Duration) (*beevik.Response, error) {
		return validResp(15*time.Millisecond, 25*time.Millisecond, 1), nil
	})
	results := e.Run(context.Background(), []string{"time.cloudflare.com", "time.google.com"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if !r.Reachable {
			t.Fatalf("%s unreachable: %s", r.Target, r.Error)
		}
		if len(r.Samples) != 3 {
			t.Fatalf("%s got %d samples, want 3", r.Target, len(r.Samples))
		}
		if r.Offset != 15*time.Millisecond {
			t.Fatalf("%s offset = %v, want 15ms", r.Target, r.Offset)
		}
		if r.ResolvedIP != "203.0.113.1" {
			t.Fatalf("%s resolved IP = %q", r.Target, r.ResolvedIP)
		}
	}
}

func TestRunTimeoutMarksUnreachable(t *testing.T) {
	e := newTestEngine(Config{Samples: 2, Workers: 1}, func(ctx context.Context, addr string, to time.Duration) (*beevik.Response, error) {
		return nil, errors.New("read udp: i/o timeout")
	})
	results := e.Run(context.Background(), []string{"unreachable.example"})
	if results[0].Reachable {
		t.Fatal("expected unreachable on timeout")
	}
	if results[0].Error == "" {
		t.Fatal("expected error text")
	}
}

func TestRunKissOfDeath(t *testing.T) {
	e := newTestEngine(Config{Samples: 1, Workers: 1}, func(ctx context.Context, addr string, to time.Duration) (*beevik.Response, error) {
		return &beevik.Response{Stratum: 0, KissCode: "RATE"}, nil
	})
	results := e.Run(context.Background(), []string{"busy.example"})
	if results[0].Reachable {
		t.Fatal("KoD should not be reachable")
	}
	if results[0].Samples[0].KissCode != "RATE" {
		t.Fatalf("KissCode = %q, want RATE", results[0].Samples[0].KissCode)
	}
}

func TestRunResolveFailure(t *testing.T) {
	e := NewEngine(Config{Samples: 1})
	e.query = func(ctx context.Context, addr string, to time.Duration) (*beevik.Response, error) {
		t.Fatal("query should not be called when resolution fails")
		return nil, nil
	}
	e.resolve = func(ctx context.Context, host string) (string, error) { return "", errors.New("no such host") }
	e.sleep = func(ctx context.Context, d time.Duration) {}
	results := e.Run(context.Background(), []string{"nope.invalid"})
	if results[0].Reachable || results[0].Error == "" {
		t.Fatalf("expected resolve failure, got %+v", results[0])
	}
}

// --- mock UDP NTP responder (integration, real beevik client path) ---

const ntpEpochOffset = 2208988800 // seconds between 1900 and 1970

func toNTP(t time.Time) uint64 {
	secs := uint64(t.Unix()) + ntpEpochOffset
	frac := uint64(t.Nanosecond()) << 32 / 1e9
	return secs<<32 | frac
}

type mockMode int

const (
	mockValid mockMode = iota
	mockKoD
	mockGarbage
	mockSilent
)

// startMockNTP starts a UDP NTP server on loopback that replies per mode. skew
// is added to the server's receive/transmit timestamps to simulate a clock
// offset. It returns the "ip:port" address and a stop func.
func startMockNTP(t *testing.T, mode mockMode, skew time.Duration) (string, func()) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			if mode == mockSilent {
				continue
			}
			if mode == mockGarbage {
				_, _ = conn.WriteToUDP([]byte{0x00, 0x01, 0x02}, raddr)
				continue
			}
			if n < 48 {
				continue
			}
			resp := make([]byte, 48)
			// LI=0, VN=4, Mode=4 (server)
			resp[0] = 0<<6 | 4<<3 | 4
			resp[1] = 2 // stratum
			if mode == mockKoD {
				resp[0] = 0<<6 | 4<<3 | 4
				resp[1] = 0 // stratum 0 => kiss of death
				copy(resp[12:16], []byte("RATE"))
			} else {
				copy(resp[12:16], []byte{203, 0, 113, 9})
			}
			resp[2] = 4                                        // poll
			resp[3] = 0xEC                                     // precision -20
			binary.BigEndian.PutUint32(resp[4:8], 0x00000100)  // root delay ~0.004s
			binary.BigEndian.PutUint32(resp[8:12], 0x00000100) // root dispersion
			now := time.Now()
			ref := now.Add(-time.Hour)
			binary.BigEndian.PutUint64(resp[16:24], toNTP(ref))
			// originate = echo of client's transmit timestamp (request bytes 40:48)
			copy(resp[24:32], buf[40:48])
			binary.BigEndian.PutUint64(resp[32:40], toNTP(now.Add(skew))) // receive
			binary.BigEndian.PutUint64(resp[40:48], toNTP(now.Add(skew))) // transmit
			_, _ = conn.WriteToUDP(resp, raddr)
		}
	}()
	return conn.LocalAddr().String(), func() {
		close(done)
		conn.Close()
	}
}

func TestEngineAgainstMockServerValid(t *testing.T) {
	skew := 250 * time.Millisecond
	addr, stop := startMockNTP(t, mockValid, skew)
	defer stop()

	e := NewEngine(Config{Samples: 2, Timeout: time.Second, Workers: 1})
	results := e.Run(context.Background(), []string{addr})
	r := results[0]
	if !r.Reachable {
		t.Fatalf("mock server unreachable: %s", r.Error)
	}
	if r.Stratum != 2 {
		t.Fatalf("stratum = %d, want 2", r.Stratum)
	}
	// offset should be close to the injected skew on loopback.
	if r.Offset < skew-60*time.Millisecond || r.Offset > skew+60*time.Millisecond {
		t.Fatalf("offset = %v, want ~%v", r.Offset, skew)
	}
}

func TestEngineAgainstMockServerKoD(t *testing.T) {
	addr, stop := startMockNTP(t, mockKoD, 0)
	defer stop()
	e := NewEngine(Config{Samples: 1, Timeout: time.Second, Workers: 1})
	r := e.Run(context.Background(), []string{addr})[0]
	if r.Reachable {
		t.Fatal("KoD server should be unreachable")
	}
	if r.Samples[0].KissCode != "RATE" {
		t.Fatalf("KissCode = %q, want RATE", r.Samples[0].KissCode)
	}
}

func TestEngineAgainstMockServerGarbage(t *testing.T) {
	addr, stop := startMockNTP(t, mockGarbage, 0)
	defer stop()
	e := NewEngine(Config{Samples: 1, Timeout: 500 * time.Millisecond, Workers: 1})
	r := e.Run(context.Background(), []string{addr})[0]
	if r.Reachable {
		t.Fatal("garbage response should be unreachable")
	}
}

func TestEngineAgainstMockServerTimeout(t *testing.T) {
	addr, stop := startMockNTP(t, mockSilent, 0)
	defer stop()
	e := NewEngine(Config{Samples: 1, Timeout: 300 * time.Millisecond, Workers: 1})
	start := time.Now()
	r := e.Run(context.Background(), []string{addr})[0]
	if r.Reachable {
		t.Fatal("silent server should time out -> unreachable")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
}
