// Copyright 2016 NetApp, Inc. All Rights Reserved.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	k8sfrontend "github.com/netapp/trident/frontend/kubernetes"
	"github.com/netapp/trident/storage"

	log "github.com/Sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/resource"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/types"
	"k8s.io/client-go/pkg/util/yaml"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
)

const (
	pvcName              = "trident"
	pvName               = "trident"
	inMemoryTridentName  = "trident-ephemeral"
	tridentContainerName = "trident-main"
	tridentLabelValue    = "trident.netapp.io"

	defaultPort = 8000

	timeout = 60
)

var (
	volBytes int64 = int64(volGB) * 1073741824
)

type KubeClient struct {
	clientset         *kubernetes.Clientset
	namespace         string
	tridentName       string
	tridentImage      string
	lastRV            string
	tridentDeployment *v1beta1.Deployment
}

func NewKubeClient(config *rest.Config, podDefinitionPath string) (
	*KubeClient, error) {
	var tridentDeployment v1beta1.Deployment

	yamlBytes, err := ioutil.ReadFile(podDefinitionPath)
	if err != nil {
		return nil, err
	}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(yamlBytes), 512).Decode(
		&tridentDeployment)
	if err != nil {
		return nil, err
	}

	tridentImage := ""
	for _, container := range tridentDeployment.Spec.Template.Spec.Containers {
		if container.Name == tridentContainerName {
			tridentImage = container.Image
		}
	}
	if tridentImage == "" {
		return nil, fmt.Errorf("Trident container not found in pod definition.")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &KubeClient{clientset: clientset,
		namespace:         tridentDeployment.Namespace,
		tridentName:       tridentDeployment.Name,
		tridentDeployment: &tridentDeployment,
		tridentImage:      tridentImage,
	}, nil
}

func (k *KubeClient) CheckTridentRunning() (bool, error) {
	_, err := k.clientset.ExtensionsV1beta1().Deployments(k.namespace).Get(
		k.tridentDeployment.Name)
	if err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.ErrStatus.Code != http.StatusNotFound {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (k *KubeClient) PVCExists() (bool, error) {
	_, err := k.clientset.Core().PersistentVolumeClaims(k.namespace).Get(
		pvcName)
	if err != nil {
		if statusErr, ok := err.(*errors.StatusError); ok && statusErr.ErrStatus.Code != http.StatusNotFound {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (k *KubeClient) getRunningTrident(
	namePrefix string,
) (createdPod *v1.Pod, err error) {
	var timeoutSeconds int64 = timeout
	podWatch, err := k.clientset.Core().Pods(k.namespace).Watch(
		v1.ListOptions{
			// TODO:  I'm not sure TimeoutSeconds actually does anything
			TimeoutSeconds: &timeoutSeconds,
			LabelSelector:  fmt.Sprintf("app=%s", tridentLabelValue),
		},
	)
	if err != nil {
		return nil, err
	}
	defer podWatch.Stop()
	// TODO:  It may be necessary to introduce a timeout here at some point.
	for event := range podWatch.ResultChan() {
		switch event.Type {
		case watch.Error:
			// TODO:  Figure out the error handling here.
			return nil, fmt.Errorf("Received error when running watch.")
		case watch.Deleted, watch.Added, watch.Modified:
			createdPod = event.Object.(*v1.Pod)
		default:
			return nil, fmt.Errorf(
				"Got unknown event type while watching:  %s", event.Type)
		}
		if !strings.HasPrefix(createdPod.Name, namePrefix) {
			continue
		}
		if event.Type == watch.Deleted {
			return nil, fmt.Errorf("Trident pod deleted before becoming " +
				"available.")
		}

		k.lastRV = createdPod.ResourceVersion
		switch createdPod.Status.Phase {
		case v1.PodPending:
			continue
		case v1.PodSucceeded, v1.PodFailed:
			return nil, fmt.Errorf("Pod exited early with status %s",
				createdPod.Status.Phase)
		case v1.PodRunning:
			return createdPod, nil
		case v1.PodUnknown:
			return nil, fmt.Errorf("Pod status is unknown.")
		default:
			return nil, fmt.Errorf("Received unknown pod status:  %s",
				createdPod.Status.Phase)
		}
	}
	return nil, fmt.Errorf("Unable to start Trident within %d seconds.",
		timeout)
}

func (k *KubeClient) waitInMemoryTridentDeletion(name string) error {
	var deletedPod *v1.Pod
	var timeoutSeconds int64 = timeout

	podWatch, err := k.clientset.Core().Pods(k.namespace).Watch(
		v1.ListOptions{
			// TODO:  I'm not sure TimeoutSeconds actually does anything
			TimeoutSeconds:  &timeoutSeconds,
			LabelSelector:   fmt.Sprintf("app=%s", tridentLabelValue),
			ResourceVersion: k.lastRV,
		},
	)
	if err != nil {
		return err
	}
	defer podWatch.Stop()
	for event := range podWatch.ResultChan() {
		switch event.Type {
		case watch.Error:
			// TODO:  Figure out the error handling here.
			return fmt.Errorf("Received error when running watch.")
		case watch.Added, watch.Modified:
			continue
		case watch.Deleted:
			deletedPod = event.Object.(*v1.Pod)
		default:
			return fmt.Errorf("Received unknown status for watch:  %s",
				event.Type)
		}
		if deletedPod.Name != name {
			continue
		}
		// The Trident pod was deleted successfully.
		return nil
	}
	return fmt.Errorf("Watch terminated early.")
}

// StartInMemoryTrident starts a Trident pod using an in-memory store and
// waits until it comes online.  Returns the pod's IP address once running
// or an error.
func (k *KubeClient) StartInMemoryTrident() (string, error) {
	// Don't bother checking if Trident exists for now.  In error cases, we
	// can have the user manually delete.
	podToCreate := &v1.Pod{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      inMemoryTridentName,
			Namespace: k.namespace,
			Labels:    map[string]string{"app": tridentLabelValue},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name:    tridentContainerName,
					Image:   k.tridentImage,
					Command: []string{"/usr/local/bin/trident_orchestrator"},
					Args: []string{
						"-port", fmt.Sprintf("%d", defaultPort),
						"-no_persistence",
					},
					Ports: []v1.ContainerPort{
						v1.ContainerPort{ContainerPort: defaultPort},
					},
				},
			},
		},
	}
	log.Debug("Creating Trident.")
	_, err := k.clientset.Core().Pods(k.namespace).Create(podToCreate)
	if err != nil {
		return "", err
	}
	createdPod, err := k.getRunningTrident(inMemoryTridentName)
	if err != nil {
		return "", err
	}
	if createdPod.Status.PodIP == "" {
		return "", fmt.Errorf("Pod IP empty.")
	}
	return createdPod.Status.PodIP, nil
}

func (k *KubeClient) StartFullTrident() error {
	_, err := k.clientset.ExtensionsV1beta1().Deployments(k.namespace).Create(k.tridentDeployment)
	if err != nil {
		return err
	}
	_, err = k.getRunningTrident(k.tridentName)
	return err
}

func (k *KubeClient) waitForPVCBind() (bool, error) {
	var createdPVC *v1.PersistentVolumeClaim

	pvcWatch, err := k.clientset.Core().PersistentVolumeClaims(
		k.namespace).Watch(v1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", tridentLabelValue),
	},
	)
	if err != nil {
		return false, err
	}
	defer pvcWatch.Stop()
	for event := range pvcWatch.ResultChan() {
		switch event.Type {
		case watch.Deleted:
			return false, fmt.Errorf("Trident PVC deleted before becoming " +
				"binding.")
		case watch.Error:
			return false, fmt.Errorf("Received error when running watch.")
		case watch.Added, watch.Modified:
			createdPVC = event.Object.(*v1.PersistentVolumeClaim)
		default:
			return false, fmt.Errorf("Got unknown event type while watching:  "+
				"%s", event.Type)
		}
		if createdPVC.Name != pvcName {
			continue
		}
		switch createdPVC.Status.Phase {
		case v1.ClaimPending:
			continue
		case v1.ClaimLost:
			return false, fmt.Errorf("Volume assigned to claim lost")
		}
		// We know if the PVC bound to the correct volume if the volume name
		// matches
		return createdPVC.Spec.VolumeName == pvName, nil
	}
	return false, fmt.Errorf("Watch stopped prematurely.  Volume state " +
		"unknown.")
}

func (k *KubeClient) createPVC() (types.UID, error) {
	pvcToCreate := &v1.PersistentVolumeClaim{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      pvcName,
			Namespace: k.namespace,
			Labels:    map[string]string{"app": tridentLabelValue},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: *resource.NewQuantity(volBytes,
						resource.BinarySI),
				},
			},
			Selector: &unversioned.LabelSelector{
				MatchLabels: map[string]string{"app": tridentLabelValue},
			},
		},
	}
	pvc, err := k.clientset.Core().PersistentVolumeClaims(k.namespace).Create(
		pvcToCreate)
	if err != nil {
		return "", err
	}
	return pvc.UID, nil
}

