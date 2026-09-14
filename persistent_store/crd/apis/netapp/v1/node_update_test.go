// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

// TestNodeConfigKeysMatchStructTags pins the literal JSON keys used by the merge-patch
// helpers to the real storage.VolumeConfig json tags, so a future rename of the Go fields cannot
// silently desync the patch from the struct.
func TestNodeConfigKeysMatchStructTags(t *testing.T) {
	structType := reflect.TypeOf(storage.VolumeConfig{})

	jsonTag := func(fieldName string) string {
		field, ok := structType.FieldByName(fieldName)
		require.True(t, ok, "storage.VolumeConfig has no field %s", fieldName)
		tag := field.Tag.Get("json")
		require.NotEmpty(t, tag, "field %s has no json tag", fieldName)
		// Strip options like ",omitempty".
		for i, c := range tag {
			if c == ',' {
				return tag[:i]
			}
		}
		return tag
	}

	assert.Equal(t, jsonTag("LUKSPassphraseNames"), ConfigKeyLUKSPassphraseNames)
	assert.ElementsMatch(t, []string{ConfigKeyLUKSPassphraseNames}, nodeConfigKeys)
}

func TestNodeVolumeUpdateMergePatch(t *testing.T) {
	ptr := func(s []string) *[]string { return &s }

	tests := []struct {
		name              string
		existingConfigRaw []byte
		update            *models.NodeVolumeUpdate
		wantChanged       bool
		wantPatch         map[string]any // decoded expectation, nil if wantChanged is false
	}{
		{
			name:              "nil update is a no-op",
			existingConfigRaw: []byte(`{}`),
			update:            nil,
			wantChanged:       false,
		},
		{
			name:              "empty update (nil field) is a no-op",
			existingConfigRaw: []byte(`{}`),
			update:            &models.NodeVolumeUpdate{},
			wantChanged:       false,
		},
		{
			name:              "absent names key is set",
			existingConfigRaw: []byte(`{"size":"1Gi"}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"a"})},
			wantChanged:       true,
			wantPatch: map[string]any{
				"config": map[string]any{
					"luksPassphraseNames": []any{"a"},
				},
			},
		},
		{
			name:              "differing names are patched",
			existingConfigRaw: []byte(`{"luksPassphraseNames":["a"]}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"a", "b"})},
			wantChanged:       true,
			wantPatch: map[string]any{
				"config": map[string]any{
					"luksPassphraseNames": []any{"a", "b"},
				},
			},
		},
		{
			name:              "value unchanged -> no patch",
			existingConfigRaw: []byte(`{"luksPassphraseNames":["a"]}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"a"})},
			wantChanged:       false,
		},
		{
			name:              "reordered names are unchanged -> no patch",
			existingConfigRaw: []byte(`{"luksPassphraseNames":["a","b"]}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"b", "a"})},
			wantChanged:       false,
		},
		{
			name:              "explicit empty slice against absent key is unchanged (both mean 'no names')",
			existingConfigRaw: []byte(`{}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{})},
			wantChanged:       false,
		},
		{
			name:              "explicit empty slice clears a populated value",
			existingConfigRaw: []byte(`{"luksPassphraseNames":["a"]}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{})},
			wantChanged:       true,
			wantPatch: map[string]any{
				"config": map[string]any{
					"luksPassphraseNames": []any{},
				},
			},
		},
		{
			name:              "pointer to nil slice normalizes to empty, never emits null",
			existingConfigRaw: []byte(`{"luksPassphraseNames":["a"]}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: new([]string)},
			wantChanged:       true,
			wantPatch: map[string]any{
				"config": map[string]any{
					"luksPassphraseNames": []any{},
				},
			},
		},
		{
			name:              "unknown sibling config keys are irrelevant to the diff",
			existingConfigRaw: []byte(`{"someFutureField":"x","luksPassphraseNames":["a"]}`),
			update:            &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"a"})},
			wantChanged:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, changed, err := NodeVolumeUpdateMergePatch(tt.existingConfigRaw, tt.update)
			require.NoError(t, err)
			assert.Equal(t, tt.wantChanged, changed)
			if !tt.wantChanged {
				assert.Nil(t, patch)
				return
			}
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(patch, &decoded))
			assert.Equal(t, tt.wantPatch, decoded)
		})
	}

	t.Run("malformed existingConfigRaw errors", func(t *testing.T) {
		_, _, err := NodeVolumeUpdateMergePatch([]byte(`not json`), &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"a"})})
		assert.Error(t, err)
	})

	t.Run("malformed existing names errors", func(t *testing.T) {
		raw := []byte(`{"luksPassphraseNames":"not-an-array"}`)
		_, _, err := NodeVolumeUpdateMergePatch(raw, &models.NodeVolumeUpdate{LUKSPassphraseNames: ptr([]string{"b"})})
		assert.Error(t, err)
	})
}

func TestPreserveNodeConfigFields(t *testing.T) {
	t.Run("node-owned keys present in existing are copied over target", func(t *testing.T) {
		existing := []byte(`{"luksPassphraseNames":["a"],"size":"1Gi"}`)
		target := &runtime.RawExtension{Raw: []byte(`{"size":"2Gi"}`)}

		require.NoError(t, PreserveNodeConfigFields(existing, target))

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(target.Raw, &decoded))
		assert.Equal(t, []any{"a"}, decoded["luksPassphraseNames"])
		assert.Equal(t, "2Gi", decoded["size"], "non-node-owned keys from target must survive untouched")
	})

	t.Run("target's own stale node-owned values are overwritten", func(t *testing.T) {
		existing := []byte(`{"luksPassphraseNames":["current"]}`)
		target := &runtime.RawExtension{Raw: []byte(`{"luksPassphraseNames":["stale"]}`)}

		require.NoError(t, PreserveNodeConfigFields(existing, target))

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(target.Raw, &decoded))
		assert.Equal(t, []any{"current"}, decoded["luksPassphraseNames"])
	})

	t.Run("absent in existing -> no-op, target untouched", func(t *testing.T) {
		existing := []byte(`{"size":"1Gi"}`)
		originalRaw := []byte(`{"size":"2Gi"}`)
		target := &runtime.RawExtension{Raw: originalRaw}

		require.NoError(t, PreserveNodeConfigFields(existing, target))

		assert.Equal(t, originalRaw, target.Raw, "must not rewrite target.Raw when nothing to preserve")
	})

	t.Run("empty existingRaw -> no-op", func(t *testing.T) {
		originalRaw := []byte(`{"size":"2Gi"}`)
		target := &runtime.RawExtension{Raw: originalRaw}

		require.NoError(t, PreserveNodeConfigFields(nil, target))

		assert.Equal(t, originalRaw, target.Raw)
	})

	t.Run("unknown keys in existing do not leak into target", func(t *testing.T) {
		existing := []byte(`{"luksPassphraseNames":["a"],"someOtherKey":"x"}`)
		target := &runtime.RawExtension{Raw: []byte(`{"size":"1Gi"}`)}

		require.NoError(t, PreserveNodeConfigFields(existing, target))

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(target.Raw, &decoded))
		_, hasOtherKey := decoded["someOtherKey"]
		assert.False(t, hasOtherKey)
	})

	t.Run("malformed existingRaw errors", func(t *testing.T) {
		target := &runtime.RawExtension{Raw: []byte(`{}`)}
		err := PreserveNodeConfigFields([]byte(`not json`), target)
		assert.Error(t, err)
	})

	t.Run("malformed target.Raw errors", func(t *testing.T) {
		existing := []byte(`{"luksPassphraseNames":["a"]}`)
		target := &runtime.RawExtension{Raw: []byte(`not json`)}
		err := PreserveNodeConfigFields(existing, target)
		assert.Error(t, err)
	})
}
