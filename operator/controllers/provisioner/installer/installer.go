// Copyright 2020 NetApp, Inc. All Rights Reserved.

package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v12 "k8s.io/api/rbac/v1"
	v1beta12 "k8s.io/api/storage/v1beta1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/client-go/rest"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	netappv1 "github.com/netapp/trident/operator/controllers/provisioner/apis/netapp/v1"
	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
)

const (
	// CRD names
	BackendCRDName      = "tridentbackends.trident.netapp.io"
	NodeCRDName         = "tridentnodes.trident.netapp.io"
	StorageClassCRDName = "tridentstorageclasses.trident.netapp.io"
	TransactionCRDName  = "tridenttransactions.trident.netapp.io"
	VersionCRDName      = "tridentversions.trident.netapp.io"
	VolumeCRDName       = "tridentvolumes.trident.netapp.io"
	SnapshotCRDName     = "tridentsnapshots.trident.netapp.io"

	VolumeSnapshotCRDName        = "volumesnapshots.snapshot.storage.k8s.io"
	VolumeSnapshotClassCRDName   = "volumesnapshotclasses.snapshot.storage.k8s.io"
	VolumeSnapshotContentCRDName = "volumesnapshotcontents.snapshot.storage.k8s.io"

	DefaultTimeout = 30

	TridentCSI       = "trident-csi"
	TridentLegacy    = "trident"
	OpenShiftSCCName = "trident"

	CSIDriver  = "csi.trident.netapp.io"
	TridentPSP = "tridentpods"
)

var (
	// CR inputs
	csi                bool
	debug              bool
	useIPv6            bool
	silenceAutosupport bool
	enableNodePrep     bool

	logFormat     string
	tridentImage  string
	imageRegistry string
	kubeletDir    string

	autosupportImage        string
	autosupportProxy        string
	autosupportSerialNumber string
	autosupportHostname     string

	imagePullSecrets []string

	k8sTimeout time.Duration

	appLabel      string
	appLabelKey   string
	appLabelValue string

	CRDnames = []string{
		BackendCRDName,
		NodeCRDName,
		StorageClassCRDName,
		TransactionCRDName,
		VersionCRDName,
		VolumeCRDName,
		SnapshotCRDName,
	}

	AlphaCRDNames = []string{
		VolumeSnapshotCRDName,
		VolumeSnapshotClassCRDName,
		VolumeSnapshotContentCRDName,
	}

	useCRDv1 bool
)

type Installer struct {
	client           k8sclient.Interface
	tridentCRDClient *crdclient.Clientset
	namespace        string
}

func NewInstaller(kubeConfig *rest.Config, namespace string, timeout int) (*Installer, error) {

	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	k8sTimeout = time.Duration(timeout) * time.Second

	// Create the Kubernetes client
	kubeClient, err := k8sclient.NewKubeClient(kubeConfig, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// Create Trident CRD client
	CRDClientForTrident, err := crdclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Trident's CRD client; %v", err)
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

	currentDeployment, _, _, err := i.TridentDeploymentInformation(appLabel, csi)
	if err != nil {
		return identifiedImageVersion,
			fmt.Errorf("unable to get existing deployment information for the image verification; err: %v", err)
	}

	if currentDeployment == nil {
		log.Debugf("No Trident deployment exist.")
		performImageVersionCheck = true
	} else {
		log.Debugf("Trident deployment exist.")

		containers := currentDeployment.Spec.Template.Spec.Containers

		for _, container := range containers {
			if container.Name == "trident-main" {
				if container.Image != tridentImage {
					log.Debugf("Current Trident deployment image '%s' is not same as the new Trident Image '%s'.",
						container.Image, tridentImage)
					performImageVersionCheck = true
				}
			}
		}

		deploymentImageVersion := currentDeployment.Labels[TridentVersionLabelKey]
		// Contingency plan to recover Trident version information
		if deploymentImageVersion == "" {
			log.Errorf("Deployment is missing the version information; fixing it.")
			performImageVersionCheck = true
		} else {
			identifiedImageVersion = deploymentImageVersion
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
			return "", fmt.Errorf("unable to get Trident image version information; err: %v", err)
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
		log.Debugf("New Trident image '%s' (version: %s) is supported.", tridentImage, tridentImageVersion.ShortStringWithRelease())
	}

	return identifiedImageVersion, nil
}

// setInstallationParams identifies the correct parameters for the Trident installation
func (i *Installer) setInstallationParams(cr netappv1.TridentProvisioner,
	currentInstallationVersion string) (map[string]string, map[string]string, bool, error) {

	var identifiedImageVersion string
	var defaultImageOverride bool
	var imageUpdateNeeded bool
	var returnError error

	// Get default values
	logFormat = DefaultLogFormat
	tridentImage = TridentImage
	imageRegistry = DefaultImageRegistry
	kubeletDir = DefaultKubeletDir
	autosupportImage = commonconfig.DefaultAutosupportImage

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

	appLabel = TridentCSILabel
	appLabelKey = TridentCSILabelKey
	appLabelValue = TridentCSILabelValue

	minForcedCRDVersion := utils.MustParseSemantic(commonconfig.KubernetesCRDVersionMinForced)
	if i.client.ServerVersion().AtLeast(minForcedCRDVersion) {
		useCRDv1 = true
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

		log.Debugf("Identified Trident image '%s' version to be '%s'", tridentImage, identifiedImageVersion)
	} else {
		identifiedImageVersion = TridentVersionLabelValue

		log.Debugf("Using default Trident image '%s', version '%s'", tridentImage, identifiedImageVersion)
	}

	// Identify if this is an update scenario, i.e. Trident version has changed
	if currentInstallationVersion != identifiedImageVersion {
		log.Infof("Current deployment version '%s' is not same as the Trident image version '%s'; need to update the"+
			" installation", currentInstallationVersion, identifiedImageVersion)
		imageUpdateNeeded = true
	}
	// Perform log prechecks
	if returnError = i.logFormatPrechecks(); returnError != nil {
		return nil, nil, false, returnError
	}

	// Update the label with the correct version
	labels[TridentVersionLabelKey] = identifiedImageVersion

	return controllingCRDetails, labels, imageUpdateNeeded, nil
}

func (i *Installer) InstallOrPatchTrident(cr netappv1.TridentProvisioner,
	currentInstallationVersion string, shouldUpdate bool) (*netappv1.TridentProvisionerSpecValues, string, error) {

	var returnError error
	var newServiceAccount bool

	// Set installation params
	controllingCRDetails, labels, imageUpdateNeeded, err := i.setInstallationParams(cr, currentInstallationVersion)
	if err != nil {
		return nil, "", err
	}

	// Identify if update is required because of change in K8s version or Trident Operator version
	shouldUpdate = shouldUpdate || imageUpdateNeeded

	// Begin Trident installation logic...

	// All checks succeeded, so proceed with installation
	log.WithField("namespace", i.namespace).Info("Starting Trident installation.")

	// Create or patch or update the RBAC objects
	if newServiceAccount, returnError = i.createRBACObjects(controllingCRDetails, labels,
		shouldUpdate); returnError != nil {
		return nil, "", returnError
	}

	// Create CRDs and ensure they are established
	returnError = i.createAndEnsureCRDs()
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident CRDs; %v", returnError)
		return nil, "", returnError
	}

	// Patch the CRD definitions with finalizers to protect them
	if returnError = i.protectCustomResourceDefinitions(); returnError != nil {
		return nil, "", returnError
	}

	// Create or patch or update the RBAC PSPs
	returnError = i.createOrPatchTridentPodSecurityPolicy(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident pod security policy; %v", returnError)
		return nil, "", returnError
	}

	if !csi {
		// Create or patch or update the legacy deployment object
		returnError = i.createOrPatchTridentDeployment(controllingCRDetails, labels, shouldUpdate, newServiceAccount)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Trident Deployment; %v", returnError)
			return nil, "", returnError
		}
	} else {

		// Create or patch or update the CSI CRDs if necessary (1.13 only)
		returnError = i.createK8S113CSICustomResourceDefinitions()
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Kubernetes 1.13 CSI CRDs; %v", returnError)
			return nil, "", returnError
		}

		// Create or patch or update the CSI Driver object if necessary (1.14+)
		returnError = i.createOrPatchK8sCSIDriver(controllingCRDetails, labels, shouldUpdate)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Kubernetes CSI Driver object; %v", returnError)
			return nil, "", returnError
		}

		// Create or patch or update the Trident Service
		returnError = i.createOrPatchTridentService(controllingCRDetails, labels, shouldUpdate)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Trident Service; %v", returnError)
			return nil, "", returnError
		}

		// Create or update the Trident Secret
		returnError = i.createOrPatchTridentSecret(controllingCRDetails, labels, shouldUpdate)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Trident Secret; %v", returnError)
			return nil, "", returnError
		}

		// Create or update the Trident CSI deployment
		returnError = i.createOrPatchTridentDeployment(controllingCRDetails, labels, shouldUpdate, newServiceAccount)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Trident Deployment; %v", returnError)
			return nil, "", returnError
		}

		// Create or update the Trident CSI daemonset
		returnError = i.createOrPatchTridentDaemonSet(controllingCRDetails, labels, shouldUpdate, newServiceAccount)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Trident DaemonSet; %v", returnError)
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

	identifiedSpecValues := netappv1.TridentProvisionerSpecValues{
		Debug:                   strconv.FormatBool(debug),
		LogFormat:               logFormat,
		TridentImage:            tridentImage,
		ImageRegistry:           imageRegistry,
		IPv6:                    strconv.FormatBool(useIPv6),
		SilenceAutosupport:      strconv.FormatBool(silenceAutosupport),
		AutosupportImage:        autosupportImage,
		AutosupportProxy:        autosupportProxy,
		AutosupportSerialNumber: autosupportSerialNumber,
		AutosupportHostname:     autosupportHostname,
		KubeletDir:              kubeletDir,
		K8sTimeout:              strconv.Itoa(int(k8sTimeout.Seconds())),
		ImagePullSecrets:        imagePullSecrets,
		EnableNodePrep:          strconv.FormatBool(enableNodePrep),
	}

	log.WithFields(log.Fields{
		"namespace":  i.namespace,
		"version":    labels[TridentVersionLabelKey],
		"specValues": identifiedSpecValues,
	}).Info("Trident is installed.")
	return &identifiedSpecValues, labels[TridentVersionLabelKey], nil
}

