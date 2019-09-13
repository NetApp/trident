// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
)

var (
	configPath string

	// kubeClient is our clientset
	kubeClient k8sclient.Interface

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
				if forceObliviate, err = getUserConfirmation(crdConfirmation); err != nil {
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

// initLogging configures logging. Logs are written to stdout.
func initLogging() {

	// Log to stdout only
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	logLevel := "info"
	if silent {
		logLevel = "fatal"
	}
	err := logging.InitLogLevel(Debug, logLevel)
	if err != nil {
		log.WithField("error", err).Fatal("Failed to initialize logging.")
	}

	log.WithField("logLevel", log.GetLevel().String()).Debug("Initialized logging.")
}

func initClients() error {

	// Detect whether we are running inside a pod
	if namespaceBytes, err := ioutil.ReadFile(config.TridentNamespaceFile); err == nil {

		if !forceObliviate {
			return errors.New("obliviation canceled")
		}

		// The namespace file exists, so we're in a pod.  Create an API-based client.
		kubeConfig, err := rest.InClusterConfig()
		if err != nil {
			return err
		}
		resetNamespace = string(namespaceBytes)

		log.WithField("namespace", resetNamespace).Debug("Running in a pod, creating API-based clients.")

		if kubeClient, err = k8sclient.NewKubeClient(kubeConfig, resetNamespace, k8sTimeout); err != nil {
			return err
		}

		if crdClientset, err = versioned.NewForConfig(kubeConfig); err != nil {
			return err
		}

	} else {

		if !forceObliviate {
			if forceObliviate, err = getUserConfirmation(crdConfirmation); err != nil {
				return err
			} else if !forceObliviate {
				return errors.New("obliviation canceled")
			}
		}

		// The namespace file didn't exist, so assume we're outside a pod.  Create a CLI-based client.
		log.Debug("Running outside a pod, creating CLI-based client.")

		if kubeClient, err = k8sclient.NewKubectlClient("", k8sTimeout); err != nil {
			return err
		}
		resetNamespace = kubeClient.Namespace()

		if restConfig, err := clientcmd.BuildConfigFromFlags("", configPath); err != nil {
			return err
		} else if crdClientset, err = versioned.NewForConfig(restConfig); err != nil {
			return err
		}
	}

	return nil
}

func obliviateCRDs() error {

	var err error

	// Create the Kubernetes client
	if client, err = initClient(); err != nil {
		return fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

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
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	versions, err := crdClientset.TridentV1().TridentVersions(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(versions.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, version := range versions.Items {
		if version.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentVersions(resetNamespace).Delete(version.Name, &metav1.DeleteOptions{})
		}
	}

	versions, err = crdClientset.TridentV1().TridentVersions(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, version := range versions.Items {
		if version.HasTridentFinalizers() {
			crCopy := version.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVersions(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentVersions(resetNamespace).Delete(version.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteBackends() error {

	crd := "tridentbackends.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithFields(logFields).Debug("CRD not present.")
		return nil
	}

	backends, err := crdClientset.TridentV1().TridentBackends(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(backends.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, backend := range backends.Items {
		if backend.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentBackends(resetNamespace).Delete(backend.Name, &metav1.DeleteOptions{})
		}
	}

	backends, err = crdClientset.TridentV1().TridentBackends(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, backend := range backends.Items {
		if backend.HasTridentFinalizers() {
			crCopy := backend.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentBackends(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentBackends(resetNamespace).Delete(backend.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteStorageClasses() error {

	crd := "tridentstorageclasses.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	storageclasses, err := crdClientset.TridentV1().TridentStorageClasses(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(storageclasses.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, sc := range storageclasses.Items {
		if sc.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentStorageClasses(resetNamespace).Delete(sc.Name, &metav1.DeleteOptions{})
		}
	}

	storageclasses, err = crdClientset.TridentV1().TridentStorageClasses(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, sc := range storageclasses.Items {
		if sc.HasTridentFinalizers() {
			crCopy := sc.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentStorageClasses(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentStorageClasses(resetNamespace).Delete(sc.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteVolumes() error {

	crd := "tridentvolumes.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	volumes, err := crdClientset.TridentV1().TridentVolumes(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(volumes.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, volume := range volumes.Items {
		if volume.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentVolumes(resetNamespace).Delete(volume.Name, &metav1.DeleteOptions{})
		}
	}

	volumes, err = crdClientset.TridentV1().TridentVolumes(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, volume := range volumes.Items {
		if volume.HasTridentFinalizers() {
			crCopy := volume.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentVolumes(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentVolumes(resetNamespace).Delete(volume.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteNodes() error {

	crd := "tridentnodes.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	nodes, err := crdClientset.TridentV1().TridentNodes(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(nodes.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, node := range nodes.Items {
		if node.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentNodes(resetNamespace).Delete(node.Name, &metav1.DeleteOptions{})
		}
	}

	nodes, err = crdClientset.TridentV1().TridentNodes(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		if node.HasTridentFinalizers() {
			crCopy := node.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentNodes(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentNodes(resetNamespace).Delete(node.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteTransactions() error {

	crd := "tridenttransactions.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	transactions, err := crdClientset.TridentV1().TridentTransactions(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(transactions.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, txn := range transactions.Items {
		if txn.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentTransactions(resetNamespace).Delete(txn.Name, &metav1.DeleteOptions{})
		}
	}

	transactions, err = crdClientset.TridentV1().TridentTransactions(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, txn := range transactions.Items {
		if txn.HasTridentFinalizers() {
			crCopy := txn.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentTransactions(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentTransactions(resetNamespace).Delete(txn.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteSnapshots() error {

	crd := "tridentsnapshots.trident.netapp.io"
	logFields := log.Fields{"CRD": crd}

	// See if CRD exists
	exists, err := kubeClient.CheckCRDExists(crd)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRD", crd).Debug("CRD not present.")
		return nil
	}

	snapshots, err := crdClientset.TridentV1().TridentSnapshots(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	} else if len(snapshots.Items) == 0 {
		log.WithFields(logFields).Info("Resources not present.")
		return nil
	}

	for _, snapshot := range snapshots.Items {
		if snapshot.DeletionTimestamp.IsZero() {
			crdClientset.TridentV1().TridentSnapshots(resetNamespace).Delete(snapshot.Name, &metav1.DeleteOptions{})
		}
	}

	snapshots, err = crdClientset.TridentV1().TridentSnapshots(resetNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, snapshot := range snapshots.Items {
		if snapshot.HasTridentFinalizers() {
			crCopy := snapshot.DeepCopy()
			crCopy.RemoveTridentFinalizers()
			_, err := crdClientset.TridentV1().TridentSnapshots(resetNamespace).Update(crCopy)
			if err != nil {
				log.Errorf("Problem removing finalizers: %v", err)
				return err
			}
		}
		crdClientset.TridentV1().TridentSnapshots(resetNamespace).Delete(snapshot.Name, &metav1.DeleteOptions{})
	}

	log.WithFields(logFields).Info("Resources deleted.")
	return nil
}

func deleteCRDs() error {

	crdNames := []string{
		"tridentversions.trident.netapp.io",
		"tridentbackends.trident.netapp.io",
		"tridentstorageclasses.trident.netapp.io",
		"tridentvolumes.trident.netapp.io",
		"tridentnodes.trident.netapp.io",
		"tridenttransactions.trident.netapp.io",
		"tridentsnapshots.trident.netapp.io",
	}

	for _, crdName := range crdNames {

		logFields := log.Fields{"CRD": crdName}

		// See if CRD exists
		exists, err := kubeClient.CheckCRDExists(crdName)
		if err != nil {
			return err
		}
		if !exists {
			log.WithFields(logFields).Info("CRD not present.")
			continue
		}

		log.WithFields(logFields).Debug("Deleting CRD.")

		// Try deleting CRD
		if err := kubeClient.DeleteCRD(crdName); err != nil {
			log.WithFields(logFields).Errorf("Could not delete CRD; %v", err)
			return err
		}

		// Check if CRD still exists (mostly likely pinned by finalizer)
		if exists, err := kubeClient.CheckCRDExists(crdName); err != nil {
			return err
		} else if !exists {
			log.WithFields(logFields).Info("CRD deleted.")
			continue
		}

		log.WithFields(logFields).Debug("CRD still present, must remove finalizers.")

		// Remove finalizer
		if err := kubeClient.RemoveFinalizerFromCRD(crdName); err != nil {
			log.WithFields(logFields).Errorf("Could not remove finalizer from CRD; %v", err)
			return err
		} else {
			log.WithFields(logFields).Debug("Removed finalizer from CRD.")
		}

		// Check if removing the finalizer allowed the CRD to be deleted
		if exists, err = kubeClient.CheckCRDExists(crdName); err != nil {
			return err
		} else if !exists {
			log.WithFields(logFields).Info("CRD deleted after removing finalizers.")
			continue
		} else {
			log.WithFields(logFields).Error("CRD still not deleted after removing finalizers.")
		}
	}

	return nil
}
