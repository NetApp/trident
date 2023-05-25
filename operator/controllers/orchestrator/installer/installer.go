// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ghodss/yaml"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/operator/controllers/orchestrator/apis/netapp/v1"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/crypto"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	// CRD names

	ActionMirrorUpdateCRDName = "tridentactionmirrorupdates.trident.netapp.io"
	BackendCRDName            = "tridentbackends.trident.netapp.io"
	BackendConfigCRDName      = "tridentbackendconfigs.trident.netapp.io"
	MirrorRelationshipCRDName = "tridentmirrorrelationships.trident.netapp.io"
	SnapshotInfoCRDName       = "tridentsnapshotinfos.trident.netapp.io"
	NodeCRDName               = "tridentnodes.trident.netapp.io"
	StorageClassCRDName       = "tridentstorageclasses.trident.netapp.io"
	TransactionCRDName        = "tridenttransactions.trident.netapp.io"
	VersionCRDName            = "tridentversions.trident.netapp.io"
	VolumeCRDName             = "tridentvolumes.trident.netapp.io"
	VolumePublicationCRDName  = "tridentvolumepublications.trident.netapp.io"
	SnapshotCRDName           = "tridentsnapshots.trident.netapp.io"
	VolumeReferenceCRDName    = "tridentvolumereferences.trident.netapp.io"

	DefaultTimeout = 30
)

var (
	// CR inputs

	enableForceDetach  bool
	disableAuditLog    bool
	debug              bool
	useIPv6            bool
	silenceAutosupport bool
	windows            bool

	logLevel        string
	logWorkflows    string
	logLayers       string
	logFormat       string
	probePort       string
	tridentImage    string
	imageRegistry   string
	kubeletDir      string
	imagePullPolicy string

	autosupportImage        string
	autosupportProxy        string
	autosupportSerialNumber string
	autosupportHostname     string

	imagePullSecrets []string

	k8sTimeout  time.Duration
	httpTimeout string

	appLabel      string
	appLabelKey   string
	appLabelValue string

	controllerPluginNodeSelector map[string]string
	controllerPluginTolerations  []netappv1.Toleration
	nodePluginNodeSelector       map[string]string
	nodePluginTolerations        []netappv1.Toleration

	CRDnames = []string{
		ActionMirrorUpdateCRDName,
		BackendCRDName,
		BackendConfigCRDName,
		MirrorRelationshipCRDName,
		SnapshotInfoCRDName,
		NodeCRDName,
		StorageClassCRDName,
		TransactionCRDName,
		VersionCRDName,
		VolumeCRDName,
		SnapshotCRDName,
		VolumeReferenceCRDName,
		VolumePublicationCRDName,
	}
)

type Installer struct {
	client           ExtendedK8sClient
	tridentCRDClient *crdclient.Clientset
	namespace        string
}

func NewInstaller(kubeConfig *rest.Config, namespace string, timeout int) (TridentInstaller, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	k8sTimeout = time.Duration(timeout) * time.Second

	// Create the Kubernetes client
	kubeClient, err := NewExtendedK8sClient(kubeConfig, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client; %v", err)
	}

	// Create Trident CRD client
	CRDClientForTrident, err := crdclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Trident's CRD client; %v", err)
	}

	Log().WithField("namespace", namespace).Debugf("Initialized installer.")

	return &Installer{
		client:           kubeClient,
		tridentCRDClient: CRDClientForTrident,
		namespace:        namespace,
	}, nil
}

func (i *Installer) logFormatPrechecks() (returnError error) {
	switch logFormat {
	case "text", "json":
		break
	default:
		returnError = fmt.Errorf("'%s' is not a valid log format", logFormat)
		return
	}

	return nil
}

func (i *Installer) imagePullPolicyPrechecks() error {
	switch v1.PullPolicy(imagePullPolicy) {
	// If the value of imagePullPolicy is either of PullIfNotPresent, PullAlways or PullNever then the imagePullPolicy
	// is valid and no action is required.
	case v1.PullIfNotPresent, v1.PullAlways, v1.PullNever:
	default:
		return fmt.Errorf("'%s' is not a valid trident image pull policy format", imagePullPolicy)
	}
	return nil
}

// imagePrechecks is important, it identifies the Trident version of the image that is provided as an input by spinning
// up a transient pod based of the image. This ensures we fail fast and not wait until the Trident installation.
func (i *Installer) imagePrechecks(labels, controllingCRDetails map[string]string) (string, error) {
	var performImageVersionCheck bool
	var identifiedImageVersion string

	currentDeployment, _, _, err := i.TridentDeploymentInformation(appLabel)
	if err != nil {
		return identifiedImageVersion,
			fmt.Errorf("unable to get existing deployment information for the image verification; err: %v", err)
	}

	if currentDeployment == nil {
		Log().Debugf("No Trident deployment exists.")
		performImageVersionCheck = true
	} else {
		Log().Debugf("Trident deployment exists.")

		containers := currentDeployment.Spec.Template.Spec.Containers

		for _, container := range containers {
			if container.Name == "trident-main" {
				if container.Image != tridentImage {
					Log().WithFields(LogFields{
						"currentTridentImage": container.Image,
						"newTridentImage":     tridentImage,
					}).Debugf("Current Trident installation image is not same as the new Trident Image.")
					performImageVersionCheck = true
				}
			}
		}

		// Contingency plan to recover Trident version information
		if currentDeployment.Labels[TridentVersionLabelKey] == "" {
			Log().Errorf("Deployment is missing the version information; fixing it.")
			performImageVersionCheck = true
		} else {
			identifiedImageVersion = currentDeployment.Labels[TridentVersionLabelKey]

			// Check if deployed Trident image matches the version this Operator supports
			if supportedVersion, err := versionutils.ParseDate(DefaultTridentVersion); err != nil {
				Log().WithField("version", DefaultTridentVersion).Error("Could not parse default version.")
				performImageVersionCheck = true
			} else if deploymentVersion, err := versionutils.ParseDate(identifiedImageVersion); err != nil {
				Log().WithField("version", identifiedImageVersion).Error("Could not parse deployment version.")
				performImageVersionCheck = true
			} else if supportedVersion.ShortString() != deploymentVersion.ShortString() {
				Log().Debugf("Current Trident deployment image '%s' is not same as the supported Trident image '%s'.",
					deploymentVersion.String(), supportedVersion.String())
				performImageVersionCheck = true
			}
		}
	}

	if performImageVersionCheck {
		Log().Debugf("Image version check needed.")

		// Create the RBAC objects for the trident version pod
		if _, err = i.createRBACObjects(controllingCRDetails, labels, false); err != nil {
			return "", fmt.Errorf("unable to create RBAC objects while verifying Trident version; err: %v", err)
		}

		tridentClientVersion, err := i.getTridentClientVersionInfo(tridentImage, controllingCRDetails)
		if err != nil {
			return "", fmt.Errorf("unable to get Trident image version information, please ensure correct "+
				"Trident image has been provided; err: %v", err)
		}

		tridentImageVersion, err := versionutils.ParseDate(tridentClientVersion.Client.Version)
		if err != nil {
			errMessage := fmt.Sprintf("unexpected parse error during Trident client version retrieval; err: %v", err)
			Log().Errorf(errMessage)
			return "", fmt.Errorf(errMessage)
		}

		supportedTridentVersion, err := versionutils.ParseDate(DefaultTridentVersion)
		if err != nil {
			errMessage := fmt.Sprintf("unexpected parse error during supported Trident version; err: %v", err)
			Log().Errorf(errMessage)
			return "", fmt.Errorf(errMessage)
		}

		tridentImageShortVersion := tridentImageVersion.ShortString()
		supportedTridentShortVersion := supportedTridentVersion.ShortString()

		if tridentImageShortVersion != supportedTridentShortVersion {
			errMessage := fmt.Sprintf("unsupported Trident image version '%s', supported Trident version is '%s'",
				tridentImageVersion.ShortStringWithRelease(), supportedTridentVersion.ShortStringWithRelease())
			Log().Errorf(errMessage)
			return "", fmt.Errorf(errMessage)
		}

		// need to append 'v', so that it can be stores in trident version label later
		identifiedImageVersion = "v" + tridentImageVersion.ShortStringWithRelease()
		Log().WithFields(LogFields{
			"tridentImage": tridentImage,
			"version":      tridentImageVersion.ShortStringWithRelease(),
		}).Debugf("New Trident image is supported.")
	}

	return identifiedImageVersion, nil
}

