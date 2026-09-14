// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

// NewTridentVolume creates a new storage class CRD object from a internal
// storage.VolumeExternal object
func NewTridentVolume(ctx context.Context, persistent *storage.VolumeExternal) (*TridentVolume, error) {
	volume := &TridentVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.Config.Name),
			Finalizers: GetTridentFinalizers(),
		},
		BackendUUID: persistent.BackendUUID,
	}

	if err := volume.Apply(ctx, persistent); err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"volume.Name":        volume.Name,
		"volume.BackendUUID": volume.BackendUUID,
		"volume.Orphaned":    volume.Orphaned,
		"volume.Pool":        volume.Pool,
	}).Debug("NewTridentVolume")

	return volume, nil
}

// Apply applies changes from an internal storage.VolumeExternal
// object to its Kubernetes CRD equivalent
func (in *TridentVolume) Apply(ctx context.Context, persistent *storage.VolumeExternal) error {
	Logc(ctx).WithFields(LogFields{
		"persistent.BackendUUID": persistent.BackendUUID,
		"persistent.Orphaned":    persistent.Orphaned,
		"persistent.Pool":        persistent.Pool,
		"persistent.State":       string(persistent.State),
	}).Debug("Applying volume update.")

	if NameFix(persistent.Config.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	in.Config.Raw = config
	in.BackendUUID = persistent.BackendUUID
	in.Orphaned = persistent.Orphaned
	in.Pool = persistent.Pool
	in.State = string(persistent.State)
	in.AutogrowStatus = persistent.AutogrowStatus

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storage.VolumeExternal equivalent
func (in *TridentVolume) Persistent() (*storage.VolumeExternal, error) {
	persistent := &storage.VolumeExternal{
		BackendUUID:    in.BackendUUID,
		Orphaned:       in.Orphaned,
		Pool:           in.Pool,
		Config:         &storage.VolumeConfig{},
		State:          storage.VolumeState(in.State),
		AutogrowStatus: in.AutogrowStatus,
	}

	if err := json.Unmarshal(in.Config.Raw, persistent.Config); err != nil {
		return nil, err
	}
	return persistent, nil
}

func (in *TridentVolume) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentVolume) GetKind() string {
	return "TridentVolume"
}

func (in *TridentVolume) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentVolume) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentVolume) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// Node-owned config keys: the storage.VolumeConfig JSON keys a Trident node may write via the
// TridentController UpdateVolume API. Literal JSON keys, not struct fields, so patches built from
// them are immune to struct-version skew between a node and a controller running different
// binaries during a rolling upgrade. TestNodeConfigKeysMatchStructTags pins them to the
// real storage.VolumeConfig json tags.
const (
	ConfigKeyLUKSPassphraseNames = "luksPassphraseNames" //nolint:gosec // JSON key name, not a credential
)

var nodeConfigKeys = []string{ConfigKeyLUKSPassphraseNames}