func (i *Installer) createCustomResourceDefinitions(crdName, crdYAML string) (returnError error) {

	var logFields log.Fields

	returnError = i.client.CreateObjectByYAML(crdYAML)
	logFields = log.Fields{"namespace": i.namespace}

	if returnError != nil {
		returnError = fmt.Errorf("could not create custom resource  %v in %s; %v", crdName, i.namespace, returnError)
		return
	}
	log.WithFields(logFields).Infof("Created custom resource definitions %v.", crdName)
	return nil
}

func (i *Installer) ensureCRDsRegistered(crdNames []string) error {

	for _, crdName := range crdNames {
		if err := i.ensureCRDRegistered(crdName); err != nil {
			return err
		}
	}

	return nil
}

// ensureCRDRegistered waits until a CRD is known to Kubernetes.
func (i *Installer) ensureCRDRegistered(crdName string) error {

	checkCRDRegistered := func() error {
		if exists, err := i.client.CheckCRDExists(crdName); err != nil {
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

// protectCustomResourceDefinitions adds finalizers to the CRD definitions to prevent accidental deletion
func (i *Installer) protectCustomResourceDefinitions() error {
	err := i.client.AddFinalizerToCRDs(CRDnames)
	if err != nil {
		return err
	}
	log.Info("Added finalizers to custom resource definitions.")
	return nil
}

func (i *Installer) createK8S113CSICustomResourceDefinitions() error {

	// We only have to create these CRDs on Kubernetes 1.13
	if i.client.ServerVersion().MajorVersion() != 1 || i.client.ServerVersion().MinorVersion() != 13 {
		return nil
	}

	csiDriversCRDExists, err := i.client.CheckCRDExists("csidrivers.csi.storage.k8s.io")
	if err != nil {
		return fmt.Errorf("could not check if CRD csidrivers.csi.storage.k8s.io exists; %v", err)
	} else if !csiDriversCRDExists {
		if err = i.client.CreateObjectByYAML(k8sclient.GetCSIDriverCRDYAML()); err != nil {
			return fmt.Errorf("could not create CRD csidrivers.csi.storage.k8s.io; %v", err)
		}
	}

	csiNodeInfosCRDExists, err := i.client.CheckCRDExists("csinodeinfos.csi.storage.k8s.io")
	if err != nil {
		return fmt.Errorf("could not check if CRD csinodeinfos.csi.storage.k8s.io exists; %v", err)
	} else if !csiNodeInfosCRDExists {
		if err = i.client.CreateObjectByYAML(k8sclient.GetCSINodeInfoCRDYAML()); err != nil {
			return fmt.Errorf("could not create CRD csinodeinfos.csi.storage.k8s.io; %v", err)
		}
	}

	return nil
}

// createCRDs creates and establishes each of the CRDs individually
func (i *Installer) createCRDs() error {
	var err error

	if err = i.createCRD(VersionCRDName, k8sclient.GetVersionCRDYAML(useCRDv1)); err != nil {
		return err
	}
	if err = i.createCRD(BackendCRDName, k8sclient.GetBackendCRDYAML(useCRDv1)); err != nil {
		return err
	}
	if err = i.createCRD(StorageClassCRDName, k8sclient.GetStorageClassCRDYAML(useCRDv1)); err != nil {
		return err
	}
	if err = i.createCRD(VolumeCRDName, k8sclient.GetVolumeCRDYAML(useCRDv1)); err != nil {
		return err
	}
	if err = i.createCRD(NodeCRDName, k8sclient.GetNodeCRDYAML(useCRDv1)); err != nil {
		return err
	}
	if err = i.createCRD(TransactionCRDName, k8sclient.GetTransactionCRDYAML(useCRDv1)); err != nil {
		return err
	}
	if err = i.createCRD(SnapshotCRDName, k8sclient.GetSnapshotCRDYAML(useCRDv1)); err != nil {
		return err
	}

	return err
}

// createCRD creates and establishes the CRD
func (i *Installer) createCRD(crdName, crdYAML string) error {

	// Discover CRD data
	crdExist, returnError := i.client.CheckCRDExists(crdName)
	if returnError != nil {
		return fmt.Errorf("unable to identify if %v CRD exists; err: %v", crdName, returnError)
	}

	if crdExist {
		log.Infof("Trident %v CRD present.", crdName)
	} else {
		// Create the CRDs and wait for them to be registered in Kubernetes
		log.Infof("Installer will create a fresh %v CRD.", crdName)

		if returnError = i.createCustomResourceDefinitions(crdName, crdYAML); returnError != nil {
			return returnError
		}

		// Wait for the CRD to be fully established
		if returnError = i.ensureCRDEstablished(crdName); returnError != nil {
			// If CRD registration failed *and* we created the CRDs, clean up by deleting the CRDs
			log.Errorf("CRDs not established; %v", returnError)
			if err := i.deleteCustomResourceDefinition(crdName, crdYAML); err != nil {
				log.Errorf("Could not delete CRDs; %v", err)
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
			if condition.Type == apiextensionv1beta1.Established {
				switch condition.Status {
				case apiextensionv1beta1.ConditionTrue:
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

func (i *Installer) createOrPatchK8sCSIDriver(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {

	// We only have to create this object on Kubernetes 1.14+
	if i.client.ServerVersion().MajorVersion() != 1 || i.client.ServerVersion().MinorVersion() < 14 {
		return nil
	}

	createCSIDriver := true
	CSIDriverName := getCSIDriverName()
	var currentK8sCSIDriver *v1beta12.CSIDriver
	var unwantedCSIDrivers []v1beta12.CSIDriver
	var logFields log.Fields
	var err error

	if csiDrivers, err := i.client.GetCSIDriversByLabel(appLabel); err != nil {
		log.Errorf("Unable to get list of CSI driver custom resources by label %v", appLabel)
		return fmt.Errorf("unable to get list of CSI driver custom resources by label")
	} else if len(csiDrivers) == 0 {
		log.Info("CSI driver custom resource not found.")

		log.Debug("Deleting unlabeled Trident CSI Driver by name as it can cause issues during installation.")
		if err = i.client.DeleteCSIDriver(CSIDriverName); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident CSI driver custom resource.")
			}
		} else {
			log.WithField("CSIDriver", CSIDriverName).Info(
				"Deleted Trident CSI driver custom resource; replacing it with a labeled Trident CSI driver custom" +
					" resource.")
		}

	} else if shouldUpdate {
		unwantedCSIDrivers = csiDrivers
	} else {
		// Rules:
		// 1. If there is no CSI driver CR named csi.trident.netapp.io and one or many other CSI driver CRs
		//    exist that matches the label then remove all the CSI driver CRs.
		// 2. If there is a CSI driver CR named csi.trident.netapp.io and one or many other CSI driver CRs
		//    exist that matches the label then remove all other CSI driver CRs.
		for _, csiDriver := range csiDrivers {
			if csiDriver.Name == CSIDriverName {
				// Found a pod security policy named tridentpod in the same namespace
				log.Infof("A Trident CSI driver CR named '%s' was found by label.", CSIDriverName)

				currentK8sCSIDriver = &csiDriver
				createCSIDriver = false
			} else {
				log.Errorf("a Trident CSI driver CR %s was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion",
					csiDriver.Name, CSIDriverName)

				unwantedCSIDrivers = append(unwantedCSIDrivers, csiDriver)
			}
		}
	}

	if err = i.RemoveMultipleCSIDriverCRs(unwantedCSIDrivers); err != nil {
		return err
	}

	newK8sCSIDriverYAML := k8sclient.GetCSIDriverCRYAML(CSIDriverName, labels, controllingCRDetails)

	if createCSIDriver {
		err = i.client.CreateObjectByYAML(newK8sCSIDriverYAML)
		logFields = log.Fields{}

		if err != nil {
			return fmt.Errorf("could not create CSI driver custom resource; %v", err)
		}
		log.WithFields(logFields).Info("Created CSI driver custom resource.")
	} else {
		log.WithFields(log.Fields{
			"CSIDriver": currentK8sCSIDriver.Name,
		}).Debug("Patching Trident CSI driver CR.")
		err = i.patchK8sCSIDriver(currentK8sCSIDriver, []byte(newK8sCSIDriverYAML))
	}

	return nil
}

func (i *Installer) createRBACObjects(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) (newServiceAccount bool, returnError error) {

	// Create service account
	newServiceAccount, returnError = i.createOrPatchTridentServiceAccount(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident service account; %v", returnError)
		return
	}

	// Create cluster role
	returnError = i.createOrPatchTridentClusterRole(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident cluster role; %v", returnError)
		return
	}

	// Create cluster role binding
	returnError = i.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, shouldUpdate)
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident cluster role binding; %v", returnError)
		return
	}

	// If OpenShift, create a new security context constraint for Trident
	if i.client.Flavor() == k8sclient.FlavorOpenShift {
		// Create OpenShift SCC
		returnError = i.createOrPatchTridentOpenShiftSCC(controllingCRDetails, labels, shouldUpdate)
		if returnError != nil {
			returnError = fmt.Errorf("could not create the Trident OpenShift SCC; %v", returnError)
			return
		}
	}

	return
}

func (i *Installer) createOrPatchTridentServiceAccount(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) (bool, error) {
	createServiceAccount := true
	newServiceAccount := false
	var currentServiceAccount *v1.ServiceAccount
	var unwantedServiceAccounts []v1.ServiceAccount
	var serviceAccountSecretNames []string
	var logFields log.Fields
	var err error

	serviceAccountName := getServiceAccountName(csi)

	if serviceAccounts, err := i.client.GetServiceAccountsByLabel(appLabel, false); err != nil {
		log.Errorf("Unable to get list of service accounts by label %v", appLabel)
		return newServiceAccount, fmt.Errorf("unable to get list of service accounts")
	} else if len(serviceAccounts) == 0 {
		log.Info("Trident service account not found.")

		log.Debug("Deleting unlabeled Trident service account by name as it can cause issues during installation.")
		if err = i.client.DeleteServiceAccount(serviceAccountName, i.namespace); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident service account.")
			}
		} else {
			log.WithField("Service Account", serviceAccountName).Info(
				"Deleted Trident service account; replacing it with a labeled Trident service account.")
		}
	} else if shouldUpdate {
		unwantedServiceAccounts = serviceAccounts
	} else {
		// Rules:
		// 1. If there are no service accounts named trident-csi and one or many other service accounts
		//    exist that matches the label then remove all the service accounts.
		// 2. If there is a service accounts named trident-csi and one or many other service accounts
		//    exist that matches the label then remove all other service accounts.
		for _, serviceAccount := range serviceAccounts {
			if serviceAccount.Name == serviceAccountName {
				// Found a service account named trident-csi in the same namespace
				log.Infof("A Trident Service account named '%s' was found by label.", serviceAccountName)

				currentServiceAccount = &serviceAccount
				createServiceAccount = false

				for _, serviceAccountSecret := range serviceAccount.Secrets {
					serviceAccountSecretNames = append(serviceAccountSecretNames, serviceAccountSecret.Name)
				}
			} else {
				log.Errorf("a Trident Service account %s was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion",
					serviceAccount.Name, serviceAccountName)

				unwantedServiceAccounts = append(unwantedServiceAccounts)
			}
		}
	}

	if err = i.RemoveMultipleServiceAccounts(unwantedServiceAccounts); err != nil {
		return newServiceAccount, err
	}

	newServiceAccountYAML := k8sclient.GetServiceAccountYAML(serviceAccountName, serviceAccountSecretNames, labels,
		controllingCRDetails)

	if createServiceAccount {
		err = i.client.CreateObjectByYAML(newServiceAccountYAML)
		logFields = log.Fields{}

		if err != nil {
			return newServiceAccount, fmt.Errorf("could not create service account; %v", err)
		}
		newServiceAccount = true
		log.WithFields(logFields).Info("Created service account.")
	} else {
		log.WithFields(log.Fields{
			"service":   currentServiceAccount.Name,
			"namespace": currentServiceAccount.Namespace,
		}).Debug("Patching Trident Service account.")

		err = i.patchTridentServiceAccount(currentServiceAccount, []byte(newServiceAccountYAML))
	}

	return newServiceAccount, nil
}

func (i *Installer) createOrPatchTridentClusterRole(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {
	createClusterRole := true
	var currentClusterRole *v12.ClusterRole
	var unwantedClusterRoles []v12.ClusterRole
	var logFields log.Fields
	var err error

	clusterRoleName := getClusterRoleName(csi)

	if clusterRoles, err := i.client.GetClusterRolesByLabel(appLabel); err != nil {
		log.Errorf("Unable to get list of cluster roles by label %v", appLabel)
		return fmt.Errorf("unable to get list of cluster roles")
	} else if len(clusterRoles) == 0 {
		log.Info("Trident cluster role not found.")

		log.Debug("Deleting unlabeled Trident cluster role by name as it can cause issues during installation.")
		if err = i.client.DeleteClusterRole(clusterRoleName); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident cluster role")
			}
		} else {
			log.WithField("Cluster Role", clusterRoleName).Info(
				"Deleted unlabeled Trident cluster role; replacing it with a labeled Trident cluster role.")
		}
	} else if shouldUpdate {
		unwantedClusterRoles = clusterRoles
	} else {
		// Rules:
		// 1. If there is no cluster role named trident-csi and one or many other cluster roles
		//    exist that matches the label then remove all the cluster roles.
		// 2. If there is a cluster role named trident-csi  and one or many other cluster roles
		//    exist that matches the label then remove all other cluster roles.
		for _, clusterRole := range clusterRoles {
			if clusterRole.Name == clusterRoleName {
				// Found a cluster role named trident/trident-csi
				log.Infof("A Trident cluster role named '%s' was found by label.", clusterRoleName)

				currentClusterRole = &clusterRole
				createClusterRole = false
			} else {
				log.Errorf("a Trident cluster role %s was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion",
					clusterRole.Name, clusterRoleName)

				unwantedClusterRoles = append(unwantedClusterRoles, clusterRole)
			}
		}
	}

	if err = i.RemoveMultipleClusterRoles(unwantedClusterRoles); err != nil {
		return err
	}

	newClusterRoleYAML := k8sclient.GetClusterRoleYAML(i.client.Flavor(), clusterRoleName, labels, controllingCRDetails, csi)

	if createClusterRole {
		err = i.client.CreateObjectByYAML(newClusterRoleYAML)
		logFields = log.Fields{}

		if err != nil {
			return fmt.Errorf("could not create cluster role; %v", err)
		}
		log.WithFields(logFields).Info("Created cluster role.")
	} else {
		log.WithFields(log.Fields{
			"clusterRole": currentClusterRole.Name,
		}).Debug("Patching Trident Cluster role.")

		err = i.patchTridentClusterRole(currentClusterRole, []byte(newClusterRoleYAML))
	}

	return nil
}

func (i *Installer) createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {
	createClusterRoleBinding := true
	var currentClusterRoleBinding *v12.ClusterRoleBinding
	var unwantedClusterRoleBindings []v12.ClusterRoleBinding
	var logFields log.Fields
	var err error

	clusterRoleBindingName := getClusterRoleBindingName(csi)

	if clusterRoleBindings, err := i.client.GetClusterRoleBindingsByLabel(appLabel); err != nil {
		log.Errorf("Unable to get list of cluster role bindings by label %v", appLabel)
		return fmt.Errorf("unable to get list of cluster role bindings")
	} else if len(clusterRoleBindings) == 0 {
		log.Info("Trident cluster role binding not found.")

		log.Debug("Deleting unlabeled Trident cluster role binding by name as it can cause issues during installation.")
		if err = i.client.DeleteClusterRoleBinding(clusterRoleBindingName); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident cluster role binding.")
			}
		} else {
			log.WithField("Cluster Role Binding", clusterRoleBindingName).Info(
				"Deleted unlabeled Trident cluster role binding; replacing it with a labeled Trident cluster role" +
					" binding.")
		}
	} else if shouldUpdate {
		unwantedClusterRoleBindings = clusterRoleBindings
	} else {
		// Rules:
		// 1. If there is no cluster role binding named trident/trident-csi and one or many other cluster role bindings
		//    exist that matches the label then remove all the cluster role bindings.
		// 2. If there is a cluster role binding named trident/trident-csi and one or many other cluster role bindings
		//    exist that matches the label then remove all other cluster role bindings.
		for _, clusterRoleBinding := range clusterRoleBindings {
			if clusterRoleBinding.Name == clusterRoleBindingName {
				// Found a cluster role binding named trident/trident-csi
				log.Infof("A Trident Cluster role binding named '%s' was found by label.", clusterRoleBindingName)

				currentClusterRoleBinding = &clusterRoleBinding
				createClusterRoleBinding = false
			} else {
				log.Errorf("a Trident Cluster role binding %s was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion",
					clusterRoleBinding.Name, clusterRoleBindingName)

				unwantedClusterRoleBindings = append(unwantedClusterRoleBindings, clusterRoleBinding)
			}
		}
	}

	if err = i.RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings); err != nil {
		return err
	}

	newClusterRoleBindingYAML := k8sclient.GetClusterRoleBindingYAML(i.namespace, i.client.Flavor(), clusterRoleBindingName,
		labels, controllingCRDetails)

	if createClusterRoleBinding {
		err = i.client.CreateObjectByYAML(newClusterRoleBindingYAML)
		logFields = log.Fields{}

		if err != nil {
			return fmt.Errorf("could not create cluster role binding; %v", err)
		}
		log.WithFields(logFields).Info("Created cluster role binding.")
	} else {
		log.WithFields(log.Fields{
			"clusterRoleBinding": currentClusterRoleBinding.Name,
		}).Debug("Patching Trident Cluster role binding.")

		err = i.patchTridentClusterRoleBinding(currentClusterRoleBinding, []byte(newClusterRoleBindingYAML))
	}

	return nil
}