// setInstallationParams identifies the correct parameters for the Trident installation
func (i *Installer) setInstallationParams(
	cr netappv1.TridentOrchestrator, currentInstallationVersion string,
) (map[string]string, map[string]string, bool, error) {
	var identifiedImageVersion string
	var defaultImageOverride bool
	var tridentUpdateNeeded bool
	var returnError error

	// Get default values
	logFormat = DefaultLogFormat
	probePort = DefaultProbePort
	tridentImage = TridentImage
	imageRegistry = ""
	kubeletDir = DefaultKubeletDir
	autosupportImage = commonconfig.DefaultAutosupportImage
	httpTimeout = commonconfig.HTTPTimeoutString
	imagePullPolicy = DefaultImagePullPolicy

	imagePullSecrets = []string{}

	// Get values from CR
	enableForceDetach = cr.Spec.EnableForceDetach
	if cr.Spec.DisableAuditLog == nil {
		disableAuditLog = true
	} else {
		disableAuditLog = *cr.Spec.DisableAuditLog
	}

	useIPv6 = cr.Spec.IPv6
	windows = cr.Spec.Windows
	silenceAutosupport = cr.Spec.SilenceAutosupport
	if cr.Spec.AutosupportProxy != "" {
		autosupportProxy = cr.Spec.AutosupportProxy
	}
	if cr.Spec.AutosupportSerialNumber != "" {
		autosupportSerialNumber = cr.Spec.AutosupportSerialNumber
	}
	if cr.Spec.AutosupportHostname != "" {
		autosupportHostname = cr.Spec.AutosupportHostname
	}
	if cr.Spec.LogFormat != "" {
		logFormat = cr.Spec.LogFormat
	}
	if cr.Spec.Debug || cr.Spec.LogLevel == "debug" {
		logLevel = "debug"
		debug = true
	} else if cr.Spec.LogLevel != "" {
		logLevel = cr.Spec.LogLevel
		debug = false
	} else {
		logLevel = "info"
		debug = false
	}

	logWorkflows = cr.Spec.LogWorkflows
	logLayers = cr.Spec.LogLayers

	if cr.Spec.ProbePort != nil {
		probePort = strconv.FormatInt(*cr.Spec.ProbePort, 10)
	}
	if cr.Spec.KubeletDir != "" {
		kubeletDir = cr.Spec.KubeletDir
	}
	if len(cr.Spec.ImagePullSecrets) != 0 {
		imagePullSecrets = cr.Spec.ImagePullSecrets
	}
	if cr.Spec.ImageRegistry != "" {
		imageRegistry = cr.Spec.ImageRegistry
	}
	if cr.Spec.TridentImage != "" {
		tridentImage = cr.Spec.TridentImage

		if tridentImage != TridentImage {
			defaultImageOverride = true
		}
	} else {
		// Override registry only if using the default Trident image name and an alternate registry was supplied
		// Do not use 'imageRegistry' here, it gets set to a default value.
		if cr.Spec.ImageRegistry != "" {
			tridentImage = utils.ReplaceImageRegistry(tridentImage, cr.Spec.ImageRegistry)
		}
	}
	if cr.Spec.AutosupportImage != "" {
		autosupportImage = cr.Spec.AutosupportImage
	} else {
		// Override registry only if using the default Autosupport image name and an alternate registry was supplied
		// Do not use 'imageRegistry' here, it gets set to a default value.
		if cr.Spec.ImageRegistry != "" {
			autosupportImage = utils.ReplaceImageRegistry(autosupportImage, cr.Spec.ImageRegistry)
		}
	}

	if cr.Spec.HTTPRequestTimeout != "" {
		httpTimeout = cr.Spec.HTTPRequestTimeout
		httpTimeoutValue, err := time.ParseDuration(cr.Spec.HTTPRequestTimeout)
		if err != nil {
			return nil, nil, false, fmt.Errorf("could not parse the http request timeout as a duration: %v", err)
		}
		if httpTimeoutValue < 0 {
			return nil, nil, false, fmt.Errorf("a negative value was used for the http request timeout, which is" +
				" not supported")
		}
	}

	appLabel = TridentCSILabel
	appLabelKey = TridentCSILabelKey
	appLabelValue = TridentCSILabelValue

	if cr.Spec.ControllerPluginNodeSelector != nil {
		controllerPluginNodeSelector = cr.Spec.ControllerPluginNodeSelector
	}
	if cr.Spec.ControllerPluginTolerations != nil {
		controllerPluginTolerations = cr.Spec.ControllerPluginTolerations
	}
	if cr.Spec.NodePluginNodeSelector != nil {
		nodePluginNodeSelector = cr.Spec.NodePluginNodeSelector
	}
	if cr.Spec.NodePluginTolerations != nil {
		nodePluginTolerations = cr.Spec.NodePluginTolerations
	}
	if cr.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cr.Spec.ImagePullPolicy
	}

	// Owner Reference details set on each of the Trident object created by the operator
	controllingCRDetails := make(map[string]string)
	managedByCR := "true"

	controllingCRDetails[CRAPIVersionKey] = cr.APIVersion
	controllingCRDetails[CRController] = managedByCR
	controllingCRDetails[CRKind] = cr.Kind
	controllingCRDetails[CRName] = cr.Name
	controllingCRDetails[CRUID] = string(cr.UID)

	// Label that are set on each of the Trident object created by the operator
	labels := make(map[string]string)

	labels[appLabelKey] = appLabelValue
	labels[K8sVersionLabelKey] = "v" + i.client.ServerVersion().ShortStringWithRelease()

	// Perform tridentImage Version check and identify the Trident version
	if defaultImageOverride {
		identifiedImageVersion, returnError = i.imagePrechecks(labels, controllingCRDetails)
		if returnError != nil {
			return nil, nil, false, returnError
		}

		Log().WithFields(LogFields{
			"tridentImage": tridentImage,
			"version":      identifiedImageVersion,
		}).Debugf("Identified Trident image's version.")
	} else {
		identifiedImageVersion = TridentVersionLabelValue

		Log().WithFields(LogFields{
			"tridentImage": tridentImage,
			"version":      identifiedImageVersion,
		}).Debugf("Using default Trident image.")
	}

	// Identify if this is an update scenario, i.e. Trident version has changed
	if currentInstallationVersion != identifiedImageVersion {
		Log().WithFields(LogFields{
			"currentVersion": currentInstallationVersion,
			"newVersion":     identifiedImageVersion,
		}).Infof("Current installed Trident version is not same as the new Trident image version; need" +
			" to update the Trident installation.")
		tridentUpdateNeeded = true
	}
	// Perform log prechecks
	if returnError = i.logFormatPrechecks(); returnError != nil {
		return nil, nil, false, returnError
	}

	// Preform image pull policy prechecks
	if returnError = i.imagePullPolicyPrechecks(); returnError != nil {
		return nil, nil, false, returnError
	}

	// Update the label with the correct version
	labels[TridentVersionLabelKey] = identifiedImageVersion

	return controllingCRDetails, labels, tridentUpdateNeeded, nil
}

