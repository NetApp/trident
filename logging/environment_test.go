// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentFieldsKeepsTridentValuesAndDropsOtherValues(t *testing.T) {
	const tridentValue = "trident.trident.svc.cluster.local:17201"
	const otherValue = "AKIAIOSFODNN7EXAMPLE"

	fields := EnvironmentFields([]string{
		"TRIDENT_SERVER=" + tridentValue,
		"TRIDENT_IMAGE=netapp/trident:26.10.0",
		"AWS_SECRET_ACCESS_KEY=" + otherValue,
		"TRIDENT_PASSWORD=" + otherValue,
		"PATH=/usr/bin:/bin",
		"NOEQUALS",
		"=novalue",
		"EMPTY=",
	})

	values, ok := fields["environment"].(map[string]string)
	require.True(t, ok, "environment field must be a map of Trident variables to their values")
	keys, ok := fields["environmentKeys"].([]string)
	require.True(t, ok, "environmentKeys field must be a list of remaining variable names")

	// A blanket change that logged no values at all would satisfy the leak assertions below, so the
	// debug purpose of the dump is asserted here as well.
	assert.Equal(t, tridentValue, values["TRIDENT_SERVER"])
	assert.Equal(t, "netapp/trident:26.10.0", values["TRIDENT_IMAGE"])

	assert.NotContains(t, values, "AWS_SECRET_ACCESS_KEY")
	assert.NotContains(t, values, "TRIDENT_PASSWORD")
	assert.Contains(t, keys, "AWS_SECRET_ACCESS_KEY")
	assert.Contains(t, keys, "EMPTY")
	assert.NotContains(t, keys, "NOEQUALS")
	assert.NotContains(t, keys, "=novalue")

	rendered := fmt.Sprintf("%v", fields)
	assert.NotContains(t, rendered, otherValue)
}

func TestEnvironmentFieldsSortsOtherKeys(t *testing.T) {
	fields := EnvironmentFields([]string{"ZZ_LAST=1", "AA_FIRST=2", "MM_MIDDLE=3"})

	keys, ok := fields["environmentKeys"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"AA_FIRST", "MM_MIDDLE", "ZZ_LAST"}, keys)
}

func TestEnvironmentFieldsFromEmptyEnvironment(t *testing.T) {
	fields := EnvironmentFields(nil)

	values, ok := fields["environment"].(map[string]string)
	require.True(t, ok)
	keys, ok := fields["environmentKeys"].([]string)
	require.True(t, ok)
	assert.Empty(t, values)
	assert.Empty(t, keys)
}