func (i *Installer) createOrPatchTridentOpenShiftSCC(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {
	createOpenShiftSCC := true
	var openShiftSCCOldUserName string
	var openShiftSCCOldName string
	var currentOpenShiftSCCJSON []byte
	var removeExistingSCC bool
	var logFields log.Fields
	var err error

	if csi {
		openShiftSCCOldUserName = "trident-csi"
		openShiftSCCOldName = "privileged"
	} else {
		openShiftSCCOldUserName = "trident"
		openShiftSCCOldName = "anyuid"
	}

	openShiftSCCUserName := getOpenShiftSCCUserName()
	openShiftSCCName := getOpenShiftSCCName()

	logFields = log.Fields{
		"sccUserName": openShiftSCCUserName,
		"sccName":     openShiftSCCName,
		"label":       appLabelValue,
	}

	if SCCExist, SCCUserExist, jsonData, err := i.client.GetOpenShiftSCCByName(openShiftSCCUserName, openShiftSCCName); err != nil {
		log.WithFields(logFields).Errorf("Unable to get OpenShift SCC for Trident; err: %v", err)
		return fmt.Errorf("unable to get OpenShift SCC for Trident")
	} else if !SCCExist {
		log.WithFields(logFields).Info("Trident OpenShift SCC not found.")
	} else if !SCCUserExist {
		log.WithFields(logFields).Info("Trident OpenShift SCC found, but SCC user does not exist.")
		removeExistingSCC = true
	} else if shouldUpdate {
		removeExistingSCC = true
	} else {
		currentOpenShiftSCCJSON = jsonData
		createOpenShiftSCC = false
	}

	if removeExistingSCC {
		if err = i.client.DeleteObjectByYAML(k8sclient.GetOpenShiftSCCQueryYAML(openShiftSCCName), true); err != nil {
			log.WithFields(logFields).Errorf("unable to delete OpenShift SCC; err: %v", err)
			return err
		}
	}

	newOpenShiftSCCYAML := k8sclient.GetOpenShiftSCCYAML(openShiftSCCName, openShiftSCCUserName, i.namespace,
		labels, controllingCRDetails)

	if createOpenShiftSCC {

		// Remove trident user from built-in SCC from previous installation
		if err = i.client.RemoveTridentUserFromOpenShiftSCC(openShiftSCCOldUserName, openShiftSCCOldName); err != nil {
			log.Debugf("No Trident user was found in SCC - continuing anyway: %v", err)
		}
		logFields = log.Fields{}

		if err = i.client.CreateObjectByYAML(newOpenShiftSCCYAML); err != nil {
			return fmt.Errorf("could not create OpenShift SCC; %v", err)
		}
		log.WithFields(logFields).Info("Created OpenShift SCC.")
	} else {
		log.WithFields(logFields).Debug("Patching Trident OpenShift SCC.")

		err = i.patchTridentOpenShiftSCC(currentOpenShiftSCCJSON, []byte(newOpenShiftSCCYAML))
	}

	return nil
}

func (i *Installer) createAndEnsureCRDs() (returnError error) {

	returnError = i.createCRDs()
	if returnError != nil {
		returnError = fmt.Errorf("could not create the Trident CRDs; %v", returnError)
		return
	}

	return nil
}

func (i *Installer) createOrPatchTridentPodSecurityPolicy(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {
	createPSP := true
	var currentPSP *v1beta1.PodSecurityPolicy
	var unwantedPSPs []v1beta1.PodSecurityPolicy
	var err error

	pspName := getPSPName()

	if podSecurityPolicies, err := i.client.GetPodSecurityPoliciesByLabel(appLabel); err != nil {
		log.Errorf("Unable to get list of pod security policies by label %v", appLabel)
		return fmt.Errorf("unable to get list of pod security policies")
	} else if len(podSecurityPolicies) == 0 {
		log.Info("Trident pod security policy not found.")

		log.Debug("Deleting unlabeled Trident pod security policy by name as it can cause issues during installation.")
		if err = i.client.DeletePodSecurityPolicy(pspName); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident pod security policy.")
			}
		} else {
			log.WithField("Pod Security Policy", pspName).Info(
				"Deleted Trident pod security policy; replacing it with a labeled Trident pod security policy.")
		}
	} else if shouldUpdate {
		unwantedPSPs = podSecurityPolicies
	} else {
		// Rules:
		// 1. If there is no psp named tridentpods and one or many other pod security policies
		//    exist that matches the label then remove all the pod security policies.
		// 2. If there is a psp named tridentpods and one or many other pod security policies
		//    exist that matches the label then remove all other pod security policies.
		for _, psp := range podSecurityPolicies {
			if psp.Name == pspName {
				// Found a pod security policy named tridentpods
				log.Infof("A Trident Pod security policy named '%s' was found by label.", pspName)

				currentPSP = &psp
				createPSP = false
			} else {
				log.Errorf("a Trident Pod security policy %s was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion",
					psp.Name, pspName)

				unwantedPSPs = append(unwantedPSPs, psp)
			}
		}
	}

	if err = i.RemoveMultiplePodSecurityPolicies(unwantedPSPs); err != nil {
		return err
	}

	var newPSPYAML string
	if csi {
		newPSPYAML = k8sclient.GetPrivilegedPodSecurityPolicyYAML(pspName, labels, controllingCRDetails)
	} else {
		newPSPYAML = k8sclient.GetUnprivilegedPodSecurityPolicyYAML(pspName, labels, controllingCRDetails)
	}

	if createPSP {
		// Create pod security policy
		err = i.client.CreateObjectByYAML(newPSPYAML)
		if err != nil {
			return fmt.Errorf("could not create Trident pod security policy; %v", err)
		}
		log.Info("Created Trident Pod security policy.")
	} else {
		log.WithFields(log.Fields{
			"podSecurityPolicy": currentPSP.Name,
		}).Debug("Patching Trident Pod security policy.")

		err = i.patchTridentPodSecurityPolicy(currentPSP, []byte(newPSPYAML))
	}

	return nil
}

