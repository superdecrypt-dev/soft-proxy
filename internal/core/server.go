package core

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"golang.org/x/crypto/acme/autocert"

	"soft-proxy/internal/acme"
	"soft-proxy/internal/autoblocker"
	"soft-proxy/internal/config"
	"soft-proxy/internal/logger"
)

func StartHTTPServer(bindAddr string, httpPort int, certManager *autocert.Manager) {
	addr := fmt.Sprintf("%s:%d", bindAddr, httpPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on port 80: %v", err)
	}
	defer listener.Close()

	connChan := make(chan net.Conn, 1000)
	chanListener := &ChanListener{
		connChan: connChan,
		addr:     listener.Addr(),
	}

	go func() {
		mux := http.NewServeMux()
		handler := func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
				if certManager != nil {
					certManager.HTTPHandler(nil).ServeHTTP(w, r)
					return
				}
				if backend, ok := config.GetBackend("http"); ok {
					proxyHTTP(w, r, backend)
					return
				}
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}

			host := r.Host
			httpsPort := config.GetHTTPSPort()
			if strings.Contains(host, ":") {
				h, _, _ := net.SplitHostPort(host)
				host = fmt.Sprintf("%s:%d", h, httpsPort)
			} else if httpsPort != 443 {
				host = fmt.Sprintf("%s:%d", host, httpsPort)
			}
			target := fmt.Sprintf("https://%s%s", host, r.URL.RequestURI())
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}
		mux.HandleFunc("/", handler)
		logger.Info("HTTP server listening on %s (Multiplexed)", addr)
		if err := http.Serve(chanListener, mux); err != nil {
			logger.Warn("HTTP server error: %v", err)
			_ = listener.Close()
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Warn("Failed to accept connection on port 80: %v", err)
			if strings.Contains(err.Error(), "closed") {
				break
			}
			continue
		}

		ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if autoblocker.IsBlocked(ip) {
			_ = conn.Close()
			continue
		}

		go func(c net.Conn) {
			peakConn := NewPeakableConn(c)
			peekBytes, err := peakConn.Peek(256)
			if err != nil && len(peekBytes) == 0 {
				_ = c.Close()
				return
			}

			protocol := DetectProtocol(peekBytes)
			if protocol == "http" {
				if bytes.HasPrefix(peekBytes, []byte("PRI ")) {
					if backend, ok := config.GetBackend("http"); ok {
						forwardToBackend(peakConn, backend)
					} else {
						_ = c.Close()
					}
					return
				}

				path := ParseHTTPPath(peekBytes)
				if strings.HasPrefix(path, "/vless-") || strings.HasPrefix(path, "/vmess-") || strings.HasPrefix(path, "/trojan-") {
					if backend, ok := config.GetBackend("http"); ok {
						forwardToBackend(peakConn, backend)
					} else {
						_ = c.Close()
					}
					return
				}

				connChan <- peakConn
				return
			}

			if protocol == "" {
				logger.Warn("Unknown protocol from %s on port 80, closing", c.RemoteAddr().String())
				_ = c.Close()
				return
			}

			logger.Info("Detected protocol on port 80: %s (no TLS) from %s", protocol, c.RemoteAddr().String())
			backendAddr, ok := config.GetBackend(protocol)
			if !ok {
				logger.Warn("No backend configured for protocol: %s", protocol)
				_ = c.Close()
				return
			}

			forwardToBackend(peakConn, backendAddr)
		}(conn)
	}
}

func StartHTTPSServer(bindAddr string, httpsPort int, certManager *autocert.Manager) {
	certFile := config.GetCertFile()
	keyFile := config.GetKeyFile()

	if certFile == "" {
		certFile = "/etc/soft-proxy/certs/selfsigned.crt"
	}
	if keyFile == "" {
		keyFile = "/etc/soft-proxy/certs/selfsigned.key"
	}

	selfSignedCert, err := acme.EnsureSelfSignedCert(certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to load or generate self-signed fallback cert: %v", err)
	}

	getCertFunc := func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		acmeCfg := config.GetACMEConfig()

		if acmeCfg.Enabled && strings.ToLower(acmeCfg.DNSProvider) == "cloudflare" {
			cert, err := acme.FindCachedCertificate(hello.ServerName, acmeCfg.CacheDir)
			if err == nil {
				return cert, nil
			}
		}

		return &selfSignedCert, nil
	}

	selfSignedConfig := &tls.Config{
		MinVersion:     tls.VersionTLS13,
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: getCertFunc,
	}

	var acmeConfig *tls.Config
	if certManager != nil {
		acmeConfig = certManager.TLSConfig()
		acmeConfig.MinVersion = tls.VersionTLS13
		acmeConfig.NextProtos = []string{"h2", "http/1.1", "acme-tls/1"}
	}

	addr := fmt.Sprintf("%s:%d", bindAddr, httpsPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	defer listener.Close()

	logger.Info("HTTPS server listening on %s (TLS SNI Sniffing & Auto-Blocker enabled)", addr)
	runHTTPSServer(listener, selfSignedConfig, acmeConfig)
}

