package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ktesting "k8s.io/client-go/testing"

	"github.com/netapp/trident/frontend/crd/controller/indexers"
	k8shelper "github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockindexers "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers"
	mockindexer "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers/indexer"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

func TestGetNodePods(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Create a fake clientset with two pods on the desired node
	pod1, _, _ := getPodPvcAndVolExt("test1", "ns-1", nodeName)
	pod2, _, _ := getPodPvcAndVolExt("test2", "ns-2", nodeName)
	pod3, _, _ := getPodPvcAndVolExt("test3", "ns-3", "otherNode")
	kubeClient := GetTestKubernetesClientset(pod1, pod2, pod3)

	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	podList, err := remediationUtils.GetNodePods(ctx(), nodeName)
	assert.NoError(t, err)
	assert.Len(t, podList, 2)
	assert.Equal(t, pod1.Name, "test1-pod")
	assert.Equal(t, pod2.Name, "test2-pod")
}

func TestGetTridentVolumesOnNode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	kubeClient := GetTestKubernetesClientset()

	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName).
		Return([]*models.VolumePublicationExternal{
			{
				VolumeName: "vol1",
				NodeName:   nodeName,
			},
		}, nil)
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	tVols, err := remediationUtils.GetTridentVolumesOnNode(ctx(), nodeName)
	assert.NoError(t, err)
	assert.Len(t, tVols, 1)
	assert.Equal(t, tVols[0], "vol1")
}

func TestGetPVCtoTvolMap(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName).
		Return([]*models.VolumePublicationExternal{
			{
				VolumeName: "vol1",
				NodeName:   nodeName,
			},
			{
				VolumeName: "vol2",
				NodeName:   nodeName,
			},
		}, nil)
	orchestrator.EXPECT().GetVolume(ctx(), "vol1").
		Return(&storage.VolumeExternal{
			Config: &storage.VolumeConfig{
				Name:        "vol1",
				RequestName: "pvc1",
				Namespace:   "ns1",
			},
		}, nil)
	orchestrator.EXPECT().GetVolume(ctx(), "vol2").
		Return(&storage.VolumeExternal{
			Config: &storage.VolumeConfig{
				Name:        "vol2",
				RequestName: "pvc2",
				Namespace:   "ns2",
			},
		}, nil)

	kubeClient := GetTestKubernetesClientset()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	tvolMap, err := remediationUtils.GetPvcToTvolMap(ctx(), nodeName)
	assert.NoError(t, err)
	assert.Len(t, tvolMap, 2)
	assert.Equal(t, "pvc1", tvolMap["ns1/pvc1"].Config.RequestName)
	assert.Equal(t, "pvc2", tvolMap["ns2/pvc2"].Config.RequestName)
	assert.Equal(t, "ns1", tvolMap["ns1/pvc1"].Config.Namespace)
	assert.Equal(t, "ns2", tvolMap["ns2/pvc2"].Config.Namespace)
	assert.Equal(t, "vol1", tvolMap["ns1/pvc1"].Config.Name)
	assert.Equal(t, "vol2", tvolMap["ns2/pvc2"].Config.Name)
}

// TestIsForceDetachSupported_AllVolumesSupported tests that the function returns true
// when all volumes used by the pod support force detach. This validates the success
// condition for force detach operations during node remediation.
func TestIsForceDetachSupported_AllVolumesSupported(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	kubeClient := GetTestKubernetesClientset()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	pod, pvc, volExternal := getPodPvcAndVolExt("test", "test-namespace", nodeName)
	volExternal.Config.AccessInfo.PublishEnforcement = true

	pvcToTvolMap := map[string]*storage.VolumeExternal{
		fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name): volExternal,
	}

	supported := remediationUtils.isForceDetachSupported(ctx(), pod, pvcToTvolMap)
	assert.True(t, supported)
}

