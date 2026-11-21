package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Provisioning is how the server may obtain a project's skills.
//
// SkillsSource, when set, is the only source: it makes the online path
// unreachable rather than merely deprioritized, so an air-gapped deployment has
// a guarantee instead of a hope. Offline disables every network-capable action.
type Provisioning struct {
	SkillsSource string
	Offline      bool
}

// ResolveProvisioning returns the provisioning settings using the same
// precedence as the port: the flag wins over the environment, and an unset flag
// lets the environment decide. flagSet reports whether the caller passed the
// flag, which is how an explicitly empty value is told apart from an absent one.
func ResolveProvisioning(
	skillsSourceFlag string,
	skillsSourceSet bool,
	offlineFlag bool,
	offlineSet bool,
	getenv func(string) string,
) (Provisioning, error) {
	var source string
	if skillsSourceSet {
		source = skillsSourceFlag
	} else {
		source = getenv("SPECFLOW_SKILLS_SOURCE")
	}
	source = strings.TrimSpace(source)
	if skillsSourceSet && source == "" {
		return Provisioning{}, fmt.Errorf("skills source is set but empty")
	}

	offline := offlineFlag
	if !offlineSet {
		raw := strings.TrimSpace(getenv("SPECFLOW_OFFLINE"))
		if raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return Provisioning{}, fmt.Errorf("invalid SPECFLOW_OFFLINE %q: must be true or false", raw)
			}
			offline = parsed
		}
	}

	return Provisioning{SkillsSource: source, Offline: offline}, nil
}
