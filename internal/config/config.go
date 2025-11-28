package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config describes runtime settings loaded from environment variables.
type Config struct {
	Port          string        `env:"PORT" envDefault:"8080"`
	TasksFile     string        `env:"TASKS_FILE" envDefault:"tasks.json"`
	HTTPTimeout   time.Duration `env:"HTTP_TIMEOUT" envDefault:"5s"`
	MaxLinks      int           `env:"MAX_LINKS" envDefault:"50"`
	MaxWorkers    int           `env:"MAX_WORKERS" envDefault:"100"`
	RateLimitRPS  float64       `env:"RATE_LIMIT_RPS" envDefault:"10"`
	RateLimitBurst int          `env:"RATE_LIMIT_BURST" envDefault:"20"`
}

// Load reads configuration from environment variables, applying defaults when necessary.
func Load() (*Config, error) {
	cfg := &Config{
		Port:           "8080",
		TasksFile:      "tasks.json",
		HTTPTimeout:    5 * time.Second,
		MaxLinks:       50,
		MaxWorkers:     100,
		RateLimitRPS:   10,
		RateLimitBurst: 20,
	}

	if port := os.Getenv("PORT"); port != "" {
		cfg.Port = port
	}

	if tasksFile := os.Getenv("TASKS_FILE"); tasksFile != "" {
		cfg.TasksFile = tasksFile
	}

	if httpTimeout := os.Getenv("HTTP_TIMEOUT"); httpTimeout != "" {
		dur, err := time.ParseDuration(httpTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse HTTP_TIMEOUT: %w", err)
		}
		cfg.HTTPTimeout = dur
	}

	if maxLinks := os.Getenv("MAX_LINKS"); maxLinks != "" {
		value, err := strconv.Atoi(maxLinks)
		if err != nil {
			return nil, fmt.Errorf("parse MAX_LINKS: %w", err)
		}
		cfg.MaxLinks = value
	}

	if maxWorkers := os.Getenv("MAX_WORKERS"); maxWorkers != "" {
		value, err := strconv.Atoi(maxWorkers)
		if err != nil {
			return nil, fmt.Errorf("parse MAX_WORKERS: %w", err)
		}
		cfg.MaxWorkers = value
	}

	if rps := os.Getenv("RATE_LIMIT_RPS"); rps != "" {
		value, err := strconv.ParseFloat(rps, 64)
		if err != nil {
			return nil, fmt.Errorf("parse RATE_LIMIT_RPS: %w", err)
		}
		cfg.RateLimitRPS = value
	}

	if burst := os.Getenv("RATE_LIMIT_BURST"); burst != "" {
		value, err := strconv.Atoi(burst)
		if err != nil {
			return nil, fmt.Errorf("parse RATE_LIMIT_BURST: %w", err)
		}
		cfg.RateLimitBurst = value
	}

	return cfg, nil
}
