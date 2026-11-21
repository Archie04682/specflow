package config

import (
	"strings"
	"testing"
)

func TestResolveProvisioning(t *testing.T) {
	tests := []struct {
		name        string
		sourceFlag  string
		sourceSet   bool
		offlineFlag bool
		offlineSet  bool
		sourceEnv   string
		offlineEnv  string
		wantSource  string
		wantOffline bool
		wantErr     string
	}{
		{name: "neither is set"},
		{name: "env supplies the source", sourceEnv: "/srv/skills", wantSource: "/srv/skills"},
		{name: "flag supplies the source", sourceFlag: "/local", sourceSet: true, wantSource: "/local"},
		{name: "flag beats env for the source", sourceFlag: "/local", sourceSet: true, sourceEnv: "/srv/skills", wantSource: "/local"},
		{name: "offline from the env", offlineEnv: "1", wantOffline: true},
		{name: "offline from the flag", offlineFlag: true, offlineSet: true, wantOffline: true},
		{name: "flag beats env for offline", offlineFlag: false, offlineSet: true, offlineEnv: "1"},
		{name: "env false is not offline", offlineEnv: "false"},
		{name: "invalid offline env", offlineEnv: "maybe", wantErr: "invalid SPECFLOW_OFFLINE"},
		{name: "empty flag source is rejected", sourceFlag: "  ", sourceSet: true, wantErr: "empty"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(key string) string {
				switch key {
				case "SPECFLOW_SKILLS_SOURCE":
					return test.sourceEnv
				case "SPECFLOW_OFFLINE":
					return test.offlineEnv
				}
				return ""
			}

			got, err := ResolveProvisioning(
				test.sourceFlag, test.sourceSet,
				test.offlineFlag, test.offlineSet,
				getenv,
			)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.SkillsSource != test.wantSource || got.Offline != test.wantOffline {
				t.Fatalf("got %+v, want source %q offline %v", got, test.wantSource, test.wantOffline)
			}
		})
	}
}