// TestIsForceDetachSupported_VolumeNotSupported tests that the function returns false
// when at least one volume doesn't support force detach. This validates that the
// function correctly identifies when force detach operations are not safe.
func TestIsForceDetachSupported_VolumeNotSupported(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	kubeClient := GetTestKubernetesClientset()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	pod, pvc, volExternal := getPodPvcAndVolExt("test", "test-namespace", nodeName)
	volExternal.Config.AccessInfo.PublishEnforcement = false

	pvcToTvolMap := map[string]*storage.VolumeExternal{
		fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name): volExternal,
	}

	supported := remediationUtils.isForceDetachSupported(ctx(), pod, pvcToTvolMap)
	assert.False(t, supported)
}

func TestIsForceDetachSupported_NonTridentVol(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	kubeClient := GetTestKubernetesClientset()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	pod, _, volExternal := getPodPvcAndVolExt("test", "test-namespace", nodeName)
	volExternal.Config.AccessInfo.PublishEnforcement = false

	pvcToTvolMap := map[string]*storage.VolumeExternal{}

	supported := remediationUtils.isForceDetachSupported(ctx(), pod, pvcToTvolMap)
	assert.False(t, supported)
}

// TestIsForceDetachSupported_PodNoPVCs tests that the function returns false
// when the pod has no PVCs. This validates handling of stateless pods that
// don't require volume operations during force detach.
func TestIsForceDetachSupported_PodNoPVCs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	kubeClient := GetTestKubernetesClientset()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	pod := getPodNoPVC("test", "test-namespace", nodeName)

	pvcToTvolMap := map[string]*storage.VolumeExternal{}
	supported := remediationUtils.isForceDetachSupported(ctx(), pod, pvcToTvolMap)
	assert.False(t, supported)
}

// TestIsForceDetachSupported_MultipleVolumes tests the function with multiple PVCs
// where some support force detach and others don't. This validates that all volumes
// must support force detach for the function to return true.
func TestIsForceDetachSupported_MultipleVolumesMixed(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	kubeClient := GetTestKubernetesClientset()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

	// Create pod with multiple PVCs
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Volumes: []corev1.Volume{
				{
					Name: "vol1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc1",
						},
					},
				},
				{
					Name: "vol2",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc2",
						},
					},
				},
			},
		},
	}

	pvc1 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc1",
			Namespace: "test-namespace",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "vol1",
		},
	}

	pvc2 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc2",
			Namespace: "test-namespace",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "vol2",
		},
	}

	volExternal1 := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name: "vol1",
			AccessInfo: models.VolumeAccessInfo{
				PublishEnforcement: true,
			},
		},
	}

	volExternal2 := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name: "vol2",
			AccessInfo: models.VolumeAccessInfo{
				PublishEnforcement: false,
			},
		},
	}

	pvcToTvolMap := map[string]*storage.VolumeExternal{
		fmt.Sprintf("%s/%s", pvc1.Namespace, pvc1.Name): volExternal1,
		fmt.Sprintf("%s/%s", pvc2.Namespace, pvc2.Name): volExternal2,
	}

	supported := remediationUtils.isForceDetachSupported(ctx(), pod, pvcToTvolMap)
	assert.False(t, supported)
}

