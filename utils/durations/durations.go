// Copyright 2024 NetApp, Inc. All Rights Reserved.

package durations

import (
	"time"

	"github.com/netapp/trident/utils/errors"
)

// TimeDuration tracks time durations
type TimeDuration map[string]time.Time

// InitStartTime adds a start time for a given key if not already set
func (t *TimeDuration) InitStartTime(key string) {
	if *t == nil {
		*t = make(map[string]time.Time)
	}
	if _, exists := (*t)[key]; !exists {
		(*t)[key] = time.Now()
	}
}

// GetCurrentDuration returns the current time duration for a given key
func (t *TimeDuration) GetCurrentDuration(key string) (time.Duration, error) {
	value, exists := (*t)[key]
	if !exists {
		return 0, errors.NotFoundError("not tracking duration for key '%s'", key)
	}
	return time.Since(value), nil
}

// RemoveDurationTracking removes duration tracking for a given key
func (t *TimeDuration) RemoveDurationTracking(key string) {
	delete(*t, key)
}