func (i *Installer) createOrPatchTridentService(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {
	createService := true
	var currentService *v1.Service
	var unwantedServices []v1.Service
	var err error

	serviceName := getServiceName()

	if services, err := i.client.GetServicesByLabel(appLabel, true); err != nil {
		log.Errorf("Unable to get list of services by label %v", appLabel)
		return fmt.Errorf("unable to get list of services")
	} else if len(services) == 0 {
		log.Info("Trident service not found.")

		log.Debug("Deleting unlabeled Trident service by name as it can cause issues during installation.")
		if err = i.client.DeleteService(serviceName, i.namespace); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident service.")
			}
		} else {
			log.WithField("Service", serviceName).Info(
				"Deleted Trident service; replacing it with a labeled Trident service.")
		}
	} else if shouldUpdate {
		unwantedServices = services
	} else {
		// Rules:
		// 1. If there is no service named trident-csi in CR namespace and one or many other services exist
		//    that matches the label then remove all the services.
		// 2. If there is a service named trident-csi in CR namespace and one or many other services exist
		//    that matches the label then remove all other services.
		for _, service := range services {
			if i.namespace == service.Namespace && service.Name == serviceName {
				// Found a service named trident-csi in the same namespace
				log.Infof("A Trident service name trident-csi was found by label in CR namespace '%s'.", i.namespace)

				createService = false
				currentService = &service
			} else {
				log.Errorf("a Trident service %s was found by label in namespace '%s', "+
					"which does not meet either name %s or namespace '%s' requirement, marking it for deletion",
					service.Name, service.Namespace, serviceName, i.namespace)

				unwantedServices = append(unwantedServices, service)
			}
		}
	}

	if err = i.RemoveMultipleServices(unwantedServices); err != nil {
		return err
	}

	newServiceYAML := k8sclient.GetCSIServiceYAML(serviceName, labels, controllingCRDetails)

	if createService {
		err = i.client.CreateObjectByYAML(newServiceYAML)
		if err != nil {
			err = fmt.Errorf("could not create Trident service; %v", err)
			return err
		}
		log.Info("Created Trident service.")
	} else {
		log.WithFields(log.Fields{
			"service":   currentService.Name,
			"namespace": currentService.Namespace,
		}).Debug("Patching Trident service.")

		err = i.patchTridentService(currentService, []byte(newServiceYAML))
	}

	return nil
}

