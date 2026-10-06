package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		environ map[string]string
		want    config
		wantErr bool
	}{
		{name: "defaults", want: config{Addr: ":8081"}},
		{
			name:    "everything set",
			environ: map[string]string{"ADDR": ":9000", "ERROR_RATE": "0.25", "THROTTLE_RATE": "0.5"},
			want:    config{Addr: ":9000", ErrorRate: 0.25, ThrottleRate: 0.5},
		},
		{name: "ERROR_RATE not a number", environ: map[string]string{"ERROR_RATE": "often"}, wantErr: true},
		{name: "ERROR_RATE above 1", environ: map[string]string{"ERROR_RATE": "1.5"}, wantErr: true},
		{name: "THROTTLE_RATE negative", environ: map[string]string{"THROTTLE_RATE": "-0.1"}, wantErr: true},
		{name: "rates add up to more than 1", environ: map[string]string{"ERROR_RATE": "0.6", "THROTTLE_RATE": "0.6"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseConfig(test.environ)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseConfig(%v) error = %v, want error: %t", test.environ, err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("parseConfig(%v) mismatch (-want +got):\n%s", test.environ, diff)
			}
		})
	}
}