// TestGetPodsToDelete tests the getPodsToDelete function with various scenarios
// using table-driven tests to validate pod deletion logic based on annotations
// and force detach support capabilities.
func TestGetPodsToDelete(t *testing.T) {
	testCases := map[string]struct {
		setupPods            func() []*corev1.Pod
		setupPvcToTvolMap    func() map[string]*storage.VolumeExternal
		expectedPodsToDelete []string
		description          string
	}{
		"RetainAnnotation": {
			setupPods: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("retain", "test-namespace", nodeName)
				pod.Annotations = map[string]string{
					k8shelper.AnnPodRemediationPolicyAnnotation: k8shelper.PodRemediationPolicyRetain,
				}
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				_, _, volExternal := getPodPvcAndVolExt("retain", "test-namespace", nodeName)
				volExternal.Config.AccessInfo.PublishEnforcement = true
				return map[string]*storage.VolumeExternal{
					"test-namespace/test-pvc": volExternal,
				}
			},
			expectedPodsToDelete: []string{},
			description:          "pods with retain annotation should be skipped",
		},
		"DeleteAnnotation": {
			setupPods: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("delete", "test-namespace", nodeName)
				pod.Annotations = map[string]string{
					k8shelper.AnnPodRemediationPolicyAnnotation: k8shelper.PodRemediationPolicyDelete,
				}
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				_, _, volExternal := getPodPvcAndVolExt("delete", "test-namespace", nodeName)
				volExternal.Config.AccessInfo.PublishEnforcement = false
				return map[string]*storage.VolumeExternal{
					"test-namespace/test-pvc": volExternal,
				}
			},
			expectedPodsToDelete: []string{"delete-pod"},
			description:          "pods with delete annotation should always be deleted",
		},
		"NoAnnotationForceDetachSupported": {
			setupPods: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("supported", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				_, _, volExternal := getPodPvcAndVolExt("supported", "test-namespace", nodeName)
				volExternal.Config.AccessInfo.PublishEnforcement = true
				return map[string]*storage.VolumeExternal{
					"test-namespace/test-pvc": volExternal,
				}
			},
			expectedPodsToDelete: []string{"supported-pod"},
			description:          "pods without annotation but with force detach support should be deleted",
		},
		"NoAnnotationForceDetachNotSupported": {
			setupPods: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("unsupported-pod", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				_, _, volExternal := getPodPvcAndVolExt("unsupported-pod", "test-namespace", nodeName)
				volExternal.Config.AccessInfo.PublishEnforcement = false
				return map[string]*storage.VolumeExternal{
					"test-namespace/test-pvc": volExternal,
				}
			},
			expectedPodsToDelete: []string{},
			description:          "pods without annotation and no force detach support should be skipped",
		},
		"StatelessPodNoPVCs": {
			setupPods: func() []*corev1.Pod {
				pod := getPodNoPVC("stateless", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				return map[string]*storage.VolumeExternal{}
			},
			expectedPodsToDelete: []string{},
			description:          "stateless pods without PVCs should be skipped",
		},
		"MixedPods": {
			setupPods: func() []*corev1.Pod {
				podRetain, _, _ := getPodPvcAndVolExt("retain", "ns1", nodeName)
				podRetain.Annotations = map[string]string{
					k8shelper.AnnPodRemediationPolicyAnnotation: k8shelper.PodRemediationPolicyRetain,
				}

				podDelete, _, _ := getPodPvcAndVolExt("delete", "ns2", nodeName)
				podDelete.Annotations = map[string]string{
					k8shelper.AnnPodRemediationPolicyAnnotation: k8shelper.PodRemediationPolicyDelete,
				}

				podSupported, _, _ := getPodPvcAndVolExt("supported", "ns3", nodeName)

				podUnsupported, _, _ := getPodPvcAndVolExt("unsupported", "ns4", nodeName)

				podStateless := getPodNoPVC("stateless", "ns5", nodeName)

				return []*corev1.Pod{podRetain, podDelete, podSupported, podUnsupported, podStateless}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				_, _, volExternal1 := getPodPvcAndVolExt("retain", "ns1", nodeName)
				volExternal1.Config.AccessInfo.PublishEnforcement = true

				_, _, volExternal2 := getPodPvcAndVolExt("delete", "ns2", nodeName)
				volExternal2.Config.AccessInfo.PublishEnforcement = false

				_, _, volExternal3 := getPodPvcAndVolExt("supported", "ns3", nodeName)
				volExternal3.Config.AccessInfo.PublishEnforcement = true

				_, _, volExternal4 := getPodPvcAndVolExt("unsupported", "ns4", nodeName)
				volExternal4.Config.AccessInfo.PublishEnforcement = false

				return map[string]*storage.VolumeExternal{
					"ns1/test-pvc": volExternal1,
					"ns2/test-pvc": volExternal2,
					"ns3/test-pvc": volExternal3,
					"ns4/test-pvc": volExternal4,
				}
			},
			expectedPodsToDelete: []string{"delete-pod", "supported-pod"},
			description:          "mixed scenario should only delete pods with delete annotation or force detach support",
		},
		"EmptyPodList": {
			setupPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				return map[string]*storage.VolumeExternal{}
			},
			expectedPodsToDelete: []string{},
			description:          "empty pod list should return no pods to delete",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			kubeClient := GetTestKubernetesClientset()
			remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

			podList := tc.setupPods()
			pvcToTvolMap := tc.setupPvcToTvolMap()

			podsToDelete := remediationUtils.GetPodsToDelete(ctx(), podList, pvcToTvolMap)

			assert.Len(t, podsToDelete, len(tc.expectedPodsToDelete), tc.description)

			if len(tc.expectedPodsToDelete) > 0 {
				podNames := make([]string, len(podsToDelete))
				for i, pod := range podsToDelete {
					podNames[i] = pod.Name
				}

				for _, expectedName := range tc.expectedPodsToDelete {
					assert.Contains(t, podNames, expectedName, tc.description)
				}
			}
		})
	}
}

