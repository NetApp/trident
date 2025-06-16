// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
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
	storagev1 "k8s.io/api/storage/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	"github.com/netapp/trident/internal/nodeprep/protocol"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/network"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	PreferredNamespace = tridentconfig.OrchestratorName

	// CRD names
	ActionMirrorUpdateCRDName    = "tridentactionmirrorupdates.trident.netapp.io"
	ActionSnapshotRestoreCRDName = "tridentactionsnapshotrestores.trident.netapp.io"
	BackendConfigCRDName         = "tridentbackendconfigs.trident.netapp.io"
	BackendCRDName               = "tridentbackends.trident.netapp.io"
	MirrorRelationshipCRDName    = "tridentmirrorrelationships.trident.netapp.io"
	NodeCRDName                  = "tridentnodes.trident.netapp.io"
	SnapshotCRDName              = "tridentsnapshots.trident.netapp.io"
	SnapshotInfoCRDName          = "tridentsnapshotinfos.trident.netapp.io"
	GroupSnapshotCRDName         = "tridentgroupsnapshots.trident.netapp.io"
	StorageClassCRDName          = "tridentstorageclasses.trident.netapp.io"
	TransactionCRDName           = "tridenttransactions.trident.netapp.io"
	VersionCRDName               = "tridentversions.trident.netapp.io"
	VolumeCRDName                = "tridentvolumes.trident.netapp.io"
	VolumePublicationCRDName     = "tridentvolumepublications.trident.netapp.io"
	VolumeReferenceCRDName       = "tridentvolumereferences.trident.netapp.io"
	ConfiguratorCRDName          = "tridentconfigurators.trident.netapp.io"

	ControllerRoleFilename               = "trident-controller-role.yaml"
	ControllerClusterRoleFilename        = "trident-controller-clusterrole.yaml"
	ControllerRoleBindingFilename        = "trident-controller-rolebinding.yaml"
	ControllerServiceAccountFilename     = "trident-controller-serviceaccount.yaml"
	ControllerClusterRoleBindingFilename = "trident-controller-clusterrolebinding.yaml"
	ControllerSCCFilename                = "trident-controller-scc.yaml"

	NodeLinuxRoleFilename           = "trident-node-linux-role.yaml"
	NodeLinuxRoleBindingFilename    = "trident-node-linux-rolebinding.yaml"
	NodeLinuxServiceAccountFilename = "trident-node-linux-serviceaccount.yaml"
	NodeLinuxSCCFilename            = "trident-node-linux-scc.yaml"

	NodeWindowsRoleFilename           = "trident-node-windows-role.yaml"
	NodeWindowsRoleBindingFilename    = "trident-node-windows-rolebinding.yaml"
	NodeWindowsServiceAccountFilename = "trident-node-windows-serviceaccount.yaml"
	NodeWindowsSCCFilename            = "trident-node-windows-scc.yaml"

	CRDsFilename             = "trident-crds.yaml"
	DaemonSetFilename        = "trident-daemonset.yaml"
	WindowsDaemonSetFilename = "trident-windows-daemonset.yaml"
	DeploymentFilename       = "trident-deployment.yaml"
	NamespaceFilename        = "trident-namespace.yaml"
	ServiceFilename          = "trident-service.yaml"
	ResourceQuotaFilename    = "trident-resourcequota.yaml"

	TridentEncryptionKeys = "trident-encryption-keys"

	TridentCSI           = "trident-csi"
	TridentMainContainer = "trident-main"

	TridentControllerResourceName  = "trident-controller"
	TridentNodeLinuxResourceName   = "trident-node-linux"
	TridentNodeWindowsResourceName = "trident-node-windows"

	CSIDriver = "csi.trident.netapp.io"
)

var (
	// CLI flags
	generateYAML             bool
	useYAML                  bool
	silent                   bool
	useIPv6                  bool
	silenceAutosupport       bool
	excludeAutosupport       bool
	nodePrep                 []string
	skipK8sVersionCheck      bool
	windows                  bool
	enableForceDetach        bool
	disableAuditLog          bool
	tridentImage             string
	autosupportImage         string
	autosupportProxy         string
	autosupportInsecure      bool
	autosupportCustomURL     string
	autosupportSerialNumber  string
	autosupportHostname      string
	kubeletDir               string
	imageRegistry            string
	logFormat                string
	imagePullPolicy          string
	imagePullSecrets         []string
	logWorkflows             string
	logLayers                string
	probePort                int64
	k8sTimeout               time.Duration
	httpRequestTimeout       time.Duration
	acpImage                 string // TODO: Remove after 26.04.
	enableACP                bool   // TODO: Remove after 26.04.
	cloudProvider            string
	cloudIdentity            string
	iscsiSelfHealingInterval time.Duration
	iscsiSelfHealingWaitTime time.Duration
	k8sAPIQPS                int
	fsGroupPolicy            string
	enableConcurrency        bool

	// CLI-based K8S client
	client k8sclient.KubernetesClient

	// File paths
	installerDirectoryPath           string
	setupPath                        string
	namespacePath                    string
	controllerServiceAccountPath     string
	nodeLinuxServiceAccountPath      string
	nodeWindowsServiceAccountPath    string
	controllerRolePath               string
	controllerClusterRolePath        string
	nodeLinuxRolePath                string
	nodeWindowsRolePath              string
	crdsPath                         string
	controllerRoleBindingPath        string
	controllerClusterRoleBindingPath string
	nodeLinuxRoleBindingPath         string
	nodeWindowsRoleBindingPath       string
	deploymentPath                   string
	servicePath                      string
	daemonsetPath                    string
	windowsDaemonSetPath             string
	resourceQuotaPath                string
	controllerSCCPath                string
	nodeLinuxSCCPath                 string
	nodeWindowsSCCPath               string
	setupYAMLPaths                   []string

	appLabel      string
	appLabelKey   string
	appLabelValue string
	identityLabel bool

	persistentObjectLabelKey   string
	persistentObjectLabelValue string

	CRDnames = []string{
		ActionMirrorUpdateCRDName,
		BackendConfigCRDName,
		BackendCRDName,
		MirrorRelationshipCRDName,
		NodeCRDName,
		VolumeReferenceCRDName,
		SnapshotCRDName,
		GroupSnapshotCRDName,
		SnapshotInfoCRDName,
		StorageClassCRDName,
		TransactionCRDName,
		VersionCRDName,
		VolumeCRDName,
		VolumePublicationCRDName,
		ActionSnapshotRestoreCRDName,
		ConfiguratorCRDName,
	}
)

