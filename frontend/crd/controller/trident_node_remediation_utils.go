package controller

//go:generate mockgen -destination=../../../mocks/mock_frontend/crd/controller/mock_node_remediation.go github.com/netapp/trident/frontend/crd/controller NodeRemediationUtils

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend/crd/controller/indexers"
	k8shelper "github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/controllers/orchestrator/installer"
	"github.com/netapp/trident/storage"
)

type NodeRemediationUtils interface {
	GetNodePods(ctx context.Context, nodeName string) ([]*corev1.Pod, error)
	GetPvcToTvolMap(ctx context.Context, nodeName string) (map[string]*storage.VolumeExternal, error)
	GetVolumeAttachmentsToDelete(ctx context.Context, podsToDelete []*corev1.Pod,
		pvcToTvols map[string]*storage.VolumeExternal, nodeName string) (map[string]string, error)
	ForceDeletePod(ctx context.Context, pod *corev1.Pod) error
	DeleteVolumeAttachment(ctx context.Context, attachmentName string) error
	GetTridentVolumesOnNode(ctx context.Context, nodeName string) ([]string, error)
	GetPodsToDelete(
		ctx context.Context, nodePods []*corev1.Pod, pvcToTvolMap map[string]*storage.VolumeExternal,
	) []*corev1.Pod
}

type nodeRemediationUtils struct {
	kubeClientset kubernetes.Interface
	orchestrator  core.Orchestrator
	indexers      indexers.Indexers
}

func NewNodeRemediationUtils(
	kubeClientset kubernetes.Interface, orchestrator core.Orchestrator, indexers indexers.Indexers,
) *nodeRemediationUtils {
	return &nodeRemediationUtils{
		kubeClientset: kubeClientset,
		orchestrator:  orchestrator,
		indexers:      indexers,
	}
}

// GetNodePods retrieves all pods on the specified node
func (n *nodeRemediationUtils) GetNodePods(ctx context.Context, nodeName string) ([]*corev1.Pod, error) {
	podList := make([]*corev1.Pod, 0)
	var continueToken string
	for {
		opts := metav1.ListOptions{
			Limit:         listLimit,
			Continue:      continueToken,
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
		}

		pods, err := n.kubeClientset.CoreV1().Pods("").List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for _, pod := range pods.Items {
			// Sanity check. Pod list should be filtered by nodeName already.
			if pod.Spec.NodeName == nodeName {
				podList = append(podList, &pod)
			}
		}

		if pods.Continue == "" {
			break
		}

		continueToken = pods.Continue
	}
	return podList, nil
}

// GetTridentVolumesOnNode returns a list of Trident volume names published on the specified node
func (n *nodeRemediationUtils) GetTridentVolumesOnNode(ctx context.Context, nodeName string) ([]string, error) {
	volList := make([]string, 0)
	publications, err := n.orchestrator.ListVolumePublicationsForNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	for _, pub := range publications {
		if pub.NodeName == nodeName {
			volList = append(volList, pub.VolumeName)
		}
	}
	return volList, err
}

// GetPvcToTvolMap creates a map of "namespace/pvc"-> tvol for all volumes on the node
func (n *nodeRemediationUtils) GetPvcToTvolMap(
	ctx context.Context, nodeName string,
) (map[string]*storage.VolumeExternal, error) {
	tridentVolumesOnNode, err := n.GetTridentVolumesOnNode(ctx, nodeName)
	if err != nil {
		Logc(ctx).WithField("node", nodeName).WithError(err).
			Error("Could not get trident volume publications for node.")
		return nil, err
	}

	pvcToTvolMap := make(map[string]*storage.VolumeExternal)
	for _, volumeName := range tridentVolumesOnNode {
		tvol, err := n.orchestrator.GetVolume(ctx, volumeName)
		if err != nil {
			Logc(ctx).WithError(err).Warnf("Could not get volume %s.", volumeName)
			continue
		}
		pvcName := tvol.Config.RequestName
		if pvcName == "" { // Sanity check, should never be empty
			Logc(ctx).WithField("volume", volumeName).Warn("Volume has no PVC associated with it.")
			continue
		}
		namespace := tvol.Config.Namespace
		if namespace == "" { // Sanity check, should never be empty
			Logc(ctx).WithField("volume", volumeName).Warn("Volume has no namespace associated with it.")
			continue
		}
		key := fmt.Sprintf("%s/%s", namespace, pvcName)
		pvcToTvolMap[key] = tvol
	}
	return pvcToTvolMap, nil
}

// GetVolumeAttachmentsToDelete returns a map of VolumeAttachment names to volume names that should be deleted
func (n *nodeRemediationUtils) GetVolumeAttachmentsToDelete(
	ctx context.Context, podsToDelete []*corev1.Pod, pvcToTvols map[string]*storage.VolumeExternal,
	nodeName string,
) (map[string]string, error) {
	// Map of PV name to VA name
	attachmentsToDeleteMap := map[string]string{}
	pvToVaMap, err := n.getVolAttachmentMap(ctx, nodeName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not get volume attachments.")
		return nil, err
	}

	for _, pod := range podsToDelete {
		pvcNames := getPodPvcNames(pod)
		for _, pvcName := range pvcNames {
			var volName string
			// Check if the associated tvol exists for the volume in use by this PVC
			key := fmt.Sprintf("%s/%s", pod.Namespace, pvcName)
			tvol, exists := pvcToTvols[key]
			if !exists {
				// This might not be a Tridnet backed PVC. Get the PV name from the PVC.
				volName, err = n.getPVNameForPVC(ctx, pvcName, pod.Namespace)
				if err != nil {
					if apierrors.IsNotFound(err) {
						Logc(ctx).WithFields(LogFields{
							"pvc":       pvcName,
							"namespace": pod.Namespace,
						}).Debug("PVC not found, skipping.")
						continue
					} else {
						Logc(ctx).WithFields(LogFields{
							"pvc":       pvcName,
							"namespace": pod.Namespace,
						}).WithError(err).Error("Could not get PVC.")
						return nil, err
					}
				}
			} else {
				volName = tvol.Config.Name
			}
			if volName == "" {
				continue
			}
			// Add the VA to the map. This map will be set in the CR status.
			volAttachemntName, ok := pvToVaMap[volName]
			if !ok {
				Logc(ctx).WithField("volume", volName).Debug("VolumeAttachment not found for volume.")
				continue
			}
			attachmentsToDeleteMap[volAttachemntName.Name] = volName
		}
	}
	return attachmentsToDeleteMap, nil
}

