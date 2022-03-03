// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage_drivers/astrads/api/v1alpha1"
	"github.com/netapp/trident/utils"
)

const (
	AstraDSNamespace = "astrads-system"
	KubeNamespace    = "kube-system"

	astraDSVolumeUUIDKey         = "astrads.netapp.io/volumeUUID"
	TridentVolumeFinalizer       = "trident.netapp.io/astradsvolume-finalizer"
	TridentExportPolicyFinalizer = "trident.netapp.io/astradsexportpolicy-finalizer"
	TridentSnapshotFinalizer     = "trident.netapp.io/astradsvolumesnapshot-finalizer"

	VolumeStateOnline = "online"

	exportPolicyCreateTimeout = 10 * time.Second
)

const (
	UpdateFlagAnnotations = iota
	UpdateFlagAddFinalizer
	UpdateFlagRemoveFinalizer
	UpdateFlagExportPolicy
	UpdateFlagRequestedSize
	UpdateFlagExportRules
)

var (
	createOpts = metav1.CreateOptions{}
	deleteOpts = metav1.DeleteOptions{}
	getOpts    = metav1.GetOptions{}
	listOpts   = metav1.ListOptions{}
	updateOpts = metav1.UpdateOptions{}

	clustersGVR = v1alpha1.GroupVersion.WithResource("astradsclusters")
	//clustersGVK = v1alpha1.GroupVersion.WithKind("AstraDSCluster")

	volumesGVR = v1alpha1.GroupVersion.WithResource("astradsvolumes")
	volumesGVK = v1alpha1.GroupVersion.WithKind("AstraDSVolume")

	exportPoliciesGVR = v1alpha1.GroupVersion.WithResource("astradsexportpolicies")
	exportPoliciesGVK = v1alpha1.GroupVersion.WithKind("AstraDSExportPolicy")

	snapshotsGVR = v1alpha1.GroupVersion.WithResource("astradsvolumesnapshots")
	snapshotsGVK = v1alpha1.GroupVersion.WithKind("AstraDSVolumeSnapshot")

	qosPoliciesGVR = v1alpha1.GroupVersion.WithResource("astradsqospolicies")
	//qosPoliciesGVK = v1alpha1.GroupVersion.WithKind("AstraDSQosPolicy")
)

type Clients struct {
	restConfig     *rest.Config
	kubeClient     *kubernetes.Clientset
	dynamicClient  dynamic.Interface
	k8SVersion     *k8sversion.Info
	namespace      string
	cluster        *Cluster
	kubeSystemUUID string
}

// NewClient is a factory method for creating a new client instance.
func NewClient() *Clients {
	return &Clients{}
}

func (c *Clients) Init(ctx context.Context, namespace, kubeConfig, cluster string) (*Cluster, string, error) {

	var err error

	c.namespace = namespace

	kubeConfigBytes, err := base64.StdEncoding.DecodeString(kubeConfig)
	if err != nil {
		return nil, "", fmt.Errorf("kubeconfig is not base64 encoded: %v", err)
	}

	c.restConfig, err = clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	if err != nil {
		return nil, "", fmt.Errorf("could not create REST config from kubeconfig; %v", err)
	}

	// Create the Kubernetes client
	if c.kubeClient, err = kubernetes.NewForConfig(c.restConfig); err != nil {
		return nil, "", err
	}

	// Create the dynamic client
	if c.dynamicClient, err = dynamic.NewForConfig(c.restConfig); err != nil {
		return nil, "", fmt.Errorf("could not initialize dynamic AstraDS CRD client; %v", err)
	}

	// Get the Kubernetes server version
	if c.k8SVersion, err = c.kubeClient.Discovery().ServerVersion(); err != nil {
		return nil, "", fmt.Errorf("could not get Kubernetes version: %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
		"namespace": c.namespace,
		"version":   c.k8SVersion.String(),
	}).Info("Created Kubernetes clients.")

	// Discover the cluster
	var adsCluster *Cluster
	if adsCluster, err = c.getCluster(ctx, cluster); err != nil {
		return nil, "", err
	}
	c.cluster = adsCluster

	// Discover the kube-system namespace UUID
	var ns *v1.Namespace
	if ns, err = c.kubeClient.CoreV1().Namespaces().Get(ctx, KubeNamespace, getOpts); err != nil {
		return nil, "", err
	}
	c.kubeSystemUUID = string(ns.UID)

	log.WithFields(log.Fields{
		"cluster": c.cluster.Name,
		"status":  c.cluster.Status,
		"version": c.cluster.Version,
	}).Info("Discovered AstraDS cluster.")

	return c.cluster, c.kubeSystemUUID, nil
}

// //////////////////////////////////////////////////////////////////////////
// Cluster operations BEGIN

// getClusterFromAstraDSCluster converts an AstraDSCluster struct to an internal cluster representation.  All values
// are taken from the status section of the AstraDSCluster, since those reflect the current state of the cluster.
func (c *Clients) getClusterFromAstraDSCluster(ftc *v1alpha1.AstraDSCluster) *Cluster {

	return &Cluster{
		Name:      ftc.Name,
		Namespace: ftc.Namespace,
		UUID:      ftc.Status.ClusterUUID,
		Status:    ftc.Status.ClusterStatus,
		Version:   ftc.Status.Versions.ADS,
	}
}

// getCluster returns the named AstraDS cluster.
func (c *Clients) getCluster(ctx context.Context, name string) (*Cluster, error) {

	unstructuredCluster, err := c.dynamicClient.Resource(clustersGVR).Namespace(AstraDSNamespace).Get(ctx, name, getOpts)
	if err != nil {
		return nil, err
	}

	var astraDSCluster v1alpha1.AstraDSCluster
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredCluster.UnstructuredContent(), &astraDSCluster)
	if err != nil {
		return nil, err
	}

	return c.getClusterFromAstraDSCluster(&astraDSCluster), nil
}

// getClusters discovers and returns all AstraDS clusters.
func (c *Clients) getClusters(ctx context.Context) ([]*Cluster, error) {

	unstructuredList, err := c.dynamicClient.Resource(clustersGVR).Namespace(AstraDSNamespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	var astraDSClusters v1alpha1.AstraDSClusterList
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredList.UnstructuredContent(), &astraDSClusters)
	if err != nil {
		return nil, err
	}

	clusters := make([]*Cluster, 0)

	for _, adscl := range astraDSClusters.Items {
		clusters = append(clusters, c.getClusterFromAstraDSCluster(&adscl))
	}

	return clusters, nil
}

// Cluster operations END
// //////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////
// Volume operations BEGIN

