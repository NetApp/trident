package cmd

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/k8s_client"
)

var (
	deleteAll bool
)

func init() {
	RootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolVarP(&deleteAll, "all", "a", false, "Deletes almost all artifacts of Trident, including the PVC and PV used by Trident; however, it doesn't delete the volume used by Trident from the storage backend. Use with caution!")
	uninstallCmd.Flags().BoolVarP(&silent, "silent", "", false, "Disable most output during uninstallation.")
	uninstallCmd.Flags().BoolVar(&csi, "csi", false, "Uninstall CSI Trident (experimental).")
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
		if err := uninstallTrident(); err != nil {
			log.Fatalf("Uninstall failed; %v", err)
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

	// Ensure we're on Linux
	if runtime.GOOS != "linux" {
		return errors.New("the Trident uninstaller only runs on Linux")
	}

	// Create the CLI-based Kubernetes client
	client, err = k8s_client.NewKubectlClient()
	if err != nil {
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
	}).Debug("Validated Trident uninstallation environment.")

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
