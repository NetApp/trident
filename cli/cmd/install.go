package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
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

	TridentLabel      = "app=trident.netapp.io"
	TridentLabelKey   = "app"
	TridentLabelValue = "trident.netapp.io"

	BackendConfigFilename      = "backend.json"
	NamespaceFilename          = "trident-namespace.yaml"
	ServiceAccountFilename     = "trident-serviceaccount.yaml"
	ClusterRoleFilename        = "trident-clusterrole.yaml"
	ClusterRoleBindingFilename = "trident-clusterrolebinding.yaml"
	PVCFilename                = "trident-pvc.yaml"
	DeploymentFilename         = "trident-deployment.yaml"
)

var (
	// CLI flags
	dryRun       bool
	generateYAML bool
	useYAML      bool
	silent       bool
	pvName       string
	pvcName      string
	volumeName   string
	volumeSize   string
	k8sTimeout   time.Duration
	image        string

	// CLI-based K8S client
	client k8s_client.Interface

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

	dns1123LabelRegex  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	dns1123DomainRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

func init() {
	RootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run all the pre-checks, but don't install anything.")
	installCmd.Flags().BoolVar(&generateYAML, "generate-custom-yaml", false, "Generate YAML files, but don't install anything.")
	installCmd.Flags().BoolVar(&useYAML, "use-custom-yaml", false, "Use any existing YAML files that exist in setup directory.")
	installCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during installation.")

	installCmd.Flags().StringVar(&pvcName, "pvc", DefaultPVCName, "The name of the PVC used by Trident.")
	installCmd.Flags().StringVar(&pvName, "pv", DefaultPVName, "The name of the PV used by Trident.")
	installCmd.Flags().StringVar(&volumeName, "volume-name", DefaultVolumeName, "The name of the storage volume used by Trident.")
	installCmd.Flags().StringVar(&volumeSize, "volume-size", DefaultVolumeSize, "The size of the storage volume used by Trident.")
	installCmd.Flags().DurationVar(&k8sTimeout, "k8s-timeout", 120*time.Second, "The number of seconds to wait before timing out on Kubernetes operations.")

	installCmd.Flags().StringVar(&image, "image", "", "The Trident image to install.")
	installCmd.Flags().MarkHidden("image")
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Trident",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {

		initInstallerLogging()

		if err := discoverInstallationEnvironment(); err != nil {
			log.Fatalf("Install pre-checks failed; %v", err)
		}
		if err := validateInstallationArguments(); err != nil {
			log.Fatalf("Invalid arguments; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {

		if generateYAML {

			// If generate-custom-yaml was specified, write the YAML files to the setup directory
			if err := prepareYAMLFiles(); err != nil {
				log.Fatalf("YAML generation failed; %v", err)
			}
			log.WithField("setupPath", setupPath).Info("Wrote installation YAML files.")

		} else {

			// Run the installer
			if err := installTrident(); err != nil {
				log.Fatalf("Install failed; %v", err)
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
	if image == "" {
		image = tridentconfig.BuildImage
	}

	// Ensure we're on Linux
	if runtime.GOOS != "linux" {
		return errors.New("The Trident installer only runs on Linux.")
	}

	// Create the CLI-based Kubernetes client
	client, err = k8s_client.NewKubectlClient()
	if err != nil {
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
	}).Debug("Validated Trident installation environment.")

	return nil
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

	return nil
}

func prepareYAMLFiles() error {

	var err error

	namespaceYAML := k8s_client.GetNamespaceYAML(TridentPodNamespace)
	if err = writeFile(namespacePath, namespaceYAML); err != nil {
		return fmt.Errorf("could not write namespace YAML file; %v", err)
	}

	serviceAccountYAML := k8s_client.GetServiceAccountYAML()
	if err = writeFile(serviceAccountPath, serviceAccountYAML); err != nil {
		return fmt.Errorf("could not write service account YAML file; %v", err)
	}

	clusterRoleYAML := k8s_client.GetClusterRoleYAML(client.Flavor(), client.Version())
	if err = writeFile(clusterRolePath, clusterRoleYAML); err != nil {
		return fmt.Errorf("could not write cluster role YAML file; %v", err)
	}

	clusterRoleBindingYAML := k8s_client.GetClusterRoleBindingYAML(TridentPodNamespace, client.Flavor(), client.Version())
	if err = writeFile(clusterRoleBindingPath, clusterRoleBindingYAML); err != nil {
		return fmt.Errorf("could not write cluster role binding YAML file; %v", err)
	}

	pvcYAML := k8s_client.GetPVCYAML(pvcName, TridentPodNamespace, volumeSize)
	if err = writeFile(pvcPath, pvcYAML); err != nil {
		return fmt.Errorf("could not write PVC YAML file; %v", err)
	}

	deploymentYAML := k8s_client.GetDeploymentYAML(pvcName, image, Debug)
	if err = writeFile(deploymentPath, deploymentYAML); err != nil {
		return fmt.Errorf("could not write deployment YAML file; %v", err)
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
		pvcCreated          bool
		pvCreated           bool
		pvc                 *v1.PersistentVolumeClaim
		pv                  *v1.PersistentVolume
		pvRequestedQuantity resource.Quantity
		storageBackend      *storage.Backend
	)

	pvRequestedQuantity, err := resource.ParseQuantity(volumeSize)
	if err != nil {
		return fmt.Errorf("volume-size '%s' is invalid; %v", volumeSize, err)
	}
	log.WithField("quantity", pvRequestedQuantity.String()).Debug("Parsed requested volume size.")

	// Ensure Trident isn't already installed
	if exists, namespace, err := client.CheckDeploymentExistsByLabel(TridentLabel, true); err != nil {
		return fmt.Errorf("could not check if Trident deployment exists; %v", err)
	} else if exists {
		return fmt.Errorf("Trident is already installed in namespace %s", namespace)
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
			returnError = fmt.Errorf("PVC %s is Bound, but not to PV %s; "+
				"please specify a different PV and/or PVC", pvcName, pvName)
			return
		}
		if pvc.Labels == nil || pvc.Labels[TridentLabelKey] != TridentLabelValue {
			returnError = fmt.Errorf("PVC %s does not have %s label; "+
				"please add label or delete PVC and try again", pvcName, TridentLabel)
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
				returnError = fmt.Errorf("PV %s is Bound, but not to PVC %s; "+
					"please delete PV and try again", pvName, pvcName)
				return
			}
			if pv.Spec.ClaimRef.Namespace != TridentPodNamespace {
				returnError = fmt.Errorf("PV %s is Bound to a PVC in namespace %s; "+
					"please delete PV and try again", pvName, pv.Spec.ClaimRef.Namespace)
				return
			}
		}
		if pv.Labels == nil || pv.Labels[TridentLabelKey] != TridentLabelValue {
			returnError = fmt.Errorf("PV %s does not have %s label; "+
				"please add label or delete PV and try again", pvName, TridentLabel)
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
		if useYAML && fileExists(namespacePath) {
			returnError = client.CreateObjectByFile(namespacePath)
			logFields = log.Fields{"path": namespacePath}
		} else {
			returnError = client.CreateObjectByYAML(k8s_client.GetNamespaceYAML(TridentPodNamespace))
			logFields = log.Fields{"namespace": TridentPodNamespace}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create namespace %s; %v", TridentPodNamespace, returnError)
			return
		}
		log.WithFields(logFields).Info("Created namespace.")
	}

	// Remove any RBAC objects from a previous Trident installation
	if anyCleanupErrors := removeRBACObjects(log.DebugLevel); anyCleanupErrors {
		returnError = fmt.Errorf("could not remove one or more previous Trident artifacts; " +
			"please delete them manually and try again")
		return
	}

	// Set up a cleanup routine in case anything fails after this point
	defer func() { cleanupAfterInstallError(returnError, pvcCreated, pvCreated) }()

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
			returnError = client.CreateObjectByYAML(k8s_client.GetPVCYAML(pvcName, TridentPodNamespace, volumeSize))
			logFields = log.Fields{}
		}
		if returnError != nil {
			returnError = fmt.Errorf("could not create PVC %s; %v", pvcName, returnError)
			return
		}
		log.WithFields(logFields).Info("Created PVC.")
		pvcCreated = true
	}

	// Create PV if necessary
	if !pvExists {
		returnError = createPV(storageBackend)
		if returnError != nil {
			returnError = fmt.Errorf("could not create PV %s; %v", pvName, returnError)
			return
		}
		pvCreated = true
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
			returnError = fmt.Errorf("PVC %s was not bound after %d seconds", pvcName, k8sTimeout)
			return
		}
	}

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
		returnError = client.CreateObjectByYAML(k8s_client.GetDeploymentYAML(pvcName, image, Debug))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create Trident deployment; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created Trident deployment.")

	// Wait for Trident pod to be running
	tridentPod, returnError := waitForTridentPod()
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

func loadStorageDriver() (backend *storage.Backend, returnError error) {

	// Set up telemetry so any PV we create has the correct metadata
	tridentconfig.OrchestratorTelemetry = tridentconfig.Telemetry{
		TridentVersion:  tridentconfig.OrchestratorVersion.String(),
		Platform:        "kubernetes",
		PlatformVersion: client.Version().ShortString(),
	}
	tridentconfig.CurrentDriverContext = tridentconfig.ContextKubernetes

	// Ensure the setup directory & backend config file are present
	if _, returnError = os.Stat(setupPath); os.IsNotExist(returnError) {
		returnError = fmt.Errorf("setup directory does not exist; %v", returnError)
		return
	}
	if _, returnError = os.Stat(backendConfigFilePath); os.IsNotExist(returnError) {
		returnError = fmt.Errorf("storage backend config file does not exist; %v", returnError)
		return
	}

	// Try to start the driver, which is the source of many installation problems and
	// will be needed to if we have to provision the Trident PV.
	log.WithField("backend", backendConfigFilePath).Info("Starting storage driver.")
	configFileBytes, returnError := ioutil.ReadFile(backendConfigFilePath)
	if returnError != nil {
		returnError = fmt.Errorf("could not read the storage backend config file; %v", returnError)
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

func createRBACObjects() (returnError error) {

	var logFields log.Fields

	// Create service account
	if useYAML && fileExists(serviceAccountPath) {
		returnError = client.CreateObjectByFile(serviceAccountPath)
		logFields = log.Fields{"path": serviceAccountPath}
	} else {
		returnError = client.CreateObjectByYAML(k8s_client.GetServiceAccountYAML())
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
		returnError = client.CreateObjectByYAML(k8s_client.GetClusterRoleYAML(client.Flavor(), client.Version()))
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
		returnError = client.CreateObjectByYAML(k8s_client.GetClusterRoleBindingYAML(
			TridentPodNamespace, client.Flavor(), client.Version()))
		logFields = log.Fields{}
	}
	if returnError != nil {
		returnError = fmt.Errorf("could not create cluster role binding; %v", returnError)
		return
	}
	log.WithFields(logFields).Info("Created cluster role binding.")

	// If OpenShift, add Trident to security context constraint
	if client.Flavor() == k8s_client.FlavorOpenShift {
		if returnError = client.AddTridentUserToOpenShiftSCC(); returnError != nil {
			returnError = fmt.Errorf("could not modify security context constraint; %v", returnError)
			return
		}
		log.Info("Added Trident user to security context constraint.")
	}

	return
}

func removeRBACObjects(logLevel log.Level) (anyErrors bool) {

	var deleted bool
	var err error

	logFunc := log.Info
	if logLevel == log.DebugLevel {
		logFunc = log.Debug
	}

	// Delete cluster role binding
	deleted = false
	if fileExists(clusterRoleBindingPath) {
		if err = client.DeleteObjectByFile(clusterRoleBindingPath, true); err != nil {
			log.WithFields(log.Fields{
				"path":  clusterRoleBindingPath,
				"error": err,
			}).Warning("Could not delete cluster role binding using existing YAML file.")
		} else {
			deleted = true
		}
	}
	if !deleted {
		clusterRoleBindingYAML := k8s_client.GetClusterRoleBindingYAML(
			TridentPodNamespace, client.Flavor(), client.Version())
		if err = client.DeleteObjectByYAML(clusterRoleBindingYAML, true); err != nil {
			log.WithField("error", err).Warning("Could not delete cluster role binding.")
		} else {
			deleted = true
		}
	}
	if deleted {
		logFunc("Deleted cluster role binding.")
	} else {
		anyErrors = true
	}

	// Delete cluster role
	deleted = false
	if fileExists(clusterRolePath) {
		if err = client.DeleteObjectByFile(clusterRolePath, true); err != nil {
			log.WithFields(log.Fields{
				"path":  clusterRolePath,
				"error": err,
			}).Warning("Could not delete cluster role using existing YAML file.")
		} else {
			deleted = true
		}
	}
	if !deleted {
		clusterRoleYAML := k8s_client.GetClusterRoleYAML(client.Flavor(), client.Version())
		if err = client.DeleteObjectByYAML(clusterRoleYAML, true); err != nil {
			log.WithField("error", err).Warning("Could not delete cluster role.")
		} else {
			deleted = true
		}
	}
	if deleted {
		logFunc("Deleted cluster role.")
	} else {
		anyErrors = true
	}

	// Delete service account
	deleted = false
	if fileExists(serviceAccountPath) {
		if err = client.DeleteObjectByFile(serviceAccountPath, true); err != nil {
			log.WithFields(log.Fields{
				"path":  serviceAccountPath,
				"error": err,
			}).Warning("Could not delete service account using existing YAML file.")
		} else {
			deleted = true
		}
	}
	if !deleted {
		serviceAccountYAML := k8s_client.GetServiceAccountYAML()
		if err = client.DeleteObjectByYAML(serviceAccountYAML, true); err != nil {
			log.WithField("error", err).Warning("Could not delete service account.")
		} else {
			deleted = true
		}
	}
	if deleted {
		logFunc("Deleted service account.")
	} else {
		anyErrors = true
	}

	// If OpenShift, remove Trident from security context constraint
	if client.Flavor() == k8s_client.FlavorOpenShift {
		if err = client.RemoveTridentUserFromOpenShiftSCC(); err != nil {
			log.WithField("error", err).Warning("Could not modify security context constraint.")
			anyErrors = true
		} else {
			logFunc("Removed Trident user from security context constraint.")
		}
	}

	return
}

func cleanupAfterInstallError(returnError error, pvcCreated, pvCreated bool) {

	if returnError == nil {
		// Nothing to do
		return
	}

	log.Warning("An error occurred during installation, cleaning up.")

	if anyCleanupErrors := removeRBACObjects(log.InfoLevel); anyCleanupErrors {
		log.Warning("RBAC cleanup failed.")
	}

	if pvcCreated {
		// Delete the PVC
		err := client.DeleteObjectByName("pvc", pvcName, false)
		if err != nil {
			log.WithFields(log.Fields{
				"pvc":   pvcName,
				"error": err,
			}).Warning("Could not delete PVC during error recovery.")
		} else {
			log.WithField("pvc", pvcName).Info("Deleted PVC.")
		}
	}

	if pvCreated {
		// Delete the PV
		err := client.DeleteObjectByName("pv", pvName, false)
		if err != nil {
			log.WithFields(log.Fields{
				"pv":    pvName,
				"error": err,
			}).Warning("Could not delete PV during error recovery.")
		} else {
			log.WithField("pv", pvcName).Info("Deleted PV.")
		}
	}
}

func validateTridentDeployment() error {

	deployment, err := client.ReadDeploymentFromFile(deploymentPath)
	if err != nil {
		return fmt.Errorf("could not load deployment YAML file; %v", err)
	}

	// Check the deployment label
	labels := deployment.Labels
	if labels[TridentLabelKey] != TridentLabelValue {
		return fmt.Errorf("the Trident deployment must have the label \"%s: %s\"",
			TridentLabelKey, TridentLabelValue)
	}

	// Check the pod label
	labels = deployment.Spec.Template.Labels
	if labels[TridentLabelKey] != TridentLabelValue {
		return fmt.Errorf("the Trident deployment's pod template must have the label \"%s: %s\"",
			TridentLabelKey, TridentLabelValue)
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

func validateTridentPVC() error {

	pvc, err := client.ReadPVCFromFile(pvcPath)
	if err != nil {
		return fmt.Errorf("could not load PVC YAML file; %v", err)
	}

	// Check the deployment label
	labels := pvc.Labels
	if labels[TridentLabelKey] != TridentLabelValue {
		return fmt.Errorf("the Trident PVC must have the label \"%s: %s\"",
			TridentLabelKey, TridentLabelValue)
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

		pvYAML = k8s_client.GetNFSPVYAML(pvName, volumeSize,
			volume.Config.AccessInfo.NfsAccessInfo.NfsServerIP,
			volume.Config.AccessInfo.NfsAccessInfo.NfsPath)

	case volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetPortal != "":

		if volume.Config.AccessInfo.IscsiTargetSecret != "" {

			// Validate CHAP support in Kubernetes
			if !client.Version().AtLeast(utils.MustParseSemantic("v1.7.0")) {
				return errors.New("CHAP requires Kubernetes 1.7.0 or later.")
			}

			// Using CHAP
			secretName, err := createCHAPSecret(volume)
			if err != nil {
				return err
			}

			pvYAML = k8s_client.GetCHAPISCSIPVYAML(pvName, volumeSize,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetPortal,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetIQN,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber,
				secretName)

		} else {

			// Not using CHAP
			pvYAML = k8s_client.GetISCSIPVYAML(pvName, volumeSize,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetPortal,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiTargetIQN,
				volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
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
		secretYAML := k8s_client.GetCHAPSecretYAML(secretName,
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
		pod, podError = client.GetPodByLabel(TridentLabel, false)
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
				fmt.Sprintf("Use '%s describe pod %s' for more information.", client.CLI(), pod.Name))
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
