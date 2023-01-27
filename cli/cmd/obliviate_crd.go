// Copyright 2021 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	. "github.com/netapp/trident/logging"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
)

var (
	configPath string

	// k8sClient is our clientset
	k8sClient k8sclient.KubernetesClient

	// crdClientset is a clientset for our own API group
	crdClientset crdclient.Interface
)

// An empty namespace tells the crdClientset to list resources across all namespaces
const allNamespaces string = ""

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

			if err := confirmObliviate(crdConfirmation); err != nil {
				return err
			}

			return obliviateCRDs()
		}
	},
}

func ObliviateCRDs(
	kubeClientVal k8sclient.KubernetesClient, crdClientsetVal crdclient.Interface, timeout time.Duration,
) error {
	k8sClient = kubeClientVal
	crdClientset = crdClientsetVal
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

	Log().Infof("Reset Trident's CRD state.")

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

	if err := deleteVolumePublications(); err != nil {
		return err
	}

	if err := deleteVolumeReferences(); err != nil {
		return err
	}

	return nil
}

func deleteVersions() error {
	crd := "tridentversions.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	versions, err := crdClientset.TridentV1().TridentVersions(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(versions.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, version := range versions.Items {
		if version.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVersions(version.Namespace).Delete(ctx(), version.Name, deleteOpts)
		}
	}

	versions, err = crdClientset.TridentV1().TridentVersions(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, version := range versions.Items {
		if version.HasTridentFinalizers() {
			crCopy := version.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVersions(version.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVersions(version.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), version.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBackends() error {
	crd := "tridentbackends.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	backends, err := crdClientset.TridentV1().TridentBackends(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(backends.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, backend := range backends.Items {
		if backend.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentBackends(backend.Namespace).Delete(ctx(), backend.Name, deleteOpts)
		}
	}

	backends, err = crdClientset.TridentV1().TridentBackends(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, backend := range backends.Items {
		if backend.HasTridentFinalizers() {
			crCopy := backend.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentBackends(backend.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentBackends(backend.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), backend.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentMirrorRelationships() error {
	crd := "tridentmirrorrelationships.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	relationships, err := crdClientset.TridentV1().TridentMirrorRelationships(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(relationships.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, relationship := range relationships.Items {
		if relationship.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentMirrorRelationships(relationship.Namespace).Delete(ctx(),
				relationship.Name, deleteOpts)
		}
	}

	relationships, err = crdClientset.TridentV1().TridentMirrorRelationships(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, relationship := range relationships.Items {
		if relationship.HasTridentFinalizers() {
			crCopy := relationship.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentMirrorRelationships(relationship.Namespace).Update(ctx(), crCopy,
				updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentMirrorRelationships(relationship.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), relationship.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentSnapshotInfos() error {
	crd := "tridentsnapshotinfos.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	snapshotInfos, err := crdClientset.TridentV1().TridentSnapshotInfos(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(snapshotInfos.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, snapshotInfo := range snapshotInfos.Items {
		if snapshotInfo.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentSnapshotInfos(snapshotInfo.Namespace).Delete(ctx(), snapshotInfo.Name,
				deleteOpts)
		}
	}

	snapshotInfos, err = crdClientset.TridentV1().TridentSnapshotInfos(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, snapshotInfo := range snapshotInfos.Items {
		if snapshotInfo.HasTridentFinalizers() {
			crCopy := snapshotInfo.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentSnapshotInfos(snapshotInfo.Namespace).Update(ctx(), crCopy,
				updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentSnapshotInfos(snapshotInfo.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), snapshotInfo.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBackendConfigs() error {
	crd := "tridentbackendconfigs.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	backendConfigs, err := crdClientset.TridentV1().TridentBackendConfigs(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(backendConfigs.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, backendConfig := range backendConfigs.Items {
		if backendConfig.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentBackendConfigs(backendConfig.Namespace).Delete(ctx(),
				backendConfig.Name, deleteOpts)
		}
	}

	backendConfigs, err = crdClientset.TridentV1().TridentBackendConfigs(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, backendConfig := range backendConfigs.Items {
		if backendConfig.HasTridentFinalizers() {
			crCopy := backendConfig.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentBackendConfigs(backendConfig.Namespace).Update(ctx(), crCopy,
				updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentBackendConfigs(backendConfig.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), backendConfig.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteStorageClasses() error {
	crd := "tridentstorageclasses.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	storageclasses, err := crdClientset.TridentV1().TridentStorageClasses(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(storageclasses.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, sc := range storageclasses.Items {
		if sc.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentStorageClasses(sc.Namespace).Delete(ctx(), sc.Name, deleteOpts)
		}
	}

	storageclasses, err = crdClientset.TridentV1().TridentStorageClasses(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, sc := range storageclasses.Items {
		if sc.HasTridentFinalizers() {
			crCopy := sc.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentStorageClasses(sc.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentStorageClasses(sc.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), sc.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteVolumes() error {
	crd := "tridentvolumes.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	volumes, err := crdClientset.TridentV1().TridentVolumes(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(volumes.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, volume := range volumes.Items {
		if volume.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVolumes(volume.Namespace).Delete(ctx(), volume.Name, deleteOpts)
		}
	}

	volumes, err = crdClientset.TridentV1().TridentVolumes(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, volume := range volumes.Items {
		if volume.HasTridentFinalizers() {
			crCopy := volume.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVolumes(volume.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVolumes(volume.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), volume.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteNodes() error {
	crd := "tridentnodes.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	nodes, err := crdClientset.TridentV1().TridentNodes(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(nodes.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, node := range nodes.Items {
		if node.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentNodes(node.Namespace).Delete(ctx(), node.Name, deleteOpts)
		}
	}

	nodes, err = crdClientset.TridentV1().TridentNodes(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		if node.HasTridentFinalizers() {
			crCopy := node.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentNodes(node.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentNodes(node.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), node.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteVolumePublications() error {
	crd := "tridentvolumepublications.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	publications, err := crdClientset.TridentV1().TridentVolumePublications(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(publications.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, publication := range publications.Items {
		if publication.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVolumePublications(publication.Namespace).Delete(ctx(),
				publication.Name, deleteOpts)
		}
	}

	publications, err = crdClientset.TridentV1().TridentVolumePublications(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, publication := range publications.Items {
		if publication.HasTridentFinalizers() {
			crCopy := publication.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVolumePublications(publication.Namespace).Update(ctx(), crCopy,
				updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVolumePublications(publication.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), publication.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTransactions() error {
	crd := "tridenttransactions.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	transactions, err := crdClientset.TridentV1().TridentTransactions(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(transactions.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, txn := range transactions.Items {
		if txn.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentTransactions(txn.Namespace).Delete(ctx(), txn.Name, deleteOpts)
		}
	}

	transactions, err = crdClientset.TridentV1().TridentTransactions(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, txn := range transactions.Items {
		if txn.HasTridentFinalizers() {
			crCopy := txn.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentTransactions(txn.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentTransactions(txn.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), txn.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteSnapshots() error {
	crd := "tridentsnapshots.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	snapshots, err := crdClientset.TridentV1().TridentSnapshots(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(snapshots.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, snapshot := range snapshots.Items {
		if snapshot.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentSnapshots(snapshot.Namespace).Delete(ctx(), snapshot.Name, deleteOpts)
		}
	}

	snapshots, err = crdClientset.TridentV1().TridentSnapshots(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, snapshot := range snapshots.Items {
		if snapshot.HasTridentFinalizers() {
			crCopy := snapshot.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentSnapshots(snapshot.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentSnapshots(snapshot.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), snapshot.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteVolumeReferences() error {
	crd := "tridentvolumereferences.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	vrefs, err := crdClientset.TridentV1().TridentVolumeReferences(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(vrefs.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, vref := range vrefs.Items {
		if vref.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVolumeReferences(vref.Namespace).Delete(ctx(), vref.Name, deleteOpts)
		}
	}

	vrefs, err = crdClientset.TridentV1().TridentVolumeReferences(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, vref := range vrefs.Items {
		if vref.HasTridentFinalizers() {
			crCopy := vref.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVolumeReferences(vref.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVolumeReferences(vref.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), vref.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
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
		"tridentvolumepublications.trident.netapp.io",
		"tridentvolumereferences.trident.netapp.io",
	}

	for _, crdName := range crdNames {

		logFields := LogFields{"CRD": crdName}

		// See if CRD exists
		exists, err := k8sClient.CheckCRDExists(crdName)
		if err != nil {
			return err
		}
		if !exists {
			Log().WithFields(logFields).Info("CRD not present.")
			continue
		}

		// Get the CRD and check for finalizers
		crd, err := k8sClient.GetCRD(crdName)
		if isNotFoundError(err) {
			Log().WithFields(logFields).Info("CRD not found.")
			continue
		}

		// Remove finalizers if present
		if len(crd.Finalizers) > 0 {
			if err := k8sClient.RemoveFinalizerFromCRD(crdName); err != nil {
				Log().WithFields(logFields).Errorf("Could not remove finalizer from CRD; %v", err)
				return err
			} else {
				Log().WithFields(logFields).Debug("Removed finalizers from CRD.")
			}
		} else {
			Log().WithFields(logFields).Debug("No finalizers found on CRD.")
		}

		// Try deleting CRD
		if crd.DeletionTimestamp.IsZero() {
			Log().WithFields(logFields).Debug("Deleting CRD.")

			err := k8sClient.DeleteCRD(crdName)
			if isNotFoundError(err) {
				Log().WithFields(logFields).Info("CRD not found during deletion.")
				continue
			} else if err != nil {
				Log().WithFields(logFields).Errorf("Could not delete CRD; %v", err)
				return err
			}
		} else {
			Log().WithFields(logFields).Debug("CRD already has deletion timestamp.")
		}

		// Give the CRD some time to disappear.  We removed any finalizers, so this should always work.
		if err := waitForCRDDeletion(crdName, k8sTimeout); err != nil {
			Log().WithFields(logFields).Error(err)
			return err
		}

		Log().WithFields(logFields).Info("CRD deleted.")
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
		Log().WithFields(LogFields{
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

	Log().WithField("name", name).Trace("Waiting for object to be deleted.")

	if err := backoff.RetryNotify(doDelete, deleteBackoff, doDeleteNotify); err != nil {
		return fmt.Errorf("object %s was not deleted after %3.2f seconds", name, timeout.Seconds())
	}

	Log().WithFields(LogFields{
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
		Log().WithFields(LogFields{
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

	Log().WithField("CRD", name).Trace("Waiting for CRD to be deleted.")

	if err := backoff.RetryNotify(checkDeleted, deleteBackoff, checkDeletedNotify); err != nil {
		return fmt.Errorf("CRD %s was not deleted after %3.2f seconds", name, timeout.Seconds())
	}

	Log().WithFields(LogFields{
		"CRD":         name,
		"retries":     retries,
		"waitSeconds": fmt.Sprintf("%3.2f", deleteBackoff.GetElapsedTime().Seconds()),
	}).Debugf("CRD deleted.")

	return nil
}