func (i *Installer) InstallOrPatchTrident(
	cr netappv1.TridentOrchestrator, currentInstallationVersion string, k8sUpdateNeeded, crdUpdateNeeded bool,
) (*netappv1.TridentOrchestratorSpecValues, string, error) {
	var returnError error
	reuseServiceAccountMap := make(map[string]bool)

	// Set installation params
	controllingCRDetails, labels, tridentUpdateNeeded, err := i.setInstallationParams(cr, currentInstallationVersion)
	if err != nil {
		return nil, "", err
	}

	// Remove any leftover transient Trident
	versionPodLabel := TridentVersionPodLabel
	err = i.client.DeleteTransientVersionPod(versionPodLabel)
	if err != nil {
		Log().WithError(err).Warn("Could not remove transient Trident pods.")
	}

	// Identify if update is required because of change in K8s version or Trident Operator version
	shouldUpdate := k8sUpdateNeeded || tridentUpdateNeeded

	// Begin Trident installation logic...

	// All checks succeeded, so proceed with installation
	Log().WithField("namespace", i.namespace).Info("Starting Trident installation.")

	// Create namespace, if one does not exist
	if returnError = i.createOrPatchTridentInstallationNamespace(); returnError != nil {
		return nil, "", returnError
	}

	// Create or patch or update the RBAC objects
	if reuseServiceAccountMap, returnError = i.createRBACObjects(controllingCRDetails, labels,
		shouldUpdate); returnError != nil {
		return nil, "", returnError
	}

	// Create CRDs and ensure they are established
	returnError = i.createAndEnsureCRDs(crdUpdateNeeded)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident CRDs; %v", returnError)
		return nil, "", returnError
	}

	// Patch the CRD definitions with finalizers to protect them
	if returnError = i.client.AddFinalizerToCRDs(CRDnames); returnError != nil {
		return nil, "", fmt.Errorf("failed to add finalizers for Trident CRDs %v; err: %v", CRDnames, returnError)
	}

	// Create or patch or update the RBAC PSPs
	if i.isPSPSupported() {
		returnError = i.createOrPatchTridentPodSecurityPolicy(controllingCRDetails, labels, shouldUpdate)
		if returnError != nil {
			returnError = fmt.Errorf("failed to create the Trident pod security policy; %v", returnError)
			return nil, "", returnError
		}
	}

	// Create or update the CSI Driver object if necessary
	returnError = i.createOrPatchK8sCSIDriver(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Kubernetes CSI Driver object; %v", returnError)
		return nil, "", returnError
	}

	// Create or patch or update the Trident Service
	returnError = i.createOrPatchTridentService(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident Service; %v", returnError)
		return nil, "", returnError
	}

	// Create or update the Trident Secret
	returnError = i.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident Protocol Secret; %v", returnError)
		return nil, "", returnError
	}

	// Create or update the Trident Encryption Secret
	returnError = i.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident Encryption Secret; %v", returnError)
		return nil, "", returnError
	}

	// Create or update the Trident Resource Quota
	returnError = i.createOrPatchTridentResourceQuota(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident Resource Quota; %v", returnError)
		return nil, "", returnError
	}

	// Create or update the Trident CSI deployment
	returnError = i.createOrPatchTridentDeployment(controllingCRDetails, labels, shouldUpdate, reuseServiceAccountMap)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident Deployment; %v", returnError)
		return nil, "", returnError
	}

	returnError = i.createOrPatchTridentDaemonSet(controllingCRDetails, labels, shouldUpdate, reuseServiceAccountMap,
		false)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create or patch Trident daemonset %s; %v", getDaemonSetName(false),
			returnError)
		return nil, "", returnError
	}

	// Create or update the Trident CSI daemonset
	if windows {
		returnError = i.createOrPatchTridentDaemonSet(controllingCRDetails, labels, shouldUpdate,
			reuseServiceAccountMap,
			true)
		if returnError != nil {
			returnError = fmt.Errorf("failed to create or patch Trident daemonset %s; %v", getDaemonSetName(true),
				returnError)
			return nil, "", returnError
		}
	}

	// Wait for Trident pod to be running
	var tridentPod *v1.Pod

	tridentPod, returnError = i.waitForTridentPod()
	if returnError != nil {
		return nil, "", returnError
	}

	// Wait for Trident REST interface to be available
	returnError = i.waitForRESTInterface(tridentPod.Name)
	if returnError != nil {
		returnError = fmt.Errorf("%v; use 'tridentctl logs' to learn more", returnError)
		return nil, "", returnError
	}

	identifiedSpecValues := netappv1.TridentOrchestratorSpecValues{
		EnableForceDetach:       strconv.FormatBool(enableForceDetach),
		DisableAuditLog:         strconv.FormatBool(disableAuditLog),
		LogFormat:               logFormat,
		Debug:                   strconv.FormatBool(debug),
		LogLevel:                determineLogLevel(),
		LogWorkflows:            logWorkflows,
		LogLayers:               logLayers,
		TridentImage:            tridentImage,
		ImageRegistry:           imageRegistry,
		IPv6:                    strconv.FormatBool(useIPv6),
		SilenceAutosupport:      strconv.FormatBool(silenceAutosupport),
		ProbePort:               probePort,
		AutosupportImage:        autosupportImage,
		AutosupportProxy:        autosupportProxy,
		AutosupportSerialNumber: autosupportSerialNumber,
		AutosupportHostname:     autosupportHostname,
		KubeletDir:              kubeletDir,
		K8sTimeout:              strconv.Itoa(int(k8sTimeout.Seconds())),
		HTTPRequestTimeout:      httpTimeout,
		ImagePullSecrets:        imagePullSecrets,
		NodePluginNodeSelector:  nodePluginNodeSelector,
		NodePluginTolerations:   nodePluginTolerations,
		ImagePullPolicy:         imagePullPolicy,
	}

	Log().WithFields(LogFields{
		"namespace": i.namespace,
		"version":   labels[TridentVersionLabelKey],
	}).Info("Trident is installed.")
	return &identifiedSpecValues, labels[TridentVersionLabelKey], nil
}

