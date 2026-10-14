// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	cosicrd "github.com/netapp/trident/cosi/crd"
	. "github.com/netapp/trident/logging"
	operatorCrdClient "github.com/netapp/trident/operator/crd/client/clientset/versioned"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils/errors"
)

var (
	configPath string

	// k8sClient is our clientset
	k8sClient k8sclient.KubernetesClient

	// crdClientset is a clientset for our own API group
	crdClientset crdclient.Interface

	// operatorCrdClient is a clientset for our own API group
	operatorCrdClientSet operatorCrdClient.Interface

	// dynamicClient reaches custom resources Trident has no typed clientset for, currently the
	// COSI ones.
	dynamicClient dynamic.Interface

	skipCRDs []string
)

// An empty namespace tells the crdClientset to list resources across all namespaces
const allNamespaces string = ""

func init() {
	obliviateCmd.AddCommand(obliviateCRDCmd)
	obliviateCRDCmd.Flags().StringVar(&configPath, "k8s-config-path", kubeConfigPath(), "Path to KubeConfig file.")
	obliviateCRDCmd.Flags().StringSliceVar(&skipCRDs, "skip-crds", []string{}, "List of CRDs to skip")
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
			if len(skipCRDs) > 0 {
				command = append(command, fmt.Sprintf("--skip-crds=%s", strings.Join(skipCRDs, ",")))
			}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			if err := initClientsFunc(); err != nil {
				return err
			}

			if err := confirmObliviate(crdConfirmation); err != nil {
				return err
			}

			return obliviateCRDs(skipCRDs)
		}
	},
}

func ObliviateCRDs(
	kubeClientVal k8sclient.KubernetesClient, crdClientsetVal crdclient.Interface, operatorCrdClientSetVal operatorCrdClient.Interface,
	dynamicClientVal dynamic.Interface, timeout time.Duration, skipCRDs []string,
) error {
	k8sClient = kubeClientVal
	crdClientset = crdClientsetVal
	operatorCrdClientSet = operatorCrdClientSetVal
	dynamicClient = dynamicClientVal
	k8sTimeout = timeout

	return obliviateCRDs(skipCRDs)
}

func obliviateCRDs(skipCRDs []string) error {
	crdNames := []string{
		"tridentversions.trident.netapp.io",
		"tridentbackendconfigs.trident.netapp.io",
		"tridentbackends.trident.netapp.io",
		"tridentbucketaccesses.trident.netapp.io",
		"tridentbuckets.trident.netapp.io",
		"tridentobjectstores.trident.netapp.io",
		"tridentstorageclasses.trident.netapp.io",
		"tridentmirrorrelationships.trident.netapp.io",
		"tridentactionmirrorupdates.trident.netapp.io",
		"tridentsnapshotinfos.trident.netapp.io",
		"tridentvolumes.trident.netapp.io",
		"tridentnodes.trident.netapp.io",
		"tridenttransactions.trident.netapp.io",
		"tridentsnapshots.trident.netapp.io",
		"tridentgroupsnapshots.trident.netapp.io",
		"tridentvolumepublications.trident.netapp.io",
		"tridentvolumereferences.trident.netapp.io",
		"tridentactionsnapshotrestores.trident.netapp.io",
		"tridentvolumemoves.trident.netapp.io",
		"tridentnoderemediations.trident.netapp.io",
		"tridentnoderemediationtemplates.trident.netapp.io",
		"tridentconfigurators.trident.netapp.io",
		"tridentorchestrators.trident.netapp.io",
		"tridentautogrowpolicies.trident.netapp.io",
		"tridentautogrowrequestinternals.trident.netapp.io",
	}

	// COSI CRDs are obliviated with the rest, but only the ones Trident installed. The filter
	// has to happen here rather than inside the delete phases, because deleting a CRD takes
	// every object under it along with it.
	cosiCRDNames, err := tridentManagedCOSICRDNames()
	if err != nil {
		return err
	}
	crdNames = append(crdNames, cosiCRDNames...)

	skipCRDMap := make(map[string]bool)
	for _, crd := range skipCRDs {
		skipCRDMap[crd] = true
	}

	var filteredCRDs []string
	for _, crd := range crdNames {
		if !skipCRDMap[crd] {
			filteredCRDs = append(filteredCRDs, crd)
		}
	}

	// Delete all instances of custom resources
	if err := deleteCRs(filteredCRDs); err != nil {
		return err
	}

	// Delete all custom resource definitions
	if err := deleteCRDs(filteredCRDs); err != nil {
		return err
	}

	Log().Infof("Reset Trident's CRD state.")

	return nil
}

