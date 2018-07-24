// Copyright 2018 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/cache/testing"
	"k8s.io/client-go/tools/record"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/k8s_client"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	k8sutilversion "github.com/netapp/trident/utils"
)

const (
	testNFSServer = "127.0.0.1"
	testNamespace = "test"
)

type pluginTest struct {
	name                  string
	expectedVols          []*v1.PersistentVolume
	expectedVolumeConfigs []*storage.VolumeConfig
	storageClassConfigs   []*sc.Config
	v1betaStorageClasses  []*k8sstoragev1beta.StorageClass
	v1StorageClasses      []*k8sstoragev1.StorageClass
	protocols             []config.Protocol
	action                testAction
}

type testAction func(
	ctrl *Plugin,
	reactor *orchestratorReactor,
	cs *framework.FakePVCControllerSource,
	vs *framework.FakePVControllerSource,
	ct *pluginTest,
) error

func testVolume(
	name string,
	pvcUID types.UID,
	size string,
	accessModes []v1.PersistentVolumeAccessMode,
	protocol config.Protocol,
	reclaimPolicy v1.PersistentVolumeReclaimPolicy,
	storageClass string,
	kubeVersion *k8sversion.Info,
) *v1.PersistentVolume {
	claimRef := v1.ObjectReference{
		Namespace: testNamespace,
		Name:      name, // Provisioned PVs will have the same name as their PVC
		UID:       pvcUID,
	}
	// This looks like overkill, but it's probably the best way to ensure that
	// the transient claim we're passing into getUniqueClaimName has everything
	// set that it needs to.
	name = getUniqueClaimName(testClaim(name, pvcUID, size, accessModes,
		v1.ClaimPending, map[string]string{}, kubeVersion))
	pv := &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				AnnDynamicallyProvisioned: AnnOrchestrator,
				AnnClass:                  storageClass,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: accessModes,
			Capacity: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse(size),
			},
			ClaimRef:                      &claimRef,
			PersistentVolumeReclaimPolicy: reclaimPolicy,
		},
	}

	version, err := ValidateKubeVersion(kubeVersion)
	if err != nil {
		panic("Invalid Kubernetes version")
	}
	switch {
	case version.AtLeast(k8sutilversion.MustParseSemantic("v1.6.0")):
		pv.Spec.StorageClassName = storageClass
	}

	switch protocol {
	case config.File:
		pv.Spec.NFS = &v1.NFSVolumeSource{
			Server: testNFSServer,
			Path:   "/" + core.GetFakeInternalName(name),
		}
	// TODO:  Support for other backends besides ONTAP.
	default:
		log.Panicf("Protocol %s not implemented!", protocol)
	}
	return pv
}

func testClaim(
	name string,
	uid types.UID,
	size string,
	accessModes []v1.PersistentVolumeAccessMode,
	phase v1.PersistentVolumeClaimPhase,
	annotations map[string]string,
	kubeVersion *k8sversion.Info,
) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   testNamespace,
			UID:         uid,
			Annotations: annotations,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}

	version, err := ValidateKubeVersion(kubeVersion)
	if err != nil {
		panic("Invalid Kubernetes version")
	}
	switch {
	case version.AtLeast(k8sutilversion.MustParseSemantic("v1.6.0")):
		if storageClass, found := annotations[AnnClass]; found {
			pvc.Spec.StorageClassName = &storageClass
		}
	}
	return pvc
}

func testVolumeConfig(
	accessModes []v1.PersistentVolumeAccessMode,
	name string,
	pvcUID types.UID,
	size string,
	annotations map[string]string,
	kubeVersion *k8sversion.Info,
) *storage.VolumeConfig {
	ret := getVolumeConfig(accessModes,
		getUniqueClaimName(testClaim(name, pvcUID, size, accessModes,
			v1.ClaimPending, annotations, kubeVersion)),
		resource.MustParse(size), annotations)
	ret.InternalName = core.GetFakeInternalName(ret.Name)
	ret.AccessInfo.NfsServerIP = testNFSServer
	ret.AccessInfo.NfsPath = fmt.Sprintf("/%s",
		core.GetFakeInternalName(ret.Name))
	return ret
}

