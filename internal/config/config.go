// Package config loads and validates runtime configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config is the resolved runtime configuration.
type Config struct {
	APIID   int
	APIHash string

	// SessionString is an optional seed (native base64 or Telethon string),
	// imported into the local store on first run. The session then lives in the
	// bbolt database and this can be unset.
	SessionString string

	// ControlChat is "me", an @username, or a numeric chat id.
	ControlChat string

	// BotToken, when set, also enables the bot DM flow.
	BotToken string

	TempDir   string
	CachePath string

	HealthAddr string
	HealthFile string

	LogLevel  slog.Level
	LogFormat string // "text" or "json"

	// Forward-mode only.
	ForwardFrom string
	ForwardTo   string
}

// Mode selects which subset of configuration is required.
type Mode int

const (
	ModeRun Mode = iota
	ModeGenSession
	ModeForward
)

// Load reads configuration for the given mode, returning an aggregated error
// listing every missing or malformed variable.
func Load(mode Mode) (*Config, error) {
	c := &Config{
		ControlChat:   envDefault("CONTROL_CHAT", "me"),
		BotToken:      os.Getenv("BOT_TOKEN"),
		TempDir:       envDefault("TEMP_DIR", os.TempDir()),
		CachePath:     envDefault("CACHE_PATH", "/data/cache.db"),
		HealthAddr:    envDefault("HEALTH_ADDR", ":8080"),
		HealthFile:    envDefault("HEALTH_FILE", "/tmp/healthz"),
		LogFormat:     strings.ToLower(envDefault("LOG_FORMAT", "text")),
		SessionString: firstNonEmpty(os.Getenv("SESSION_STRING"), os.Getenv("STRING_SESSION")),
		ForwardFrom:   os.Getenv("FORWARD_FROM"),
		ForwardTo:     os.Getenv("FORWARD_TO"),
	}

	var errs []string
	req := func(name, val string) {
		if strings.TrimSpace(val) == "" {
			errs = append(errs, name+" is required")
		}
	}

	if raw := os.Getenv("API_ID"); raw == "" {
		errs = append(errs, "API_ID is required")
	} else if id, err := strconv.Atoi(strings.TrimSpace(raw)); err != nil {
		errs = append(errs, "API_ID must be an integer")
	} else {
		c.APIID = id
	}
	c.APIHash = strings.TrimSpace(os.Getenv("API_HASH"))
	req("API_HASH", c.APIHash)

	if mode == ModeForward {
		req("FORWARD_FROM", c.ForwardFrom)
		req("FORWARD_TO", c.ForwardTo)
	}

	switch strings.ToLower(envDefault("LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info", "":
		c.LogLevel = slog.LevelInfo
	case "warn", "warning":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		errs = append(errs, "LOG_LEVEL must be one of debug, info, warn, error")
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		errs = append(errs, "LOG_FORMAT must be text or json")
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return c, nil
}

// BotEnabled reports whether the optional bot DM mode is configured.
func (c *Config) BotEnabled() bool { return strings.TrimSpace(c.BotToken) != "" }

// Logger builds the configured slog logger.
func (c *Config) Logger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	var h slog.Handler
	if c.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

func envDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
