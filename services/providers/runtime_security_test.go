package providers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheTLSVerifiesTheConfiguredServerIdentity(t *testing.T) {
	ca, caPEM, caKey := testCertificateAuthority(t)
	leaf := testServerCertificate(t, ca, caKey, "cache.internal")
	caPath := filepath.Join(t.TempDir(), "redis-ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := buildCacheTLSConfig(CacheConfig{
		Host: "cache.internal",
		TLS:  CacheTLSConfig{Enabled: true, CAFile: caPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("Redis TLS must never skip certificate verification")
	}
	if cfg.MinVersion < 0x0303 { // TLS 1.2
		t.Fatalf("minimum TLS version = %#x, want TLS 1.2 or newer", cfg.MinVersion)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: cfg.RootCAs, DNSName: cfg.ServerName}); err != nil {
		t.Fatalf("configured Redis identity did not verify: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: cfg.RootCAs, DNSName: "other.internal"}); err == nil {
		t.Fatal("certificate unexpectedly verified for a different Redis identity")
	}
}

func TestCacheTLSRejectsInvalidOrInactiveCertificateSettings(t *testing.T) {
	badCA := filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(badCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := buildCacheTLSConfig(CacheConfig{TLS: CacheTLSConfig{Enabled: true, CAFile: badCA}}); err == nil {
		t.Fatal("invalid CA file was accepted")
	}
	if _, err := buildCacheTLSConfig(CacheConfig{TLS: CacheTLSConfig{CAFile: badCA}}); err == nil {
		t.Fatal("certificate setting with TLS disabled was accepted")
	}
	if _, err := buildCacheTLSConfig(CacheConfig{TLS: CacheTLSConfig{Enabled: true, CertFile: "client.pem"}}); err == nil {
		t.Fatal("client certificate without a private key was accepted")
	}
	if _, err := buildCacheTLSConfig(CacheConfig{TLS: CacheTLSConfig{Enabled: true}}); err == nil {
		t.Fatal("TLS without a host or explicit server name was accepted")
	}
}

func TestDatabaseLoggerDropsQueryParameters(t *testing.T) {
	type paramsFilter interface {
		ParamsFilter(context.Context, string, ...interface{}) (string, []interface{})
	}

	filter, ok := parameterizedDatabaseLogger().(paramsFilter)
	if !ok {
		t.Fatal("database logger does not expose GORM's parameter filter")
	}
	query, params := filter.ParamsFilter(context.Background(), "SELECT * FROM users WHERE email = ?", "private@example.com")
	if query != "SELECT * FROM users WHERE email = ?" {
		t.Fatalf("query template changed to %q", query)
	}
	if len(params) != 0 {
		t.Fatalf("database logger retained %d query parameter(s)", len(params))
	}
}

func testCertificateAuthority(t *testing.T) (*x509.Certificate, []byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test Redis CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), key
}

func testServerCertificate(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, name string) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}