// tridentManagedCOSICRDNames returns the COSI CRDs obliviate is allowed to remove: the ones
// Trident installed, plus any that are absent, which both delete phases treat as a no-op.
//
// A COSI CRD that exists without Trident's managed-by label belongs to another provider. Deleting
// it would take that provider's objects with it, and there is no way back: a CRD deletionTimestamp
// cannot be revoked, so even a CRD wedged on someone else's finalizer stays doomed.
func tridentManagedCOSICRDNames() ([]string, error) {
	cosiCRDNames := k8sclient.COSICRDNames()
	names := make([]string, 0, len(cosiCRDNames))

	for _, crdName := range cosiCRDNames {
		crd, err := k8sClient.GetCRD(crdName)
		if err != nil {
			if isNotFoundError(err) {
				names = append(names, crdName)
				continue
			}
			return nil, fmt.Errorf("could not retrieve COSI CRD %s; %v", crdName, err)
		}

		if !cosicrd.IsTridentManaged(crd.Labels) {
			Log().WithField("CRD", crdName).Warning(
				"COSI CRD was not installed by Trident; leaving it and its custom resources alone.")
			continue
		}

		names = append(names, crdName)
	}

	return names, nil
}

func deleteCRs(filteredCRDs []string) error {
	for _, crd := range filteredCRDs {
		switch crd {
		case "tridentversions.trident.netapp.io":
			if err := deleteVersions(); err != nil {
				return err
			}
		case "tridentbackendconfigs.trident.netapp.io":
			if err := deleteBackendConfigs(); err != nil {
				return err
			}
		case "tridentbackends.trident.netapp.io":
			if err := deleteBackends(); err != nil {
				return err
			}
		case "tridentobjectstores.trident.netapp.io":
			if err := deleteObjectStores(); err != nil {
				return err
			}
		case "tridentbuckets.trident.netapp.io":
			if err := deleteBuckets(); err != nil {
				return err
			}
		case "tridentbucketaccesses.trident.netapp.io":
			if err := deleteBucketAccesses(); err != nil {
				return err
			}
		case "tridentstorageclasses.trident.netapp.io":
			if err := deleteStorageClasses(); err != nil {
				return err
			}
		case "tridentmirrorrelationships.trident.netapp.io":
			if err := deleteTridentMirrorRelationships(); err != nil {
				return err
			}
		case "tridentactionmirrorupdates.trident.netapp.io":
			if err := deleteTridentActionMirrorUpdates(); err != nil {
				return err
			}
		case "tridentsnapshotinfos.trident.netapp.io":
			if err := deleteTridentSnapshotInfos(); err != nil {
				return err
			}
		case "tridentvolumes.trident.netapp.io":
			if err := deleteVolumes(); err != nil {
				return err
			}
		case "tridentnodes.trident.netapp.io":
			if err := deleteNodes(); err != nil {
				return err
			}
		case "tridenttransactions.trident.netapp.io":
			if err := deleteTransactions(); err != nil {
				return err
			}
		case "tridentsnapshots.trident.netapp.io":
			if err := deleteSnapshots(); err != nil {
				return err
			}
		case "tridentgroupsnapshots.trident.netapp.io":
			if err := deleteGroupSnapshots(); err != nil {
				return err
			}
		case "tridentvolumepublications.trident.netapp.io":
			if err := deleteVolumePublications(); err != nil {
				return err
			}
		case "tridentvolumereferences.trident.netapp.io":
			if err := deleteVolumeReferences(); err != nil {
				return err
			}
		case "tridentactionsnapshotrestores.trident.netapp.io":
			if err := deleteActionSnapshotRestores(); err != nil {
				return err
			}
		case "tridentvolumemoves.trident.netapp.io":
			if err := deleteVolumeMoves(); err != nil {
				return err
			}
		case "tridentnoderemediations.trident.netapp.io":
			if err := deleteNodeRemediations(); err != nil {
				return err
			}
		case "tridentnoderemediationtemplates.trident.netapp.io":
			if err := deleteNodeRemediationTemplates(); err != nil {
				return err
			}
		case "tridentconfigurators.trident.netapp.io":
			if err := deleteTridentConfigurators(); err != nil {
				return err
			}
		case "tridentorchestrators.trident.netapp.io":
			if err := deleteTridentOrchestrators(); err != nil {
				return err
			}
		case "tridentautogrowpolicies.trident.netapp.io":
			if err := deleteTridentAutogrowPolicies(); err != nil {
				return err
			}
		case "tridentautogrowrequestinternals.trident.netapp.io":
			if err := deleteTridentAutogrowRequestInternals(); err != nil {
				return err
			}
		default:
			if slices.Contains(k8sclient.COSICRDNames(), crd) {
				if err := deleteCOSICRs(crd); err != nil {
					return err
				}
				continue
			}
			Log().WithField("CRD", crd).Debug("CRD not present.")
		}
	}

	return nil
}

