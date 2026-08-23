// Package proxy implements the SMTP server that accepts outbound mail,
// scans it via the ComplyMail backend, and relays it upstream — flagging or
// redirecting it first if the backend says to.
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/mail"
	"net/smtp"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"github.com/genericaccount-de/comply-mail-poc/smtp-proxy/internal/header"
)

// maxBodyBytes caps how much of the message body is sent to the backend for
// classification, bounding request size/latency (design requires <1-2s
// added latency per email).
const maxBodyBytes = 64 * 1024

// Backend implements github.com/emersion/go-smtp's Backend interface: it
// hands out a Session for each inbound SMTP connection.
type Backend struct {
	// Upstream is the "host:port" of the customer's real mail server.
	Upstream string
	// ReviewMailbox is where redirected emails are rerouted. If empty,
	// redirected emails are still header-stamped but delivered to their
	// original recipients.
	ReviewMailbox string
	Scanner       ScanClient
	Metrics       *Metrics

	// relay sends a message upstream. Defaults to relaySMTP; overridable
	// in tests.
	relay func(addr, from string, to []string, raw []byte) error
}

// NewSession implements gosmtp.Backend.
func (b *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &session{backend: b}, nil
}

func (b *Backend) doRelay(addr, from string, to []string, raw []byte) error {
	if b.relay != nil {
		return b.relay(addr, from, to, raw)
	}
	return relaySMTP(addr, from, to, raw)
}

// session implements gosmtp.Session for a single SMTP transaction.
type session struct {
	backend *Backend
	from    string
	rcpts   []string
}

func (s *session) Mail(from string, _ *gosmtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.rcpts = nil
}

func (s *session) Logout() error {
	return nil
}

// Data reads the message, scans it via the backend, and relays it upstream
// (unmodified, flagged, or redirected) based on the result.
func (s *session) Data(r io.Reader) error {
	start := time.Now()

	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("proxy: read message: %w", err)
	}

	subject, body, err := parseMessage(raw)
	if err != nil {
		return fmt.Errorf("proxy: parse message: %w", err)
	}

	b := s.backend
	result, err := b.Scanner.Scan(context.Background(), ScanRequest{
		From:    s.from,
		To:      s.rcpts,
		Subject: subject,
		Body:    body,
	})
	if err != nil {
		// Fail open: a backend outage must not block outbound mail for
		// the pilot. Relay unscanned and count it so the gap is visible.
		log.Printf("proxy: scan failed, relaying unscanned: %v", err)
		b.Metrics.RecordScanError()
		return b.doRelay(b.Upstream, s.from, s.rcpts, raw)
	}

	outRaw := raw
	rcpts := s.rcpts
	if result.Action != "pass" {
		outRaw = header.Inject(raw, result.Sensitivity, result.Flags, result.Action)
	}
	if result.Action == "redirect" && b.ReviewMailbox != "" {
		rcpts = []string{b.ReviewMailbox}
	}

	if err := b.doRelay(b.Upstream, s.from, rcpts, outRaw); err != nil {
		return fmt.Errorf("proxy: relay: %w", err)
	}
	b.Metrics.RecordScan(result.Action, time.Since(start))
	return nil
}

// parseMessage extracts the subject and body text from a raw RFC 5322
// message. Per the POC's pilot scope (English-only, no attachment content
// scanning), it does not walk MIME multipart bodies — the raw body is sent
// as-is, which is a known gap for HTML-composed mail.
func parseMessage(raw []byte) (subject, body string, err error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}

	subject = msg.Header.Get("Subject")
	if decoded, err := (&mime.WordDecoder{}).DecodeHeader(subject); err == nil {
		subject = decoded
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(msg.Body, maxBodyBytes))
	if err != nil {
		return "", "", err
	}
	return subject, string(bodyBytes), nil
}

// relaySMTP delivers raw to the given recipients at addr using stdlib
// net/smtp. It deliberately never negotiates STARTTLS (skipped for this
// POC on both SMTP legs), unlike net/smtp.SendMail which upgrades
// opportunistically if the peer advertises the extension.
func relaySMTP(addr, from string, to []string, raw []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("relay: dial %s: %w", addr, err)
	}
	defer c.Close()

	if err := c.Hello("localhost"); err != nil {
		return fmt.Errorf("relay: hello: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("relay: mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("relay: rcpt to %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("relay: data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("relay: write data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("relay: close data: %w", err)
	}

	return c.Quit()
}
