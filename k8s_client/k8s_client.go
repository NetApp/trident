// Copyright 2016 NetApp, Inc. All Rights Reserved.

package k8s_client

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
)

type Interface interface {
	Version() *version.Info
	GetDeployment(deploymentName string) (*v1beta1.Deployment, error)
	CheckDeploymentExists(deploymentName string) (bool, error)
	CreateDeployment(deployment *v1beta1.Deployment) (*v1beta1.Deployment, error)
	GetPod(podName string) (*v1.Pod, error)
	GetPodByLabels(listOptions *v1.ListOptions) (*v1.Pod, error)
	GetPodPhase(podName string) (v1.PodPhase, error)
	CheckPodExists(pod string) (bool, error)
	CreatePod(pod *v1.Pod) (*v1.Pod, error)
	DeletePod(podName string, options *v1.DeleteOptions) error
	WatchPod(listOptions *v1.ListOptions) (watch.Interface, error)
	ListPod(listOptions *v1.ListOptions) (*v1.PodList, error)
	GetRunningPod(pod *v1.Pod, timeout *int64, labels map[string]string) (*v1.Pod, error)
	GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error)
	GetPVCPhase(pvcName string) (v1.PersistentVolumeClaimPhase, error)
	CheckPVCExists(pvc string) (bool, error)
	CreatePVC(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error)
	DeletePVC(pvcName string, options *v1.DeleteOptions) error
	WatchPVC(listOptions *v1.ListOptions) (watch.Interface, error)
	GetBoundPVC(pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume, timeout *int64, labels map[string]string) (*v1.PersistentVolumeClaim, error)
	CreatePV(pv *v1.PersistentVolume) (*v1.PersistentVolume, error)
	DeletePV(pvName string, options *v1.DeleteOptions) error
}

type KubeClient struct {
	clientset   *kubernetes.Clientset
	namespace   string
	versionInfo *version.Info
}

func NewKubeClient(config *rest.Config, namespace string) (*KubeClient, error) {
	var versionInfo *version.Info
	if namespace == "" {
		return nil, fmt.Errorf("An empty namespace is not acceptable!")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	versionInfo, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("Couldn't retrieve API server's version: %v", err)
	}
	kubeClient := &KubeClient{
		clientset:   clientset,
		namespace:   namespace,
		versionInfo: versionInfo,
	}
	return kubeClient, nil
}

func (k *KubeClient) Version() *version.Info {
	return k.versionInfo
}

func (k *KubeClient) GetDeployment(deploymentName string) (*v1beta1.Deployment, error) {
	return k.clientset.ExtensionsV1beta1().Deployments(k.namespace).Get(
		deploymentName)
}

func (k *KubeClient) CheckDeploymentExists(deploymentName string) (bool, error) {
	if _, err := k.GetDeployment(deploymentName); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == unversioned.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) CreateDeployment(deployment *v1beta1.Deployment) (*v1beta1.Deployment, error) {
	return k.clientset.ExtensionsV1beta1().Deployments(k.namespace).Create(
		deployment)
}

func (k *KubeClient) GetPod(podName string) (*v1.Pod, error) {
	return k.clientset.Core().Pods(k.namespace).Get(podName)
}

func (k *KubeClient) GetPodByLabels(listOptions *v1.ListOptions) (*v1.Pod, error) {
	var (
		watchedPod *v1.Pod
		timeout    int64
	)
	startTime := time.Now()
	timeout = *listOptions.TimeoutSeconds
	listOptions.TimeoutSeconds = nil //no timeout
	pods, err := k.ListPod(listOptions)
	if len(pods.Items) == 1 {
		return &pods.Items[0], nil
	} else if len(pods.Items) > 1 {
		return nil, fmt.Errorf("Multiple pods have the label %s: %v",
			listOptions.LabelSelector, pods.Items)
	}
	listOptions.TimeoutSeconds = &timeout
	log.Debugf("KubeClient took %v to retrieve pods: %v.",
		time.Since(startTime), pods.Items)
	podWatch, err := k.WatchPod(listOptions)
	if err != nil {
		return nil, err
	}
	defer podWatch.Stop()
	for event := range podWatch.ResultChan() {
		switch event.Type {
		case watch.Error:
			//TODO: Validate error handling.
			return nil, fmt.Errorf("Received error when watching pod: %s",
				event.Object.GetObjectKind().GroupVersionKind().String())
		case watch.Deleted:
			return nil, fmt.Errorf("Pod got deleted before becoming available!")
		case watch.Added, watch.Modified:
			watchedPod = event.Object.(*v1.Pod)
		default:
			return nil, fmt.Errorf(
				"Got unknown event type %s while watching pod!",
				event.Type)
		}
	}
	log.Debugf("KubeClient took %v to retrieve pod %v.",
		time.Since(startTime), watchedPod.Name)
	return watchedPod, nil
}

func (k *KubeClient) GetPodPhase(podName string) (v1.PodPhase, error) {
	pod, err := k.GetPod(podName)
	if err != nil {
		var phase v1.PodPhase = ""
		return phase, err
	}
	return pod.Status.Phase, nil
}

func (k *KubeClient) CheckPodExists(pod string) (bool, error) {
	if _, err := k.GetPod(pod); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == unversioned.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) CreatePod(pod *v1.Pod) (*v1.Pod, error) {
	return k.clientset.Core().Pods(k.namespace).Create(pod)
}

func (k *KubeClient) DeletePod(podName string, options *v1.DeleteOptions) error {
	return k.clientset.Core().Pods(k.namespace).Delete(podName, options)
}