// createCRDs creates and establishes each of the CRDs individually
func (i *Installer) createCRDs(performOperationOnce bool) error {
	var err error

	if err = i.CreateOrPatchCRD(VersionCRDName, k8sclient.GetVersionCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(BackendCRDName, k8sclient.GetBackendCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(SnapshotInfoCRDName, k8sclient.GetSnapshotInfoCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(BackendConfigCRDName, k8sclient.GetBackendConfigCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(StorageClassCRDName, k8sclient.GetStorageClassCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(VolumeCRDName, k8sclient.GetVolumeCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(VolumePublicationCRDName, k8sclient.GetVolumePublicationCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(NodeCRDName, k8sclient.GetNodeCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(TransactionCRDName, k8sclient.GetTransactionCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(SnapshotCRDName, k8sclient.GetSnapshotCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(VolumeReferenceCRDName, k8sclient.GetVolumeReferenceCRDYAML(), false); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(MirrorRelationshipCRDName, k8sclient.GetMirrorRelationshipCRDYAML(),
		performOperationOnce); err != nil {
		return err
	}
	if err = i.CreateOrPatchCRD(ActionMirrorUpdateCRDName, k8sclient.GetActionMirrorUpdateCRDYAML(),
		performOperationOnce); err != nil {
		return err
	}

	return err
}

// CreateOrPatchCRD creates and establishes a CRD or patches an existing one.
// TODO: Once Trident v22.01 approaches EOL or CRD versioning schema is established,
//
//	re-evaluate if performOperationOnce is necessary.
func (i *Installer) CreateOrPatchCRD(crdName, crdYAML string, performOperationOnce bool) error {
	var currentCRD *apiextensionv1.CustomResourceDefinition
	var err error

	// Discover CRD data.
	crdExists, err := i.client.CheckCRDExists(crdName)
	if err != nil {
		return fmt.Errorf("unable to identify if %v CRD exists; err: %v", crdName, err)
	}

	if crdExists {
		// Avoid patching the CRD iff the CRD already exists and performOperationOnce is false.
		if !performOperationOnce {
			Log().WithFields(LogFields{
				"CRD":       crdName,
				"crdExists": crdExists,
			}).Debugf("Found existing %s CRD; creation or patch is not required.", crdName)
			return nil
		}

		currentCRD, err = i.client.GetCRD(crdName)
		if err != nil {
			return fmt.Errorf("could not retrieve the %v CRD; %v", crdName, err)
		}
	}

	return i.client.PutCustomResourceDefinition(currentCRD, crdName, !crdExists, crdYAML)
}

func (i *Installer) createOrPatchK8sCSIDriver(controllingCRDetails, labels map[string]string, shouldUpdate bool) error {
	CSIDriverName := getCSIDriverName()

	currentCSIDriver, unwantedCSIDrivers, createCSIDriver, err := i.client.GetCSIDriverInformation(CSIDriverName,
		appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get K8s CSI drivers; %v", err)
	}

	if err = i.client.RemoveMultipleCSIDriverCRs(unwantedCSIDrivers); err != nil {
		return fmt.Errorf("failed to remove unwanted K8s CSI drivers; %v", err)
	}

	newK8sCSIDriverYAML := k8sclient.GetCSIDriverYAML(CSIDriverName, labels, controllingCRDetails)

	err = i.client.PutCSIDriver(currentCSIDriver, createCSIDriver, newK8sCSIDriverYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch K8s CSI drivers; %v", err)
	}

	return nil
}

func (i *Installer) createRBACObjects(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) (reuseServiceAccountMap map[string]bool, returnError error) {
	// Create service account
	reuseServiceAccountMap, returnError = i.createOrPatchTridentServiceAccounts(controllingCRDetails, labels, false)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident service account; %v", returnError)
		return
	}

	// Create cluster role
	returnError = i.createOrPatchTridentClusterRole(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident cluster role; %v", returnError)
		return
	}

	// Create cluster role bindings
	returnError = i.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident cluster role binding; %v", returnError)
		return
	}

	// Create roles
	returnError = i.createOrPatchTridentRoles(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident role; %v", returnError)
		return
	}

	// Create role bindings
	returnError = i.createOrPatchTridentRoleBindings(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident role binding; %v", returnError)
		return
	}

	// If OpenShift, create a new security context constraint for Trident
	if i.client.Flavor() == k8sclient.FlavorOpenShift {
		// Create OpenShift SCC
		returnError = i.createOrPatchTridentOpenShiftSCC(controllingCRDetails, labels, shouldUpdate)
		if returnError != nil {
			returnError = fmt.Errorf("failed to create the Trident OpenShift SCC; %v", returnError)
			return
		}
	}

	return
}

func (i *Installer) createOrPatchTridentInstallationNamespace() error {
	createNamespace := true

	namespaceExists, returnError := i.client.CheckNamespaceExists(i.namespace)
	if returnError != nil {
		Log().WithFields(LogFields{
			"namespace": i.namespace,
			"err":       returnError,
		}).Errorf("Unable to check if namespace exists.")
		return fmt.Errorf("unable to check if namespace %s exists; %v", i.namespace, returnError)
	} else if namespaceExists {
		Log().WithField("namespace", i.namespace).Debug("Namespace exists.")
		createNamespace = false
	} else {
		Log().WithField("namespace", i.namespace).Debug("Namespace does not exist.")
	}

	if createNamespace {
		// Create namespace
		err := i.client.CreateObjectByYAML(k8sclient.GetNamespaceYAML(i.namespace))
		if err != nil {
			return fmt.Errorf("failed to create Trident installation namespace %s; %v", i.namespace, err)
		}
		Log().WithField("namespace", i.namespace).Info("Created Trident installation namespace.")
	} else {
		// Patch namespace
		err := i.client.PatchNamespaceLabels(i.namespace, map[string]string{
			commonconfig.PodSecurityStandardsEnforceLabel: commonconfig.PodSecurityStandardsEnforceProfile,
		})
		if err != nil {
			return fmt.Errorf("failed to patch Trident installation namespace %s; %v", i.namespace, err)
		}
		Log().WithField("namespace", i.namespace).Info("Patched Trident installation namespace")
	}

	return nil
}

func (i *Installer) createOrPatchTridentServiceAccounts(
	controllingCRDetails, _ map[string]string, shouldUpdate bool,
) (map[string]bool, error) {
	serviceAccountNames := getRBACResourceNames()

	// Service account details are fetched with controller label
	currentServiceAccountMap,
		unwantedServiceAccounts,
		serviceAccountSecretMap,
		reuseServiceAccountMap, err := i.client.GetMultipleServiceAccountInformation(serviceAccountNames, appLabel,
		i.namespace, shouldUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to get Trident service accounts with controller label; %v", err)
	}

	// Service account details are fetched with node label
	nodeServiceAccountMap,
		unwantedNodeServiceAccounts,
		nodeServiceAccountSecretMap,
		reuseNodeServiceAccountMap, err := i.client.GetMultipleServiceAccountInformation(serviceAccountNames,
		TridentNodeLabel,
		i.namespace, shouldUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to get Trident service accounts with node label; %v", err)
	}

	// Merge the maps together
	for key, value := range nodeServiceAccountMap {
		currentServiceAccountMap[key] = value
	}

	for key, value := range nodeServiceAccountSecretMap {
		serviceAccountSecretMap[key] = value
	}

	for key, value := range reuseNodeServiceAccountMap {
		reuseServiceAccountMap[key] = value
	}

	combinedUnwantedServiceAccounts := append(unwantedServiceAccounts, unwantedNodeServiceAccounts...)

	if err = i.client.RemoveMultipleServiceAccounts(combinedUnwantedServiceAccounts); err != nil {
		return nil, fmt.Errorf("failed to remove unwanted service accounts; %v", err)
	}

	for _, accountName := range serviceAccountNames {
		labelMap, labelString := getAppLabelForResource(accountName)
		newServiceAccountYAML := k8sclient.GetServiceAccountYAML(accountName, serviceAccountSecretMap[accountName],
			labelMap, controllingCRDetails)

		_, err = i.client.PutServiceAccount(currentServiceAccountMap[accountName], reuseServiceAccountMap[accountName],
			newServiceAccountYAML, labelString)
		if err != nil {
			return nil, fmt.Errorf("failed to create or patch Trident service account %v got error: %v", accountName,
				err)
		}
	}

	return reuseServiceAccountMap, nil
}

func (i *Installer) createOrPatchTridentClusterRole(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	clusterRoleName := getControllerRBACResourceName()

	currentClusterRole, unwantedClusterRoles, createClusterRole, err := i.client.GetClusterRoleInformation(
		clusterRoleName, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident cluster roles; %v", err)
	}

	// Retrieve cluster roles with node label
	// This needs to happen to identify the cluster roles defined for node pods, prior to 23.xx so that
	// they can be removed
	nodeClusterRoles, err := i.client.GetClusterRolesByLabel(TridentNodeLabel)
	if err == nil && len(nodeClusterRoles) > 0 {
		unwantedClusterRoles = append(unwantedClusterRoles, nodeClusterRoles...)
	}

	if err = i.client.RemoveMultipleClusterRoles(unwantedClusterRoles); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident cluster roles; %v", err)
	}

	newClusterRoleYAML := k8sclient.GetClusterRoleYAML(clusterRoleName, labels, controllingCRDetails)

	err = i.client.PutClusterRole(currentClusterRole, createClusterRole, newClusterRoleYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident cluster role; %v", err)
	}

	return nil
}

func (i *Installer) createOrPatchTridentRoles(
	controllingCRDetails, _ map[string]string, shouldUpdate bool,
) error {
	// Operator should create Role and RoleBinding for node resources only if PSP is supported
	roleNames := []string{getControllerRBACResourceName()}
	if i.isPSPSupported() {
		roleNames = append(roleNames, getNodeResourceNames()...)
	}

	// Retrieve roles with controller label
	currentRoleMap, unwantedRoles, reuseRoleMap, err := i.client.GetMultipleRoleInformation(
		roleNames, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident roles with controller label; %v", err)
	}

	combinedUnwantedRoles := unwantedRoles

	if i.isPSPSupported() {
		// Retrieve roles with node label
		nodeRoleMap, unwantedNodeRoles, reuseNodeRoleMap, err := i.client.GetMultipleRoleInformation(
			roleNames, TridentNodeLabel, shouldUpdate)
		if err != nil {
			return fmt.Errorf("failed to get Trident roles with node label; %v", err)
		}

		// Merging the maps together
		for key, value := range nodeRoleMap {
			currentRoleMap[key] = value
		}

		for key, value := range reuseNodeRoleMap {
			reuseRoleMap[key] = value
		}
		combinedUnwantedRoles = append(combinedUnwantedRoles, unwantedNodeRoles...)
	}

	if err = i.client.RemoveMultipleRoles(combinedUnwantedRoles); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident roles; %v", err)
	}

	for _, roleName := range roleNames {
		labelMap, labelString := getAppLabelForResource(roleName)
		newRoleYAML := k8sclient.GetRoleYAML(i.client.Namespace(), roleName, labelMap, controllingCRDetails)

		err = i.client.PutRole(currentRoleMap[roleName], reuseRoleMap[roleName],
			newRoleYAML, labelString)
		if err != nil {
			return fmt.Errorf("failed to create or patch Trident role name %v got error; %v", roleName, err)
		}
	}

	return nil
}

func (i *Installer) createOrPatchTridentRoleBindings(
	controllingCRDetails, _ map[string]string, shouldUpdate bool,
) error {
	// Operator should create Role and RoleBinding for node resources only if PSP is supported
	roleBindingNames := []string{getControllerRBACResourceName()}
	if i.isPSPSupported() {
		roleBindingNames = append(roleBindingNames, getNodeResourceNames()...)
	}

	currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap,
		err := i.client.GetMultipleRoleBindingInformation(roleBindingNames, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident role bindings with controller label; %v", err)
	}

	combinedUnwantedRoleBindings := unwantedRoleBindings

	if i.isPSPSupported() {
		// Retrieve role bindings with node label
		nodeRoleBindingMap, unwantedNodeRoleBindings, reuseNodeRoleBindingMap,
			err := i.client.GetMultipleRoleBindingInformation(roleBindingNames, TridentNodeLabel, shouldUpdate)
		if err != nil {
			return fmt.Errorf("failed to get Trident role bindings with node label; %v", err)
		}

		// Merging the maps together
		for key, value := range nodeRoleBindingMap {
			currentRoleBindingMap[key] = value
		}
		for key, value := range reuseNodeRoleBindingMap {
			reuseRoleBindingMap[key] = value
		}

		combinedUnwantedRoleBindings = append(combinedUnwantedRoleBindings, unwantedNodeRoleBindings...)
	}

	if err = i.client.RemoveMultipleRoleBindings(combinedUnwantedRoleBindings); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident role bindings; %v", err)
	}

	for _, roleBindingName := range roleBindingNames {
		labelMap, labelString := getAppLabelForResource(roleBindingName)
		newRoleBindingYAML := k8sclient.GetRoleBindingYAML(i.namespace, roleBindingName,
			labelMap, controllingCRDetails)

		err = i.client.PutRoleBinding(currentRoleBindingMap[roleBindingName],
			reuseRoleBindingMap[roleBindingName], newRoleBindingYAML, labelString)

		if err != nil {
			return fmt.Errorf("failed to create or patch Trident role binding; %v", err)
		}
	}

	return nil
}

func (i *Installer) createOrPatchTridentClusterRoleBinding(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	clusterRoleBindingName := getControllerRBACResourceName()

	currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding,
		err := i.client.GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident cluster role bindings; %v", err)
	}

	// Retrieve cluster role bindings with node label
	// This needs to happen to identify the cluster role bindings defined for node pods, prior to 23.xx so that
	// they can be removed
	nodeClusterRoleBindings, err := i.client.GetClusterRoleBindingsByLabel(TridentNodeLabel)
	if err == nil && len(nodeClusterRoleBindings) > 0 {
		unwantedClusterRoleBindings = append(unwantedClusterRoleBindings, nodeClusterRoleBindings...)
	}

	if err = i.client.RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident cluster role bindings; %v", err)
	}

	newClusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(i.namespace, clusterRoleBindingName,
		i.client.Flavor(), labels, controllingCRDetails)

	err = i.client.PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding,
		newClusterRoleBindingYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident cluster role binding; %v", err)
	}

	return nil
}

func (i *Installer) createOrPatchTridentOpenShiftSCC(
	controllingCRDetails, _ map[string]string, shouldUpdate bool,
) error {
	openShiftSCCUserNames := getRBACResourceNames()
	openShiftSCCNames := getRBACResourceNames()

	currentOpenShiftSCCJSONMap, reuseOpenShiftSCCMap, removeExistingSCCMap, err := i.client.GetMultipleTridentOpenShiftSCCInformation(
		openShiftSCCUserNames, openShiftSCCNames, shouldUpdate)
	if err != nil {
		Log().WithFields(nil).Errorf("Unable to get OpenShift SCC for Trident; err: %v", err)
		return fmt.Errorf("failed to get Trident OpenShift SCC; %v", err)
	}

	deleteOpenShiftSCCByName := func(sccName string) error {
		var err error
		err = i.client.DeleteObjectByYAML(k8sclient.GetOpenShiftSCCQueryYAML(sccName), true)
		if err != nil {
			logFields := LogFields{"sccName": sccName}
			logFields["err"] = err
			Log().WithFields(logFields).Errorf("Unable to delete OpenShift SCC.")
			return fmt.Errorf("unable to delete OpenShift SCC name %v got error: %v", sccName, err)
		}
		return err
	}

	// Remove old trident SCC during upgrade scenario
	if err = deleteOpenShiftSCCByName(getOpenShiftSCCName()); err != nil {
		return err
	}

	for idx := 0; idx < len(openShiftSCCNames); idx++ {
		if removeExistingSCCMap[openShiftSCCNames[idx]] {
			if err = deleteOpenShiftSCCByName(openShiftSCCNames[idx]); err != nil {
				return err
			}
		}

		appLabels, _ := getAppLabelForResource(openShiftSCCNames[idx])
		newOpenShiftSCCYAML := k8sclient.GetOpenShiftSCCYAML(openShiftSCCNames[idx], openShiftSCCUserNames[idx],
			i.namespace,
			appLabels, controllingCRDetails, isLinuxNodeSCCUser(openShiftSCCUserNames[idx]))

		err = i.client.PutOpenShiftSCC(currentOpenShiftSCCJSONMap[openShiftSCCNames[idx]],
			reuseOpenShiftSCCMap[openShiftSCCNames[idx]],
			newOpenShiftSCCYAML)
		if err != nil {
			return fmt.Errorf("failed to create or patch Trident OpenShift SCC name %v got error: %v",
				openShiftSCCNames[idx], err)
		}
	}

	return nil
}

func (i *Installer) createAndEnsureCRDs(performOperationOnce bool) (returnError error) {
	returnError = i.createCRDs(performOperationOnce)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident CRDs; %v", returnError)
	}
	return
}

func (i *Installer) createOrPatchTridentPodSecurityPolicy(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	// Creating PodSecurityPolicy for controller
	currentPSPMap, unwantedPSPs, reusePSPMap, err := i.client.GetMultiplePodSecurityPolicyInformation(
		[]string{getControllerRBACResourceName()}, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident controller pod security policies; %v", err)
	}

	if err = i.client.RemoveMultiplePodSecurityPolicies(unwantedPSPs); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident controller pod security policies; %v", err)
	}

	newPSPYAML := k8sclient.GetUnprivilegedPodSecurityPolicyYAML(getControllerRBACResourceName(), labels,
		controllingCRDetails)
	if err = i.client.PutPodSecurityPolicy(
		currentPSPMap[getControllerRBACResourceName()],
		reusePSPMap[getControllerRBACResourceName()],
		newPSPYAML,
		appLabel); err != nil {
		return fmt.Errorf("failed to create or patch Trident controller pod security policy; %v", err)
	}

	nodeLabels, _ := getAppLabelForResource(getNodeRBACResourceName(false))
	// Creating PodSecurityPolicy for node
	nodePSPMap, unwantedNodePSPs, reuseNodePSP, err := i.client.GetMultiplePodSecurityPolicyInformation(
		[]string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}, TridentNodeLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident node pod security policies; %v", err)
	}

	if err = i.client.RemoveMultiplePodSecurityPolicies(unwantedNodePSPs); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident node pod security policies; %v", err)
	}

	newPSPYAML = k8sclient.GetPrivilegedPodSecurityPolicyYAML(getNodeRBACResourceName(false), nodeLabels,
		controllingCRDetails)
	if err = i.client.PutPodSecurityPolicy(
		nodePSPMap[getNodeRBACResourceName(false)],
		reuseNodePSP[getNodeRBACResourceName(false)],
		newPSPYAML,
		TridentNodeLabel); err != nil {
		return fmt.Errorf("failed to create or patch Trident linux pod security policy; %v", err)
	}

	// Creating PodSecurityPolicy for windows node
	if windows {
		newPSPYAML := k8sclient.GetUnprivilegedPodSecurityPolicyYAML(getNodeRBACResourceName(true), nodeLabels,
			controllingCRDetails)
		if err = i.client.PutPodSecurityPolicy(
			nodePSPMap[getNodeRBACResourceName(true)],
			reuseNodePSP[getNodeRBACResourceName(true)],
			newPSPYAML,
			TridentNodeLabel); err != nil {
			return fmt.Errorf("failed to create or patch windows node pod security policy; %v", err)
		}
	}
	return nil
}

