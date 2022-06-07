// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
)

const (
	PreferredNamespace = tridentconfig.OrchestratorName
	DefaultPVCName     = tridentconfig.OrchestratorName
	DefaultPVName      = tridentconfig.OrchestratorName

	// CRD names
	BackendConfigCRDName      = "tridentbackendconfigs.trident.netapp.io"
	BackendCRDName            = "tridentbackends.trident.netapp.io"
	MirrorRelationshipCRDName = "tridentmirrorrelationships.trident.netapp.io"
	NodeCRDName               = "tridentnodes.trident.netapp.io"
	SnapshotCRDName           = "tridentsnapshots.trident.netapp.io"
	SnapshotInfoCRDName       = "tridentsnapshotinfos.trident.netapp.io"
	StorageClassCRDName       = "tridentstorageclasses.trident.netapp.io"
	TransactionCRDName        = "tridenttransactions.trident.netapp.io"
	VersionCRDName            = "tridentversions.trident.netapp.io"
	VolumeCRDName             = "tridentvolumes.trident.netapp.io"
	VolumePublicationCRDName  = "tridentvolumepublications.trident.netapp.io"

	ClusterRoleBindingFilename = "trident-clusterrolebinding.yaml"
	ClusterRoleFilename        = "trident-clusterrole.yaml"
	CRDsFilename               = "trident-crds.yaml"
	DaemonSetFilename          = "trident-daemonset.yaml"
	DeploymentFilename         = "trident-deployment.yaml"
	NamespaceFilename          = "trident-namespace.yaml"
	PodSecurityPolicyFilename  = "trident-podsecuritypolicy.yaml"
	ServiceAccountFilename     = "trident-serviceaccount.yaml"
	ServiceFilename            = "trident-service.yaml"

	TridentEncryptionKeys = "trident-encryption-keys"

	TridentCSI           = "trident-csi"
	TridentLegacy        = "trident"
	TridentMainContainer = "trident-main"

	CSIDriver  = "csi.trident.netapp.io"
	TridentPSP = "tridentpods"
)

var (
	// CLI flags
	generateYAML            bool
	useYAML                 bool
	silent                  bool
	csi                     bool
	useIPv6                 bool
	silenceAutosupport      bool
	enableNodePrep          bool
	skipK8sVersionCheck     bool
	pvName                  string
	pvcName                 string
	tridentImage            string
	autosupportImage        string
	autosupportProxy        string
	autosupportCustomURL    string
	autosupportSerialNumber string
	autosupportHostname     string
	kubeletDir              string
	imageRegistry           string
	logFormat               string
	probePort               int64
	k8sTimeout              time.Duration
	httpRequestTimeout      time.Duration

	// CLI-based K8S client
	client k8sclient.KubernetesClient

	// File paths
	installerDirectoryPath string
	setupPath              string
	namespacePath          string
	serviceAccountPath     string
	clusterRolePath        string
	crdsPath               string
	clusterRoleBindingPath string
	deploymentPath         string
	servicePath            string
	daemonsetPath          string
	podSecurityPolicyPath  string
	setupYAMLPaths         []string

	appLabel      string
	appLabelKey   string
	appLabelValue string

	persistentObjectLabelKey   string
	persistentObjectLabelValue string

	dns1123LabelRegex  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	dns1123DomainRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

	CRDnames = []string{
		BackendConfigCRDName,
		BackendCRDName,
		MirrorRelationshipCRDName,
		NodeCRDName,
		SnapshotCRDName,
		SnapshotInfoCRDName,
		StorageClassCRDName,
		TransactionCRDName,
		VersionCRDName,
		VolumeCRDName,
		VolumePublicationCRDName,
	}
)

