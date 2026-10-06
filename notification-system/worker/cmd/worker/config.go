package main

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// config is what the worker reads from its environment.
type config struct {
	// Provider is the provider to send through: twilio, mailchimp, apns or fcm.
	Provider string `env:"PROVIDER,required,notEmpty"`
	// AMQPURL is the RabbitMQ connection string.
	AMQPURL string `env:"AMQP_URL,required,notEmpty"`
	// RabbitMQAPIURL is the RabbitMQ management API, such as http://user:pass@host:15672.
	RabbitMQAPIURL string `env:"RABBITMQ_API_URL,required,notEmpty"`
	// ProviderURL is the base URL of the provider's API.
	ProviderURL string `env:"PROVIDER_URL,required,notEmpty"`
	// Concurrency is the most requests to the provider at once.
	Concurrency int `env:"CONCURRENCY" envDefault:"200"`
	// Prefetch is the most deliveries held per app at once.
	Prefetch int `env:"PREFETCH" envDefault:"50"`
	// MetricsAddr is the address to serve /metrics on.
	MetricsAddr string `env:"METRICS_ADDR" envDefault:":9090"`
	// OTLPEndpoint is where traces are exported. Empty exports none.
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
}

// parseConfig reads the config from environ, which maps variable names to values.
func parseConfig(environ map[string]string) (config, error) {
	cfg, err := env.ParseAsWithOptions[config](env.Options{Environment: environ})
	if err != nil {
		return config{}, err
	}
	if cfg.Concurrency < 1 {
		return config{}, fmt.Errorf("CONCURRENCY must be a positive integer, not %d", cfg.Concurrency)
	}
	if cfg.Prefetch < 1 {
		return config{}, fmt.Errorf("PREFETCH must be a positive integer, not %d", cfg.Prefetch)
	}
	return cfg, nil
}