func (i *Installer) createOrPatchTridentService(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	serviceName := getServiceName()

	currentService, unwantedServices, createService, err := i.client.GetServiceInformation(serviceName, appLabel,
		i.namespace, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident services; %v", err)
	}

	if err = i.client.RemoveMultipleServices(unwantedServices); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident services; %v", err)
	}

	newServiceYAML := k8sclient.GetCSIServiceYAML(serviceName, labels, controllingCRDetails)

	if err = i.client.PutService(currentService, createService, newServiceYAML, appLabel); err != nil {
		return fmt.Errorf("failed to create or patch Trident service; %v", err)
	}

	return nil
}

func (i *Installer) createOrPatchTridentProtocolSecret(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	secretMap := make(map[string]string)
	secretName := getProtocolSecretName()

	_, unwantedSecrets, createSecret, err := i.client.GetSecretInformation(secretName, appLabel, i.namespace,
		shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident secrets; %v", err)
	}

	if err = i.client.RemoveMultipleSecrets(unwantedSecrets); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident secrets; %v", err)
	}

	if createSecret {
		// Create the certificates for the CSI controller's HTTPS REST interface
		certInfo, err := crypto.MakeHTTPCertInfo(commonconfig.CACertName, commonconfig.ServerCertName,
			commonconfig.ClientCertName)
		if err != nil {
			return fmt.Errorf("failed to create Trident X509 certificates; %v", err)
		}

		// Create the secret for the HTTP certs & keys
		secretMap = map[string]string{
			commonconfig.CAKeyFile:      certInfo.CAKey,
			commonconfig.CACertFile:     certInfo.CACert,
			commonconfig.ServerKeyFile:  certInfo.ServerKey,
			commonconfig.ServerCertFile: certInfo.ServerCert,
			commonconfig.ClientKeyFile:  certInfo.ClientKey,
			commonconfig.ClientCertFile: certInfo.ClientCert,
		}
	}

	newSecretYAML := k8sclient.GetSecretYAML(secretName, i.namespace, labels, controllingCRDetails, secretMap, nil)

	if err = i.client.PutSecret(createSecret, newSecretYAML, getProtocolSecretName()); err != nil {
		return fmt.Errorf("failed to create or patch Trident secret; %v", err)
	}

	return nil
}