// getVolumeFromAstraDSVolume converts an AstraDSVolume struct to an internal volume representation.  All values
// are taken from the status section of the AstraDSVolume, since those reflect the current state of the volume.
func (c *Clients) getVolumeFromAstraDSVolume(adsvo *v1alpha1.AstraDSVolume) *Volume {

	volume := &Volume{
		Name:              adsvo.Name,
		Namespace:         adsvo.Namespace,
		ResourceVersion:   adsvo.ResourceVersion,
		Annotations:       adsvo.Annotations,
		Labels:            adsvo.Labels,
		DeletionTimestamp: adsvo.DeletionTimestamp,
		Finalizers:        adsvo.Finalizers,

		Type: AstraDSVolumeType(adsvo.Spec.Type),

		VolumePath:      adsvo.Status.VolumePath,
		Permissions:     adsvo.Spec.Permissions,
		DisplayName:     adsvo.Status.DisplayName,
		Created:         adsvo.Status.Created,
		State:           adsvo.Status.State,
		RequestedSize:   adsvo.Status.RequestedSize,
		ActualSize:      adsvo.Status.Size,
		VolumeUUID:      adsvo.Status.VolumeUUID,
		ExportAddress:   adsvo.Status.ExportAddress,
		ExportPolicy:    adsvo.Status.ExportPolicy,
		SnapshotReserve: adsvo.Status.SnapshotReservePercent,
		NoSnapDir:       adsvo.Status.NoSnapDir,
		QoSPolicy:       adsvo.Status.QosPolicy,
		RestorePercent:  adsvo.Status.RestorePercent,
		CloneVolume:     adsvo.Status.CloneVolume,
		CloneSnapshot:   adsvo.Status.CloneSnapshot,
	}

	for _, condition := range adsvo.Status.Conditions {
		switch condition.Type {
		case v1alpha1.AstraDSVolumeCreated:
			if condition.Status == v1.ConditionFalse {
				volume.CreateError = VolumeCreateError(condition.Message)
			}
		case v1alpha1.AstraDSVolumeOnline:
			if condition.Status == v1.ConditionFalse {
				volume.OnlineError = VolumeOnlineError(condition.Message)
			}
		case v1alpha1.AstraDSVolumeModifyError:
			if condition.Status == v1.ConditionTrue {
				volume.ModifyError = VolumeModifyError(condition.Message)
			}
		case v1alpha1.AstraDSVolumeDeleted:
			if condition.Status == v1.ConditionFalse {
				volume.DeleteError = VolumeDeleteError(condition.Message)
			}
		}
	}

	return volume
}

// getAstraDSVolumeFromVolume converts an internal volume representation to an AstraDSVolume struct.  All values
// are placed into the Spec section of the AstraDSVolume, since we should never be writing to the Status section.
func (c *Clients) getAstraDSVolumeFromVolume(volume *Volume) *v1alpha1.AstraDSVolume {

	return &v1alpha1.AstraDSVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       volumesGVK.Kind,
			APIVersion: volumesGVR.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              volume.Name,
			Namespace:         volume.Namespace,
			ResourceVersion:   volume.ResourceVersion,
			Annotations:       volume.Annotations,
			Labels:            volume.Labels,
			DeletionTimestamp: volume.DeletionTimestamp,
			Finalizers:        volume.Finalizers,
		},
		Spec: v1alpha1.AstraDSVolumeSpec{
			Cluster:                c.cluster.Name,
			Size:                   volume.RequestedSize,
			Type:                   v1alpha1.AstraDSVolumeType(volume.Type),
			ExportPolicy:           volume.ExportPolicy,
			QosPolicy:              volume.QoSPolicy,
			VolumePath:             volume.VolumePath,
			DisplayName:            volume.DisplayName,
			Permissions:            volume.Permissions,
			SnapshotReservePercent: &volume.SnapshotReserve,
			NoSnapDir:              volume.NoSnapDir,
			CloneVolume:            volume.CloneVolume,
			CloneSnapshot:          volume.CloneSnapshot,
		},
	}
}

// Volumes returns all AstraDS volumes.
func (c *Clients) Volumes(ctx context.Context) ([]*Volume, error) {

	unstructuredList, err := c.dynamicClient.Resource(volumesGVR).Namespace(c.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	var astraDSVolumes v1alpha1.AstraDSVolumeList
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredList.UnstructuredContent(), &astraDSVolumes)
	if err != nil {
		return nil, err
	}

	volumes := make([]*Volume, 0)

	for _, adsvo := range astraDSVolumes.Items {
		volumes = append(volumes, c.getVolumeFromAstraDSVolume(&adsvo))
	}

	return volumes, nil
}

// Volume returns the named AstraDS volume.
func (c *Clients) Volume(ctx context.Context, name string) (*Volume, error) {

	unstructuredVolume, err := c.dynamicClient.Resource(volumesGVR).Namespace(c.namespace).Get(ctx, name, getOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, utils.NotFoundError(err.Error())
		}
		return nil, err
	}

	var astraDSVolume v1alpha1.AstraDSVolume
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVolume.UnstructuredContent(), &astraDSVolume)
	if err != nil {
		return nil, err
	}

	return c.getVolumeFromAstraDSVolume(&astraDSVolume), nil
}

// VolumeExists checks whether an AstraDS volume exists, and it returns the volume if so.
func (c *Clients) VolumeExists(ctx context.Context, name string) (bool, *Volume, error) {

	unstructuredVolume, err := c.dynamicClient.Resource(volumesGVR).Namespace(c.namespace).Get(ctx, name, getOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil, nil
		}
		return false, nil, err
	}

	var astraDSVolume v1alpha1.AstraDSVolume
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVolume.UnstructuredContent(), &astraDSVolume)
	if err != nil {
		return false, nil, err
	}

	return true, c.getVolumeFromAstraDSVolume(&astraDSVolume), nil
}

// CreateVolume creates an AstraDS volume.
func (c *Clients) CreateVolume(ctx context.Context, request *Volume) (*Volume, error) {

	// Define the AstraDSVolume object
	newVolume := c.getAstraDSVolumeFromVolume(request)

	// Convert to unstructured map
	unstructuredVolumeMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newVolume)
	if err != nil {
		return nil, err
	}

	// Convert to unstructured object
	unstructuredVolume := &unstructured.Unstructured{
		Object: unstructuredVolumeMap,
	}

	// Create volume
	unstructuredVolume, err = c.dynamicClient.Resource(volumesGVR).Namespace(request.Namespace).Create(ctx, unstructuredVolume, createOpts)
	if err != nil {
		return nil, err
	}

	// Convert returned unstructured object to AstraDSVolume
	var astraDSVolume v1alpha1.AstraDSVolume
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVolume.UnstructuredContent(), &astraDSVolume)
	if err != nil {
		return nil, err
	}

	// Convert to intermediate volume struct
	return c.getVolumeFromAstraDSVolume(&astraDSVolume), nil
}

