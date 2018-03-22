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
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initInstallerLogging()
		if err := discoverUninstallationEnvironment(); err != nil {
			log.Fatalf("Uninstall pre-checks failed; %v", err)
		}
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
		return errors.New("the Trident uninstaller only runs on Linux.")
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

func validateUninstallationArguments() error {

	if !dns1123LabelRegex.MatchString(TridentPodNamespace) {
		return fmt.Errorf("%s is not a valid namespace name; a DNS-1123 label must consist "+
			"of lower case alphanumeric characters or '-', and must start and end with an alphanumeric "+
			"character", TridentPodNamespace)
	}

	return nil
}

func uninstallTrident() error {

	var deleted bool
	var anyErrors = false

	// Delete Trident deployment (try label first, fall back to YAML file)
	deleted = false
	if deployment, err := client.GetDeploymentByLabel(TridentLabel, true); err == nil {

		// Deployment found by label, so ensure there isn't a namespace clash
		if TridentPodNamespace != deployment.Namespace {
			return fmt.Errorf("Trident deployment found in namespace '%s', not in "+
				"specified namespace '%s'", deployment.Namespace, TridentPodNamespace)
		}

		log.WithFields(log.Fields{
			"deployment": deployment.Name,
			"namespace":  deployment.Namespace,
		}).Debug("Trident deployment found by label.")

		// Delete the deployment
		if err = client.DeleteDeploymentByLabel(TridentLabel); err != nil {
			log.WithFields(log.Fields{
				"deployment": deployment.Name,
				"namespace":  deployment.Namespace,
				"label":      TridentLabel,
				"error":      err,
			}).Warning("Could not delete deployment.")
		} else {
			deleted = true
		}

	} else if fileExists(deploymentPath) {

		log.WithFields(log.Fields{
			"namespace": client.Namespace(),
		}).Warning("Trident deployment not found by label, using existing YAML file.")

		// Delete the deployment in the current namespace
		if err = client.DeleteObjectByFile(deploymentPath, false); err != nil {
			log.WithFields(log.Fields{
				"path":  deploymentPath,
				"error": err,
			}).Warning("Could not delete deployment using existing YAML file.")
		} else {
			deleted = true
		}

	} else {
		log.Warning("Trident deployment not found.")
	}
	if deleted {
		log.Info("Deleted Trident deployment.")
	} else {
		anyErrors = true
	}

	anyErrors = removeRBACObjects(log.InfoLevel) || anyErrors

	if deleteAll {

		// Ensure the Trident PVC may be uniquely identified, then delete it
		if pvc, err := client.GetPVCByLabel(TridentLabel, false); err != nil {
			log.WithField("error", err).Warning("Could not uniquely identify Trident PVC.")
			anyErrors = true
		} else if err = client.DeletePVCByLabel(TridentLabel); err != nil {
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
		if pv, err := client.GetPVByLabel(TridentLabel); err != nil {
			log.WithField("error", err).Warning("Could not uniquely identify Trident PV.")
			anyErrors = true
		} else if err = client.DeletePVByLabel(TridentLabel); err != nil {
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
