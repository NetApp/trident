// Copyright 2021 NetApp, Inc. All Rights Reserved.

package installer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	netappv1 "github.com/netapp/trident/operator/controllers/orchestrator/apis/netapp/v1"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
)

const (
	// CRD names
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

	VolumeSnapshotCRDName        = "volumesnapshots.snapshot.storage.k8s.io"
	VolumeSnapshotClassCRDName   = "volumesnapshotclasses.snapshot.storage.k8s.io"
	VolumeSnapshotContentCRDName = "volumesnapshotcontents.snapshot.storage.k8s.io"

	DefaultTimeout = 30
)

var (
	// CR inputs
	csi                bool
	debug              bool
	useIPv6            bool
	silenceAutosupport bool
	enableNodePrep     bool

	logFormat     string
	probePort     string
	tridentImage  string
	imageRegistry string
	kubeletDir    string

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
		VolumePublicationCRDName,
	}

	AlphaCRDNames = []string{
		VolumeSnapshotCRDName,
		VolumeSnapshotClassCRDName,
		VolumeSnapshotContentCRDName,
	}

	// TODO (cknight): remove when 1.18 is our minimum version
	KubernetesVersionMinV1CSIDriver = utils.MustParseSemantic("1.18.0-0")
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

	log.WithField("namespace", namespace).Debugf("Initialized installer.")

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
		log.Debugf("No Trident deployment exists.")
		performImageVersionCheck = true
	} else {
		log.Debugf("Trident deployment exists.")

		containers := currentDeployment.Spec.Template.Spec.Containers

		for _, container := range containers {
			if container.Name == "trident-main" {
				if container.Image != tridentImage {
					log.WithFields(log.Fields{
						"currentTridentImage": container.Image,
						"newTridentImage":     tridentImage,
					}).Debugf("Current Trident installation image is not same as the new Trident Image.")
					performImageVersionCheck = true
				}
			}
		}

		// Contingency plan to recover Trident version information
		if currentDeployment.Labels[TridentVersionLabelKey] == "" {
			log.Errorf("Deployment is missing the version information; fixing it.")
			performImageVersionCheck = true
		} else {
			identifiedImageVersion = currentDeployment.Labels[TridentVersionLabelKey]

			// Check if deployed Trident image matches the version this Operator supports
			if supportedVersion, err := utils.ParseDate(DefaultTridentVersion); err != nil {
				log.WithField("version", DefaultTridentVersion).Error("Could not parse default version.")
				performImageVersionCheck = true
			} else if deploymentVersion, err := utils.ParseDate(identifiedImageVersion); err != nil {
				log.WithField("version", identifiedImageVersion).Error("Could not parse deployment version.")
				performImageVersionCheck = true
			} else if supportedVersion.ShortString() != deploymentVersion.ShortString() {
				log.Debugf("Current Trident deployment image '%s' is not same as the supported Trident image '%s'.",
					deploymentVersion.String(), supportedVersion.String())
				performImageVersionCheck = true
			}
		}
	}

	if performImageVersionCheck {
		log.Debugf("Image version check needed.")

		// Create the RBAC objects for the trident version pod
		if _, err = i.createRBACObjects(controllingCRDetails, labels, false); err != nil {
			return "", fmt.Errorf("unable to create RBAC objects while verifying Trident version; err: %v", err)
		}

		tridentClientVersion, err := i.getTridentClientVersionInfo(tridentImage, controllingCRDetails)
		if err != nil {
			return "", fmt.Errorf("unable to get Trident image version information, please ensure correct "+
				"Trident image has been provided; err: %v", err)
		}

		tridentImageVersion, err := utils.ParseDate(tridentClientVersion.Client.Version)
		if err != nil {
			errMessage := fmt.Sprintf("unexpected parse error during Trident client version retrieval; err: %v", err)
			log.Errorf(errMessage)
			return "", fmt.Errorf(errMessage)
		}

		supportedTridentVersion, err := utils.ParseDate(DefaultTridentVersion)
		if err != nil {
			errMessage := fmt.Sprintf("unexpected parse error during supported Trident version; err: %v", err)
			log.Errorf(errMessage)
			return "", fmt.Errorf(errMessage)
		}

		tridentImageShortVersion := tridentImageVersion.ShortString()
		supportedTridentShortVersion := supportedTridentVersion.ShortString()

		if tridentImageShortVersion != supportedTridentShortVersion {
			errMessage := fmt.Sprintf("unsupported Trident image version '%s', supported Trident version is '%s'",
				tridentImageVersion.ShortStringWithRelease(), supportedTridentVersion.ShortStringWithRelease())
			log.Errorf(errMessage)
			return "", fmt.Errorf(errMessage)
		}

		// need to append 'v', so that it can be stores in trident version label later
		identifiedImageVersion = "v" + tridentImageVersion.ShortStringWithRelease()
		log.WithFields(log.Fields{
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

	imagePullSecrets = []string{}

	// Get values from CR
	csi = true
	debug = cr.Spec.Debug
	useIPv6 = cr.Spec.IPv6
	enableNodePrep = cr.Spec.EnableNodePrep
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

		log.WithFields(log.Fields{
			"tridentImage": tridentImage,
			"version":      identifiedImageVersion,
		}).Debugf("Identified Trident image's version.")
	} else {
		identifiedImageVersion = TridentVersionLabelValue

		log.WithFields(log.Fields{
			"tridentImage": tridentImage,
			"version":      identifiedImageVersion,
		}).Debugf("Using default Trident image.")
	}

	// Identify if this is an update scenario, i.e. Trident version has changed
	if currentInstallationVersion != identifiedImageVersion {
		log.WithFields(log.Fields{
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

	// Update the label with the correct version
	labels[TridentVersionLabelKey] = identifiedImageVersion

	return controllingCRDetails, labels, tridentUpdateNeeded, nil
}

func (i *Installer) InstallOrPatchTrident(
	cr netappv1.TridentOrchestrator, currentInstallationVersion string, k8sUpdateNeeded bool,
) (*netappv1.TridentOrchestratorSpecValues, string, error) {

	var returnError error
	var newServiceAccount bool

	// Set installation params
	controllingCRDetails, labels, tridentUpdateNeeded, err := i.setInstallationParams(cr, currentInstallationVersion)
	if err != nil {
		return nil, "", err
	}

	// Remove any leftover transient Trident
	versionPodLabel := TridentVersionPodLabel
	err = i.client.DeleteTransientVersionPod(versionPodLabel)
	if err != nil {
		log.WithError(err).Warn("Could not remove transient Trident pods.")
	}

	// Identify if update is required because of change in K8s version or Trident Operator version
	shouldUpdate := k8sUpdateNeeded || tridentUpdateNeeded

	// Begin Trident installation logic...

	// All checks succeeded, so proceed with installation
	log.WithField("namespace", i.namespace).Info("Starting Trident installation.")

	// Create namespace, if one does not exist
	if returnError = i.createTridentInstallationNamespace(); returnError != nil {
		return nil, "", returnError
	}

	// Create or patch or update the RBAC objects
	if newServiceAccount, returnError = i.createRBACObjects(controllingCRDetails, labels,
		shouldUpdate); returnError != nil {
		return nil, "", returnError
	}

	// Create CRDs and ensure they are established
	returnError = i.createAndEnsureCRDs()
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident CRDs; %v", returnError)
		return nil, "", returnError
	}

	// Patch the CRD definitions with finalizers to protect them
	if returnError = i.client.AddFinalizerToCRDs(CRDnames); returnError != nil {
		return nil, "", fmt.Errorf("failed to add finalizers for Trident CRDs %v; err: %v", CRDnames, returnError)
	}

	// Create or patch or update the RBAC PSPs
	returnError = i.createOrPatchTridentPodSecurityPolicy(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident pod security policy; %v", returnError)
		return nil, "", returnError
	}

	// Create or update the CSI Driver object if necessary
	if i.client.ServerVersion().AtLeast(KubernetesVersionMinV1CSIDriver) {
		returnError = i.createOrPatchK8sCSIDriver(controllingCRDetails, labels, shouldUpdate)
	} else {
		returnError = i.createOrPatchK8sBetaCSIDriver(controllingCRDetails, labels, shouldUpdate)
	}
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

	// Create or update the Trident CSI deployment
	returnError = i.createOrPatchTridentDeployment(controllingCRDetails, labels, shouldUpdate, newServiceAccount)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident Deployment; %v", returnError)
		return nil, "", returnError
	}

	// Create or update the Trident CSI daemonset
	returnError = i.createOrPatchTridentDaemonSet(controllingCRDetails, labels, shouldUpdate, newServiceAccount)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident DaemonSet; %v", returnError)
		return nil, "", returnError
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
		Debug:                   strconv.FormatBool(debug),
		LogFormat:               logFormat,
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
		EnableNodePrep:          strconv.FormatBool(enableNodePrep),
		NodePluginNodeSelector:  nodePluginNodeSelector,
		NodePluginTolerations:   nodePluginTolerations,
	}

	log.WithFields(log.Fields{
		"namespace": i.namespace,
		"version":   labels[TridentVersionLabelKey],
	}).Info("Trident is installed.")
	return &identifiedSpecValues, labels[TridentVersionLabelKey], nil
}

// createCRDs creates and establishes each of the CRDs individually
func (i *Installer) createCRDs() error {
	var err error

	if err = i.CreateCRD(VersionCRDName, k8sclient.GetVersionCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(BackendCRDName, k8sclient.GetBackendCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(MirrorRelationshipCRDName, k8sclient.GetMirrorRelationshipCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(SnapshotInfoCRDName, k8sclient.GetSnapshotInfoCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(BackendConfigCRDName, k8sclient.GetBackendConfigCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(StorageClassCRDName, k8sclient.GetStorageClassCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(VolumeCRDName, k8sclient.GetVolumeCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(VolumePublicationCRDName, k8sclient.GetVolumePublicationCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(NodeCRDName, k8sclient.GetNodeCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(TransactionCRDName, k8sclient.GetTransactionCRDYAML()); err != nil {
		return err
	}
	if err = i.CreateCRD(SnapshotCRDName, k8sclient.GetSnapshotCRDYAML()); err != nil {
		return err
	}

	return err
}

// CreateCRD creates and establishes the CRD
func (i *Installer) CreateCRD(crdName, crdYAML string) error {

	// Discover CRD data
	crdExist, returnError := i.client.CheckCRDExists(crdName)
	if returnError != nil {
		return fmt.Errorf("unable to identify if %v CRD exists; err: %v", crdName, returnError)
	}

	if crdExist {
		log.WithField("CRD", crdName).Infof("CRD present.")
	} else {
		// Create the CRDs and wait for them to be registered in Kubernetes
		log.WithField("CRD", crdName).Infof("Installer will create a fresh CRD.")

		if returnError = i.client.CreateCustomResourceDefinition(crdName, crdYAML); returnError != nil {
			return returnError
		}

		// Wait for the CRD to be fully established
		if returnError = i.client.WaitForCRDEstablished(crdName, k8sTimeout); returnError != nil {
			// If CRD registration failed *and* we created the CRD, clean up by deleting the CRD
			log.WithFields(log.Fields{
				"CRD": crdName,
				"err": returnError,
			}).Errorf("CRD not established.")

			if err := i.client.DeleteCustomResourceDefinition(crdName, crdYAML); err != nil {
				log.WithFields(log.Fields{
					"CRD": crdName,
					"err": err,
				}).Errorf("Could not delete CRD.")
			}
			return returnError
		}
	}

	return returnError
}

// ensureCRDEstablished waits until a CRD is Established.
func (i *Installer) ensureCRDEstablished(crdName string) error {

	checkCRDEstablished := func() error {
		crd, err := i.client.GetCRD(crdName)
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

func (i *Installer) createOrPatchK8sBetaCSIDriver(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {

	// TODO (cknight): remove when 1.18 is our minimum version

	CSIDriverName := getCSIDriverName()

	currentCSIDriver, unwantedCSIDrivers, createCSIDriver, err := i.client.GetBetaCSIDriverInformation(
		CSIDriverName, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get K8s (beta) CSI drivers; %v", err)
	}

	if err = i.client.RemoveMultipleBetaCSIDriverCRs(unwantedCSIDrivers); err != nil {
		return fmt.Errorf("failed to remove unwanted K8s (beta) CSI drivers; %v", err)
	}

	newK8sCSIDriverYAML := k8sclient.GetCSIDriverYAML(CSIDriverName, i.client.ServerVersion(), labels,
		controllingCRDetails)

	err = i.client.PutBetaCSIDriver(currentCSIDriver, createCSIDriver, newK8sCSIDriverYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch K8s (beta) CSI drivers; %v", err)
	}
	return nil
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

	newK8sCSIDriverYAML := k8sclient.GetCSIDriverYAML(CSIDriverName, i.client.ServerVersion(), labels,
		controllingCRDetails)

	err = i.client.PutCSIDriver(currentCSIDriver, createCSIDriver, newK8sCSIDriverYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch K8s CSI drivers; %v", err)
	}

	return nil
}

func (i *Installer) createRBACObjects(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) (newServiceAccount bool, returnError error) {

	// Create service account
	newServiceAccount, returnError = i.createOrPatchTridentServiceAccount(controllingCRDetails, labels, false)
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

	// Create cluster role binding
	returnError = i.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident cluster role binding; %v", returnError)
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

func (i *Installer) createTridentInstallationNamespace() error {
	createNamespace := true

	namespaceExists, returnError := i.client.CheckNamespaceExists(i.namespace)
	if returnError != nil {
		log.WithFields(log.Fields{
			"namespace": i.namespace,
			"err":       returnError,
		}).Errorf("Unable to check if namespace exists.")
		return fmt.Errorf("unable to check if namespace %s exists; %v", i.namespace, returnError)
	} else if namespaceExists {
		log.WithField("namespace", i.namespace).Debug("Namespace exists.")
		createNamespace = false
	} else {
		log.WithField("namespace", i.namespace).Debug("Namespace does not exist.")
	}

	if createNamespace {
		// Create namespace
		err := i.client.CreateObjectByYAML(k8sclient.GetNamespaceYAML(i.namespace))
		if err != nil {
			return fmt.Errorf("failed to create Trident installation namespace %s; %v", i.namespace, err)
		}
		log.WithField("namespace", i.namespace).Info("Created Trident installation namespace.")
	}

	return nil
}

func (i *Installer) createOrPatchTridentServiceAccount(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) (bool, error) {
	newServiceAccount := false
	serviceAccountName := getServiceAccountName(true)

	currentServiceAccount, unwantedServiceAccounts, serviceAccountSecretNames, createServiceAccount, err := i.client.GetServiceAccountInformation(serviceAccountName,
		appLabel, i.namespace, shouldUpdate)
	if err != nil {
		return newServiceAccount, fmt.Errorf("failed to get Trident service accounts; %v", err)
	}

	if err = i.client.RemoveMultipleServiceAccounts(unwantedServiceAccounts); err != nil {
		return newServiceAccount, fmt.Errorf("failed to remove unwanted service accounts; %v", err)
	}

	newServiceAccountYAML := k8sclient.GetServiceAccountYAML(serviceAccountName, serviceAccountSecretNames, labels,
		controllingCRDetails)

	if newServiceAccount, err = i.client.PutServiceAccount(currentServiceAccount, createServiceAccount,
		newServiceAccountYAML, appLabel); err != nil {
		return newServiceAccount, fmt.Errorf("failed to create or patch Trident service accounts; %v", err)
	}

	return newServiceAccount, nil
}

func (i *Installer) createOrPatchTridentClusterRole(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {

	clusterRoleName := getClusterRoleName(true)

	currentClusterRole, unwantedClusterRoles, createClusterRole, err := i.client.GetClusterRoleInformation(
		clusterRoleName, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident cluster roles; %v", err)
	}

	if err = i.client.RemoveMultipleClusterRoles(unwantedClusterRoles); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident cluster roles; %v", err)
	}

	k8sFlavor := i.client.Flavor()
	newClusterRoleYAML := k8sclient.GetClusterRoleYAML(k8sFlavor, clusterRoleName, labels, controllingCRDetails,
		true)

	err = i.client.PutClusterRole(currentClusterRole, createClusterRole, newClusterRoleYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident cluster role; %v", err)
	}

	return nil
}

func (i *Installer) createOrPatchTridentClusterRoleBinding(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {

	clusterRoleBindingName := getClusterRoleBindingName(true)

	currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding,
		err := i.client.GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident cluster role bindings; %v", err)
	}

	if err = i.client.RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident cluster role bindings; %v", err)
	}

	newClusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(i.namespace, i.client.Flavor(),
		clusterRoleBindingName,
		labels, controllingCRDetails, true)

	err = i.client.PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding,
		newClusterRoleBindingYAML, appLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident cluster role binding; %v", err)
	}

	return nil
}

func (i *Installer) createOrPatchTridentOpenShiftSCC(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {
	var currentOpenShiftSCCJSON []byte

	openShiftSCCUserName := getOpenShiftSCCUserName()
	openShiftSCCName := getOpenShiftSCCName()

	// duplicated in GetTridentOpenShiftSCCInformation because it's valuable in both places
	logFields := log.Fields{
		"sccUserName": openShiftSCCUserName,
		"sccName":     openShiftSCCName,
	}

	currentOpenShiftSCCJSON, createOpenShiftSCC, removeExistingSCC, err := i.client.GetTridentOpenShiftSCCInformation(
		openShiftSCCName, openShiftSCCUserName, shouldUpdate)
	if err != nil {
		log.WithFields(logFields).Errorf("Unable to get OpenShift SCC for Trident; err: %v", err)
		return fmt.Errorf("failed to get Trident OpenShift SCC; %v", err)
	}

	// keep this logic here
	if removeExistingSCC {
		if err = i.client.DeleteObjectByYAML(k8sclient.GetOpenShiftSCCQueryYAML(openShiftSCCName), true); err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Errorf("Unable to delete OpenShift SCC.")
			return fmt.Errorf("unable to delete OpenShift SCC; %v", err)
		}
	}

	newOpenShiftSCCYAML := k8sclient.GetOpenShiftSCCYAML(openShiftSCCName, openShiftSCCUserName, i.namespace,
		labels, controllingCRDetails)

	err = i.client.PutOpenShiftSCC(currentOpenShiftSCCJSON, createOpenShiftSCC, newOpenShiftSCCYAML)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident OpenShift SCC; %v", err)
	}

	return nil
}

func (i *Installer) createAndEnsureCRDs() (returnError error) {

	returnError = i.createCRDs()
	if returnError != nil {
		returnError = fmt.Errorf("failed to create the Trident CRDs; %v", returnError)
	}
	return
}

func (i *Installer) createOrPatchTridentPodSecurityPolicy(
	controllingCRDetails, labels map[string]string, shouldUpdate bool,
) error {

	pspName := getPSPName()

	currentPSP, unwantedPSPs, createPSP, err := i.client.GetPodSecurityPolicyInformation(pspName, appLabel,
		shouldUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Trident pod security policies; %v", err)
	}

	if err = i.client.RemoveMultiplePodSecurityPolicies(unwantedPSPs); err != nil {
		return fmt.Errorf("failed to remove unwanted Trident pod security policies; %v", err)
	}

	newPSPYAML := k8sclient.GetPrivilegedPodSecurityPolicyYAML(pspName, labels, controllingCRDetails)

	if err = i.client.PutPodSecurityPolicy(currentPSP, createPSP, newPSPYAML, appLabel); err != nil {
		return fmt.Errorf("failed to create or patch Trident pod security policy; %v", err)
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
		certInfo, err := utils.MakeHTTPCertInfo(commonconfig.CACertName, commonconfig.ServerCertName,
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
		aesKey, err := utils.GenerateAESKey()
		if err != nil {
			return fmt.Errorf("failed to generate secure AES key; %v", err)
		}

		// Create the secret map
		secretMap := make(map[string]string)
		secretMap = map[string]string{
			commonconfig.AESKeyFile:     aesKey,
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

func (i *Installer) createOrPatchTridentDeployment(
	controllingCRDetails, labels map[string]string, shouldUpdate, newServiceAccount bool,
) error {

	topologyEnabled, err := i.client.IsTopologyInUse()
	if err != nil {
		return fmt.Errorf("failed to determine if node topology settings; %v", err)
	}

	deploymentName := getDeploymentName(true)

	currentDeployment, unwantedDeployments, createDeployment, err := i.client.GetDeploymentInformation(deploymentName,
		appLabel, i.namespace)
	if err != nil {
		return fmt.Errorf("failed to get Trident deployment information; %v", err)
	}

	// Create a new deployment if there is a current deployment and
	// a new service account or it should be updated
	if currentDeployment != nil && (shouldUpdate || newServiceAccount) {
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

	snapshotCRDVersion := i.client.GetCSISnapshotterVersion(currentDeployment)

	deploymentArgs := &k8sclient.DeploymentYAMLArguments{
		DeploymentName:          deploymentName,
		TridentImage:            tridentImage,
		AutosupportImage:        autosupportImage,
		AutosupportProxy:        autosupportProxy,
		AutosupportCustomURL:    "",
		AutosupportSerialNumber: autosupportSerialNumber,
		AutosupportHostname:     autosupportHostname,
		ImageRegistry:           imageRegistry,
		LogFormat:               logFormat,
		SnapshotCRDVersion:      snapshotCRDVersion,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    controllingCRDetails,
		Debug:                   debug,
		UseIPv6:                 useIPv6,
		SilenceAutosupport:      silenceAutosupport,
		Version:                 i.client.ServerVersion(),
		TopologyEnabled:         topologyEnabled,
		HTTPRequestTimeout:      httpTimeout,
		NodeSelector:            controllerPluginNodeSelector,
		Tolerations:             tolerations,
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

	return i.client.GetDeploymentInformation(getDeploymentName(true), deploymentLabel, i.namespace)
}

func (i *Installer) createOrPatchTridentDaemonSet(
	controllingCRDetails, labels map[string]string, shouldUpdate, newServiceAccount bool,
) error {

	daemonSetName := getDaemonSetName()
	nodeLabel := TridentNodeLabel

	currentDaemonSet, unwantedDaemonSets, createDaemonSet, err := i.client.GetDaemonSetInformation(daemonSetName,
		nodeLabel, i.namespace)
	if err != nil {
		return fmt.Errorf("failed to get Trident daemonsets; %v", err)
	}

	// Create a new daemonset if there is a current daemonset and
	// a new service account or it should be updated
	if currentDaemonSet != nil && (shouldUpdate || newServiceAccount) {
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
		DaemonsetName:        daemonSetName,
		TridentImage:         tridentImage,
		ImageRegistry:        imageRegistry,
		KubeletDir:           kubeletDir,
		LogFormat:            logFormat,
		ProbePort:            probePort,
		ImagePullSecrets:     imagePullSecrets,
		Labels:               labels,
		ControllingCRDetails: controllingCRDetails,
		Debug:                debug,
		NodePrep:             enableNodePrep,
		Version:              i.client.ServerVersion(),
		HTTPRequestTimeout:   httpTimeout,
		NodeSelector:         nodePluginNodeSelector,
		Tolerations:          tolerations,
	}
	newDaemonSetYAML := k8sclient.GetCSIDaemonSetYAML(daemonSetArgs)

	err = i.client.PutDaemonSet(currentDaemonSet, createDaemonSet, newDaemonSetYAML, nodeLabel)
	if err != nil {
		return fmt.Errorf("failed to create or patch Trident daemonset; %v", err)
	}

	return nil
}

// TridentDaemonSetInformation identifies the Operator based Trident CSI daemonset and unwanted daemonsets,
// this method can be used for multiple purposes at different point during the Reconcile so it makes sense to
// keep it separate from the createOrPatchTridentDaemonSet
func (i *Installer) TridentDaemonSetInformation() (*appsv1.DaemonSet,
	[]appsv1.DaemonSet, bool, error) {

	daemonSetName := getDaemonSetName()
	nodeLabel := TridentNodeLabel
	return i.client.GetDaemonSetInformation(daemonSetName, nodeLabel, i.namespace)
}

func (i *Installer) waitForTridentPod() (*v1.Pod, error) {

	var pod *v1.Pod

	// Add sleep to make sure we get the pod name, esp. in case where we kill one deployment and the
	// create a new one.
	waitTime := 7 * time.Second
	log.Debugf("Waiting for %v after the patch to make sure we get the right trident-pod name.", waitTime)
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
				log.Debug("Containers are still in creating state.")
				return utils.TempOperatorError(fmt.Errorf(
					"pod provisioning in progress; containers are still in creating state"))
			}

			log.WithField("err", containerErrors).Errorf("Encountered error while creating container(s).")
			return fmt.Errorf("unable to provision pod; encountered error while creating container(s): %v",
				containerErrors)
		}

		// If DeletionTimestamp is set this pod is in a terminating state
		// and may be related to a terminating deployment.
		if pod.DeletionTimestamp != nil {
			log.Debug("Unable to find Trident pod; found a pod in terminating state.")
			return utils.TempOperatorError(fmt.Errorf("unable to find Trident pod; found a pod in terminating state"))
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
		totalWaitTime := k8sTimeout
		// In case pod is still creating and taking extra time due to issues such as latency in pulling
		// container image, then additional time should be allocated for the pod to come online.
		if utils.IsTempOperatorError(err) {
			extraWaitTime := 150 * time.Second
			totalWaitTime = totalWaitTime + extraWaitTime
			podBackoff.MaxElapsedTime = extraWaitTime

			log.Debugf("Pod is still provisioning after %3.2f seconds, "+
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

			log.Error(strings.Join(errMessages, " "))
			return nil, err
		}
	}

	log.WithFields(log.Fields{
		"pod":       pod.Name,
		"namespace": i.namespace,
	}).Info("Trident pod started.")

	return pod, nil
}

func (i *Installer) waitForRESTInterface(tridentPodName string) error {

	var version, versionWithMetadata string

	checkRESTInterface := func() error {

		cliCommand := []string{"tridentctl", "-s", ControllerServer, "version", "-o", "json"}
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
		log.WithFields(log.Fields{
			"increment": duration,
		}).Debugf("REST interface not yet up, waiting.")
	}
	restBackoff := backoff.NewExponentialBackOff()
	restBackoff.MaxElapsedTime = k8sTimeout

	log.Info("Waiting for Trident REST interface.")

	if err := backoff.RetryNotify(checkRESTInterface, restBackoff, restNotify); err != nil {
		totalWaitTime := k8sTimeout
		// In case of HTTP 503 error wait for extra 180 seconds, if the backends takes additional time to respond
		// this will ensure we do not error out early.
		if strings.Contains(err.Error(), "503 Service Unavailable") {
			extraWaitTime := 180 * time.Second
			totalWaitTime = totalWaitTime + extraWaitTime
			restBackoff.MaxElapsedTime = extraWaitTime

			log.Debugf("Encountered HTTP 503 error, REST interface is not up after 30 seconds, "+
				"waiting %3.2f seconds extra.", extraWaitTime.Seconds())
			err = backoff.RetryNotify(checkRESTInterface, restBackoff, restNotify)
		}

		if err != nil {
			log.Errorf("Trident REST interface was not available after %3.2f seconds; err: %v", totalWaitTime.Seconds(),
				err)
			return err
		}
	}

	versionInfo, err := utils.ParseDate(versionWithMetadata)
	if err != nil {
		log.WithField("version", versionWithMetadata).Errorf("unable to parse version with metadata")
		version = versionWithMetadata
	} else {
		version = versionInfo.ShortStringWithRelease()
	}

	log.WithField("version", version).Info("Trident REST interface is up.")

	return nil
}

// getTridentClientVersionInfo takes trident image name and identifies the Trident client version
func (i *Installer) getTridentClientVersionInfo(imageName string, controllingCRDetails map[string]string) (*api.
	ClientVersionResponse,
	error) {

	clientVersionYAML, err := i.getTridentVersionYAML(imageName, controllingCRDetails)
	if err != nil {
		return nil, err
	}

	log.WithField("image", imageName).Debugf("Successfully retrieved version information in YAML format: \n%s",
		string(clientVersionYAML))
	clientVersion := api.ClientVersionResponse{}
	if err := yaml.Unmarshal(clientVersionYAML, &clientVersion); err != nil {
		errMessage := fmt.Sprintf("unable to umarshall client version YAML to Version struct; err: %v", err)
		log.WithField("image", imageName).Errorf(errMessage)

		return nil, fmt.Errorf(errMessage)
	}

	log.WithField("version", clientVersion).Debugf("Successfully found Trident image version information.")

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
		log.Error(errMessage)

		if err := i.client.DeletePodByLabel(TridentVersionPodLabel); err != nil {
			log.WithFields(log.Fields{
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
		log.WithFields(log.Fields{
			"image": imageName,
			"pod":   podName,
			"err":   err,
		}).Errorf("Could not delete Trident version pod.")
	}

	log.WithFields(log.Fields{
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

	serviceAccountName := getServiceAccountName(true)

	newTridentVersionPodYAML := k8sclient.GetTridentVersionPodYAML(
		podName, imageName, serviceAccountName, imagePullSecrets, podLabels, controllingCRDetails,
	)

	err = i.client.CreateObjectByYAML(newTridentVersionPodYAML)
	if err != nil {
		err = fmt.Errorf("failed to create Trident version pod; %v", err)
		return err
	}
	log.WithFields(log.Fields{
		"pod":       podName,
		"namespace": i.namespace,
	}).Info("Created Trident version pod.")

	return nil
}