// SetVolumeAttributes updates one or more attributes of an AstraDS volume, retrying as necessary.
func (c *Clients) SetVolumeAttributes(ctx context.Context, volume *Volume, updateFlags *roaring.Bitmap) error {

	setAttrs := func() error {

		// Get the volume so we always modify the latest version
		unstructuredVolume, err := c.dynamicClient.Resource(volumesGVR).Namespace(c.namespace).Get(ctx, volume.Name, getOpts)
		if err != nil {
			if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				return backoff.Permanent(err)
			}
			return err
		}

		var latestVolume v1alpha1.AstraDSVolume
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVolume.UnstructuredContent(), &latestVolume)
		if err != nil {
			return err
		}

		// Replace the flagged values on the latest copy
		if updateFlags.Contains(UpdateFlagAnnotations) {
			latestVolume.Annotations = volume.Annotations
		}
		if updateFlags.Contains(UpdateFlagExportPolicy) {
			latestVolume.Spec.ExportPolicy = volume.ExportPolicy
		}
		if updateFlags.Contains(UpdateFlagRequestedSize) {
			latestVolume.Spec.Size = volume.RequestedSize
		}

		// Update collections by modifying them on the latest copy
		if updateFlags.Contains(UpdateFlagAddFinalizer) {
			latestVolume.Finalizers = c.addFinalizer(latestVolume.Finalizers, TridentVolumeFinalizer)
		}
		if updateFlags.Contains(UpdateFlagRemoveFinalizer) {
			latestVolume.Finalizers = c.removeFinalizer(latestVolume.Finalizers, TridentVolumeFinalizer)
		}

		// Convert to unstructured map
		unstructuredVolumeMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&latestVolume)
		if err != nil {
			return err
		}

		// Convert to unstructured object
		unstructuredVolume = &unstructured.Unstructured{
			Object: unstructuredVolumeMap,
		}

		// Update volume
		unstructuredVolume, err = c.dynamicClient.Resource(volumesGVR).Namespace(volume.Namespace).Update(
			ctx, unstructuredVolume, updateOpts)
		if err != nil {
			if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				return backoff.Permanent(err)
			}
			return err
		}

		return nil
	}

	setAttrsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Could not update volume, retrying.")
	}

	setAttrsBackoff := backoff.NewExponentialBackOff()
	setAttrsBackoff.MaxElapsedTime = 10 * time.Second
	setAttrsBackoff.MaxInterval = 2 * time.Second
	setAttrsBackoff.RandomizationFactor = 0.1
	setAttrsBackoff.InitialInterval = backoff.DefaultInitialInterval
	setAttrsBackoff.Multiplier = 1.414

	Logc(ctx).WithField("name", volume.Name).Info("Updating volume.")

	if err := backoff.RetryNotify(setAttrs, setAttrsBackoff, setAttrsNotify); err != nil {
		if permanentErr, ok := err.(*backoff.PermanentError); ok {
			Logc(ctx).Errorf("Volume was not updated; %v", permanentErr)
		} else {
			Logc(ctx).Errorf("Volume was updated after %3.2f seconds.", setAttrsBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("name", volume.Name).Debug("Volume updated.")

	return nil
}

// DeleteVolume deletes an AstraDS volume.
func (c *Clients) DeleteVolume(ctx context.Context, volume *Volume) error {

	err := c.dynamicClient.Resource(volumesGVR).Namespace(volume.Namespace).Delete(ctx, volume.Name, deleteOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil
		} else {
			return err
		}
	}

	return nil
}

// removeFinalizer accepts a slice of finalizers and returns a new slice with the specified finalizer added.
func (c *Clients) addFinalizer(finalizers []string, value string) []string {

	// Prevent duplicate values using a map
	finalizerMap := make(map[string]bool)
	for _, finalizer := range finalizers {
		finalizerMap[finalizer] = true
	}

	finalizerMap[value] = true

	finalizers = make([]string, 0)
	for finalizer := range finalizerMap {
		finalizers = append(finalizers, finalizer)
	}

	return finalizers
}

// removeFinalizer accepts a slice of finalizers and returns a new slice with the specified finalizer removed.
func (c *Clients) removeFinalizer(finalizers []string, value string) []string {

	// Prevent duplicate values using a map
	finalizerMap := make(map[string]bool)
	for _, finalizer := range finalizers {
		finalizerMap[finalizer] = true
	}

	delete(finalizerMap, value)

	finalizers = make([]string, 0)
	for finalizer := range finalizerMap {
		finalizers = append(finalizers, finalizer)
	}

	return finalizers
}

// WaitForVolumeReady uses a backoff retry loop to wait for an ADS volume to be fully created.
func (c *Clients) WaitForVolumeReady(ctx context.Context, volume *Volume, maxElapsedTime time.Duration) error {

	logFields := log.Fields{"volume": volume.Name}

	checkVolumeReady := func() error {

		v, err := c.Volume(ctx, volume.Name)
		if err != nil {
			if utils.IsNotFoundError(err) {
				return backoff.Permanent(err)
			}
			return fmt.Errorf("could not get volume %s; %v", volume.Name, err)
		}

		// If the creation failed, return a permanent error to stop waiting
		if creationErr := v.GetCreationError(); creationErr != nil {
			return backoff.Permanent(creationErr)
		}

		if !v.DeletionTimestamp.IsZero() {
			return backoff.Permanent(fmt.Errorf("volume %s is deleting", volume.Name))
		}

		if !v.IsReady(ctx) {
			return utils.VolumeCreatingError(fmt.Sprintf("volume %s is not yet ready", volume.Name))
		}

		return nil
	}

	volumeReadyNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(logFields).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for volume readiness.")
	}

	volumeReadyBackoff := backoff.NewExponentialBackOff()
	volumeReadyBackoff.MaxElapsedTime = maxElapsedTime
	volumeReadyBackoff.MaxInterval = 5 * time.Second
	volumeReadyBackoff.RandomizationFactor = 0.1
	volumeReadyBackoff.InitialInterval = backoff.DefaultInitialInterval
	volumeReadyBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Info("Waiting for volume to become ready.")

	if err := backoff.RetryNotify(checkVolumeReady, volumeReadyBackoff, volumeReadyNotify); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Volume was not ready after %3.2f seconds.",
			volumeReadyBackoff.GetElapsedTime().Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume ready.")

	return nil
}

