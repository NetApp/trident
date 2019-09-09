// Copyright 2019 NetApp, Inc. All Rights Reserved.
package cmd

import (
	"errors"
	"fmt"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
)

func init() {
	RootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during uninstallation.")
	uninstallCmd.Flags().StringVar(&tridentImage, "trident-image", "", "The Trident image to use for an in-cluster uninstall operation.")
	uninstallCmd.Flags().BoolVar(&inCluster, "in-cluster", true, "Run the installer as a job in the cluster.")

	uninstallCmd.Flags().MarkHidden("trident-image")
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

	var anyErrors = false

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

	// Next handle all the other common CSI components (daemonset, service).  Some/all of these may
	// not be present if uninstalling legacy Trident or preview CSI Trident, in which case we log
	// warnings only.

	if daemonset, err := client.GetDaemonSetByLabel(TridentNodeLabel, true); err != nil {

		log.WithFields(log.Fields{
			"label": TridentNodeLabel,
			"error": err,
		}).Warning("Trident daemonset not found.")

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
			}).Warning("Could not delete Trident daemonset.")
			anyErrors = true
		} else {
			log.Info("Deleted Trident daemonset.")
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

	if secret, err := client.GetSecretByLabel(TridentCSILabel, true); err != nil {

		log.WithFields(log.Fields{
			"label": TridentCSILabel,
			"error": err,
		}).Warning("Trident secret not found.")

	} else {

		// Secret found by label, so ensure there isn't a namespace clash
		if TridentPodNamespace != secret.Namespace {
			return fmt.Errorf("a Trident secret was found in namespace '%s', "+
				"not in specified namespace '%s'", secret.Namespace, TridentPodNamespace)
		}

		log.WithFields(log.Fields{
			"service":   secret.Name,
			"namespace": secret.Namespace,
		}).Debug("Trident secret found by label.")

		// Delete the secret
		if err = client.DeleteSecretByLabel(TridentCSILabel); err != nil {
			log.WithFields(log.Fields{
				"service":   secret.Name,
				"namespace": secret.Namespace,
				"label":     TridentCSILabel,
				"error":     err,
			}).Warning("Could not delete secret.")
			anyErrors = true
		} else {
			log.Info("Deleted Trident secret.")
		}
	}

	anyErrors = removeRBACObjects(log.InfoLevel) || anyErrors

	if csi {
		// Delete pod security policy
		podSecurityPolicyYAML := k8sclient.GetPodSecurityPolicyYAML()
		if err = client.DeleteObjectByYAML(podSecurityPolicyYAML, true); err != nil {
			log.WithField("error", err).Warning("Could not delete pod security policy.")
			anyErrors = true
		} else {
			log.WithField("podSecurityPolicy", "tridentpods").Info("Deleted pod security policy.")
		}

		log.Info("The uninstaller did not delete Trident's namespace in case it is going to be reused.")
	}

	if !anyErrors {
		log.Info("Trident uninstallation succeeded.")
	} else {
		log.Error("Trident uninstallation completed with errors. " +
			"Please resolve those and run the uninstaller again.")
	}

	return nil
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
	if silent {
		commandArgs = append(commandArgs, "--silent")
	}
	if tridentImage != "" {
		commandArgs = append(commandArgs, "--trident-image")
		commandArgs = append(commandArgs, tridentImage)
	}
	commandArgs = append(commandArgs, "--in-cluster=false")

	if csi {
		// Delete installer pod security policy if it already exists
		podSecurityPolicyYAML := k8sclient.GetInstallerSecurityPolicyYAML()
		if err := client.DeleteObjectByYAML(podSecurityPolicyYAML, true); err != nil {
			log.WithField("error", err).Errorf("Could not delete installer pod security policy; " +
				"please delete it manually.")
		} else {
			log.WithField("podSecurityPolicy", "tridentinstaller").Info("Deleted previous installer pod security policy.")
		}

		// Create installer pod security policy
		errMessage := "could not create installer pod security policy"
		returnError = createObjectsByYAML("installerPodSecurityPolicy",
			k8sclient.GetInstallerSecurityPolicyYAML(), errMessage)
		if returnError != nil {
			return returnError
		}
		log.WithFields(log.Fields{"podsecuritypolicy": "tridentinstaller"}).Info("Created installer pod security policy.")

		defer func() {
			// Delete pod security policy
			podSecurityPolicyYAML := k8sclient.GetInstallerSecurityPolicyYAML()
			if err := client.DeleteObjectByYAML(podSecurityPolicyYAML, true); err != nil {
				log.WithField("error", err).Errorf("Could not delete installer pod security policy; " +
					"please delete it manually.")
			} else {
				log.WithField("podSecurityPolicy", "tridentinstaller").Info("Deleted installer pod security policy.")
			}
		}()
	}

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
	uninstallPod, returnError = waitForPodToStart(TridentInstallerLabel, "installer")
	if returnError != nil {
		return
	}

	// Wait for pod to finish & output logs
	client.FollowPodLogs(uninstallPod.Name, "", uninstallPod.Namespace, logLogFmtMessage)

	uninstallPod, returnError = waitForPodToFinish(TridentInstallerLabel, "installer")
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
