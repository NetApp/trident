// Copyright 2020 NetApp, Inc. All Rights Reserved.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/ghodss/yaml"
	"github.com/go-logfmt/logfmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/client-go/rest"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
)

const (
	PreferredNamespace = tridentconfig.OrchestratorName
	DefaultPVCName     = tridentconfig.OrchestratorName
	DefaultPVName      = tridentconfig.OrchestratorName

	// CRD names
	BackendCRDName      = "tridentbackends.trident.netapp.io"
	NodeCRDName         = "tridentnodes.trident.netapp.io"
	StorageClassCRDName = "tridentstorageclasses.trident.netapp.io"
	TransactionCRDName  = "tridenttransactions.trident.netapp.io"
	VersionCRDName      = "tridentversions.trident.netapp.io"
	VolumeCRDName       = "tridentvolumes.trident.netapp.io"
	SnapshotCRDName     = "tridentsnapshots.trident.netapp.io"

	NamespaceFilename          = "trident-namespace.yaml"
	ServiceAccountFilename     = "trident-serviceaccount.yaml"
	ClusterRoleFilename        = "trident-clusterrole.yaml"
	ClusterRoleBindingFilename = "trident-clusterrolebinding.yaml"
	DeploymentFilename         = "trident-deployment.yaml"
	ServiceFilename            = "trident-service.yaml"
	DaemonSetFilename          = "trident-daemonset.yaml"
	CRDsFilename               = "trident-crds.yaml"
	PodSecurityPolicyFilename  = "trident-podsecuritypolicy.yaml"
)

var (
	// CLI flags
	generateYAML    bool
	useYAML         bool
	silent          bool
	csi             bool
	inCluster       bool
	pvName          string
	pvcName         string
	tridentImage    string
	etcdImage       string
	logFormat       string
	k8sTimeout      time.Duration
	migratorTimeout time.Duration

	// CLI-based K8S client
	client k8sclient.Interface

	// File paths
	installerDirectoryPath string
	setupPath              string
	namespacePath          string
	serviceAccountPath     string
	clusterRolePath        string
	crdsPath               string
	clusterRoleBindingPath string
	deploymentPath         string
	csiServicePath         string
	csiDaemonSetPath       string
	podSecurityPolicyPath  string
	setupYAMLPaths         []string

	appLabel      string
	appLabelKey   string
	appLabelValue string

	dns1123LabelRegex  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	dns1123DomainRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

	CRDnames = []string{
		BackendCRDName,
		NodeCRDName,
		StorageClassCRDName,
		TransactionCRDName,
		VersionCRDName,
		VolumeCRDName,
		SnapshotCRDName,
	}
)

func init() {
	RootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&generateYAML, "generate-custom-yaml", false, "Generate YAML files, but don't install anything.")
	installCmd.Flags().BoolVar(&useYAML, "use-custom-yaml", false, "Use any existing YAML files that exist in setup directory.")
	installCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during installation.")
	installCmd.Flags().BoolVar(&csi, "csi", false, "Install CSI Trident (override for Kubernetes 1.13 only, requires feature gates).")
	installCmd.Flags().BoolVar(&inCluster, "in-cluster", false, "Run the installer as a pod in the cluster.")

	installCmd.Flags().StringVar(&pvcName, "pvc", DefaultPVCName, "The name of the legacy PVC used by Trident, will be migrated to CRDs.")
	installCmd.Flags().StringVar(&pvName, "pv", DefaultPVName, "The name of the legacy PV used by Trident, will be migrated to CRDs.")
	installCmd.Flags().StringVar(&tridentImage, "trident-image", "", "The Trident image to install.")
	installCmd.Flags().StringVar(&etcdImage, "etcd-image", "", "The etcd image to install.")
	installCmd.Flags().StringVar(&logFormat, "log-format", "text", "The Trident logging format (text, json).")

	installCmd.Flags().DurationVar(&k8sTimeout, "k8s-timeout", 180*time.Second, "The timeout for all Kubernetes operations.")
	installCmd.Flags().DurationVar(&migratorTimeout, "migrator-timeout", 300*time.Minute, "The timeout for etcd-to-CRD migration.")

	installCmd.Flags().MarkHidden("in-cluster")
	installCmd.Flags().MarkHidden("migrator-timeout")
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {

		initInstallerLogging()

		if err := discoverInstallationEnvironment(); err != nil {
			log.Fatalf("Install pre-checks failed; %v", err)
		}
		processInstallationArguments(cmd)
		if err := validateInstallationArguments(); err != nil {
			log.Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {

		if generateYAML {

			// Ensure the setup directory exists
			if err := ensureSetupDirExists(); err != nil {
				log.Fatalf("Could not find or create setup directory; %v", err)
			}

			// If generate-custom-yaml was specified, write the YAML files to the setup directory
			if csi {
				if err := prepareCSIYAMLFiles(); err != nil {
					log.Fatalf("YAML generation failed; %v", err)
				}
			} else {
				if err := prepareYAMLFiles(); err != nil {
					log.Fatalf("YAML generation failed; %v", err)
				}
			}
			log.WithField("setupPath", setupPath).Info("Wrote installation YAML files.")

		} else if inCluster {
			// Run the installer as a Kubernetes pod
			if err := installTridentInCluster(); err != nil {
				log.Fatalf("Install failed; %v.  Resolve the issue; use 'tridentctl uninstall' "+
					"to clean up; and try again.", err)
			}
		} else {
			// Run the installer directly using the Kubernetes client
			if err := installTrident(); err != nil {
				log.Fatalf("Install failed; %v.  Resolve the issue; use 'tridentctl uninstall' "+
					"to clean up; and try again.", err)
			}
		}
	},
}

// initInstallerLogging configures logging for Trident installation. Logs are written to stdout.
func initInstallerLogging() {

	// Installer logs to stdout only
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

// discoverInstallationEnvironment inspects the current environment and checks
// that everything looks good for Trident installation, but it makes no changes
// to the environment.
func discoverInstallationEnvironment() error {

	var err error

	OperatingMode = ModeInstall

	// Default deployment image to what Trident was built with
	if tridentImage == "" {
		tridentImage = tridentconfig.BuildImage
	}

	// Default deployment image to what etcd was built with
	if etcdImage == "" {
		etcdImage = tridentconfig.BuildEtcdImage
	} else if !strings.Contains(etcdImage, tridentconfig.BuildEtcdVersion) {
		log.Warningf("Trident was qualified with etcd %s. You appear to be using a different version.", tridentconfig.BuildEtcdVersion)
	}

	// Create the Kubernetes client
	if client, err = initClient(); err != nil {
		return fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// Prepare input file paths
	if err = prepareYAMLFilePaths(); err != nil {
		return err
	}

	// Infer installation namespace if not specified
	if TridentPodNamespace == "" {
		TridentPodNamespace = client.Namespace()

		// Warn the user if no namespace was specified and the current namespace isn't "trident"
		if client.Namespace() != PreferredNamespace {
			log.WithFields(log.Fields{
				"example": fmt.Sprintf("./tridentctl install -n %s", PreferredNamespace),
			}).Warning("For maximum security, we recommend running Trident in its own namespace.")
		}
	}

	// Direct all subsequent client commands to the chosen namespace
	client.SetNamespace(TridentPodNamespace)

	log.WithFields(log.Fields{
		"installationNamespace": TridentPodNamespace,
		"kubernetesVersion":     client.Version().String(),
	}).Debug("Validated installation environment.")

	return nil
}

func initClient() (k8sclient.Interface, error) {

	// Detect whether we are running inside a pod
	if namespaceBytes, err := ioutil.ReadFile(tridentconfig.TridentNamespaceFile); err == nil {

		// The namespace file exists, so we're in a pod.  Create an API-based client.
		kubeConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		namespace := string(namespaceBytes)

		log.WithField("namespace", namespace).Debug("Running in a pod, creating API-based client.")

		return k8sclient.NewKubeClient(kubeConfig, namespace, k8sTimeout)

	} else {

		// The namespace file didn't exist, so assume we're outside a pod.  Create a CLI-based client.
		log.Debug("Running outside a pod, creating CLI-based client.")

		return k8sclient.NewKubectlClient("", k8sTimeout)
	}
}

func processInstallationArguments(cmd *cobra.Command) {

	// Determine whether CSI or legacy Trident will be installed
	minOptionalCSIVersion := utils.MustParseSemantic(tridentconfig.KubernetesCSIVersionMinOptional)
	minForcedCSIVersion := utils.MustParseSemantic(tridentconfig.KubernetesCSIVersionMinForced)

	if client.ServerVersion().LessThan(minOptionalCSIVersion) {
		if csi {
			log.Warningf("CSI Trident requires Kubernetes %s or later, ignoring --csi switch.",
				minOptionalCSIVersion.ShortString())
			csi = false
		}
	} else if client.ServerVersion().LessThan(minForcedCSIVersion) {
		if csi {
			log.Warningf("CSI Trident supports Kubernetes %s, but multiple Kubernetes feature gates must "+
				"have been enabled.  See Trident documentation for details.", client.ServerVersion().ShortString())
		}
	} else {
		if csiFlag := cmd.Flag("csi"); csiFlag != nil {
			if csiFlag.Value.String() == "false" && csiFlag.Changed {
				log.Warningf("Only CSI Trident is supported on Kubernetes %s or later, ignoring --csi switch.",
					minForcedCSIVersion.ShortString())
			}
		}

		csi = true
	}

	if csi {
		appLabel = TridentCSILabel
		appLabelKey = TridentCSILabelKey
		appLabelValue = TridentCSILabelValue
	} else {
		appLabel = TridentLegacyLabel
		appLabelKey = TridentLegacyLabelKey
		appLabelValue = TridentLegacyLabelValue
	}
}

func validateInstallationArguments() error {

	labelFormat := "a DNS-1123 label must consist of lower case alphanumeric characters or '-', " +
		"and must start and end with an alphanumeric character"
	subdomainFormat := "a DNS-1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', " +
		"and must start and end with an alphanumeric character"

	if !dns1123LabelRegex.MatchString(TridentPodNamespace) {
		return fmt.Errorf("'%s' is not a valid namespace name; %s", TridentPodNamespace, labelFormat)
	}
	if !dns1123DomainRegex.MatchString(pvcName) {
		return fmt.Errorf("'%s' is not a valid PVC name; %s", pvcName, subdomainFormat)
	}
	if !dns1123DomainRegex.MatchString(pvName) {
		return fmt.Errorf("'%s' is not a valid PV name; %s", pvName, subdomainFormat)
	}

	switch logFormat {
	case "text", "json":
		break
	default:
		return fmt.Errorf("'%s' is not a valid log format", logFormat)
	}

	return nil
}

// prepareYAMLFilePaths sets up the absolute file paths to all files
func prepareYAMLFilePaths() error {

	var err error

	// Get directory of installer
	installerDirectoryPath, err = os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine installer working directory; %v", err)
	}

	setupPath = path.Join(installerDirectoryPath, "setup")
	namespacePath = path.Join(setupPath, NamespaceFilename)
	serviceAccountPath = path.Join(setupPath, ServiceAccountFilename)
	clusterRolePath = path.Join(setupPath, ClusterRoleFilename)
	clusterRoleBindingPath = path.Join(setupPath, ClusterRoleBindingFilename)
	crdsPath = path.Join(setupPath, CRDsFilename)
	deploymentPath = path.Join(setupPath, DeploymentFilename)
	csiServicePath = path.Join(setupPath, ServiceFilename)
	csiDaemonSetPath = path.Join(setupPath, DaemonSetFilename)
	podSecurityPolicyPath = path.Join(setupPath, PodSecurityPolicyFilename)

	setupYAMLPaths = []string{
		namespacePath, serviceAccountPath, clusterRolePath, clusterRoleBindingPath, crdsPath,
		deploymentPath, csiServicePath, csiDaemonSetPath, podSecurityPolicyPath,
	}

	return nil
}

func cleanYAMLFiles() {

	for _, filePath := range setupYAMLPaths {
		os.Remove(filePath)
	}
}

func prepareYAMLFiles() error {

	var err error

	cleanYAMLFiles()

	namespaceYAML := k8sclient.GetNamespaceYAML(TridentPodNamespace)
	if err = writeFile(namespacePath, namespaceYAML); err != nil {
		return fmt.Errorf("could not write namespace YAML file; %v", err)
	}

	serviceAccountYAML := k8sclient.GetServiceAccountYAML(false)
	if err = writeFile(serviceAccountPath, serviceAccountYAML); err != nil {
		return fmt.Errorf("could not write service account YAML file; %v", err)
	}

	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), false)
	if err = writeFile(clusterRolePath, clusterRoleYAML); err != nil {
		return fmt.Errorf("could not write cluster role YAML file; %v", err)
	}

	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(), false)
	if err = writeFile(clusterRoleBindingPath, clusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write cluster role binding YAML file; %v", err)
	}

	crdsYAML := k8sclient.GetCRDsYAML()
	if err = writeFile(crdsPath, crdsYAML); err != nil {
		return fmt.Errorf("could not write custom resource definition YAML file; %v", err)
	}

	deploymentYAML := k8sclient.GetDeploymentYAML(tridentImage, appLabelValue, logFormat, Debug)
	if err = writeFile(deploymentPath, deploymentYAML); err != nil {
		return fmt.Errorf("could not write deployment YAML file; %v", err)
	}

	return nil
}