// WaitForVolumeDeleted uses a backoff retry loop to wait for a deleted ADS volume to disappear, indicating
// that Firetap reclaimed the underlying storage and ADS removed the volume CR finalizer.
func (c *Clients) WaitForVolumeDeleted(ctx context.Context, volume *Volume, maxElapsedTime time.Duration) error {

	logFields := log.Fields{"volume": volume.Name}

	checkVolumeDeleted := func() error {

		volumeExists, extantVolume, err := c.VolumeExists(ctx, volume.Name)
		if err != nil {
			return fmt.Errorf("could not check if volume %s exists; %v", volume.Name, err)
		}

		if volumeExists {
			if IsVolumeDeleteError(extantVolume.DeleteError) {
				return backoff.Permanent(extantVolume.DeleteError)
			}
			return fmt.Errorf("volume %s still exists", volume.Name)
		}

		return nil
	}

	volumeDeletedNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(logFields).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for volume deletion.")
	}

	volumeDeletedBackoff := backoff.NewExponentialBackOff()
	volumeDeletedBackoff.MaxElapsedTime = maxElapsedTime
	volumeDeletedBackoff.MaxInterval = 5 * time.Second
	volumeDeletedBackoff.RandomizationFactor = 0.1
	volumeDeletedBackoff.InitialInterval = backoff.DefaultInitialInterval
	volumeDeletedBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Info("Waiting for volume to be deleted.")

	if err := backoff.RetryNotify(checkVolumeDeleted, volumeDeletedBackoff, volumeDeletedNotify); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Volume was not deleted after %3.2f seconds.",
			volumeDeletedBackoff.GetElapsedTime().Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume deleted.")

	return nil
}

// WaitForVolumeResize uses a backoff retry loop to wait for an ADS volume to become the requested size,
// or report an error indicating that a resize operation failed.
func (c *Clients) WaitForVolumeResize(
	ctx context.Context, volume *Volume, newRequestedSize int64, maxElapsedTime time.Duration,
) error {

	logFields := log.Fields{"volume": volume.Name}

	checkVolumeResize := func() error {

		volumeExists, extantVolume, err := c.VolumeExists(ctx, volume.Name)
		if err != nil {
			return fmt.Errorf("could not check if volume %s exists; %v", volume.Name, err)
		} else if !volumeExists {
			return backoff.Permanent(fmt.Errorf("volume %s does not exist", volume.Name))
		}

		// If the resize failed, return a permanent error to stop waiting
		if IsVolumeModifyError(extantVolume.ModifyError) {
			return backoff.Permanent(extantVolume.ModifyError)
		}

		// Get the size the volume should already be
		currentRequestedSize, err := extantVolume.GetRequestedSize(ctx)
		if err != nil {
			return fmt.Errorf("could not get requested volume size %s; %v", extantVolume.RequestedSize.String(), err)
		}

		// Get the actual size, which may be slightly larger than the requested size
		currentActualSize, err := extantVolume.GetActualSize(ctx)
		if err != nil {
			return fmt.Errorf("could not get actual volume size %s; %v", extantVolume.ActualSize.String(), err)
		}

		// Ensure the requested size has been acknowledged
		if currentRequestedSize != newRequestedSize {
			return fmt.Errorf("volume %s not resized, new requested %d, current requested %d",
				volume.Name, newRequestedSize, currentRequestedSize)
		}

		// Ensure the requested size has been fully honored
		if currentActualSize < currentRequestedSize {
			return fmt.Errorf("volume %s not resized, new requested %d, current actual %d",
				volume.Name, newRequestedSize, currentActualSize)
		}

		return nil
	}

	volumeResizeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(logFields).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for volume resize.")
	}

	volumeResizeBackoff := backoff.NewExponentialBackOff()
	volumeResizeBackoff.MaxElapsedTime = maxElapsedTime
	volumeResizeBackoff.MaxInterval = 5 * time.Second
	volumeResizeBackoff.RandomizationFactor = 0.1
	volumeResizeBackoff.InitialInterval = backoff.DefaultInitialInterval
	volumeResizeBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Info("Waiting for volume resize.")

	if err := backoff.RetryNotify(checkVolumeResize, volumeResizeBackoff, volumeResizeNotify); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Volume was not resized after %3.2f seconds.",
			volumeResizeBackoff.GetElapsedTime().Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume resized.")

	return nil
}

// Volume operations END
// //////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////
// Export policy operations BEGIN

// getExportPolicyFromAstraDSExportPolicy converts an AstraDSExportPolicy struct to an internal export policy
// representation.  All values are taken from the status section of the AstraDSExportPolicy, since those reflect
// the current state of the export policy.
func (c *Clients) getExportPolicyFromAstraDSExportPolicy(adsep *v1alpha1.AstraDSExportPolicy) *ExportPolicy {

	rules := make([]ExportPolicyRule, 0)

	for _, rule := range adsep.Spec.Rules {

		protocols := make([]string, 0)
		for _, protocol := range rule.Protocols {
			protocols = append(protocols, string(protocol))
		}

		roRules := make([]string, 0)
		for _, roRule := range rule.RoRules {
			roRules = append(roRules, string(roRule))
		}

		rwRules := make([]string, 0)
		for _, rwRule := range rule.RwRules {
			rwRules = append(rwRules, string(rwRule))
		}

		superUser := make([]string, 0)
		for _, su := range rule.SuperUser {
			superUser = append(superUser, string(su))
		}

		rules = append(rules, ExportPolicyRule{
			Clients:   rule.Clients,
			Protocols: protocols,
			RuleIndex: rule.RuleIndex,
			RoRules:   roRules,
			RwRules:   rwRules,
			SuperUser: superUser,
			AnonUser:  rule.AnonUser,
		})
	}

	exportPolicy := &ExportPolicy{
		Name:              adsep.Name,
		Namespace:         adsep.Namespace,
		ResourceVersion:   adsep.ResourceVersion,
		Annotations:       adsep.Annotations,
		Labels:            adsep.Labels,
		DeletionTimestamp: adsep.DeletionTimestamp,
		Finalizers:        adsep.Finalizers,
		Rules:             rules,
	}

	for _, condition := range adsep.Status.Conditions {
		switch condition.Type {
		case v1alpha1.NetAppExportPolicyCreated:
			if condition.Status == v1.ConditionFalse {
				exportPolicy.CreateError = ExportPolicyCreateError(condition.Message)
			}
		}
	}

	return exportPolicy
}

