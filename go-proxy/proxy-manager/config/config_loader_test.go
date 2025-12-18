package config

import (
	"os"
	"testing"
)

func TestLoadGlobalConfig(t *testing.T) {
	yaml := `
defaults:
  options:
    http2: true
    http3: false
tls:
  certificates:
    - domains: ["example.com", "www.example.com"]
      cert_file: "/path/cert.pem"
      key_file: "/path/key.pem"
`
	f, err := os.CreateTemp("", "global-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := LoadGlobalConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadGlobalConfig error: %v", err)
	}
	if len(cfg.TLS.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(cfg.TLS.Certificates))
	}
	if cfg.Defaults.Options.HTTP2 == nil || *cfg.Defaults.Options.HTTP2 != true {
		t.Fatalf("expected HTTP2=true")
	}
	if cfg.Defaults.Options.HTTP3 == nil || *cfg.Defaults.Options.HTTP3 != false {
		t.Fatalf("expected HTTP3=false")
	}
}

func TestLoadSiteConfigValidateAndOptions(t *testing.T) {
	yaml := `
enabled: true
service:
  name: api
routes:
  - domains: ["example.com"]
    path: /api
    backend: http://localhost:8080
options:
  health_check_interval: 30s
  health_check_timeout: 5s
  timeout: 15s
  max_body_size: 10M
  http2: true
  http3: true
`
	f, err := os.CreateTemp("", "site-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := LoadSiteConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadSiteConfig error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate error: %v", err)
	}

	opts, err := cfg.GetOptions()
	if err != nil {
		t.Fatalf("GetOptions error: %v", err)
	}
	if opts["max_body_size"].(int64) <= 0 {
		t.Error("expected parsed max_body_size > 0")
	}
}
