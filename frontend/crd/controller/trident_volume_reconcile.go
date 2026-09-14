// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"encoding/json"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/utils/errors"
)

// updateTridentVolumeHandler filters TridentVolume update events down to genuine changes in the
// node-owned config (LUKSPassphraseNames, written directly to the CR by a node), so a resize,
// autogrow status update, or any other controller-driven write does not enqueue work. This cannot
// reuse the generic updateCRHandler's metadata.generation filter: TridentVolume's CRD has no
// status subresource, so apiextensions bumps metadata.generation on every non-metadata change, not
// just this one - using it here would enqueue work on the hottest CR in the system for every
// resize/autogrow update too.
func (c *TridentCrdController) updateTridentVolumeHandler(old, new interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	oldVol, ok := old.(*tridentv1.TridentVolume)
	if !ok {
		Logx(ctx).Errorf("Incorrect type (%T) provided to updateTridentVolumeHandler, cannot process old CR", old)
		return
	}
	newVol, ok := new.(*tridentv1.TridentVolume)
	if !ok {
		Logx(ctx).Errorf("Incorrect type (%T) provided to updateTridentVolumeHandler, cannot process new CR", new)
		return
	}

	oldNames, err := configLUKSPassphraseNames(oldVol.Config.Raw)
	if err != nil {
		Logx(ctx).WithError(err).Error("Could not parse old TridentVolume config; skipping.")
		return
	}
	newNames, err := configLUKSPassphraseNames(newVol.Config.Raw)
	if err != nil {
		Logx(ctx).WithError(err).Error("Could not parse new TridentVolume config; skipping.")
		return
	}
	// Compared as an unordered set, matching how every other consumer treats these names: a
	// reordering of the same names is not a change and must not enqueue work.
	if collection.EqualValues(oldNames, newNames) {
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(new)
	if err != nil {
		Logx(ctx).Error(err)
		return
	}
	c.addEventToWorkqueue(key, EventUpdate, ctx, ObjectTypeTridentVolume)
}

// configLUKSPassphraseNames reads just the LUKSPassphraseNames key out of a TridentVolume's raw
// config, without decoding the whole thing into storage.VolumeConfig - the filter above only ever
// needs this one field, and decoding the rest would be wasted work on every TridentVolume update
// the cluster produces.
func configLUKSPassphraseNames(configRaw []byte) ([]string, error) {
	if len(configRaw) == 0 {
		return nil, nil
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(configRaw, &config); err != nil {
		return nil, err
	}
	raw, ok := config[tridentv1.ConfigKeyLUKSPassphraseNames]
	if !ok {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, err
	}
	return names, nil
}

// handleTridentVolume pushes node-owned TridentVolume config fields into the orchestrator's
// in-memory cache. The node has already durably persisted them directly to the CR, so this does
// not write the persistent store - it only makes the cache stop being stale.
func (c *TridentCrdController) handleTridentVolume(keyItem *KeyItem) error {
	ctx := keyItem.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(keyItem.key)
	if err != nil {
		return err
	}

	volumeCR, err := c.volumesLister.TridentVolumes(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	persistent, err := volumeCR.Persistent()
	if err != nil {
		return err
	}

	if err := c.orchestrator.RefreshVolumeNodeConfig(
		ctx, persistent.Config.Name, persistent.Config.LUKSPassphraseNames,
	); err != nil {
		if errors.IsNotFoundError(err) {
			// The controller hasn't heard of this volume (yet). Bootstrap will pick it up from
			// the store; nothing to reconcile here.
			return nil
		}
		return err
	}
	return nil
}
