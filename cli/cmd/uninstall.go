// Copyright 2021 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
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
			log.Fatalf("Uninstall pre-checks failed; %v", err)
		}
		if err := validateUninstallationArguments(); err != nil {
			log.Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Run the uninstaller directly using the Kubernetes client
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

	log.WithFields(log.Fields{
		"kubernetesVersion": client.Version().String(),
	}).Debug("Validated uninstallation environment.")

	return nil
}

func isTridentInstalled() (installed bool, namespace string, err error) {
	return client.CheckDeploymentExistsByLabel(TridentLegacyLabel, true)
}

func isPreviewCSITridentInstalled() (installed bool, namespace string, err error) {
	return client.CheckStatefulSetExistsByLabel(TridentCSILabel, true)
}

func isCSITridentInstalled() (installed bool, namespace string, err error) {
	return client.CheckDeploymentExistsByLabel(TridentCSILabel, true)
}

func discoverTrident() (legacy, csi, csiPreview bool, err error) {
	// Check if legacy Trident is installed
	if legacy, _, err = isTridentInstalled(); err != nil {
		err = fmt.Errorf("could not check if legacy Trident is installed; %v", err)
		return
	}

	// Check if preview CSI Trident is installed
	if csiPreview, _, err = isPreviewCSITridentInstalled(); err != nil {
		err = fmt.Errorf("could not check if preview CSI Trident is installed; %v", err)
		return
	}

	// Check if CSI Trident is installed
	if csi, _, err = isCSITridentInstalled(); err != nil {
		err = fmt.Errorf("could not check if CSI Trident is installed; %v", err)
		return
	}

	return
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
	// 1. preview CSI Trident --> uninstall preview CSI Trident
	// 2. preview CSI Trident & legacy Trident --> uninstall preview CSI Trident
	// 3. CSI Trident --> uninstall CSI Trident
	// 4. legacy Trident --> uninstall legacy Trident
	//
	// if csiPreview, uninstall csiPreview
	// else if csi, uninstall csi
	// else if legacy, uninstall legacy

	anyErrors := false

	legacyTridentInstalled, csiTridentInstalled, csiPreviewTridentInstalled, err := discoverTrident()
	if err != nil {
		return err
	}

	if legacyTridentInstalled && csiPreviewTridentInstalled {
		log.Warning("Both legacy and CSI Trident are installed.  CSI Trident will be uninstalled, and " +
			"you must run the uninstaller again to remove legacy Trident before running the Trident installer.")
	}

	// Set the global csi variable, which controls things like RBAC and app labels
	csi = csiTridentInstalled || csiPreviewTridentInstalled

	// Set the app labels (CSI takes precedence)
	if csi {
		appLabel = TridentCSILabel
		appLabelKey = TridentCSILabelKey
		appLabelValue = TridentCSILabelValue
	} else {
		appLabel = TridentLegacyLabel
		appLabelKey = TridentLegacyLabelKey
		appLabelValue = TridentLegacyLabelValue
	}

	// First handle the deployment (legacy, CSI) / statefulset (preview CSI)

	if csiPreviewTridentInstalled {
		// Delete Trident statefulset
		if statefulset, err := client.GetStatefulSetByLabel(appLabel, true); err != nil {
			log.WithFields(log.Fields{
				"label": appLabel,
				"error": err,
			}).Warn("Trident statefulset not found.")
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
				}).Warning("Could not delete Trident statefulset.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident statefulset.")
			}
		}
	} else {
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
				}).Warning("Could not delete Trident deployment.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident deployment.")
			}
		}
	}

	// Next handle all the other common CSI components (DaemonSet(s), service).  Some/all of these may
	// not be present if uninstalling legacy Trident or preview CSI Trident, in which case we log
	// warnings only.
	if daemonsets, err := client.GetDaemonSetsByLabel(TridentNodeLabel, true); err != nil {
		log.WithFields(log.Fields{
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

			log.WithFields(log.Fields{
				"DaemonSet": daemonsets[i].Name,
				"namespace": daemonsets[i].Namespace,
			}).Debug("Trident DaemonSet found by label.")

			// Delete the DaemonSet
			if err = client.DeleteDaemonSetByLabelAndName(TridentNodeLabel, daemonsets[i].Name); err != nil {
				log.WithFields(log.Fields{
					"DaemonSet": daemonsets[i].Name,
					"namespace": daemonsets[i].Namespace,
					"label":     TridentNodeLabel,
					"error":     err,
				}).Warning("Could not delete Trident DaemonSet.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident DaemonSet.")
			}
		}
	}

	if service, err := client.GetServiceByLabel(TridentCSILabel, true); err != nil {
		log.WithFields(log.Fields{
			"label": TridentCSILabel,
			"error": err,
		}).Warning("Trident service not found.")
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
		if err = client.DeleteServiceByLabel(TridentCSILabel); err != nil {
			log.WithFields(log.Fields{
				"service":   service.Name,
				"namespace": service.Namespace,
				"label":     TridentCSILabel,
				"error":     err,
			}).Warning("Could not delete service.")
			anyErrors = true
		} else {
			log.Info("Deleted Trident service.")
		}
	}

	if resourceQuota, err := client.GetResourceQuotaByLabel(TridentNodeLabel); err != nil {
		log.WithFields(log.Fields{
			"label": TridentNodeLabel,
			"error": err,
		}).Warning("Trident resource quota not found by label.")
	} else {

		// ResourceQuota found by label; ensure there isn't a namespace clash
		if TridentPodNamespace != resourceQuota.Namespace {
			return fmt.Errorf("a Trident resource quota was found in namespace '%s', "+
				"not in specified namespace '%s'", resourceQuota.Namespace, TridentPodNamespace)
		}

		log.WithFields(log.Fields{
			"resourcequota": resourceQuota.Name,
			"namespace":     resourceQuota.Namespace,
		}).Debug("Trident resource quota found by label.")

		// Delete the resource quota
		if err = client.DeleteResourceQuotaByLabel(TridentNodeLabel); err != nil {
			log.WithFields(log.Fields{
				"resourcequota": resourceQuota.Name,
				"namespace":     resourceQuota.Namespace,
				"label":         TridentNodeLabel,
				"error":         err,
			}).Warning("Could not delete resource quota.")
			anyErrors = true
		} else {
			log.Info("Deleted Trident resource quota.")
		}
	}

	if secrets, err := client.GetSecretsByLabel(TridentCSILabel, false); err != nil {
		log.WithFields(log.Fields{
			"label": TridentCSILabel,
			"error": err,
		}).Warning("Trident secrets not found.")
	} else {
		for _, secret := range secrets {

			log.WithFields(log.Fields{
				"secret":    secret.Name,
				"namespace": secret.Namespace,
			}).Debug("Trident secret found by label.")

			// Check if the secret has our persistent object label and value. If so, don't remove it.
			if value, ok := secret.GetLabels()[TridentPersistentObjectLabelKey]; ok {
				if value == TridentPersistentObjectLabelValue {
					log.WithFields(log.Fields{
						"secret":    secret.Name,
						"namespace": secret.Namespace,
						"label":     TridentPersistentObjectLabel,
					}).Info("Retaining Trident secret.")
					continue
				}
			}

			// Deleting the secret by name should be safe since namespaced objects have unique names.
			if err = client.DeleteSecret(secret.Name, TridentPodNamespace); err != nil {
				log.WithFields(log.Fields{
					"secret":    secret.Name,
					"namespace": secret.Namespace,
					"label":     TridentCSILabel,
					"error":     err,
				}).Warning("Could not delete secret.")
				anyErrors = true
			} else {
				log.Info("Deleted Trident secret.")
			}
		}
	}

	anyErrors = removeRBACObjects(log.InfoLevel) || anyErrors

	// Delete pod security policy
	podSecurityPolicyYAML := k8sclient.GetPrivilegedPodSecurityPolicyYAML(getPSPName(), nil, nil)
	if !csi {
		podSecurityPolicyYAML = k8sclient.GetUnprivilegedPodSecurityPolicyYAML(getPSPName(), nil, nil)
	}
	if err = client.DeleteObjectByYAML(podSecurityPolicyYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete pod security policy.")
		anyErrors = true
	} else {
		log.WithField("podSecurityPolicy", "tridentpods").Info("Deleted pod security policy.")
	}

	if csi {
		CSIDriverYAML := k8sclient.GetCSIDriverYAML(getCSIDriverName(), nil, nil)

		if err = client.DeleteObjectByYAML(CSIDriverYAML, true); err != nil {
			log.WithField("error", err).Warning("Could not delete csidriver custom resource.")
			anyErrors = true
		} else {
			log.WithField("CSIDriver", getCSIDriverName()).Info("Deleted csidriver custom resource.")
		}
	}

	log.Info("The uninstaller did not delete Trident's namespace in case it is going to be reused.")

	if !anyErrors {
		log.Info("Trident uninstallation succeeded.")
	} else {
		log.Error("Trident uninstallation completed with errors. " +
			"Please resolve those and run the uninstaller again.")
	}

	return nil
}