func storageClassConfigs(classNames ...string) []*sc.Config {
	ret := make([]*sc.Config, len(classNames))
	for i, name := range classNames {
		ret[i] = &sc.Config{
			Name: name,
		}
	}
	return ret
}

func newTestPlugin(
	orchestrator *core.MockOrchestrator,
	client *fake.Clientset,
	claimSource *framework.FakePVCControllerSource,
	volumeSource *framework.FakePVControllerSource,
	classSource *framework.FakeControllerSource,
	resizeSource *framework.FakePVCControllerSource,
	protocols []config.Protocol,
	kubeVersion *k8sversion.Info,
) (*Plugin, error) {
	ret := &Plugin{
		orchestrator:             orchestrator,
		claimControllerStopChan:  make(chan struct{}),
		volumeControllerStopChan: make(chan struct{}),
		classControllerStopChan:  make(chan struct{}),
		resizeControllerStopChan: make(chan struct{}),
		mutex:                 &sync.Mutex{},
		pendingClaimMatchMap:  make(map[string]*v1.PersistentVolume),
		defaultStorageClasses: make(map[string]bool, 1),
		storageClassCache:     make(map[string]*StorageClassSummary),
	}
	ret.kubernetesVersion = kubeVersion
	ret.claimSource = claimSource
	_, ret.claimController = cache.NewInformer(
		ret.claimSource,
		&v1.PersistentVolumeClaim{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ret.addClaim,
			UpdateFunc: ret.updateClaim,
			DeleteFunc: ret.deleteClaim,
		},
	)
	ret.volumeSource = volumeSource
	_, ret.volumeController = cache.NewInformer(
		ret.volumeSource,
		&v1.PersistentVolume{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ret.addVolume,
			UpdateFunc: ret.updateVolume,
			DeleteFunc: ret.deleteVolume,
		},
	)
	ret.resizeSource = resizeSource
	_, ret.resizeController = cache.NewInformer(
		ret.resizeSource,
		&v1.PersistentVolumeClaim{},
		KubernetesResizeSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: ret.updateClaimResize,
		},
	)
	version, err := ValidateKubeVersion(ret.kubernetesVersion)
	if err != nil {
		return nil, err
	}

	switch {
	case version.AtLeast(k8sutilversion.MustParseSemantic("v1.6.0")):
		ret.classSource = classSource
		_, ret.classController = cache.NewInformer(
			ret.classSource,
			&k8sstoragev1.StorageClass{},
			KubernetesSyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc:    ret.addClass,
				UpdateFunc: ret.updateClass,
				DeleteFunc: ret.deleteClass,
			},
		)
	case version.AtLeast(k8sutilversion.MustParseSemantic("v1.4.0")):
		ret.classSource = classSource
		_, ret.classController = cache.NewInformer(
			ret.classSource,
			&k8sstoragev1beta.StorageClass{},
			KubernetesSyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc:    ret.addClass,
				UpdateFunc: ret.updateClass,
				DeleteFunc: ret.deleteClass,
			},
		)
	}
	ret.kubeClient = client
	ret.getNamespacedKubeClient = k8sclient.NewFakeKubeClientBasic
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(
		&corev1.EventSinkImpl{
			Interface: client.Core().Events(""),
		})
	ret.eventRecorder = broadcaster.NewRecorder(runtime.NewScheme(),
		v1.EventSource{Component: AnnOrchestrator})
	// Note that at the moment we can only actually support NFS here; the
	// iSCSI backends all trigger interactions with a real backend to map
	// newly provisioned LUNs, which won't work in a test environment.
	for _, p := range protocols {
		switch p {
		case config.File:
			orchestrator.AddMockONTAPNFSBackend("nfs", testNFSServer)
		default:
			log.Panic("Unsupported protocol:  ", p)
		}
	}
	return ret, nil
}

/*
Cases to test:
- PVC binds to PV
- Pre-existing PV:  PVC binds to preexisting, newly provisioned gets deleted
- Resized PVC:  Made larger than original request; new PV gets deleted
- Resized PVC:  Made smaller than original request; no change
*/

