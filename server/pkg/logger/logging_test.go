package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func options(t *testing.T, console *bytes.Buffer) Options {
	t.Helper()
	return Options{Level: "info", File: filepath.Join(t.TempDir(), "logs", "server.log"), MaxSizeMB: 1, MaxBackups: 2, MaxAgeDays: 1, Console: console}
}

func records(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("invalid JSON log: %v", err)
		}
		out = append(out, record)
	}
	return out
}

func TestJSONFanoutAndRestrictedPermissions(t *testing.T) {
	var console bytes.Buffer
	opts := options(t, &console)
	logger, closer, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("request completed", "status", 200)
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(opts.File)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(console.Bytes(), file) {
		t.Fatal("console and file logs differ")
	}
	got := records(t, file)
	if len(got) != 1 || got[0]["msg"] != "request completed" || got[0]["status"] != float64(200) {
		t.Fatalf("unexpected records: %v", got)
	}
	if got[0]["level"] != "INFO" {
		t.Fatalf("unexpected log level: %v", got[0]["level"])
	}
	stamp, err := time.Parse(time.RFC3339Nano, got[0]["time"].(string))
	if err != nil || stamp.Location() != time.UTC {
		t.Fatal("log timestamp must be UTC RFC3339")
	}
	for path, want := range map[string]os.FileMode{opts.File: 0600, filepath.Dir(opts.File): 0700} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("incorrect permissions for %s", path)
		}
	}
}

func TestLevels(t *testing.T) {
	for _, tc := range []struct {
		level string
		count int
	}{{"debug", 4}, {"info", 3}, {"warn", 2}, {"error", 1}} {
		t.Run(tc.level, func(t *testing.T) {
			var console bytes.Buffer
			opts := options(t, &console)
			opts.Level = tc.level
			logger, closer, err := New(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer closer.Close()
			logger.Debug("debug")
			logger.Info("info")
			logger.Warn("warn")
			logger.Error("error")
			if got := len(records(t, console.Bytes())); got != tc.count {
				t.Fatalf("got %d records, want %d", got, tc.count)
			}
		})
	}
}

func TestRedactionAndPanicRecovery(t *testing.T) {
	var console bytes.Buffer
	opts := options(t, &console)
	opts.Secrets = []string{"known-sensitive-value", "quoted-\"secret"}
	logger, closer, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	logger.With("password", "field-sensitive-value").WithGroup("request").Error("failed known-sensitive-value quoted-\"secret",
		"error", errors.New("upstream known-sensitive-value failed"),
		"nested", map[string]any{"Authorization": "bearer-sensitive-value", "rows": []any{map[string]any{"access_token": "nested-sensitive-value", "description": "known-sensitive-value"}}, "safe": "retained"},
		slog.Group("config", "dsn", "dsn-sensitive-value"))
	func() { defer Recover(logger, "worker"); panic("panic known-sensitive-value") }()
	output := console.String()
	for _, secret := range []string{"known-sensitive-value", "quoted-\\\"secret", "field-sensitive-value", "bearer-sensitive-value", "nested-sensitive-value", "dsn-sensitive-value"} {
		if strings.Contains(output, secret) {
			t.Fatal("sensitive value was logged")
		}
	}
	got := records(t, console.Bytes())
	if len(got) != 2 || !strings.Contains(output, "retained") || !strings.Contains(output, "upstream") || !strings.Contains(output, "stack") || !strings.Contains(output, "worker") {
		t.Fatal("diagnostic context was lost")
	}
}

func TestConcurrentLogging(t *testing.T) {
	var console bytes.Buffer
	opts := options(t, &console)
	logger, closer, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; n < 30; n++ {
				logger.Info("concurrent", "id", id, "n", n)
			}
		}(i)
	}
	wg.Wait()
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if len(records(t, console.Bytes())) != 300 {
		t.Fatal("lost concurrent records")
	}
	data, err := os.ReadFile(opts.File)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, console.Bytes()) {
		t.Fatal("concurrent outputs differ")
	}
}

func TestSensitiveGroupsAndNestedValues(t *testing.T) {
	var console bytes.Buffer
	opts := options(t, &console)
	opts.Secrets = []string{"known-value"}
	logger, closer, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	logger.Info("groups", slog.Group("credentials", "value", "group-sensitive"), "nested", struct {
		Password string `json:"p"`
		Error    error  `json:"error"`
	}{Password: "struct-sensitive", Error: errors.New("known-value detail")}, "raw", json.RawMessage(`{"token":"raw-sensitive","count":3}`))
	output := console.String()
	for _, secret := range []string{"group-sensitive", "struct-sensitive", "known-value", "raw-sensitive"} {
		if strings.Contains(output, secret) {
			t.Fatal("nested sensitive value was logged")
		}
	}
	got := records(t, console.Bytes())
	if got[0]["raw"].(map[string]any)["count"] != float64(3) {
		t.Fatal("JSON numbers were not preserved")
	}
}

func TestValidationAndEarlyOpenFailure(t *testing.T) {
	for _, mutate := range []func(*Options){func(o *Options) { o.Level = "invalid" }, func(o *Options) { o.File = "" }, func(o *Options) { o.MaxSizeMB = -1 }, func(o *Options) { o.MaxBackups = -1 }, func(o *Options) { o.MaxAgeDays = -1 }, func(o *Options) { o.File = t.TempDir() }} {
		var console bytes.Buffer
		opts := options(t, &console)
		mutate(&opts)
		if _, closer, err := New(opts); err == nil {
			closer.Close()
			t.Fatal("invalid options accepted")
		}
	}
}

func TestRotation(t *testing.T) {
	var console bytes.Buffer
	opts := options(t, &console)
	logger, closer, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		logger.Info("rotation", "sequence", i, "payload", strings.Repeat("x", 100_000))
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(opts.File), "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Fatalf("expected rotated files, got %v", files)
	}
	count := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		count += len(records(t, data))
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatal("rotated log has loose permissions")
		}
	}
	if count != 12 {
		t.Fatal(fmt.Sprintf("rotation lost records: %d", count))
	}
}
