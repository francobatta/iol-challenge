package main

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// config is what the mock provider reads from its environment.
type config struct {
	// Addr is the address to listen on.
	Addr string `env:"ADDR" envDefault:":8081"`
	// ErrorRate is the fraction of requests answered 500, from 0 to 1.
	ErrorRate float64 `env:"ERROR_RATE" envDefault:"0"`
	// ThrottleRate is the fraction of requests answered 429, from 0 to 1.
	ThrottleRate float64 `env:"THROTTLE_RATE" envDefault:"0"`
}

// parseConfig reads the config from environ, which maps variable names to values.
func parseConfig(environ map[string]string) (config, error) {
	cfg, err := env.ParseAsWithOptions[config](env.Options{Environment: environ})
	if err != nil {
		return config{}, err
	}
	if cfg.ErrorRate < 0 || cfg.ErrorRate > 1 {
		return config{}, fmt.Errorf("ERROR_RATE must be a number from 0 to 1, not %g", cfg.ErrorRate)
	}
	if cfg.ThrottleRate < 0 || cfg.ThrottleRate > 1 {
		return config{}, fmt.Errorf("THROTTLE_RATE must be a number from 0 to 1, not %g", cfg.ThrottleRate)
	}
	if sum := cfg.ErrorRate + cfg.ThrottleRate; sum > 1 {
		return config{}, fmt.Errorf("ERROR_RATE and THROTTLE_RATE add up to %g, more than 1", sum)
	}
	return cfg, nil
}