// getAstraDSExportPolicyFromExportPolicy converts an internal export policy representation
// to an AstraDSExportPolicy struct.  All values are placed into the Spec section of the
// AstraDSExportPolicy, since we should never be writing to the Status section.
func (c *Clients) getAstraDSExportPolicyFromExportPolicy(exportPolicy *ExportPolicy) *v1alpha1.AstraDSExportPolicy {

	return &v1alpha1.AstraDSExportPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       exportPoliciesGVK.Kind,
			APIVersion: exportPoliciesGVR.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              exportPolicy.Name,
			Namespace:         exportPolicy.Namespace,
			ResourceVersion:   exportPolicy.ResourceVersion,
			Annotations:       exportPolicy.Annotations,
			Labels:            exportPolicy.Labels,
			DeletionTimestamp: exportPolicy.DeletionTimestamp,
			Finalizers:        exportPolicy.Finalizers,
		},
		Spec: v1alpha1.AstraDSExportPolicySpec{
			Rules:   c.getAstraDSExportPolicyRulesFromExportPolicyRules(exportPolicy),
			Cluster: c.cluster.Name,
		},
	}
}

func (c *Clients) getAstraDSExportPolicyRulesFromExportPolicyRules(
	exportPolicy *ExportPolicy,
) v1alpha1.AstraDSExportPolicyRules {

	rules := make(v1alpha1.AstraDSExportPolicyRules, 0)

	for _, rule := range exportPolicy.Rules {

		protocols := make([]v1alpha1.Protocol, 0)
		for _, protocol := range rule.Protocols {
			protocols = append(protocols, v1alpha1.Protocol(protocol))
		}

		roRules := make([]v1alpha1.RoRule, 0)
		for _, roRule := range rule.RoRules {
			roRules = append(roRules, v1alpha1.RoRule(roRule))
		}

		rwRules := make([]v1alpha1.RwRule, 0)
		for _, rwRule := range rule.RwRules {
			rwRules = append(rwRules, v1alpha1.RwRule(rwRule))
		}

		superUser := make([]v1alpha1.SuperUser, 0)
		for _, su := range rule.SuperUser {
			superUser = append(superUser, v1alpha1.SuperUser(su))
		}

		rules = append(rules, v1alpha1.AstraDSExportPolicyRule{
			Clients:   rule.Clients,
			Protocols: protocols,
			RuleIndex: rule.RuleIndex,
			RoRules:   roRules,
			RwRules:   rwRules,
			SuperUser: superUser,
			AnonUser:  rule.AnonUser,
		})
	}

	return rules
}

// getExportPolicy returns the named AstraDS export policy.
func (c *Clients) getExportPolicy(ctx context.Context, name string) (*ExportPolicy, error) {

	unstructuredExportPolicy, err := c.dynamicClient.Resource(exportPoliciesGVR).Namespace(c.namespace).Get(ctx, name, getOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, utils.NotFoundError(err.Error())
		}
		return nil, err
	}

	var astraDSExportPolicy v1alpha1.AstraDSExportPolicy
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredExportPolicy.UnstructuredContent(), &astraDSExportPolicy)
	if err != nil {
		return nil, err
	}

	return c.getExportPolicyFromAstraDSExportPolicy(&astraDSExportPolicy), nil
}

// ExportPolicyExists checks whether an AstraDS export policy exists, and it returns the export policy if so.
func (c *Clients) ExportPolicyExists(ctx context.Context, name string) (bool, *ExportPolicy, error) {

	if exportPolicy, err := c.getExportPolicy(ctx, name); err == nil {
		return true, exportPolicy, nil
	} else if utils.IsNotFoundError(err) {
		return false, nil, nil
	} else {
		return false, nil, err
	}
}

// EnsureExportPolicyExists checks whether an AstraDS export policy exists, and it returns the export policy if so.
func (c *Clients) EnsureExportPolicyExists(ctx context.Context, name string) (*ExportPolicy, error) {

	if exists, exportPolicy, err := c.ExportPolicyExists(ctx, name); err != nil {
		return nil, err
	} else if exists {
		return exportPolicy, nil
	}

	// Policy does not exist, so create it
	exportPolicy, err := c.createExportPolicy(ctx, name)
	if err != nil {
		return nil, err
	}

	if err = c.waitForExportPolicyReady(ctx, exportPolicy, exportPolicyCreateTimeout); err != nil {
		// If we failed to create an export policy, clean up the export policy CR
		if deleteErr := c.DeleteExportPolicy(ctx, name); deleteErr != nil {
			Logc(ctx).WithField("name", name).WithError(deleteErr).Error("Could not delete failed export policy.")
		} else {
			Logc(ctx).WithField("name", name).Warning("Deleted failed export policy.")
		}
		return nil, err
	}

	// The creation succeeded, so place a finalizer on the export policy
	if err = c.SetExportPolicyAttributes(ctx, exportPolicy, roaring.BitmapOf(UpdateFlagAddFinalizer)); err != nil {
		Logc(ctx).WithField("exportPolicy", exportPolicy.Name).WithError(err).Error(
			"Could not add finalizer to export policy.")
	}

	return c.getExportPolicy(ctx, name)
}

// createExportPolicy creates an AstraDS export policy.
func (c *Clients) createExportPolicy(ctx context.Context, name string) (*ExportPolicy, error) {

	// Define the AstraDSExportPolicy object
	exportPolicy := &v1alpha1.AstraDSExportPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       exportPoliciesGVK.Kind,
			APIVersion: exportPoliciesGVR.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
		},
		Spec: v1alpha1.AstraDSExportPolicySpec{
			Rules:   make(v1alpha1.AstraDSExportPolicyRules, 0),
			Cluster: c.cluster.Name,
		},
	}

	// Convert to unstructured map
	unstructuredExportPolicyMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(exportPolicy)
	if err != nil {
		return nil, err
	}

	// Convert to unstructured object
	unstructuredExportPolicy := &unstructured.Unstructured{
		Object: unstructuredExportPolicyMap,
	}

	// Create export policy
	unstructuredExportPolicy, err = c.dynamicClient.Resource(exportPoliciesGVR).Namespace(c.namespace).Create(ctx, unstructuredExportPolicy, createOpts)
	if err != nil {
		return nil, err
	}

	// Convert returned unstructured object to AstraDSExportPolicy
	var astraDSExportPolicy v1alpha1.AstraDSExportPolicy
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredExportPolicy.UnstructuredContent(), &astraDSExportPolicy)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithField("name", name).Info("Created export policy.")

	// Convert to intermediate export policy struct
	return c.getExportPolicyFromAstraDSExportPolicy(&astraDSExportPolicy), nil
}

