package config

import (
	"os"
	"strings"
	"sync"
	"time"

	"soft-proxy/internal/logger"

	"gopkg.in/yaml.v3"
)

type AcmeConfig struct {
	Enabled         bool     `yaml:"enabled"`
	Domains         []string `yaml:"domains"`
	CacheDir        string   `yaml:"cache_dir"`
	DNSProvider     string   `yaml:"dns_provider"`
	Email           string   `yaml:"email"`
	CloudflareToken string   `yaml:"cloudflare_token"`
}

type Config struct {
	BindAddr        string              `yaml:"bind_addr"`
	HTTPPort        int                 `yaml:"http_port"`
	HTTPSPort       int                 `yaml:"https_port"`
	ACME            AcmeConfig          `yaml:"acme"`
	CertFile        string              `yaml:"cert_file"`
	KeyFile         string              `yaml:"key_file"`
	Backends        map[string]string   `yaml:"backends"`
	RealityDomains  []string            `yaml:"reality_domains"`
	RealityBackends map[string][]string `yaml:"reality_backends"`
}

var (
	ConfigPath      = "config.yaml"
	realityLookup   = make(map[string]string)
	realityLookupMu sync.RWMutex

	configMu  sync.RWMutex
	globalCfg Config

	OnCloudflareDNS func()
)

func ReloadConfig() error {
	configMu.RLock()
	path := ConfigPath
	configMu.RUnlock()

	configFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer configFile.Close()

	var newCfg Config
	if err := yaml.NewDecoder(configFile).Decode(&newCfg); err != nil {
		return err
	}

	configMu.Lock()
	globalCfg = newCfg
	configMu.Unlock()

	realityLookupMu.Lock()
	for k := range realityLookup {
		delete(realityLookup, k)
	}
	for backend, domains := range newCfg.RealityBackends {
		for _, domain := range domains {
			realityLookup[domain] = backend
		}
	}
	realityLookupMu.Unlock()

	if strings.ToLower(newCfg.ACME.DNSProvider) == "cloudflare" && OnCloudflareDNS != nil {
		OnCloudflareDNS()
	}

	logger.Info("Configuration reloaded. Backends: %+v, Domains mapped: %d", newCfg.Backends, len(realityLookup))
	return nil
}

func StartConfigWatcher(filePath string) {
	configMu.Lock()
	ConfigPath = filePath
	configMu.Unlock()

	var lastModTime time.Time

	if info, err := os.Stat(filePath); err == nil {
		lastModTime = info.ModTime()
	}

	for {
		time.Sleep(2 * time.Second)

		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		modTime := info.ModTime()
		if modTime.After(lastModTime) {
			lastModTime = modTime
			logger.Info("Config file modification detected. Hot-reloading configuration automatically...")

			time.Sleep(100 * time.Millisecond)

			if err := ReloadConfig(); err != nil {
				logger.Error("Auto hot-reload failed: %v", err)
			}
		}
	}
}

func GetBackend(protocol string) (string, bool) {
	configMu.RLock()
	defer configMu.RUnlock()
	addr, ok := globalCfg.Backends[protocol]
	return addr, ok
}

func GetHTTPSPort() int {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg.HTTPSPort
}

func GetBindAddr() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg.BindAddr
}

func GetHTTPPort() int {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg.HTTPPort
}

func GetCertFile() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg.CertFile
}

func GetKeyFile() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg.KeyFile
}

func IsACMEDomain(sni string) bool {
	configMu.RLock()
	defer configMu.RUnlock()
	if !globalCfg.ACME.Enabled {
		return false
	}
	for _, domain := range globalCfg.ACME.Domains {
		if sni == domain {
			return true
		}
	}
	return false
}

func GetACMEConfig() AcmeConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg.ACME
}

func GetRealityBackend(sni string) (string, bool) {
	realityLookupMu.RLock()
	addr, ok := realityLookup[sni]
	realityLookupMu.RUnlock()
	if ok {
		return addr, true
	}

	configMu.RLock()
	defer configMu.RUnlock()
	for _, domain := range globalCfg.RealityDomains {
		if sni == domain {
			if bAddr, exists := globalCfg.Backends["reality"]; exists {
				return bAddr, true
			}
			return "127.0.0.1:10444", true
		}
	}
	return "", false
}
