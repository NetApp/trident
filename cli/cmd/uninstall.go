package cmd

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	snapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"

	"github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
)

var (
	deleteAll bool
)

func init() {
	RootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolVarP(&deleteAll, "all", "a", false, "Deletes almost all artifacts of Trident, including the PVC and PV used by Trident; however, it doesn't delete the volume used by Trident from the storage backend. Use with caution!")
	uninstallCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during uninstallation.")
	uninstallCmd.Flags().BoolVar(&csi, "csi", false, "Uninstall CSI Trident (alpha, not for production clusters).")
	uninstallCmd.Flags().StringVar(&tridentImage, "trident-image", "", "The Trident image to use for an in-cluster uninstall operation.")
	uninstallCmd.Flags().MarkHidden("trident-image")
	uninstallCmd.Flags().BoolVar(&inCluster, "in-cluster", true, "Run the installer as a job in the cluster.")
	uninstallCmd.Flags().MarkHidden("in-cluster")
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initInstallerLogging()
		if err := discoverUninstallationEnvironment(); err != nil {
			log.Fatalf("Uninstall pre-checks failed; %v", err)
		}
		processUninstallationArguments()
		if err := validateUninstallationArguments(); err != nil {
			log.Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {

		if inCluster {
			// Run the uninstaller as a Kubernetes job
			if err := uninstallTridentInCluster(); err != nil {
				log.Fatalf("Uninstall failed; %v", err)
			}
		} else {
			// Run the uninstaller directly using the Kubernetes client
			if err := uninstallTrident(); err != nil {
				log.Fatalf("Uninstall failed; %v", err)
			}
		}
	},
}

// discoverUninstallationEnvironment inspects the current environment and checks
// that everything looks good for Trident uninstallation, but it makes no changes
// to the environment.
func discoverUninstallationEnvironment() error {

	var err error

	OperatingMode = ModeInstall
	Server = ""

	// Default deployment image to what Trident was built with
	if tridentImage == "" {
		tridentImage = tridentconfig.BuildImage
	}

	// Ensure we're on Linux
	if runtime.GOOS != "linux" {
		return errors.New("the Trident uninstaller only runs on Linux")
	}

	// Create the Kubernetes client
	if client, err = initClient(); err != nil {
		return fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// Infer installation namespace if not specified
	if TridentPodNamespace == "" {
		TridentPodNamespace = client.Namespace()
	}

	// Direct all subsequent client commands to the chosen namespace
	client.SetNamespace(TridentPodNamespace)

	// Prepare input file paths
	if err = prepareYAMLFilePaths(); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"kubernetesVersion": client.Version().String(),
	}).Debug("Validated uninstallation environment.")

	return nil
}

func isTridentInstalled() (installed bool, namespace string, err error) {

	if deploymentExists, namespace, err := client.CheckDeploymentExistsByLabel(TridentLabel, true); err != nil {
		return false, "", err
	} else if deploymentExists {
		return true, namespace, nil
	}

	return false, "", nil
}

func isCSITridentInstalled() (installed bool, namespace string, err error) {

	if statefulSetExists, namespace, err := client.CheckStatefulSetExistsByLabel(TridentCSILabel, true); err != nil {
		return false, "", err
	} else if statefulSetExists {
		return true, namespace, nil
	}

	if daemonSetExists, namespace, err := client.CheckDaemonSetExistsByLabel(TridentNodeLabel, true); err != nil {
		return false, "", err
	} else if daemonSetExists {
		return true, namespace, nil
	}

	return false, "", nil
}

func processUninstallationArguments() {

	if csi {
		appLabel = TridentCSILabel
		appLabelKey = TridentCSILabelKey
		appLabelValue = TridentCSILabelValue
	} else {
		appLabel = TridentLabel
		appLabelKey = TridentLabelKey
		appLabelValue = TridentLabelValue
	}
}

func validateUninstallationArguments() error {

	if !dns1123LabelRegex.MatchString(TridentPodNamespace) {
		return fmt.Errorf("%s is not a valid namespace name; a DNS-1123 label must consist "+
			"of lower case alphanumeric characters or '-', and must start and end with an alphanumeric "+
			"character", TridentPodNamespace)
	}

	return nil
}

