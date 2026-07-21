// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
)

// handleTridentNode reconciles node-owned TridentNode spec into orchestrator state and then
// writes controller-owned acknowledgment/status fields.
func (c *TridentCrdController) handleTridentNode(keyItem *KeyItem) error {
	ctx := keyItem.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(keyItem.key)
	if err != nil {
		return err
	}

	nodeCR, err := c.nodesLister.TridentNodes(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	var reconciler controllerhelpers.TridentNodeReconciler
	// This controller runs only when the Trident CRD controller frontend is started (Kubernetes
	// mode in main). PlainCSIHelper is a fallback for odd test/misconfiguration cases; real plain-CSI
	// deployments do not run this reconcile path.
	csiFrontend, err := c.orchestrator.GetFrontend(ctx, controllerhelpers.KubernetesHelper)
	if err != nil {
		csiFrontend, err = c.orchestrator.GetFrontend(ctx, controllerhelpers.PlainCSIHelper)
	}
	if err != nil {
		return err
	}
	if r, ok := csiFrontend.(controllerhelpers.TridentNodeReconciler); ok {
		reconciler = r
	}
	if reconciler == nil {
		return fmt.Errorf("frontend %T does not implement TridentNodeReconciler", csiFrontend)
	}
	return reconciler.ReconcileTridentNode(ctx, nodeCR)
}