// createOrConsumeTridentEncryptionSecret checks for a Trident Encryption Secret or uses an existing one if it exists;
// This helper disregards shouldUpdate to avoid destroying the Trident Encryption Secret across upgrades scenarios.
func (i *Installer) createOrConsumeTridentEncryptionSecret(
	controllingCRDetails, labels map[string]string, _ bool,
) error {
	secretName := getEncryptionSecretName()

	// Check if Trident's Encryption Secret already exists.
	secretExists, err := i.client.CheckSecretExists(secretName)
	if err != nil {
		return fmt.Errorf("failed to check for existing Trident encryption secret; %v", err)
	}

	// Create a new Trident Encryption Secret if it doesn't exist.
	if !secretExists {

		// Generate a fresh AES Key because the Trident Crypto Secret doesn't exist.
		aesKey, err := crypto.GenerateAESKey()
		if err != nil {
			return fmt.Errorf("failed to generate secure AES key; %v", err)
		}

		// Create the secret map
		secretMap := make(map[string]string)
		secretMap = map[string]string{
			commonconfig.AESKeyFile: aesKey,
		}

		// Setup persistent object labels.
		persistentObjectLabels := make(map[string]string)
		persistentObjectLabels[TridentPersistentObjectLabelKey] = TridentPersistentObjectLabelValue

		// Add default labels to maintain consistency in Trident related K8s objects.
		for key, value := range labels {
			persistentObjectLabels[key] = value
		}

		newSecretYAML := k8sclient.GetSecretYAML(secretName, i.namespace, persistentObjectLabels, controllingCRDetails,
			secretMap, nil)

		if err = i.client.PutSecret(true, newSecretYAML, getEncryptionSecretName()); err != nil {
			return fmt.Errorf("failed to create Trident encryption secret; %v", err)
		}
	}

	return nil
}

// createOrPatchTridentResourceQuota creates a Trident Resource Quota or patches an existing one;
// this should happen before the Trident Deployment and DaemonSet are created.
func (i *Installer) createOrPatchTridentResourceQuota(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	resourceQuotaName := getResourceQuotaName()
	nodeLabel := TridentNodeLabel

	currentResourceQuota, unwantedResourceQuotas, createResourceQuota, err := i.client.GetResourceQuotaInformation(
		resourceQuotaName, nodeLabel, i.namespace)
	if err != nil {
		return fmt.Errorf("failed to get Trident resource quota information; %v", err)
	}

	// Create a new resource quota if there is a current resource quota, but it should be updated.
	if currentResourceQuota != nil && shouldUpdate {
		unwantedResourceQuotas = append(unwantedResourceQuotas, *currentResourceQuota)
		createResourceQuota = true
	}

	if err = i.client.RemoveMultipleResourceQuotas(unwantedResourceQuotas); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident resource quotas; %v", err)
	}

	// copy all labels then replace the app label locally to avoid mutating the map of labels.
	nodeLabels := make(map[string]string)
	for k, v := range labels {
		nodeLabels[k] = v
	}
	nodeLabels[appLabelKey] = TridentNodeLabelValue
	newResourceQuotaYAML := k8sclient.GetResourceQuotaYAML(resourceQuotaName, i.namespace, nodeLabels,
		controllingCRDetails)

	err = i.client.PutResourceQuota(currentResourceQuota, createResourceQuota, newResourceQuotaYAML, nodeLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident resource quota; %v", err)
	}

	return nil
}

func (i *Installer) createOrPatchTridentDeployment(
	controllingCRDetails, labels map[string]string, shouldUpdate bool, reuseServiceAccountMap map[string]bool,
) error {
	topologyEnabled, err := i.client.IsTopologyInUse()
	if err != nil {
		return fmt.Errorf("failed to determine if node topology settings; %v", err)
	}

	deploymentName := getDeploymentName()
	serviceAccName := getControllerRBACResourceName()

	currentDeployment, unwantedDeployments, createDeployment, err := i.client.GetDeploymentInformation(deploymentName,
		appLabel, i.namespace)
	if err != nil {
		return fmt.Errorf("failed to get Trident deployment information; %v", err)
	}

	// Create a new deployment if there is a current deployment and
	// a new service account or it should be updated
	if currentDeployment != nil && (shouldUpdate || !reuseServiceAccountMap[serviceAccName]) {
		unwantedDeployments = append(unwantedDeployments, *currentDeployment)
		createDeployment = true
	}

	var tolerations []map[string]string
	if controllerPluginTolerations != nil {
		tolerations = make([]map[string]string, 0)
		for _, t := range controllerPluginTolerations {
			tolerations = append(tolerations, t.GetMap())
		}
	}

	if err = i.client.RemoveMultipleDeployments(unwantedDeployments); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident deployments; %v", err)
	}

	deploymentArgs := &k8sclient.DeploymentYAMLArguments{
		DeploymentName:          deploymentName,
		TridentImage:            tridentImage,
		AutosupportImage:        autosupportImage,
		AutosupportProxy:        autosupportProxy,
		AutosupportCustomURL:    "",
		AutosupportSerialNumber: autosupportSerialNumber,
		AutosupportHostname:     autosupportHostname,
		ImageRegistry:           imageRegistry,
		Debug:                   debug,
		LogLevel:                determineLogLevel(),
		LogWorkflows:            logWorkflows,
		LogLayers:               logLayers,
		LogFormat:               logFormat,
		DisableAuditLog:         disableAuditLog,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    controllingCRDetails,
		UseIPv6:                 useIPv6,
		SilenceAutosupport:      silenceAutosupport,
		Version:                 i.client.ServerVersion(),
		TopologyEnabled:         topologyEnabled,
		HTTPRequestTimeout:      httpTimeout,
		NodeSelector:            controllerPluginNodeSelector,
		Tolerations:             tolerations,
		ServiceAccountName:      serviceAccName,
		ImagePullPolicy:         imagePullPolicy,
		EnableForceDetach:       enableForceDetach,
	}

	newDeploymentYAML := k8sclient.GetCSIDeploymentYAML(deploymentArgs)

	err = i.client.PutDeployment(currentDeployment, createDeployment, newDeploymentYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident deployment; %v", err)
	}

	return nil
}

