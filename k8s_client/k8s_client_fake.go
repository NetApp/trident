// Copyright 2016 NetApp, Inc. All Rights Reserved.

package k8s_client

import (
	"fmt"
	"sort"

	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/pkg/watch"
)

type FakeKubeClient struct {
	Deployments map[string]*v1beta1.Deployment
	PVCs        map[string]*v1.PersistentVolumeClaim
	failMatrix  map[string]bool
}

func NewFakeKubeClient(failMatrix map[string]bool) *FakeKubeClient {
	return &FakeKubeClient{
		Deployments: make(map[string]*v1beta1.Deployment, 0),
		PVCs:        make(map[string]*v1.PersistentVolumeClaim, 0),
		failMatrix:  failMatrix,
	}
}

type FakeKubeClientState struct {
	Deployments []string
	PVCs        []string
}

func (k *FakeKubeClient) SnapshotState() *FakeKubeClientState {
	state := &FakeKubeClientState{
		Deployments: make([]string, 0),
		PVCs:        make([]string, 0),
	}
	for key, _ := range k.Deployments {
		state.Deployments = append(state.Deployments, key)
	}
	sort.Strings(state.Deployments)
	for key, _ := range k.PVCs {
		state.PVCs = append(state.PVCs, key)
	}
	sort.Strings(state.PVCs)
	return state
}

func (k *FakeKubeClient) Version() *version.Info {
	return nil
}

func (k *FakeKubeClient) GetDeployment(deploymentName string) (*v1beta1.Deployment, error) {
	if fail, ok := k.failMatrix["GetDeployment"]; fail && ok {
		return nil, fmt.Errorf("GetDeployment failed")
	}
	if deployment, ok := k.Deployments[deploymentName]; ok {
		return deployment, nil
	}
	err := &errors.StatusError{}
	err.ErrStatus.Reason = unversioned.StatusReasonNotFound
	return nil, err
}

func (k *FakeKubeClient) CheckDeploymentExists(deploymentName string) (bool, error) {
	if _, err := k.GetDeployment(deploymentName); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == unversioned.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *FakeKubeClient) CreateDeployment(deployment *v1beta1.Deployment) (*v1beta1.Deployment, error) {
	if fail, ok := k.failMatrix["CreateDeployment"]; fail && ok {
		return deployment, fmt.Errorf("CreateDeployment failed")
	}
	k.Deployments[deployment.Name] = deployment
	return deployment, nil
}

func (k *FakeKubeClient) GetPod(podName string) (*v1.Pod, error) {
	return nil, nil
}

func (k *FakeKubeClient) GetPodByLabels(listOptions *v1.ListOptions) (*v1.Pod, error) {
	return nil, nil
}

func (k *FakeKubeClient) GetPodPhase(podName string) (v1.PodPhase, error) {
	return v1.PodRunning, nil
}

func (k *FakeKubeClient) CheckPodExists(pod string) (bool, error) {
	return true, nil
}

func (k *FakeKubeClient) CreatePod(pod *v1.Pod) (*v1.Pod, error) {
	return nil, nil
}

func (k *FakeKubeClient) DeletePod(podName string, options *v1.DeleteOptions) error {
	return nil
}

func (k *FakeKubeClient) WatchPod(listOptions *v1.ListOptions) (watch.Interface, error) {
	return watch.NewEmptyWatch(), nil
}

func (k *FakeKubeClient) ListPod(listOptions *v1.ListOptions) (*v1.PodList, error) {
	return nil, nil
}

func (k *FakeKubeClient) GetRunningPod(pod *v1.Pod, timeout *int64, labels map[string]string) (*v1.Pod, error) {
	return nil, nil
}

func (k *FakeKubeClient) GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error) {
	if fail, ok := k.failMatrix["GetPVC"]; fail && ok {
		return nil, fmt.Errorf("GetPVC failed")
	}
	if pvc, ok := k.PVCs[pvcName]; ok {
		return pvc, nil
	}
	err := &errors.StatusError{}
	err.ErrStatus.Reason = unversioned.StatusReasonNotFound
	return nil, err
}

func (k *FakeKubeClient) GetPVCPhase(pvcName string) (v1.PersistentVolumeClaimPhase, error) {
	pvc, err := k.GetPVC(pvcName)
	if err != nil {
		var phase v1.PersistentVolumeClaimPhase = ""
		return phase, err
	}
	return pvc.Status.Phase, nil
}

func (k *FakeKubeClient) CheckPVCExists(pvc string) (bool, error) {
	if _, err := k.GetPVC(pvc); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == unversioned.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *FakeKubeClient) CreatePVC(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	if fail, ok := k.failMatrix["CreatePVC"]; fail && ok {
		return pvc, fmt.Errorf("CreatePVC failed")
	}
	k.PVCs[pvc.Name] = pvc
	return pvc, nil
}

func (k *FakeKubeClient) DeletePVC(pvcName string, options *v1.DeleteOptions) error {
	return nil
}

func (k *FakeKubeClient) WatchPVC(listOptions *v1.ListOptions) (watch.Interface, error) {
	return watch.NewEmptyWatch(), nil
}

func (k *FakeKubeClient) GetBoundPVC(pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume, timeout *int64, labels map[string]string) (*v1.PersistentVolumeClaim, error) {
	return nil, nil
}

func (k *FakeKubeClient) CreatePV(pv *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	return nil, nil
}

func (k *FakeKubeClient) DeletePV(pvName string, options *v1.DeleteOptions) error {
	return nil
}
