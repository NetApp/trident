// Copyright 2019 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	framework "k8s.io/client-go/tools/cache/testing"
	"k8s.io/client-go/tools/record"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	k8sclient "github.com/netapp/trident/k8s_client"
	"github.com/netapp/trident/storage"
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
	volumeMode *v1.PersistentVolumeMode,
	name string,
	pvcUID types.UID,
	size string,
	annotations map[string]string,
	kubeVersion *k8sversion.Info,
) *storage.VolumeConfig {
	ret := getVolumeConfig(accessModes, volumeMode,
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
		mutex:                    &sync.Mutex{},
		pendingClaimMatchMap:     make(map[string]*v1.PersistentVolume),
		defaultStorageClasses:    make(map[string]bool, 1),
		storageClassCache:        make(map[string]*StorageClassSummary),
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
			Interface: client.CoreV1().Events(""),
		})
	ret.eventRecorder = broadcaster.NewRecorder(runtime.NewScheme(),
		v1.EventSource{Component: AnnOrchestrator})
	// Note that at the moment we can only actually support NFS here; the
	// iSCSI backends all trigger interactions with a real backend to map
	// newly provisioned LUNs, which won't work in a test environment.
	for _, p := range protocols {
		switch p {
		case config.File:
			orchestrator.AddMockONTAPNFSBackend(context.Background(), "nfs", testNFSServer)
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
