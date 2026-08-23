package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxErrBodyBytes bounds how much of an error response body is included in
// error messages.
const maxErrBodyBytes = 512

// ScanRequest is the JSON body sent to the backend's /scan-outbound-email
// endpoint. Field names/casing must match docs/api-spec.yaml exactly.
type ScanRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

// ScanResponse is the JSON body returned by /scan-outbound-email.
type ScanResponse struct {
	Sensitivity string   `json:"sensitivity"`
	Flags       []string `json:"flags"`
	Action      string   `json:"action"`
}

// ScanClient scans an outbound email via the ComplyMail backend. Defining it
// as an interface lets proxy dispatch logic be tested without a live
// backend.
type ScanClient interface {
	Scan(ctx context.Context, req ScanRequest) (ScanResponse, error)
}

// HTTPScanClient is a ScanClient that calls the backend's REST API.
type HTTPScanClient struct {
	baseURL string
	http    *http.Client
}

// NewHTTPScanClient builds an HTTPScanClient. timeout bounds each scan
// request, acting as a hard ceiling so a slow backend can't hang a session.
func NewHTTPScanClient(baseURL string, timeout time.Duration) *HTTPScanClient {
	return &HTTPScanClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

// Scan calls POST /scan-outbound-email on the backend.
func (c *HTTPScanClient) Scan(ctx context.Context, req ScanRequest) (ScanResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ScanResponse{}, fmt.Errorf("scan: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/scan-outbound-email", bytes.NewReader(body))
	if err != nil {
		return ScanResponse{}, fmt.Errorf("scan: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ScanResponse{}, fmt.Errorf("scan: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		return ScanResponse{}, fmt.Errorf("scan: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var out ScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ScanResponse{}, fmt.Errorf("scan: decode response: %w", err)
	}
	return out, nil
}