func (k *KubeClient) createPV(
	volConfig *storage.VolumeConfig, pvcUID types.UID,
) error {
	pvToCreate := &v1.PersistentVolume{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:   pvName,
			Labels: map[string]string{"app": tridentLabelValue},
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceStorage: *resource.NewQuantity(volBytes,
					resource.BinarySI),
			},
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			ClaimRef: &v1.ObjectReference{
				Namespace: k.namespace,
				Name:      pvcName,
				UID:       pvcUID,
			},
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
		},
	}
	switch {
	case volConfig.AccessInfo.NfsAccessInfo.NfsServerIP != "":
		pvToCreate.Spec.NFS = k8sfrontend.CreateNFSVolumeSource(volConfig)
	case volConfig.AccessInfo.IscsiAccessInfo.IscsiTargetPortal != "":
		pvToCreate.Spec.ISCSI = k8sfrontend.CreateISCSIVolumeSource(volConfig)
	default:
		return fmt.Errorf("Cannot create PV.  Unrecognized volume type.")
	}
	_, err := k.clientset.Core().PersistentVolumes().Create(pvToCreate)
	if err != nil {
		return err
	}
	return err
}

func (k *KubeClient) CreateKubeVolume(volConfig *storage.VolumeConfig) (
	bool, error) {
	pvcUID, err := k.createPVC()
	if err != nil {
		return false, fmt.Errorf("Unable to create PVC:  %v", err)
	}
	err = k.createPV(volConfig, pvcUID)
	if err != nil {
		return false, fmt.Errorf("Unable to create PV:  %v", err)
	}
	return k.waitForPVCBind()
}

func (k *KubeClient) RemoveInMemoryTrident() error {
	err := k.clientset.Core().Pods(k.namespace).Delete(inMemoryTridentName,
		&v1.DeleteOptions{})
	if err != nil {
		return err
	}
	return k.waitInMemoryTridentDeletion(inMemoryTridentName)
}
