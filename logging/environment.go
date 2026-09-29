// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"sort"
	"strings"
)

// tridentEnvPrefix namespaces Trident's own environment variables. Their values are deployment
// knobs (images, ports, feature flags) that the startup dump exists to show; the values of any
// other variables are not ours to log.
const tridentEnvPrefix = "TRIDENT_"

// credentialKeyMarkers keep the prefix rule above from logging the value of a credential-bearing
// TRIDENT_* variable, should one be added later.
var credentialKeyMarkers = []string{"PASSWORD", "SECRET", "TOKEN", "KEY", "CREDENTIAL"}

// EnvironmentFields reduces an os.Environ() slice to Trident's own variables with their values, and
// every other variable by name only. It collapses the startup dump into one entry instead of one
// log line per variable.
func EnvironmentFields(environ []string) LogFields {
	tridentValues := make(map[string]string)
	otherKeys := make([]string, 0, len(environ))

	for _, entry := range environ {
		key, value, found := strings.Cut(entry, "=")
		if !found || key == "" {
			continue
		}

		if isTridentVariable(key) {
			tridentValues[key] = value
			continue
		}

		otherKeys = append(otherKeys, key)
	}

	sort.Strings(otherKeys)

	return LogFields{
		"environment":     tridentValues,
		"environmentKeys": otherKeys,
	}
}

func isTridentVariable(key string) bool {
	if !strings.HasPrefix(key, tridentEnvPrefix) {
		return false
	}

	for _, marker := range credentialKeyMarkers {
		if strings.Contains(key, marker) {
			return false
		}
	}

	return true
}