func (i *Installer) createOrPatchTridentSecret(controllingCRDetails, labels map[string]string,
	shouldUpdate bool) error {
	createSecret := true
	//var currentSecret *v1.Secret
	var unwantedSecrets []v1.Secret
	secretMap := make(map[string]string)
	var err error

	secretName := getSecretName()

	if secrets, err := i.client.GetSecretsByLabel(appLabel, false); err != nil {
		log.Errorf("Unable to get list of secrets by label %v", appLabel)
		return fmt.Errorf("unable to get list of secrets by label")
	} else if len(secrets) == 0 {
		log.Info("Trident secret not found.")

		log.Debug("Deleting unlabeled Trident secret by name as it can cause issues during installation.")
		if err = i.client.DeleteSecret(secretName, i.namespace); err != nil {
			if !utils.IsResourceNotFoundError(err) {
				log.WithField("error", err).Warning("Could not delete Trident secret.")
			}
		} else {
			log.WithField("Secret", secretName).Info(
				"Deleted Trident secret; replacing it with a labeled Trident secret.")
		}
	} else if shouldUpdate {
		unwantedSecrets = secrets
	} else {
		// 1. If there is no secret named trident-csi in CR namespace and one or many other secrets
		//    exist that matches the label then remove all the secrets.
		// 2. If there is a secret named trident-csi in CR namespace and one or many other secret
		//    exist that matches the label then remove all other secrets.
		for _, secret := range secrets {
			if secret.Namespace == i.namespace && secret.Name == secretName {
				// Found a secret named trident-csi in the same namespace
				log.Infof("A Trident secret named '%s' was found by label in CR namespace '%s'.", secretName,
					i.namespace)

				//currentSecret = &secret
				createSecret = false
			} else {
				log.Errorf("a Trident secret %s was found by label in namespace '%s', "+
					"which does not meet either name %s or namespace '%s' requirement, marking it for deletion",
					secret.Name, secret.Namespace, secretName, i.namespace)

				unwantedSecrets = append(unwantedSecrets, secret)
			}
		}
	}

	if err = i.RemoveMultipleSecrets(unwantedSecrets); err != nil {
		return err
	}

	if createSecret {
		// Create the certificates for the CSI controller's HTTPS REST interface
		certInfo, err := utils.MakeHTTPCertInfo(commonconfig.CACertName, commonconfig.ServerCertName, commonconfig.ClientCertName)
		if err != nil {
			return fmt.Errorf("could not create Trident X509 certificates; %v", err)
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

	// Create Secret
	if createSecret {
		err = i.client.CreateObjectByYAML(newSecretYAML)
		if err != nil {
			return fmt.Errorf("could not create Trident secret; %v", err)
		}
		log.Info("Created Trident secret.")
	} else {
		// It is very debatable if secrets should be patch

		//log.WithFields(log.Fields{
		//	"service":   currentSercret.Name,
		//	"namespace": currentSercret.Namespace,
		//}).Debug("Patching Trident secret.")
		//i.patchTridentSecret(currentSecret, []byte(newSecretYAML)
	}

	return nil
}

func (i *Installer) createOrPatchTridentDeployment(controllingCRDetails, labels map[string]string,
	shouldUpdate, newServiceAccount bool) error {

	deploymentName := getDeploymentName(csi)

	topologyEnabled, err := i.client.IsTopologyInUse()
	if err != nil {
		return err
	}

	currentDeployment, unwantedDeployments, createDeployment, err := i.TridentDeploymentInformation(appLabel, csi)
	if err != nil {
		return err
	}

	// Create a new deployment if there is a current deployment and
	// a new service account or it should be updated
	if currentDeployment != nil && (shouldUpdate || newServiceAccount) {
		unwantedDeployments = append(unwantedDeployments, *currentDeployment)
		createDeployment = true
	}

	if err = i.RemoveMultipleDeployments(unwantedDeployments); err != nil {
		return err
	}
	var newDeploymentYAML string
	if csi {
		newDeploymentYAML = k8sclient.GetCSIDeploymentYAML(deploymentName, tridentImage,
			autosupportImage, autosupportProxy, "", autosupportSerialNumber, autosupportHostname,
			imageRegistry, logFormat, imagePullSecrets, labels, controllingCRDetails, debug, useIPv6,
			silenceAutosupport, i.client.ServerVersion(), topologyEnabled)
	} else {
		newDeploymentYAML = k8sclient.GetDeploymentYAML(deploymentName, tridentImage, logFormat, imagePullSecrets, labels,
			controllingCRDetails, debug)
	}

	if createDeployment {
		// Create the deployment
		err = i.client.CreateObjectByYAML(newDeploymentYAML)
		if err != nil {
			return fmt.Errorf("could not create Trident deployment; %v", err)
		}
		log.Info("Created Trident deployment.")
	} else {
		log.WithFields(log.Fields{
			"deployment": currentDeployment.Name,
			"namespace":  currentDeployment.Namespace,
		}).Debug("Patching Trident deployment.")
		err = i.patchTridentDeployment(currentDeployment, []byte(newDeploymentYAML))
	}

	return err
}

// TridentDeploymentInformation identifies the Operator based Trident CSI deployment and unwanted deployments,
// this method can be used for multiple purposes at different point during the Reconcile so it makes sense to
// keep it separate from the createOrPatchTridentDeployment
func (i *Installer) TridentDeploymentInformation(deploymentLabel string, csiVal bool) (*appsv1.Deployment,
	[]appsv1.Deployment, bool, error) {
	createDeployment := true
	var currentDeployment *appsv1.Deployment
	var unwantedDeployments []appsv1.Deployment

	deploymentName := getDeploymentName(csiVal)

	if deployments, err := i.client.GetDeploymentsByLabel(deploymentLabel, true); err != nil {

		log.Errorf("Unable to get list of deployments by label %v", deploymentLabel)
		return nil, nil, createDeployment, fmt.Errorf("unable to get list of deployments")

	} else if len(deployments) == 0 {
		log.Info("Trident deployment not found.")
	} else {
		// Rules:
		// 1. If there is no deployment named trident/trident-csi in CR namespace and one or many other deployment
		//    exist that matches the label then remove all the deployments.
		// 2. If there is a deployment named trident/trident-csi in CR namespace and one or many other deployment
		//    exist that matches the label then remove all other deployments.
		for _, deployment := range deployments {
			if deployment.Namespace == i.namespace && deployment.Name == deploymentName {
				// Found a deployment named in the same namespace
				log.Infof("A Trident deployment named '%s' was found by label in CR namespace '%s'.", deploymentName,
					i.namespace)

				currentDeployment = &deployment
				createDeployment = false
			} else {
				log.Errorf("a Trident deployment %s was found by label in namespace '%s', "+
					"which does not meet either name %s or namespace '%s' requirement, marking it for deletion",
					deployment.Name, deployment.Namespace, deploymentName, i.namespace)

				unwantedDeployments = append(unwantedDeployments, deployment)
			}
		}
	}

	return currentDeployment, unwantedDeployments, createDeployment, nil
}

func (i *Installer) createOrPatchTridentDaemonSet(controllingCRDetails, labels map[string]string,
	shouldUpdate, newServiceAccount bool) error {

	daemonsetName := getDaemonSetName()

	currentDaemonset, unwantedDaemonsets, createDaemonset, err := i.TridentDaemonSetInformation()
	if err != nil {
		return err
	}

	// Create a new daemonset if there is a current daemonset and
	// a new service account or it should be updated
	if currentDaemonset != nil && (shouldUpdate || newServiceAccount) {
		unwantedDaemonsets = append(unwantedDaemonsets, *currentDaemonset)
		createDaemonset = true
	}

	if err = i.RemoveMultipleDaemonSets(unwantedDaemonsets); err != nil {
		return err
	}

	labels[appLabelKey] = TridentNodeLabelValue

	newDaemonSetYAML := k8sclient.GetCSIDaemonSetYAML(daemonsetName, tridentImage, imageRegistry, kubeletDir,
		logFormat, imagePullSecrets, labels, controllingCRDetails, debug, enableNodePrep, i.client.ServerVersion())

	if createDaemonset {
		// Create the daemonset
		err = i.client.CreateObjectByYAML(newDaemonSetYAML)
		if err != nil {
			return fmt.Errorf("could not create Trident daemonset; %v", err)
		}
		log.Info("Created Trident daemonset.")
	} else {
		log.WithFields(log.Fields{
			"daemontset": currentDaemonset.Name,
			"namespace":  currentDaemonset.Namespace,
		}).Debug("Patching Trident daemonset.")
		err = i.patchTridentDaemonSet(currentDaemonset, []byte(newDaemonSetYAML))
	}

	return err
}

// TridentDaemonSetInformation identifies the Operator based Trident CSI daemonset and unwanted daemonsets,
// this method can be used for multiple purposes at different point during the Reconcile so it makes sense to
// keep it separate from the createOrPatchTridentDaemonSet
func (i *Installer) TridentDaemonSetInformation() (*appsv1.DaemonSet,
	[]appsv1.DaemonSet, bool, error) {

	createDaemonset := true
	var currentDaemonset *appsv1.DaemonSet
	var unwantedDaemonsets []appsv1.DaemonSet

	daemonsetName := getDaemonSetName()

	if daemonsets, err := i.client.GetDaemonSetsByLabel(TridentNodeLabel, true); err != nil {

		log.Errorf("Unable to get list of daemonset by label %v", TridentNodeLabel)
		return nil, nil, createDaemonset, fmt.Errorf("unable to get list of daemonset")

	} else if len(daemonsets) == 0 {
		log.Info("Trident daemonset not found.")
	} else {
		// Rules:
		// 1. If there is no daemonset named trident-csi in CR namespace and one or many other daemonsets
		//    exist that matches the label then remove all the daemonset.
		// 2. If there is a daemonset named trident-csi in CR namespace and one or many other daemonsets
		//    exist that matches the label then remove all other daemonsets.
		for _, daemonset := range daemonsets {
			if daemonset.Namespace == i.namespace && daemonset.Name == daemonsetName {
				// Found a daemonset named in the same namespace
				log.Infof("A Trident daemonset named '%s' was found by label in CR namespace '%s'.", daemonsetName,
					i.namespace)

				currentDaemonset = &daemonset
				createDaemonset = false
			} else {
				log.Errorf("a Trident daemonset %s was found by label in namespace '%s', "+
					"which does not meet either name %s or namespace '%s' requirement, marking it for deletion",
					daemonset.Name, daemonset.Namespace, daemonsetName, i.namespace)

				unwantedDaemonsets = append(unwantedDaemonsets, daemonset)
			}
		}
	}

	return currentDaemonset, unwantedDaemonsets, createDaemonset, nil
}

func (i *Installer) waitForTridentPod() (*v1.Pod, error) {

	var pod *v1.Pod

	// Add sleep to make sure we get the pod name, esp. in case where we kill one deployment and the
	// create a new one.
	waitTime := 7 * time.Second
	log.Debugf("Waiting for %v after the patch to make sure we get the right trident-pod name", waitTime)
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

			log.Errorf("encountered error while creating container(s): %v", containerErrors)
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

	log.WithFields(log.Fields{"pod": pod.Name, "namespace": i.namespace}).Info("Trident pod started.")

	return pod, nil
}

func (i *Installer) checkVersionUsingREST(tridentPodName string) (string, error) {
	cliCommand := []string{"tridentctl", "-s", ControllerServer, "version", "-o", "json"}
	versionJSON, err := i.client.Exec(tridentPodName, TridentContainer, cliCommand)
	if err != nil {
		if len(versionJSON) > 0 {
			err = fmt.Errorf("%v; %s", err, strings.TrimSpace(string(versionJSON)))
		}
		return "", err
	}

	var versionResponse api.VersionResponse
	err = json.Unmarshal(versionJSON, &versionResponse)
	if err != nil {
		return "", err
	}

	return versionResponse.Server.Version, nil
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
			log.Errorf("Trident REST interface was not available after %3.2f seconds; err: %v", totalWaitTime.Seconds(), err)
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

	log.WithField("image", imageName).Debugf("Successfully retrieved version information in YAML format: \n%s", string(clientVersionYAML))
	clientVersion := api.ClientVersionResponse{}
	if err := yaml.Unmarshal(clientVersionYAML, &clientVersion); err != nil {
		errMessage := fmt.Sprintf("unable to umarshall client version YAML to Version struct; err: %v", err)
		log.WithField("image", imageName).Errorf(errMessage)

		return nil, fmt.Errorf(errMessage)
	}

	log.Debugf("Successfully found Trident image version information: %+v", clientVersion)

	return &clientVersion, nil

}

// getTridentVersionYAML takes trident image name and identifies the Trident client version YAML, this workflow
// resembles the `kubectl run --rm -it --restart=Never transient-trident-verion-pod --image=<image_name> --
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
		return []byte{}, fmt.Errorf("could not create Trident Version pod from the image provided; %v", returnError)
	}

	// Wait for Trident version pod to provide information
	output, err := i.execPodForVersionInformation(podName, tridentVersionCommand)
	if err != nil {
		errMessage := fmt.Sprintf("could not exec Trident version pod '%s' (image: '%s') for the information; err: %v",
			podName, imageName, err)
		log.Error(errMessage)

		if err := i.client.DeletePodByLabel(TridentVersionPodLabel); err != nil {
			log.WithField("image", imageName).Errorf("could not delete Trident version pod '%s'; err: %v", podName, err)
		}
		return []byte{}, fmt.Errorf(errMessage)
	}

	outputString := string(output)
	messageToDelete := fmt.Sprintf("\npod \"%s\" deleted", podName)
	outputString = strings.Replace(outputString, messageToDelete, "", 1)
	clientVersionYAML := []byte(outputString)

	if err := i.client.DeletePodByLabel(TridentVersionPodLabel); err != nil {
		log.WithField("image", imageName).Errorf("could not delete Trident version pod '%s'; err: %v", podName, err)
	}

	log.WithFields(log.Fields{
		"image": imageName,
	}).Debug("Found Trident version yaml.")

	return clientVersionYAML, nil
}

