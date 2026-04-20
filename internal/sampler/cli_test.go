package sampler

import (
	"errors"
	"os"
	"testing"
	"time"
)

func withCleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SOURCE_ELASTICSEARCH_HOST",
		"SOURCE_ELASTICSEARCH_API_KEY",
		"ELASTICSEARCH_HOST",
		"ELASTICSEARCH_USERNAME",
		"ELASTICSEARCH_PASSWORD",
		"ELASTICSEARCH_API_KEY",
		"SYNC_INDEX_PATTERN",
		"SYNC_SIZE",
		"SYNC_INTERVAL_SECONDS",
		"SYNC_LOOKBACK",
		"SYNC_RANDOM_SEED",
		"SYNC_TARGET_INDEX",
		"SYNC_BATCH_SIZE",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

func TestParseConfig_RequiresSourceHost(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	_, err := ParseConfig(nil)
	if err == nil || errors.Is(err, ErrHelpRequested) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestParseConfig_SourceFromEnv(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "https://src:9200/")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceHost != "https://src:9200" {
		t.Fatalf("trailing slash not stripped: %q", cfg.SourceHost)
	}
	if cfg.SourceAPIKey != "k" {
		t.Fatalf("api key: %q", cfg.SourceAPIKey)
	}
	if cfg.Size != 100 {
		t.Fatalf("size default: %d", cfg.Size)
	}
	if cfg.BatchSize != 100 {
		t.Fatalf("batch size default: %d", cfg.BatchSize)
	}
	if cfg.Interval != time.Second {
		t.Fatalf("interval default: %v", cfg.Interval)
	}
	if cfg.Lookback != 24*time.Hour {
		t.Fatalf("lookback default: %v", cfg.Lookback)
	}
}

func TestParseConfig_LookbackFromCLI(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	cfg, err := ParseConfig([]string{"--lookback=15m"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lookback != 15*time.Minute {
		t.Fatalf("lookback: %v", cfg.Lookback)
	}
}

func TestParseConfig_LookbackFromEnv(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")
	t.Setenv("SYNC_LOOKBACK", "2h30m")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lookback != 2*time.Hour+30*time.Minute {
		t.Fatalf("lookback: %v", cfg.Lookback)
	}
}

func TestParseConfig_LookbackMustBePositive(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	for _, v := range []string{"0s", "-5m", "bogus"} {
		if _, err := ParseConfig([]string{"--lookback=" + v}); err == nil {
			t.Fatalf("expected error for --lookback=%q", v)
		}
	}
}

func TestParseConfig_CLIOverridesEnv(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://env:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "envkey")

	cfg, err := ParseConfig([]string{
		"--source-host=http://cli:9200", "--source-api-key=clikey",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceHost != "http://cli:9200" || cfg.SourceAPIKey != "clikey" {
		t.Fatalf("cli override failed: %+v", cfg)
	}
}

func TestParseConfig_DestinationDefaults(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DestHost != "http://localhost:9200" {
		t.Fatalf("dest host default: %q", cfg.DestHost)
	}
	if cfg.DestUsername != "elastic" {
		t.Fatalf("dest username default: %q", cfg.DestUsername)
	}
	if cfg.DestPassword != "changeme" {
		t.Fatalf("dest password default: %q", cfg.DestPassword)
	}
	if cfg.DestAPIKey != "" {
		t.Fatalf("dest api key default: %q", cfg.DestAPIKey)
	}
}

func TestParseConfig_DestinationFromEnv(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")
	t.Setenv("ELASTICSEARCH_HOST", "https://dst:9200/")
	t.Setenv("ELASTICSEARCH_USERNAME", "alice")
	t.Setenv("ELASTICSEARCH_PASSWORD", "secret")
	t.Setenv("ELASTICSEARCH_API_KEY", "env_key")

	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DestHost != "https://dst:9200" {
		t.Fatalf("dest host: %q", cfg.DestHost)
	}
	if cfg.DestUsername != "alice" {
		t.Fatalf("dest username: %q", cfg.DestUsername)
	}
	if cfg.DestPassword != "secret" {
		t.Fatalf("dest password: %q", cfg.DestPassword)
	}
	if cfg.DestAPIKey != "env_key" {
		t.Fatalf("dest api key: %q", cfg.DestAPIKey)
	}
}

func TestParseConfig_DestAPIKeyCLIOverridesEnv(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")
	t.Setenv("ELASTICSEARCH_API_KEY", "env_key")

	cfg, err := ParseConfig([]string{"--dest-api-key=cli_key"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DestAPIKey != "cli_key" {
		t.Fatalf("dest api key: %q", cfg.DestAPIKey)
	}
}

func TestParseConfig_SizeMustBePositive(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	_, err := ParseConfig([]string{"--size=0"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseConfig_RandomSeedMustBeInt(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	_, err := ParseConfig([]string{"--random-seed=abc"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseConfig_UnknownFlagRejected(t *testing.T) {
	withCleanEnv(t)
	chdirTemp(t)
	t.Setenv("SOURCE_ELASTICSEARCH_HOST", "http://src:9200")
	t.Setenv("SOURCE_ELASTICSEARCH_API_KEY", "k")

	for _, flag := range []string{"--from=2024-01-01T00:00:00Z", "--sample-mode=recent"} {
		if _, err := ParseConfig([]string{flag}); err == nil {
			t.Fatalf("expected unknown-flag error for %q", flag)
		}
	}
}