// TridentDeploymentInformation identifies the Operator based Trident CSI deployment and unwanted deployments,
// this method can be used for multiple purposes at different point during the Reconcile so it makes sense to
// keep it separate from the createOrPatchTridentDeployment
func (i *Installer) TridentDeploymentInformation(
	deploymentLabel string,
) (*appsv1.Deployment, []appsv1.Deployment, bool, error) {
	return i.client.GetDeploymentInformation(getDeploymentName(), deploymentLabel, i.namespace)
}

func (i *Installer) createOrPatchTridentDaemonSet(
	controllingCRDetails, labels map[string]string, shouldUpdate bool, reuseServiceAccountMap map[string]bool,
	isWindows bool,
) error {
	nodeLabel := TridentNodeLabel
	serviceAccountName := getNodeRBACResourceName(isWindows)

	currentDaemonSet, unwantedDaemonSets, createDaemonSet, err := i.client.GetDaemonSetInformation(nodeLabel,
		i.namespace, isWindows)
	if err != nil {
		return fmt.Errorf("failed to get Trident daemonsets; %v", err)
	}

	// Create a new daemonset if there is a current daemonset and
	// a new service account or it should be updated
	if currentDaemonSet != nil && (shouldUpdate || !reuseServiceAccountMap[serviceAccountName]) {
		unwantedDaemonSets = append(unwantedDaemonSets, *currentDaemonSet)
		createDaemonSet = true
	}

	if err = i.client.RemoveMultipleDaemonSets(unwantedDaemonSets); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident daemonsets; %v", err)
	}

	labels[appLabelKey] = TridentNodeLabelValue

	var tolerations []map[string]string
	if nodePluginTolerations != nil {
		tolerations = make([]map[string]string, 0)
		for _, t := range nodePluginTolerations {
			tolerations = append(tolerations, t.GetMap())
		}
	}

	daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
		DaemonsetName:        getDaemonSetName(isWindows),
		TridentImage:         tridentImage,
		ImageRegistry:        imageRegistry,
		KubeletDir:           kubeletDir,
		LogFormat:            logFormat,
		DisableAuditLog:      disableAuditLog,
		Debug:                debug,
		LogLevel:             determineLogLevel(),
		LogWorkflows:         logWorkflows,
		LogLayers:            logLayers,
		ProbePort:            probePort,
		ImagePullSecrets:     imagePullSecrets,
		Labels:               labels,
		ControllingCRDetails: controllingCRDetails,
		EnableForceDetach:    enableForceDetach,
		Version:              i.client.ServerVersion(),
		HTTPRequestTimeout:   httpTimeout,
		NodeSelector:         nodePluginNodeSelector,
		Tolerations:          tolerations,
		ServiceAccountName:   serviceAccountName,
		ImagePullPolicy:      imagePullPolicy,
	}

	var newDaemonSetYAML string
	if isWindows {
		newDaemonSetYAML = k8sclient.GetCSIDaemonSetYAMLWindows(daemonSetArgs)
		daemonSetArgs.ServiceAccountName = getNodeRBACResourceName(true)
	} else {
		newDaemonSetYAML = k8sclient.GetCSIDaemonSetYAMLLinux(daemonSetArgs)
		daemonSetArgs.ServiceAccountName = getNodeRBACResourceName(false)
	}

	err = i.client.PutDaemonSet(currentDaemonSet, createDaemonSet, newDaemonSetYAML, nodeLabel,
		getDaemonSetName(isWindows))
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident daemonset; %v", err)
	}

	return nil
}

// TridentDaemonSetInformation identifies the Operator based Trident CSI daemonset and unwanted daemonsets,
// this method can be used for multiple purposes at different point during the Reconcile so it makes sense to
// keep it separate from the createOrPatchTridentDaemonSet
func (i *Installer) TridentDaemonSetInformation() (*appsv1.DaemonSet,
	[]appsv1.DaemonSet, bool, error,
) {
	nodeLabel := TridentNodeLabel
	return i.client.GetDaemonSetInformation(nodeLabel, i.namespace, false)
}

