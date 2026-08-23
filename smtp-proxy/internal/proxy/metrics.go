package proxy

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Metrics tracks basic counters for the SMTP proxy, per the design's
// "maintain basic metrics" requirement. A plain mutex is sufficient at the
// POC's target throughput (10 emails/minute) — no need for atomics.
type Metrics struct {
	mu           sync.Mutex
	total        int64
	flagged      int64
	redirected   int64
	scanErrors   int64
	totalLatency time.Duration
}

// NewMetrics builds an empty Metrics store.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordScan records the outcome and latency of one completed scan+relay.
func (m *Metrics) RecordScan(action string, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	m.totalLatency += latency
	switch action {
	case "flag":
		m.flagged++
	case "redirect":
		m.redirected++
	}
}

// RecordScanError records a failed backend scan call (the message is still
// relayed, fail-open).
func (m *Metrics) RecordScanError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	m.scanErrors++
}

// MetricsSnapshot is the JSON shape returned by the metrics endpoint.
type MetricsSnapshot struct {
	Total        int64 `json:"total"`
	Flagged      int64 `json:"flagged"`
	Redirected   int64 `json:"redirected"`
	ScanErrors   int64 `json:"scan_errors"`
	AvgLatencyMs int64 `json:"avg_latency_ms"`
}

// Snapshot returns a point-in-time copy of the counters.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	var avgMs int64
	if m.total > 0 {
		avgMs = (m.totalLatency / time.Duration(m.total)).Milliseconds()
	}

	return MetricsSnapshot{
		Total:        m.total,
		Flagged:      m.flagged,
		Redirected:   m.redirected,
		ScanErrors:   m.scanErrors,
		AvgLatencyMs: avgMs,
	}
}

// Handler serves the current snapshot as JSON.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.Snapshot())
	})
}
