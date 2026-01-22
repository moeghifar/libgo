package envy

import (
	"os"
	"reflect"
	"testing"
)

type Config struct {
	AppPort int    `env:"APP_PORT" default:"8080"`
	Debug   bool   `env:"DEBUG" default:"false"`
	APIKey  string `env:"API_KEY" required:"true"`

	// Slice types
	AllowedHosts []string  `env:"ALLOWED_HOSTS" default:"localhost"`
	RetryBackoff []int     `env:"RETRY_BACKOFF" default:"1,2,4"`
	Temperatures []float64 `env:"TEMPERATURES"`

	Database struct {
		DSN string `env:"DB_DSN"`
	}
}

func TestLoad_NoEnvFile(t *testing.T) {
	// Ensure no .env file exists
	os.Remove(".env")

	// Set required env var manually to avoid error
	os.Setenv("API_KEY", "secret")
	defer os.Unsetenv("API_KEY")

	cfg := Config{}
	err := Load(&cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.AppPort != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.AppPort)
	}
	if len(cfg.AllowedHosts) != 1 || cfg.AllowedHosts[0] != "localhost" {
		t.Errorf("expected default allowed hosts [localhost], got %v", cfg.AllowedHosts)
	}
}

func TestLoad_WithEnvFileAndSlices(t *testing.T) {
	envContent := `APP_PORT=9090
DEBUG=true
API_KEY=testkey
DB_DSN=postgres://user:pass@localhost:5432/db
ALLOWED_HOSTS=example.com, api.example.com
RETRY_BACKOFF=10, 20, 30
TEMPERATURES=23.5, 98.6, 100.0`

	err := os.WriteFile(".env", []byte(envContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(".env")

	cfg := Config{}
	err = Load(&cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.AppPort != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.AppPort)
	}
	if !cfg.Debug {
		t.Errorf("expected debug true, got false")
	}
	if cfg.APIKey != "testkey" {
		t.Errorf("expected API key testkey, got %s", cfg.APIKey)
	}
	if cfg.Database.DSN != "postgres://user:pass@localhost:5432/db" {
		t.Errorf("expected DB DSN, got %s", cfg.Database.DSN)
	}

	// Verify slices
	expectedHosts := []string{"example.com", "api.example.com"}
	if !reflect.DeepEqual(cfg.AllowedHosts, expectedHosts) {
		t.Errorf("expected hosts %v, got %v", expectedHosts, cfg.AllowedHosts)
	}

	expectedBackoff := []int{10, 20, 30}
	if !reflect.DeepEqual(cfg.RetryBackoff, expectedBackoff) {
		t.Errorf("expected backoff %v, got %v", expectedBackoff, cfg.RetryBackoff)
	}

	expectedTemps := []float64{23.5, 98.6, 100.0}
	if !reflect.DeepEqual(cfg.Temperatures, expectedTemps) {
		t.Errorf("expected temperatures %v, got %v", expectedTemps, cfg.Temperatures)
	}
}

func TestLoad_RequiredMissing(t *testing.T) {
	os.Remove(".env")
	os.Unsetenv("API_KEY")

	cfg := Config{}
	err := Load(&cfg)
	if err == nil {
		t.Fatal("expected error due to missing required field, got nil")
	}
}