func prepareCSIYAMLFiles() error {

	var err error

	cleanYAMLFiles()

	namespaceYAML := k8sclient.GetNamespaceYAML(TridentPodNamespace)
	if err = writeFile(namespacePath, namespaceYAML); err != nil {
		return fmt.Errorf("could not write namespace YAML file; %v", err)
	}

	serviceAccountYAML := k8sclient.GetServiceAccountYAML(true)
	if err = writeFile(serviceAccountPath, serviceAccountYAML); err != nil {
		return fmt.Errorf("could not write service account YAML file; %v", err)
	}

	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), true)
	if err = writeFile(clusterRolePath, clusterRoleYAML); err != nil {
		return fmt.Errorf("could not write cluster role YAML file; %v", err)
	}

	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(), true)
	if err = writeFile(clusterRoleBindingPath, clusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write cluster role binding YAML file; %v", err)
	}

	crdsYAML := k8sclient.GetCRDsYAML()
	if err = writeFile(crdsPath, crdsYAML); err != nil {
		return fmt.Errorf("could not write custom resource definition YAML file; %v", err)
	}

	serviceYAML := k8sclient.GetCSIServiceYAML(appLabelValue)
	if err = writeFile(csiServicePath, serviceYAML); err != nil {
		return fmt.Errorf("could not write service YAML file; %v", err)
	}

	deploymentYAML := k8sclient.GetCSIDeploymentYAML(
		tridentImage, appLabelValue, logFormat, Debug, client.ServerVersion())
	if err = writeFile(deploymentPath, deploymentYAML); err != nil {
		return fmt.Errorf("could not write deployment YAML file; %v", err)
	}

	daemonSetYAML := k8sclient.GetCSIDaemonSetYAML(
		tridentImage, TridentNodeLabelValue, logFormat, Debug, client.ServerVersion())
	if err = writeFile(csiDaemonSetPath, daemonSetYAML); err != nil {
		return fmt.Errorf("could not write daemonset YAML file; %v", err)
	}

	podSecurityPolicyYAML := k8sclient.GetPodSecurityPolicyYAML()
	if err = writeFile(podSecurityPolicyPath, podSecurityPolicyYAML); err != nil {
		return fmt.Errorf("could not write pod security policy YAML file; %v", err)
	}

	return nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func writeFile(filePath, data string) error {
	return ioutil.WriteFile(filePath, []byte(data), 0644)
}

func ensureSetupDirExists() error {
	if !fileExists(setupPath) {
		if err := os.MkdirAll(setupPath, os.ModePerm); err != nil {
			return err
		}
		log.WithField("path", setupPath).Info("Created setup directory.")
	}
	return nil
}

