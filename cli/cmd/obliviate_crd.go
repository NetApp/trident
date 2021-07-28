// Copyright 2021 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
)

var (
	configPath string

	// k8sClient is our clientset
	k8sClient k8sclient.Interface

	// crdClientset is a clientset for our own API group
	crdClientset crdclient.Interface

	resetNamespace string
)

func init() {
	obliviateCmd.AddCommand(obliviateCRDCmd)
	obliviateCRDCmd.Flags().StringVar(&configPath, "k8s-config-path", kubeConfigPath(), "Path to KubeConfig file.")
}

var obliviateCRDCmd = &cobra.Command{
	Use:              "crd",
	Short:            "Reset Trident's CRD state (deletes all custom resources and CRDs)",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {

		var err error
		initLogging()

		if OperatingMode == ModeTunnel {
			if !forceObliviate {
				if forceObliviate, err = getUserConfirmation(crdConfirmation, cmd); err != nil {
					return err
				} else if !forceObliviate {
					return errors.New("obliviation canceled")
				}
			}
			command := []string{"obliviate", "crd", fmt.Sprintf("--%s", forceConfirmation)}
			TunnelCommand(command)
			return nil
		} else {
			if err := initClients(); err != nil {
				return err
			}
			return obliviateCRDs()
		}
	},
}

func ObliviateCRDs(
	kubeClientVal k8sclient.Interface, crdClientsetVal crdclient.Interface, namespace string, timeout time.Duration,
) error {

	k8sClient = kubeClientVal
	crdClientset = crdClientsetVal
	resetNamespace = namespace
	k8sTimeout = timeout

	return obliviateCRDs()
}

func obliviateCRDs() error {

	// Delete all instances of custom resources
	if err := deleteCRs(); err != nil {
		return err
	}

	// Delete all custom resource definitions
	if err := deleteCRDs(); err != nil {
		return err
	}

	log.Infof("Reset Trident's CRD state.")

	return nil
}

func deleteCRs() error {

	if err := deleteVersions(); err != nil {
		return err
	}

	if err := deleteTridentMirrorRelationships(); err != nil {
		return err
	}

	if err := deleteTridentSnapshotInfos(); err != nil {
		return err
	}

	// deleting backend config before backends is desirable, do not want backend deletion without
	// the backendconfig deletion to trigger another backend creation
	if err := deleteBackendConfigs(); err != nil {
		return err
	}

	if err := deleteBackends(); err != nil {
		return err
	}

	if err := deleteStorageClasses(); err != nil {
		return err
	}

	if err := deleteVolumes(); err != nil {
		return err
	}

	if err := deleteNodes(); err != nil {
		return err
	}

	if err := deleteTransactions(); err != nil {
		return err
	}

	if err := deleteSnapshots(); err != nil {
		return err
	}

	return nil
}

