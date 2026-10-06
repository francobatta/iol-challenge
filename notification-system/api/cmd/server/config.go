package main

import "github.com/caarlos0/env/v11"

// config is what the server reads from its environment.
type config struct {
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	// AMQPURL is the RabbitMQ connection string.
	AMQPURL string `env:"AMQP_URL,required,notEmpty"`
	// JWTSecret signs app tokens.
	JWTSecret string `env:"JWT_SECRET,required,notEmpty"`
	// AdminKey authorizes creating apps.
	AdminKey string `env:"ADMIN_KEY,required,notEmpty"`
	// Addr is the address to listen on.
	Addr string `env:"ADDR" envDefault:":8080"`
	// MetricsAddr is the address to serve /metrics on.
	MetricsAddr string `env:"METRICS_ADDR" envDefault:":9090"`
	// OTLPEndpoint is where traces are exported. Empty exports none.
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	// PrometheusURL is the base URL of the Prometheus that scrapes the services, which
	// GET /v1/metrics reads from. Empty makes that route answer 503.
	PrometheusURL string `env:"PROMETHEUS_URL"`
}

// parseConfig reads the config from environ, which maps variable names to values.
func parseConfig(environ map[string]string) (config, error) {
	return env.ParseAsWithOptions[config](env.Options{Environment: environ})
}
