package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseConfig(t *testing.T) {
	required := map[string]string{
		"PROVIDER":         "twilio",
		"AMQP_URL":         "amqp://mq",
		"RABBITMQ_API_URL": "http://mq:15672",
		"PROVIDER_URL":     "http://provider",
	}
	base := config{
		Provider:       "twilio",
		AMQPURL:        "amqp://mq",
		RabbitMQAPIURL: "http://mq:15672",
		ProviderURL:    "http://provider",
		Concurrency:    200,
		Prefetch:       50,
		MetricsAddr:    ":9090",
	}
	everything := base
	everything.Concurrency = 10
	everything.Prefetch = 5
	everything.MetricsAddr = ":9000"
	everything.OTLPEndpoint = "http://collector:4318"

	tests := []struct {
		name string
		// set is applied on top of required; an empty value removes the variable.
		set     map[string]string
		want    config
		wantErr bool
	}{
		{name: "defaults", want: base},
		{
			name: "everything set",
			set: map[string]string{
				"CONCURRENCY":                 "10",
				"PREFETCH":                    "5",
				"METRICS_ADDR":                ":9000",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
			},
			want: everything,
		},
		{name: "no PROVIDER", set: map[string]string{"PROVIDER": ""}, wantErr: true},
		{name: "no AMQP_URL", set: map[string]string{"AMQP_URL": ""}, wantErr: true},
		{name: "no RABBITMQ_API_URL", set: map[string]string{"RABBITMQ_API_URL": ""}, wantErr: true},
		{name: "no PROVIDER_URL", set: map[string]string{"PROVIDER_URL": ""}, wantErr: true},
		{name: "CONCURRENCY not a number", set: map[string]string{"CONCURRENCY": "many"}, wantErr: true},
		{name: "CONCURRENCY zero", set: map[string]string{"CONCURRENCY": "0"}, wantErr: true},
		{name: "PREFETCH negative", set: map[string]string{"PREFETCH": "-1"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environ := environWith(required, test.set)
			got, err := parseConfig(environ)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseConfig(%v) error = %v, want error: %t", environ, err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("parseConfig(%v) mismatch (-want +got):\n%s", environ, diff)
			}
		})
	}
}

// environWith returns base with set applied; an empty value in set removes the variable.
func environWith(base, set map[string]string) map[string]string {
	environ := make(map[string]string)
	for name, value := range base {
		environ[name] = value
	}
	for name, value := range set {
		if value == "" {
			delete(environ, name)
			continue
		}
		environ[name] = value
	}
	return environ
}
