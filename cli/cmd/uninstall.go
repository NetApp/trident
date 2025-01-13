// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/network"
)

func init() {
	RootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during uninstallation.")
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initInstallerLogging()
		if err := discoverUninstallationEnvironment(); err != nil {
			Log().Fatalf("Uninstall pre-checks failed; %v", err)
		}
		if err := validateUninstallationArguments(); err != nil {
			Log().Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Run the uninstaller directly using the Kubernetes client
		if err := uninstallTrident(); err != nil {
			Log().Fatalf("Uninstall failed; %v", err)
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

	Log().WithFields(LogFields{
		"kubernetesVersion": client.Version().String(),
	}).Debug("Validated uninstallation environment.")

	return nil
}

func isCSITridentInstalled() (installed bool, namespace string, err error) {
	return client.CheckDeploymentExistsByLabel(TridentCSILabel, true)
}

// discoverTrident checks whether CSI Trident is installed
func discoverTrident() (installed bool, err error) {
	if installed, _, err = isCSITridentInstalled(); err != nil {
		err = fmt.Errorf("could not check if CSI Trident is installed; %v", err)
		return
	}

	return
}

func validateUninstallationArguments() error {
	if !network.DNS1123LabelRegex.MatchString(TridentPodNamespace) {
		return fmt.Errorf("%s is not a valid namespace name; a DNS-1123 label must consist "+
			"of lower case alphanumeric characters or '-', and must start and end with an alphanumeric "+
			"character", TridentPodNamespace)
	}

	return nil
}

func uninstallTrident() error {
	if client == nil {
		return fmt.Errorf("not able to connect to Kubernetes API server")
	}

	anyErrors := false

	_, err := discoverTrident()
	if err != nil {
		return err
	}

	// Set the app labels
	appLabel = TridentCSILabel
	appLabelKey = TridentCSILabelKey
	appLabelValue = TridentCSILabelValue

	// Delete Trident deployment
	if deployment, err := client.GetDeploymentByLabel(appLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warn("Trident deployment not found.")
	} else {
		// Deployment found by label, so ensure there isn't a namespace clash
		if TridentPodNamespace != deployment.Namespace {
			return fmt.Errorf("a Trident deployment was found in namespace '%s', "+
				"not in specified namespace '%s'", deployment.Namespace, TridentPodNamespace)
		}

		Log().WithFields(LogFields{
			"deployment": deployment.Name,
			"namespace":  deployment.Namespace,
		}).Debug("Trident deployment found by label.")

		// Delete the deployment
		if err = client.DeleteDeploymentByLabel(appLabel); err != nil {
			Log().WithFields(LogFields{
				"deployment": deployment.Name,
				"namespace":  deployment.Namespace,
				"label":      appLabel,
				"error":      err,
			}).Warning("Could not delete Trident deployment.")
			anyErrors = true
		} else {
			Log().Info("Deleted Trident deployment.")
		}
	}

	// Next handle all the other common CSI components (DaemonSet(s), service).
	if daemonsets, err := client.GetDaemonSetsByLabel(TridentNodeLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": TridentNodeLabel,
			"error": err,
		}).Warning("Trident DaemonSet(s) not found.")
	} else {
		for i := range daemonsets {
			// Daemonset found by label, so ensure there isn't a namespace clash
			if TridentPodNamespace != daemonsets[i].Namespace {
				return fmt.Errorf("a Trident DaemonSet was found in namespace '%s', "+
					"not in specified namespace '%s'", daemonsets[i].Namespace, TridentPodNamespace)
			}

			Log().WithFields(LogFields{
				"DaemonSet": daemonsets[i].Name,
				"namespace": daemonsets[i].Namespace,
			}).Debug("Trident DaemonSet found by label.")

			// Delete the DaemonSet
			if err = client.DeleteDaemonSetByLabelAndName(TridentNodeLabel, daemonsets[i].Name); err != nil {
				Log().WithFields(LogFields{
					"DaemonSet": daemonsets[i].Name,
					"namespace": daemonsets[i].Namespace,
					"label":     TridentNodeLabel,
					"error":     err,
				}).Warning("Could not delete Trident daemonset.")
				anyErrors = true
			} else {
				Log().Info("Deleted Trident daemonset.")
			}
		}
	}

	if service, err := client.GetServiceByLabel(TridentCSILabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": TridentCSILabel,
			"error": err,
		}).Warning("Trident service not found.")
	} else {

		// Service found by label, so ensure there isn't a namespace clash
		if TridentPodNamespace != service.Namespace {
			return fmt.Errorf("a Trident service was found in namespace '%s', "+
				"not in specified namespace '%s'", service.Namespace, TridentPodNamespace)
		}

		Log().WithFields(LogFields{
			"service":   service.Name,
			"namespace": service.Namespace,
		}).Debug("Trident service found by label.")

		// Delete the service
		if err = client.DeleteServiceByLabel(TridentCSILabel); err != nil {
			Log().WithFields(LogFields{
				"service":   service.Name,
				"namespace": service.Namespace,
				"label":     TridentCSILabel,
				"error":     err,
			}).Warning("Could not delete service.")
			anyErrors = true
		} else {
			Log().Info("Deleted Trident service.")
		}
	}

	if resourceQuota, err := client.GetResourceQuotaByLabel(TridentNodeLabel); err != nil {
		Log().WithFields(LogFields{
			"label": TridentNodeLabel,
			"error": err,
		}).Warning("Trident resource quota not found by label.")
	} else {

		// ResourceQuota found by label; ensure there isn't a namespace clash
		if TridentPodNamespace != resourceQuota.Namespace {
			return fmt.Errorf("a Trident resource quota was found in namespace '%s', "+
				"not in specified namespace '%s'", resourceQuota.Namespace, TridentPodNamespace)
		}

		Log().WithFields(LogFields{
			"resourcequota": resourceQuota.Name,
			"namespace":     resourceQuota.Namespace,
		}).Debug("Trident resource quota found by label.")

		// Delete the resource quota
		if err = client.DeleteResourceQuotaByLabel(TridentNodeLabel); err != nil {
			Log().WithFields(LogFields{
				"resourcequota": resourceQuota.Name,
				"namespace":     resourceQuota.Namespace,
				"label":         TridentNodeLabel,
				"error":         err,
			}).Warning("Could not delete resource quota.")
			anyErrors = true
		} else {
			Log().Info("Deleted Trident resource quota.")
		}
	}

	if secrets, err := client.GetSecretsByLabel(TridentCSILabel, false); err != nil {
		Log().WithFields(LogFields{
			"label": TridentCSILabel,
			"error": err,
		}).Warning("Trident secrets not found.")
	} else {
		for _, secret := range secrets {

			Log().WithFields(LogFields{
				"secret":    secret.Name,
				"namespace": secret.Namespace,
			}).Debug("Trident secret found by label.")

			// Check if the secret has our persistent object label and value. If so, don't remove it.
			if value, ok := secret.GetLabels()[TridentPersistentObjectLabelKey]; ok {
				if value == TridentPersistentObjectLabelValue {
					Log().WithFields(LogFields{
						"secret":    secret.Name,
						"namespace": secret.Namespace,
						"label":     TridentPersistentObjectLabel,
					}).Info("Retaining Trident secret.")
					continue
				}
			}

			// Deleting the secret by name should be safe since namespaced objects have unique names.
			if err = client.DeleteSecret(secret.Name, TridentPodNamespace); err != nil {
				Log().WithFields(LogFields{
					"secret":    secret.Name,
					"namespace": secret.Namespace,
					"label":     TridentCSILabel,
					"error":     err,
				}).Warning("Could not delete secret.")
				anyErrors = true
			} else {
				Log().Info("Deleted Trident secret.")
			}
		}
	}

	anyErrors = removeRBACObjects(log.InfoLevel) || anyErrors

	CSIDriverYAML := k8sclient.GetCSIDriverYAML(getCSIDriverName(), nil, nil)

	if err = client.DeleteObjectByYAML(CSIDriverYAML, true); err != nil {
		Log().WithField("error", err).Warning("Could not delete csidriver custom resource.")
		anyErrors = true
	} else {
		Log().WithField("CSIDriver", getCSIDriverName()).Info("Deleted csidriver custom resource.")
	}

	Log().Info("The uninstaller did not delete Trident's namespace in case it is going to be reused.")

	if !anyErrors {
		Log().Info("Trident uninstallation succeeded.")
	} else {
		Log().Error("Trident uninstallation completed with errors. " +
			"Please resolve those and run the uninstaller again.")
	}

	return nil
}