func installTrident() (returnError error) {

	var (
		logFields     log.Fields
		crdsExist     bool
		pvcExists     bool
		pvExists      bool
		migrateToCRDs bool
		crd           *apiextensionv1beta1.CustomResourceDefinition
	)

	// Ensure legacy Trident isn't already installed
	if installed, namespace, err := isTridentInstalled(); err != nil {
		return fmt.Errorf("could not check if Trident deployment exists; %v", err)
	} else if installed {
		return fmt.Errorf("trident is already installed in namespace %s", namespace)
	}

	// Ensure CSI Trident isn't already installed
	if installed, namespace, err := isCSITridentInstalled(); err != nil {
		return fmt.Errorf("could not check if CSI Trident deployment exists; %v", err)
	} else if installed {
		return fmt.Errorf("CSI Trident is already installed in namespace %s", namespace)
	}

	// Ensure preview CSI Trident isn't already installed
	if installed, namespace, err := isPreviewCSITridentInstalled(); err != nil {
		return fmt.Errorf("could not check if preview CSI Trident deployment exists; %v", err)
	} else if installed {
		return fmt.Errorf("CSI Trident is already installed in namespace %s", namespace)
	}

	// Check if the required namespace exists
	namespaceExists, returnError := client.CheckNamespaceExists(TridentPodNamespace)
	if returnError != nil {
		returnError = fmt.Errorf("could not check if namespace %s exists; %v", TridentPodNamespace, returnError)
		return
	}
	if namespaceExists {
		log.WithField("namespace", TridentPodNamespace).Debug("Namespace exists.")
	} else {
		log.WithField("namespace", TridentPodNamespace).Debug("Namespace does not exist.")
	}

	// Check for alpha snapshot CRDs
	crdNames := []string{
		"volumesnapshotclasses.snapshot.storage.k8s.io",
		"volumesnapshotcontents.snapshot.storage.k8s.io",
		"volumesnapshots.snapshot.storage.k8s.io",
	}

	for _, crdName := range crdNames {
		// See if CRD exists
		crdsExist, returnError = client.CheckCRDExists(crdName)
		if returnError != nil {
			return
		}
		if !crdsExist {
			log.WithField("CRD", crdName).Debug("CRD not present.")
			continue
		}

		// Get the CRD and check version
		crd, returnError = client.GetCRD(crdName)
		if returnError != nil {
			return
		}
		alphaSnapshotErrorString := "kubernetes snapshot beta feature is not backwards compatible; run `tridentctl" +
			" obliviate alpha-snapshot-crd` to remove previous kubernetes snapshot CRDs, " +
			"then retry installation; for details, please refer to Trident’s online documentation"
		if strings.ToLower(crd.Spec.Version) == "v1alpha1" {
			returnError = fmt.Errorf(alphaSnapshotErrorString)
			return
		}
		for _, version := range crd.Spec.Versions {
			if strings.ToLower(version.Name) == "v1alpha1" {
				returnError = fmt.Errorf(alphaSnapshotErrorString)
				return
			}
		}
	}

	// Discover CRD data
	crdsExist, returnError = client.CheckCRDExists(VersionCRDName)
	if returnError != nil {
		return
	}

	if crdsExist {
		log.Debug("Trident CRDs present, skipping PVC/PV check.")
	} else {

		// We didn't find any CRD data, so look for legacy etcd data
		pvcExists, pvExists, returnError = discoverLegacyEtcdData()
		if returnError != nil {
			return
		}

		migrateToCRDs = false

		logFields = log.Fields{"pv": pvName, "pvc": pvcName}

		if pvcExists && pvExists {
			log.WithFields(logFields).Debug("PV and PVC exist, installer will migrate etcd data to CRDs.")
			migrateToCRDs = true
		} else if !pvcExists && !pvExists {
			log.WithFields(logFields).Debug("PV and PVC do not exist, installer will create a fresh " +
				"CRD-based deployment.")
		} else if pvcExists && !pvExists {
			log.WithFields(logFields).Error("PVC exists but PV does not.")
			returnError = fmt.Errorf("PVC %s exists but PV %s does not; if you have data from a previous "+
				"Trident installation, please use the installer from that version to recreate the missing PV, "+
				"else delete the PVC and try again", pvcName, pvName)
			return
		} else if !pvcExists && pvExists {
			log.WithFields(logFields).Error("PV exists but PVC does not.")
			returnError = fmt.Errorf("PV %s exists but PVC %s does not; if you have data from a previous "+
				"Trident installation, please use the installer from that version to recreate the missing PVC, "+
				"else delete the PV and try again", pvName, pvcName)
			return
		}
	}

	// All checks succeeded, so proceed with installation
	log.WithField("namespace", TridentPodNamespace).Info("Starting Trident installation.")

	// Create namespace if it doesn't exist
	if !namespaceExists {
		returnError = createNamespace()
		if returnError != nil {
			return
		}
	}

	// Remove any RBAC objects from a previous Trident installation
	if anyCleanupErrors := removeRBACObjects(log.DebugLevel); anyCleanupErrors {
		returnError = fmt.Errorf("could not remove one or more previous Trident artifacts; " +
			"please delete them manually and try again")
		return
	}

	// Create the RBAC objects
	if returnError = createRBACObjects(); returnError != nil {
		return
	}

	// Create the CRDs and wait for them to be registered in Kubernetes
	crdsCreated := false
	if !crdsExist {
		if returnError = createCustomResourceDefinitions(); returnError != nil {
			return
		}
		if returnError = ensureCRDsRegistered(k8sclient.GetCRDNames()); returnError != nil {
			return
		}
		crdsCreated = true
	}

	// Do the data migration if necessary
	if migrateToCRDs {
		if returnError = runTridentMigrator(); returnError != nil {
			// If migration failed *and* we created the CRDs, clean up by deleting the CRDs
			if crdsCreated {
				deleteCustomResourceDefinitions()
			}
			return
		}
	}

	// Patch the CRD definitions with finalizers to protect them
	if returnError = protectCustomResourceDefinitions(); returnError != nil {
		return
	}

	if !csi {

		// Create the deployment
		if useYAML && fileExists(deploymentPath) {
			returnError = validateTridentDeployment()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the deployment YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(deploymentPath)
			logFields = log.Fields{"path": deploymentPath}
		} else {
			returnError = client.CreateObjectByYAML(k8sclient.GetDeploymentYAML(
				tridentImage, appLabelValue, logFormat, Debug))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident deployment; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident deployment.")

	} else {

		// Create pod security policy
		if useYAML && fileExists(podSecurityPolicyPath) {
			returnError = validateTridentPodSecurityPolicy()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the pod security policy YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(podSecurityPolicyPath)
			logFields = log.Fields{"path": podSecurityPolicyPath}
		} else {
			returnError = client.CreateObjectByYAML(k8sclient.GetPodSecurityPolicyYAML())
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident pod security policy; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident pod security policy.")

		// Create the CSI CRDs if necessary (1.13 only)
		returnError = createK8S113CSICustomResourceDefinitions()
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Kubernetes 1.13 CSI CRDs; %v", returnError)
			return
		}

		// Create the CSI Driver object if necessary (1.14+)
		returnError = createK8SCSIDriver()
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Kubernetes CSI Driver object; %v", returnError)
			return
		}

		// Create the service
		if useYAML && fileExists(csiServicePath) {
			returnError = validateTridentService()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the service YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(csiServicePath)
			logFields = log.Fields{"path": csiServicePath}
		} else {
			returnError = client.CreateObjectByYAML(k8sclient.GetCSIServiceYAML(appLabelValue))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident service; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident service.")

		// Create the certificates for the CSI controller's HTTPS REST interface
		certInfo, err := utils.MakeHTTPCertInfo(
			tridentconfig.CACertName, tridentconfig.ServerCertName, tridentconfig.ClientCertName)
		if err != nil {
			returnError = fmt.Errorf("could not create Trident X509 certificates; %v", err)
			return
		}

		// Create the secret for the HTTP certs & keys
		secretMap := map[string]string{
			tridentconfig.CAKeyFile:      certInfo.CAKey,
			tridentconfig.CACertFile:     certInfo.CACert,
			tridentconfig.ServerKeyFile:  certInfo.ServerKey,
			tridentconfig.ServerCertFile: certInfo.ServerCert,
			tridentconfig.ClientKeyFile:  certInfo.ClientKey,
			tridentconfig.ClientCertFile: certInfo.ClientCert,
		}
		err = client.CreateObjectByYAML(
			k8sclient.GetSecretYAML("trident-csi", TridentPodNamespace, appLabelValue, secretMap, nil))
		if err != nil {
			returnError = fmt.Errorf("could not create Trident secret; %v", err)
			return
		}
		log.WithFields(logFields).Info("Created Trident secret.")

		// Create the deployment
		if useYAML && fileExists(deploymentPath) {
			returnError = validateTridentDeployment()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the deployment YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(deploymentPath)
			logFields = log.Fields{"path": deploymentPath}
		} else {
			returnError = client.CreateObjectByYAML(
				k8sclient.GetCSIDeploymentYAML(tridentImage, appLabelValue, logFormat, Debug, client.ServerVersion()))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident deployment; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident deployment.")

		// Create the daemonset
		if useYAML && fileExists(csiDaemonSetPath) {
			returnError = validateTridentDaemonSet()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the daemonset YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(csiDaemonSetPath)
			logFields = log.Fields{"path": csiDaemonSetPath}
		} else {
			returnError = client.CreateObjectByYAML(
				k8sclient.GetCSIDaemonSetYAML(
					tridentImage, TridentNodeLabelValue, logFormat, Debug, client.ServerVersion()))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident daemonset; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident daemonset.")
	}

	// Wait for Trident pod to be running
	var tridentPod *v1.Pod

	tridentPod, returnError = waitForTridentPod()
	if returnError != nil {
		return
	}

	// Wait for Trident REST interface to be available
	TridentPodName = tridentPod.Name
	returnError = waitForRESTInterface()
	if returnError != nil {
		returnError = fmt.Errorf("%v; use 'tridentctl logs' to learn more", returnError)
		return
	}

	log.Info("Trident installation succeeded.")
	return nil
}

func discoverLegacyEtcdData() (pvcExists bool, pvExists bool, returnError error) {

	var (
		pvc *v1.PersistentVolumeClaim
		pv  *v1.PersistentVolume
	)

	// Check for PVC (also returns (false, nil) if namespace does not exist)
	pvcExists, returnError = client.CheckPVCExists(pvcName)
	if returnError != nil {
		returnError = fmt.Errorf("could not establish the presence of PVC %s; %v", pvcName, returnError)
		return
	}
	if pvcExists {
		pvc, returnError = client.GetPVC(pvcName)
		if returnError != nil {
			returnError = fmt.Errorf("could not retrieve PVC %s; %v", pvcName, returnError)
			return
		}

		// Ensure that the PVC is in a state that we can work with
		if pvc.Status.Phase == v1.ClaimLost {
			returnError = fmt.Errorf("PVC %s phase is Lost; please delete it and try again", pvcName)
			return
		}
		if pvc.Status.Phase == v1.ClaimBound && pvc.Spec.VolumeName != pvName {
			returnError = fmt.Errorf("PVC %s is Bound to PV %s, does not match expected PV name %s; "+
				"please specify a different PV and/or PVC", pvcName, pvc.Spec.VolumeName, pvName)
			return
		}
		if pvc.Labels == nil || pvc.Labels[TridentLegacyLabelKey] != TridentLegacyLabelValue {
			returnError = fmt.Errorf("PVC %s does not have %s label; "+
				"please add label or delete PVC and try again", pvcName, TridentLegacyLabel)
			return
		}

		log.WithFields(log.Fields{
			"pvc":       pvcName,
			"namespace": pvc.Namespace,
			"phase":     pvc.Status.Phase,
		}).Debug("PVC already exists.")

	} else {
		log.WithField("pvc", pvcName).Debug("PVC does not exist.")
	}

	// Check for PV
	pvExists, returnError = client.CheckPVExists(pvName)
	if returnError != nil {
		returnError = fmt.Errorf("could not establish the presence of PV %s; %v", pvName, returnError)
		return
	}
	if pvExists {
		pv, returnError = client.GetPV(pvName)
		if returnError != nil {
			returnError = fmt.Errorf("could not retrieve PV %s; %v", pvName, returnError)
			return
		}

		// Ensure that the PV is in a state we can work with
		if pv.Status.Phase == v1.VolumeReleased {
			returnError = fmt.Errorf("PV %s phase is Released; please delete it and try again", pvName)
			return
		}
		if pv.Status.Phase == v1.VolumeFailed {
			returnError = fmt.Errorf("PV %s phase is Failed; please delete it and try again", pvName)
			return
		}
		if pv.Status.Phase == v1.VolumeBound && pv.Spec.ClaimRef != nil {
			if pv.Spec.ClaimRef.Name != pvcName {
				returnError = fmt.Errorf("PV %s is Bound to PVC %s, does not match expected PVC name %s; "+
					"please specify a different PV and/or PVC", pvName, pv.Spec.ClaimRef.Name, pvcName)
				return
			}
			if pv.Spec.ClaimRef.Namespace != TridentPodNamespace {
				returnError = fmt.Errorf("PV %s is Bound to a PVC in namespace %s; "+
					"please delete PV and try again", pvName, pv.Spec.ClaimRef.Namespace)
				return
			}
		}
		if pv.Labels == nil || pv.Labels[TridentLegacyLabelKey] != TridentLegacyLabelValue {
			returnError = fmt.Errorf("PV %s does not have %s label; "+
				"please add label or delete PV and try again", pvName, TridentLegacyLabel)
			return
		}

		log.WithFields(log.Fields{
			"pv":    pvName,
			"phase": pv.Status.Phase,
		}).Debug("PV already exists.")

	} else {
		log.WithField("pv", pvName).Debug("PV does not exist.")
	}

	return
}

func createNamespace() (returnError error) {

	var logFields log.Fields

	if useYAML && fileExists(namespacePath) {
		returnError = client.CreateObjectByFile(namespacePath)
		logFields = log.Fields{"path": namespacePath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetNamespaceYAML(TridentPodNamespace))
		logFields = log.Fields{"namespace": TridentPodNamespace}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create namespace %s; %v", TridentPodNamespace, returnError)
		return
	}
	log.WithFields(logFields).Info("Created namespace.")
	return nil
}

func createCustomResourceDefinitions() (returnError error) {

	var logFields log.Fields

	if useYAML && fileExists(crdsPath) {
		returnError = client.CreateObjectByFile(crdsPath)
		logFields = log.Fields{"path": crdsPath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetCRDsYAML())
		logFields = log.Fields{"namespace": TridentPodNamespace}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create custom resource definitions in %s; %v",
			TridentPodNamespace, returnError)
		return
	}
	log.WithFields(logFields).Info("Created custom resource definitions.")
	return nil
}

func ensureCRDsRegistered(crdNames []string) error {

	for _, crdName := range crdNames {
		if err := ensureCRDRegistered(crdName); err != nil {
			return err
		}
	}

	return nil
}

// ensureCRDRegistered waits until a CRD is known to Kubernetes.
func ensureCRDRegistered(crdName string) error {

	checkCRDRegistered := func() error {
		if exists, err := client.CheckCRDExists(crdName); err != nil {
			return err
		} else if !exists {
			return errors.New("CRD not registered")
		}
		return nil
	}

	checkCRDNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"CRD": crdName,
			"err": err,
		}).Debug("CRD not registered, waiting.")
	}

	checkCRDBackoff := backoff.NewExponentialBackOff()
	checkCRDBackoff.MaxInterval = 5 * time.Second
	checkCRDBackoff.MaxElapsedTime = k8sTimeout

	log.WithField("CRD", crdName).Trace("Waiting for CRD to be registered.")

	if err := backoff.RetryNotify(checkCRDRegistered, checkCRDBackoff, checkCRDNotify); err != nil {
		return fmt.Errorf("CRD was not registered after %3.2f seconds", k8sTimeout.Seconds())
	}

	log.WithField("CRD", crdName).Debug("CRD registered.")
	return nil
}

