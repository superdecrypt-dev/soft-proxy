package core

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"soft-proxy/internal/logger"
)

var (
	lbMu         sync.Mutex
	lbIndex      = make(map[string]int)
	deadBackends = make(map[string]time.Time)
)

func markBackendDead(addr string) {
	lbMu.Lock()
	deadBackends[addr] = time.Now().Add(30 * time.Second)
	lbMu.Unlock()
}

func markBackendAlive(addr string) {
	lbMu.Lock()
	delete(deadBackends, addr)
	lbMu.Unlock()
}

func getOrderedAddrs(key string, addrs []string) []string {
	lbMu.Lock()
	defer lbMu.Unlock()

	var alive []string
	var dead []string

	now := time.Now()
	for _, addr := range addrs {
		if until, exists := deadBackends[addr]; exists && now.Before(until) {
			dead = append(dead, addr)
		} else {
			alive = append(alive, addr)
		}
	}

	if len(alive) == 0 {
		alive = addrs
		dead = nil
	}

	idx := lbIndex[key]
	lbIndex[key] = (idx + 1) % len(alive)

	var result []string
	result = append(result, alive[idx%len(alive)])
	for i := 1; i < len(alive); i++ {
		result = append(result, alive[(idx+i)%len(alive)])
	}
	result = append(result, dead...)

	return result
}

func forwardToBackend(clientConn net.Conn, backendAddr string) {
	rawAddrs := strings.Split(backendAddr, ",")
	var addrs []string
	for _, a := range rawAddrs {
		trimmed := strings.TrimSpace(a)
		if trimmed != "" {
			addrs = append(addrs, trimmed)
		}
	}

	if len(addrs) == 0 {
		logger.Warn("No backend address specified")
		_ = clientConn.Close()
		return
	}

	orderedAddrs := getOrderedAddrs(backendAddr, addrs)

	var backendConn net.Conn
	var dialErr error
	var successfulAddr string

	for _, addr := range orderedAddrs {
		backendConn, dialErr = net.DialTimeout("tcp", addr, 3*time.Second)
		if dialErr == nil {
			successfulAddr = addr
			break
		}
		markBackendDead(addr)
		logger.Warn("Backend %s failed to connect: %v. Trying next backend...", addr, dialErr)
	}

	if dialErr != nil {
		logger.Error("All backends for %q failed: %v", backendAddr, dialErr)
		_ = clientConn.Close()
		return
	}

	markBackendAlive(successfulAddr)

	go func() {
		_, _ = io.Copy(backendConn, clientConn)
		_ = backendConn.Close()
		_ = clientConn.Close()
	}()
	go func() {
		_, _ = io.Copy(clientConn, backendConn)
		_ = clientConn.Close()
		_ = backendConn.Close()
	}()
}

func proxyHTTP(w http.ResponseWriter, r *http.Request, backendAddr string) {
	transport := &http.Transport{}
	outReq, err := http.NewRequest(r.Method, "http://"+backendAddr+r.URL.RequestURI(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for k, vv := range r.Header {
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func proxyWebSocket(w http.ResponseWriter, r *http.Request, backendAddr string) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	backendConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	r.RequestURI = ""
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.URL.Host == "" {
		r.URL.Host = backendAddr
	}

	err = r.Write(backendConn)
	if err != nil {
		log.Printf("Failed to write request to backend: %v", err)
		return
	}

	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(backendConn, bufrw.Reader)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errChan <- err
	}()

	<-errChan
}
