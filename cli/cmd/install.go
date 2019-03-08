package cmd

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/ghodss/yaml"
	"github.com/go-logfmt/logfmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	frontendrest "github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
)

const (
	PreferredNamespace = tridentconfig.OrchestratorName
	DefaultPVCName     = tridentconfig.OrchestratorName
	DefaultPVName      = tridentconfig.OrchestratorName
	DefaultVolumeName  = tridentconfig.OrchestratorName
	DefaultVolumeSize  = "2Gi"

	BackendConfigFilename      = "backend.json"
	NamespaceFilename          = "trident-namespace.yaml"
	ServiceAccountFilename     = "trident-serviceaccount.yaml"
	ClusterRoleFilename        = "trident-clusterrole.yaml"
	ClusterRoleBindingFilename = "trident-clusterrolebinding.yaml"
	PVCFilename                = "trident-pvc.yaml"
	DeploymentFilename         = "trident-deployment.yaml"
	ServiceFilename            = "trident-service.yaml"
	StatefulSetFilename        = "trident-statefulset.yaml"
	DaemonSetFilename          = "trident-daemonset.yaml"
)

var (
	// CLI flags
	dryRun       bool
	generateYAML bool
	useYAML      bool
	silent       bool
	csi          bool
	inCluster    bool
	pvName       string
	pvcName      string
	volumeName   string
	volumeSize   string
	tridentImage string
	etcdImage    string
	k8sTimeout   time.Duration

	// CLI-based K8S client
	client k8sclient.Interface

	// File paths
	installerDirectoryPath string
	setupPath              string
	backendConfigFilePath  string
	namespacePath          string
	serviceAccountPath     string
	clusterRolePath        string
	clusterRoleBindingPath string
	pvcPath                string
	deploymentPath         string
	csiServicePath         string
	csiStatefulSetPath     string
	csiDaemonSetPath       string
	setupYAMLPaths         []string

	appLabel      string
	appLabelKey   string
	appLabelValue string

	dns1123LabelRegex  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	dns1123DomainRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

func init() {
	RootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run all the pre-checks, but don't install anything.")
	installCmd.Flags().BoolVar(&generateYAML, "generate-custom-yaml", false, "Generate YAML files, but don't install anything.")
	installCmd.Flags().BoolVar(&useYAML, "use-custom-yaml", false, "Use any existing YAML files that exist in setup directory.")
	installCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during installation.")
	installCmd.Flags().BoolVar(&csi, "csi", false, "Install CSI Trident (alpha, not for production clusters).")
	installCmd.Flags().BoolVar(&inCluster, "in-cluster", true, "Run the installer as a pod in the cluster.")
	installCmd.Flags().MarkHidden("in-cluster")

	installCmd.Flags().StringVar(&pvcName, "pvc", "", "The name of the PVC used by Trident.")
	installCmd.Flags().StringVar(&pvName, "pv", "", "The name of the PV used by Trident.")
	installCmd.Flags().StringVar(&volumeName, "volume-name", "", "The name of the storage volume used by Trident.")
	installCmd.Flags().StringVar(&volumeSize, "volume-size", DefaultVolumeSize, "The size of the storage volume used by Trident.")
	installCmd.Flags().StringVar(&tridentImage, "trident-image", "", "The Trident image to install.")
	installCmd.Flags().StringVar(&etcdImage, "etcd-image", "", "The etcd image to install.")

	installCmd.Flags().DurationVar(&k8sTimeout, "k8s-timeout", 180*time.Second, "The number of seconds to wait before timing out on Kubernetes operations.")
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {

		initInstallerLogging()

		if err := discoverInstallationEnvironment(); err != nil {
			log.Fatalf("Install pre-checks failed; %v", err)
		}
		processInstallationArguments()
		if err := validateInstallationArguments(); err != nil {
			log.Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {

		if generateYAML {

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

	// Ensure we're on Linux
	if runtime.GOOS != "linux" {
		return errors.New("the Trident installer only runs on Linux")
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

		return k8sclient.NewKubeClient(kubeConfig, namespace)

	} else {

		// The namespace file didn't exist, so assume we're outside a pod.  Create a CLI-based client.
		log.Debug("Running outside a pod, creating CLI-based client.")

		return k8sclient.NewKubectlClient("")
	}
}

func processInstallationArguments() {

	if pvcName == "" {
		if csi {
			pvcName = DefaultPVCName + "-csi"
		} else {
			pvcName = DefaultPVCName
		}
	}

	if pvName == "" {
		if csi {
			pvName = DefaultPVName + "-csi"
		} else {
			pvName = DefaultPVName
		}
	}

	if volumeName == "" {
		if csi {
			volumeName = DefaultVolumeName + "-csi"
		} else {
			volumeName = DefaultVolumeName
		}
	}

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

	return nil
}

// prepareYAMLFilePaths sets up the absolute file paths to all files
func prepareYAMLFilePaths() error {

	var err error

	// Get directory of installer
	installerDirectoryPath, err = filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return fmt.Errorf("could not determine installer working directory; %v", err)
	}

	setupPath = path.Join(installerDirectoryPath, "setup")
	backendConfigFilePath = path.Join(setupPath, BackendConfigFilename)
	namespacePath = path.Join(setupPath, NamespaceFilename)
	serviceAccountPath = path.Join(setupPath, ServiceAccountFilename)
	clusterRolePath = path.Join(setupPath, ClusterRoleFilename)
	clusterRoleBindingPath = path.Join(setupPath, ClusterRoleBindingFilename)
	pvcPath = path.Join(setupPath, PVCFilename)
	deploymentPath = path.Join(setupPath, DeploymentFilename)
	csiServicePath = path.Join(setupPath, ServiceFilename)
	csiStatefulSetPath = path.Join(setupPath, StatefulSetFilename)
	csiDaemonSetPath = path.Join(setupPath, DaemonSetFilename)

	setupYAMLPaths = []string{
		namespacePath, serviceAccountPath, clusterRolePath, clusterRoleBindingPath,
		pvcPath, deploymentPath, csiServicePath, csiStatefulSetPath, csiDaemonSetPath,
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

	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), client.ServerVersion(), false)
	if err = writeFile(clusterRolePath, clusterRoleYAML); err != nil {
		return fmt.Errorf("could not write cluster role YAML file; %v", err)
	}

	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(
		TridentPodNamespace, client.Flavor(), client.ServerVersion(), false)
	if err = writeFile(clusterRoleBindingPath, clusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write cluster role binding YAML file; %v", err)
	}

	pvcYAML := k8sclient.GetPVCYAML(pvcName, TridentPodNamespace, volumeSize, appLabelValue)
	if err = writeFile(pvcPath, pvcYAML); err != nil {
		return fmt.Errorf("could not write PVC YAML file; %v", err)
	}

	deploymentYAML := k8sclient.GetDeploymentYAML(pvcName, tridentImage, etcdImage, appLabelValue, Debug)
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

	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), client.ServerVersion(), true)
	if err = writeFile(clusterRolePath, clusterRoleYAML); err != nil {
		return fmt.Errorf("could not write cluster role YAML file; %v", err)
	}

	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(
		TridentPodNamespace, client.Flavor(), client.ServerVersion(), true)
	if err = writeFile(clusterRoleBindingPath, clusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write cluster role binding YAML file; %v", err)
	}

	pvcYAML := k8sclient.GetPVCYAML(pvcName, TridentPodNamespace, volumeSize, appLabelValue)
	if err = writeFile(pvcPath, pvcYAML); err != nil {
		return fmt.Errorf("could not write PVC YAML file; %v", err)
	}

	serviceYAML := k8sclient.GetCSIServiceYAML(appLabelValue)
	if err = writeFile(csiServicePath, serviceYAML); err != nil {
		return fmt.Errorf("could not write service YAML file; %v", err)
	}

	statefulSetYAML := k8sclient.GetCSIStatefulSetYAML(pvcName, tridentImage, etcdImage, appLabelValue, Debug)
	if err = writeFile(csiStatefulSetPath, statefulSetYAML); err != nil {
		return fmt.Errorf("could not write statefulset YAML file; %v", err)
	}

	daemonSetYAML := k8sclient.GetCSIDaemonSetYAML(tridentImage, TridentNodeLabelValue,
		Debug, client.ServerVersion())
	if err = writeFile(csiDaemonSetPath, daemonSetYAML); err != nil {
		return fmt.Errorf("could not write daemonset YAML file; %v", err)
	}

	return nil
}

func writeFile(filePath, data string) error {
	return ioutil.WriteFile(filePath, []byte(data), 0644)
}

func installTrident() (returnError error) {

	var (
		logFields           log.Fields
		pvcExists           bool
		pvExists            bool
		pvc                 *v1.PersistentVolumeClaim
		pv                  *v1.PersistentVolume
		pvRequestedQuantity resource.Quantity
		storageBackend      *storage.Backend
	)

	// Validate volume size
	pvRequestedQuantity, err := resource.ParseQuantity(volumeSize)
	if err != nil {
		return fmt.Errorf("volume-size '%s' is invalid; %v", volumeSize, err)
	}
	log.WithField("quantity", pvRequestedQuantity.String()).Debug("Parsed requested volume size.")

	if !csi {

		// Ensure Trident isn't already installed
		if installed, namespace, err := isTridentInstalled(); err != nil {
			return fmt.Errorf("could not check if Trident deployment exists; %v", err)
		} else if installed {
			return fmt.Errorf("trident is already installed in namespace %s", namespace)
		}

	} else {

		// Ensure CSI minimum requirements are met
		minCSIVersion := utils.MustParseSemantic(tridentconfig.KubernetesCSIVersionMin)
		if !client.ServerVersion().AtLeast(minCSIVersion) {
			return fmt.Errorf("CSI Trident requires Kubernetes %s or later", minCSIVersion.ShortString())
		}

		// Ensure CSI Trident isn't already installed
		if installed, namespace, err := isCSITridentInstalled(); err != nil {
			return fmt.Errorf("could not check if Trident statefulset exists; %v", err)
		} else if installed {
			return fmt.Errorf("CSI Trident is already installed in namespace %s", namespace)
		}

		log.Warning("CSI Trident for Kubernetes is a technology preview " +
			"and should not be installed in production environments!")
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
		if pvc.Labels == nil || pvc.Labels[appLabelKey] != appLabelValue {
			returnError = fmt.Errorf("PVC %s does not have %s label; "+
				"please add label or delete PVC and try again", pvcName, appLabel)
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
		if pv.Labels == nil || pv.Labels[appLabelKey] != appLabelValue {
			returnError = fmt.Errorf("PV %s does not have %s label; "+
				"please add label or delete PV and try again", pvName, appLabel)
			return
		}

		// Ensure PV size matches the request
		if pvActualQuantity, ok := pv.Spec.Capacity[v1.ResourceStorage]; !ok {
			log.WithField("pv", pvName).Warning("Could not determine size of existing PV.")
		} else if pvRequestedQuantity.Cmp(pvActualQuantity) != 0 {
			log.WithFields(log.Fields{
				"existing": pvActualQuantity.String(),
				"request":  pvRequestedQuantity.String(),
				"pv":       pvName,
			}).Warning("Existing PV size does not match request.")
		}

		log.WithFields(log.Fields{
			"pv":    pvName,
			"phase": pv.Status.Phase,
		}).Debug("PV already exists.")

	} else {
		log.WithField("pv", pvName).Debug("PV does not exist.")
	}

	// If the PV doesn't exist, we will need the storage driver to create it. Load the driver
	// here to detect any problems before starting the installation steps.
	if !pvExists {
		if storageBackend, returnError = loadStorageDriver(); returnError != nil {
			return
		}
	} else {
		log.Debug("PV exists, skipping storage driver check.")
	}

	// If dry-run was specified, stop before we change anything
	if dryRun {
		log.Info("Dry run completed, no problems found.")
		return
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

	// Create PVC if necessary
	if !pvcExists {
		if useYAML && fileExists(pvcPath) {
			returnError = validateTridentPVC()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the PVC YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(pvcPath)
			logFields = log.Fields{"path": pvcPath}
		} else {
			returnError = client.CreateObjectByYAML(k8sclient.GetPVCYAML(
				pvcName, TridentPodNamespace, volumeSize, appLabelValue))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create PVC %s; %v", pvcName, returnError)
			return
		}
		log.WithFields(logFields).Info("Created PVC.")
	}

	// Create PV if necessary
	if !pvExists {
		returnError = createPV(storageBackend)
		if returnError != nil {
			returnError = fmt.Errorf("could not create PV %s; %v", pvName, returnError)
			return
		}
		log.WithField("pv", pvName).Info("Created PV.")
	}

	// Wait for PV/PVC to be bound
	checkPVCBound := func() error {
		bound, err := client.CheckPVCBound(pvcName)
		if err != nil || !bound {
			return errors.New("PVC not bound")
		}
		return nil
	}
	if checkError := checkPVCBound(); checkError != nil {
		pvcNotify := func(err error, duration time.Duration) {
			log.WithFields(log.Fields{
				"pvc":       pvcName,
				"increment": duration,
			}).Debugf("PVC not yet bound, waiting.")
		}
		pvcBackoff := backoff.NewExponentialBackOff()
		pvcBackoff.MaxElapsedTime = k8sTimeout

		log.WithField("pvc", pvcName).Info("Waiting for PVC to be bound.")

		if err := backoff.RetryNotify(checkPVCBound, pvcBackoff, pvcNotify); err != nil {
			returnError = fmt.Errorf("PVC %s was not bound after %3.2f seconds", pvcName, k8sTimeout.Seconds())
			return
		}
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
			returnError = client.CreateObjectByYAML(
				k8sclient.GetDeploymentYAML(pvcName, tridentImage, etcdImage, appLabelValue, Debug))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident deployment; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident deployment.")

	} else {

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
			frontendrest.CACertName, frontendrest.ServerCertName, frontendrest.ClientCertName)
		if err != nil {
			returnError = fmt.Errorf("could not create Trident X509 certificates; %v", err)
			return
		}

		// Create the secret for the HTTP certs & keys
		secretMap := map[string]string{
			frontendrest.CAKeyFile:      certInfo.CAKey,
			frontendrest.CACertFile:     certInfo.CACert,
			frontendrest.ServerKeyFile:  certInfo.ServerKey,
			frontendrest.ServerCertFile: certInfo.ServerCert,
			frontendrest.ClientKeyFile:  certInfo.ClientKey,
			frontendrest.ClientCertFile: certInfo.ClientCert,
		}
		err = client.CreateObjectByYAML(
			k8sclient.GetSecretYAML("trident-csi", TridentPodNamespace, appLabelValue, secretMap))
		if err != nil {
			returnError = fmt.Errorf("could not create Trident secret; %v", err)
			return
		}
		log.WithFields(logFields).Info("Created Trident secret.")

		// Create the statefulset
		if useYAML && fileExists(csiStatefulSetPath) {
			returnError = validateTridentStatefulSet()
			if returnError != nil {
				returnError = fmt.Errorf("please correct the statefulset YAML file; %v", returnError)
				return
			}
			returnError = client.CreateObjectByFile(csiStatefulSetPath)
			logFields = log.Fields{"path": csiStatefulSetPath}
		} else {
			returnError = client.CreateObjectByYAML(
				k8sclient.GetCSIStatefulSetYAML(pvcName, tridentImage, etcdImage, appLabelValue, Debug))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create Trident statefulset; %v", returnError)
			return
		}
		log.WithFields(logFields).Info("Created Trident statefulset.")

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
				k8sclient.GetCSIDaemonSetYAML(tridentImage, TridentNodeLabelValue, Debug, client.ServerVersion()))
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

	// Add backend if we created the PV during this installation
	if storageBackend == nil {
		log.WithField("pvName", pvcName).Debug("PV existed prior to installation, " +
			"so installer will not add a backend automatically.")
	} else {
		backendFile := fmt.Sprintf("setup/%s", BackendConfigFilename)
		backendName, err := addTridentBackend()
		if err != nil {
			log.WithFields(log.Fields{
				"backendFile": backendFile,
				"error":       err,
			}).Warning("Could not add backend to Trident.")
		} else {
			log.WithFields(log.Fields{
				"backendFile": backendFile,
				"backendName": backendName,
			}).Info("Added backend to Trident.")
		}
	}

	log.Info("Trident installation succeeded.")
	return nil
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

func loadStorageDriver() (backend *storage.Backend, returnError error) {

	// Set up telemetry so any PV we create has the correct metadata
	tridentconfig.OrchestratorTelemetry = tridentconfig.Telemetry{
		TridentVersion:  tridentconfig.OrchestratorVersion.String(),
		Platform:        "kubernetes",
		PlatformVersion: client.ServerVersion().ShortString(),
	}

	if csi {
		tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	} else {
		tridentconfig.CurrentDriverContext = tridentconfig.ContextKubernetes
	}

	// Try to start the driver, which is the source of many installation problems and
	// will be needed to if we have to provision the Trident PV.
	log.WithField("backend", backendConfigFilePath).Info("Starting storage driver.")

	configFileBytes, returnError := loadStorageDriverConfig()
	if returnError != nil {
		return
	}

	backend, returnError = factory.NewStorageBackendForConfig(string(configFileBytes))
	if returnError != nil {
		returnError = fmt.Errorf("could not start the storage backend driver; %v", returnError)
		return
	}

	log.WithField("driver", backend.GetDriverName()).Info("Storage driver loaded.")
	return
}

func loadStorageDriverConfig() ([]byte, error) {

	// Ensure the setup directory & backend config file are present
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("setup directory does not exist; %v", err)
	}
	if _, err := os.Stat(backendConfigFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("storage backend config file does not exist; %v", err)
	}

	configFileBytes, err := ioutil.ReadFile(backendConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("could not read the storage backend config file; %v", err)
	}

	return configFileBytes, nil
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
		returnError = client.CreateObjectByYAML(k8sclient.GetClusterRoleYAML(client.Flavor(),
			client.ServerVersion(), csi))
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
		returnError = client.CreateObjectByYAML(k8sclient.GetClusterRoleBindingYAML(
			TridentPodNamespace, client.Flavor(), client.ServerVersion(), csi))
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
	clusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(
		TridentPodNamespace, client.Flavor(), client.ServerVersion(), csi)
	if err := client.DeleteObjectByYAML(clusterRoleBindingYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete cluster role binding.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted cluster role binding.")
	}

	// Delete cluster role
	clusterRoleYAML := k8sclient.GetClusterRoleYAML(client.Flavor(), client.ServerVersion(), csi)
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

	deployment, err := ReadDeploymentFromFile(deploymentPath)
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

func validateTridentService() error {

	service, err := ReadServiceFromFile(csiServicePath)
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

func validateTridentStatefulSet() error {

	statefulset, err := ReadStatefulSetFromFile(csiStatefulSetPath)
	if err != nil {
		return fmt.Errorf("could not load statefulset YAML file; %v", err)
	}

	// Check the statefulset label
	labels := statefulset.Labels
	if labels[appLabelKey] != appLabelValue {
		return fmt.Errorf("the Trident statefulset must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	// Check the pod label
	labels = statefulset.Spec.Template.Labels
	if labels[appLabelKey] != appLabelValue {
		return fmt.Errorf("the Trident statefulset's pod template must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	tridentImage := ""
	for _, container := range statefulset.Spec.Template.Spec.Containers {
		if container.Name == tridentconfig.ContainerTrident {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return fmt.Errorf("the Trident statefulset must define the %s container", tridentconfig.ContainerTrident)
	}

	return nil
}

func validateTridentDaemonSet() error {

	daemonset, err := ReadDaemonSetFromFile(csiDaemonSetPath)
	if err != nil {
		return fmt.Errorf("could not load daemonset YAML file; %v", err)
	}

	// Check the daemonset label
	labels := daemonset.Labels
	if labels[TridentNodeLabelKey] != TridentNodeLabelValue {
		return fmt.Errorf("the Trident daemonset must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	// Check the pod label
	labels = daemonset.Spec.Template.Labels
	if labels[TridentNodeLabelKey] != TridentNodeLabelValue {
		return fmt.Errorf("the Trident daemonset's pod template must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
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

func validateTridentPVC() error {

	pvc, err := ReadPVCFromFile(pvcPath)
	if err != nil {
		return fmt.Errorf("could not load PVC YAML file; %v", err)
	}

	// Check the label
	labels := pvc.Labels
	if labels[appLabelKey] != appLabelValue {
		return fmt.Errorf("the Trident PVC must have the label \"%s: %s\"",
			appLabelKey, appLabelValue)
	}

	// Check the name
	if pvc.Name != pvcName {
		return fmt.Errorf("the Trident PVC must be named %s", pvcName)
	}

	// Check the namespace
	if pvc.Namespace != TridentPodNamespace {
		return fmt.Errorf("the Trident PVC must specify namespace %s", TridentPodNamespace)
	}

	return nil
}

func createPV(sb *storage.Backend) error {

	// Choose a pool
	if len(sb.Storage) == 0 {
		return fmt.Errorf("backend %s has no storage pools", sb.Name)
	}
	var pool *storage.Pool
	for _, pool = range sb.Storage {
		// Let Golang's map iteration randomization choose a pool for us
		break
	}

	// Create the volume config
	volConfig := &storage.VolumeConfig{
		Version:  "1",
		Name:     volumeName,
		Size:     volumeSize,
		Protocol: sb.GetProtocol(),
	}

	volAttributes := make(map[string]sa.Request)

	// Create the volume on the backend
	volume, err := sb.AddVolume(volConfig, pool, volAttributes)
	if err != nil {
		return fmt.Errorf("could not create a volume on the storage backend; %v", err)
	}

	// Get the PV YAML (varies by volume protocol type)
	var pvYAML string
	switch {
	case volume.Config.AccessInfo.NfsAccessInfo.NfsServerIP != "":

		pvYAML = k8sclient.GetNFSPVYAML(
			pvName, volumeSize, volume.Config.AccessInfo.MountOptions, pvcName, TridentPodNamespace,
			volume.Config.AccessInfo.NfsAccessInfo.NfsServerIP,
			volume.Config.AccessInfo.NfsAccessInfo.NfsPath,
			appLabelValue)

	case volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetPortal != "":

		if volume.Config.AccessInfo.IscsiTargetSecret != "" {

			// Validate CHAP support in Kubernetes
			if !client.ServerVersion().AtLeast(utils.MustParseSemantic("v1.7.0")) {
				return errors.New("iSCSI CHAP requires Kubernetes 1.7.0 or later")
			}

			// Using CHAP
			secretName, err := createCHAPSecret(volume)
			if err != nil {
				return err
			}

			pvYAML = k8sclient.GetCHAPISCSIPVYAML(
				pvName, volumeSize, volume.Config.AccessInfo.MountOptions, pvcName, TridentPodNamespace, secretName,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetPortal,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiPortals,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetIQN,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber,
				appLabelValue)

		} else {

			// Not using CHAP
			pvYAML = k8sclient.GetISCSIPVYAML(
				pvName, volumeSize, volume.Config.AccessInfo.MountOptions, pvcName, TridentPodNamespace,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetPortal,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiPortals,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetIQN,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber,
				appLabelValue)
		}

	default:
		return errors.New("unrecognized volume type")
	}

	// Create the PV
	err = client.CreateObjectByYAML(pvYAML)
	if err != nil {
		return fmt.Errorf("could not create PV %s; %v", pvName, err)
	}

	return nil
}

func createCHAPSecret(volume *storage.Volume) (secretName string, returnError error) {

	secretName = volume.ConstructExternal().GetCHAPSecretName()
	log.WithField("secret", secretName).Debug("Using iSCSI CHAP secret.")

	secretExists, err := client.CheckSecretExists(secretName)
	if err != nil {
		returnError = fmt.Errorf("could not check for existing iSCSI CHAP secret; %v", err)
		return
	}
	if !secretExists {
		log.WithField("secret", secretName).Debug("iSCSI CHAP secret does not exist.")

		// Create the YAML for the new secret
		secretYAML := k8sclient.GetCHAPSecretYAML(secretName,
			volume.Config.AccessInfo.IscsiUsername,
			volume.Config.AccessInfo.IscsiInitiatorSecret,
			volume.Config.AccessInfo.IscsiTargetSecret)

		// Create the secret
		err = client.CreateObjectByYAML(secretYAML)
		if err != nil {
			returnError = fmt.Errorf("could not create CHAP secret; %v", err)
			return
		}
		log.WithField("secret", secretName).Info("Created iSCSI CHAP secret.")
	} else {
		log.WithField("secret", secretName).Debug("iSCSI CHAP secret already exists.")
	}

	return
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

func addTridentBackend() (string, error) {

	configFileBytes, err := loadStorageDriverConfig()
	if err != nil {
		return "", err
	}

	cliCommand := []string{
		"tridentctl",
		"-s", PodServer,
		"create", "backend",
		"-o", "json",
		"--base64", base64.StdEncoding.EncodeToString(configFileBytes),
	}
	backendJSON, err := client.Exec(TridentPodName, tridentconfig.ContainerTrident, cliCommand)
	if err != nil {
		if backendJSON != nil && len(backendJSON) > 0 {
			err = fmt.Errorf("%v; %s", err, strings.TrimSpace(string(backendJSON)))
		}
		return "", err
	}

	var backendsResponse api.MultipleBackendResponse
	err = json.Unmarshal(backendJSON, &backendsResponse)
	if err != nil {
		return "", err
	}

	if len(backendsResponse.Items) != 1 {
		return "", errors.New("could not discern added backend")
	}

	return backendsResponse.Items[0].Name, nil
}

// ReadDeploymentFromFile parses and returns a deployment object from a file.
func ReadDeploymentFromFile(filePath string) (*v1beta1.Deployment, error) {

	var deployment v1beta1.Deployment

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

// ReadServiceFromFile parses and returns a service object from a file.
func ReadServiceFromFile(filePath string) (*v1.Service, error) {

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

// ReadStatefulSetFromFile parses and returns a statefulset object from a file.
func ReadStatefulSetFromFile(filePath string) (*appsv1.StatefulSet, error) {

	var statefulset appsv1.StatefulSet

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &statefulset)
	if err != nil {
		return nil, err
	}
	return &statefulset, nil
}

// ReadDaemonSetFromFile parses and returns a daemonset object from a file.
func ReadDaemonSetFromFile(filePath string) (*v1beta1.DaemonSet, error) {

	var daemonset v1beta1.DaemonSet

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

// ReadPVCFromFile parses and returns a PVC object from a file.
func ReadPVCFromFile(filePath string) (*v1.PersistentVolumeClaim, error) {

	var pvc v1.PersistentVolumeClaim

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &pvc)
	if err != nil {
		return nil, err
	}
	return &pvc, nil
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

		if dryRun {
			returnError = fmt.Errorf("namespace %s must exist to perform an in-cluster dry run; "+
				"please create it manually", TridentPodNamespace)
			return
		}

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
	if returnError = client.CreateConfigMapFromDirectory(
		setupPath, "trident-installer", TridentInstallerLabel); returnError != nil {
		return
	}
	log.WithFields(log.Fields{"configmap": "trident-installer"}).Info("Created installer configmap.")

	// Create the installer arguments
	commandArgs := []string{
		"tridentctl", "install",
		"--volume-size", volumeSize,
		"--k8s-timeout", k8sTimeout.String(),
		"--namespace", TridentPodNamespace,
	}
	if Debug {
		commandArgs = append(commandArgs, "--debug")
	}
	if dryRun {
		commandArgs = append(commandArgs, "--dry-run")
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
	if volumeName != "" {
		commandArgs = append(commandArgs, "--volume-name")
		commandArgs = append(commandArgs, volumeName)
	}
	if tridentImage != "" {
		commandArgs = append(commandArgs, "--trident-image")
		commandArgs = append(commandArgs, tridentImage)
	}
	if etcdImage != "" {
		commandArgs = append(commandArgs, "--etcd-image")
		commandArgs = append(commandArgs, etcdImage)
	}
	commandArgs = append(commandArgs, "--in-cluster=false")

	// Create the install pod
	errMessage := "could not create installer pod"
	returnError = backoffCreateObjectsByYAML("installerPod",
		k8sclient.GetInstallerPodYAML(TridentInstallerLabelValue, tridentImage, commandArgs), errMessage)
	if returnError != nil {
		return
	}
	log.WithFields(log.Fields{"pod": "trident-installer"}).Info("Created installer pod.")

	// Wait for Trident installation pod to start
	var installPod *v1.Pod
	installPod, returnError = waitForTridentInstallationPodToStart()
	if returnError != nil {
		return
	}

	// Wait for pod to finish & output logs
	followInstallationLogs(installPod.Name, "", installPod.Namespace)

	installPod, returnError = waitForTridentInstallationPodToFinish()
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

func backoffCreateObjectsByYAML(objectName string, installerYAML string, errorMessage string) error {
	// Wait for PV/PVC to be bound
	checkCreateObjectByYAML := func() error {
		returnError := client.CreateObjectByYAML(installerYAML)
		if returnError != nil {
			log.WithFields(log.Fields{
				"objectName": objectName,
				"err":        returnError,
			}).Errorf("Object creation failed.")
			return errors.New(errorMessage)
		}
		return nil
	}

	createObjectNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"objectName": objectName,
			"increment":  duration,
			"err":        err,
		}).Debugf("Object not created, waiting.")
	}
	createObjectBackoff := backoff.NewExponentialBackOff()
	createObjectBackoff.MaxElapsedTime = k8sTimeout

	log.WithField("objectName", objectName).Info("Waiting for object to be created.")

	if err := backoff.RetryNotify(checkCreateObjectByYAML, createObjectBackoff, createObjectNotify); err != nil {
		returnError := fmt.Errorf("object %s was not created after %3.2f seconds", objectName, k8sTimeout.Seconds())
		return returnError
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
	returnError = backoffCreateObjectsByYAML("clusterRole",
		k8sclient.GetInstallerClusterRoleYAML(client.Flavor(), client.ServerVersion()), errMessage)
	if returnError != nil {
		return returnError
	}
	log.WithFields(log.Fields{"clusterrole": "trident-installer"}).Info("Created installer cluster role.")

	// Create cluster role binding
	errMessage = "could not create installer cluster role binding"
	returnError = backoffCreateObjectsByYAML("clusterRoleBinding",
		k8sclient.GetInstallerClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(), client.ServerVersion()), errMessage)
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
	clusterRoleBindingYAML := k8sclient.GetInstallerClusterRoleBindingYAML(
		TridentPodNamespace, client.Flavor(), client.ServerVersion())
	if err := client.DeleteObjectByYAML(clusterRoleBindingYAML, true); err != nil {
		log.WithField("error", err).Warning("Could not delete installer cluster role binding.")
		anyErrors = true
	} else {
		logFunc(log.Fields{})("Deleted installer cluster role binding.")
	}

	// Delete cluster role
	clusterRoleYAML := k8sclient.GetInstallerClusterRoleYAML(client.Flavor(), client.ServerVersion())
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

func waitForTridentInstallationPodToStart() (*v1.Pod, error) {

	var pod *v1.Pod

	checkPodRunning := func() error {
		var podError error
		pod, podError = client.GetPodByLabel(TridentInstallerLabel, false)
		if podError != nil {
			return fmt.Errorf("pod not yet started; %v", podError)
		}
		switch pod.Status.Phase {
		case v1.PodPending, v1.PodUnknown:
			return fmt.Errorf("pod not yet started (%s)", pod.Status.Phase)
		default:
			// Stop waiting if if pod is any of Running, Succeeded, Failed, Unschedulable
			log.WithField("phase", pod.Status.Phase).Debug("Pod started.")
			return nil
		}
	}
	podNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Trident installer pod not yet started, waiting.")
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	log.Info("Waiting for Trident installer pod to start.")

	if err := backoff.RetryNotify(checkPodRunning, podBackoff, podNotify); err != nil {

		// Build up an error message with as much detail as available.
		var errMessages []string
		errMessages = append(errMessages,
			fmt.Sprintf("Trident installer pod was not started after %3.2f seconds.", k8sTimeout.Seconds()))

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
	}).Info("Trident installer pod started.")

	return pod, nil
}

func waitForTridentInstallationPodToFinish() (*v1.Pod, error) {

	var pod *v1.Pod

	checkPodFinished := func() error {
		var podError error
		pod, podError = client.GetPodByLabel(TridentInstallerLabel, false)
		if podError != nil {
			return fmt.Errorf("pod not yet finished; %v", podError)
		}
		switch pod.Status.Phase {
		case v1.PodPending, v1.PodUnknown, v1.PodRunning:
			return fmt.Errorf("pod not yet finished (%s)", pod.Status.Phase)
		default:
			// Stop waiting if if pod is any of Succeeded, Failed, Unschedulable
			log.WithField("phase", pod.Status.Phase).Debug("Pod finished.")
			return nil
		}
	}
	podNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Trident installer pod not yet finished, waiting.")
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	log.Info("Waiting for Trident installer pod to finish.")

	if err := backoff.RetryNotify(checkPodFinished, podBackoff, podNotify); err != nil {

		// Build up an error message with as much detail as available.
		var errMessages []string
		errMessages = append(errMessages,
			fmt.Sprintf("Trident installer pod was not finished after %3.2f seconds.", k8sTimeout.Seconds()))

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
	}).Info("Trident installer pod finished.")

	return pod, nil
}

func followInstallationLogs(pod, container, namespace string) {

	var (
		err error
		cmd *exec.Cmd
	)

RetryLoop:
	for {
		time.Sleep(1 * time.Second)

		args := []string{
			fmt.Sprintf("--namespace=%s", namespace),
			"logs",
			pod,
			"-f",
		}
		if container != "" {
			args = append(args, []string{"-c", container}...)
		}

		log.WithField("cmd", client.CLI()+" "+strings.Join(args, " ")).Debug("Getting logs.")

		cmd = exec.Command(client.CLI(), args...)

		// Create a pipe that holds stdout
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		// Start the child process
		err = cmd.Start()
		if err != nil {
			log.WithFields(log.Fields{
				"pod":       pod,
				"container": container,
				"namespace": namespace,
				"error":     err,
			}).Error("Could not get pod logs.")
			return
		}

		// Create a new scanner
		buff := bufio.NewScanner(io.MultiReader(stdout, stderr))

		// Iterate over buff and append content to the slice
		for buff.Scan() {
			line := buff.Text()

			// If we get an error from Kubernetes, just try again
			if strings.Contains(line, "Error from server") {

				log.WithFields(log.Fields{
					"pod":       pod,
					"container": container,
					"error":     line,
				}).Debug("Got server error, retrying.")

				cmd.Wait()
				continue RetryLoop
			}

			logLogFmtMessage(line)
		}

		log.WithFields(log.Fields{
			"pod":       pod,
			"container": container,
		}).Debug("Received EOF from pod logs.")
		break
	}

	// Clean up
	cmd.Wait()
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