// TestForceDeletePod tests the forceDeletePod function with various scenarios
// using table-driven tests to validate pod deletion behavior including success
// cases, error handling, and edge cases.
func TestForceDeletePod(t *testing.T) {
	testCases := map[string]struct {
		description     string
		setupPod        func() *corev1.Pod
		setupKubeClient func(*corev1.Pod) kubernetes.Interface
		expectedError   bool
		errorContains   string
	}{
		"SuccessfulPodDeletion": {
			description: "Pod is successfully force deleted with zero grace period",
			setupPod: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				}
			},
			setupKubeClient: func(pod *corev1.Pod) kubernetes.Interface {
				return GetTestKubernetesClientset(pod)
			},
			expectedError: false,
		},
		"PodNotFound": {
			description: "Pod deletion handles not found error gracefully",
			setupPod: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-existent-pod",
						Namespace: "test-namespace",
					},
				}
			},
			setupKubeClient: func(pod *corev1.Pod) kubernetes.Interface {
				return GetTestKubernetesClientset()
			},
			expectedError: false,
		},
		"PodDeletionError": {
			description: "Pod deletion fails with API server error",
			setupPod: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				}
			},
			setupKubeClient: func(pod *corev1.Pod) kubernetes.Interface {
				// Create a clientset that will return an error on delete
				kubeClient := GetTestKubernetesClientset(pod)
				kubeClient.PrependReactor("delete", "pods", func(action ktesting.Action) (handled bool,
					ret runtime.Object, err error,
				) {
					return true, nil, fmt.Errorf("API server error")
				})
				return kubeClient
			},
			expectedError: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			pod := tc.setupPod()
			kubeClient := tc.setupKubeClient(pod)

			remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)

			err := remediationUtils.ForceDeletePod(ctx(), pod)

			if tc.expectedError {
				assert.Error(t, err, tc.description)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains, tc.description)
				}
			} else {
				assert.NoError(t, err, tc.description)
			}
		})
	}
}

func getPodNoPVC(name, namespace, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name:  "mock-container",
					Image: "mock-image",
				},
			},
		},
	}
}

func getPodPvcAndVolExt(namePrefix, namespace, node string) (
	*corev1.Pod, *corev1.PersistentVolumeClaim, *storage.VolumeExternal,
) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namePrefix + "-pod",
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeName: node,
			Containers: []corev1.Container{
				{
					Name:  "mock-container",
					Image: "mock-image",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "mock-mount",
							MountPath: "/test",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: namePrefix + "-pvc",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: namePrefix + "-volume",
		},
	}

	volExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:       namePrefix + "-volume",
			AccessInfo: models.VolumeAccessInfo{},
		},
	}
	return pod, pvc, volExternal
}