// Needed for modify events to avoid races.
func cloneClaim(claim *v1.PersistentVolumeClaim) *v1.PersistentVolumeClaim {
	return claim.DeepCopy()
}

func TestVolumeControllerKubeVersion1_5(t *testing.T) {
	kubeVersion := k8sclient.NewFakeKubeClient(nil, "1", "5").Version()
	tests := []pluginTest{
		{
			name: "Basic bind",
			expectedVols: []*v1.PersistentVolume{
				testVolume("basic", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig(
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"basic", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("basic", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "Misplaced bind",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("misplaced", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = "wrongVol"
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name: "Larger resized PVC",
			expectedVols: []*v1.PersistentVolume{
				testVolume("resized-larger", "pvc1", "21M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"resized-larger", "pvc1", "21M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("resized-larger", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.Resources = v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("21M"),
					},
				}
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name: "Smaller resized PVC",
			expectedVols: []*v1.PersistentVolume{
				testVolume("resized-smaller", "pvc1", "40M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"resized-smaller", "pvc1", "40M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("resized-smaller", "pvc1", "40M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.Resources = v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("20M"),
					},
				}
				cs.Modify(claimClone)
				// TODO:  Send a final bound message?
				return nil
			},
		},
		{
			name: "ReadWriteOnceNFS",
			expectedVols: []*v1.PersistentVolume{
				testVolume("readwriteonce-nfs", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					"readwriteonce-nfs", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("readwriteonce-nfs", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "WrongStorageClass",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("wrong-storage-class", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "bronze",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				return nil
			},
		},
		{
			name:                  "DeleteVolumeStandard",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("delete-standard", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				volumeName := getUniqueClaimName(claim)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume probably not created", volumeName)
				}
				// Simulate each possible event:  added while pending, then
				// bound, then released.  Note that the spec should already
				// be set correctly.
				volumeClone.Status.Phase = v1.VolumePending
				vs.Add(volumeClone)
				reactor.wait()
				volumeClone.Status.Phase = v1.VolumeBound
				vs.Modify(volumeClone)

				cs.Delete(claimClone)
				reactor.wait()

				volumeClone = reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeReleased
				vs.Modify(volumeClone)
				return nil
			},
		},
		{
			name:                  "DeleteVolumeMissedAdd",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("delete-missed-add", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				cs.Delete(claimClone)
				reactor.wait()
				// Don't generate the initial add; just do the modification.
				// This gets reflected through as a single add event; simulates
				// the case where the plugin goes down then comes back online
				volumeName := getUniqueClaimName(claim)
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeReleased
				vs.Modify(volumeClone)
				return nil
			},
		},
		{
			name:                  "DeleteFailedVolume",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				// This test roughly simulates what occurs when Trident goes
				// offline and the user deletes a PVC whose PV has the Delete
				// retention policy.  Kubernetes will mark the volume
				// Failed in this case.  We ensure here that Trident still
				// deletes the volume.
				claim := testClaim("delete-failed-volume", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				cs.Delete(claimClone)
				reactor.wait()
				// Don't generate the initial add; just do the modification.
				// This gets reflected through as a single add event; simulates
				// the case where the plugin goes down then comes back online
				volumeName := getUniqueClaimName(claim)
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeFailed
				vs.Modify(volumeClone)
				return nil
			},
		},
		{
			name: "DeleteVolumeRetain",
			expectedVols: []*v1.PersistentVolume{
				testVolume("delete-retain", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					config.File,
					v1.PersistentVolumeReclaimRetain,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					"delete-retain", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("delete-retain", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
						AnnReclaimPolicy: string(
							v1.PersistentVolumeReclaimRetain),
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				cs.Delete(claimClone)
				reactor.wait()
				volumeName := getUniqueClaimName(claim)
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeReleased
				vs.Modify(volumeClone)
				return nil
			},
		},
	}
	for _, test := range tests {
		orchestrator := core.NewMockOrchestrator()
		// Initialize storage classes
		for _, conf := range test.storageClassConfigs {
			_, err := orchestrator.AddStorageClass(conf)
			if err != nil {
				t.Fatalf("Unable to add storage class %s:  %v", conf.Name, err)
			}
		}
		// Initialize the Kubernetes components
		client := &fake.Clientset{}
		cs := framework.NewFakePVCControllerSource()
		volumeSource := framework.NewFakePVControllerSource()
		classSource := framework.NewFakeControllerSource()
		resizeSource := framework.NewFakePVCControllerSource()
		ctrl, err := newTestPlugin(orchestrator, client, cs, volumeSource,
			classSource, resizeSource, test.protocols, kubeVersion)
		if err != nil {
			t.Fatalf("Unable to create the Kubernetes plugin: %v", err)
		}
		reactor := newReactor(client, cs)

		log.WithFields(log.Fields{
			"test": test.name,
		}).Debug("Starting controller.")
		ctrl.Activate()
		err = test.action(ctrl, reactor, cs, volumeSource, &test)
		if err != nil {
			t.Error("Unable to perform action:  ", err)
		}
		reactor.wait()
		ctrl.Deactivate()
		frontendSuccess := reactor.validateVolumes(t, test.expectedVols)
		backendSuccess := orchestrator.ValidateVolumes(t,
			test.expectedVolumeConfigs)
		if !(frontendSuccess && backendSuccess) {
			t.Error("Test failed:  ", test.name)
		}
	}
}

func testStorageClassV1_5(
	name string, useTrident bool, parameters map[string]string,
) *k8sstoragev1beta.StorageClass {
	ret := k8sstoragev1beta.StorageClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StorageClass",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Parameters: parameters,
	}
	if useTrident {
		ret.Provisioner = AnnOrchestrator
	} else {
		ret.Provisioner = "nonexistent.notnetapp.io"
	}
	return &ret
}

func testStorageClassV1_6(
	name string, useTrident, defaultClass bool, parameters map[string]string,
) *k8sstoragev1.StorageClass {
	ret := k8sstoragev1.StorageClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StorageClass",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Parameters: parameters,
	}
	if useTrident {
		ret.Provisioner = AnnOrchestrator
	} else {
		ret.Provisioner = "nonexistent.notnetapp.io"
	}
	if defaultClass {
		ret.Annotations = make(map[string]string)
		ret.Annotations[AnnDefaultStorageClass] = "true"
	}
	return &ret
}

func TestVolumeControllerKubeVersion1_6(t *testing.T) {
	kubeVersion := k8sclient.NewFakeKubeClient(nil, "1", "6").Version()
	tests := []pluginTest{
		{
			name: "Basic bind",
			expectedVols: []*v1.PersistentVolume{
				testVolume("basic", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig(
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"basic", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("basic", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "Misplaced bind",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("misplaced", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = "wrongVol"
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name: "Larger resized PVC",
			expectedVols: []*v1.PersistentVolume{
				testVolume("resized-larger", "pvc1", "21M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"resized-larger", "pvc1", "21M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("resized-larger", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.Resources = v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("21M"),
					},
				}
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name: "Smaller resized PVC",
			expectedVols: []*v1.PersistentVolume{
				testVolume("resized-smaller", "pvc1", "40M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"resized-smaller", "pvc1", "40M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("resized-smaller", "pvc1", "40M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.Resources = v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("20M"),
					},
				}
				cs.Modify(claimClone)
				// TODO:  Send a final bound message?
				return nil
			},
		},
		{
			name: "ReadWriteOnceNFS",
			expectedVols: []*v1.PersistentVolume{
				testVolume("readwriteonce-nfs", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					"readwriteonce-nfs", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("readwriteonce-nfs", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "WrongStorageClass",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("wrong-storage-class", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "bronze",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				return nil
			},
		},
		{
			name:                  "DeleteVolumeStandard",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("delete-standard", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				volumeName := getUniqueClaimName(claim)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume probably not created", volumeName)
				}
				// Simulate each possible event:  added while pending, then
				// bound, then released.  Note that the spec should already
				// be set correctly.
				volumeClone.Status.Phase = v1.VolumePending
				vs.Add(volumeClone)
				reactor.wait()
				volumeClone.Status.Phase = v1.VolumeBound
				vs.Modify(volumeClone)

				cs.Delete(claimClone)
				reactor.wait()

				volumeClone = reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeReleased
				vs.Modify(volumeClone)
				return nil
			},
		},
		{
			name:                  "DeleteVolumeMissedAdd",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("delete-missed-add", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				cs.Delete(claimClone)
				reactor.wait()
				// Don't generate the initial add; just do the modification.
				// This gets reflected through as a single add event; simulates
				// the case where the plugin goes down then comes back online
				volumeName := getUniqueClaimName(claim)
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeReleased
				vs.Modify(volumeClone)
				return nil
			},
		},
		{
			name:                  "DeleteFailedVolume",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			storageClassConfigs:   storageClassConfigs("silver"),
			protocols:             []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				// This test roughly simulates what occurs when Trident goes
				// offline and the user deletes a PVC whose PV has the Delete
				// retention policy.  Kubernetes will mark the volume
				// Failed in this case.  We ensure here that Trident still
				// deletes the volume.
				claim := testClaim("delete-failed-volume", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				cs.Delete(claimClone)
				reactor.wait()
				// Don't generate the initial add; just do the modification.
				// This gets reflected through as a single add event; simulates
				// the case where the plugin goes down then comes back online
				volumeName := getUniqueClaimName(claim)
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeFailed
				vs.Modify(volumeClone)
				return nil
			},
		},
		{
			name: "DeleteVolumeRetain",
			expectedVols: []*v1.PersistentVolume{
				testVolume("delete-retain", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					config.File,
					v1.PersistentVolumeReclaimRetain,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					"delete-retain", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			storageClassConfigs: storageClassConfigs("silver"),
			protocols:           []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				cs *framework.FakePVCControllerSource,
				vs *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("delete-retain", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
						AnnReclaimPolicy: string(
							v1.PersistentVolumeReclaimRetain),
					},
					kubeVersion,
				)
				cs.Add(claim)
				// Prevent event coalescing.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				cs.Modify(claimClone)
				reactor.wait()
				cs.Delete(claimClone)
				reactor.wait()
				volumeName := getUniqueClaimName(claim)
				volumeClone := reactor.getVolumeClone(volumeName)
				if volumeClone == nil {
					return fmt.Errorf("unable to find volume %s in reactor; "+
						"volume likely deleted too early", volumeName)
				}
				volumeClone.Status.Phase = v1.VolumeReleased
				vs.Modify(volumeClone)
				return nil
			},
		},
	}
	for _, test := range tests {
		orchestrator := core.NewMockOrchestrator()
		// Initialize storage classes
		for _, conf := range test.storageClassConfigs {
			_, err := orchestrator.AddStorageClass(conf)
			if err != nil {
				t.Fatalf("Unable to add storage class %s:  %v", conf.Name, err)
			}
		}
		// Initialize the Kubernetes components
		client := &fake.Clientset{}
		cs := framework.NewFakePVCControllerSource()
		volumeSource := framework.NewFakePVControllerSource()
		classSource := framework.NewFakeControllerSource()
		resizeSource := framework.NewFakePVCControllerSource()
		ctrl, err := newTestPlugin(orchestrator, client, cs, volumeSource,
			classSource, resizeSource, test.protocols, kubeVersion)
		if err != nil {
			t.Fatalf("Unable to create the Kubernetes plugin: %v", err)
		}
		reactor := newReactor(client, cs)

		log.WithFields(log.Fields{
			"test": test.name,
		}).Debug("Starting controller.")
		ctrl.Activate()
		err = test.action(ctrl, reactor, cs, volumeSource, &test)
		if err != nil {
			t.Error("Unable to perform action:  ", err)
		}
		reactor.wait()
		ctrl.Deactivate()
		frontendSuccess := reactor.validateVolumes(t, test.expectedVols)
		backendSuccess := orchestrator.ValidateVolumes(t,
			test.expectedVolumeConfigs)
		if !(frontendSuccess && backendSuccess) {
			t.Error("Test failed:  ", test.name)
		}
	}
}

func TestStorageClassControllerKubeVersion1_5(t *testing.T) {
	kubeVersion := k8sclient.NewFakeKubeClient(nil, "1", "5").Version()
	for _, test := range []struct {
		name          string
		classToPost   *k8sstoragev1beta.StorageClass
		expectedClass *sc.StorageClass
	}{
		{
			name: "other-provisioner",
			classToPost: testStorageClassV1_5("other-sc", false, map[string]string{
				sa.Media:            "hdd",
				sa.ProvisioningType: "thin",
				sa.Snapshots:        "true",
				sa.IOPS:             "500",
				sa.RequiredStorage:  "solidfire_10.63.171.153:Bronze",
			}),
			expectedClass: nil,
		},
		{
			name: "attributes-only",
			classToPost: testStorageClassV1_5("attributes-only", true,
				map[string]string{
					sa.Media:            "hdd",
					sa.ProvisioningType: "thin",
					sa.Snapshots:        "true",
					sa.IOPS:             "500",
				}),
			expectedClass: sc.New(&sc.Config{
				Name: "attributes-only",
				Attributes: map[string]sa.Request{
					sa.Media:            sa.NewStringRequest("hdd"),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.IOPS:             sa.NewIntRequest(500),
				},
			}),
		},
		{
			name: "backends-only",
			classToPost: testStorageClassV1_5("backends-only", true,
				map[string]string{
					sa.RequiredStorage: "sampleBackend:vc1,vc2;otherBackend:vc1",
				}),
			expectedClass: sc.New(&sc.Config{
				Name:       "backends-only",
				Attributes: make(map[string]sa.Request),
				AdditionalPools: map[string][]string{
					"sampleBackend": {"vc1", "vc2"},
					"otherBackend":  {"vc1"},
				},
			}),
		},
		{
			name: "backends-and-attributes",
			classToPost: testStorageClassV1_5("backends-and-attributes", true,
				map[string]string{
					sa.Media:            "hdd",
					sa.ProvisioningType: "thin",
					sa.Snapshots:        "true",
					sa.IOPS:             "500",
					sa.RequiredStorage:  "sampleBackend:vc1,vc2;otherBackend:vc1",
				}),
			expectedClass: sc.New(&sc.Config{
				Name: "backends-and-attributes",
				Attributes: map[string]sa.Request{
					sa.Media:            sa.NewStringRequest("hdd"),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.IOPS:             sa.NewIntRequest(500),
				},
				AdditionalPools: map[string][]string{
					"sampleBackend": {"vc1", "vc2"},
					"otherBackend":  {"vc1"},
				},
			}),
		},
		{
			name: "empty",
			classToPost: testStorageClassV1_5("empty", true,
				map[string]string{}),
			expectedClass: sc.New(&sc.Config{
				Name:            "empty",
				Attributes:      make(map[string]sa.Request),
				AdditionalPools: nil,
			}),
		},
	} {
		orchestrator := core.NewMockOrchestrator()
		client := &fake.Clientset{}
		claimSource := framework.NewFakePVCControllerSource()
		volumeSource := framework.NewFakePVControllerSource()
		classSource := framework.NewFakeControllerSource()
		resizeSource := framework.NewFakePVCControllerSource()
		ctrl, err := newTestPlugin(orchestrator, client, claimSource, volumeSource,
			classSource, resizeSource, []config.Protocol{config.File}, kubeVersion)
		if err != nil {
			t.Fatalf("Unable to create the Kubernetes plugin: %v", err)
		}
		ctrl.Activate()

		// Begin test
		classSource.Add(test.classToPost)
		// Wait for the frontend to propagate the event to the orchestrator.
		oldSCCount := -1
		storageClasses, _ := orchestrator.ListStorageClasses()
		newSCCount := len(storageClasses)
		for oldSCCount != newSCCount {
			time.Sleep(10 * time.Millisecond)
			oldSCCount = newSCCount
			storageClasses, _ = orchestrator.ListStorageClasses()
			newSCCount = len(storageClasses)
		}

		found, _ := orchestrator.GetStorageClass(test.classToPost.Name)
		if found == nil && test.expectedClass != nil {
			t.Errorf("%s:  Did not find expected storage class.", test.name)
		} else if test.expectedClass == nil && found != nil {
			t.Errorf("%s:  Found storage class when not expecting one.",
				test.name)
		} else if test.expectedClass != nil && !reflect.DeepEqual(found,
			test.expectedClass.ConstructExternal()) {
			t.Errorf("%s:  Found and expected classes differ:  %s",
				test.name, diff.ObjectDiff(found,
					test.expectedClass.ConstructExternal()),
			)
		}
	}
}

func TestV1betaStorageClassKube1_5(t *testing.T) {
	kubeVersion := k8sclient.NewFakeKubeClient(nil, "1", "5").Version()
	tests := []pluginTest{
		{
			name: "Basic v1beta storage class provisioning",
			expectedVols: []*v1.PersistentVolume{
				testVolume("pvc-silver", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig(
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"pvc-silver", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			v1betaStorageClasses: []*k8sstoragev1beta.StorageClass{
				testStorageClassV1_5("silver", true, map[string]string{})},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-silver", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "Unsupported default storage class",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			v1StorageClasses: []*k8sstoragev1.StorageClass{
				testStorageClassV1_6("silver", true, true, map[string]string{})},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-default", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},
	}

	for _, test := range tests {
		orchestrator := core.NewMockOrchestrator()

		// Initialize the Kubernetes components
		client := &fake.Clientset{}
		claimSource := framework.NewFakePVCControllerSource()
		volumeSource := framework.NewFakePVControllerSource()
		classSource := framework.NewFakeControllerSource()
		resizeSource := framework.NewFakePVCControllerSource()
		ctrl, err := newTestPlugin(orchestrator, client, claimSource,
			volumeSource, classSource, resizeSource, test.protocols, kubeVersion)
		if err != nil {
			t.Fatalf("Unable to create the Kubernetes plugin: %v", err)
		}
		reactor := newReactor(client, claimSource)

		log.WithFields(log.Fields{
			"test": test.name,
		}).Debug("Starting controller.")
		ctrl.Activate()

		// Add storage classes
		for _, class := range test.v1betaStorageClasses {
			classSource.Add(class)
		}
		oldSCCount := -1 //TODO: change this approach
		storageClasses, _ := orchestrator.ListStorageClasses()
		newSCCount := len(storageClasses)
		for oldSCCount != newSCCount {
			time.Sleep(10 * time.Millisecond)
			oldSCCount = newSCCount
			storageClasses, _ = orchestrator.ListStorageClasses()
			newSCCount = len(storageClasses)
		}

		err = test.action(ctrl, reactor, claimSource, volumeSource, &test)
		if err != nil {
			t.Error("Unable to perform action:  ", err)
		}
		reactor.wait()
		ctrl.Deactivate()
		frontendSuccess := reactor.validateVolumes(t, test.expectedVols)
		backendSuccess := orchestrator.ValidateVolumes(t,
			test.expectedVolumeConfigs)
		if !(frontendSuccess && backendSuccess) {
			t.Error("Test failed:  ", test.name)
		}
	}
}

func TestV1StorageClassKube1_6(t *testing.T) {
	kubeVersion := k8sclient.NewFakeKubeClient(nil, "1", "6").Version()
	tests := []pluginTest{
		{
			name: "Basic v1 storage class provisioning",
			expectedVols: []*v1.PersistentVolume{
				testVolume("pvc-silver", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig(
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"pvc-silver", "pvc1", "20M",
					map[string]string{
						AnnClass: "silver",
					},
					kubeVersion,
				),
			},
			v1StorageClasses: []*k8sstoragev1.StorageClass{
				testStorageClassV1_6("silver", true, false, map[string]string{}),
			},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-silver", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnClass:              "silver",
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},

		{
			name: "Provisioning using the default storage class",
			expectedVols: []*v1.PersistentVolume{
				testVolume("pvc-default", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					config.File,
					v1.PersistentVolumeReclaimDelete,
					"silver",
					kubeVersion,
				),
			},
			expectedVolumeConfigs: []*storage.VolumeConfig{
				testVolumeConfig(
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					"pvc-default", "pvc1", "20M",
					map[string]string{AnnClass: "silver"},
					kubeVersion,
				),
			},
			v1StorageClasses: []*k8sstoragev1.StorageClass{
				testStorageClassV1_6("silver", true, true, map[string]string{}),
			},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-default", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "No provisioning without the default storage class",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			v1StorageClasses: []*k8sstoragev1.StorageClass{testStorageClassV1_6("silver", true, false,
				map[string]string{})},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-default", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "Provisioning with two default storage classes",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			v1StorageClasses: []*k8sstoragev1.StorageClass{
				testStorageClassV1_6("silver", true, true,
					map[string]string{}),
				testStorageClassV1_6("bronze", true, true,
					map[string]string{}),
			},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-default", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnStorageProvisioner: AnnOrchestrator,
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},
		{
			name:                  "No dynamic provisioning for unset storage class",
			expectedVols:          []*v1.PersistentVolume{},
			expectedVolumeConfigs: []*storage.VolumeConfig{},
			v1StorageClasses: []*k8sstoragev1.StorageClass{
				testStorageClassV1_6("bronze", true, true,
					map[string]string{}),
			},
			protocols: []config.Protocol{config.File},
			action: func(
				ctrl *Plugin,
				reactor *orchestratorReactor,
				claimSource *framework.FakePVCControllerSource,
				pvSource *framework.FakePVControllerSource,
				ct *pluginTest,
			) error {
				claim := testClaim("pvc-default", "pvc1", "20M",
					[]v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					v1.ClaimPending,
					map[string]string{
						AnnStorageProvisioner: AnnOrchestrator,
						AnnClass:              "",
					},
					kubeVersion,
				)
				claimSource.Add(claim)
				// We need to wait here; otherwise, the client may coalesce
				// events.
				reactor.wait()
				claimClone := cloneClaim(claim)
				claimClone.Spec.VolumeName = getUniqueClaimName(claimClone)
				claimClone.Status.Phase = v1.ClaimBound
				claimSource.Modify(claimClone)
				return nil
			},
		},
	}

	for _, test := range tests {
		orchestrator := core.NewMockOrchestrator()

		// Initialize the Kubernetes components
		client := &fake.Clientset{}
		claimSource := framework.NewFakePVCControllerSource()
		volumeSource := framework.NewFakePVControllerSource()
		classSource := framework.NewFakeControllerSource()
		resizeSource := framework.NewFakePVCControllerSource()
		ctrl, err := newTestPlugin(orchestrator, client, claimSource,
			volumeSource, classSource, resizeSource, test.protocols, kubeVersion)
		if err != nil {
			t.Fatalf("Unable to create the Kubernetes plugin: %v", err)
		}
		reactor := newReactor(client, claimSource)

		log.WithFields(log.Fields{
			"test": test.name,
		}).Debug("Starting controller.")
		ctrl.Activate()

		// Add storage classes
		for _, class := range test.v1StorageClasses {
			classSource.Add(class)
		}
		oldSCCount := -1 //TODO: change this approach
		storageClasses, _ := orchestrator.ListStorageClasses()
		newSCCount := len(storageClasses)
		for oldSCCount != newSCCount {
			time.Sleep(10 * time.Millisecond)
			oldSCCount = newSCCount
			storageClasses, _ := orchestrator.ListStorageClasses()
			newSCCount = len(storageClasses)
		}

		err = test.action(ctrl, reactor, claimSource, volumeSource, &test)
		if err != nil {
			t.Error("Unable to perform action:  ", err)
		}
		reactor.wait()
		ctrl.Deactivate()
		frontendSuccess := reactor.validateVolumes(t, test.expectedVols)
		backendSuccess := orchestrator.ValidateVolumes(t,
			test.expectedVolumeConfigs)
		if !(frontendSuccess && backendSuccess) {
			t.Error("Test failed:  ", test.name)
		}
	}
}