func deleteObjectStores() error {
	crd := "tridentobjectstores.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	objectStores, err := crdClientset.TridentV1().TridentObjectStores(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(objectStores.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, objectStore := range objectStores.Items {
		if objectStore.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentObjectStores(objectStore.Namespace).Delete(
				ctx(), objectStore.Name, deleteOpts,
			)
		}
	}

	objectStores, err = crdClientset.TridentV1().TridentObjectStores(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, objectStore := range objectStores.Items {
		if objectStore.HasTridentFinalizers() {
			crCopy := objectStore.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentObjectStores(objectStore.Namespace).Update(
				ctx(), crCopy, updateOpts,
			)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentObjectStores(objectStore.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), objectStore.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBuckets() error {
	crd := "tridentbuckets.trident.netapp.io"
	logFields := LogFields{"CRD": crd}
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil || !exists {
		return err
	}
	buckets, err := crdClientset.TridentV1().TridentBuckets(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}
	for _, bucket := range buckets.Items {
		if err := crdClientset.TridentV1().TridentBuckets(bucket.Namespace).Delete(
			ctx(), bucket.Name, deleteOpts,
		); err != nil && !isNotFoundError(err) {
			return err
		}
	}
	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBucketAccesses() error {
	crd := "tridentbucketaccesses.trident.netapp.io"
	logFields := LogFields{"CRD": crd}
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil || !exists {
		return err
	}
	accesses, err := crdClientset.TridentV1().TridentBucketAccesses(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}
	for _, access := range accesses.Items {
		if err := crdClientset.TridentV1().TridentBucketAccesses(access.Namespace).Delete(
			ctx(), access.Name, deleteOpts,
		); err != nil && !isNotFoundError(err) {
			return err
		}
	}
	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

// deleteCOSICRs clears finalizers from every custom resource of one COSI CRD, so that deleting
// the CRD afterwards is not blocked.
//
// Trident's own CRs each get a typed helper above. COSI CRs get one generic function over the
// dynamic client instead, because pulling the COSI clientset into tridentctl would spread the
// COSI dependency into the CLI for the sake of a cleanup path.
//
// The finalizers matter here more than for Trident's CRDs: COSI puts a protection finalizer on
// every Bucket and BucketAccess, and obliviate runs after Trident is gone, so there is nothing
// left to remove them. Deleting the CRD without this would hang.
func deleteCOSICRs(crdName string) error {
	logFields := LogFields{"CRD": crdName}

	exists, err := k8sClient.CheckCRDExists(crdName)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	crd, err := k8sClient.GetCRD(crdName)
	if err != nil {
		return err
	}

	// Only clean up what Trident installed. An unlabelled CRD belongs to another COSI
	// provider, and obliviating Trident is no reason to delete their objects.
	if !cosicrd.IsTridentManaged(crd.Labels) {
		Log().WithFields(logFields).Warning("COSI CRD was not installed by Trident; leaving it alone.")
		return nil
	}

	gvr := schema.GroupVersionResource{
		Group:    crd.Spec.Group,
		Version:  cosicrd.Version,
		Resource: crd.Spec.Names.Plural,
	}

	list, err := dynamicClient.Resource(gvr).List(ctx(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("could not list %s; %v", crdName, err)
	}

	for i := range list.Items {
		item := list.Items[i]

		// A BucketAccess owns a credentials Secret that carries the same finalizer. The
		// Secret is not a COSI resource, so deleting the CRDs does nothing for it, and a
		// namespace holding one stays in Terminating forever. Clear it while the
		// BucketAccess still says where it is.
		if crdName == cosicrd.BucketAccessCRDName {
			if err = clearCOSISecretFinalizers(&item); err != nil {
				return err
			}
		}

		if len(item.GetFinalizers()) == 0 {
			continue
		}

		item.SetFinalizers(nil)
		if _, err = dynamicClient.Resource(gvr).Namespace(item.GetNamespace()).
			Update(ctx(), &item, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("could not remove finalizers from %s %s; %v", crdName, item.GetName(), err)
		}

		Log().WithFields(logFields).WithField("name", item.GetName()).Debug("Removed finalizers.")
	}

	Log().WithFields(logFields).Info("Cleared COSI custom resources.")

	return nil
}

// secretGVR addresses core Secrets through the dynamic client.
var secretGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}

// clearCOSISecretFinalizers removes the COSI protection finalizer from the credentials Secrets
// a BucketAccess names.
//
// The Secret names are read off the BucketAccess rather than found by listing Secrets, because
// the names are chosen by whoever wrote the BucketAccess and a cluster-wide Secret list is an
// expensive and privileged thing to do for a cleanup path.
func clearCOSISecretFinalizers(access *unstructured.Unstructured) error {
	claims, found, err := unstructured.NestedSlice(access.Object, "spec", "bucketClaims")
	if err != nil || !found {
		return nil
	}

	for _, entry := range claims {
		claim, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		secretName, _, _ := unstructured.NestedString(claim, "accessSecretName")
		if secretName == "" {
			continue
		}

		secrets := dynamicClient.Resource(secretGVR).Namespace(access.GetNamespace())

		secret, err := secrets.Get(ctx(), secretName, metav1.GetOptions{})
		if err != nil {
			if isNotFoundError(err) {
				continue
			}
			return fmt.Errorf("could not retrieve COSI credentials secret %s; %v", secretName, err)
		}

		if len(secret.GetFinalizers()) == 0 {
			continue
		}

		secret.SetFinalizers(nil)
		if _, err = secrets.Update(ctx(), secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("could not remove finalizers from COSI credentials secret %s; %v",
				secretName, err)
		}

		Log().WithField("secret", secretName).Debug("Removed finalizers from COSI credentials secret.")
	}

	return nil
}

func deleteTridentConfigurators() error {
	crd := "tridentconfigurators.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	configurators, err := operatorCrdClientSet.TridentV1().TridentConfigurators().List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(configurators.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, configurator := range configurators.Items {
		if configurator.DeletionTimestamp.IsZero() {
			_ = operatorCrdClientSet.TridentV1().TridentConfigurators().Delete(ctx(), configurator.Name, deleteOpts)
		}
	}

	configurators, err = operatorCrdClientSet.TridentV1().TridentConfigurators().List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, configurator := range configurators.Items {
		if configurator.HasTridentFinalizers() {
			crCopy := configurator.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := operatorCrdClientSet.TridentV1().TridentConfigurators().Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := operatorCrdClientSet.TridentV1().TridentConfigurators().Delete
		if err := deleteWithRetry(deleteFunc, ctx(), configurator.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentOrchestrators() error {
	crd := "tridentorchestrators.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	orchestrators, err := operatorCrdClientSet.TridentV1().TridentOrchestrators().List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(orchestrators.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, orchestrator := range orchestrators.Items {
		if orchestrator.DeletionTimestamp.IsZero() {
			_ = operatorCrdClientSet.TridentV1().TridentOrchestrators().Delete(ctx(), orchestrator.Name, deleteOpts)
		}
	}

	orchestrators, err = operatorCrdClientSet.TridentV1().TridentOrchestrators().List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, orchestrator := range orchestrators.Items {
		if orchestrator.HasTridentFinalizers() {
			crCopy := orchestrator.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := operatorCrdClientSet.TridentV1().TridentOrchestrators().Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := operatorCrdClientSet.TridentV1().TridentOrchestrators().Delete
		if err := deleteWithRetry(deleteFunc, ctx(), orchestrator.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentAutogrowPolicies() error {
	crd := "tridentautogrowpolicies.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	policies, err := crdClientset.TridentV1().TridentAutogrowPolicies().List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(policies.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, policy := range policies.Items {
		if policy.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentAutogrowPolicies().Delete(ctx(), policy.Name, deleteOpts)
		}
	}

	policies, err = crdClientset.TridentV1().TridentAutogrowPolicies().List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, policy := range policies.Items {
		if policy.HasTridentFinalizers() {
			crCopy := policy.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentAutogrowPolicies().Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentAutogrowPolicies().Delete
		if err := deleteWithRetry(deleteFunc, ctx(), policy.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTridentAutogrowRequestInternals() error {
	crd := "tridentautogrowrequestinternals.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	// List requests in all namespaces
	requests, err := crdClientset.TridentV1().TridentAutogrowRequestInternals("").List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(requests.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, request := range requests.Items {
		if request.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentAutogrowRequestInternals(request.Namespace).Delete(ctx(), request.Name, deleteOpts)
		}
	}

	requests, err = crdClientset.TridentV1().TridentAutogrowRequestInternals("").List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, request := range requests.Items {
		if request.HasTridentFinalizers() {
			crCopy := request.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentAutogrowRequestInternals(request.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := func(ctx context.Context, name string, opts metav1.DeleteOptions) error {
			return crdClientset.TridentV1().TridentAutogrowRequestInternals(request.Namespace).Delete(ctx, name, opts)
		}
		if err := deleteWithRetry(deleteFunc, ctx(), request.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
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

func deleteTridentActionMirrorUpdates() error {
	crd := "tridentactionmirrorupdates.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	tridentActionMirrorUpdates, err := crdClientset.TridentV1().TridentActionMirrorUpdates(allNamespaces).List(ctx(),
		listOpts)
	if err != nil {
		return err
	} else if len(tridentActionMirrorUpdates.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, mirrorUpdate := range tridentActionMirrorUpdates.Items {
		if mirrorUpdate.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentActionMirrorUpdates(mirrorUpdate.Namespace).Delete(ctx(),
				mirrorUpdate.Name, deleteOpts)
		}
	}

	tridentActionMirrorUpdates, err = crdClientset.TridentV1().TridentActionMirrorUpdates(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, mirrorUpdate := range tridentActionMirrorUpdates.Items {
		if mirrorUpdate.HasTridentFinalizers() {
			crCopy := mirrorUpdate.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentActionMirrorUpdates(mirrorUpdate.Namespace).Update(ctx(), crCopy,
				updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentActionMirrorUpdates(mirrorUpdate.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), mirrorUpdate.Name, nil); err != nil {
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

func deleteGroupSnapshots() error {
	crd := "tridentgroupsnapshots.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	tgsnaps, err := crdClientset.TridentV1().TridentGroupSnapshots(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(tgsnaps.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, tgsnap := range tgsnaps.Items {
		if tgsnap.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentGroupSnapshots(tgsnap.Namespace).Delete(ctx(), tgsnap.Name, deleteOpts)
		}
	}

	tgsnaps, err = crdClientset.TridentV1().TridentGroupSnapshots(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, tgsnap := range tgsnaps.Items {
		if tgsnap.HasTridentFinalizers() {
			crCopy := tgsnap.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentGroupSnapshots(tgsnap.Namespace).Update(ctx(), crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentGroupSnapshots(tgsnap.Namespace).Delete
		if err := deleteWithRetry(deleteFunc, ctx(), tgsnap.Name, nil); err != nil {
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

func deleteActionSnapshotRestores() error {
	crd := "tridentactionsnapshotrestores.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	vrefs, err := crdClientset.TridentV1().TridentActionSnapshotRestores(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(vrefs.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, vref := range vrefs.Items {
		if vref.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentActionSnapshotRestores(vref.Namespace).Delete(ctx(),
				vref.Name, deleteOpts)
		}
	}

	vrefs, err = crdClientset.TridentV1().TridentActionSnapshotRestores(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, vref := range vrefs.Items {
		if vref.HasTridentFinalizers() {
			crCopy := vref.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentActionSnapshotRestores(vref.Namespace).Update(ctx(),
				crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentActionSnapshotRestores(vref.Namespace).Delete
		if err = deleteWithRetry(deleteFunc, ctx(), vref.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteVolumeMoves() error {
	crd := "tridentvolumemoves.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	vrefs, err := crdClientset.TridentV1().TridentVolumeMoves(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(vrefs.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, vref := range vrefs.Items {
		if vref.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentVolumeMoves(vref.Namespace).Delete(ctx(),
				vref.Name, deleteOpts)
		}
	}

	vrefs, err = crdClientset.TridentV1().TridentVolumeMoves(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, vref := range vrefs.Items {
		if vref.HasTridentFinalizers() {
			crCopy := vref.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVolumeMoves(vref.Namespace).Update(ctx(),
				crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentVolumeMoves(vref.Namespace).Delete
		if err = deleteWithRetry(deleteFunc, ctx(), vref.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteNodeRemediations() error {
	crd := "tridentnoderemediations.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	allTnrs, err := crdClientset.TridentV1().TridentNodeRemediations(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(allTnrs.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, tnr := range allTnrs.Items {
		if tnr.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentNodeRemediations(tnr.Namespace).Delete(ctx(),
				tnr.Name, deleteOpts)
		}
	}

	allTnrs, err = crdClientset.TridentV1().TridentNodeRemediations(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, tnr := range allTnrs.Items {
		if tnr.HasTridentFinalizers() {
			crCopy := tnr.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentNodeRemediations(tnr.Namespace).Update(ctx(),
				crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentNodeRemediations(tnr.Namespace).Delete
		if err = deleteWithRetry(deleteFunc, ctx(), tnr.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteNodeRemediationTemplates() error {
	crd := "tridentnoderemediationtemplates.trident.netapp.io"
	logFields := LogFields{"CRD": crd}

	// See if CRD exists
	exists, err := k8sClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		Log().WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	vrefs, err := crdClientset.TridentV1().TridentNodeRemediationTemplates(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	} else if len(vrefs.Items) == 0 {
		Log().WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, vref := range vrefs.Items {
		if vref.DeletionTimestamp.IsZero() {
			_ = crdClientset.TridentV1().TridentNodeRemediationTemplates(vref.Namespace).Delete(ctx(),
				vref.Name, deleteOpts)
		}
	}

	vrefs, err = crdClientset.TridentV1().TridentNodeRemediationTemplates(allNamespaces).List(ctx(), listOpts)
	if err != nil {
		return err
	}

	for _, vref := range vrefs.Items {
		if vref.HasTridentFinalizers() {
			crCopy := vref.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentNodeRemediationTemplates(vref.Namespace).Update(ctx(),
				crCopy, updateOpts)
			if isNotFoundError(err) {
				continue
			} else if err != nil {
				Log().Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}

		deleteFunc := crdClientset.TridentV1().TridentNodeRemediationTemplates(vref.Namespace).Delete
		if err = deleteWithRetry(deleteFunc, ctx(), vref.Name, nil); err != nil {
			Log().Errorf("Problem deleting resource: %v", err)
			return err
		}
	}

	Log().WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteCRDs(crdNames []string) error {
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

// deleteWithRetry backoff tuning (overridden in unit tests for speed).
var (
	deleteWithRetryMaxElapsedTime  = 10 * time.Second
	deleteWithRetryInitialInterval = 1 * time.Second
	deleteWithRetryMaxInterval     = 5 * time.Second
)

func deleteWithRetry(deleteFunc crDeleter, c context.Context, name string, deleteOptions *metav1.DeleteOptions) error {
	if deleteOptions == nil {
		deleteOptions = &deleteOpts
	}

	timeout := deleteWithRetryMaxElapsedTime
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
	deleteBackoff.InitialInterval = deleteWithRetryInitialInterval
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.Multiplier = 1.414
	deleteBackoff.MaxInterval = deleteWithRetryMaxInterval
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
	deleteBackoff.InitialInterval = deleteWithRetryInitialInterval
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.Multiplier = 1.414
	deleteBackoff.MaxInterval = deleteWithRetryMaxInterval
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
