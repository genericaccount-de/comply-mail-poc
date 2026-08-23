package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPScanClient_Scan(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ScanResponse{
			Sensitivity: "HIGH",
			Flags:       []string{"contains credentials"},
			Action:      "redirect",
		})
	}))
	defer srv.Close()

	c := NewHTTPScanClient(srv.URL, time.Second)
	resp, err := c.Scan(context.Background(), ScanRequest{
		From:    "a@x.com",
		To:      []string{"b@y.com"},
		Subject: "s",
		Body:    "b",
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if resp.Sensitivity != "HIGH" || resp.Action != "redirect" || len(resp.Flags) != 1 {
		t.Errorf("unexpected response: %+v", resp)
	}

	wantFields := map[string]any{
		"from":    "a@x.com",
		"to":      []any{"b@y.com"},
		"subject": "s",
		"body":    "b",
	}
	for k, want := range wantFields {
		got, ok := gotBody[k]
		if !ok {
			t.Errorf("request body missing field %q", k)
			continue
		}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("request field %q = %s, want %s", k, gotJSON, wantJSON)
		}
	}
}

func TestHTTPScanClient_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewHTTPScanClient(srv.URL, time.Second)
	if _, err := c.Scan(context.Background(), ScanRequest{Subject: "s"}); err == nil {
		t.Error("expected error for non-2xx status")
	}
}

func TestHTTPScanClient_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	c := NewHTTPScanClient(srv.URL, time.Second)
	if _, err := c.Scan(context.Background(), ScanRequest{Subject: "s"}); err == nil {
		t.Error("expected error for malformed response JSON")
	}
}

func TestHTTPScanClient_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	c := NewHTTPScanClient(srv.URL, time.Millisecond)
	if _, err := c.Scan(context.Background(), ScanRequest{Subject: "s"}); err == nil {
		t.Error("expected error for timed-out request")
	}
}