// SetExportPolicyAttributes updates one or more attributes of an AstraDS export policy, retrying as necessary.
func (c *Clients) SetExportPolicyAttributes(
	ctx context.Context, exportPolicy *ExportPolicy, updateFlags *roaring.Bitmap,
) error {

	setAttrs := func() error {

		// Get the export policy so we always modify the latest version
		unstructuredExportPolicy, err := c.dynamicClient.Resource(exportPoliciesGVR).Namespace(c.namespace).Get(ctx, exportPolicy.Name, getOpts)
		if err != nil {
			if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				return backoff.Permanent(err)
			}
			return err
		}

		var latestExportPolicy v1alpha1.AstraDSExportPolicy
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredExportPolicy.UnstructuredContent(), &latestExportPolicy)
		if err != nil {
			return err
		}

		// Replace the flagged values on the latest copy
		if updateFlags.Contains(UpdateFlagExportRules) {
			latestExportPolicy.Spec.Rules = c.getAstraDSExportPolicyRulesFromExportPolicyRules(exportPolicy)
		}

		// Update collections by modifying them on the latest copy
		if updateFlags.Contains(UpdateFlagAddFinalizer) {
			latestExportPolicy.Finalizers = c.addFinalizer(latestExportPolicy.Finalizers, TridentExportPolicyFinalizer)
		}
		if updateFlags.Contains(UpdateFlagRemoveFinalizer) {
			latestExportPolicy.Finalizers = c.removeFinalizer(latestExportPolicy.Finalizers, TridentExportPolicyFinalizer)
		}

		// Convert to unstructured map
		unstructuredExportPolicyMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&latestExportPolicy)
		if err != nil {
			return err
		}

		// Convert to unstructured object
		unstructuredExportPolicy = &unstructured.Unstructured{
			Object: unstructuredExportPolicyMap,
		}

		// Update export policy
		unstructuredExportPolicy, err = c.dynamicClient.Resource(exportPoliciesGVR).Namespace(exportPolicy.Namespace).Update(
			ctx, unstructuredExportPolicy, updateOpts)
		if err != nil {
			if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				return backoff.Permanent(err)
			}
			return err
		}

		return nil
	}

	setAttrsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Could not update export policy, retrying.")
	}

	setAttrsBackoff := backoff.NewExponentialBackOff()
	setAttrsBackoff.MaxElapsedTime = 10 * time.Second
	setAttrsBackoff.MaxInterval = 2 * time.Second
	setAttrsBackoff.RandomizationFactor = 0.1
	setAttrsBackoff.InitialInterval = backoff.DefaultInitialInterval
	setAttrsBackoff.Multiplier = 1.414

	Logc(ctx).WithField("name", exportPolicy.Name).Info("Updating export policy.")

	if err := backoff.RetryNotify(setAttrs, setAttrsBackoff, setAttrsNotify); err != nil {
		if permanentErr, ok := err.(*backoff.PermanentError); ok {
			Logc(ctx).Errorf("Export policy was not updated; %v", permanentErr)
		} else {
			Logc(ctx).Errorf("Export policy was not updated after %3.2f seconds.",
				setAttrsBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("name", exportPolicy.Name).Debug("Export policy updated.")

	return nil
}

// DeleteExportPolicy deletes an AstraDS export policy.
func (c *Clients) DeleteExportPolicy(ctx context.Context, name string) error {

	err := c.dynamicClient.Resource(exportPoliciesGVR).Namespace(c.namespace).Delete(ctx, name, deleteOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil
		} else {
			return err
		}
	}

	return nil
}

// waitForExportPolicyReady uses a backoff retry loop to wait for an ADS export policy to be fully created.
func (c *Clients) waitForExportPolicyReady(
	ctx context.Context, exportPolicy *ExportPolicy, maxElapsedTime time.Duration,
) error {

	logFields := log.Fields{"name": exportPolicy.Name}

	checkExportPolicyReady := func() error {

		ep, err := c.getExportPolicy(ctx, exportPolicy.Name)
		if err != nil {
			if utils.IsNotFoundError(err) {
				return backoff.Permanent(err)
			}
			return fmt.Errorf("could not get export policy %s; %v", exportPolicy.Name, err)
		}

		// If the creation failed, return a permanent error to stop waiting
		if creationErr := ep.GetCreationError(); creationErr != nil {
			return backoff.Permanent(creationErr)
		}

		if !ep.DeletionTimestamp.IsZero() {
			return backoff.Permanent(fmt.Errorf("export policy %s is deleting", exportPolicy.Name))
		}

		if !ep.IsReady(ctx) {
			return fmt.Errorf(fmt.Sprintf("export policy %s is not yet ready", exportPolicy.Name))
		}

		return nil
	}

	exportPolicyReadyNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(logFields).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for export policy readiness.")
	}

	exportPolicyReadyBackoff := backoff.NewExponentialBackOff()
	exportPolicyReadyBackoff.MaxElapsedTime = maxElapsedTime
	exportPolicyReadyBackoff.MaxInterval = 5 * time.Second
	exportPolicyReadyBackoff.RandomizationFactor = 0.1
	exportPolicyReadyBackoff.InitialInterval = backoff.DefaultInitialInterval
	exportPolicyReadyBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Info("Waiting for export policy to become ready.")

	err := backoff.RetryNotify(checkExportPolicyReady, exportPolicyReadyBackoff, exportPolicyReadyNotify)
	if err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Export policy was not ready after %3.2f seconds.",
			exportPolicyReadyBackoff.GetElapsedTime().Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Export policy ready.")

	return nil
}

// Export policy operations END
// //////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////
// Snapshot operations BEGIN

// getSnapshotFromAstraDSSnapshot converts an AstraDSVolumeSnapshot struct to an internal snapshot representation.
// All values are taken from the status section of the AstraDSVolumeSnapshot, since those reflect the current state
// of the snapshot.
func (c *Clients) getSnapshotFromAstraDSSnapshot(adsvs *v1alpha1.AstraDSVolumeSnapshot) *Snapshot {

	snapshot := &Snapshot{
		Name:              adsvs.Name,
		Namespace:         adsvs.Namespace,
		ResourceVersion:   adsvs.ResourceVersion,
		Annotations:       adsvs.Annotations,
		Labels:            adsvs.Labels,
		DeletionTimestamp: adsvs.DeletionTimestamp,
		Finalizers:        adsvs.Finalizers,

		VolumeName:   adsvs.Status.VolumeName,
		CreationTime: adsvs.Status.CreationTime,
		VolumeUUID:   adsvs.Status.VolumeUUID,
		ReadyToUse:   adsvs.Status.ReadyToUse,
	}

	for _, condition := range adsvs.Status.Conditions {
		switch condition.Type {
		case v1alpha1.AstraDSVolumeSnapshotReady:
			if condition.Status == v1.ConditionFalse {
				snapshot.ReadyError = SnapshotReadyError(condition.Message)
			}
		}
	}

	return snapshot
}