func (k *KubeClient) WatchPod(listOptions *v1.ListOptions) (watch.Interface, error) {
	return k.clientset.Core().Pods(k.namespace).Watch(*listOptions)
}

func (k *KubeClient) ListPod(listOptions *v1.ListOptions) (*v1.PodList, error) {
	return k.clientset.Core().Pods(k.namespace).List(*listOptions)
}

func (k *KubeClient) GetRunningPod(pod *v1.Pod, timeout *int64, labels map[string]string) (*v1.Pod, error) {
	var watchedPod *v1.Pod
	podWatch, err := k.WatchPod(CreateListOptions(
		timeout, labels, pod.ResourceVersion))
	if err != nil {
		return pod, err
	}
	defer podWatch.Stop()
	for event := range podWatch.ResultChan() {
		switch event.Type {
		case watch.Error:
			//TODO: Validate error handling.
			return pod, fmt.Errorf("Received error when watching pod %s: %s",
				pod.Name,
				event.Object.GetObjectKind().GroupVersionKind().String())
		case watch.Deleted, watch.Added, watch.Modified:
			watchedPod = event.Object.(*v1.Pod)
		default:
			return pod, fmt.Errorf(
				"Got unknown event type %s while watching pod %s!",
				event.Type, pod.Name)
		}
		if watchedPod.Name != pod.Name {
			continue
		}
		if event.Type == watch.Deleted {
			return pod, fmt.Errorf("Pod %s got deleted before becoming available!",
				pod.Name)
		}
		switch watchedPod.Status.Phase {
		case v1.PodPending:
			continue
		case v1.PodSucceeded, v1.PodFailed:
			return pod, fmt.Errorf("Pod %s exited early with status %s!",
				pod.Name, pod.Status.Phase)
		case v1.PodRunning:
			return watchedPod, nil
		case v1.PodUnknown:
			return pod, fmt.Errorf("Couldn't obtain Pod %s's state!", pod.Name)
		default:
			return pod, fmt.Errorf("Pod %s has unknown status (%s)!", pod.Name)
		}
	}
	return pod, fmt.Errorf("Pod %s wasn't running within %d seconds!",
		pod.Name, *timeout)
}

func (k *KubeClient) GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error) {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Get(
		pvcName)
}

func (k *KubeClient) GetPVCPhase(pvcName string) (v1.PersistentVolumeClaimPhase,
	error) {
	pvc, err := k.GetPVC(pvcName)
	if err != nil {
		var phase v1.PersistentVolumeClaimPhase = ""
		return phase, err
	}
	return pvc.Status.Phase, nil
}

func (k *KubeClient) CheckPVCExists(pvc string) (bool, error) {
	if _, err := k.GetPVC(pvc); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == unversioned.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) CreatePVC(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Create(pvc)
}

func (k *KubeClient) DeletePVC(pvcName string, options *v1.DeleteOptions) error {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Delete(pvcName, options)
}

func (k *KubeClient) WatchPVC(listOptions *v1.ListOptions) (watch.Interface, error) {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Watch(*listOptions)
}

func (k *KubeClient) GetBoundPVC(pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume, timeout *int64, labels map[string]string) (*v1.PersistentVolumeClaim, error) {
	var watchedPVC *v1.PersistentVolumeClaim
	pvcWatch, err := k.WatchPVC(CreateListOptions(
		timeout, labels, pvc.ResourceVersion))
	if err != nil {
		return pvc, err
	}
	defer pvcWatch.Stop()
	for event := range pvcWatch.ResultChan() {
		switch event.Type {
		case watch.Deleted:
			return pvc, fmt.Errorf("PVC deleted before becoming bound.")
		case watch.Error:
			//TODO: Validate error handling.
			return pvc, fmt.Errorf("Received error when watching PVC %s: %s",
				pvc.Name,
				event.Object.GetObjectKind().GroupVersionKind().String())
		case watch.Added, watch.Modified:
			watchedPVC = event.Object.(*v1.PersistentVolumeClaim)
		default:
			return pvc,
				fmt.Errorf("Got unknown event type %s while watching PVC %s!",
					event.Type, pvc.Name)
		}
		if watchedPVC.Name != pvc.Name {
			continue
		}
		switch watchedPVC.Status.Phase {
		case v1.ClaimPending:
			continue
		case v1.ClaimLost:
			return pvc, fmt.Errorf("PVC is in the lost phase!")
		case v1.ClaimBound:
			if watchedPVC.Spec.VolumeName == pv.Name {
				return watchedPVC, nil
			}
		}
	}
	return pvc, fmt.Errorf("PVC %s wasn't bound to PV %s within %d seconds!",
		pvc.Name, pv.Name, *timeout)
}

func (k *KubeClient) CreatePV(pv *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	return k.clientset.Core().PersistentVolumes().Create(pv)
}

func (k *KubeClient) DeletePV(pvName string, options *v1.DeleteOptions) error {
	return k.clientset.Core().PersistentVolumes().Delete(pvName, options)
}

func CreateLabelSelectorString(labels map[string]string) string {
	ret := ""
	for k, v := range labels {
		ret += k
		ret += "="
		ret += v
		ret += ","
	}
	return strings.TrimSuffix(ret, ",")
}

func CreateListOptions(timeout *int64, labels map[string]string, resourceVersion string) *v1.ListOptions {
	listOptions := &v1.ListOptions{
		TimeoutSeconds: timeout,
	}
	if len(labels) > 0 {
		listOptions.LabelSelector = CreateLabelSelectorString(labels)
	}
	if resourceVersion != "" {
		listOptions.ResourceVersion = resourceVersion
	}
	return listOptions
}
