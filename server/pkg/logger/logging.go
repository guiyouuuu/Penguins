// Package logger provides the service's structured console and rotating file logs.
package logger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	Level      string
	File       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
	Secrets    []string
	Console    io.Writer
}

// New opens the log file immediately, so startup reports inaccessible destinations.
// Zero rotation settings default to 10 MB, 5 backups, and 30 days.
func New(opts Options) (*slog.Logger, io.Closer, error) {
	var level slog.Level
	switch strings.ToLower(strings.TrimSpace(opts.Level)) {
	case "", "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, nil, errors.New("invalid log level: expected debug, info, warn, or error")
	}
	if strings.TrimSpace(opts.File) == "" {
		return nil, nil, errors.New("log file is required")
	}
	if opts.MaxSizeMB < 0 || opts.MaxBackups < 0 || opts.MaxAgeDays < 0 {
		return nil, nil, errors.New("log rotation settings cannot be negative")
	}
	if opts.MaxSizeMB == 0 {
		opts.MaxSizeMB = 10
	}
	if opts.MaxBackups == 0 {
		opts.MaxBackups = 5
	}
	if opts.MaxAgeDays == 0 {
		opts.MaxAgeDays = 30
	}
	if err := os.MkdirAll(filepath.Dir(opts.File), 0700); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	if info, err := os.Lstat(opts.File); err == nil && !info.Mode().IsRegular() {
		return nil, nil, errors.New("log destination must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("inspect log file: %w", err)
	}
	file, err := os.OpenFile(opts.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	chmodErr := file.Chmod(0600)
	closeErr := file.Close()
	if err := errors.Join(chmodErr, closeErr); err != nil {
		return nil, nil, fmt.Errorf("initialize log file: %w", err)
	}
	rolling := &lumberjack.Logger{Filename: opts.File, MaxSize: opts.MaxSizeMB, MaxBackups: opts.MaxBackups, MaxAge: opts.MaxAgeDays, Compress: opts.Compress}
	console := opts.Console
	if console == nil {
		console = os.Stdout
	}
	redact := newRedactor(opts.Secrets)
	handler := slog.NewJSONHandler(fanoutWriter{console: console, file: rolling}, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			for _, group := range groups {
				if sensitiveKey(group) {
					return slog.String(redact.text(attr.Key), "[REDACTED]")
				}
			}
			if sensitiveKey(attr.Key) {
				return slog.String(redact.text(attr.Key), "[REDACTED]")
			}
			attr.Key = redact.text(attr.Key)
			switch attr.Value.Kind() {
			case slog.KindString:
				attr.Value = slog.StringValue(redact.text(attr.Value.String()))
			case slog.KindAny:
				attr.Value = slog.AnyValue(redact.value(reflect.ValueOf(attr.Value.Any()), 0))
			case slog.KindTime:
				attr.Value = slog.TimeValue(attr.Value.Time().UTC())
			}
			return attr
		},
	})
	return slog.New(handler), rolling, nil
}

// Recover must be deferred directly by the goroutine whose panic it handles.
func Recover(logger *slog.Logger, operation string) {
	if value := recover(); value != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error("goroutine panic recovered", "operation", operation, "panic", fmt.Sprint(value), "stack", string(debug.Stack()))
	}
}

type fanoutWriter struct{ console, file io.Writer }

// JSONHandler serializes writes, keeping each JSON record intact in both destinations.
func (w fanoutWriter) Write(p []byte) (int, error) {
	nConsole, consoleErr := w.console.Write(p)
	nFile, fileErr := w.file.Write(p)
	if nConsole != len(p) && consoleErr == nil {
		consoleErr = io.ErrShortWrite
	}
	if nFile != len(p) && fileErr == nil {
		fileErr = io.ErrShortWrite
	}
	return min(nConsole, nFile), errors.Join(consoleErr, fileErr)
}

type redactor struct{ replacer *strings.Replacer }

func newRedactor(secrets []string) redactor {
	values := append([]string(nil), secrets...)
	sort.SliceStable(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	pairs := make([]string, 0, len(values)*2)
	for _, value := range values {
		if value != "" {
			pairs = append(pairs, value, "[REDACTED]")
		}
	}
	return redactor{replacer: strings.NewReplacer(pairs...)}
}

func (r redactor) text(value string) string { return r.replacer.Replace(value) }

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	for _, sensitive := range []string{"password", "passwd", "token", "authorization", "cookie", "secret", "dsn", "credential", "privatekey", "apikey", "accesskey", "sessionid"} {
		if strings.Contains(key, sensitive) {
			return true
		}
	}
	return false
}

// Limit traversal depth to safely handle cyclic application values in diagnostic logs.
func (r redactor) value(v reflect.Value, depth int) any {
	if !v.IsValid() {
		return nil
	}
	if depth > 20 {
		return "[MAX_DEPTH]"
	}
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
		return nil
	}
	if v.CanInterface() {
		switch value := v.Interface().(type) {
		case slog.Level:
			return value.String()
		case error:
			return r.text(value.Error())
		case time.Time:
			return value.UTC()
		case json.Number:
			return value
		case []byte:
			return r.text(string(value))
		case slog.LogValuer:
			return r.value(reflect.ValueOf(slog.AnyValue(value).Resolve().Any()), depth+1)
		case json.RawMessage:
			var parsed any
			decoder := json.NewDecoder(strings.NewReader(string(value)))
			decoder.UseNumber()
			if decoder.Decode(&parsed) == nil {
				return r.value(reflect.ValueOf(parsed), depth+1)
			}
			return r.text(string(value))
		}
	}
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Pointer, reflect.Interface:
		return r.value(v.Elem(), depth+1)
	case reflect.String:
		return r.text(v.String())
	case reflect.Map:
		result := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := fmt.Sprint(iter.Key().Interface())
			if sensitiveKey(key) {
				result[r.text(key)] = "[REDACTED]"
			} else {
				result[r.text(key)] = r.value(iter.Value(), depth+1)
			}
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			result[i] = r.value(v.Index(i), depth+1)
		}
		return result
	case reflect.Struct:
		result := make(map[string]any)
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if sensitiveKey(name) || sensitiveKey(field.Name) {
				result[r.text(name)] = "[REDACTED]"
			} else {
				result[r.text(name)] = r.value(v.Field(i), depth+1)
			}
		}
		return result
	default:
		if v.CanInterface() {
			return r.text(fmt.Sprint(v.Interface()))
		}
		return "[UNAVAILABLE]"
	}
}