func (i *Installer) waitForTridentPod() (*v1.Pod, error) {
	var pod *v1.Pod

	// Add sleep to make sure we get the pod name, esp. in case where we kill one deployment and the
	// create a new one.
	waitTime := 7 * time.Second
	Log().Debugf("Waiting for %v after the patch to make sure we get the right trident-pod name.", waitTime)
	time.Sleep(waitTime)

	checkPodRunning := func() error {
		var podError error
		pod, podError = i.client.GetPodByLabel(appLabel, false)
		if podError != nil || pod.Status.Phase != v1.PodRunning {

			// Try to identify the reason for container not running, it could be a
			// temporary error due to latency in pulling the image or a more
			// permanent error like ImagePullBackOff.
			tempError := true
			containerErrors := make(map[string]string)
			if pod != nil {
				for _, containerStatus := range pod.Status.ContainerStatuses {
					// If there exists a container still in creating state verify that
					// the reason for waiting is "ContainerCreating" and nothing else
					// like ImagePullBackOff
					if containerStatus.State.Waiting != nil {
						if containerStatus.State.Waiting.Reason != "ContainerCreating" {
							tempError = false
							containerErrors[containerStatus.Name] = " Reason: " + containerStatus.State.Waiting.
								Reason + ", " +
								"Message: " + containerStatus.State.Waiting.Message
						}
					}
				}
			}

			if tempError {
				Log().Debug("Containers are still in creating state.")
				return errors.TempOperatorError(fmt.Errorf(
					"pod provisioning in progress; containers are still in creating state"))
			}

			Log().WithField("err", containerErrors).Errorf("Encountered error while creating container(s).")
			return fmt.Errorf("unable to provision pod; encountered error while creating container(s): %v",
				containerErrors)
		}

		// If DeletionTimestamp is set this pod is in a terminating state
		// and may be related to a terminating deployment.
		if pod.DeletionTimestamp != nil {
			Log().Debug("Unable to find Trident pod; found a pod in terminating state.")
			return errors.TempOperatorError(fmt.Errorf("unable to find Trident pod; found a pod in terminating state"))
		}

		return nil
	}
	podNotify := func(err error, duration time.Duration) {
		Log().WithFields(LogFields{
			"increment": duration,
		}).Debugf("Trident pod not yet running, waiting.")
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.MaxElapsedTime = k8sTimeout

	Log().Info("Waiting for Trident pod to start.")

	if err := backoff.RetryNotify(checkPodRunning, podBackoff, podNotify); err != nil {
		totalWaitTime := k8sTimeout
		// In case pod is still creating and taking extra time due to issues such as latency in pulling
		// container image, then additional time should be allocated for the pod to come online.
		if errors.IsTempOperatorError(err) {
			extraWaitTime := 150 * time.Second
			totalWaitTime = totalWaitTime + extraWaitTime
			podBackoff.MaxElapsedTime = extraWaitTime

			Log().Debugf("Pod is still provisioning after %3.2f seconds, "+
				"waiting %3.2f seconds extra.", k8sTimeout.Seconds(), extraWaitTime.Seconds())
			err = backoff.RetryNotify(checkPodRunning, podBackoff, podNotify)
		}

		if err != nil {
			// Build up an error message with as much detail as available.
			var errMessages []string
			errMessages = append(errMessages,
				fmt.Sprintf("Trident pod was not running after %3.2f seconds; err: %v.", totalWaitTime.Seconds(), err))

			if pod != nil {
				if pod.Status.Phase != "" {
					errMessages = append(errMessages, fmt.Sprintf("Pod status is %s.", pod.Status.Phase))
					if pod.Status.Message != "" {
						errMessages = append(errMessages, pod.Status.Message)
					}
				}
				errMessages = append(errMessages,
					fmt.Sprintf("Use '%s describe pod %s -n %s' for more information.",
						i.client.CLI(), pod.Name, i.client.Namespace()))
			}

			Log().Error(strings.Join(errMessages, " "))
			return nil, err
		}
	}

	Log().WithFields(LogFields{
		"pod":       pod.Name,
		"namespace": i.namespace,
	}).Info("Trident pod started.")

	return pod, nil
}

func (i *Installer) waitForRESTInterface(tridentPodName string) error {
	var version, versionWithMetadata, controllerServer string
	if useIPv6 {
		controllerServer = "[::1]:8000"
	} else {
		controllerServer = "127.0.0.1:8000"
	}

	checkRESTInterface := func() error {
		cliCommand := []string{"tridentctl", "-s", controllerServer, "version", "-o", "json"}
		versionJSON, err := i.client.Exec(tridentPodName, TridentContainer, cliCommand)
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

		versionWithMetadata = versionResponse.Server.Version
		return nil
	}
	restNotify := func(err error, duration time.Duration) {
		Log().WithFields(LogFields{
			"increment": duration,
		}).Debugf("REST interface not yet up, waiting.")
	}
	restBackoff := backoff.NewExponentialBackOff()
	restBackoff.MaxElapsedTime = k8sTimeout

	Log().Info("Waiting for Trident REST interface.")

	if err := backoff.RetryNotify(checkRESTInterface, restBackoff, restNotify); err != nil {
		totalWaitTime := k8sTimeout
		// In case of HTTP 503 error wait for extra 180 seconds, if the backends takes additional time to respond
		// this will ensure we do not error out early.
		if strings.Contains(err.Error(), "503 Service Unavailable") {
			extraWaitTime := 180 * time.Second
			totalWaitTime = totalWaitTime + extraWaitTime
			restBackoff.MaxElapsedTime = extraWaitTime

			Log().Debugf("Encountered HTTP 503 error, REST interface is not up after 30 seconds, "+
				"waiting %3.2f seconds extra.", extraWaitTime.Seconds())
			err = backoff.RetryNotify(checkRESTInterface, restBackoff, restNotify)
		}

		if err != nil {
			Log().Errorf("Trident REST interface was not available after %3.2f seconds; err: %v", totalWaitTime.Seconds(),
				err)
			return err
		}
	}

	versionInfo, err := versionutils.ParseDate(versionWithMetadata)
	if err != nil {
		Log().WithField("version", versionWithMetadata).Errorf("unable to parse version with metadata")
		version = versionWithMetadata
	} else {
		version = versionInfo.ShortStringWithRelease()
	}

	Log().WithField("version", version).Info("Trident REST interface is up.")

	return nil
}

// getTridentClientVersionInfo takes trident image name and identifies the Trident client version
func (i *Installer) getTridentClientVersionInfo(
	imageName string, controllingCRDetails map[string]string,
) (*api.ClientVersionResponse, error) {
	clientVersionYAML, err := i.getTridentVersionYAML(imageName, controllingCRDetails)
	if err != nil {
		return nil, err
	}

	Log().WithField("image", imageName).Debugf("Successfully retrieved version information in YAML format: \n%s",
		string(clientVersionYAML))
	clientVersion := api.ClientVersionResponse{}
	if err := yaml.Unmarshal(clientVersionYAML, &clientVersion); err != nil {
		errMessage := fmt.Sprintf("unable to umarshall client version YAML to Version struct; err: %v", err)
		Log().WithField("image", imageName).Errorf(errMessage)

		return nil, fmt.Errorf(errMessage)
	}

	Log().WithField("version", clientVersion).Debugf("Successfully found Trident image version information.")

	return &clientVersion, nil
}

// getTridentVersionYAML takes trident image name and identifies the Trident client version YAML, this workflow
// resembles the `kubectl run --rm -it --restart=Never transient-trident-version-pod --image=<image_name> --
// /bin/tridentctl version --client -o yaml` command
func (i *Installer) getTridentVersionYAML(imageName string, controllingCRDetails map[string]string) ([]byte, error) {
	podName := "transient-trident-version-pod"
	podLabels := make(map[string]string)

	podLabels[TridentVersionPodLabelKey] = TridentVersionPodLabelValue
	podLabels[K8sVersionLabelKey] = i.client.ServerVersion().ShortStringWithRelease()

	tridentctlFilePath := "/bin/tridentctl"
	tridentVersionCommand := []string{tridentctlFilePath, "version", "--client", "-o", "yaml"}

	// Create TridentVersion Pod from image
	returnError := i.createTridentVersionPod(podName, imageName, controllingCRDetails, podLabels)
	if returnError != nil {
		return []byte{}, fmt.Errorf("failed to create Trident Version pod from the image provided; %v", returnError)
	}

	// Wait for Trident version pod to provide information
	timeout := 30 * time.Second
	output, err := i.client.ExecPodForVersionInformation(podName, tridentVersionCommand, timeout)
	if err != nil {
		errMessage := fmt.Sprintf("failed to exec Trident version pod '%s' (image: '%s') for the information; err: %v",
			podName, imageName, err)
		Log().Error(errMessage)

		if err := i.client.DeletePodByLabel(TridentVersionPodLabel); err != nil {
			Log().WithFields(LogFields{
				"image": imageName,
				"pod":   podName,
				"err":   err,
			}).Errorf("Could not delete Trident version pod.")
		}
		return []byte{}, fmt.Errorf(errMessage)
	}

	outputString := string(output)
	messageToDelete := fmt.Sprintf("\npod \"%s\" deleted", podName)
	outputString = strings.Replace(outputString, messageToDelete, "", 1)
	clientVersionYAML := []byte(outputString)

	if err := i.client.DeletePodByLabel(TridentVersionPodLabel); err != nil {
		Log().WithFields(LogFields{
			"image": imageName,
			"pod":   podName,
			"err":   err,
		}).Errorf("Could not delete Trident version pod.")
	}

	Log().WithFields(LogFields{
		"image": imageName,
	}).Debug("Found Trident version yaml.")

	return clientVersionYAML, nil
}

// createTridentVersionPod takes the pod name and trident image name to create a pod
func (i *Installer) createTridentVersionPod(
	podName, imageName string, controllingCRDetails, podLabels map[string]string,
) error {
	versionPodLabel := TridentVersionPodLabel
	err := i.client.DeleteTransientVersionPod(versionPodLabel)
	if err != nil {
		return fmt.Errorf("failed to delete previous transient version pod; %v", err)
	}

	// Using node service account which requires minimal permissions, to be used for
	// spinning up this transient pod.
	serviceAccountName := getNodeRBACResourceName(false)

	newTridentVersionPodYAML := k8sclient.GetTridentVersionPodYAML(
		podName, imageName, serviceAccountName, imagePullPolicy, imagePullSecrets, podLabels, controllingCRDetails,
	)

	err = i.client.CreateObjectByYAML(newTridentVersionPodYAML)
	if err != nil {
		err = fmt.Errorf("failed to create Trident version pod; %v", err)
		return err
	}
	Log().WithFields(LogFields{
		"pod":       podName,
		"namespace": i.namespace,
	}).Info("Created Trident version pod.")

	return nil
}

// getAppLabelForResource returns the right app labels for RBAC resource name passed
func getAppLabelForResource(resourceName string) (map[string]string, string) {
	var label string
	labelMap := make(map[string]string, 1)
	if strings.Contains(resourceName, "node") {
		labelMap[TridentAppLabelKey] = TridentNodeLabelValue
		label = TridentNodeLabel
	} else {
		labelMap[TridentAppLabelKey] = TridentCSILabelValue
		label = TridentCSILabel
	}
	return labelMap, label
}

func (i *Installer) isPSPSupported() bool {
	pspRemovedVersion := versionutils.MustParseMajorMinorVersion(commonconfig.PodSecurityPoliciesRemovedKubernetesVersion)
	return i.client.ServerVersion().LessThan(pspRemovedVersion)
}

func isLinuxNodeSCCUser(user string) bool {
	return user == TridentNodeLinuxResourceName
}

func determineLogLevel() string {
	if debug {
		return "debug"
	}

	if logLevel == "" {
		return "info"
	}

	return logLevel
}
