package autoblocker

import (
	"sync"
	"time"

	"soft-proxy/internal/logger"
)

var (
	blocklist           = make(map[string]time.Time)
	blocklistMu         sync.RWMutex
	failures            = make(map[string]int)
	failuresMu          sync.Mutex
	failuresCleanerOnce sync.Once
)

func IsBlocked(ip string) bool {
	blocklistMu.RLock()
	expire, blocked := blocklist[ip]
	blocklistMu.RUnlock()

	if !blocked {
		return false
	}

	if time.Now().After(expire) {
		blocklistMu.Lock()
		delete(blocklist, ip)
		blocklistMu.Unlock()
		return false
	}
	return true
}

func RecordFailure(ip string) {
	failuresCleanerOnce.Do(func() {
		go func() {
			for {
				time.Sleep(1 * time.Minute)
				failuresMu.Lock()
				for k, v := range failures {
					if v > 1 {
						failures[k] = v - 1
					} else {
						delete(failures, k)
					}
				}
				failuresMu.Unlock()
			}
		}()
	})

	failuresMu.Lock()
	failures[ip]++
	count := failures[ip]
	failuresMu.Unlock()

	if count >= 10 {
		blocklistMu.Lock()
		blocklist[ip] = time.Now().Add(1 * time.Hour)
		blocklistMu.Unlock()
		logger.Warn("IP %s auto-blocked for 1 hour due to 10+ failed handshakes/probing attempts", ip)

		failuresMu.Lock()
		delete(failures, ip)
		failuresMu.Unlock()
	}
}
