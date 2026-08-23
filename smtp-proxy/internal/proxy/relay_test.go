package proxy

import (
	"bytes"
	"io"
	"net"
	"testing"

	gosmtp "github.com/emersion/go-smtp"
)

// capturingBackend is a minimal gosmtp.Backend that records the single
// message it receives, used to stand in for a fake upstream mail server.
type capturingBackend struct {
	from string
	to   []string
	raw  []byte
}

func (b *capturingBackend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &capturingSession{backend: b}, nil
}

type capturingSession struct{ backend *capturingBackend }

func (s *capturingSession) Mail(from string, _ *gosmtp.MailOptions) error {
	s.backend.from = from
	return nil
}
func (s *capturingSession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.backend.to = append(s.backend.to, to)
	return nil
}
func (s *capturingSession) Reset()        {}
func (s *capturingSession) Logout() error { return nil }
func (s *capturingSession) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.backend.raw = raw
	return nil
}

func TestRelaySMTP(t *testing.T) {
	be := &capturingBackend{}
	srv := gosmtp.NewServer(be)
	srv.Domain = "localhost"
	srv.AllowInsecureAuth = true

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(l)
	defer srv.Close()

	// Ends in CRLF, like a real RFC 5322 message body: net/smtp's DotWriter
	// appends one on Close if the payload doesn't already end in a newline,
	// which would otherwise make this message-preservation check flaky.
	raw := []byte("Subject: hi\r\nFrom: a@x.com\r\nTo: b@y.com\r\n\r\nhello\r\n")
	if err := relaySMTP(l.Addr().String(), "a@x.com", []string{"b@y.com"}, raw); err != nil {
		t.Fatalf("relaySMTP() error = %v", err)
	}

	if be.from != "a@x.com" {
		t.Errorf("upstream got from = %q, want a@x.com", be.from)
	}
	if len(be.to) != 1 || be.to[0] != "b@y.com" {
		t.Errorf("upstream got to = %v, want [b@y.com]", be.to)
	}
	if !bytes.Equal(be.raw, raw) {
		t.Errorf("upstream got raw = %q, want %q", be.raw, raw)
	}
}