func deleteVersions() error {

	crd := "tridentversions.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	versions, err := crdClientset.TridentV1().TridentVersions(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(versions.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, version := range versions.Items {
		if version.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVersions(resetNamespace).Delete(ctx(), version.Name, deleteOpts)
		}
	}

	versions, err = crdClientset.TridentV1().TridentVersions(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, version := range versions.Items {
		if version.HasTridentFinalizers() {
			crCopy := version.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVersions(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVersions(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), version.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBackends() error {

	crd := "tridentbackends.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	backends, err := crdClientset.TridentV1().TridentBackends(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(backends.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, backend := range backends.Items {
		if backend.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentBackends(resetNamespace).Delete(ctx(), backend.Name, deleteOpts)
		}
	}

	backends, err = crdClientset.TridentV1().TridentBackends(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, backend := range backends.Items {
		if backend.HasTridentFinalizers() {
			crCopy := backend.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentBackends(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentBackends(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), backend.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentMirrorRelationships() error {

	crd := "tridentmirrorrelationships.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	relationships, err := crdClientset.TridentV1().TridentMirrorRelationships(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(relationships.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, relationship := range relationships.Items {
		if relationship.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentMirrorRelationships(resetNamespace).Delete(ctx(), relationship.Name, deleteOpts)
		}
	}

	relationships, err = crdClientset.TridentV1().TridentMirrorRelationships(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, relationship := range relationships.Items {
		if relationship.HasTridentFinalizers() {
			crCopy := relationship.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentMirrorRelationships(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentMirrorRelationships(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), relationship.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentSnapshotInfos() error {

	crd := "tridentsnapshotinfos.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	snapshotInfos, err := crdClientset.TridentV1().TridentSnapshotInfos(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(snapshotInfos.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, snapshotInfo := range snapshotInfos.Items {
		if snapshotInfo.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentSnapshotInfos(resetNamespace).Delete(ctx(), snapshotInfo.Name, deleteOpts)
		}
	}

	snapshotInfos, err = crdClientset.TridentV1().TridentSnapshotInfos(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, snapshotInfo := range snapshotInfos.Items {
		if snapshotInfo.HasTridentFinalizers() {
			crCopy := snapshotInfo.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentSnapshotInfos(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentSnapshotInfos(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), snapshotInfo.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBackendConfigs() error {

	crd := "tridentbackendconfigs.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	backendConfigs, err := crdClientset.TridentV1().TridentBackendConfigs(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(backendConfigs.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, backendConfig := range backendConfigs.Items {
		if backendConfig.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentBackendConfigs(resetNamespace).Delete(ctx(), backendConfig.Name,
				deleteOpts)
		}
	}

	backendConfigs, err = crdClientset.TridentV1().TridentBackendConfigs(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, backendConfig := range backendConfigs.Items {
		if backendConfig.HasTridentFinalizers() {
			crCopy := backendConfig.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentBackendConfigs(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentBackendConfigs(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), backendConfig.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteStorageClasses() error {

	crd := "tridentstorageclasses.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	storageclasses, err := crdClientset.TridentV1().TridentStorageClasses(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(storageclasses.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, sc := range storageclasses.Items {
		if sc.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentStorageClasses(resetNamespace).Delete(ctx(), sc.Name, deleteOpts)
		}
	}

	storageclasses, err = crdClientset.TridentV1().TridentStorageClasses(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, sc := range storageclasses.Items {
		if sc.HasTridentFinalizers() {
			crCopy := sc.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentStorageClasses(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentStorageClasses(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), sc.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteVolumes() error {

	crd := "tridentvolumes.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	volumes, err := crdClientset.TridentV1().TridentVolumes(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(volumes.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, volume := range volumes.Items {
		if volume.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVolumes(resetNamespace).Delete(ctx(), volume.Name, deleteOpts)
		}
	}

	volumes, err = crdClientset.TridentV1().TridentVolumes(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, volume := range volumes.Items {
		if volume.HasTridentFinalizers() {
			crCopy := volume.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVolumes(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVolumes(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), volume.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteNodes() error {

	crd := "tridentnodes.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	nodes, err := crdClientset.TridentV1().TridentNodes(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(nodes.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, node := range nodes.Items {
		if node.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentNodes(resetNamespace).Delete(ctx(), node.Name, deleteOpts)
		}
	}

	nodes, err = crdClientset.TridentV1().TridentNodes(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		if node.HasTridentFinalizers() {
			crCopy := node.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentNodes(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentNodes(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), node.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTransactions() error {

	crd := "tridenttransactions.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	transactions, err := crdClientset.TridentV1().TridentTransactions(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(transactions.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, txn := range transactions.Items {
		if txn.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentTransactions(resetNamespace).Delete(ctx(), txn.Name, deleteOpts)
		}
	}

	transactions, err = crdClientset.TridentV1().TridentTransactions(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, txn := range transactions.Items {
		if txn.HasTridentFinalizers() {
			crCopy := txn.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentTransactions(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentTransactions(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), txn.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteSnapshots() error {

	crd := "tridentsnapshots.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	snapshots, err := crdClientset.TridentV1().TridentSnapshots(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(snapshots.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, snapshot := range snapshots.Items {
		if snapshot.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentSnapshots(resetNamespace).Delete(ctx(), snapshot.Name, deleteOpts)
		}
	}

	snapshots, err = crdClientset.TridentV1().TridentSnapshots(resetNamespace).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, snapshot := range snapshots.Items {
		if snapshot.HasTridentFinalizers() {
			crCopy := snapshot.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentSnapshots(resetNamespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentSnapshots(resetNamespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), snapshot.Name, nil); err != nil {
			log.Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteCRDs() error {

	crdNames := []string{
		"tridentversions.trident.netapp.io",
		"tridentbackendconfigs.trident.netapp.io",
		"tridentbackends.trident.netapp.io",
		"tridentstorageclasses.trident.netapp.io",
		"tridentmirrorrelationships.trident.netapp.io",
		"tridentsnapshotinfos.trident.netapp.io",
		"tridentvolumes.trident.netapp.io",
		"tridentnodes.trident.netapp.io",
		"tridenttransactions.trident.netapp.io",
		"tridentsnapshots.trident.netapp.io",
	}

	for _, crdName := range crdNames {

		logFields := log.Fields{"CRD": crdName}

		// See if CRD exists
		exists, err := k8sClient.CheckCRDExists(crdName)
		if err != nil {
			return err
		}
		if !exists {
			log.WithFields(logFields).Info("CRD not present.")
			continue
		}

		// Get the CRD and check for finalizers
		crd, err := k8sClient.GetCRD(crdName)
		if isNotFoundError(err) {
			log.WithFields(logFields).Info("CRD not found.")
			continue
		}

		// Remove finalizers if present
		if len(crd.Finalizers) > 0 {
			if err := k8sClient.RemoveFinalizerFromCRD(crdName); err != nil {
				log.WithFields(logFields).Errorf("Could not remove finalizer from CRD; %v", err)
				return err
			} else {
				log.WithFields(logFields).Debug("Removed finalizers from CRD.")
			}
		} else {
			log.WithFields(logFields).Debug("No finalizers found on CRD.")
		}

		// Try deleting CRD
		if crd.DeletionTimestamp.IsZero() {
			log.WithFields(logFields).Debug("Deleting CRD.")

			err := k8sClient.DeleteCRD(crdName)
			if isNotFoundError(err) {
				log.WithFields(logFields).Info("CRD not found during deletion.")
				continue
			} else if err != nil {
				log.WithFields(logFields).Errorf("Could not delete CRD; %v", err)
				return err
			}
		} else {
			log.WithFields(logFields).Debug("CRD already has deletion timestamp.")
		}

		// Give the CRD some time to disappear.  We removed any finalizers, so this should always work.
		if err := waitForCRDDeletion(crdName, k8sTimeout); err != nil {
			log.WithFields(logFields).Error(err)
			return err
		}

		log.WithFields(logFields).Info("CRD deleted.")
		continue
	}

	return nil
}

func isNotFoundError(err error) bool {
	if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
		return true
	}
	return false
}

// crDeleter deletes a custom resource
type crDeleter func(context.Context, string, metav1.DeleteOptions) error

func deleteWithRetry(deleteFunc crDeleter, c context.Context, name string, deleteOptions *metav1.DeleteOptions) error {

	if deleteOptions == nil {
		deleteOptions = &deleteOpts
	}

	timeout := 10 * time.Second
	retries := 0

	doDelete := func() error {

		err := deleteFunc(c, name, *deleteOptions)
		if err == nil || isNotFoundError(err) {
			return nil
		}

		return fmt.Errorf("object %s not yet deleted", name)
	}

	doDeleteNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"name": name,
			"err":  err,
		}).Debug("Object not yet deleted, waiting.")

		retries++
	}

	deleteBackoff := backoff.NewExponentialBackOff()
	deleteBackoff.InitialInterval = 1 * time.Second
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.Multiplier = 1.414
	deleteBackoff.MaxInterval = 5 * time.Second
	deleteBackoff.MaxElapsedTime = timeout

	log.WithField("name", name).Trace("Waiting for object to be deleted.")

	if err := backoff.RetryNotify(doDelete, deleteBackoff, doDeleteNotify); err != nil {
		return fmt.Errorf("object %s was not deleted after %3.2f seconds", name, timeout.Seconds())
	}

	log.WithFields(log.Fields{
		"name":        name,
		"retries":     retries,
		"waitSeconds": fmt.Sprintf("%3.2f", deleteBackoff.GetElapsedTime().Seconds()),
	}).Debugf("Object deleted.")

	return nil
}

func waitForCRDDeletion(name string, timeout time.Duration) error {

	retries := 0

	checkDeleted := func() error {

		exists, err := k8sClient.CheckCRDExists(name)
		if !exists || isNotFoundError(err) {
			return nil
		}

		return fmt.Errorf("CRD %s not yet deleted", name)
	}

	checkDeletedNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"CRD": name,
			"err": err,
		}).Debug("CRD not yet deleted, waiting.")

		retries++
	}

	deleteBackoff := backoff.NewExponentialBackOff()
	deleteBackoff.InitialInterval = 1 * time.Second
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.Multiplier = 1.414
	deleteBackoff.MaxInterval = 5 * time.Second
	deleteBackoff.MaxElapsedTime = timeout

	log.WithField("CRD", name).Trace("Waiting for CRD to be deleted.")

	if err := backoff.RetryNotify(checkDeleted, deleteBackoff, checkDeletedNotify); err != nil {
		return fmt.Errorf("CRD %s was not deleted after %3.2f seconds", name, timeout.Seconds())
	}

	log.WithFields(log.Fields{
		"CRD":         name,
		"retries":     retries,
		"waitSeconds": fmt.Sprintf("%3.2f", deleteBackoff.GetElapsedTime().Seconds()),
	}).Debugf("CRD deleted.")

	return nil
}
