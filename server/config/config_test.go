package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentOverridesFileAndValidatesWithoutLeaking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(validYAML), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PENGUIN_ADDR", ":8082")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8082" {
		t.Fatal("environment did not override file")
	}
	t.Setenv("PENGUIN_LOG_MAX_BACKUPS", "-1")
	_, err = Load(path)
	if err == nil || strings.Contains(err.Error(), "private-pass") {
		t.Fatal("unsafe validation")
	}
}

const validYAML = `mysql:
  dsn: "user:private-pass@tcp(localhost:3306)/game"
auth:
  jwt_secret: "12345678901234567890123456789012"
server:
  addr: ":8080"
  request_timeout: 3s
redis:
  password: "p#ass: word"
log:
  max_backups: 8
`

func TestYAMLValuesAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(validYAML), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.RequestTimeout != 3*time.Second || c.StartupTimeout != 15*time.Second || c.LogMaxBackups != 8 || c.LogMaxSizeMB != 10 || c.RedisPass != "p#ass: word" {
		t.Fatal("YAML values or defaults were not preserved")
	}
}

func TestRejectsInvalidYAMLWithoutLeakingValues(t *testing.T) {
	for name, content := range map[string]string{
		"unknown field":      validYAML + "unexpected: private-pass\n",
		"nested unknown":     strings.Replace(validYAML, "max_backups", "max_backup", 1),
		"wrong type":         strings.Replace(validYAML, "max_backups: 8", "max_backups: private-pass", 1),
		"duplicate key":      validYAML + "log:\n  level: debug\n",
		"multiple documents": validYAML + "---\nserver:\n  addr: ':9090'\n",
		"zero backups":       strings.Replace(validYAML, "max_backups: 8", "max_backups: 0", 1),
		"bad duration":       strings.Replace(validYAML, "request_timeout: 3s", "request_timeout: invalid", 1),
		"empty file":         "",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || strings.Contains(err.Error(), "private-pass") {
				t.Fatal("invalid configuration accepted or secret leaked")
			}
		})
	}
}

func TestMissingFileAndEnvironmentOnlyMode(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("missing explicit file accepted")
	}
	t.Setenv("PENGUIN_MYSQL_DSN", "user:pass@tcp(localhost:3306)/game")
	t.Setenv("PENGUIN_JWT_SECRET", "12345678901234567890123456789012")
	if _, err := Load(""); err != nil {
		t.Fatal(err)
	}
}

func TestWebSocketAllowedOriginsConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := strings.Replace(validYAML, "  addr: \":8080\"", "  addr: \":8080\"\n  ws_allowed_origins: [\"https://penguins.iepose.cn\"]", 1)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.WSAllowedOrigins) != 1 || c.WSAllowedOrigins[0] != "https://penguins.iepose.cn" {
		t.Fatalf("origins: %v", c.WSAllowedOrigins)
	}
	t.Setenv("PENGUIN_WS_ALLOWED_ORIGINS", " https://GAME.example:8443, http://localhost:5173 ")
	c, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.WSAllowedOrigins) != 2 || c.WSAllowedOrigins[0] != "https://game.example:8443" || c.WSAllowedOrigins[1] != "http://localhost:5173" {
		t.Fatalf("origins: %v", c.WSAllowedOrigins)
	}
	t.Setenv("PENGUIN_WS_ALLOWED_ORIGINS", "")
	c, err = Load(path)
	if err != nil || len(c.WSAllowedOrigins) != 0 {
		t.Fatalf("cannot clear file origins: %v, %v", c.WSAllowedOrigins, err)
	}
	for _, value := range []string{"*", "https://*.example", "penguins.iepose.cn", "wss://penguins.iepose.cn", "https://example/path", "https://example?", "https://example#", "https://user:private-pass@example", "https://", "null"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PENGUIN_WS_ALLOWED_ORIGINS", value)
			_, err := Load(path)
			if err == nil || strings.Contains(err.Error(), "private-pass") {
				t.Fatal("invalid origin accepted or secret leaked")
			}
		})
	}
}
