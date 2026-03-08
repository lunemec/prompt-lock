package main

import (
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

	"github.com/lunemec/promptlock/internal/config"
)

func TestBuildTLSConfigRequiresValidClientCAInMTLSMode(t *testing.T) {
	cfg := config.Default()
	cfg.TLS.Enable = true
	cfg.TLS.RequireClientCert = true
	cfg.TLS.ClientCAFile = filepath.Join(t.TempDir(), "missing-ca.pem")
	if _, err := buildTLSConfig(cfg); err == nil {
		t.Fatalf("expected error for missing client ca file")
	}
}

func TestBuildTLSConfigLoadsClientCA(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	writeSelfSignedCA(t, caPath)

	cfg := config.Default()
	cfg.TLS.Enable = true
	cfg.TLS.RequireClientCert = true
	cfg.TLS.ClientCAFile = caPath

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("expected tls config success, got %v", err)
	}
	if tlsCfg.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func writeSelfSignedCA(t *testing.T, out string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "PromptLock Test CA"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(out, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}
