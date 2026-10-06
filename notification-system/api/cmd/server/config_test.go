package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseConfig(t *testing.T) {
	required := map[string]string{
		"DATABASE_URL": "postgres://db",
		"AMQP_URL":     "amqp://mq",
		"JWT_SECRET":   "secret",
		"ADMIN_KEY":    "key",
	}
	tests := []struct {
		name string
		// set is applied on top of required; an empty value removes the variable.
		set     map[string]string
		want    config
		wantErr bool
	}{
		{
			name: "defaults",
			want: config{DatabaseURL: "postgres://db", AMQPURL: "amqp://mq", JWTSecret: "secret", AdminKey: "key", Addr: ":8080", MetricsAddr: ":9090"},
		},
		{
			name: "everything set",
			set:  map[string]string{"ADDR": ":9000", "METRICS_ADDR": ":9001", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318"},
			want: config{DatabaseURL: "postgres://db", AMQPURL: "amqp://mq", JWTSecret: "secret", AdminKey: "key", Addr: ":9000", MetricsAddr: ":9001", OTLPEndpoint: "http://collector:4318"},
		},
		{name: "no DATABASE_URL", set: map[string]string{"DATABASE_URL": ""}, wantErr: true},
		{name: "no AMQP_URL", set: map[string]string{"AMQP_URL": ""}, wantErr: true},
		{name: "no JWT_SECRET", set: map[string]string{"JWT_SECRET": ""}, wantErr: true},
		{name: "no ADMIN_KEY", set: map[string]string{"ADMIN_KEY": ""}, wantErr: true},
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

func TestParseConfigRejectsEmptyRequired(t *testing.T) {
	environ := map[string]string{"DATABASE_URL": "", "AMQP_URL": "amqp://mq", "JWT_SECRET": "secret", "ADMIN_KEY": "key"}
	if _, err := parseConfig(environ); err == nil {
		t.Errorf("parseConfig(%v) succeeded, want an error for the empty DATABASE_URL", environ)
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
