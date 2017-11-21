// Copyright 2016 NetApp, Inc. All Rights Reserved.

package k8s_client

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Interface interface {
	Version() *version.Info
	GetDeployment(deploymentName string, options metav1.GetOptions) (*v1beta1.Deployment, error)
	CheckDeploymentExists(deploymentName string) (bool, error)
	CreateDeployment(deployment *v1beta1.Deployment) (*v1beta1.Deployment, error)
	GetPod(podName string, options metav1.GetOptions) (*v1.Pod, error)
	GetPodByLabels(listOptions *metav1.ListOptions) (*v1.Pod, error)
	GetPodPhase(podName string, options metav1.GetOptions) (v1.PodPhase, error)
	CheckPodExists(pod string) (bool, error)
	CreatePod(pod *v1.Pod) (*v1.Pod, error)
	DeletePod(podName string, options *metav1.DeleteOptions) error
	WatchPod(listOptions *metav1.ListOptions) (watch.Interface, error)
	ListPod(listOptions *metav1.ListOptions) (*v1.PodList, error)
	GetRunningPod(pod *v1.Pod, timeout *int64, labels map[string]string) (*v1.Pod, error)
	GetPVC(pvcName string, options metav1.GetOptions) (*v1.PersistentVolumeClaim, error)
	GetPVCPhase(pvcName string, options metav1.GetOptions) (v1.PersistentVolumeClaimPhase, error)
	CheckPVCExists(pvc string) (bool, error)
	CreatePVC(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error)
	DeletePVC(pvcName string, options *metav1.DeleteOptions) error
	WatchPVC(listOptions *metav1.ListOptions) (watch.Interface, error)
	GetBoundPVC(pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume, timeout *int64, labels map[string]string) (*v1.PersistentVolumeClaim, error)
	CreatePV(pv *v1.PersistentVolume) (*v1.PersistentVolume, error)
	DeletePV(pvName string, options *metav1.DeleteOptions) error
	CreateSecret(secret *v1.Secret) (*v1.Secret, error)
	CreateCHAPSecret(secretName, accountName, initiatorSecret, targetSecret string) (*v1.Secret, error)
	GetSecret(secretName string, options metav1.GetOptions) (*v1.Secret, error)
	CheckSecretExists(secretName string) (bool, error)
	DeleteSecret(secretName string, options *metav1.DeleteOptions) error
}

type KubeClient struct {
	clientset   *kubernetes.Clientset
	namespace   string
	versionInfo *version.Info
}

func NewKubeClient(config *rest.Config, namespace string) (Interface, error) {
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

func (k *KubeClient) GetDeployment(
	deploymentName string,
	options metav1.GetOptions) (*v1beta1.Deployment, error) {
	return k.clientset.ExtensionsV1beta1().Deployments(k.namespace).Get(
		deploymentName, options)
}

func (k *KubeClient) CheckDeploymentExists(deploymentName string) (bool, error) {
	var options metav1.GetOptions
	if _, err := k.GetDeployment(deploymentName, options); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
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

func (k *KubeClient) GetPod(podName string, options metav1.GetOptions) (*v1.Pod, error) {
	return k.clientset.Core().Pods(k.namespace).Get(podName, options)
}

func (k *KubeClient) GetPodByLabels(listOptions *metav1.ListOptions) (*v1.Pod, error) {
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

func (k *KubeClient) GetPodPhase(podName string, options metav1.GetOptions) (v1.PodPhase, error) {
	pod, err := k.GetPod(podName, options)
	if err != nil {
		var phase v1.PodPhase = ""
		return phase, err
	}
	return pod.Status.Phase, nil
}

func (k *KubeClient) CheckPodExists(pod string) (bool, error) {
	var options metav1.GetOptions
	if _, err := k.GetPod(pod, options); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) CreatePod(pod *v1.Pod) (*v1.Pod, error) {
	return k.clientset.Core().Pods(k.namespace).Create(pod)
}

func (k *KubeClient) DeletePod(podName string, options *metav1.DeleteOptions) error {
	return k.clientset.Core().Pods(k.namespace).Delete(podName, options)
}

func (k *KubeClient) WatchPod(listOptions *metav1.ListOptions) (watch.Interface, error) {
	return k.clientset.Core().Pods(k.namespace).Watch(*listOptions)
}

func (k *KubeClient) ListPod(listOptions *metav1.ListOptions) (*v1.PodList, error) {
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

func (k *KubeClient) GetPVC(pvcName string,
	options metav1.GetOptions) (*v1.PersistentVolumeClaim, error) {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Get(
		pvcName, options)
}

func (k *KubeClient) GetPVCPhase(pvcName string,
	options metav1.GetOptions) (v1.PersistentVolumeClaimPhase, error) {
	pvc, err := k.GetPVC(pvcName, options)
	if err != nil {
		var phase v1.PersistentVolumeClaimPhase = ""
		return phase, err
	}
	return pvc.Status.Phase, nil
}

func (k *KubeClient) CheckPVCExists(pvc string) (bool, error) {
	var options metav1.GetOptions
	if _, err := k.GetPVC(pvc, options); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) CreatePVC(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Create(pvc)
}

func (k *KubeClient) DeletePVC(pvcName string, options *metav1.DeleteOptions) error {
	return k.clientset.Core().PersistentVolumeClaims(k.namespace).Delete(pvcName, options)
}

func (k *KubeClient) WatchPVC(listOptions *metav1.ListOptions) (watch.Interface, error) {
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

func (k *KubeClient) DeletePV(pvName string, options *metav1.DeleteOptions) error {
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

func CreateListOptions(timeout *int64, labels map[string]string, resourceVersion string) *metav1.ListOptions {
	listOptions := &metav1.ListOptions{
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

// CreateSecret creates a new Secret
func (k *KubeClient) CreateSecret(secret *v1.Secret) (*v1.Secret, error) {
	return k.clientset.Core().Secrets(k.namespace).Create(secret)
}

// CreateCHAPSecret creates a new Secret for iSCSI CHAP mutual authentication
func (k *KubeClient) CreateCHAPSecret(secretName, accountName, initiatorSecret, targetSecret string) (*v1.Secret, error) {
	return k.CreateSecret(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: k.namespace,
			Name:      secretName,
		},
		Type: "kubernetes.io/iscsi-chap",
		Data: map[string][]byte{
			"discovery.sendtargets.auth.username":    []byte(accountName),
			"discovery.sendtargets.auth.password":    []byte(initiatorSecret),
			"discovery.sendtargets.auth.username_in": []byte(accountName),
			"discovery.sendtargets.auth.password_in": []byte(targetSecret),
			"node.session.auth.username":             []byte(accountName),
			"node.session.auth.password":             []byte(initiatorSecret),
			"node.session.auth.username_in":          []byte(accountName),
			"node.session.auth.password_in":          []byte(targetSecret),
		},
	})
}

// GetSecret looks up a Secret by name
func (k *KubeClient) GetSecret(secretName string, options metav1.GetOptions) (*v1.Secret, error) {
	return k.clientset.Core().Secrets(k.namespace).Get(secretName, options)
}

// CheckSecretExists returns true if the Secret exists
func (k *KubeClient) CheckSecretExists(secretName string) (bool, error) {
	var options metav1.GetOptions
	if _, err := k.GetSecret(secretName, options); err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteSecret deletes the specified Secret
func (k *KubeClient) DeleteSecret(secretName string, options *metav1.DeleteOptions) error {
	return k.clientset.Core().Secrets(k.namespace).Delete(secretName, options)
}
