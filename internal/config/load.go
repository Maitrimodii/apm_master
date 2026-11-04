package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// envFile is loaded from the working directory if present. Variables already
// set in the environment take precedence over it.
const envFile = ".env"

// Load reads configuration from the environment, after loading .env if it
// exists. A malformed value (for example a non-numeric *_MS setting) is an
// error rather than a silent fallback to the default.
func Load() (*Config, error) {
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load %s: %w", envFile, err)
	}
	return parse(nil)
}

// parse reads configuration from environ, or the process environment when nil.
func parse(environ map[string]string) (*Config, error) {
	var cfg Config
	opts := env.Options{
		Environment: environ,
		FuncMap: map[reflect.Type]env.ParserFunc{
			reflect.TypeOf(time.Duration(0)): parseMillis,
		},
	}
	if err := env.ParseWithOptions(&cfg, opts); err != nil {
		return nil, fmt.Errorf("parse configuration: %w", err)
	}
	// APP_URL is the clearer name; APPURL remains for existing deployments.
	lookup := os.LookupEnv
	if environ != nil {
		lookup = func(k string) (string, bool) { v, ok := environ[k]; return v, ok }
	}
	if v, ok := lookup("APP_URL"); ok && v != "" {
		cfg.App.URL = v
	}
	cfg.Kafka.Brokers = trimAll(cfg.Kafka.Brokers)
	cfg.App.AllowedOrigins = trimAll(cfg.App.AllowedOrigins)
	return &cfg, nil
}

// parseMillis reads a duration given as integer milliseconds.
func parseMillis(v string) (any, error) {
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("want integer milliseconds, got %q", v)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// trimAll trims list entries and drops empty ones ("a, b," -> [a b]).
func trimAll(items []string) []string {
	out := items[:0]
	for _, it := range items {
		if it = strings.TrimSpace(it); it != "" {
			out = append(out, it)
		}
	}
	return out
}