// getVolAttachmentMap creates a map of PV name to VolumeAttachment for all VAs on the specified node
func (n *nodeRemediationUtils) getVolAttachmentMap(
	ctx context.Context, nodeName string,
) (map[string]*storagev1.VolumeAttachment, error) {
	attachemntsOnNode, err := n.indexers.VolumeAttachmentIndexer().GetCachedVolumeAttachmentsByNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	pvToVa := map[string]*storagev1.VolumeAttachment{}
	for _, va := range attachemntsOnNode {
		if va.Spec.Source.PersistentVolumeName != nil {
			pvToVa[*va.Spec.Source.PersistentVolumeName] = va
		}
	}
	return pvToVa, nil
}

// getPVNameForPVC retrieves the PV name for the specified PVC
func (n *nodeRemediationUtils) getPVNameForPVC(ctx context.Context, pvcName, namespace string) (string, error) {
	pvc, err := n.kubeClientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, getOpts)
	if err != nil {
		return "", err
	}
	return pvc.Spec.VolumeName, nil
}

// ForceDeletePod force deletes the specified pod
func (n *nodeRemediationUtils) ForceDeletePod(ctx context.Context, pod *corev1.Pod) error {
	Logc(ctx).WithFields(LogFields{
		"pod":       pod.Name,
		"node":      pod.Spec.NodeName,
		"namespace": pod.Namespace,
	}).Debug("Deleting pod.")
	forceDeleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: new(int64), // Use zero value
	}

	err := n.kubeClientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, forceDeleteOpts)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// deleteVolumeAttachment deletes the specified VolumeAttachment
func (n *nodeRemediationUtils) DeleteVolumeAttachment(
	ctx context.Context, attachmentName string,
) error {
	Logc(ctx).WithFields(LogFields{
		"volumeAttachment": attachmentName,
	}).Debug("Deleting volume attachment.")

	forceDeleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: new(int64), // Use zero value
	}

	err := n.kubeClientset.StorageV1().VolumeAttachments().Delete(
		ctx,
		attachmentName,
		forceDeleteOpts,
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// getPodPvcNames returns a list of PVC names used by the specified pod
func getPodPvcNames(pod *corev1.Pod) []string {
	pvcNames := []string{}
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, vol.PersistentVolumeClaim.ClaimName)
		}
	}
	return pvcNames
}

// podHasPVC checks if the specified pod has any persistent volume claims
func podHasPVC(ctx context.Context, pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

// GetPodsToDelete returns a list of pods to delete on the specified node.
// Pods with the retain annotation are skipped. Pods with the delete annotation are always deleted.
// If no annotation is set, only pods using PVCs that support force detach are deleted.
func (n *nodeRemediationUtils) GetPodsToDelete(
	ctx context.Context, nodePods []*corev1.Pod, pvcToTvolMap map[string]*storage.VolumeExternal,
) []*corev1.Pod {
	podsToDelete := make([]*corev1.Pod, 0)
	for _, pod := range nodePods {
		value, exists := pod.Annotations[k8shelper.AnnPodRemediationPolicyAnnotation]
		if exists {
			// Skip pods with the retain annotation
			if value == k8shelper.PodRemediationPolicyRetain {
				continue
			}

			// Skip Trident controller pods
			if strings.HasPrefix(pod.Name, installer.TridentControllerResourceName) {
				continue
			}

			// Delete pods with the delete annotation
			if value == k8shelper.PodRemediationPolicyDelete {
				podsToDelete = append(podsToDelete, pod)
				continue
			}
		}
		// If no annotation set, only delete pods using PVCs that support force detach.
		// Pods with no PVCs are not deleted.
		if supported := n.isForceDetachSupported(ctx, pod, pvcToTvolMap); supported {
			podsToDelete = append(podsToDelete, pod)
		}
	}
	return podsToDelete
}

// isForceDetachSupported checks if all of the pod's PVCs are backed by volumes that support force detach
func (n *nodeRemediationUtils) isForceDetachSupported(
	ctx context.Context, pod *corev1.Pod, pvcToTvolMap map[string]*storage.VolumeExternal,
) bool {
	if !podHasPVC(ctx, pod) {
		return false
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcName := volume.PersistentVolumeClaim.ClaimName
			key := fmt.Sprintf("%s/%s", pod.Namespace, pvcName)
			tvol, exists := pvcToTvolMap[key]
			if !exists {
				Logc(ctx).WithFields(LogFields{
					"namespace": pod.Namespace,
					"pod":       pod.Name,
					"pvc":       pvcName,
				}).Debug("Non-Trident volume found, pod does not support force detach.")
				return false
			}

			if !tvol.Config.AccessInfo.PublishEnforcement {
				Logc(ctx).WithFields(LogFields{
					"namespace": pod.Namespace,
					"pod":       pod.Name,
					"pvc":       pvcName,
					"volume":    tvol.Config.Name,
				}).Debug("Volume does not support force detach.")
				return false
			}
		}
	}
	return true
}