func deleteCustomResourceDefinitions() (returnError error) {

	var logFields log.Fields

	if useYAML && fileExists(crdsPath) {
		returnError = client.DeleteObjectByFile(crdsPath, false)
		logFields = log.Fields{"path": crdsPath}
	} else {
		returnError = client.DeleteObjectByYAML(k8sclient.GetCRDsYAML(), false)
		logFields = log.Fields{"namespace": TridentPodNamespace}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not delete custom resource definitions in %s; %v",
			TridentPodNamespace, returnError)
		return
	}
	log.WithFields(logFields).Info("Deleted custom resource definitions.")
	return nil
}

// protectCustomResourceDefinitions adds finalizers to the CRD definitions to prevent accidental deletion
func protectCustomResourceDefinitions() error {
	for _, crdName := range CRDnames {
		err := client.AddFinalizerToCRD(crdName)
		if err != nil {
			return err
		}
	}
	log.Info("Added finalizers to custom resource definitions.")
	return nil
}

func createK8S113CSICustomResourceDefinitions() error {

	// We only have to create these CRDs on Kubernetes 1.13
	if client.ServerVersion().MajorVersion() != 1 || client.ServerVersion().MinorVersion() != 13 {
		return nil
	}

	csiDriversCRDExists, err := client.CheckCRDExists("csidrivers.csi.storage.k8s.io")
	if err != nil {
		return fmt.Errorf("could not check if CRD csidrivers.csi.storage.k8s.io exists; %v", err)
	} else if !csiDriversCRDExists {
		if err = client.CreateObjectByYAML(k8sclient.GetCSIDriverCRDYAML()); err != nil {
			return fmt.Errorf("could not create CRD csidrivers.csi.storage.k8s.io; %v", err)
		}
	}

	csiNodeInfosCRDExists, err := client.CheckCRDExists("csinodeinfos.csi.storage.k8s.io")
	if err != nil {
		return fmt.Errorf("could not check if CRD csinodeinfos.csi.storage.k8s.io exists; %v", err)
	} else if !csiNodeInfosCRDExists {
		if err = client.CreateObjectByYAML(k8sclient.GetCSINodeInfoCRDYAML()); err != nil {
			return fmt.Errorf("could not create CRD csinodeinfos.csi.storage.k8s.io; %v", err)
		}
	}

	return nil
}

