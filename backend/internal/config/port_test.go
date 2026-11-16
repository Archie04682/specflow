package config

import (
	"strings"
	"testing"
)

func TestResolvePort(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		flagSet   bool
		env       string
		want      int
		wantErr   string
	}{
		{name: "default when nothing is set", want: DefaultPort},
		{name: "flag wins", flagValue: "9090", flagSet: true, env: "9091", want: 9090},
		{name: "env used without flag", env: "9091", want: 9091},
		{name: "env ignored when flag set", flagValue: "9090", flagSet: true, env: "9091", want: 9090},
		{name: "non-numeric flag", flagValue: "not-a-number", flagSet: true, wantErr: "not-a-number"},
		{name: "non-numeric env", env: "not-a-number", wantErr: "not-a-number"},
		{name: "zero is rejected", flagValue: "0", flagSet: true, wantErr: "invalid port 0"},
		{name: "above range is rejected", flagValue: "70000", flagSet: true, wantErr: "invalid port 70000"},
		{name: "empty flag is rejected", flagValue: "", flagSet: true, wantErr: `invalid port ""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == "PORT" {
					return tt.env
				}
				return ""
			}

			got, err := ResolvePort(tt.flagValue, tt.flagSet, getenv)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolvePort() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ResolvePort() error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePort() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolvePort() = %d, want %d", got, tt.want)
			}
		})
	}
}