func uninstallTrident() error {

	var anyErrors = false

	if !csi {

		// Delete Trident deployment
		if deployment, err := client.GetDeploymentByLabel(appLabel, true); err != nil {

			log.WithFields(log.Fields{
				"label": appLabel,
				"error": err,
			}).Warn("Trident deployment not found.")

		} else {

			// Deployment found by label, so ensure there isn't a namespace clash
			if TridentPodNamespace != deployment.Namespace {
				return fmt.Errorf("a Trident deployment was found in namespace '%s', "+
					"not in specified namespace '%s'", deployment.Namespace, TridentPodNamespace)
			}

			log.WithFields(log.Fields{
				"deployment": deployment.Name,
				"namespace":  deployment.Namespace,
			}).Debug("Trident deployment found by label.")

			// Delete the deployment
			if err = client.DeleteDeploymentByLabel(appLabel); err != nil {
				log.WithFields(log.Fields{
					"deployment": deployment.Name,
					"namespace":  deployment.Namespace,
					"label":      appLabel,
					"error":      err,
				}).Warning("Could not delete deployment.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident deployment.")
			}
		}

	} else {

		// Delete CSI Trident components
		if daemonset, err := client.GetDaemonSetByLabel(TridentNodeLabel, true); err != nil {

			log.WithFields(log.Fields{
				"label": TridentNodeLabel,
				"error": err,
			}).Warning("Trident daemonset not found.")
			anyErrors = true

		} else {
			// Daemonset found by label, so ensure there isn't a namespace clash
			if TridentPodNamespace != daemonset.Namespace {
				return fmt.Errorf("a Trident daemonset was found in namespace '%s', "+
					"not in specified namespace '%s'", daemonset.Namespace, TridentPodNamespace)
			}

			log.WithFields(log.Fields{
				"daemonset": daemonset.Name,
				"namespace": daemonset.Namespace,
			}).Debug("Trident daemonset found by label.")

			// Delete the daemonset
			if err = client.DeleteDaemonSetByLabel(TridentNodeLabel); err != nil {
				log.WithFields(log.Fields{
					"daemonset": daemonset.Name,
					"namespace": daemonset.Namespace,
					"label":     TridentNodeLabel,
					"error":     err,
				}).Warning("Could not delete daemonset.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident daemonset.")
			}
		}

		if statefulset, err := client.GetStatefulSetByLabel(appLabel, true); err != nil {

			log.WithFields(log.Fields{
				"label": appLabel,
				"error": err,
			}).Warning("Trident statefulset not found.")
			anyErrors = true

		} else {

			// Statefulset found by label, so ensure there isn't a namespace clash
			if TridentPodNamespace != statefulset.Namespace {
				return fmt.Errorf("a Trident statefulset was found in namespace '%s', "+
					"not in specified namespace '%s'", statefulset.Namespace, TridentPodNamespace)
			}

			log.WithFields(log.Fields{
				"statefulset": statefulset.Name,
				"namespace":   statefulset.Namespace,
			}).Debug("Trident statefulset found by label.")

			// Delete the statefulset
			if err = client.DeleteStatefulSetByLabel(appLabel); err != nil {
				log.WithFields(log.Fields{
					"statefulset": statefulset.Name,
					"namespace":   statefulset.Namespace,
					"label":       appLabel,
					"error":       err,
				}).Warning("Could not delete statefulset.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident statefulset.")
			}
		}

		if service, err := client.GetServiceByLabel(appLabel, true); err != nil {

			log.WithFields(log.Fields{
				"label": appLabel,
				"error": err,
			}).Warning("Trident service not found.")
			anyErrors = true

		} else {

			// Service found by label, so ensure there isn't a namespace clash
			if TridentPodNamespace != service.Namespace {
				return fmt.Errorf("a Trident service was found in namespace '%s', "+
					"not in specified namespace '%s'", service.Namespace, TridentPodNamespace)
			}

			log.WithFields(log.Fields{
				"service":   service.Name,
				"namespace": service.Namespace,
			}).Debug("Trident service found by label.")

			// Delete the service
			if err = client.DeleteServiceByLabel(appLabel); err != nil {
				log.WithFields(log.Fields{
					"service":   service.Name,
					"namespace": service.Namespace,
					"label":     appLabel,
					"error":     err,
				}).Warning("Could not delete service.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident service.")
			}
		}

	}

	anyErrors = removeRBACObjects(log.InfoLevel) || anyErrors

	if deleteAll {

		// Ensure the Trident PVC may be uniquely identified, then delete it
		if pvc, err := client.GetPVCByLabel(appLabel, false); err != nil {
			log.WithField("error", err).Warning("Could not uniquely identify Trident PVC.")
			anyErrors = true
		} else if err = client.DeletePVCByLabel(appLabel); err != nil {
			log.WithFields(log.Fields{
				"pvc":       pvc.Name,
				"namespace": pvc.Namespace,
				"error":     err,
			}).Warning("Could not delete Trident PVC.")
			anyErrors = true
		} else {
			log.WithFields(log.Fields{
				"pvc":       pvc.Name,
				"namespace": pvc.Namespace,
			}).Info("Deleted Trident PVC.")
		}

		// Ensure the Trident PV may be uniquely identified, then delete it
		if pv, err := client.GetPVByLabel(appLabel); err != nil {
			log.WithField("error", err).Warning("Could not uniquely identify Trident PV.")
			anyErrors = true
		} else if err = client.DeletePVByLabel(appLabel); err != nil {
			log.WithFields(log.Fields{
				"pv":    pv.Name,
				"error": err,
			}).Warning("Could not delete Trident PV.")
			anyErrors = true
		} else {
			log.WithField("pv", pv.Name).Info("Deleted Trident PV.")
		}

		log.Info("If desired, the volume on the storage backend must be manually deleted. " +
			"Deleting the volume would result in losing all the state that Trident maintains " +
			"to manage storage backends, storage classes, and provisioned volumes!")
	} else {

		log.Info("The uninstaller did not delete the Trident's namespace, PVC, and PV " +
			"in case they are going to be reused. Please use the --all option if you need " +
			"the PVC and PV deleted.")
	}

	// Delete the snapshot CRDs
	if client.Flavor() == k8sclient.FlavorKubernetes {
		anyErrors = deleteSnapshotCRD() || anyErrors
	}

	if !anyErrors {
		log.Info("Trident uninstallation succeeded.")
	} else {
		log.Error("Trident uninstallation completed with errors. Please resolve those " +
			"and run the uninstaller again.")
	}

	return nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func uninstallTridentInCluster() (returnError error) {

	// Ensure Trident installer pod isn't already present
	if podPresent, namespace, err := client.CheckPodExistsByLabel(TridentInstallerLabel, true); err != nil {
		return fmt.Errorf("could not check if Trident uninstaller pod is present; %v", err)
	} else if podPresent {
		if returnError = client.DeletePodByLabel(TridentInstallerLabel); returnError != nil {
			log.WithFields(log.Fields{
				"pod":       "trident-installer",
				"namespace": namespace,
			}).Error("Could not delete pod; please delete it manually and try again.")
			return
		}
	}

	// Ensure Trident installer configmap isn't already present (try to clean it up if present)
	if configmapPresent, namespace, err := client.CheckConfigMapExistsByLabel(TridentInstallerLabel, true); err != nil {
		return fmt.Errorf("could not check if Trident installer configmap is present; %v", err)
	} else if configmapPresent {
		if returnError = client.DeleteConfigMapByLabel(TridentInstallerLabel); returnError != nil {
			log.WithFields(log.Fields{
				"configmap": "trident-installer",
				"namespace": namespace,
			}).Error("Could not delete configmap; please delete it manually and try again.")
			return
		}
	}

	// Remove any RBAC objects from a previous Trident installation
	if anyCleanupErrors := removeInstallerRBACObjects(log.DebugLevel); anyCleanupErrors {
		returnError = fmt.Errorf("could not remove one or more previous Trident installer artifacts; " +
			"please delete them manually and try again")
		return
	}

	// Create the RBAC objects
	if returnError = createInstallerRBACObjects(); returnError != nil {
		return
	}

	// Make sure we always clean up the RBAC objects
	defer func() {
		if anyCleanupErrors := removeInstallerRBACObjects(log.InfoLevel); anyCleanupErrors {
			log.Errorf("could not remove one or more Trident installer artifacts; " +
				"please delete them manually")
		}
	}()

	// Create the installer arguments
	commandArgs := []string{
		"tridentctl", "uninstall",
		"--namespace", TridentPodNamespace,
	}
	if Debug {
		commandArgs = append(commandArgs, "--debug")
	}
	if deleteAll {
		commandArgs = append(commandArgs, "--all")
	}
	if silent {
		commandArgs = append(commandArgs, "--silent")
	}
	if csi {
		commandArgs = append(commandArgs, "--csi")
	}
	if tridentImage != "" {
		commandArgs = append(commandArgs, "--trident-image")
		commandArgs = append(commandArgs, tridentImage)
	}
	commandArgs = append(commandArgs, "--in-cluster=false")

	// Create the uninstall pod
	returnError = client.CreateObjectByYAML(k8sclient.GetUninstallerPodYAML(
		TridentInstallerLabelValue, tridentImage, commandArgs))
	if returnError != nil {
		returnError = fmt.Errorf("could not create uninstaller pod; %v", returnError)
		return
	}
	log.WithFields(log.Fields{"pod": "trident-installer"}).Info("Created uninstaller pod.")

	// Wait for Trident installation pod to start
	var uninstallPod *v1.Pod
	uninstallPod, returnError = waitForTridentInstallationPodToStart()
	if returnError != nil {
		return
	}

	// Wait for pod to finish & output logs
	followInstallationLogs(uninstallPod.Name, "", uninstallPod.Namespace)

	uninstallPod, returnError = waitForTridentInstallationPodToFinish()
	if returnError != nil {
		return
	}

	// Clean up the pod if it succeeded, otherwise leave it around for inspection
	if uninstallPod.Status.Phase == v1.PodSucceeded {

		if returnError = client.DeletePodByLabel(TridentInstallerLabel); returnError != nil {
			log.WithFields(log.Fields{"pod": "trident-installer"}).Error("Could not delete uninstaller pod;" +
				"please delete it manually.")
		} else {
			log.WithFields(log.Fields{"pod": "trident-installer"}).Info("Deleted uninstaller pod.")
		}
	} else {

		log.WithFields(log.Fields{
			"pod": "trident-installer",
		}).Warningf("Uninstaller pod status is %s. Use '%s describe pod %s -n %s' for more information.",
			uninstallPod.Status.Phase, client.CLI(), uninstallPod.Name, client.Namespace())
	}

	return
}

func deleteSnapshotCRD() bool {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Error("failed to get InClusterConfig")
		return true
	}
	aeclient, err := apiextensionsclient.NewForConfig(kubeConfig)
	if err != nil {
		log.WithError(err).Error("failed to create Kubernetes API Extensions client")
		return true
	}
	// VolumeSnapshotClass
	anyErrors := false
	crdName := snapshotv1alpha1.VolumeSnapshotClassResourcePlural + "." + snapshotv1alpha1.GroupName
	if err = aeclient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(crdName, nil); err != nil {
		log.WithError(err).Error("failed to delete VolumeSnapshotClass resource")
		anyErrors = true
	}
	// VolumeSnapshotContent
	crdName = snapshotv1alpha1.VolumeSnapshotContentResourcePlural + "." + snapshotv1alpha1.GroupName
	if err = aeclient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(crdName, nil); err != nil {
		log.WithError(err).Error("failed to delete VolumeSnapshotContent resource")
		anyErrors = true
	}
	// VolumeSnapshot
	crdName = snapshotv1alpha1.VolumeSnapshotResourcePlural + "." + snapshotv1alpha1.GroupName
	if err = aeclient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(crdName, nil); err != nil {
		log.WithError(err).Error("failed to delete VolumeSnapshot resource")
		anyErrors = true
	}
	if !anyErrors {
		log.Info("Deleted Volume Snapshot CRD.")
	}
	return anyErrors
}