func createK8SCSIDriver() error {

	// We only have to create this object on Kubernetes 1.14+
	if client.ServerVersion().MajorVersion() != 1 || client.ServerVersion().MinorVersion() < 14 {
		return nil
	}

	// Delete the object in case it already exists and we need to update it
	if err := client.DeleteObjectByYAML(k8sclient.GetCSIDriverCRYAML(), true); err != nil {
		return fmt.Errorf("could not delete csidriver custom resource; %v", err)
	}

	if err := client.CreateObjectByYAML(k8sclient.GetCSIDriverCRYAML()); err != nil {
		return fmt.Errorf("could not create csidriver custom resource; %v", err)
	}

	return nil
}

func createRBACObjects() (returnError error) {

	var logFields log.Fields

	// Create service account
	if useYAML && fileExists(serviceAccountPath) {
		returnError = client.CreateObjectByFile(serviceAccountPath)
		logFields = log.Fields{"path": serviceAccountPath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetServiceAccountYAML(csi))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create service account; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created service account.")

	// Create cluster role
	if useYAML && fileExists(clusterRolePath) {
		returnError = client.CreateObjectByFile(clusterRolePath)
		logFields = log.Fields{"path": clusterRolePath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetClusterRoleYAML(client.Flavor(), csi))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create cluster role; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created cluster role.")

	// Create cluster role binding
	if useYAML && fileExists(clusterRoleBindingPath) {
		returnError = client.CreateObjectByFile(clusterRoleBindingPath)
		logFields = log.Fields{"path": clusterRoleBindingPath}
	} else {
		returnError = client.CreateObjectByYAML(
			k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(), csi))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create cluster role binding; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created cluster role binding.")

	// If OpenShift, add Trident to security context constraint(s)
	if client.Flavor() == k8sclient.FlavorOpenShift {
		if csi {
			if returnError = client.AddTridentUserToOpenShiftSCC("trident-csi", "privileged"); returnError != nil {
				returnError = fmt.Errorf("could not modify security context constraint; %v", returnError)
				return
			}
			log.WithFields(log.Fields{
				"scc":  "privileged",
				"user": "trident-csi",
			}).Info("Added security context constraint user.")
		} else {
			if returnError = client.AddTridentUserToOpenShiftSCC("trident", "anyuid"); returnError != nil {
				returnError = fmt.Errorf("could not modify security context constraint; %v", returnError)
				return
			}
			log.WithFields(log.Fields{
				"scc":  "anyuid",
				"user": "trident",
			}).Info("Added security context constraint user.")
		}
	}

	return
}

func removeRBACObjects(logLevel log.Level) (anyErrors bool) {

	logFunc := func(fields log.Fields) func(args ...interface{}) {
		if logLevel == log.DebugLevel {
			return log.WithFields(fields).Debug
		} else {
			return log.WithFields(fields).Info
		}
	}

	// Delete cluster role binding
	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(), csi)
	if err := client.DeleteObjectByYAML(clusterRoleBindingYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete cluster role binding.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted cluster role binding.")
	}

	// Delete cluster role
	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), csi)
	if err := client.DeleteObjectByYAML(clusterRoleYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete cluster role.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted cluster role.")
	}

	// Delete service account
	serviceAccountYAML := k8sclient.GetServiceAccountYAML(csi)
	if err := client.DeleteObjectByYAML(serviceAccountYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete service account.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted service account.")
	}

	// If OpenShift, remove Trident from security context constraint(s)
	if client.Flavor() == k8sclient.FlavorOpenShift {
		if csi {
			if err := client.RemoveTridentUserFromOpenShiftSCC("trident-csi", "privileged"); err != nil {
				log.WithField("error", err).Warning("Could not modify security context constraint.")
				anyErrors = true
			} else {
				logFunc(log.Fields{
					"scc":  "privileged",
					"user": "trident-csi",
				})("Removed security context constraint user.")
			}
		} else {
			if err := client.RemoveTridentUserFromOpenShiftSCC("trident", "anyuid"); err != nil {
				log.WithField("error", err).Warning("Could not modify security context constraint.")
				anyErrors = true
			} else {
				logFunc(log.Fields{
					"scc":  "anyuid",
					"user": "trident",
				})("Removed security context constraint user.")
			}
		}
	}

	return
}