// NodeVolumeUpdateMergePatch computes a JSON merge patch against TridentVolume from the CR's
// current config (existingConfigRaw, i.e. Config.Raw) and a NodeVolumeUpdate. changed reports
// whether the update actually differs from the current value; when false, patch is nil and callers
// must skip the write entirely. When true, patch sets exactly the allowlisted keys touched by
// update.
//
// This deliberately never decodes existingConfigRaw into storage.VolumeConfig: doing so and
// re-marshalling would silently drop config keys this binary's struct version doesn't recognize -
// see the CRD transport's UpdateVolume doc comment for why that matters during a version-skewed
// rolling upgrade.
func NodeVolumeUpdateMergePatch(existingConfigRaw []byte, update *models.NodeVolumeUpdate) ([]byte, bool, error) {
	if update.IsEmpty() {
		return nil, false, nil
	}

	existing := map[string]json.RawMessage{}
	if len(existingConfigRaw) > 0 {
		if err := json.Unmarshal(existingConfigRaw, &existing); err != nil {
			return nil, false, fmt.Errorf("could not parse existing volume config: %w", err)
		}
	}

	configPatch := map[string]any{}
	changed := false

	if update.LUKSPassphraseNames != nil {
		// Defensive normalization: a pointer to a nil slice must not marshal to JSON null, which a
		// merge patch would interpret as "remove this key" rather than "set to empty." Both nil and
		// non-nil-empty callers intend the same thing here: clear the field.
		desired := *update.LUKSPassphraseNames
		if desired == nil {
			desired = []string{}
		}

		current, err := currentStringSlice(existing, ConfigKeyLUKSPassphraseNames)
		if err != nil {
			return nil, false, err
		}

		// Compared as an unordered set: no consumer of LUKSPassphraseNames reads a name by
		// position, so a reordering of the same names is not a change and must not produce a write.
		if !collection.EqualValues(desired, current) {
			configPatch[ConfigKeyLUKSPassphraseNames] = desired
			changed = true
		}
	}

	if !changed {
		return nil, false, nil
	}

	patch, err := json.Marshal(map[string]any{"config": configPatch})
	if err != nil {
		return nil, false, fmt.Errorf("could not build merge patch: %w", err)
	}
	return patch, true, nil
}

// PreserveNodeConfigFields copies node-owned config keys from the live CR's config JSON
// (existingRaw) into a controller-built config (target).
//
// Where: CRDClientV1.UpdateVolume, after Apply() and before the Kubernetes Update(). That
// path is every controller-driven volume write (resize, autogrow, state, clone bookkeeping,
// ...). Apply() marshals this process's in-memory VolumeConfig over Config.Raw, which would
// replace keys a node already persisted via UpdateVolumeNodeConfig / the node's JSON merge
// patch. Restoring from the CR just GETed keeps the node's latest durable write.
//
// Why not a field-scoped patch instead: the node's write already is one
// (NodeVolumeUpdateMergePatch: only luksPassphraseNames).
// Controller UpdateVolume is the opposite problem — many callers mutate many VolumeConfig
// fields — so converting every caller to a sparse patch is a much larger change. Copying
// node-owned keys back after the typed round-trip keeps that path as a full Update() without
// clobbering node writes. Keys are copied as raw JSON, not decoded into this binary's
// VolumeConfig, so unrecognized keys also survive a version-skewed rolling upgrade.
//
// A no-op if existingRaw carries none of the node-owned keys (no node has ever written this
// volume).
func PreserveNodeConfigFields(existingRaw []byte, target *runtime.RawExtension) error {
	if len(existingRaw) == 0 {
		return nil
	}

	existing := map[string]json.RawMessage{}
	if err := json.Unmarshal(existingRaw, &existing); err != nil {
		return fmt.Errorf("could not parse existing volume config: %w", err)
	}

	var toPreserve []string
	for _, key := range nodeConfigKeys {
		if _, ok := existing[key]; ok {
			toPreserve = append(toPreserve, key)
		}
	}
	if len(toPreserve) == 0 {
		return nil
	}

	targetMap := map[string]json.RawMessage{}
	if len(target.Raw) > 0 {
		if err := json.Unmarshal(target.Raw, &targetMap); err != nil {
			return fmt.Errorf("could not parse target volume config: %w", err)
		}
	}

	for _, key := range toPreserve {
		targetMap[key] = existing[key]
	}

	merged, err := json.Marshal(targetMap)
	if err != nil {
		return fmt.Errorf("could not re-marshal target volume config: %w", err)
	}
	target.Raw = merged
	return nil
}

// currentStringSlice reads a []string out of a raw config map, treating an absent or JSON-null key
// as an empty slice rather than nil, so callers comparing "current" against a caller-supplied
// value never need to special-case emptiness themselves.
func currentStringSlice(existing map[string]json.RawMessage, key string) ([]string, error) {
	raw, ok := existing[key]
	if !ok || string(raw) == "null" {
		return []string{}, nil
	}
	var value []string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("could not parse existing %s: %w", key, err)
	}
	if value == nil {
		value = []string{}
	}
	return value, nil
}
