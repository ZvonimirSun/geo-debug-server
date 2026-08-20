package config

import (
	"flag"
	"os"
	"strings"
)

type Config struct {
	Listen    string
	BasePath  string
	PublicURL string
	Database  string
	Version   bool
}

func Parse() Config {
	cfg := Config{}
	flag.StringVar(&cfg.Listen, "listen", envOr("GEO_DEBUG_LISTEN", ":8080"), "HTTP listen address")
	flag.StringVar(&cfg.BasePath, "base-path", envOr("GEO_DEBUG_BASE_PATH", "/geo-debug-server"), "HTTP base path")
	flag.StringVar(&cfg.PublicURL, "public-url", envOr("GEO_DEBUG_PUBLIC_URL", ""), "public base URL used in metadata")
	flag.StringVar(&cfg.Database, "database", envOr("GEO_DEBUG_DATABASE", "./data/geo-debug.db"), "SQLite database path")
	flag.BoolVar(&cfg.Version, "version", false, "print version information and exit")
	flag.Parse()
	cfg.BasePath = NormalizeBasePath(cfg.BasePath)
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	return cfg
}

func NormalizeBasePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	return "/" + strings.Trim(value, "/")
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