func init() {
	RootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&generateYAML, "generate-custom-yaml", false,
		"Generate YAML files, but don't install anything.")
	installCmd.Flags().BoolVar(&windows, "windows", false, "Install Trident on Windows worker nodes.")
	installCmd.Flags().BoolVar(&useYAML, "use-custom-yaml", false,
		"Use any existing YAML files that exist in setup directory.")
	installCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during installation.")
	installCmd.Flags().BoolVar(&skipK8sVersionCheck, "skip-k8s-version-check", false,
		"(Deprecated) Skip Kubernetes version check for Trident compatibility")
	installCmd.Flags().BoolVar(&useIPv6, "use-ipv6", false, "Use IPv6 for Trident's communication.")
	installCmd.Flags().BoolVar(&silenceAutosupport, "silence-autosupport", tridentconfig.BuildType != "stable",
		"Don't send autosupport bundles to NetApp automatically.")
	installCmd.Flags().BoolVar(&excludeAutosupport, "exclude-autosupport", false,
		"Don't install or run the autosupport container.")
	installCmd.Flags().StringSliceVar(&nodePrep, "node-prep", []string{}, "Comma separated list of protocols to prepare nodes for.  Currently only iSCSI is supported.")
	installCmd.Flags().BoolVar(&enableForceDetach, "enable-force-detach", false,
		"Enable the force detach feature.")
	installCmd.Flags().BoolVar(&disableAuditLog, "disable-audit-log", true, "Disable the audit logger.")
	installCmd.Flags().StringVar(&tridentImage, "trident-image", "",
		"Trident container image. When installing Trident from a private image registry, this flag must be set to the path of the container image.")
	installCmd.Flags().StringVar(&logFormat, "log-format", "text", "The Trident logging format (text, json).")
	installCmd.Flags().StringVar(&logWorkflows, "log-workflows", "", "A comma-delimited list of Trident "+
		"workflows for which to enable trace logging.")
	installCmd.Flags().StringVar(&logLayers, "log-layers", "", "A comma-delimited list of Trident "+
		"log layers for which to enable trace logging.")
	installCmd.Flags().Int64Var(&probePort, "probe-port", 17546,
		"The port used by the node pods for liveness/readiness probes. Must not already be in use on the worker hosts.")
	installCmd.Flags().StringVar(&kubeletDir, "kubelet-dir", "/var/lib/kubelet",
		"The host location of kubelet's internal state.")
	installCmd.Flags().StringVar(&imageRegistry, "image-registry", "",
		"The address/port of an internal image registry location. For more information on specifying image locations, "+
			"consult the Trident documentation.")
	installCmd.Flags().StringVar(&autosupportProxy, "autosupport-proxy", "",
		"The address/port of a proxy for sending Autosupport Telemetry")
	installCmd.Flags().BoolVar(&autosupportInsecure, "autosupport-insecure", false,
		"Skip TLS verification in Autosupport client.")
	installCmd.Flags().StringVar(&autosupportCustomURL, "autosupport-custom-url", "", "Custom Autosupport endpoint")
	installCmd.Flags().StringVar(&autosupportImage, "autosupport-image", "",
		"Trident Autosupport container image. When "+
			"installing Trident from a private image registry, this flag must be set to the path of the Trident Autosupport container image.")
	installCmd.Flags().StringVar(&autosupportSerialNumber, "autosupport-serial-number", "",
		"The value to set for the serial number field in Autosupport payloads")
	installCmd.Flags().StringVar(&autosupportHostname, "autosupport-hostname", "",
		"The value to set for the hostname field in Autosupport payloads")
	installCmd.Flags().StringVar(&imagePullPolicy, "image-pull-policy", "IfNotPresent",
		"The image pull policy for the Trident.")
	installCmd.Flags().StringSliceVar(&imagePullSecrets, "image-pull-secrets", []string{},
		"Comma separated list of image pull secrets.")
	installCmd.Flags().DurationVar(&k8sTimeout, "k8s-timeout", 180*time.Second,
		"The timeout for all Kubernetes operations.")
	installCmd.Flags().DurationVar(&httpRequestTimeout, "http-request-timeout", tridentconfig.HTTPTimeout,
		"Override the HTTP request timeout for Trident controller’s REST API")
	installCmd.Flags().DurationVar(&iscsiSelfHealingInterval, "iscsi-self-healing-interval", tridentconfig.IscsiSelfHealingInterval,
		"Override the default iSCSI self-healing interval.")
	installCmd.Flags().DurationVar(&iscsiSelfHealingWaitTime, "iscsi-self-healing-wait-time", tridentconfig.ISCSISelfHealingWaitTime,
		"Override the default iSCSI self-healing wait time after which unhealthy sessions are logged out.")

	installCmd.Flags().BoolVar(&enableACP, "enable-acp", false, "Enable the Trident-ACP premium features (obsolete).")
	installCmd.Flags().StringVar(&acpImage, "acp-image", "",
		"Override the default trident-acp container image (obsolete).")

	installCmd.Flags().StringVar(&cloudProvider, "cloud-provider", "", "Name of the cloud provider")
	installCmd.Flags().StringVar(&cloudIdentity, "cloud-identity", "", "Cloud identity to be set on service account")

	installCmd.Flags().IntVar(&k8sAPIQPS, "k8s-api-qps", 0, "The QPS used by the controller while talking "+
		"with the Kubernetes API server. The Burst value is automatically set as a function of the QPS value.")
	installCmd.Flags().StringVar(&fsGroupPolicy, "fs-group-policy", "", "The FSGroupPolicy "+
		"to set on Trident's CSIDriver resource.")
	installCmd.Flags().BoolVar(&enableConcurrency, "enable-concurrency", false, "Enable concurrency for Trident's controller **TECH PREVIEW**")

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
	if err := installCmd.Flags().MarkHidden("exclude-autosupport"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if err := installCmd.Flags().MarkHidden("fs-group-policy"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initInstallerLogging()

		if err := discoverInstallationEnvironment(); err != nil {
			Log().Fatalf("Install pre-checks failed; %v. Resolve the issue and try again.", err)
		}
		processInstallationArguments(cmd)
		if err := validateInstallationArguments(); err != nil {
			Log().Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		if generateYAML {

			// Ensure the setup directory exists
			if err := ensureSetupDirExists(); err != nil {
				Log().Fatalf("Could not find or create setup directory; %v", err)
			}

			// If generate-custom-yaml was specified, write the YAML files to the setup directory
			if err := prepareYAMLFiles(); err != nil {
				Log().Fatalf("YAML generation failed; %v", err)
			}
			Log().WithField("setupPath", setupPath).Info("Wrote installation YAML files.")

		} else {
			// Run the installer directly using the Kubernetes client
			if err := installTrident(); err != nil {
				Log().Fatalf("Install failed; %v.  Resolve the issue; use 'tridentctl uninstall' "+
					"to clean up; and try again.", err)
			}
		}
	},
}

// initInstallerLogging configures logging for Trident installation. Logs are written to stdout.
func initInstallerLogging() {
	var err error
	initCmdLogging()

	logLevel := GetDefaultLogLevel()
	if silent {
		logLevel = "fatal"
		err = InitLogLevel(logLevel)
		if err != nil {
			Log().WithField("error", err).Fatal("Failed to initialize logging.")
		}
	}

	// Installer logs to stdout only
	InitLogOutput(os.Stdout)
	InitLogFormatter(&log.TextFormatter{DisableTimestamp: true})
	// Set provided logging workflows.
	if err = SetWorkflows(logWorkflows); err != nil {
		Log().WithField("error", err).Fatal("Failed to initialize logging while setting log workflows.")
	}

	if err = SetLogLayers(logLayers); err != nil {
		Log().WithField("error", err).Fatal("Failed to initialize logging while setting log layers.")
	}

	Log().WithField("logLevel", LogLevel).Debug("Initialized logging.")
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
			tridentImage = network.ReplaceImageRegistry(tridentImage, imageRegistry)
		}
	}
	Log().Debugf("Trident image: %s", tridentImage)

	if autosupportImage == "" {
		autosupportImage = tridentconfig.DefaultAutosupportImage

		if imageRegistry != "" {
			autosupportImage = network.ReplaceImageRegistry(autosupportImage, imageRegistry)
		}
	}
	Log().Debugf("Autosupport image: %s", autosupportImage)

	// Create the Kubernetes client
	if client, err = initClient(); err != nil {
		return fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// Before the installation ensure K8s version is valid
	err = tridentconfig.ValidateKubernetesVersion(tridentconfig.KubernetesVersionMin, client.ServerVersion())
	if err != nil {
		if versionutils.IsUnsupportedKubernetesVersionError(err) {
			Log().Errorf("Kubernetes version %s is an %v. NetApp will not take Support calls or "+
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
			Log().WithFields(LogFields{
				"example": fmt.Sprintf("./tridentctl install -n %s", PreferredNamespace),
			}).Warning("For maximum security, we recommend running Trident in its own namespace.")
		}
	}

	// Direct all subsequent client commands to the chosen namespace
	client.SetNamespace(TridentPodNamespace)

	Log().WithFields(LogFields{
		"installationNamespace": TridentPodNamespace,
		"KubernetesVersion":     client.ServerVersion().String(),
	}).Debug("Validated installation environment.")

	return nil
}

func initClient() (k8sclient.KubernetesClient, error) {
	clients, err := k8sclient.CreateK8SClients("", KubeConfigPath, "")
	if err != nil {
		return nil, err
	}

	clients.K8SClient.SetTimeout(k8sTimeout)

	return clients.K8SClient, nil
}

func processInstallationArguments(_ *cobra.Command) {
	appLabel = TridentCSILabel
	appLabelKey = TridentCSILabelKey
	appLabelValue = TridentCSILabelValue

	persistentObjectLabelKey = TridentPersistentObjectLabelKey
	persistentObjectLabelValue = TridentPersistentObjectLabelValue
	nodePrep = protocol.FormatProtocols(nodePrep)
}

func validateInstallationArguments() error {
	labelFormat := "a DNS-1123 label must consist of lower case alphanumeric characters or '-', " +
		"and must start and end with an alphanumeric character"

	if !network.DNS1123LabelRegex.MatchString(TridentPodNamespace) {
		return fmt.Errorf("'%s' is not a valid namespace name; %s", TridentPodNamespace, labelFormat)
	}

	if err := protocol.ValidateProtocols(nodePrep); err != nil {
		return err
	}

	switch logFormat {
	case "text", "json":
		break
	default:
		return fmt.Errorf("'%s' is not a valid log format", logFormat)
	}

	switch v1.PullPolicy(imagePullPolicy) {
	// If the value of imagePullPolicy is either of PullIfNotPresent, PullAlways or PullNever then the imagePullPolicy
	// is valid and no action is required.
	case v1.PullIfNotPresent, v1.PullAlways, v1.PullNever:
	default:
		return fmt.Errorf("'%s' is not a valid trident image pull policy", imagePullPolicy)
	}

	// Validate the cloud provider
	if !(cloudProvider == "" || strings.EqualFold(cloudProvider, k8sclient.CloudProviderAzure) || strings.EqualFold(cloudProvider,
		k8sclient.CloudProviderAWS) || strings.EqualFold(cloudProvider, k8sclient.CloudProviderGCP)) {
		return fmt.Errorf("'%s' is not a valid cloud provider ", cloudProvider)
	}

	// Validate the flags to set the Workload Identity/IAM Role as annotation.
	// cloud provider cannot be empty.
	if cloudProvider == "" && cloudIdentity != "" {
		return fmt.Errorf("cloud provider must be specified for the cloud identity '%s'", cloudIdentity)
	}

	// Validate the cloud provider and cloud identity for Azure.
	if strings.EqualFold(cloudProvider, k8sclient.CloudProviderAzure) && strings.Contains(cloudIdentity, k8sclient.AzureCloudIdentityKey) {
		// Add identity label.
		identityLabel = true

		// Set the value of cloud provider as empty to avoid updating deployment yaml.
		cloudProvider = ""
	} else if strings.EqualFold(cloudProvider, k8sclient.CloudProviderAzure) && cloudIdentity != "" && !strings.Contains(cloudIdentity, k8sclient.AzureCloudIdentityKey) {
		return fmt.Errorf("'%s' is not a valid cloud identity for the cloud provider '%s'", cloudIdentity, k8sclient.CloudProviderAzure)
	}

	// Validate the cloud provider and cloud identity for AWS.
	if strings.EqualFold(cloudProvider, k8sclient.CloudProviderAWS) && !strings.Contains(cloudIdentity, k8sclient.AWSCloudIdentityKey) {
		return fmt.Errorf("'%s' is not a valid cloud identity for the cloud provider '%s'", cloudIdentity, k8sclient.CloudProviderAWS)
	}

	// Validate the cloud provider and cloud identity for GCP.
	if strings.EqualFold(cloudProvider, k8sclient.CloudProviderGCP) && !strings.Contains(cloudIdentity, k8sclient.GCPCloudIdentityKey) {
		return fmt.Errorf("'%s' is not a valid cloud identity for the cloud provider '%s'", cloudIdentity, k8sclient.CloudProviderGCP)
	}

	// Validate the fsGroupPolicy
	switch strings.ToLower(fsGroupPolicy) {
	case "":
		break
	case strings.ToLower(string(storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy)):
		break
	case strings.ToLower(string(storagev1.NoneFSGroupPolicy)):
		break
	case strings.ToLower(string(storagev1.FileFSGroupPolicy)):
		break
	default:
		return fmt.Errorf("'%s' is not a valid fsGroupPolicy", fsGroupPolicy)
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

	controllerClusterRoleBindingPath = path.Join(setupPath, ControllerClusterRoleBindingFilename)
	controllerRolePath = path.Join(setupPath, ControllerRoleFilename)
	controllerClusterRolePath = path.Join(setupPath, ControllerClusterRoleFilename)
	controllerRoleBindingPath = path.Join(setupPath, ControllerRoleBindingFilename)
	controllerServiceAccountPath = path.Join(setupPath, ControllerServiceAccountFilename)

	nodeLinuxRolePath = path.Join(setupPath, NodeLinuxRoleFilename)
	nodeLinuxRoleBindingPath = path.Join(setupPath, NodeLinuxRoleBindingFilename)
	nodeLinuxServiceAccountPath = path.Join(setupPath, NodeLinuxServiceAccountFilename)

	nodeWindowsRolePath = path.Join(setupPath, NodeWindowsRoleFilename)
	nodeWindowsRoleBindingPath = path.Join(setupPath, NodeWindowsRoleBindingFilename)
	nodeWindowsServiceAccountPath = path.Join(setupPath, NodeWindowsServiceAccountFilename)

	crdsPath = path.Join(setupPath, CRDsFilename)
	servicePath = path.Join(setupPath, ServiceFilename)
	daemonsetPath = path.Join(setupPath, DaemonSetFilename)
	deploymentPath = path.Join(setupPath, DeploymentFilename)
	resourceQuotaPath = path.Join(setupPath, ResourceQuotaFilename)
	windowsDaemonSetPath = path.Join(setupPath, WindowsDaemonSetFilename)

	setupYAMLPaths = []string{
		namespacePath,
		controllerServiceAccountPath,
		nodeLinuxServiceAccountPath,
		nodeWindowsServiceAccountPath,
		controllerRolePath,
		controllerRoleBindingPath,
		controllerClusterRolePath,
		nodeLinuxRolePath,
		nodeWindowsRolePath,
		controllerClusterRoleBindingPath,
		nodeLinuxRoleBindingPath,
		nodeWindowsRoleBindingPath,
		crdsPath,
		deploymentPath,
		servicePath,
		daemonsetPath,
		windowsDaemonSetPath,
		resourceQuotaPath,
	}

	if client.Flavor() == k8sclient.FlavorOpenShift {
		controllerSCCPath = path.Join(setupPath, ControllerSCCFilename)
		nodeLinuxSCCPath = path.Join(setupPath, NodeLinuxSCCFilename)
		nodeWindowsSCCPath = path.Join(setupPath, NodeWindowsSCCFilename)
		setupYAMLPaths = append(setupYAMLPaths, controllerSCCPath, nodeLinuxSCCPath, nodeWindowsSCCPath)
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

	namespaceYAML := k8sclient.GetNamespaceYAML(TridentPodNamespace)
	if err = writeFile(namespacePath, namespaceYAML); err != nil {
		return fmt.Errorf("could not write namespace YAML file; %v", err)
	}

	// Creating Controller RBAC objects
	// Creating service account for controller
	controllerServiceAccountYAML := k8sclient.GetServiceAccountYAML(getControllerRBACResourceName(), nil, labels,
		nil, cloudIdentity)
	if err = writeFile(controllerServiceAccountPath, controllerServiceAccountYAML); err != nil {
		return fmt.Errorf("could not write controller service account YAML file; %v", err)
	}
	// Creating cluster role for controller service account
	controllerClusterRoleYAML := k8sclient.GetClusterRoleYAML(getControllerRBACResourceName(), labels, nil)
	if err = writeFile(controllerClusterRolePath, controllerClusterRoleYAML); err != nil {
		return fmt.Errorf("could not write controller cluster role YAML file; %v", err)
	}

	// Creating role (trident-namespaced) for controller
	controllerRoleYAML := k8sclient.GetRoleYAML(TridentPodNamespace, getControllerRBACResourceName(), labels, nil)
	if err = writeFile(controllerRolePath, controllerRoleYAML); err != nil {
		return fmt.Errorf("could not write controller role YAML file; %v", err)
	}

	// Creating role-binding (trident-namespaced) for controller
	controllerRoleBindingYAML := k8sclient.GetRoleBindingYAML(TridentPodNamespace,
		getControllerRBACResourceName(), labels, nil)
	if err = writeFile(controllerRoleBindingPath, controllerRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write controller role binding YAML file; %v", err)
	}

	// Creating cluster role binding for controller service account
	controllerClusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace,
		getControllerRBACResourceName(), client.Flavor(), labels, nil)
	if err = writeFile(controllerClusterRoleBindingPath, controllerClusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write controller cluster role binding YAML file; %v", err)
	}

	// Creating Linux Node RBAC objects
	// Creating service account for node linux
	nodeServiceAccountYAML := k8sclient.GetServiceAccountYAML(getNodeRBACResourceName(false),
		nil, daemonSetlabels, nil, "")
	if err = writeFile(nodeLinuxServiceAccountPath, nodeServiceAccountYAML); err != nil {
		return fmt.Errorf("could not write node linux service account YAML file; %v", err)
	}

	crdsYAML := k8sclient.GetCRDsYAML()
	if err = writeFile(crdsPath, crdsYAML); err != nil {
		return fmt.Errorf("could not write custom resource definition YAML file; %v", err)
	}

	serviceYAML := k8sclient.GetCSIServiceYAML(getServiceName(), labels, nil)
	if err = writeFile(servicePath, serviceYAML); err != nil {
		return fmt.Errorf("could not write service YAML file; %v", err)
	}

	// DaemonSetlabels are used because this object is used by the DaemonSet / Node Pods.
	resourceQuotaYAML := k8sclient.GetResourceQuotaYAML(getResourceQuotaName(), TridentPodNamespace, daemonSetlabels,
		nil)
	if err = writeFile(resourceQuotaPath, resourceQuotaYAML); err != nil {
		return fmt.Errorf("could not write resource quota YAML file; %v", err)
	}

	deploymentArgs := &k8sclient.DeploymentYAMLArguments{
		DeploymentName:          getDeploymentName(),
		TridentImage:            tridentImage,
		AutosupportImage:        autosupportImage,
		AutosupportProxy:        autosupportProxy,
		AutosupportInsecure:     autosupportInsecure,
		AutosupportCustomURL:    autosupportCustomURL,
		AutosupportSerialNumber: autosupportSerialNumber,
		AutosupportHostname:     autosupportHostname,
		ImageRegistry:           imageRegistry,
		LogFormat:               logFormat,
		DisableAuditLog:         disableAuditLog,
		Debug:                   useDebug,
		LogLevel:                LogLevel,
		LogLayers:               logLayers,
		LogWorkflows:            logWorkflows,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    nil,
		UseIPv6:                 useIPv6,
		SilenceAutosupport:      silenceAutosupport,
		ExcludeAutosupport:      excludeAutosupport,
		Version:                 client.ServerVersion(),
		HTTPRequestTimeout:      httpRequestTimeout.String(),
		ServiceAccountName:      getControllerRBACResourceName(),
		ImagePullPolicy:         imagePullPolicy,
		EnableForceDetach:       enableForceDetach,
		ACPImage:                acpImage,
		EnableACP:               enableACP,
		CloudProvider:           cloudProvider,
		IdentityLabel:           identityLabel,
		K8sAPIQPS:               k8sAPIQPS,
		EnableConcurrency:       enableConcurrency,
	}
	deploymentYAML := k8sclient.GetCSIDeploymentYAML(deploymentArgs)
	if err = writeFile(deploymentPath, deploymentYAML); err != nil {
		return fmt.Errorf("could not write deployment YAML file; %v", err)
	}

	daemonArgs := &k8sclient.DaemonsetYAMLArguments{
		DaemonsetName:            getDaemonSetName(false),
		TridentImage:             tridentImage,
		ImageRegistry:            imageRegistry,
		KubeletDir:               kubeletDir,
		LogFormat:                logFormat,
		DisableAuditLog:          disableAuditLog,
		Debug:                    useDebug,
		LogLevel:                 LogLevel,
		LogWorkflows:             logWorkflows,
		LogLayers:                logLayers,
		ProbePort:                strconv.FormatInt(probePort, 10),
		ImagePullSecrets:         imagePullSecrets,
		Labels:                   daemonSetlabels,
		ControllingCRDetails:     nil,
		EnableForceDetach:        enableForceDetach,
		Version:                  client.ServerVersion(),
		HTTPRequestTimeout:       httpRequestTimeout.String(),
		ServiceAccountName:       getNodeRBACResourceName(false),
		ImagePullPolicy:          imagePullPolicy,
		ISCSISelfHealingInterval: iscsiSelfHealingInterval.String(),
		ISCSISelfHealingWaitTime: iscsiSelfHealingWaitTime.String(),
		NodePrep:                 nodePrep,
	}
	daemonSetYAML := k8sclient.GetCSIDaemonSetYAMLLinux(daemonArgs)
	if err = writeFile(daemonsetPath, daemonSetYAML); err != nil {
		return fmt.Errorf("could not write DaemonSet YAML file; %v", err)
	}

	// If OpenShift, generate corresponding SCCs
	if client.Flavor() == k8sclient.FlavorOpenShift {
		// Creating trident controller security context constraint (SCC)
		controllerSCCYAML := k8sclient.GetOpenShiftSCCYAML(getControllerRBACResourceName(), getControllerRBACResourceName(), TridentPodNamespace, labels, nil,
			isLinuxNodeSCCUser(getControllerRBACResourceName()))
		if err = writeFile(controllerSCCPath, controllerSCCYAML); err != nil {
			return fmt.Errorf("could not write controller SCC YAML file; %v", err)
		}

		// Creating trident node security context constraint (SCC)
		nodeLinuxSCCYAML := k8sclient.GetOpenShiftSCCYAML(getNodeRBACResourceName(false), getNodeRBACResourceName(false), TridentPodNamespace, daemonSetlabels, nil,
			isLinuxNodeSCCUser(getNodeRBACResourceName(false)))
		if err = writeFile(nodeLinuxSCCPath, nodeLinuxSCCYAML); err != nil {
			return fmt.Errorf("could not write node linux SCC YAML file; %v", err)
		}
	}

	if windows {
		daemonArgs = &k8sclient.DaemonsetYAMLArguments{
			DaemonsetName:        getDaemonSetName(true),
			TridentImage:         tridentImage,
			ImageRegistry:        imageRegistry,
			KubeletDir:           kubeletDir,
			LogFormat:            logFormat,
			DisableAuditLog:      disableAuditLog,
			Debug:                useDebug,
			LogLevel:             LogLevel,
			LogWorkflows:         logWorkflows,
			LogLayers:            logLayers,
			ProbePort:            strconv.FormatInt(probePort, 10),
			ImagePullSecrets:     imagePullSecrets,
			Labels:               daemonSetlabels,
			ControllingCRDetails: nil,
			Version:              client.ServerVersion(),
			HTTPRequestTimeout:   httpRequestTimeout.String(),
			ServiceAccountName:   getNodeRBACResourceName(true),
			ImagePullPolicy:      imagePullPolicy,
			NodePrep:             nodePrep,
		}
		windowsDaemonSetYAML := k8sclient.GetCSIDaemonSetYAMLWindows(daemonArgs)
		if err = writeFile(windowsDaemonSetPath, windowsDaemonSetYAML); err != nil {
			return fmt.Errorf("could not write DaemonSet Windows YAML file; %v", err)
		}
		// Creating node windows RBAC objects
		// Creating service account for node windows
		nodeWindowsServiceAccountYAML := k8sclient.GetServiceAccountYAML(getNodeRBACResourceName(true), nil,
			daemonSetlabels, nil, "")
		if err = writeFile(nodeWindowsServiceAccountPath, nodeWindowsServiceAccountYAML); err != nil {
			return fmt.Errorf("could not write node windows service account YAML file; %v", err)
		}

		if client.Flavor() == k8sclient.FlavorOpenShift {
			nodeWindowsSCCYAML := k8sclient.GetOpenShiftSCCYAML(getNodeRBACResourceName(true), getNodeRBACResourceName(true), TridentPodNamespace, daemonSetlabels, nil,
				isLinuxNodeSCCUser(getNodeRBACResourceName(true)))
			if err = writeFile(nodeWindowsSCCPath, nodeWindowsSCCYAML); err != nil {
				return fmt.Errorf("could not write node windows SCC YAML file; %v", err)
			}
		}
	}

	return nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func writeFile(filePath, data string) error {
	return os.WriteFile(filePath, []byte(data), 0o600)
}

func ensureSetupDirExists() error {
	if !fileExists(setupPath) {
		if err := os.MkdirAll(setupPath, os.ModePerm); err != nil {
			return err
		}
		Log().WithField("path", setupPath).Info("Created setup directory.")
	}
	return nil
}

func installTrident() (returnError error) {
	var logFields LogFields

	labels := make(map[string]string)
	labels[appLabelKey] = appLabelValue

	nodeLabels := make(map[string]string)
	nodeLabels[appLabelKey] = TridentNodeLabelValue

	persistentObjectLabels := make(map[string]string)
	persistentObjectLabels[appLabelKey] = appLabelValue
	persistentObjectLabels[persistentObjectLabelKey] = persistentObjectLabelValue

	if client == nil {
		return fmt.Errorf("not able to connect to Kubernetes API server")
	}

	// Ensure CSI Trident isn't already installed
	if installed, namespace, err := isCSITridentInstalled(); err != nil {
		return fmt.Errorf("could not check if CSI Trident deployment exists; %v", err)
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
		Log().WithField("namespace", TridentPodNamespace).Debug("Namespace exists.")
	} else {
		Log().WithField("namespace", TridentPodNamespace).Debug("Namespace does not exist.")
	}

	// Discover CRD data
	var installedCRDs []string
	for _, crdName := range CRDnames {
		// See if any of the CRDs exist
		var crdExists bool
		crdExists, returnError = client.CheckCRDExists(crdName)
		if returnError != nil {
			return
		}
		if crdExists {
			Log().WithField("CRD", crdName).Debug("CRD present.")
			installedCRDs = append(installedCRDs, crdName)
		} else {
			Log().WithField("CRD", crdName).Debug("CRD not present.")
		}
	}

	// Automatically enable CSI feature gates.
	csiFeatureGateYAMLSnippets, err := k8sclient.ConstructCSIFeatureGateYAMLSnippets(client)
	if err != nil {
		Log().WithError(err).Debug("Could not enable some CSI feature gates.")
	} else {
		Log().WithField("featureGates", csiFeatureGateYAMLSnippets).Debug("Enabling CSI feature gates.")
	}

	// All checks succeeded, so proceed with installation
	Log().WithField("namespace", TridentPodNamespace).Info("Starting Trident installation.")

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

	// Create or update CRDs and ensure they are established.
	if returnError = createAndEnsureCRDs(); returnError != nil {
		Log().Errorf("could not create or update the Trident CRDs; %v", returnError)
		return
	}

	// Patch the CRD definitions with finalizers to protect them
	if returnError = protectCustomResourceDefinitions(); returnError != nil {
		return
	}

	// Create the CSI Driver object
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
		logFields = LogFields{"path": servicePath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetCSIServiceYAML(getServiceName(), labels, nil))
		logFields = LogFields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident service; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created Trident service.")
	logFields = LogFields{}

	// Check if Trident's Encryption Secret already exists in the specified namespace.
	secretExists, err := client.CheckSecretExists(getEncryptionSecretName())
	if err != nil {
		returnError = fmt.Errorf("could not check for existing Trident encryption secret; %v", err)
		return
	}

	// If the encryption secret doesn't exist create a new one.
	if !secretExists {
		aesKey, err := crypto.GenerateAESKey()
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
		Log().WithFields(logFields).Info("Created Trident encryption secret.")
	}

	// Create the certificates for the CSI controller's HTTPS REST interface
	certInfo, err := crypto.MakeHTTPCertInfo(
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
	Log().WithFields(logFields).Info("Created Trident protocol secret.")

	err = client.CreateObjectByYAML(k8sclient.GetResourceQuotaYAML(getResourceQuotaName(), TridentPodNamespace,
		nodeLabels, nil))
	if err != nil {
		returnError = fmt.Errorf("could not create Trident resource quota; %v", err)
		return
	}
	Log().WithFields(logFields).Info("Created Trident resource quota.")

	// Remove any previous deployments found of name 'trident-csi'
	if found, _ := client.CheckDeploymentExists(getDeploymentName(), TridentPodNamespace); found {
		if err := client.DeleteDeployment(getDeploymentName(), TridentPodNamespace, true); err != nil {
			returnError = fmt.Errorf("could not delete previous Trident deployment; %v", err)
			return
		}
	}

	// ACP is obsolete now, so log a message and move on.
	if acpImage != "" || enableACP {
		Log().WithFields(LogFields{
			"acpImage":  acpImage,
			"enableACP": enableACP,
		}).Info("ACP is now obsolete; All workflows are now enabled by default.")
	}

	// excludeAutosupport completely removes the autosupport container from the deployment.
	if excludeAutosupport {
		Log().WithFields(LogFields{
			"excludeAutosupport": excludeAutosupport,
		}).Warn("Autosupport bundles cannot be generated.")
	}

	// Create the deployment
	if useYAML && fileExists(deploymentPath) {
		returnError = validateTridentDeployment()
		if returnError != nil {
			returnError = fmt.Errorf("please correct the deployment YAML file; %v", returnError)
			return
		}
		returnError = client.CreateObjectByFile(deploymentPath)
		logFields = LogFields{"path": deploymentPath}
	} else {
		deploymentArgs := &k8sclient.DeploymentYAMLArguments{
			DeploymentName:          getDeploymentName(),
			TridentImage:            tridentImage,
			AutosupportImage:        autosupportImage,
			AutosupportProxy:        autosupportProxy,
			AutosupportInsecure:     autosupportInsecure,
			AutosupportCustomURL:    autosupportCustomURL,
			AutosupportSerialNumber: autosupportSerialNumber,
			AutosupportHostname:     autosupportHostname,
			ImageRegistry:           imageRegistry,
			LogFormat:               logFormat,
			DisableAuditLog:         disableAuditLog,
			Debug:                   useDebug,
			LogLevel:                LogLevel,
			LogWorkflows:            logWorkflows,
			LogLayers:               logLayers,
			ImagePullSecrets:        imagePullSecrets,
			Labels:                  labels,
			ControllingCRDetails:    nil,
			UseIPv6:                 useIPv6,
			SilenceAutosupport:      silenceAutosupport,
			ExcludeAutosupport:      excludeAutosupport,
			Version:                 client.ServerVersion(),
			HTTPRequestTimeout:      httpRequestTimeout.String(),
			ServiceAccountName:      getControllerRBACResourceName(),
			ImagePullPolicy:         imagePullPolicy,
			EnableForceDetach:       enableForceDetach,
			ACPImage:                acpImage,
			EnableACP:               enableACP,
			CloudProvider:           cloudProvider,
			IdentityLabel:           identityLabel,
			K8sAPIQPS:               k8sAPIQPS,
			EnableConcurrency:       enableConcurrency,
			CSIFeatureGates:         csiFeatureGateYAMLSnippets,
		}
		returnError = client.CreateObjectByYAML(
			k8sclient.GetCSIDeploymentYAML(deploymentArgs))
		logFields = LogFields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident deployment; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created Trident deployment.")

	// Create the DaemonSet
	if windows {
		// Remove any previous windows daemonset found of name 'trident-csi'
		if found, _ := client.CheckDaemonSetExists(getDaemonSetName(true), TridentPodNamespace); found {
			if err := client.DeleteDaemonSet(getDaemonSetName(true), TridentPodNamespace, true); err != nil {
				returnError = fmt.Errorf("could not delete previous Trident windows daemonset; %v", err)
				return
			}
		}
		if useYAML && fileExists(windowsDaemonSetPath) {
			returnError = validateTridentDaemonSet(windowsDaemonSetPath)
			if returnError != nil {
				returnError = fmt.Errorf("please correct the daemonset YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(windowsDaemonSetPath)
			logFields = LogFields{"path": windowsDaemonSetPath}
		} else {
			daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
				DaemonsetName:        getDaemonSetName(true),
				TridentImage:         tridentImage,
				ImageRegistry:        imageRegistry,
				KubeletDir:           kubeletDir,
				LogFormat:            logFormat,
				DisableAuditLog:      disableAuditLog,
				Debug:                useDebug,
				LogLevel:             LogLevel,
				LogWorkflows:         logWorkflows,
				LogLayers:            logLayers,
				ProbePort:            strconv.FormatInt(probePort, 10),
				ImagePullSecrets:     imagePullSecrets,
				Labels:               nodeLabels,
				ControllingCRDetails: nil,
				Version:              client.ServerVersion(),
				HTTPRequestTimeout:   httpRequestTimeout.String(),
				ServiceAccountName:   getNodeRBACResourceName(true),
				ImagePullPolicy:      imagePullPolicy,
				NodePrep:             nodePrep,
			}
			returnError = client.CreateObjectByYAML(
				k8sclient.GetCSIDaemonSetYAMLWindows(daemonSetArgs))
			logFields = LogFields{}

			if returnError != nil {
				returnError = fmt.Errorf("could not create Trident daemonset; %v", returnError)
				return
			}
		}
	}

	// Remove any previous linux daemonset found of name 'trident-csi'
	if found, _ := client.CheckDaemonSetExists(getDaemonSetName(false), TridentPodNamespace); found {
		if err := client.DeleteDaemonSet(getDaemonSetName(false), TridentPodNamespace, true); err != nil {
			returnError = fmt.Errorf("could not delete previous Trident linux daemonset; %v", err)
			return
		}
	}

	// Create the DaemonSet
	if useYAML && fileExists(daemonsetPath) {
		returnError = validateTridentDaemonSet(daemonsetPath)
		if returnError != nil {
			returnError = fmt.Errorf("please correct the daemonset YAML file; %v", returnError)
			return
		}
		returnError = client.CreateObjectByFile(daemonsetPath)
		logFields = LogFields{"path": daemonsetPath}
	} else {
		daemonSetlabels := make(map[string]string)
		daemonSetlabels[appLabelKey] = TridentNodeLabelValue
		daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
			DaemonsetName:            getDaemonSetName(false),
			TridentImage:             tridentImage,
			ImageRegistry:            imageRegistry,
			KubeletDir:               kubeletDir,
			LogFormat:                logFormat,
			DisableAuditLog:          disableAuditLog,
			Debug:                    useDebug,
			LogLevel:                 LogLevel,
			LogWorkflows:             logWorkflows,
			LogLayers:                logLayers,
			ProbePort:                strconv.FormatInt(probePort, 10),
			ImagePullSecrets:         imagePullSecrets,
			Labels:                   daemonSetlabels,
			ControllingCRDetails:     nil,
			EnableForceDetach:        enableForceDetach,
			Version:                  client.ServerVersion(),
			HTTPRequestTimeout:       httpRequestTimeout.String(),
			ServiceAccountName:       getNodeRBACResourceName(false),
			ImagePullPolicy:          imagePullPolicy,
			ISCSISelfHealingInterval: iscsiSelfHealingInterval.String(),
			ISCSISelfHealingWaitTime: iscsiSelfHealingWaitTime.String(),
			NodePrep:                 nodePrep,
		}
		returnError = client.CreateObjectByYAML(
			k8sclient.GetCSIDaemonSetYAMLLinux(daemonSetArgs))
		logFields = LogFields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident daemonset; %v", returnError)
		return
	}

	Log().WithFields(logFields).Info("Created Trident daemonset.")

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

	Log().Info("Trident installation succeeded.")
	return nil
}

func patchNamespace() error {
	labels := map[string]string{
		tridentconfig.PodSecurityStandardsEnforceLabel: tridentconfig.PodSecurityStandardsEnforceProfile,
	}
	return client.PatchNamespaceLabels(TridentPodNamespace, labels)
}

func createNamespace() (returnError error) {
	var logFields LogFields

	if useYAML && fileExists(namespacePath) {
		returnError = client.CreateObjectByFile(namespacePath)
		logFields = LogFields{"path": namespacePath}
	} else {
		returnError = client.CreateObjectByYAML(k8sclient.GetNamespaceYAML(TridentPodNamespace))
		logFields = LogFields{"namespace": TridentPodNamespace}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create namespace %s; %v", TridentPodNamespace, returnError)
		return
	}
	Log().WithFields(logFields).Info("Created namespace.")
	return nil
}

// createAndEnsureCRDs creates CRDs or updates existing ones. If we are on an upgrade install, CRDs should be updated.
func createAndEnsureCRDs() error {
	var bundleCRDYAML string
	if useYAML && fileExists(crdsPath) {

		content, err := os.ReadFile(crdsPath)
		if err != nil {
			return err
		}

		bundleCRDYAML = string(content)
	} else {
		bundleCRDYAML = k8sclient.GetCRDsYAML()
	}

	crdMap := getCRDMapFromBundle(bundleCRDYAML)

	// Ensure crdMap has all the CRDs, nothing is missing or extra.
	if err := validateCRDs(crdMap); err != nil {
		return fmt.Errorf("could not create the Trident CRDs; %v", err)
	}

	Log().Info("Creating or patching the Trident CRDs.")

	// Loop through all CRDs and check if each exists; if it does, update it. Otherwise, create it.
	for crdName, crdYAML := range crdMap {

		crdExists, err := client.CheckCRDExists(crdName)
		if err != nil {
			return fmt.Errorf("unable to identify if %v CRD exists; err: %v", crdName, err)
		}

		if crdExists {
			Log().Debugf("%v CRD present; patching CRD to ensure it is not stale.", crdName)

			if err = patchCRD(crdName, crdYAML); err != nil {
				return fmt.Errorf("could not patch the Trident %s CRD; %v", crdName, err)
			}
		} else {
			Log().Debugf("%v CRD not present; creating a fresh CRD.", crdName)

			if err = createCRD(crdName, crdYAML); err != nil {
				return fmt.Errorf("could not create the Trident %s CRD; %v", crdName, err)
			}
		}
	}

	Log().Info("Applied latest Trident CRDs.")

	return nil
}

// getCRDMapFromBundle creates a map of CRD name to CRD definition from the bundle
func getCRDMapFromBundle(bundle string) map[string]string {
	labelEqualRegex := regexp.MustCompile(`(?m)^\s*name:\s*(?P<crdName>[\w.]+)$`)
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
		Log().Errorf(errMsg)
		return fmt.Errorf("CRD validation failed; %v", errMsg)
	}

	return nil
}

// createCRD creates and establishes the CRD.
func createCRD(crdName, crdYAML string) error {
	var returnError error

	// Create the CRDs and wait for them to be registered in Kubernetes.
	Log().Debugf("Installer will create a fresh %v CRD.", crdName)

	if returnError = createCustomResourceDefinition(crdName, crdYAML); returnError != nil {
		return returnError
	}

	// Wait for the CRD to be fully established.
	if returnError = ensureCRDEstablished(crdName); returnError != nil {
		// If CRD registration failed *and* we created the CRDs, clean up by deleting the CRD.
		Log().Errorf("CRD %v not established; %v", crdName, returnError)
		if err := deleteCustomResourceDefinition(crdName, crdYAML); err != nil {
			Log().Errorf("Could not delete CRD %v; %v", crdName, err)
		}

		return returnError
	}

	return returnError
}

func createCustomResourceDefinition(crdName, crdYAML string) (returnError error) {
	returnError = client.CreateObjectByYAML(crdYAML)
	if returnError != nil {
		returnError = fmt.Errorf("could not create custom resource %v in %s; %v", crdName, TridentPodNamespace,
			returnError)
		return
	}
	Log().Debugf("Created custom resource definition %v.", crdName)
	return nil
}

// patchCRD patches an existing CRD to meet Trident's expected specification in the installer.
func patchCRD(crdName, crdYAML string) error {
	currentCRD, err := client.GetCRD(crdName)
	if err != nil {
		return fmt.Errorf("could not retrieve the %v CRD; %v", crdName, err)
	}

	// Generate the deltas between the currentCRD and the new CRD YAML using a json merge patch strategy.
	patchBytes, err := k8sclient.GenericPatch(currentCRD, []byte(crdYAML))
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for %s CRD; %v", crdName, err)
	}

	if err = client.PatchCRD(crdName, patchBytes, types.MergePatchType); err != nil {
		return fmt.Errorf("could not patch custom resource %v in %s; %v", crdName, TridentPodNamespace, err)
	}
	Log().Debugf("Patched custom resource definition %v.", crdName)
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
		Log().WithFields(LogFields{
			"CRD": crdName,
			"err": err,
		}).Debug("CRD not yet established, waiting.")
	}

	checkCRDBackoff := backoff.NewExponentialBackOff()
	checkCRDBackoff.MaxInterval = 5 * time.Second
	checkCRDBackoff.MaxElapsedTime = k8sTimeout

	Log().WithField("CRD", crdName).Trace("Waiting for CRD to be established.")

	if err := backoff.RetryNotify(checkCRDEstablished, checkCRDBackoff, checkCRDNotify); err != nil {
		return fmt.Errorf("CRD was not established after %3.2f seconds", k8sTimeout.Seconds())
	}

	Log().WithField("CRD", crdName).Debug("CRD established.")
	return nil
}

func deleteCustomResourceDefinition(crdName, crdYAML string) (returnError error) {
	var logFields LogFields

	returnError = client.DeleteObjectByYAML(crdYAML, true)
	if returnError != nil {
		returnError = fmt.Errorf("could not delete custom resource definition %v in %s; %v", crdName,
			TridentPodNamespace,
			returnError)
		return
	}
	Log().WithFields(logFields).Infof("Deleted custom resource definition %v.", crdName)
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
	Log().Info("Added finalizers to custom resource definitions.")
	return nil
}

func createK8SCSIDriver() error {
	// Delete the object in case it already exists and we need to update it
	err := client.DeleteObjectByYAML(k8sclient.GetCSIDriverYAML(getCSIDriverName(), fsGroupPolicy, nil, nil), true)
	if err != nil {
		return fmt.Errorf("could not delete csidriver custom resource; %v", err)
	}

	err = client.CreateObjectByYAML(k8sclient.GetCSIDriverYAML(getCSIDriverName(), fsGroupPolicy, nil, nil))
	if err != nil {
		return fmt.Errorf("could not create csidriver custom resource; %v", err)
	}

	return nil
}

func createRBACObjects() (returnError error) {
	var logFields LogFields

	labels := make(map[string]string)
	labels[TridentCSILabelKey] = TridentCSILabelValue

	daemonSetlabels := make(map[string]string)
	daemonSetlabels[TridentNodeLabelKey] = TridentNodeLabelValue

	// Defining a closure to perform the creation of RBAC objects
	createObjectFunc := func(resourcePath, resourceYAML string) error {
		if useYAML && fileExists(resourcePath) {
			returnError = client.CreateObjectByFile(resourcePath)
			logFields = LogFields{"path": resourcePath}
		} else {
			returnError = client.CreateObjectByYAML(resourceYAML)
			logFields = LogFields{}
		}
		return returnError
	}

	// Creating controller RBAC Objects
	// Create service account for controller
	if createObjectFunc(controllerServiceAccountPath,
		k8sclient.GetServiceAccountYAML(getControllerRBACResourceName(), nil, labels, nil, cloudIdentity)) != nil {
		returnError = fmt.Errorf("could not create controller service account; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created controller service account.")

	// Create role (trident-namespaced) for controller
	if createObjectFunc(controllerRolePath,
		k8sclient.GetRoleYAML(TridentPodNamespace, getControllerRBACResourceName(),
			labels, nil)) != nil {
		returnError = fmt.Errorf("could not create controller role; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created controller role.")

	// Create role binding (trident-namespaced) for controller
	if createObjectFunc(controllerRoleBindingPath,
		k8sclient.GetRoleBindingYAML(TridentPodNamespace, getControllerRBACResourceName(), labels, nil)) != nil {
		returnError = fmt.Errorf("could not create controller role binding; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created controller role binding.")

	// Create cluster role for controller
	if createObjectFunc(controllerClusterRolePath,
		k8sclient.GetClusterRoleYAML(getControllerRBACResourceName(), labels, nil)) != nil {
		returnError = fmt.Errorf("could not create controller cluster role; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created controller cluster role.")

	// Create cluster role binding for controller
	if createObjectFunc(controllerClusterRoleBindingPath,
		k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, getControllerRBACResourceName(), client.Flavor(),
			labels, nil)) != nil {
		returnError = fmt.Errorf("could not create controller cluster role binding; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created controller cluster role binding.")

	// Creating node linux RBAC Objects
	// Create service account for node linux
	if createObjectFunc(nodeLinuxServiceAccountPath,
		k8sclient.GetServiceAccountYAML(getNodeRBACResourceName(false), nil, daemonSetlabels, nil, "")) != nil {
		returnError = fmt.Errorf("could not create node linux service account; %v", returnError)
		return
	}
	Log().WithFields(logFields).Info("Created node linux service account.")

	// Creating node windows RBAC Objects
	if windows {
		if createObjectFunc(nodeWindowsServiceAccountPath,
			k8sclient.GetServiceAccountYAML(getNodeRBACResourceName(true), nil, daemonSetlabels, nil, "")) != nil {
			returnError = fmt.Errorf("could not create node windows service account; %v", returnError)
			return
		}
		Log().WithFields(logFields).Info("Created node windows service account.")
	}

	// If OpenShift, add Trident to security context constraint(s)
	if client.Flavor() == k8sclient.FlavorOpenShift {
		sccUsers := []string{getControllerRBACResourceName(), getNodeRBACResourceName(false)}
		if windows {
			sccUsers = append(sccUsers, getNodeRBACResourceName(true))
		}
		for _, user := range sccUsers {
			// Identify the label to add based on the resource name
			_, label := getAppLabelForResource(user)
			// Create multiple SCC with name same as 'user'
			// This will be changed in the future to accomodate one SCC to hold all three users
			if returnError = CreateOpenShiftTridentSCC(user, strings.Split(label, "=")[1]); returnError != nil {
				returnError = fmt.Errorf("could not create security context constraint; %v", returnError)
				return
			}
			Log().WithFields(LogFields{
				"scc":  "trident",
				"user": user,
			}).Info("Created Trident's security context constraint.")
		}
	}

	return
}

func removeRBACObjects(logLevel log.Level) (anyErrors bool) {
	labels := make(map[string]string)
	labels[TridentCSILabelKey] = TridentCSILabelValue

	daemonSetlabels := make(map[string]string)
	daemonSetlabels[TridentNodeLabelKey] = TridentNodeLabelValue

	logFunc := func(fields LogFields) func(args ...interface{}) {
		if logLevel >= log.DebugLevel {
			return Log().WithFields(fields).Debug
		} else {
			return Log().WithFields(fields).Info
		}
	}

	deleteObjectFunc := func(resourceYAML, deletefailMsg, deleteSuccessMsg string) {
		if err := client.DeleteObjectByYAML(resourceYAML, true); err != nil {
			Log().WithField("error", err).Warning(deletefailMsg)
			anyErrors = true
		} else {
			logFunc(LogFields{})(deleteSuccessMsg)
		}
	}

	// Remove RBAC objects of name 'trident-csi' from previous installations if found
	// Delete service account
	deleteObjectFunc(
		k8sclient.GetServiceAccountYAML(getServiceAccountName(), nil, nil, nil, cloudIdentity),
		"Could not delete trident-csi service account.",
		"Deleted trident-csi service account.",
	)

	// Delete cluster role
	deleteObjectFunc(
		k8sclient.GetClusterRoleYAML(getClusterRoleName(), nil, nil),
		"Could not delete trident-csi cluster role.",
		"Deleted trident-csi cluster role.",
	)

	// Delete cluster role binding
	deleteObjectFunc(
		k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, getClusterRoleBindingName(), client.Flavor(), nil,
			nil),
		"Could not delete trident-csi cluster role binding.",
		"Deleted trident-csi cluster role binding.",
	)

	// Remove RBAC objects - ServiceAccount, ClusterRole, ClusterRoleBinding for each of the controller,
	// node linux and node windows components

	// DELETING Controller RBAC objects
	// Delete controller service account
	deleteObjectFunc(
		k8sclient.GetServiceAccountYAML(getControllerRBACResourceName(), nil, labels, nil, cloudIdentity),
		"Could not delete controller service account.",
		"Deleted controller service account.",
	)

	// Delete controller cluster role
	deleteObjectFunc(
		k8sclient.GetClusterRoleYAML(getControllerRBACResourceName(), labels, nil),
		"Could not delete controller cluster role.",
		"Deleted controller cluster role.",
	)

	// Delete controller cluster role binding
	deleteObjectFunc(
		k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, getControllerRBACResourceName(),
			client.Flavor(), labels, nil),
		"Could not delete controller cluster role binding.",
		"Deleted controller cluster role binding.",
	)

	// Delete controller role
	deleteObjectFunc(
		k8sclient.GetRoleYAML(TridentPodNamespace, getControllerRBACResourceName(), labels, nil),
		"Could not delete controller role.",
		"Deleted controller role.",
	)

	// Delete controller role binding
	deleteObjectFunc(
		k8sclient.GetRoleBindingYAML(TridentPodNamespace, getControllerRBACResourceName(), labels, nil),
		"Could not delete controller role binding.",
		"Deleted controller role binding.",
	)

	// DELETING node linux RBAC objects
	// Delete node linux service account
	deleteObjectFunc(
		k8sclient.GetServiceAccountYAML(getNodeRBACResourceName(false), nil, daemonSetlabels, nil, ""),
		"Could not delete node linux service account.",
		"Deleted node linux service account.",
	)

	// Delete node linux cluster role, role bindings
	deleteObjectFunc(
		k8sclient.GetClusterRoleYAML(getNodeRBACResourceName(false), daemonSetlabels, nil),
		"Could not delete node linux cluster role.",
		"Deleted node linux cluster role.",
	)

	deleteObjectFunc(
		k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, getNodeRBACResourceName(false), client.Flavor(),
			daemonSetlabels, nil),
		"Could not delete node linux cluster role binding.",
		"Deleted node linux cluster role binding.",
	)

	// Delete node linux role
	deleteObjectFunc(
		k8sclient.GetRoleYAML(TridentPodNamespace, getNodeRBACResourceName(false), daemonSetlabels, nil),
		"Could not delete node linux role.",
		"Deleted node linux role.",
	)

	// Delete node linux role binding
	deleteObjectFunc(
		k8sclient.GetRoleBindingYAML(TridentPodNamespace, getNodeRBACResourceName(false), daemonSetlabels, nil),
		"Could not delete node linux role binding.",
		"Deleted node linux role binding.",
	)

	// DELETING node windows RBAC objects
	// Delete node windows service account
	deleteObjectFunc(
		k8sclient.GetServiceAccountYAML(getNodeRBACResourceName(true), nil, daemonSetlabels, nil, ""),
		"Could not delete node windows service account.",
		"Deleted node windows service account.",
	)

	// Delete node windows cluster role, role bindings
	deleteObjectFunc(
		k8sclient.GetClusterRoleYAML(getNodeRBACResourceName(true), daemonSetlabels, nil),
		"Could not delete node windows cluster role.",
		"Deleted node windows cluster role.",
	)

	deleteObjectFunc(
		k8sclient.GetClusterRoleBindingYAML(TridentPodNamespace, getNodeRBACResourceName(true), client.Flavor(),
			daemonSetlabels, nil),
		"Could not delete node windows cluster role binding.",
		"Deleted node windows cluster role binding.",
	)

	// Delete node windows role
	deleteObjectFunc(
		k8sclient.GetRoleYAML(TridentPodNamespace, getNodeRBACResourceName(true), daemonSetlabels, nil),
		"Could not delete node windows role.",
		"Deleted node windows role.",
	)

	// Delete node windows role binding
	deleteObjectFunc(
		k8sclient.GetRoleBindingYAML(TridentPodNamespace, getNodeRBACResourceName(true), daemonSetlabels, nil),
		"Could not delete node windows role binding.",
		"Deleted node windows role binding.",
	)

	// If OpenShift, delete Trident's security context constraint
	if client.Flavor() == k8sclient.FlavorOpenShift {
		sccUsers := []string{
			TridentCSI,
			getControllerRBACResourceName(),
			getNodeRBACResourceName(false),
			getNodeRBACResourceName(true),
		}
		for _, sccUser := range sccUsers {
			_, label := getAppLabelForResource(sccUser)
			label = strings.Split(label, "=")[1]

			if err := DeleteOpenShiftTridentSCC(sccUser, label); err != nil {
				Log().WithField("error", err).Warning("Could not delete security context constraint.")
				anyErrors = true
			} else {
				logFunc(LogFields{
					"scc":  "trident",
					"user": sccUser,
				})("Deleted Trident's security context constraint.")
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

	tridentImage = ""
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == tridentconfig.ContainerTrident {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return fmt.Errorf("the Trident deployment must define the %s container", tridentconfig.ContainerTrident)
	} else {
		Log().WithField("image", tridentImage).Debug("Read Trident image from custom YAML.")
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

func validateTridentDaemonSet(yamlPath string) error {
	daemonset, err := readDaemonSetFromFile(yamlPath)
	if err != nil {
		return fmt.Errorf("could not load DaemonSet YAML file; %v", err)
	}

	// Check the DaemonSet label
	labels := daemonset.Labels
	if labels[TridentNodeLabelKey] != TridentNodeLabelValue {
		return fmt.Errorf("the Trident DaemonSet must have the label \"%s: %s\"",
			TridentNodeLabelKey, TridentNodeLabelValue)
	}

	// Check the pod label
	labels = daemonset.Spec.Template.Labels
	if labels[TridentNodeLabelKey] != TridentNodeLabelValue {
		return fmt.Errorf("the Trident DaemonSet's pod template must have the label \"%s: %s\"",
			TridentNodeLabelKey, TridentNodeLabelValue)
	}

	tridentImage := ""
	for _, container := range daemonset.Spec.Template.Spec.Containers {
		if container.Name == tridentconfig.ContainerTrident {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return fmt.Errorf("the Trident DaemonSet must define the %s container", tridentconfig.ContainerTrident)
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
		Log().WithFields(LogFields{
			"status":    err,
			"increment": duration,
		}).Debugf("Trident pod not yet running, waiting.")
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	Log().Info("Waiting for Trident pod to start.")

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

		Log().Error(strings.Join(errMessages, " "))
		return nil, err
	}

	Log().WithFields(LogFields{
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
		Log().WithFields(LogFields{
			"error":     err,
			"increment": duration,
		}).Debugf("REST interface not yet up, waiting.")
	}
	restBackoff := backoff.NewExponentialBackOff()
	restBackoff.MaxElapsedTime = k8sTimeout

	Log().Info("Waiting for Trident REST interface.")

	if err := backoff.RetryNotify(checkRESTInterface, restBackoff, restNotify); err != nil {
		Log().Errorf("Trident REST interface was not available after %3.2f seconds.", k8sTimeout.Seconds())
		return err
	}

	Log().WithField("version", version).Info("Trident REST interface is up.")

	return nil
}

// readDeploymentFromFile parses and returns a deployment object from a file.
func readDeploymentFromFile(filePath string) (*appsv1.Deployment, error) {
	var deployment appsv1.Deployment

	yamlBytes, err := os.ReadFile(filePath)
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
func readServiceFromFile(filePath string) (*v1.Service, error) {
	var service v1.Service

	yamlBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &service)
	if err != nil {
		return nil, err
	}
	return &service, nil
}

// readDaemonSetFromFile parses and returns a DaemonSet object from a file.
func readDaemonSetFromFile(filePath string) (*appsv1.DaemonSet, error) {
	var daemonset appsv1.DaemonSet

	yamlBytes, err := os.ReadFile(filePath)
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

	err := client.CreateObjectByYAML(k8sclient.GetOpenShiftSCCYAML(user, user, TridentPodNamespace, labels, nil,
		isLinuxNodeSCCUser(user)))
	if err != nil {
		return fmt.Errorf("cannot create trident's scc; %v", err)
	}
	return nil
}

// DeleteOpenShiftTridentSCC deletes the trident-only SCC that the trident user uses. This only works for OpenShift.
func DeleteOpenShiftTridentSCC(user, labelVal string) error {
	labels := make(map[string]string)
	labels["app"] = labelVal

	err := client.DeleteObjectByYAML(
		k8sclient.GetOpenShiftSCCYAML(user, user, TridentPodNamespace, labels, nil, isLinuxNodeSCCUser(user)), true)
	if err != nil {
		return fmt.Errorf("%s; %v", "could not delete trident's scc", err)
	}
	return nil
}

func getControllerRBACResourceName() string {
	return TridentControllerResourceName
}

func getNodeRBACResourceName(windows bool) string {
	if windows {
		return TridentNodeWindowsResourceName
	}
	return TridentNodeLinuxResourceName
}

func getServiceAccountName() string {
	return TridentCSI
}

func getClusterRoleName() string {
	return TridentCSI
}

func getClusterRoleBindingName() string {
	return TridentCSI
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

func getResourceQuotaName() string {
	return TridentCSI
}

func getDeploymentName() string {
	return TridentControllerResourceName
}

func getDaemonSetName(windows bool) string {
	if windows {
		return TridentNodeWindowsResourceName
	} else {
		return TridentNodeLinuxResourceName
	}
}

func getCSIDriverName() string {
	return CSIDriver
}

// getAppLabelForResource returns the right app labels for RBAC resource name passed
func getAppLabelForResource(resourceName string) (map[string]string, string) {
	var label string
	labelMap := make(map[string]string, 1)
	if !strings.Contains(resourceName, "node") {
		labelMap[TridentNodeLabelKey] = TridentCSILabelValue
		label = TridentCSILabel
	} else {
		labelMap[TridentCSILabelKey] = TridentNodeLabelValue
		label = TridentNodeLabel
	}
	return labelMap, label
}

func isLinuxNodeSCCUser(user string) bool {
	return user == TridentNodeLinuxResourceName
}