// createTridentVersionPod takes the pod name and trident image name to create a pod
func (i *Installer) createTridentVersionPod(podName, imageName string, controllingCRDetails,
	podLabels map[string]string) error {
	var unwantedPods []v1.Pod
	var err error

	if pods, err := i.client.GetPodsByLabel(TridentVersionPodLabel, true); err != nil {
		log.Errorf("Unable to get list of Trident version pod by label %v", appLabel)
		return fmt.Errorf("unable to get list of Trident version pods")
	} else if len(pods) == 0 {
		log.Info("Trident version pod found.")
	} else {
		unwantedPods = pods
	}

	if err = i.RemoveMultiplePods(unwantedPods); err != nil {
		return err
	}

	serviceAccountName := getServiceAccountName(csi)

	newTridentVersionPodYAML := k8sclient.GetTridentVersionPodYAML(podName, imageName,
		serviceAccountName, imagePullSecrets, podLabels, controllingCRDetails)

	err = i.client.CreateObjectByYAML(newTridentVersionPodYAML)
	if err != nil {
		err = fmt.Errorf("could not create Trident version pod; %v", err)
		return err
	}
	log.Info("Created Trident version pod.")

	return nil
}

// execPodForVersionInformation takes the pod name and command to execute the command into the container matching
// the pod name
func (i *Installer) execPodForVersionInformation(podName string, tridentVersionCommand []string) ([]byte, error) {

	var execOutput []byte

	checkExecSuccessful := func() error {
		output, execError := i.client.Exec(podName, "", tridentVersionCommand)
		if execError != nil {
			return fmt.Errorf("exec error; %v", execError)
		}

		execOutput = output
		return nil
	}

	execNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Unable to get version information from the Trident version pod yet, waiting.")
	}

	timeout := 30 * time.Second
	execBackoff := backoff.NewExponentialBackOff()
	execBackoff.MaxElapsedTime = timeout

	log.Infof("Waiting for Trident version pod to provide information.")

	if err := backoff.RetryNotify(checkExecSuccessful, execBackoff, execNotify); err != nil {
		errMessage := fmt.Sprintf("Trident version pod was unable to provide information after %3."+
			"2f seconds; err: %v", timeout.Seconds(), err)

		log.Error(errMessage)
		return []byte{}, err
	}

	log.WithFields(log.Fields{
		"pod": podName,
	}).Infof("Trident version pod started.")

	return execOutput, nil
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

func getSecretName() string {
	return TridentCSI
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

func getOpenShiftSCCUserName() string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getOpenShiftSCCName() string {
	return OpenShiftSCCName
}
