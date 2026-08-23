package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"github.com/genericaccount-de/comply-mail-poc/smtp-proxy/internal/config"
	"github.com/genericaccount-de/comply-mail-poc/smtp-proxy/internal/proxy"
)

const (
	smtpReadTimeout  = 30 * time.Second
	smtpWriteTimeout = 30 * time.Second
	maxMessageBytes  = 25 * 1024 * 1024 // 25MB
	shutdownTimeout  = 10 * time.Second
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	scanClient := proxy.NewHTTPScanClient(cfg.Backend.APIURL, time.Duration(cfg.Backend.TimeoutSeconds)*time.Second)
	metrics := proxy.NewMetrics()
	be := &proxy.Backend{
		Upstream:      fmt.Sprintf("%s:%d", cfg.Upstream.Host, cfg.Upstream.Port),
		ReviewMailbox: cfg.Review.Mailbox,
		Scanner:       scanClient,
		Metrics:       metrics,
	}

	smtpSrv := gosmtp.NewServer(be)
	smtpSrv.Addr = cfg.ListenAddr
	smtpSrv.Domain = "localhost"
	smtpSrv.ReadTimeout = smtpReadTimeout
	smtpSrv.WriteTimeout = smtpWriteTimeout
	smtpSrv.MaxMessageBytes = maxMessageBytes
	// No STARTTLS/AUTH for this POC (see DESIGN.md follow-up decision):
	// the proxy and upstream are assumed to be on a trusted internal
	// network for the pilot. AllowInsecureAuth silences go-smtp's refusal
	// to advertise AUTH without TLS; the proxy never actually requires
	// auth from clients.
	smtpSrv.AllowInsecureAuth = true

	metricsSrv := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler()}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("ComplyMail SMTP Proxy listening on %s (upstream=%s, backend=%s)",
			cfg.ListenAddr, be.Upstream, cfg.Backend.APIURL)
		errCh <- smtpSrv.ListenAndServe()
	}()
	go func() {
		log.Printf("ComplyMail SMTP Proxy metrics listening on %s", cfg.MetricsAddr)
		errCh <- metricsSrv.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	case <-ctx.Done():
		log.Println("shutting down...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := smtpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("smtp server shutdown: %v", err)
	}
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("metrics server shutdown: %v", err)
	}
}
