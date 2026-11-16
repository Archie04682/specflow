// Package config resolves runtime settings from flags and the environment.
package config

import (
	"fmt"
	"strconv"
)

// DefaultPort is used when neither the --port flag nor PORT is set.
const DefaultPort = 8080

// ResolvePort returns the listen port using the precedence
// --port flag > PORT environment variable > DefaultPort.
//
// flagSet reports whether the caller explicitly passed the flag, which is
// how the flag wins over the environment even when both are present.
// getenv is injected so the resolution is testable without touching the
// process environment.
func ResolvePort(flagValue string, flagSet bool, getenv func(string) string) (int, error) {
	var raw string
	if flagSet {
		raw = flagValue
	} else {
		raw = getenv("PORT")
	}

	// No flag and no environment variable: use the default.
	if raw == "" && !flagSet {
		return DefaultPort, nil
	}
	if raw == "" {
		return 0, fmt.Errorf("invalid port %q: must be a number", raw)
	}

	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: must be a number", raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %d: must be between 1 and 65535", port)
	}
	return port, nil
}