func validateTridentDeployment() error {

	deployment, err := readDeploymentFromFile(deploymentPath)
	if err != nil {
		return fmt.Errorf("could not load deployment YAML file; %v", err)
	}

	// Check the deployment label
	labels := deployment.Labels
	if labels[appLabelKey] != appLabelValue {
		return fmt.Errorf("the Trident deployment must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	// Check the pod label
	labels = deployment.Spec.Template.Labels
	if labels[appLabelKey] != appLabelValue {
		return fmt.Errorf("the Trident deployment's pod template must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	tridentImage := ""
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == tridentconfig.ContainerTrident {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return fmt.Errorf("the Trident deployment must define the %s container", tridentconfig.ContainerTrident)
	}

	return nil
}

func validateTridentPodSecurityPolicy() error {

	securityPolicy, err := readPodSecurityPolicyFromFile(podSecurityPolicyPath)
	if err != nil {
		return fmt.Errorf("could not load pod security policy YAML file; %v", err)
	}

	// Check the security settings
	spec := securityPolicy.Spec
	if !spec.Privileged {
		return fmt.Errorf("trident's pod security policy must allow privileged pods")
	}
	if spec.AllowPrivilegeEscalation != nil && !*spec.AllowPrivilegeEscalation {
		return fmt.Errorf("trident's pod security policy must allow privilege escalation")
	}
	if !spec.HostIPC {
		return fmt.Errorf("trident's pod security policy must allow hostIPC")
	}
	if !spec.HostNetwork {
		return fmt.Errorf("trident's pod security policy must allow hostNetwork")
	}
	found := false
	for _, allowedCap := range spec.AllowedCapabilities {
		if allowedCap == "SYS_ADMIN" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("trident's pod security policy must allow SYS_ADMIN capability")
	}
	return nil
}

func validateTridentService() error {

	service, err := readServiceFromFile(csiServicePath)
	if err != nil {
		return fmt.Errorf("could not load service YAML file; %v", err)
	}

	// Check the service label
	labels := service.Labels
	if labels[appLabelKey] != appLabelValue {
		return fmt.Errorf("the Trident service must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	return nil
}

func validateTridentDaemonSet() error {

	daemonset, err := readDaemonSetFromFile(csiDaemonSetPath)
	if err != nil {
		return fmt.Errorf("could not load daemonset YAML file; %v", err)
	}

	// Check the daemonset label
	labels := daemonset.Labels
	if labels[TridentNodeLabelKey] != TridentNodeLabelValue {
		return fmt.Errorf("the Trident daemonset must have the label \"%s: %s\"",
			TridentNodeLabelKey, TridentNodeLabelValue)
	}

	// Check the pod label
	labels = daemonset.Spec.Template.Labels
	if labels[TridentNodeLabelKey] != TridentNodeLabelValue {
		return fmt.Errorf("the Trident daemonset's pod template must have the label \"%s: %s\"",
			TridentNodeLabelKey, TridentNodeLabelValue)
	}

	tridentImage := ""
	for _, container := range daemonset.Spec.Template.Spec.Containers {
		if container.Name == tridentconfig.ContainerTrident {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return fmt.Errorf("the Trident daemonset must define the %s container", tridentconfig.ContainerTrident)
	}

	return nil
}

func waitForTridentPod() (*v1.Pod, error) {

	var pod *v1.Pod

	checkPodRunning := func() error {
		var podError error
		pod, podError = client.GetPodByLabel(appLabel, false)
		if podError != nil || pod.Status.Phase != v1.PodRunning {
			return errors.New("pod not running")
		}
		return nil
	}
	podNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
		}).Debugf("Trident pod not yet running, waiting.")
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	log.Info("Waiting for Trident pod to start.")

	if err := backoff.RetryNotify(checkPodRunning, podBackoff, podNotify); err != nil {

		// Build up an error message with as much detail as available.
		var errMessages []string
		errMessages = append(errMessages,
			fmt.Sprintf("Trident pod was not running after %3.2f seconds.", k8sTimeout.Seconds()))

		if pod != nil {
			if pod.Status.Phase != "" {
				errMessages = append(errMessages, fmt.Sprintf("Pod status is %s.", pod.Status.Phase))
				if pod.Status.Message != "" {
					errMessages = append(errMessages, fmt.Sprintf("%s", pod.Status.Message))
				}
			}
			errMessages = append(errMessages,
				fmt.Sprintf("Use '%s describe pod %s -n %s' for more information.",
					client.CLI(), pod.Name, client.Namespace()))
		}

		log.Error(strings.Join(errMessages, " "))
		return nil, err
	}

	log.WithFields(log.Fields{
		"pod":       pod.Name,
		"namespace": TridentPodNamespace,
	}).Info("Trident pod started.")

	return pod, nil
}

func waitForRESTInterface() error {

	var version string

	checkRESTInterface := func() error {

		cliCommand := []string{"tridentctl", "-s", PodServer, "version", "-o", "json"}
		versionJSON, err := client.Exec(TridentPodName, tridentconfig.ContainerTrident, cliCommand)
		if err != nil {
			if versionJSON != nil && len(versionJSON) > 0 {
				err = fmt.Errorf("%v; %s", err, strings.TrimSpace(string(versionJSON)))
			}
			return err
		}

		var versionResponse api.VersionResponse
		err = json.Unmarshal(versionJSON, &versionResponse)
		if err != nil {
			return err
		}

		version = versionResponse.Server.Version
		return nil
	}
	restNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
		}).Debugf("REST interface not yet up, waiting.")
	}
	restBackoff := backoff.NewExponentialBackOff()
	restBackoff.MaxElapsedTime = k8sTimeout

	log.Info("Waiting for Trident REST interface.")

	if err := backoff.RetryNotify(checkRESTInterface, restBackoff, restNotify); err != nil {
		log.Errorf("Trident REST interface was not available after %3.2f seconds.", k8sTimeout.Seconds())
		return err
	}

	log.WithField("version", version).Info("Trident REST interface is up.")

	return nil
}

// readDeploymentFromFile parses and returns a deployment object from a file.
func readDeploymentFromFile(filePath string) (*appsv1.Deployment, error) {

	var deployment appsv1.Deployment

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &deployment)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// readServiceFromFile parses and returns a service object from a file.
func readPodSecurityPolicyFromFile(filePath string) (*policy.PodSecurityPolicy, error) {

	var securityPolicy policy.PodSecurityPolicy

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &securityPolicy)
	if err != nil {
		return nil, err
	}
	return &securityPolicy, nil
}

// readServiceFromFile parses and returns a service object from a file.
func readServiceFromFile(filePath string) (*v1.Service, error) {

	var service v1.Service

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &service)
	if err != nil {
		return nil, err
	}
	return &service, nil
}

// readDaemonSetFromFile parses and returns a daemonset object from a file.
func readDaemonSetFromFile(filePath string) (*appsv1.DaemonSet, error) {

	var daemonset appsv1.DaemonSet

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &daemonset)
	if err != nil {
		return nil, err
	}
	return &daemonset, nil
}