func init() {
	RootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&generateYAML, "generate-custom-yaml", false,
		"Generate YAML files, but don't install anything.")
	installCmd.Flags().BoolVar(&useYAML, "use-custom-yaml", false,
		"Use any existing YAML files that exist in setup directory.")
	installCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during installation.")
	installCmd.Flags().BoolVar(&skipK8sVersionCheck, "skip-k8s-version-check", false,
		"(Deprecated) Skip Kubernetes version check for Trident compatibility")
	installCmd.Flags().BoolVar(&useIPv6, "use-ipv6", false, "Use IPv6 for Trident's communication.")
	installCmd.Flags().BoolVar(&silenceAutosupport, "silence-autosupport", tridentconfig.BuildType != "stable",
		"Don't send autosupport bundles to NetApp automatically.")
	installCmd.Flags().BoolVar(&enableNodePrep, "enable-node-prep", false,
		"*BETA* Attempt to automatically install required packages on nodes.")

	installCmd.Flags().StringVar(&pvcName, "pvc", DefaultPVCName,
		"The name of the legacy PVC used by Trident, will ensure this does not exist.")
	installCmd.Flags().StringVar(&pvName, "pv", DefaultPVName,
		"The name of the legacy PV used by Trident, will ensure this does not exist.")
	installCmd.Flags().StringVar(&tridentImage, "trident-image", "", "The Trident image to install.")
	installCmd.Flags().StringVar(&logFormat, "log-format", "text", "The Trident logging format (text, json).")
	installCmd.Flags().Int64Var(&probePort, "probe-port", 17546,
		"The port used by the node pods for liveness/readiness probes. Must not already be in use on the worker hosts.")
	installCmd.Flags().StringVar(&kubeletDir, "kubelet-dir", "/var/lib/kubelet",
		"The host location of kubelet's internal state.")
	installCmd.Flags().StringVar(&imageRegistry, "image-registry", "",
		"The address/port of an internal image registry.")
	installCmd.Flags().StringVar(&autosupportProxy, "autosupport-proxy", "",
		"The address/port of a proxy for sending Autosupport Telemetry")
	installCmd.Flags().StringVar(&autosupportCustomURL, "autosupport-custom-url", "", "Custom Autosupport endpoint")
	installCmd.Flags().StringVar(&autosupportImage, "autosupport-image", "",
		"The container image for Autosupport Telemetry")
	installCmd.Flags().StringVar(&autosupportSerialNumber, "autosupport-serial-number", "",
		"The value to set for the serial number field in Autosupport payloads")
	installCmd.Flags().StringVar(&autosupportHostname, "autosupport-hostname", "",
		"The value to set for the hostname field in Autosupport payloads")

	installCmd.Flags().DurationVar(&k8sTimeout, "k8s-timeout", 180*time.Second,
		"The timeout for all Kubernetes operations.")
	installCmd.Flags().DurationVar(&httpRequestTimeout, "http-request-timeout", tridentconfig.HTTPTimeout,
		"Override the HTTP request timeout for Trident controller’s REST API")

	if err := installCmd.Flags().MarkHidden("skip-k8s-version-check"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if err := installCmd.Flags().MarkHidden("autosupport-custom-url"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if err := installCmd.Flags().MarkHidden("autosupport-serial-number"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if err := installCmd.Flags().MarkHidden("autosupport-hostname"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if err := installCmd.Flags().MarkDeprecated("enable-node-prep",
		"enable-node-prep is disabled; flag may be removed in a future release."); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
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
			if err := prepareYAMLFiles(); err != nil {
				log.Fatalf("YAML generation failed; %v", err)
			}
			log.WithField("setupPath", setupPath).Info("Wrote installation YAML files.")

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

		// Override registry only if using the default Trident image name and an alternate registry was supplied
		if imageRegistry != "" {
			tridentImage = utils.ReplaceImageRegistry(tridentImage, imageRegistry)
		}
	}
	log.Debugf("Trident image: %s", tridentImage)

	if autosupportImage == "" {
		autosupportImage = tridentconfig.DefaultAutosupportImage

		if imageRegistry != "" {
			autosupportImage = utils.ReplaceImageRegistry(autosupportImage, imageRegistry)
		}
	}
	log.Debugf("Autosupport image: %s", autosupportImage)

	// Create the Kubernetes client
	if client, err = initClient(); err != nil {
		return fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// Before the installation ensure K8s version is valid
	err = tridentconfig.ValidateKubernetesVersion(tridentconfig.KubernetesVersionMin, client.ServerVersion())
	if err != nil {
		if utils.IsUnsupportedKubernetesVersionError(err) {
			log.Errorf("Kubernetes version %s is an %v. NetApp will not take Support calls or "+
				"open Support tickets when using Trident with an unsupported Kubernetes version.",
				client.ServerVersion().String(), err)
		} else {
			return err
		}
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
		"kubernetesVersion":     client.ServerVersion().String(),
	}).Debug("Validated installation environment.")

	return nil
}

func initClient() (k8sclient.KubernetesClient, error) {
	clients, err := k8sclient.CreateK8SClients("", "", "")
	if err != nil {
		return nil, err
	}

	clients.K8SClient.SetTimeout(k8sTimeout)

	return clients.K8SClient, nil
}

func processInstallationArguments(_ *cobra.Command) {
	csi = true
	appLabel = TridentCSILabel
	appLabelKey = TridentCSILabelKey
	appLabelValue = TridentCSILabelValue

	persistentObjectLabelKey = TridentPersistentObjectLabelKey
	persistentObjectLabelValue = TridentPersistentObjectLabelValue
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
	servicePath = path.Join(setupPath, ServiceFilename)
	daemonsetPath = path.Join(setupPath, DaemonSetFilename)
	podSecurityPolicyPath = path.Join(setupPath, PodSecurityPolicyFilename)

	setupYAMLPaths = []string{
		namespacePath, serviceAccountPath, clusterRolePath, clusterRoleBindingPath, crdsPath,
		deploymentPath, servicePath, daemonsetPath, podSecurityPolicyPath,
	}

	return nil
}

func cleanYAMLFiles() {
	for _, filePath := range setupYAMLPaths {
		_ = os.Remove(filePath)
	}
}

func prepareYAMLFiles() error {
	var err error

	cleanYAMLFiles()

	labels := make(map[string]string)
	labels[appLabelKey] = appLabelValue

	daemonSetlabels := make(map[string]string)
	daemonSetlabels[appLabelKey] = TridentNodeLabelValue

	snapshotCRDVersion := client.GetSnapshotterCRDVersion()

	topologyEnabled, err := client.IsTopologyInUse()
	if err != nil {
		return fmt.Errorf("could not determine node topology; %v", err)
	}

	namespaceYAML := k8sclient.GetNamespaceYAML(TridentPodNamespace)
	if err = writeFile(namespacePath, namespaceYAML); err != nil {
		return fmt.Errorf("could not write namespace YAML file; %v", err)
	}

	serviceAccountYAML := k8sclient.GetServiceAccountYAML(getServiceAccountName(true), nil, nil, nil)
	if err = writeFile(serviceAccountPath, serviceAccountYAML); err != nil {
		return fmt.Errorf("could not write service account YAML file; %v", err)
	}

	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), getClusterRoleName(true), nil, nil, true)
	if err = writeFile(clusterRolePath, clusterRoleYAML); err != nil {
		return fmt.Errorf("could not write cluster role YAML file; %v", err)
	}

	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(),
		getClusterRoleBindingName(true), nil, nil, true)
	if err = writeFile(clusterRoleBindingPath, clusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write cluster role binding YAML file; %v", err)
	}

	crdsYAML := k8sclient.GetCRDsYAML()
	if err = writeFile(crdsPath, crdsYAML); err != nil {
		return fmt.Errorf("could not write custom resource definition YAML file; %v", err)
	}

	serviceYAML := k8sclient.GetCSIServiceYAML(getServiceName(), labels, nil)
	if err = writeFile(servicePath, serviceYAML); err != nil {
		return fmt.Errorf("could not write service YAML file; %v", err)
	}

	deploymentArgs := &k8sclient.DeploymentYAMLArguments{
		DeploymentName:          getDeploymentName(true),
		TridentImage:            tridentImage,
		AutosupportImage:        autosupportImage,
		AutosupportProxy:        autosupportProxy,
		AutosupportCustomURL:    autosupportCustomURL,
		AutosupportSerialNumber: autosupportSerialNumber,
		AutosupportHostname:     autosupportHostname,
		ImageRegistry:           imageRegistry,
		LogFormat:               logFormat,
		SnapshotCRDVersion:      snapshotCRDVersion,
		ImagePullSecrets:        []string{},
		Labels:                  labels,
		ControllingCRDetails:    nil,
		Debug:                   Debug,
		UseIPv6:                 useIPv6,
		SilenceAutosupport:      silenceAutosupport,
		Version:                 client.ServerVersion(),
		TopologyEnabled:         topologyEnabled,
		HTTPRequestTimeout:      httpRequestTimeout.String(),
	}
	deploymentYAML := k8sclient.GetCSIDeploymentYAML(deploymentArgs)
	if err = writeFile(deploymentPath, deploymentYAML); err != nil {
		return fmt.Errorf("could not write deployment YAML file; %v", err)
	}

	daemonArgs := &k8sclient.DaemonsetYAMLArguments{
		DaemonsetName:        getDaemonSetName(),
		TridentImage:         tridentImage,
		ImageRegistry:        imageRegistry,
		KubeletDir:           kubeletDir,
		LogFormat:            logFormat,
		ProbePort:            strconv.FormatInt(probePort, 10),
		ImagePullSecrets:     []string{},
		Labels:               daemonSetlabels,
		ControllingCRDetails: nil,
		Debug:                Debug,
		Version:              client.ServerVersion(),
		HTTPRequestTimeout:   httpRequestTimeout.String(),
	}
	daemonSetYAML := k8sclient.GetCSIDaemonSetYAML(daemonArgs)
	if err = writeFile(daemonsetPath, daemonSetYAML); err != nil {
		return fmt.Errorf("could not write daemonset YAML file; %v", err)
	}

	podSecurityPolicyYAML := k8sclient.GetPrivilegedPodSecurityPolicyYAML(getPSPName(), nil, nil)
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
	return ioutil.WriteFile(filePath, []byte(data), 0o644)
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
		logFields log.Fields
		pvcExists bool
		pvExists  bool
		crd       *apiextensionv1.CustomResourceDefinition
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
		var crdExists bool
		crdExists, returnError = client.CheckCRDExists(crdName)
		if returnError != nil {
			return
		}
		if !crdExists {
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
		if strings.Contains(strings.ToLower(crd.APIVersion), "alpha") {
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
	allCRDsPresent := true
	var installedCRDs []string
	for _, crdName := range CRDnames {
		// See if any of the CRDs exist
		var crdExists bool
		crdExists, returnError = client.CheckCRDExists(crdName)
		if returnError != nil {
			return
		}
		if crdExists {
			log.WithField("CRD", crdName).Debug("CRD present.")
			installedCRDs = append(installedCRDs, crdName)
		} else {
			log.WithField("CRD", crdName).Debug("CRD not present.")
			allCRDsPresent = false
		}
	}

	// Let TridentVersions CRD be the deciding factor if the CRDs exist, since
	// TridentVersions is the last CRD created during installation.
	if utils.SliceContainsString(installedCRDs, VersionCRDName) {
		log.Debug("Trident Version CRD present, skipping PVC/PV check.")
	} else {

		// We didn't find any CRD data, so look for legacy etcd data
		pvcExists, pvExists, returnError = discoverLegacyEtcdData()
		if returnError != nil {
			return
		}

		logFields = log.Fields{"pv": pvName, "pvc": pvcName}

		if pvcExists && pvExists {
			log.WithFields(logFields).Debug("PV and PVC exist from a pre-19.07 Trident installation.")
			returnError = errors.New("PV and PVC exist from a pre-19.07 Trident installation.  This data " +
				"must be migrated to CRDs.  Install Trident 20.07 to accomplish this migration, and then upgrade " +
				"to any Trident version newer than 20.07")
			return
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

	// Create or patch namespace
	if namespaceExists {
		returnError = patchNamespace()
	} else {
		returnError = createNamespace()
	}
	if returnError != nil {
		return
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

	// Create the CRDs and wait for them to be established in Kubernetes
	if !allCRDsPresent {

		// Create CRDs and ensure they are established
		if returnError = createAndEnsureCRDs(); returnError != nil {
			log.Errorf("could not create the Trident CRDs; %v", returnError)
			return
		}

		// Ensure we can create a CRD client
		if _, returnError = getCRDClient(); returnError != nil {
			log.Errorf("Could not create CRD client; %v", returnError)
			return
		}
	}

	// Create pod security policy
	if useYAML && fileExists(podSecurityPolicyPath) {
		returnError = validateTridentPodSecurityPolicy()
		if returnError != nil {
			returnError = fmt.Errorf("please correct the pod security policy YAML file; %v", returnError)
			return
		}
		// Delete the object in case it already exists and we need to update it
		if err := client.DeleteObjectByFile(podSecurityPolicyPath, true); err != nil {
			returnError = fmt.Errorf("could not delete pod security policy; %v", err)
			return
		}
		returnError = client.CreateObjectByFile(podSecurityPolicyPath)
		logFields = log.Fields{"path": podSecurityPolicyPath}
	} else {
		// Delete the object in case it already exists and we need to update it
		pspYAML := k8sclient.GetPrivilegedPodSecurityPolicyYAML(getPSPName(), nil, nil)

		if err := client.DeleteObjectByYAML(pspYAML, true); err != nil {
			returnError = fmt.Errorf("could not delete pod security policy; %v", err)
			return
		}
		returnError = client.CreateObjectByYAML(pspYAML)
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident pod security policy; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created Trident pod security policy.")

	// Patch the CRD definitions with finalizers to protect them
	if returnError = protectCustomResourceDefinitions(); returnError != nil {
		return
	}

	labels := make(map[string]string)
	labels[appLabelKey] = appLabelValue

	persistentObjectLabels := make(map[string]string)
	persistentObjectLabels[appLabelKey] = appLabelValue
	persistentObjectLabels[persistentObjectLabelKey] = persistentObjectLabelValue

	snapshotCRDVersion := client.GetSnapshotterCRDVersion()

	topologyEnabled, err := client.IsTopologyInUse()
	if err != nil {
		return fmt.Errorf("could not determine node topology; %v", err)
	}

	// Create the CSI Driver object if necessary (1.14+)
	returnError = createK8SCSIDriver()
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Kubernetes CSI Driver object; %v", returnError)
		return
	}

	// Create the service
	if useYAML && fileExists(servicePath) {
		returnError = validateTridentService()
		if returnError != nil {
			returnError = fmt.Errorf("please correct the service YAML file; %v", returnError)
			return
		}
		returnError = client.CreateObjectByFile(servicePath)
		logFields = log.Fields{"path": servicePath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetCSIServiceYAML(getServiceName(), labels, nil))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident service; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created Trident service.")

	// Check if Trident's Encryption Secret already exists in the specified namespace.
	secretExists, err := client.CheckSecretExists(getEncryptionSecretName())
	if err != nil {
		returnError = fmt.Errorf("could not check for existing Trident encryption secret; %v", err)
		return
	}

	// If the encryption secret doesn't exist create a new one.
	if !secretExists {
		aesKey, err := utils.GenerateAESKey()
		if err != nil {
			returnError = fmt.Errorf("could not generate secure AES key; %v", err)
			return
		}

		// Create the secret for the HTTP certs & keys
		secretMap := map[string]string{
			tridentconfig.AESKeyFile: aesKey,
		}

		// Pass in the persistentObjectLabels here
		err = client.CreateObjectByYAML(
			k8sclient.GetSecretYAML(getEncryptionSecretName(), TridentPodNamespace, persistentObjectLabels, nil,
				secretMap, nil))
		if err != nil {
			returnError = fmt.Errorf("could not create Trident encryption secret; %v", err)
			return
		}
		log.WithFields(logFields).Info("Created Trident encryption secret.")
	}

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
		k8sclient.GetSecretYAML(getProtocolSecretName(), TridentPodNamespace, labels, nil, secretMap, nil))
	if err != nil {
		returnError = fmt.Errorf("could not create Trident protocol secret; %v", err)
		return
	}
	log.WithFields(logFields).Info("Created Trident protocol secret.")

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
		deploymentArgs := &k8sclient.DeploymentYAMLArguments{
			DeploymentName:          getDeploymentName(true),
			TridentImage:            tridentImage,
			AutosupportImage:        autosupportImage,
			AutosupportProxy:        autosupportProxy,
			AutosupportCustomURL:    autosupportCustomURL,
			AutosupportSerialNumber: autosupportSerialNumber,
			AutosupportHostname:     autosupportHostname,
			ImageRegistry:           imageRegistry,
			LogFormat:               logFormat,
			SnapshotCRDVersion:      snapshotCRDVersion,
			ImagePullSecrets:        []string{},
			Labels:                  labels,
			ControllingCRDetails:    nil,
			Debug:                   Debug,
			UseIPv6:                 useIPv6,
			SilenceAutosupport:      silenceAutosupport,
			Version:                 client.ServerVersion(),
			TopologyEnabled:         topologyEnabled,
			HTTPRequestTimeout:      httpRequestTimeout.String(),
		}
		returnError = client.CreateObjectByYAML(
			k8sclient.GetCSIDeploymentYAML(deploymentArgs))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident deployment; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created Trident deployment.")

	// Create the daemonset
	if useYAML && fileExists(daemonsetPath) {
		returnError = validateTridentDaemonSet()
		if returnError != nil {
			returnError = fmt.Errorf("please correct the daemonset YAML file; %v", returnError)
			return
		}
		returnError = client.CreateObjectByFile(daemonsetPath)
		logFields = log.Fields{"path": daemonsetPath}
	} else {
		daemonSetlabels := make(map[string]string)
		daemonSetlabels[appLabelKey] = TridentNodeLabelValue
		daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
			DaemonsetName:        getDaemonSetName(),
			TridentImage:         tridentImage,
			ImageRegistry:        imageRegistry,
			KubeletDir:           kubeletDir,
			LogFormat:            logFormat,
			ProbePort:            strconv.FormatInt(probePort, 10),
			ImagePullSecrets:     []string{},
			Labels:               daemonSetlabels,
			ControllingCRDetails: nil,
			Debug:                Debug,
			Version:              client.ServerVersion(),
			HTTPRequestTimeout:   httpRequestTimeout.String(),
		}
		returnError = client.CreateObjectByYAML(
			k8sclient.GetCSIDaemonSetYAML(daemonSetArgs))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident daemonset; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created Trident daemonset.")

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

func discoverLegacyEtcdData() (pvcExists, pvExists bool, returnError error) {
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

func patchNamespace() error {
	labels := map[string]string{
		tridentconfig.PodSecurityStandardsEnforceLabel: tridentconfig.PodSecurityStandardsEnforceProfile,
	}
	return client.PatchNamespaceLabels(TridentPodNamespace, labels)
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

func createAndEnsureCRDs() (returnError error) {
	var bundleCRDYAML string
	if useYAML && fileExists(crdsPath) {

		content, err := ioutil.ReadFile(crdsPath)
		if err != nil {
			return err
		}

		bundleCRDYAML = string(content)
	} else {
		bundleCRDYAML = k8sclient.GetCRDsYAML()
	}

	crdMap := getCRDMapFromBundle(bundleCRDYAML)

	// Ensure crdMap has all the CRDs, nothing is missing or extra
	returnError = validateCRDs(crdMap)
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident CRDs; %v", returnError)
		return
	}

	returnError = createCRDs(crdMap)
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident CRDs; %v", returnError)
		return
	}

	log.Info("Created custom resource definitions.")

	return nil
}

// getCRDMapFromBundle creates a map of CRD name to CRD definition from the bundle
func getCRDMapFromBundle(bundle string) map[string]string {
	labelEqualRegex := regexp.MustCompile(`(?m)^\s*name:\s*(?P<crdName>[\w\.]+)$`)
	yamls := strings.Split(bundle, "---")
	crdMap := make(map[string]string)

	for i := range yamls {
		match := labelEqualRegex.FindStringSubmatch(yamls[i])
		for j := range labelEqualRegex.SubexpNames() {
			if j > 0 && j <= len(match) {
				crdMap[match[j]] = yamls[i]
				break
			}
		}
	}

	return crdMap
}

// validateCRDs validates the list of CRDs
func validateCRDs(crdMap map[string]string) error {
	crdMatch := make(map[string]bool)
	var errMessages []string
	var missingCRDs []string
	var extraCRDs []string

	for crdName := range crdMap {
		crdMatch[crdName] = false
	}

	for _, crdName := range CRDnames {
		if _, ok := crdMap[crdName]; ok {
			crdMatch[crdName] = true
		} else {
			missingCRDs = append(missingCRDs, crdName)
		}
	}

	for crdName, isValid := range crdMatch {
		if !isValid {
			extraCRDs = append(extraCRDs, crdName)
		}
	}

	if len(missingCRDs) > 0 {
		errMessages = append(errMessages, fmt.Sprintf("missing CRD(s): %v", missingCRDs))
	}

	if len(extraCRDs) > 0 {
		errMessages = append(errMessages, fmt.Sprintf("unrecognized CRD(s) found: %v", extraCRDs))
	}

	if len(errMessages) > 0 {
		errMsg := strings.Join(errMessages, "; ")
		log.Errorf(errMsg)
		return fmt.Errorf("CRD validation failed; %v", errMsg)
	}

	return nil
}

// createCRDs creates and establishes each of the CRDs individually
func createCRDs(crdMap map[string]string) error {
	var returnError error

	for crdName, crdYAML := range crdMap {
		if returnError = createCRD(crdName, crdYAML); returnError != nil {
			return returnError
		}
	}

	return returnError
}

// createCRD creates and establishes the CRD
func createCRD(crdName, crdYAML string) error {
	// Discover CRD data
	crdExists, returnError := client.CheckCRDExists(crdName)
	if returnError != nil {
		return fmt.Errorf("unable to identify if %v CRD exists; err: %v", crdName, returnError)
	}

	if crdExists {
		log.Infof("Trident %v CRD present.", crdName)
	} else {
		// Create the CRDs and wait for them to be registered in Kubernetes
		log.Infof("Installer will create a fresh %v CRD.", crdName)

		if returnError = createCustomResourceDefinition(crdName, crdYAML); returnError != nil {
			return returnError
		}

		// Wait for the CRD to be fully established
		if returnError = ensureCRDEstablished(crdName); returnError != nil {
			// If CRD registration failed *and* we created the CRDs, clean up by deleting all the CRDs
			log.Errorf("CRD %v not established; %v", crdName, returnError)
			if err := deleteCustomResourceDefinition(crdName, crdYAML); err != nil {
				log.Errorf("Could not delete CRD %v; %v", crdName, err)
			}
			return returnError
		}
	}

	return returnError
}

func createCustomResourceDefinition(crdName, crdYAML string) (returnError error) {
	var logFields log.Fields

	returnError = client.CreateObjectByYAML(crdYAML)
	logFields = log.Fields{"namespace": TridentPodNamespace}
	if returnError != nil {
		returnError = fmt.Errorf("could not create custom resource  %v in %s; %v", crdName, TridentPodNamespace,
			returnError)
		return
	}
	log.WithFields(logFields).Infof("Created custom resource definitions %v.", crdName)
	return nil
}

// ensureCRDEstablished waits until a CRD is Established.
func ensureCRDEstablished(crdName string) error {
	checkCRDEstablished := func() error {
		crd, err := client.GetCRD(crdName)
		if err != nil {
			return err
		}
		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionv1.Established {
				switch condition.Status {
				case apiextensionv1.ConditionTrue:
					return nil
				default:
					return fmt.Errorf("CRD %s Established condition is %s", crdName, condition.Status)
				}
			}
		}
		return fmt.Errorf("CRD %s Established condition is not yet available", crdName)
	}

	checkCRDNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"CRD": crdName,
			"err": err,
		}).Debug("CRD not yet established, waiting.")
	}

	checkCRDBackoff := backoff.NewExponentialBackOff()
	checkCRDBackoff.MaxInterval = 5 * time.Second
	checkCRDBackoff.MaxElapsedTime = k8sTimeout

	log.WithField("CRD", crdName).Trace("Waiting for CRD to be established.")

	if err := backoff.RetryNotify(checkCRDEstablished, checkCRDBackoff, checkCRDNotify); err != nil {
		return fmt.Errorf("CRD was not established after %3.2f seconds", k8sTimeout.Seconds())
	}

	log.WithField("CRD", crdName).Debug("CRD established.")
	return nil
}

func deleteCustomResourceDefinition(crdName, crdYAML string) (returnError error) {
	var logFields log.Fields

	returnError = client.DeleteObjectByYAML(crdYAML, true)
	if returnError != nil {
		returnError = fmt.Errorf("could not delete custom resource definition %v in %s; %v", crdName,
			TridentPodNamespace,
			returnError)
		return
	}
	log.WithFields(logFields).Infof("Deleted custom resource definition %v.", crdName)
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

func createK8SCSIDriver() error {
	// Delete the object in case it already exists and we need to update it
	err := client.DeleteObjectByYAML(k8sclient.GetCSIDriverYAML(getCSIDriverName(), client.ServerVersion(),
		nil, nil), true)
	if err != nil {
		return fmt.Errorf("could not delete csidriver custom resource; %v", err)
	}

	err = client.CreateObjectByYAML(k8sclient.GetCSIDriverYAML(getCSIDriverName(), client.ServerVersion(),
		nil, nil))
	if err != nil {
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
		returnError = client.CreateObjectByYAML(k8sclient.GetServiceAccountYAML(getServiceAccountName(csi), nil, nil,
			nil))
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
		returnError = client.CreateObjectByYAML(k8sclient.GetClusterRoleYAML(client.Flavor(), getClusterRoleName(csi),
			nil, nil, csi))
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
			k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(),
				getClusterRoleBindingName(csi), nil, nil, csi))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create cluster role binding; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created cluster role binding.")

	// If OpenShift, add Trident to security context constraint(s)
	if client.Flavor() == k8sclient.FlavorOpenShift {
		if returnError = CreateOpenShiftTridentSCC("trident-csi", TridentCSILabelValue); returnError != nil {
			returnError = fmt.Errorf("could not create security context constraint; %v", returnError)
			return
		}
		log.WithFields(log.Fields{
			"scc":  "trident",
			"user": "trident-csi",
		}).Info("Created Trident's security context constraint.")
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
	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(),
		getClusterRoleBindingName(csi), nil, nil, csi)
	if err := client.DeleteObjectByYAML(clusterRoleBindingYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete cluster role binding.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted cluster role binding.")
	}

	// Delete cluster role
	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), getClusterRoleName(csi),
		nil, nil, csi)
	if err := client.DeleteObjectByYAML(clusterRoleYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete cluster role.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted cluster role.")
	}

	// Delete service account
	serviceAccountYAML := k8sclient.GetServiceAccountYAML(getServiceAccountName(csi), nil, nil,
		nil)
	if err := client.DeleteObjectByYAML(serviceAccountYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete service account.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted service account.")
	}

	// If OpenShift, delete Trident's security context constraint
	if client.Flavor() == k8sclient.FlavorOpenShift {
		user := "trident"
		if csi {
			user = "trident-csi"
		}
		if err := DeleteOpenShiftTridentSCC(user); err != nil {
			log.WithField("error", err).Warning("Could not delete security context constraint.")
			anyErrors = true
		} else {
			logFunc(log.Fields{
				"scc":  "trident",
				"user": user,
			})("Deleted Trident's security context constraint.")
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

	tridentImage = ""
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == tridentconfig.ContainerTrident {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return fmt.Errorf("the Trident deployment must define the %s container", tridentconfig.ContainerTrident)
	} else {
		log.WithField("image", tridentImage).Debug("Read Trident image from custom YAML.")
	}

	return nil
}

func validateTridentPodSecurityPolicy() error {
	securityPolicy, err := readPodSecurityPolicyFromFile(podSecurityPolicyPath)
	if err != nil {
		return fmt.Errorf("could not load pod security policy YAML file; %v", err)
	}

	// Check the security settings, CSI Trident requires enhanced privileges for performing mounts
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
	if !spec.HostPID {
		return fmt.Errorf("trident's pod security policy must allow hostPID")
	}
	if !spec.HostNetwork {
		return fmt.Errorf("trident's pod security policy must allow hostNetwork")
	}
	found := false
	for _, allowedCap := range spec.AllowedCapabilities {
		if allowedCap == "SYS_ADMIN" {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("trident's pod security policy must allow SYS_ADMIN capability")
	}

	return nil
}

func validateTridentService() error {
	service, err := readServiceFromFile(servicePath)
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
	daemonset, err := readDaemonSetFromFile(daemonsetPath)
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
	var deployment *appsv1.Deployment
	var pod *v1.Pod
	var err error

	checkPodRunning := func() error {
		deployment, err = client.GetDeploymentByLabel(appLabel, false)
		if err != nil {
			return err
		}

		// If Trident was just uninstalled, there could be multiple pods for a brief time.  Calling GetPodByLabel
		// ensures only one is running.
		pod, err = client.GetPodByLabel(appLabel, false)
		if err != nil {
			return err
		}

		// Ensure the pod hasn't been deleted.
		if pod.DeletionTimestamp != nil {
			return errors.New("trident pod is terminating")
		}

		// Ensure the pod is running
		if pod.Status.Phase != v1.PodRunning {
			return errors.New("trident pod is not running")
		}

		// Ensure the pod spec contains the correct image.  The running container may report a different
		// image name if there are multiple tags for the same image hash, but the pod spec should be correct.
		for _, container := range pod.Spec.Containers {
			if container.Name == TridentMainContainer {
				if container.Image != tridentImage {
					return fmt.Errorf("trident pod spec reports a different image (%s) than required (%s)",
						container.Image, tridentImage)
				}
			}
		}

		// Ensure the Trident controller container is the correct image.
		tridentContainerOK := false
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == TridentMainContainer {
				if container.State.Running == nil {
					return errors.New("trident container is not running")
				} else if !container.Ready {
					return errors.New("trident container is not ready")
				}
				tridentContainerOK = true
			}
		}

		if !tridentContainerOK {
			return fmt.Errorf("running container %s not found in trident deployment", TridentMainContainer)
		}

		return nil
	}

	podNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"status":    err,
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
					errMessages = append(errMessages, pod.Status.Message)
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
		"deployment": deployment.Name,
		"pod":        pod.Name,
		"namespace":  TridentPodNamespace,
	}).Info("Trident pod started.")

	return pod, nil
}