func getVolAttachment(name, pvName, nodeName string) *storagev1.VolumeAttachment {
	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: "csi-driver",
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &pvName,
			},
			NodeName: nodeName,
		},
	}
}

func TestDeleteVolumeAttachment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	va := getVolAttachment("test-volume", "test-pv", "test-node")

	type parameters struct {
		expectErr  bool
		kubeClient func() kubernetes.Interface
	}

	tests := map[string]parameters{
		"Delete VA": {
			expectErr: false,
			kubeClient: func() kubernetes.Interface {
				kubeClient := GetTestKubernetesClientset(va)
				return kubeClient
			},
		},
		"VA not found": {
			expectErr: false,
			kubeClient: func() kubernetes.Interface {
				kubeClient := GetTestKubernetesClientset()
				return kubeClient
			},
		},
		"Delete API error": {
			expectErr: true,
			kubeClient: func() kubernetes.Interface {
				kubeClient := GetTestKubernetesClientset()
				kubeClient.PrependReactor("delete", "volumeattachments",
					func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, apierrors.NewBadRequest("mock error")
					})
				return kubeClient
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			kubeClient := params.kubeClient()
			remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, nil)
			err := remediationUtils.DeleteVolumeAttachment(ctx(), va.Name)
			assert.Equal(t, params.expectErr, err != nil)
		})
	}
}