func installTridentInCluster() (returnError error) {

	// Ensure Trident installer pod isn't already present
	if podPresent, namespace, err := client.CheckPodExistsByLabel(TridentInstallerLabel, true); err != nil {
		return fmt.Errorf("could not check if Trident installer pod is present; %v", err)
	} else if podPresent {
		return fmt.Errorf("trident installer pod 'trident-installer' is already present in namespace %s; "+
			"please remove it manually and try again", namespace)
	}

	// Ensure Trident installer configmap isn't already present
	if configmapPresent, namespace, err := client.CheckConfigMapExistsByLabel(TridentInstallerLabel, true); err != nil {
		return fmt.Errorf("could not check if Trident installer configmap is present; %v", err)
	} else if configmapPresent {
		if err := client.DeleteConfigMapByLabel(TridentInstallerLabel); err != nil {
			log.WithFields(log.Fields{
				"configmap": "trident-installer",
				"namespace": namespace,
			}).Error("Could not delete configmap; please delete it manually.")
		}
	}

	// Ensure the required namespace exists
	namespaceExists, returnError := client.CheckNamespaceExists(TridentPodNamespace)
	if returnError != nil {
		returnError = fmt.Errorf("could not check if namespace %s exists; %v", TridentPodNamespace, returnError)
		return
	}
	if namespaceExists {
		log.WithField("namespace", TridentPodNamespace).Debug("Namespace exists.")
	} else {
		log.WithField("namespace", TridentPodNamespace).Debug("Namespace does not exist.")

		returnError = createNamespace()
		if returnError != nil {
			return
		}
	}

	// Remove any RBAC objects from a previous Trident installation
	if anyCleanupErrors := removeInstallerRBACObjects(log.DebugLevel); anyCleanupErrors {
		returnError = fmt.Errorf("could not remove one or more previous Trident installer artifacts; " +
			"please delete them manually and try again")
		return
	}

	if csi {
		// Delete any previous installer pod security policy
		podSecurityPolicyYAML := k8sclient.GetInstallerSecurityPolicyYAML()
		if err := client.DeleteObjectByYAML(podSecurityPolicyYAML, true); err != nil {
			log.WithField("error", err).Errorf("Could not delete installer pod security policy; " +
				"please delete it manually.")
		} else {
			log.WithField("podSecurityPolicy", "tridentinstaller").Info(
				"Deleted previous installer pod security policy.")
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

	// Create the configmap
	if fileExists(setupPath) {
		if returnError = client.CreateConfigMapFromDirectory(
			setupPath, "trident-installer", TridentInstallerLabel); returnError != nil {
			return
		}
	} else {
		emptyConfigMapYAML := k8sclient.GetEmptyConfigMapYAML(
			TridentInstallerLabelValue, "trident-installer", TridentPodNamespace)
		if returnError = client.CreateObjectByYAML(emptyConfigMapYAML); returnError != nil {
			return
		}
	}
	log.WithFields(log.Fields{"configmap": "trident-installer"}).Info("Created installer configmap.")

	// Create the installer arguments
	commandArgs := []string{
		"tridentctl", "install",
		"--k8s-timeout", k8sTimeout.String(),
		"--namespace", TridentPodNamespace,
	}
	if Debug {
		commandArgs = append(commandArgs, "--debug")
	}
	if useYAML {
		commandArgs = append(commandArgs, "--use-custom-yaml")
	}
	if silent {
		commandArgs = append(commandArgs, "--silent")
	}
	if csi {
		commandArgs = append(commandArgs, "--csi")
	}
	if pvcName != "" {
		commandArgs = append(commandArgs, "--pvc")
		commandArgs = append(commandArgs, pvcName)
	}
	if pvName != "" {
		commandArgs = append(commandArgs, "--pv")
		commandArgs = append(commandArgs, pvName)
	}
	if tridentImage != "" {
		commandArgs = append(commandArgs, "--trident-image")
		commandArgs = append(commandArgs, tridentImage)
	}
	if etcdImage != "" {
		commandArgs = append(commandArgs, "--etcd-image")
		commandArgs = append(commandArgs, etcdImage)
	}
	if logFormat != "" {
		commandArgs = append(commandArgs, "--log-format")
		commandArgs = append(commandArgs, logFormat)
	}
	commandArgs = append(commandArgs, "--in-cluster=false")

	// Create the install pod
	errMessage := "could not create installer pod"
	returnError = createObjectsByYAML("installerPod",
		k8sclient.GetInstallerPodYAML(TridentInstallerLabelValue, tridentImage, commandArgs), errMessage)
	if returnError != nil {
		return
	}
	log.WithFields(log.Fields{"pod": "trident-installer"}).Info("Created installer pod.")

	// Wait for Trident installation pod to start
	var installPod *v1.Pod
	installPod, returnError = waitForPodToStart(TridentInstallerLabel, "installer")
	if returnError != nil {
		return
	}

	// Wait for pod to finish & output logs
	client.FollowPodLogs(installPod.Name, "", installPod.Namespace, logLogFmtMessage)

	installPod, returnError = waitForPodToFinish(TridentInstallerLabel, "installer")
	if returnError != nil {
		return
	}

	// Clean up the pod if it succeeded, otherwise leave it around for inspection
	if installPod.Status.Phase == v1.PodSucceeded {

		if returnError = client.DeletePodByLabel(TridentInstallerLabel); returnError != nil {
			log.WithFields(log.Fields{"pod": "trident-installer"}).Error("Could not delete installer pod;" +
				"please delete it manually.")
		} else {
			log.WithFields(log.Fields{"pod": "trident-installer"}).Info("Deleted installer pod.")
		}

		if returnError = client.DeleteConfigMapByLabel(TridentInstallerLabel); returnError != nil {
			log.WithFields(log.Fields{"configmap": "trident-installer"}).Error("Could not delete installer " +
				"configmap; please delete it manually.")
		} else {
			log.WithFields(log.Fields{"configmap": "trident-installer"}).Info("Deleted installer configmap.")
		}

	} else {

		log.WithFields(log.Fields{
			"pod": "trident-installer",
		}).Warningf("Installer pod status is %s. Use '%s describe pod %s -n %s' for more information.",
			installPod.Status.Phase, client.CLI(), installPod.Name, client.Namespace())
	}

	log.Info("In-cluster installation completed.")

	return
}

func runTridentMigrator() (returnError error) {

	// Ensure Trident migrator pod isn't already present
	if podPresent, namespace, err := client.CheckPodExistsByLabel(TridentMigratorLabel, true); err != nil {
		return fmt.Errorf("could not check if Trident migrator pod is present; %v", err)
	} else if podPresent {
		return fmt.Errorf("trident migrator pod 'trident-migrator' is already present in namespace %s; "+
			"please remove it manually and try again", namespace)
	}

	// Ensure the required namespace exists
	namespaceExists, returnError := client.CheckNamespaceExists(TridentPodNamespace)
	if returnError != nil {
		returnError = fmt.Errorf("could not check if namespace %s exists; %v", TridentPodNamespace, returnError)
		return
	}
	if namespaceExists {
		log.WithField("namespace", TridentPodNamespace).Debug("Namespace exists.")
	} else {
		log.WithField("namespace", TridentPodNamespace).Debug("Namespace does not exist.")

		returnError = createNamespace()
		if returnError != nil {
			return
		}
	}

	// Create the installer arguments
	commandArgs := []string{
		"tridentctl",
		"migrate",
		"--etcd-v3=http://127.0.0.1:8001",
	}
	if Debug {
		commandArgs = append(commandArgs, "--debug")
	}
	if silent {
		commandArgs = append(commandArgs, "--silent")
	}

	// Create the migrator pod
	errMessage := "could not create migrator pod"
	returnError = createObjectsByYAML("migratorPod",
		k8sclient.GetMigratorPodYAML(pvcName, tridentImage, etcdImage,
			TridentMigratorLabelValue, csi, commandArgs), errMessage)
	if returnError != nil {
		return
	}
	log.WithFields(log.Fields{"pod": "trident-migrator"}).Info("Created migrator pod.")

	// Wait for Trident migrator pod to start
	var migratorPod *v1.Pod
	migratorPod, returnError = waitForPodToStart(TridentMigratorLabel, "migrator")
	if returnError != nil {
		return
	}

	// Wait for pod to finish & output logs
	client.FollowPodLogs(migratorPod.Name, "trident-migrator", migratorPod.Namespace, logLogFmtMessage)

	migratorPod, returnError = waitForContainerToFinish(TridentMigratorLabel, "trident-migrator",
		"migrator", migratorTimeout)
	if returnError != nil {
		return
	}

	if returnError = client.DeletePodByLabel(TridentMigratorLabel); returnError != nil {
		log.WithFields(log.Fields{"pod": "trident-migrator"}).Error("Could not delete migrator pod;" +
			"please delete it manually.")
	} else {
		log.WithFields(log.Fields{"pod": "trident-migrator"}).Info("Deleted migrator pod.")
	}

	return
}

func createObjectsByYAML(objectName string, objectYAML string, errorMessage string) error {
	returnError := client.CreateObjectByYAML(objectYAML)
	if returnError != nil {
		log.WithFields(log.Fields{
			"objectName": objectName,
			"err":        returnError,
		}).Errorf("Object creation failed.")
		return errors.New(errorMessage)
	}
	return nil
}

func createInstallerRBACObjects() error {

	// Create service account
	returnError := client.CreateObjectByYAML(k8sclient.GetInstallerServiceAccountYAML())
	if returnError != nil {
		returnError = fmt.Errorf("could not create installer service account; %v", returnError)
		return returnError
	}
	log.WithFields(log.Fields{"serviceaccount": "trident-installer"}).Info("Created installer service account.")

	// Create cluster role
	errMessage := "could not create installer cluster role"
	returnError = createObjectsByYAML("clusterRole",
		k8sclient.GetInstallerClusterRoleYAML(client.Flavor()), errMessage)
	if returnError != nil {
		return returnError
	}
	log.WithFields(log.Fields{"clusterrole": "trident-installer"}).Info("Created installer cluster role.")

	// Create cluster role binding
	errMessage = "could not create installer cluster role binding"
	returnError = createObjectsByYAML("clusterRoleBinding",
		k8sclient.GetInstallerClusterRoleBindingYAML(TridentPodNamespace, client.Flavor()), errMessage)
	if returnError != nil {
		return returnError
	}
	log.WithFields(log.Fields{"clusterrolebinding": "trident-installer"}).Info("Created installer cluster role binding.")

	//If OpenShift, add Trident to security context constraint(s)
	if client.Flavor() == k8sclient.FlavorOpenShift {
		if returnError = client.AddTridentUserToOpenShiftSCC("trident-installer", "privileged"); returnError != nil {
			returnError = fmt.Errorf("could not modify security context constraint; %v", returnError)
			return returnError
		}
		log.WithFields(log.Fields{
			"scc":  "privileged",
			"user": "trident-installer",
		}).Info("Added security context constraint user.")
	}

	return nil
}

func removeInstallerRBACObjects(logLevel log.Level) (anyErrors bool) {

	logFunc := func(fields log.Fields) func(args ...interface{}) {
		if logLevel == log.DebugLevel {
			return log.WithFields(fields).Debug
		} else {
			return log.WithFields(fields).Info
		}
	}

	// Delete cluster role binding
	clusterRoleBindingYAML := k8sclient.GetInstallerClusterRoleBindingYAML(TridentPodNamespace, client.Flavor())
	if err := client.DeleteObjectByYAML(clusterRoleBindingYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete installer cluster role binding.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted installer cluster role binding.")
	}

	// Delete cluster role
	clusterRoleYAML := k8sclient.GetInstallerClusterRoleYAML(client.Flavor())
	if err := client.DeleteObjectByYAML(clusterRoleYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete installer cluster role.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted installer cluster role.")
	}

	// Delete service account
	serviceAccountYAML := k8sclient.GetInstallerServiceAccountYAML()
	if err := client.DeleteObjectByYAML(serviceAccountYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete installer service account.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted installer service account.")
	}

	// If OpenShift, remove Trident from security context constraint(s)
	if client.Flavor() == k8sclient.FlavorOpenShift {
		if err := client.RemoveTridentUserFromOpenShiftSCC("trident-installer", "privileged"); err != nil {
			log.WithField("error", err).Warning("Could not modify security context constraint.")
			anyErrors = true
		} else {
			logFunc(log.Fields{
				"scc":  "privileged",
				"user": "trident-installer",
			})("Removed security context constraint user.")
		}
	}

	return
}

func waitForPodToStart(label, purpose string) (*v1.Pod, error) {

	var pod *v1.Pod

	checkPodRunning := func() error {
		var podError error
		pod, podError = client.GetPodByLabel(label, false)
		if podError != nil {
			return fmt.Errorf("pod not yet started; %v", podError)
		}
		switch pod.Status.Phase {
		case v1.PodPending, v1.PodUnknown:
			return fmt.Errorf("pod not yet started (%s)", pod.Status.Phase)
		default:
			// Stop waiting if pod is any of Running, Succeeded, Failed, Unschedulable
			log.WithField("phase", pod.Status.Phase).Debug("Pod started.")
			return nil
		}
	}
	podNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Trident %s pod not yet started, waiting.", purpose)
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	log.Infof("Waiting for Trident %s pod to start.", purpose)

	if err := backoff.RetryNotify(checkPodRunning, podBackoff, podNotify); err != nil {

		// Build up an error message with as much detail as available.
		var errMessages []string
		errMessages = append(errMessages,
			fmt.Sprintf("Trident %s pod was not started after %3.2f seconds.", purpose, k8sTimeout.Seconds()))

		if pod != nil {
			if pod.Status.Phase != "" {
				errMessages = append(errMessages, fmt.Sprintf("Pod status is %s.", pod.Status.Phase))
				if pod.Status.Message != "" {
					errMessages = append(errMessages, fmt.Sprintf("%s", pod.Status.Message))
				}
			}
			errMessages = append(errMessages,
				fmt.Sprintf("Use '%s describe pod %s -n %s' for more information.",
					client.CLI(), pod.Name, client.Namespace()))
		}

		log.Error(strings.Join(errMessages, " "))
		return nil, err
	}

	log.WithFields(log.Fields{
		"pod":       pod.Name,
		"namespace": pod.Namespace,
	}).Infof("Trident %s pod started.", purpose)

	return pod, nil
}

func waitForPodToFinish(label, purpose string) (*v1.Pod, error) {

	var pod *v1.Pod

	checkPodFinished := func() error {
		var podError error
		pod, podError = client.GetPodByLabel(label, false)
		if podError != nil {
			return fmt.Errorf("pod not yet finished; %v", podError)
		}
		switch pod.Status.Phase {
		case v1.PodPending, v1.PodUnknown, v1.PodRunning:
			return fmt.Errorf("pod not yet finished (%s)", pod.Status.Phase)
		default:
			// Stop waiting if pod is any of Succeeded, Failed, Unschedulable
			log.WithField("phase", pod.Status.Phase).Debug("Pod finished.")
			return nil
		}
	}
	podNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Trident %s pod not yet finished, waiting.", purpose)
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	log.Infof("Waiting for Trident %s pod to finish.", purpose)

	if err := backoff.RetryNotify(checkPodFinished, podBackoff, podNotify); err != nil {

		// Build up an error message with as much detail as available.
		var errMessages []string
		errMessages = append(errMessages,
			fmt.Sprintf("Trident %s pod was not finished after %3.2f seconds.", purpose, k8sTimeout.Seconds()))

		if pod != nil {
			if pod.Status.Phase != "" {
				errMessages = append(errMessages, fmt.Sprintf("Pod status is %s.", pod.Status.Phase))
				if pod.Status.Message != "" {
					errMessages = append(errMessages, fmt.Sprintf("%s", pod.Status.Message))
				}
			}
			errMessages = append(errMessages,
				fmt.Sprintf("Use '%s describe pod %s -n %s' for more information.",
					client.CLI(), pod.Name, client.Namespace()))
		}

		log.Error(strings.Join(errMessages, " "))
		return nil, err
	}

	log.WithFields(log.Fields{
		"pod":       pod.Name,
		"namespace": pod.Namespace,
	}).Infof("Trident %s pod finished.", purpose)

	return pod, nil
}

func waitForContainerToFinish(podLabel, containerName, purpose string, timeout time.Duration) (*v1.Pod, error) {

	var pod *v1.Pod

	checkContainerFinished := func() error {
		var podError error
		pod, podError = client.GetPodByLabel(podLabel, false)
		if podError != nil {
			return fmt.Errorf("container not yet finished; %v", podError)
		}

		for _, c := range pod.Status.ContainerStatuses {
			if c.Name == containerName {
				if c.State.Terminated != nil {
					// Stop waiting if container is Terminated
					log.WithFields(log.Fields{
						"pod":       pod.Name,
						"container": c.Name,
						"exitCode":  c.State.Terminated.ExitCode,
					}).Debug("Container finished.")
					return nil
				} else {
					return errors.New("container not yet finished")
				}
			}
		}
		return errors.New("container not found")
	}
	containerNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Trident %s container not yet finished, waiting.", purpose)
	}
	containerBackoff := backoff.NewExponentialBackOff()
	containerBackoff.MaxElapsedTime = timeout

	log.Infof("Waiting for Trident %s pod to finish.", purpose)

	if err := backoff.RetryNotify(checkContainerFinished, containerBackoff, containerNotify); err != nil {

		// Build up an error message with as much detail as available.
		var errMessages []string
		errMessages = append(errMessages,
			fmt.Sprintf("Trident %s container was not finished after %3.2f seconds.", purpose, k8sTimeout.Seconds()))

		if pod != nil {
			if pod.Status.Phase != "" {
				errMessages = append(errMessages, fmt.Sprintf("Pod status is %s.", pod.Status.Phase))
				if pod.Status.Message != "" {
					errMessages = append(errMessages, fmt.Sprintf("%s", pod.Status.Message))
				}
			}
			errMessages = append(errMessages,
				fmt.Sprintf("Use '%s describe pod %s -n %s' for more information.",
					client.CLI(), pod.Name, client.Namespace()))
		}

		log.Error(strings.Join(errMessages, " "))
		return nil, err
	}

	log.WithFields(log.Fields{
		"pod":       pod.Name,
		"container": containerName,
		"namespace": pod.Namespace,
	}).Infof("Trident %s container finished.", purpose)

	return pod, nil
}

func parseLogFmtMessage(message string) map[string]string {

	fields := make(map[string]string)
	d := logfmt.NewDecoder(strings.NewReader(message))
	for d.ScanRecord() {
		for d.ScanKeyval() {
			fields[string(d.Key())] = string(d.Value())
		}
	}
	if d.Err() != nil {
		fields["msg"] = message
	}

	return fields
}

func logLogFmtMessage(message string) {

	fields := parseLogFmtMessage(message)

	delete(fields, "time")

	msg := fields["msg"]
	if msg == "" {
		return
	}
	delete(fields, "msg")

	level := fields["level"]
	if level == "" {
		level = "info"
	}
	delete(fields, "level")

	logFields := make(map[string]interface{})
	for k, v := range fields {
		logFields[k] = v
	}

	entry := log.WithFields(logFields)

	switch level {
	case "debug":
		entry.Debug(msg)
	case "info":
		entry.Info(msg)
	case "warning":
		entry.Warning(msg)
	case "error":
		entry.Error(msg)
	case "fatal":
		entry.Fatal(msg)
	case "panic":
		entry.Panic(msg)
	default:
		entry.Info(msg)
	}
}