func waitForRESTInterface() error {
	var version string

	checkRESTInterface := func() error {
		cliCommand := []string{"tridentctl", "version", "-o", "json"}
		versionJSON, err := client.Exec(TridentPodName, tridentconfig.ContainerTrident, cliCommand)
		if err != nil {
			if len(versionJSON) > 0 {
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
			"error":     err,
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

// CreateOpenShiftTridentSCC creates an SCC solely for use with the trident user. This only works for OpenShift.
func CreateOpenShiftTridentSCC(user, appLabelVal string) error {
	// Remove trident user from built-in SCC from previous installation
	if user == "trident-installer" {
		_ = client.RemoveTridentUserFromOpenShiftSCC("trident-installer", "privileged")
	} else if strings.Contains(appLabelValue, "csi") {
		_ = client.RemoveTridentUserFromOpenShiftSCC("trident-csi", "privileged")
	} else {
		_ = client.RemoveTridentUserFromOpenShiftSCC("trident", "anyuid")
	}

	labels := make(map[string]string)
	labels["app"] = appLabelVal

	err := client.CreateObjectByYAML(k8sclient.GetOpenShiftSCCYAML("trident", user, TridentPodNamespace, labels, nil))
	if err != nil {
		return fmt.Errorf("cannot create trident's scc; %v", err)
	}
	return nil
}

// DeleteOpenShiftTridentSCC deletes the trident-only SCC that the trident user uses. This only works for OpenShift.
func DeleteOpenShiftTridentSCC(user string) error {
	labelVal := TridentLegacyLabelValue
	if csi {
		labelVal = TridentCSILabelValue
	}

	labels := make(map[string]string)
	labels["app"] = labelVal

	err := client.DeleteObjectByYAML(
		k8sclient.GetOpenShiftSCCYAML("trident", user, TridentPodNamespace, labels, nil), true)
	if err != nil {
		return fmt.Errorf("%s; %v", "could not delete trident's scc", err)
	}
	return nil
}

func getCRDClient() (*crdclient.Clientset, error) {
	var crdClient *crdclient.Clientset

	createCRDClient := func() error {
		var err error
		crdClient, err = client.GetCRDClient()
		return err
	}

	createCRDClientNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration.Truncate(100 * time.Millisecond),
			"message":   err.Error(),
		}).Debug("CRD client not yet created, waiting.")
	}

	createCRDClientBackoff := backoff.NewExponentialBackOff()
	createCRDClientBackoff.InitialInterval = 1 * time.Second
	createCRDClientBackoff.RandomizationFactor = 0.1
	createCRDClientBackoff.Multiplier = 1.414
	createCRDClientBackoff.MaxInterval = 5 * time.Second
	createCRDClientBackoff.MaxElapsedTime = k8sTimeout

	log.Debug("Creating CRD client.")

	if err := backoff.RetryNotify(createCRDClient, createCRDClientBackoff, createCRDClientNotify); err != nil {
		return nil, err
	}

	log.Debug("Created CRD client.")

	return crdClient, nil
}

func getServiceAccountName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getClusterRoleName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getClusterRoleBindingName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getPSPName() string {
	return TridentPSP
}

func getServiceName() string {
	return TridentCSI
}

func getProtocolSecretName() string {
	return TridentCSI
}

func getEncryptionSecretName() string {
	return TridentEncryptionKeys
}

func getDeploymentName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getDaemonSetName() string {
	return TridentCSI
}

func getCSIDriverName() string {
	return CSIDriver
}