func runHTTPSServer(listener net.Listener, selfSignedConfig *tls.Config, acmeConfig *tls.Config) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if autoblocker.IsBlocked(ip) {
			_ = conn.Close()
			continue
		}

		go handleTLSConn(conn, selfSignedConfig, acmeConfig)
	}
}

func sniffSNI(c net.Conn) (string, *PeakableConn, error) {
	peakConn := NewPeakableConn(c)
	peekBytes, err := peakConn.PeekTLSRecord()
	if err != nil && len(peekBytes) == 0 {
		return "", peakConn, err
	}

	if len(peekBytes) == 0 || peekBytes[0] != 0x16 {
		return "", peakConn, nil
	}

	var sni string
	sniffReader := bytes.NewReader(peekBytes)
	sniffConn := &sniSniffConn{
		Conn: peakConn,
		r:    sniffReader,
	}

	tlsCfg := &tls.Config{
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			sni = info.ServerName
			return nil, errors.New("sniff complete")
		},
	}

	tlsServer := tls.Server(sniffConn, tlsCfg)
	_ = tlsServer.Handshake()

	return sni, peakConn, nil
}

func handleTLSConn(conn net.Conn, selfSignedConfig *tls.Config, acmeConfig *tls.Config) {
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	sni, peakConn, err := sniffSNI(conn)
	if err != nil {
		logger.Warn("Failed to sniff SNI from %s: %v", conn.RemoteAddr().String(), err)
		autoblocker.RecordFailure(ip)
		_ = conn.Close()
		return
	}
	logger.Info("Sniffed SNI: %q from %s", sni, conn.RemoteAddr().String())

	backendAddr, isReality := config.GetRealityBackend(sni)
	if isReality {
		logger.Info("Bypassing TLS termination for Reality SNI: %q to %s", sni, backendAddr)
		forwardToBackend(peakConn, backendAddr)
		return
	}

	var activeConfig *tls.Config
	isAllowedDomain := config.IsACMEDomain(sni)
	if isAllowedDomain && acmeConfig != nil {
		activeConfig = acmeConfig
	} else {
		activeConfig = selfSignedConfig
	}

	tlsConn := tls.Server(peakConn, activeConfig)
	if err := tlsConn.Handshake(); err != nil {
		logger.Warn("TLS Handshake failed from %s: %v", conn.RemoteAddr().String(), err)
		autoblocker.RecordFailure(ip)
		_ = peakConn.Close()
		return
	}

	decryptedPeakConn := NewPeakableConn(tlsConn)
	peekBytes, err := decryptedPeakConn.Peek(60)
	if err != nil && len(peekBytes) == 0 {
		logger.Warn("Failed to peek decrypted connection data from %s: %v", conn.RemoteAddr().String(), err)
		autoblocker.RecordFailure(ip)
		_ = tlsConn.Close()
		return
	}

	protocol := DetectProtocol(peekBytes)
	logger.Info("Detected protocol: %s (peek length: %d) from %s", protocol, len(peekBytes), conn.RemoteAddr().String())

	if protocol == "" {
		logger.Warn("Unknown protocol from %s after TLS, closing", conn.RemoteAddr().String())
		autoblocker.RecordFailure(ip)
		_ = tlsConn.Close()
		return
	}

	backendAddr, ok := config.GetBackend(protocol)
	if !ok {
		if fallback, exists := config.GetBackend("http"); exists {
			backendAddr = fallback
		} else {
			logger.Warn("No backend configured for protocol: %s", protocol)
			autoblocker.RecordFailure(ip)
			_ = tlsConn.Close()
			return
		}
	}

	forwardToBackend(decryptedPeakConn, backendAddr)
}
