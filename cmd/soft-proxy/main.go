package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/crypto/acme/autocert"

	"soft-proxy/internal/acme"
	"soft-proxy/internal/config"
	"soft-proxy/internal/core"
	"soft-proxy/internal/logger"
)

func main() {
	log.SetFlags(0)

	logDir := "/var/log/soft-proxy"
	_ = os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "soft-proxy.log")
	rotator, err := logger.NewRotatorWriter(logFile, 10*1024*1024)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, rotator))
	} else {
		log.Printf("Warning: Failed to initialize log file: %v. Logging to stderr only.", err)
	}

	config.OnCloudflareDNS = acme.TriggerLegoCheck

	if err := config.ReloadConfig(); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	go config.StartConfigWatcher("config.yaml")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for range sigChan {
			logger.Info("SIGHUP received, reloading configuration...")
			if err := config.ReloadConfig(); err != nil {
				logger.Error("Failed to reload configuration: %v", err)
			}
		}
	}()

	bindAddr := config.GetBindAddr()
	httpPort := config.GetHTTPPort()
	httpsPort := config.GetHTTPSPort()
	acmeCfg := config.GetACMEConfig()

	var certManager *autocert.Manager

	if acmeCfg.Enabled {
		if err := os.MkdirAll(acmeCfg.CacheDir, 0700); err != nil {
			log.Fatalf("Failed to create ACME cache directory: %v", err)
		}
		if strings.ToLower(acmeCfg.DNSProvider) == "cloudflare" {
			logger.Info("DNS Challenge enabled with Cloudflare provider. Starting background Lego ACME manager...")
			go acme.StartLegoACME()
		} else {
			certManager = &autocert.Manager{
				Prompt: autocert.AcceptTOS,
				HostPolicy: func(ctx context.Context, host string) error {
					if config.IsACMEDomain(host) {
						return nil
					}
					return fmt.Errorf("host %q not configured in ACME domains whitelist", host)
				},
				Cache: autocert.DirCache(acmeCfg.CacheDir),
			}
			certManager.HTTPHandler(nil)
			logger.Info("ACME HTTP Challenge enabled. Cache dir: %s", acmeCfg.CacheDir)
		}
	}

	go core.StartHTTPServer(bindAddr, httpPort, certManager)

	core.StartHTTPSServer(bindAddr, httpsPort, certManager)
}
