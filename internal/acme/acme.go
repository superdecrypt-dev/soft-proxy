package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"

	"soft-proxy/internal/config"
	"soft-proxy/internal/logger"
)

var legoTriggerChan = make(chan struct{}, 1)

func TriggerLegoCheck() {
	select {
	case legoTriggerChan <- struct{}{}:
	default:
	}
}

func StartLegoACME() {
	time.Sleep(2 * time.Second)

	for {
		acmeCfg := config.GetACMEConfig()

		if acmeCfg.Enabled && strings.ToLower(acmeCfg.DNSProvider) == "cloudflare" && len(acmeCfg.Domains) > 0 && acmeCfg.Email != "" && acmeCfg.CloudflareToken != "" {
			for _, domain := range acmeCfg.Domains {
				safeDomain := strings.ReplaceAll(domain, "*", "_wildcard")
				crtPath := filepath.Join(acmeCfg.CacheDir, safeDomain+".crt")

				if !isCertValid(crtPath) {
					logger.Info("Certificate for %s is missing or expiring. Requesting new certificate via Lego Cloudflare DNS challenge...", domain)
					err := requestLegoCert(acmeCfg.Email, domain, acmeCfg.CloudflareToken, acmeCfg.CacheDir)
					if err != nil {
						logger.Error("Failed to obtain certificate for %s: %v", domain, err)
					} else {
						logger.Info("Successfully obtained certificate for %s", domain)
					}
				}
			}
		}

		select {
		case <-legoTriggerChan:
		case <-time.After(12 * time.Hour):
		}
	}
}

type LegoUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *LegoUser) GetEmail() string {
	return u.Email
}
func (u *LegoUser) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *LegoUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func isCertValid(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return time.Now().Add(30 * 24 * time.Hour).Before(cert.NotAfter)
}

func requestLegoCert(email, domain, token, cacheDir string) error {
	userKeyPath := filepath.Join(cacheDir, "user.key")
	var privateKey crypto.PrivateKey
	keyData, err := os.ReadFile(userKeyPath)
	if err == nil {
		block, _ := pem.Decode(keyData)
		if block != nil {
			privateKey, _ = x509.ParseECPrivateKey(block.Bytes)
		}
	}

	if privateKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate account key: %v", err)
		}
		privateKey = key

		der, err := x509.MarshalECPrivateKey(key)
		if err == nil {
			_ = os.WriteFile(userKeyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0600)
		}
	}

	user := &LegoUser{
		Email: email,
		key:   privateKey,
	}

	legoCfg := lego.NewConfig(user)
	legoCfg.CADirURL = lego.LEDirectoryProduction
	legoCfg.Certificate.KeyType = certcrypto.EC256

	client, err := lego.NewClient(legoCfg)
	if err != nil {
		return fmt.Errorf("failed to create lego client: %v", err)
	}

	cfConfig := cloudflare.NewDefaultConfig()
	cfConfig.AuthToken = token
	cfProvider, err := cloudflare.NewDNSProviderConfig(cfConfig)
	if err != nil {
		return fmt.Errorf("failed to create cloudflare provider: %v", err)
	}

	err = client.Challenge.SetDNS01Provider(cfProvider)
	if err != nil {
		return fmt.Errorf("failed to setup challenge provider: %v", err)
	}

	reg, err := client.Registration.ResolveAccountByKey()
	if err != nil {
		reg, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			logger.Warn("Lego account registration warning: %v", err)
		}
	}
	user.Registration = reg

	request := certificate.ObtainRequest{
		Domains: []string{domain},
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %v", err)
	}

	safeDomain := strings.ReplaceAll(domain, "*", "_wildcard")
	crtPath := filepath.Join(cacheDir, safeDomain+".crt")
	keyPath := filepath.Join(cacheDir, safeDomain+".key")

	_ = os.WriteFile(crtPath, certificates.Certificate, 0644)
	_ = os.WriteFile(keyPath, certificates.PrivateKey, 0600)

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func FindCachedCertificate(sni, cacheDir string) (*tls.Certificate, error) {
	sni = strings.ToLower(strings.TrimSpace(sni))
	if sni == "" {
		return nil, fmt.Errorf("empty SNI")
	}

	crtPath := filepath.Join(cacheDir, sni+".crt")
	keyPath := filepath.Join(cacheDir, sni+".key")
	if fileExists(crtPath) && fileExists(keyPath) {
		cert, err := tls.LoadX509KeyPair(crtPath, keyPath)
		if err == nil {
			return &cert, nil
		}
	}

	parts := strings.Split(sni, ".")
	if len(parts) > 1 {
		parts[0] = "_wildcard"
		wildcardDomain := strings.Join(parts, ".")
		crtPath = filepath.Join(cacheDir, wildcardDomain+".crt")
		keyPath = filepath.Join(cacheDir, wildcardDomain+".key")
		if fileExists(crtPath) && fileExists(keyPath) {
			cert, err := tls.LoadX509KeyPair(crtPath, keyPath)
			if err == nil {
				return &cert, nil
			}
		}
	}

	return nil, fmt.Errorf("certificate not found for %s", sni)
}

func EnsureSelfSignedCert(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err == nil {
		return cert, nil
	}

	logger.Info("Fallback self-signed certificate not found or invalid at %s. Generating a new one dynamically...", certPath)

	dir := filepath.Dir(certPath)
	_ = os.MkdirAll(dir, 0755)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %v", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(3650 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "fallback-domain.com",
			Organization: []string{"Soft Proxy Multiplexer"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %v", err)
	}

	certBlock := &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}
	certPEM := pem.EncodeToMemory(certBlock)
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to write certificate file: %v", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to marshal private key: %v", err)
	}
	keyBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}
	keyPEM := pem.EncodeToMemory(keyBlock)
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to write private key file: %v", err)
	}

	logger.Info("Successfully generated and saved fallback certificate to %s", certPath)

	return tls.X509KeyPair(certPEM, keyPEM)
}
