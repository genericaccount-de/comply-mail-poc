package proxy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	gosmtp "github.com/emersion/go-smtp"
)

// fakeScanner is a test double for ScanClient.
type fakeScanner struct {
	result ScanResponse
	err    error
}

func (f *fakeScanner) Scan(_ context.Context, _ ScanRequest) (ScanResponse, error) {
	return f.result, f.err
}

const testMessage = "Subject: hi\r\nFrom: a@x.com\r\nTo: b@y.com\r\n\r\nhello there"

func newSession(be *Backend, from string, rcpts []string) *session {
	return &session{backend: be, from: from, rcpts: rcpts}
}

func TestData_Pass(t *testing.T) {
	var relayedTo []string
	var relayedRaw []byte
	be := &Backend{
		Upstream: "upstream:25",
		Scanner:  &fakeScanner{result: ScanResponse{Sensitivity: "LOW", Action: "pass"}},
		Metrics:  NewMetrics(),
		relay: func(addr, from string, to []string, raw []byte) error {
			relayedTo, relayedRaw = to, raw
			return nil
		},
	}
	s := newSession(be, "a@x.com", []string{"b@y.com"})

	if err := s.Data(strings.NewReader(testMessage)); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	if len(relayedTo) != 1 || relayedTo[0] != "b@y.com" {
		t.Errorf("relayed to %v, want original recipient", relayedTo)
	}
	if !bytes.Equal(relayedRaw, []byte(testMessage)) {
		t.Error("pass action must relay the message unmodified")
	}
	if got := be.Metrics.Snapshot().Total; got != 1 {
		t.Errorf("metrics total = %d, want 1", got)
	}
}

func TestData_Flag(t *testing.T) {
	var relayedTo []string
	var relayedRaw []byte
	be := &Backend{
		Upstream: "upstream:25",
		Scanner: &fakeScanner{result: ScanResponse{
			Sensitivity: "MEDIUM", Flags: []string{"internal only"}, Action: "flag",
		}},
		Metrics: NewMetrics(),
		relay: func(addr, from string, to []string, raw []byte) error {
			relayedTo, relayedRaw = to, raw
			return nil
		},
	}
	s := newSession(be, "a@x.com", []string{"b@y.com"})

	if err := s.Data(strings.NewReader(testMessage)); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	if len(relayedTo) != 1 || relayedTo[0] != "b@y.com" {
		t.Errorf("relayed to %v, want original recipient", relayedTo)
	}
	if !bytes.Contains(relayedRaw, []byte("X-ComplyMail-Sensitivity: MEDIUM")) {
		t.Error("flag action must inject ComplyMail headers")
	}
	if snap := be.Metrics.Snapshot(); snap.Flagged != 1 {
		t.Errorf("metrics flagged = %d, want 1", snap.Flagged)
	}
}

func TestData_RedirectWithReviewMailbox(t *testing.T) {
	var relayedTo []string
	var relayedRaw []byte
	be := &Backend{
		Upstream:      "upstream:25",
		ReviewMailbox: "review@example.com",
		Scanner: &fakeScanner{result: ScanResponse{
			Sensitivity: "HIGH", Flags: []string{"credentials"}, Action: "redirect",
		}},
		Metrics: NewMetrics(),
		relay: func(addr, from string, to []string, raw []byte) error {
			relayedTo, relayedRaw = to, raw
			return nil
		},
	}
	s := newSession(be, "a@x.com", []string{"b@y.com"})

	if err := s.Data(strings.NewReader(testMessage)); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	if len(relayedTo) != 1 || relayedTo[0] != "review@example.com" {
		t.Errorf("relayed to %v, want review mailbox only", relayedTo)
	}
	if !bytes.Contains(relayedRaw, []byte("X-ComplyMail-Action: redirect")) {
		t.Error("redirect action must inject ComplyMail headers")
	}
	if snap := be.Metrics.Snapshot(); snap.Redirected != 1 {
		t.Errorf("metrics redirected = %d, want 1", snap.Redirected)
	}
}

func TestData_RedirectWithoutReviewMailbox(t *testing.T) {
	var relayedTo []string
	be := &Backend{
		Upstream:      "upstream:25",
		ReviewMailbox: "",
		Scanner:       &fakeScanner{result: ScanResponse{Sensitivity: "HIGH", Action: "redirect"}},
		Metrics:       NewMetrics(),
		relay: func(addr, from string, to []string, raw []byte) error {
			relayedTo = to
			return nil
		},
	}
	s := newSession(be, "a@x.com", []string{"b@y.com"})

	if err := s.Data(strings.NewReader(testMessage)); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	if len(relayedTo) != 1 || relayedTo[0] != "b@y.com" {
		t.Errorf("relayed to %v, want original recipient when no review mailbox configured", relayedTo)
	}
}

func TestData_ScanErrorFailsOpen(t *testing.T) {
	var relayedRaw []byte
	relayCalled := false
	be := &Backend{
		Upstream: "upstream:25",
		Scanner:  &fakeScanner{err: errors.New("backend down")},
		Metrics:  NewMetrics(),
		relay: func(addr, from string, to []string, raw []byte) error {
			relayCalled = true
			relayedRaw = raw
			return nil
		},
	}
	s := newSession(be, "a@x.com", []string{"b@y.com"})

	if err := s.Data(strings.NewReader(testMessage)); err != nil {
		t.Fatalf("Data() error = %v, want nil (fail open)", err)
	}

	if !relayCalled {
		t.Error("expected message to be relayed despite scan error (fail open)")
	}
	if !bytes.Equal(relayedRaw, []byte(testMessage)) {
		t.Error("fail-open relay must send the message unmodified")
	}
	if snap := be.Metrics.Snapshot(); snap.ScanErrors != 1 {
		t.Errorf("metrics scan_errors = %d, want 1", snap.ScanErrors)
	}
}

// compile-time check that Backend satisfies gosmtp.Backend.
var _ gosmtp.Backend = (*Backend)(nil)