// TestGetVolumeAttachmentsToDelete tests the GetVolumeAttachmentsToDelete function
// with various scenarios to validate VolumeAttachment identification logic.
func TestGetVolumeAttachmentsToDelete(t *testing.T) {
	testCases := map[string]struct {
		description       string
		setupPodsToDelete func() []*corev1.Pod
		setupPvcToTvolMap func() map[string]*storage.VolumeExternal
		setupIndexers     func(*gomock.Controller) indexers.Indexers
		expectedVAMap     map[string]string
		expectedError     bool
		errorContains     string
	}{
		"TridentBackedPVC": {
			description: "Pod with Trident-backed PVC should return correct VolumeAttachment",
			setupPodsToDelete: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("test", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				_, _, volExternal := getPodPvcAndVolExt("test", "test-namespace", nodeName)
				return map[string]*storage.VolumeExternal{
					"test-namespace/test-pvc": volExternal,
				}
			},
			setupIndexers: func(mockCtrl *gomock.Controller) indexers.Indexers {
				mockIndexers := mockindexers.NewMockIndexers(mockCtrl)
				va := getVolAttachment("test-va", "test-volume", nodeName)
				mockVaIndexer := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)
				mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(),
					nodeName).Return([]*storagev1.VolumeAttachment{va}, nil)
				mockIndexers.EXPECT().VolumeAttachmentIndexer().Return(mockVaIndexer)
				return mockIndexers
			},
			expectedVAMap: map[string]string{
				"test-va": "test-volume",
			},
			expectedError: false,
		},
		"NonTridentBackedPVC": {
			description: "Pod with non-Trident PVC should lookup PV name from PVC",
			setupPodsToDelete: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("test", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				return map[string]*storage.VolumeExternal{}
			},
			setupIndexers: func(mockCtrl *gomock.Controller) indexers.Indexers {
				mockIndexers := mockindexers.NewMockIndexers(mockCtrl)
				va := getVolAttachment("non-trident-va", "non-trident-pv", nodeName)
				mockVaIndexer := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)
				mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(),
					nodeName).Return([]*storagev1.VolumeAttachment{va}, nil)
				mockIndexers.EXPECT().VolumeAttachmentIndexer().Return(mockVaIndexer)
				return mockIndexers
			},
			expectedVAMap: map[string]string{
				"non-trident-va": "non-trident-pv",
			},
			expectedError: false,
		},
		"VANotFound": {
			description: "PV with no VA should skip gracefully",
			setupPodsToDelete: func() []*corev1.Pod {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
					Spec: corev1.PodSpec{
						NodeName: nodeName,
						Volumes: []corev1.Volume{
							{
								Name: "test-vol",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "test-pvc",
									},
								},
							},
						},
					},
				}
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				return map[string]*storage.VolumeExternal{}
			},
			setupIndexers: func(mockCtrl *gomock.Controller) indexers.Indexers {
				mockVaIndexer := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)
				mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(),
					nodeName).Return([]*storagev1.VolumeAttachment{}, nil)
				mockIndexers := mockindexers.NewMockIndexers(mockCtrl)
				mockIndexers.EXPECT().VolumeAttachmentIndexer().Return(mockVaIndexer)
				return mockIndexers
			},
			expectedVAMap: map[string]string{},
			expectedError: false,
		},
		"PodWithNoPVCs": {
			description: "Pod without PVCs should return empty map",
			setupPodsToDelete: func() []*corev1.Pod {
				pod := getPodNoPVC("stateless", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				return map[string]*storage.VolumeExternal{}
			},
			setupIndexers: func(mockCtrl *gomock.Controller) indexers.Indexers {
				mockVaIndexer := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)
				mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(),
					nodeName).Return([]*storagev1.VolumeAttachment{}, nil)
				mockIndexers := mockindexers.NewMockIndexers(mockCtrl)
				mockIndexers.EXPECT().VolumeAttachmentIndexer().Return(mockVaIndexer)
				return mockIndexers
			},
			expectedVAMap: map[string]string{},
			expectedError: false,
		},
		"IndexerError": {
			description: "Indexer error should return error",
			setupPodsToDelete: func() []*corev1.Pod {
				pod, _, _ := getPodPvcAndVolExt("test", "test-namespace", nodeName)
				return []*corev1.Pod{pod}
			},
			setupPvcToTvolMap: func() map[string]*storage.VolumeExternal {
				return map[string]*storage.VolumeExternal{}
			},
			setupIndexers: func(mockCtrl *gomock.Controller) indexers.Indexers {
				mockVaIndexer := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)
				mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(),
					nodeName).Return(nil, fmt.Errorf("mock error"))
				mockIndexers := mockindexers.NewMockIndexers(mockCtrl)
				mockIndexers.EXPECT().VolumeAttachmentIndexer().Return(mockVaIndexer)
				return mockIndexers
			},
			expectedVAMap: nil,
			expectedError: true,
			errorContains: "mock error",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Setup test data
			podsToDelete := tc.setupPodsToDelete()
			pvcToTvolMap := tc.setupPvcToTvolMap()
			indexers := tc.setupIndexers(mockCtrl)

			// Create PVCs in the clientset for non-Trident backed volumes
			var kubeObjects []runtime.Object
			for _, pod := range podsToDelete {
				pvcNames := getPodPvcNames(pod)
				for _, pvcName := range pvcNames {
					key := fmt.Sprintf("%s/%s", pod.Namespace, pvcName)
					if _, exists := pvcToTvolMap[key]; !exists {
						// Create PVC for non-Trident volumes
						pvc := &corev1.PersistentVolumeClaim{
							ObjectMeta: metav1.ObjectMeta{
								Name:      pvcName,
								Namespace: pod.Namespace,
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								VolumeName: "non-trident-pv",
							},
						}
						kubeObjects = append(kubeObjects, pvc)
					}
				}
			}

			kubeClient := GetTestKubernetesClientset(kubeObjects...)
			remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, indexers)

			// Execute the function
			vaMap, err := remediationUtils.GetVolumeAttachmentsToDelete(ctx(), podsToDelete, pvcToTvolMap, nodeName)

			// Validate results
			if tc.expectedError {
				assert.Error(t, err, tc.description)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains, tc.description)
				}
			} else {
				assert.NoError(t, err, tc.description)
				assert.Equal(t, tc.expectedVAMap, vaMap, tc.description)
			}
		})
	}
}