// getAstraDSSnapshotFromSnapshot converts an internal snapshot representation to an AstraDSVolumeSnapshot struct.
// All values are placed into the Spec section of the AstraDSVolumeSnapshot, since we should never be writing to
// the Status section.
func (c *Clients) getAstraDSSnapshotFromSnapshot(snapshot *Snapshot) *v1alpha1.AstraDSVolumeSnapshot {

	return &v1alpha1.AstraDSVolumeSnapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       snapshotsGVK.Kind,
			APIVersion: snapshotsGVR.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              snapshot.Name,
			Namespace:         snapshot.Namespace,
			ResourceVersion:   snapshot.ResourceVersion,
			Annotations:       snapshot.Annotations,
			Labels:            snapshot.Labels,
			DeletionTimestamp: snapshot.DeletionTimestamp,
			Finalizers:        snapshot.Finalizers,
		},
		Spec: v1alpha1.AstraDSVolumeSnapshotSpec{
			VolumeName: snapshot.VolumeName,
			Cluster:    c.cluster.Name,
		},
	}
}

// Snapshots returns all AstraDS snapshots for a volume.
func (c *Clients) Snapshots(ctx context.Context, volume *Volume) ([]*Snapshot, error) {

	// Restrict snapshots to the specified volume
	snapshotListOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", astraDSVolumeUUIDKey, volume.VolumeUUID),
	}

	unstructuredList, err := c.dynamicClient.Resource(snapshotsGVR).Namespace(c.namespace).List(ctx, snapshotListOpts)
	if err != nil {
		return nil, err
	}

	var astraDSSnapshots v1alpha1.AstraDSVolumeSnapshotList
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredList.UnstructuredContent(), &astraDSSnapshots)
	if err != nil {
		return nil, err
	}

	snapshots := make([]*Snapshot, 0)

	for _, adsvs := range astraDSSnapshots.Items {
		snapshots = append(snapshots, c.getSnapshotFromAstraDSSnapshot(&adsvs))
	}

	return snapshots, nil
}

// Snapshot returns the named AstraDS snapshot.
func (c *Clients) Snapshot(ctx context.Context, name string) (*Snapshot, error) {

	unstructuredSnapshot, err := c.dynamicClient.Resource(snapshotsGVR).Namespace(c.namespace).Get(ctx, name, getOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, utils.NotFoundError(err.Error())
		}
		return nil, err
	}

	var astraDSVolumeSnapshot v1alpha1.AstraDSVolumeSnapshot
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredSnapshot.UnstructuredContent(), &astraDSVolumeSnapshot)
	if err != nil {
		return nil, err
	}

	return c.getSnapshotFromAstraDSSnapshot(&astraDSVolumeSnapshot), nil
}

// SnapshotExists checks whether an AstraDS snapshot exists, and it returns the snapshot if so.
func (c *Clients) SnapshotExists(ctx context.Context, name string) (bool, *Snapshot, error) {

	unstructuredSnapshot, err := c.dynamicClient.Resource(snapshotsGVR).Namespace(c.namespace).Get(ctx, name, getOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil, nil
		}
		return false, nil, err
	}

	var astraDSVolumeSnapshot v1alpha1.AstraDSVolumeSnapshot
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredSnapshot.UnstructuredContent(), &astraDSVolumeSnapshot)
	if err != nil {
		return false, nil, err
	}

	return true, c.getSnapshotFromAstraDSSnapshot(&astraDSVolumeSnapshot), nil
}

// CreateSnapshot creates an AstraDS snapshot.
func (c *Clients) CreateSnapshot(ctx context.Context, request *Snapshot) (*Snapshot, error) {

	// Define the AstraDSVolumeSnapshot object
	newSnapshot := c.getAstraDSSnapshotFromSnapshot(request)

	// Convert to unstructured map
	unstructuredSnapshotMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newSnapshot)
	if err != nil {
		return nil, err
	}

	// Convert to unstructured object
	unstructuredSnapshot := &unstructured.Unstructured{
		Object: unstructuredSnapshotMap,
	}

	// Create snapshot
	unstructuredSnapshot, err = c.dynamicClient.Resource(snapshotsGVR).Namespace(request.Namespace).Create(ctx, unstructuredSnapshot, createOpts)
	if err != nil {
		return nil, err
	}

	// Convert returned unstructured object to AstraDSVolumeSnapshot
	var astraDSVolumeSnapshot v1alpha1.AstraDSVolumeSnapshot
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredSnapshot.UnstructuredContent(), &astraDSVolumeSnapshot)
	if err != nil {
		return nil, err
	}

	// Convert to intermediate snapshot struct
	return c.getSnapshotFromAstraDSSnapshot(&astraDSVolumeSnapshot), nil
}

// SetSnapshotAttributes updates one or more attributes of an AstraDS snapshot, retrying as necessary.
func (c *Clients) SetSnapshotAttributes(
	ctx context.Context, snapshot *Snapshot, updateFlags *roaring.Bitmap,
) error {

	setAttrs := func() error {

		// Get the snapshot so we always modify the latest version
		unstructuredSnapshot, err := c.dynamicClient.Resource(snapshotsGVR).Namespace(c.namespace).Get(ctx, snapshot.Name, getOpts)
		if err != nil {
			if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				return backoff.Permanent(err)
			}
			return err
		}

		var latestSnapshot v1alpha1.AstraDSVolumeSnapshot
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredSnapshot.UnstructuredContent(), &latestSnapshot)
		if err != nil {
			return err
		}

		// Update collections by modifying them on the latest copy
		if updateFlags.Contains(UpdateFlagAddFinalizer) {
			latestSnapshot.Finalizers = c.addFinalizer(latestSnapshot.Finalizers, TridentSnapshotFinalizer)
		}
		if updateFlags.Contains(UpdateFlagRemoveFinalizer) {
			latestSnapshot.Finalizers = c.removeFinalizer(latestSnapshot.Finalizers, TridentSnapshotFinalizer)
		}

		// Convert to unstructured map
		unstructuredSnapshotMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&latestSnapshot)
		if err != nil {
			return err
		}

		// Convert to unstructured object
		unstructuredSnapshot = &unstructured.Unstructured{
			Object: unstructuredSnapshotMap,
		}

		// Update snapshot
		unstructuredSnapshot, err = c.dynamicClient.Resource(snapshotsGVR).Namespace(snapshot.Namespace).Update(
			ctx, unstructuredSnapshot, updateOpts)
		if err != nil {
			if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				return backoff.Permanent(err)
			}
			return err
		}

		return nil
	}

	setAttrsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Could not update snapshot, retrying.")
	}

	setAttrsBackoff := backoff.NewExponentialBackOff()
	setAttrsBackoff.MaxElapsedTime = 10 * time.Second
	setAttrsBackoff.MaxInterval = 2 * time.Second
	setAttrsBackoff.RandomizationFactor = 0.1
	setAttrsBackoff.InitialInterval = backoff.DefaultInitialInterval
	setAttrsBackoff.Multiplier = 1.414

	Logc(ctx).WithField("name", snapshot.Name).Info("Updating snapshot.")

	if err := backoff.RetryNotify(setAttrs, setAttrsBackoff, setAttrsNotify); err != nil {
		if permanentErr, ok := err.(*backoff.PermanentError); ok {
			Logc(ctx).Errorf("Snapshot was not updated; %v", permanentErr)
		} else {
			Logc(ctx).Errorf("Snapshot was not updated after %3.2f seconds.",
				setAttrsBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("name", snapshot.Name).Debug("Snapshot updated.")

	return nil
}

// DeleteSnapshot deletes an AstraDS snapshot.
func (c *Clients) DeleteSnapshot(ctx context.Context, snapshot *Snapshot) error {

	err := c.dynamicClient.Resource(snapshotsGVR).Namespace(snapshot.Namespace).Delete(ctx, snapshot.Name, deleteOpts)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil
		} else {
			return err
		}
	}

	return nil
}

// WaitForSnapshotReady uses a backoff retry loop to wait for an ADS snapshot to report it is ready to use.
func (c *Clients) WaitForSnapshotReady(ctx context.Context, snapshot *Snapshot, maxElapsedTime time.Duration) error {

	logFields := log.Fields{
		"volume":   snapshot.VolumeName,
		"snapshot": snapshot.Name,
	}

	checkSnapshotReady := func() error {

		s, err := c.Snapshot(ctx, snapshot.Name)
		if err != nil {
			if utils.IsNotFoundError(err) {
				return backoff.Permanent(err)
			}
			return fmt.Errorf("could not get snapshot %s of volume %s; %v", snapshot.Name, snapshot.VolumeName, err)
		}

		// If the creation failed, return a permanent error to stop waiting
		if creationErr := s.GetReadyError(); creationErr != nil {
			return backoff.Permanent(creationErr)
		}

		if !s.DeletionTimestamp.IsZero() {
			return backoff.Permanent(fmt.Errorf("snapshot %s of volume %s is deleting",
				snapshot.Name, snapshot.VolumeName))
		}

		if !s.IsReady(ctx) {
			err = fmt.Errorf("snapshot %s of volume %s is not yet ready", snapshot.Name, snapshot.VolumeName)
		}

		return err
	}

	snapshotReadyNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(logFields).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for snapshot readiness.")
	}

	snapshotReadyBackoff := backoff.NewExponentialBackOff()
	snapshotReadyBackoff.MaxElapsedTime = maxElapsedTime
	snapshotReadyBackoff.MaxInterval = 5 * time.Second
	snapshotReadyBackoff.RandomizationFactor = 0.1
	snapshotReadyBackoff.InitialInterval = backoff.DefaultInitialInterval
	snapshotReadyBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Debug("Waiting for snapshot to become ready.")

	if err := backoff.RetryNotify(checkSnapshotReady, snapshotReadyBackoff, snapshotReadyNotify); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Snapshot was not ready after %3.2f seconds.",
			snapshotReadyBackoff.GetElapsedTime().Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Snapshot ready.")

	return nil
}

// WaitForSnapshotDeleted uses a backoff retry loop to wait for a deleted ADS snapshot to disappear, indicating
// that Firetap reclaimed the underlying storage and ADS removed the volume CR finalizer.
func (c *Clients) WaitForSnapshotDeleted(ctx context.Context, snapshot *Snapshot, maxElapsedTime time.Duration) error {

	logFields := log.Fields{
		"volume":   snapshot.VolumeName,
		"snapshot": snapshot.Name,
	}

	checkSnapshotDeleted := func() error {

		snapshotExists, _, err := c.SnapshotExists(ctx, snapshot.Name)
		if err != nil {
			return fmt.Errorf("could not check if snapshot %s of volume %s exists; %v",
				snapshot.Name, snapshot.VolumeName, err)
		}

		if snapshotExists {
			return fmt.Errorf("snapshot %s of volume %s still exists", snapshot.Name, snapshot.VolumeName)
		}

		return nil
	}

	snapshotDeletedNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(logFields).WithFields(log.Fields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for snapshot deletion.")
	}

	snapshotDeletedBackoff := backoff.NewExponentialBackOff()
	snapshotDeletedBackoff.MaxElapsedTime = maxElapsedTime
	snapshotDeletedBackoff.MaxInterval = 5 * time.Second
	snapshotDeletedBackoff.RandomizationFactor = 0.1
	snapshotDeletedBackoff.InitialInterval = backoff.DefaultInitialInterval
	snapshotDeletedBackoff.Multiplier = 1.414

	Logc(ctx).WithFields(logFields).Debug("Waiting for snapshot to be deleted.")

	if err := backoff.RetryNotify(checkSnapshotDeleted, snapshotDeletedBackoff, snapshotDeletedNotify); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Snapshot was not deleted after %3.2f seconds.",
			snapshotDeletedBackoff.GetElapsedTime().Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Snapshot deleted.")

	return nil
}

// Snapshot operations END
// //////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////
// QoS operations BEGIN

// getQosPolicyFromAstraDSQosPolicy converts an AstraDSQosPolicy struct to an internal QoS policy representation.  All
// values are taken from the status section of the AstraDSQosPolicy, since those reflect the current state of the policy.
func (c *Clients) getQosPolicyFromAstraDSQosPolicy(adsqp *v1alpha1.AstraDSQosPolicy) *QosPolicy {

	return &QosPolicy{
		Name:      adsqp.Name,
		Cluster:   adsqp.Status.Cluster,
		MinIOPS:   adsqp.Status.MinIOPS,
		MaxIOPS:   adsqp.Status.MaxIOPS,
		BurstIOPS: adsqp.Status.BurstIOPS,
	}
}

// QosPolicies returns all AstraDS QoS policies.
func (c *Clients) QosPolicies(ctx context.Context) ([]*QosPolicy, error) {

	unstructuredList, err := c.dynamicClient.Resource(qosPoliciesGVR).Namespace(AstraDSNamespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	var astraDSQosPolicies v1alpha1.AstraDSQosPolicyList
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredList.UnstructuredContent(), &astraDSQosPolicies)
	if err != nil {
		return nil, err
	}

	qosPolicies := make([]*QosPolicy, 0)

	for _, adsqp := range astraDSQosPolicies.Items {
		qosPolicies = append(qosPolicies, c.getQosPolicyFromAstraDSQosPolicy(&adsqp))
	}

	return qosPolicies, nil
}

// QoS operations END
// //////////////////////////////////////////////////////////////////////////
