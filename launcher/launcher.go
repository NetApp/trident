// Copyright 2016 NetApp, Inc. All Rights Reserved.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/trident/k8s_client"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	k8srest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	k8s_util_version "k8s.io/kubernetes/pkg/util/version"

	"github.com/netapp/trident/config"
	k8sfrontend "github.com/netapp/trident/frontend/kubernetes"
	tridentrest "github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

const (
	tridentContainerName    = config.ContainerTrident
	tridentEphemeralPodName = "trident-ephemeral"
	tridentDefaultPort      = 8000
	tridentStorageClassName = "trident-basic"
	tridentNamespaceFile    = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	//based on the number of seconds Trident waits on etcd to bootstrap
	bootstrappingTimeout int64 = 11
)

var (
	apiServerIP = flag.String("apiserver", "", "Kubernetes API server IP address.")
	backendFile = flag.String("backend", "/etc/config/backend.json",
		"Configuration file for the backend that will host the etcd volume.")
	deploymentFile = flag.String("deployment_file",
		"/etc/config/trident-deployment.yaml", "Deployment definition file for Trident")
	debug                  = flag.Bool("debug", false, "Enable debug output.")
	k8sTimeout             = flag.Int64("k8s_timeout", 60, "The number of seconds to wait before timing out on Kubernetes operations.")
	tridentVolumeSize      = flag.Int("volume_size", 1, "The size of the volume provisioned by launcher in GB.")
	tridentVolumeName      = flag.String("volume_name", "trident", "The name of the volume used by etcd.")
	tridentPVCName         = flag.String("pvc_name", "trident", "The name of the PVC used by Trident.")
	tridentPVName          = flag.String("pv_name", "trident", "The name of the PV used by Trident.")
	tridentTimeout         = flag.Int("trident_timeout", 10, "The number of seconds to wait before timing out on a Trident connection.")
	tridentImage           = ""
	tridentNamespace       = ""
	tridentLabels          map[string]string
	tridentEphemeralLabels = map[string]string{"app": "trident-launcher.netapp.io"}
)

// createTridentDeploymentFromFile creates a deployment object from a file.
func createTridentDeploymentFromFile(deploymentFile string) (*v1beta1.Deployment, error) {
	var deployment v1beta1.Deployment

	yamlBytes, err := ioutil.ReadFile(deploymentFile)
	if err != nil {
		return nil, err
	}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(yamlBytes), 512).Decode(
		&deployment)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// createTridentEphemeralPod creates the ephemeral Trident pod.
func createTridentEphemeralPod(kubeClient k8s_client.Interface) (*v1.Pod, error) {
	// Check if the pod already exists
	exists, err := kubeClient.CheckPodExists(tridentEphemeralPodName)
	if err != nil {
		return nil,
			fmt.Errorf("Launcher couldn't detect the presence of %s pod: %s",
				tridentEphemeralPodName, err)
	}
	if exists {
		return nil, fmt.Errorf("Please run 'kubectl delete pod %s' and try again!",
			tridentEphemeralPodName)
	}

	// Create the pod
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tridentEphemeralPodName,
			Namespace: tridentNamespace,
			Labels:    tridentEphemeralLabels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name:    tridentContainerName,
					Image:   tridentImage,
					Command: []string{"/usr/local/bin/trident_orchestrator"},
					Args: []string{
						"-port", fmt.Sprintf("%d", tridentDefaultPort),
						"-address", "",
						"-no_persistence",
					},
					Ports: []v1.ContainerPort{
						v1.ContainerPort{ContainerPort: tridentDefaultPort},
					},
				},
			},
		},
	}
	if *debug {
		pod.Spec.Containers[0].Args = append(pod.Spec.Containers[0].Args,
			"-debug")
	}
	return kubeClient.CreatePod(pod)
}

// stopTridentEphemeralPod stops the ephemeral Trident pod.
func stopTridentEphemeralPod(kubeClient k8s_client.Interface) error {
	var gracePeriod int64 = 0
	options := &metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	}
	return kubeClient.DeletePod(tridentEphemeralPodName, options)
}

// checkTridentAPI checks the responsiveness of a Trident pod.
func checkTridentAPI(tridentClient tridentrest.Interface, podName string) error {
	var err error
	startTime := time.Now()
	for t := 0; time.Since(startTime) < time.Duration(bootstrappingTimeout)*time.Second; t++ {
		if err != nil {
			log.Debugf("Launcher validating pod %s has bootstrapped (trial #%d: %v).",
				podName, t, err)
		}
		_, err = tridentClient.ListBackends()
		if err == nil {
			log.Debugf("Launcher validated pod %s has bootstrapped after %v.",
				podName, time.Since(startTime))
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("Pod %s isn't running after %v (%s).",
		podName, time.Since(startTime), err)
}

// addBackend adds a backend to a Trident pod
func addBackend(tridentClient tridentrest.Interface,
	fileName string) (string, error) {
	addBackendResponse, err := tridentClient.PostBackend(fileName)
	if err != nil {
		return "",
			fmt.Errorf("Launcher failed in communication with Pod %s: %s",
				tridentEphemeralPodName, err)
	} else if addBackendResponse.Error != "" {
		return "", fmt.Errorf("Pod %s failed in adding a backend: %s",
			tridentEphemeralPodName, addBackendResponse.Error)
	}
	log.Infof("Launcher successfully added backend %s to pod %s.",
		addBackendResponse.BackendID, tridentEphemeralPodName)
	return addBackendResponse.BackendID, nil
}

// getStoragePools retrieves the storage pools from a given backend added to a Trident pod.
func getStoragePools(tridentClient tridentrest.Interface,
	backendID string) ([]string, error) {
	/* //TODO: Fix the unmarshaling problem with  StorageBackendExternal.Storage.Attributes
	getBackendResponse, err := tridentClient.GetBackend(addBackendResponse.BackendID)
	if err != nil {
		return nil, fmt.Errorf("Launcher failed in communication with Pod %s: %s",
			tridentEphemeralPodName, err)
	} else if addBackendResponse.Error != "" {
		return nil, fmt.Errorf("Pod %s failed in getting a backend: %s",
			tridentEphemeralPodName, getBackendResponse.Error)
	}
	log.Infof("Launcher retrieved storage pools for backend %s: %v",
		addBackendResponse.BackendID, getBackendResponse.Backend.Storage)
	*/
	// Workaround for the unmarshalling problem
	type partialBackend struct {
		Backend struct {
			Name    string                      `json:"name"`
			Storage map[string]*json.RawMessage `json:"storage"`
		} `json:"backend"`
		Error string `json:"error"`
	}
	var (
		resp            *http.Response
		backendResponse partialBackend
		err             error
		bytes           []byte
		storagePools    []string = make([]string, 0)
	)

	if resp, err = tridentClient.Get("backend/" + backendID); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if bytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(bytes, &backendResponse); err != nil {
		return nil, err
	}
	if backendResponse.Error != "" {
		return nil, fmt.Errorf("%s", backendResponse.Error)
	}
	for pool, _ := range backendResponse.Backend.Storage {
		storagePools = append(storagePools, pool)
	}
	return storagePools, nil
}

// addStorageClass adds a storage class to a Trident pod.
func addStorageClass(tridentClient tridentrest.Interface,
	storageClassConfig *storage_class.Config) error {
	addStorageClassResponse,
		err := tridentClient.AddStorageClass(storageClassConfig)
	if err != nil {
		return err
	} else if addStorageClassResponse.Error != "" {
		return fmt.Errorf("Pod %s failed in adding storage class %s: %s",
			tridentEphemeralPodName, storageClassConfig.Name,
			addStorageClassResponse.Error)
	}
	log.Infof("Launcher successfully added storage class %s to pod %s.",
		addStorageClassResponse.StorageClassID, tridentEphemeralPodName)
	return nil
}

// checkVolumeExists checks whether a Trident pod has created a volume with
// the given name.
func checkVolumeExists(
	tridentClient tridentrest.Interface,
	volName string) (bool, error) {
	getVolResponse, err := tridentClient.GetVolume(volName)
	if err != nil {
		return false, err
	} else if strings.Contains(getVolResponse.Error, "not found") {
		return false, nil
	} else if getVolResponse.Error == "" {
		if getVolResponse.Volume.Config.Name == volName {
			return true, nil
		}
	}
	return false, fmt.Errorf(getVolResponse.Error)
}

// getVolume retrieves a volume from a Trident pod.
func getVolume(tridentClient tridentrest.Interface,
	volName string) (*storage.VolumeExternal, error) {
	getVolumeResponse, err := tridentClient.GetVolume(volName)
	if err != nil {
		return nil, fmt.Errorf("Launcher failed in getting volume %s: %s",
			volName, err)
	} else if getVolumeResponse.Error != "" {
		return nil, fmt.Errorf("Pod %s failed in getting volume %s: %s",
			tridentEphemeralPodName, volName, getVolumeResponse.Error)
	}
	return getVolumeResponse.Volume, nil
}

// addVolume adds a volume to a Trident pod.
func addVolume(tridentClient tridentrest.Interface, volConfig *storage.VolumeConfig) error {
	addVolumeResponse, err := tridentClient.AddVolume(volConfig)
	if err != nil {
		return fmt.Errorf("%s", err)
	} else if addVolumeResponse.Error != "" {
		return fmt.Errorf("%s", addVolumeResponse.Error)
	}
	return nil
}

// deleteVolume deletes a volume from a Trident pod.
func deleteVolume(tridentClient tridentrest.Interface, volName string) error {
	deleteResponse, err := tridentClient.DeleteVolume(volName)
	if err != nil {
		return fmt.Errorf("%s", err)
	} else if deleteResponse.Error != "" {
		return fmt.Errorf("%s", deleteResponse.Error)
	}
	return nil
}

// provisionVolume undertakes all the steps required to provision a volume on a
// new Trident pod.
func provisionVolume(tridentClient tridentrest.Interface) error {
	// Add a backend
	backendID, err := addBackend(tridentClient, *backendFile)
	if err != nil {
		return err
	}

	// Get backend storage pool to define a storage class
	pools, err := getStoragePools(tridentClient, backendID)
	if err != nil {
		return err
	}

	// Create the storage class
	storageClassConfig := &storage_class.Config{
		Version:             "v1",
		Name:                tridentStorageClassName,
		BackendStoragePools: map[string][]string{backendID: pools},
	}

	// Add the storage class
	if err := addStorageClass(tridentClient, storageClassConfig); err != nil {
		return err
	}

	// Create the volume
	volConfig := &storage.VolumeConfig{
		Name:         *tridentVolumeName,
		Size:         fmt.Sprintf("%dGB", *tridentVolumeSize),
		StorageClass: tridentStorageClassName,
	}
	return addVolume(tridentClient, volConfig)
}

// createPVC creates a PVC in the Kubernetes cluster.
func createPVC(kubeClient k8s_client.Interface, pvcName string) (*v1.PersistentVolumeClaim, error) {
	pvc := &v1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: tridentNamespace,
			Labels:    tridentLabels,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: *resource.NewQuantity(
						int64(*tridentVolumeSize)*1073741824,
						resource.BinarySI),
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: tridentLabels,
			},
		},
	}
	return kubeClient.CreatePVC(pvc)
}

// createPV creates a PV in the Kubernetes cluster.
func createPV(kubeClient k8s_client.Interface, pvName string,
	vol *storage.VolumeExternal,
	pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolume, error) {
	volConfig := vol.Config
	pv := &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   pvName,
			Labels: tridentLabels,
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceStorage: *resource.NewQuantity(
					int64(*tridentVolumeSize)*1073741824,
					resource.BinarySI),
			},
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			ClaimRef: &v1.ObjectReference{
				Namespace: pvc.Namespace,
				Name:      pvc.Name,
				UID:       pvc.UID,
			},
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
		},
	}
	switch {
	case volConfig.AccessInfo.NfsAccessInfo.NfsServerIP != "":
		pv.Spec.NFS = k8sfrontend.CreateNFSVolumeSource(vol)
	case volConfig.AccessInfo.IscsiAccessInfo.IscsiTargetPortal != "":
		kubeVersion, err := k8sfrontend.ValidateKubeVersion(kubeClient.Version())
		if err != nil {
			return nil, fmt.Errorf("Error validating Kubernetes version %v", err.Error())
		}
		pv.Spec.ISCSI, err = k8sfrontend.CreateISCSIVolumeSource(kubeClient, kubeVersion, vol)
		if err != nil {
			return nil, fmt.Errorf("Error creating ISCSI volume source %v", err.Error())
		}
		log.WithFields(log.Fields{
			"pvName":            pvName,
			"volumeExternal":    vol,
			"volConfig":         volConfig,
			"kubeVersion":       kubeVersion,
			"iSCSIVolumeSource": pv.Spec.ISCSI,
		}).Info("Created iSCSIVolumeSource with fields")

	default:
		return nil, fmt.Errorf("Unrecognized volume type")
	}
	return kubeClient.CreatePV(pv)
}

type Launcher struct {
	kubeClient             k8s_client.Interface
	tridentClient          tridentrest.Interface
	tridentEphemeralClient tridentrest.Interface
	tridentDeployment      *v1beta1.Deployment
}

// NewLauncher creates a new launcher object.
func NewLauncher(kubeClient k8s_client.Interface, tridentClient tridentrest.Interface,
	tridentEphemeralClient tridentrest.Interface,
	tridentDeployment *v1beta1.Deployment) *Launcher {
	return &Launcher{
		kubeClient:             kubeClient,
		tridentClient:          tridentClient,
		tridentEphemeralClient: tridentEphemeralClient,
		tridentDeployment:      tridentDeployment,
	}
}

// ValidateVersion checks whether the container orchestrator version is
// supported or not.
func (launcher *Launcher) ValidateKubeVersion(versionInfo *version.Info) (*k8s_util_version.Version, error) {
	return k8sfrontend.ValidateKubeVersion(versionInfo)
}

// Run runs the launcher.
func (launcher *Launcher) Run() (errors []error) {
	var (
		deleteTridentEphemeral                            = false
		deploymentExists                                  = false
		deploymentCreated                                 = false
		err                     error                     = nil
		launcherErr             error                     = nil
		pv                      *v1.PersistentVolume      = nil
		pvExists                                          = false
		pvcExists                                         = false
		pvcCreated                                        = false
		pvc                     *v1.PersistentVolumeClaim = nil
		pvCreated                                         = false
		tridentEphemeralCreated                           = false
		tridentEphemeralPod     *v1.Pod
		tridentPod              *v1.Pod
		volConfig               *storage.VolumeConfig
		volumeCreated           = false
	)

	errors = make([]error, 0)

	defer func() {
		// Cleanup after success (err == nil)
		if launcherErr == nil && tridentEphemeralCreated {
			// Delete pod trident-ephemeral
			if errCleanup := stopTridentEphemeralPod(launcher.kubeClient); errCleanup != nil {
				log.WithFields(log.Fields{
					"error": errCleanup,
					"pod":   tridentEphemeralPodName,
				}).Error("Launcher failed to delete the pod, so it needs " +
					"to be manually deleted!")
				errors = append(errors,
					fmt.Errorf("Launcher failed to delete pod %s: %s. "+
						"Manual deletion is required!",
						tridentEphemeralPodName, errCleanup))
			} else {
				log.WithFields(log.Fields{
					"pod": tridentEphemeralPodName,
				}).Info("Launcher successfully deleted the pod during cleanup.")
			}
		} else if launcherErr != nil {
			log.Error(launcherErr)
			errors = append(errors, launcherErr)

			// Cleanup after failure (err != nil)
			var gracePeriod int64 = 0
			options := &metav1.DeleteOptions{
				GracePeriodSeconds: &gracePeriod,
			}
			log.WithFields(log.Fields{
				"error":                   err,
				"pvcCreated":              pvcCreated,
				"pvCreated":               pvCreated,
				"volumeCreated":           volumeCreated,
				"deploymentCreated":       deploymentCreated,
				"tridentEphemeralCreated": tridentEphemeralCreated,
			}).Debug("Launcher is starting the cleanup after failure.")
			if pvcCreated && !deploymentCreated {
				// Delete the PVC
				if errCleanup := launcher.kubeClient.DeletePVC(*tridentPVCName, options); errCleanup != nil {
					log.WithFields(log.Fields{
						"error": errCleanup,
						"pvc":   *tridentPVCName,
					}).Error("Launcher failed to delete the PVC during cleanup. " +
						"Manual deletion is required!")
					errors = append(errors,
						fmt.Errorf("Launcher failed to delete PVC %s during cleanup: %s. "+
							"Manual deletion is required!", *tridentPVCName, errCleanup))
				} else {
					log.WithFields(log.Fields{
						"pvc": *tridentPVCName,
					}).Info("Launcher successfully deleted the PVC during cleanup.")
				}
			}
			if pvCreated && !deploymentCreated {
				// Delete the PV
				if errCleanup := launcher.kubeClient.DeletePV(*tridentPVName, options); errCleanup != nil {
					log.WithFields(log.Fields{
						"error": errCleanup,
						"pv":    *tridentPVName,
					}).Error("Launcher failed to delete the PV during cleanup! " +
						"Manual deletion is required!")
					errors = append(errors,
						fmt.Errorf("Launcher failed to delete PV %s during cleanup: %s. "+
							"Manual deletion is required!", *tridentPVName, errCleanup))
				} else {
					log.WithFields(log.Fields{
						"pv": *tridentPVName,
					}).Info("Launcher successfully deleted the PV during cleanup.")
				}
			}
			if volumeCreated && !deploymentCreated {
				// Delete the volume
				if errCleanup := deleteVolume(launcher.tridentEphemeralClient, *tridentVolumeName); errCleanup != nil {
					log.WithFields(log.Fields{
						"error":  errCleanup,
						"volume": *tridentVolumeName,
					}).Error("Launcher failed to delete the volume during cleanup. "+
						"Manual deletion is required! "+
						"Run 'kubectl logs %s' for more information.",
						tridentEphemeralPodName)
					errors = append(errors,
						fmt.Errorf("Launcher failed to delete volume %s during cleanup: %s. "+
							"Manual deletion is required! "+
							"Run 'kubectl logs %s' for more information.",
							*tridentVolumeName, errCleanup))
					deleteTridentEphemeral = false
				} else {
					log.WithFields(log.Fields{
						"volume": *tridentVolumeName,
					}).Info("Launcher successfully deleted the volume during cleanup.")
				}
			}
			if tridentEphemeralCreated && (deploymentCreated || deleteTridentEphemeral) {
				//TODO: Capture the logs for pod trident-ephemeral
				/* kubectl logs trident-ephemeral --v=8
				 * https://kubernetes.io/docs/api-reference/v1/operations/
				 * https://kubernetes.io/docs/admin/authorization/
				 */
				// Delete pod trident-ephemeral
				if errCleanup := stopTridentEphemeralPod(launcher.kubeClient); errCleanup != nil {
					log.WithFields(log.Fields{
						"error": errCleanup,
						"pod":   tridentEphemeralPodName,
					}).Error("Launcher failed to delete the pod. " +
						"Manual deletion is required!")
					errors = append(errors,
						fmt.Errorf("Launcher failed to delete pod %s: %s. "+
							"Manual deletion is required!",
							tridentEphemeralPodName, errCleanup))
				} else {
					log.Infof("Launcher successfully deleted pod %s during cleanup.",
						tridentEphemeralPodName)
				}
			}
		}
	}()

	// Check for an existing Trident deployment
	if deploymentExists, err = launcher.kubeClient.CheckDeploymentExists(launcher.tridentDeployment.Name); deploymentExists {
		launcherErr = fmt.Errorf("Launcher detected a preexisting deployment "+
			"called %s, so it will quit!", launcher.tridentDeployment.Name)
		return
	} else if err != nil {
		launcherErr = fmt.Errorf("Launcher couldn't establish the presence "+
			"of deployment %s: %s. Please check your service account setup.",
			launcher.tridentDeployment.Name, err)
		return
	}

	// Check for an existing PVC for Trident
	if pvcExists, err = launcher.kubeClient.CheckPVCExists(*tridentPVCName); pvcExists {
		var (
			options metav1.GetOptions
			phase   v1.PersistentVolumeClaimPhase
		)
		log.WithFields(log.Fields{
			"pvc": *tridentPVCName,
		}).Info("Launcher detected a preexisting PVC. It assumes " +
			"this PVC was created for the Trident deployment.")
		phase, err = launcher.kubeClient.GetPVCPhase(*tridentPVCName, options)
		if err != nil {
			launcherErr = fmt.Errorf(
				"Launcher couldn't detect the phase for PVC %s: %s",
				*tridentPVCName, err)
			return
		}
		switch phase {
		case v1.ClaimPending:
			log.WithFields(log.Fields{
				"pvc": *tridentPVCName,
			}).Info("Launcher detected that the PVC is still pending; " +
				"proceeding with the creation of the corresponding PV.")
		case v1.ClaimBound:
			pvExists = true
			log.WithFields(log.Fields{
				"pvc": *tridentPVCName,
			}).Info("Launcher detected that the PVC is bound; proceeding " +
				"with the creation of the Trident deployment.")
		case v1.ClaimLost:
			launcherErr = fmt.Errorf("Please delete the preexisting PVC %s "+
				"and try again.", *tridentPVCName)
			return
		}
	} else if err != nil {
		launcherErr = fmt.Errorf("Launcher couldn't establish the presence "+
			"of PVC %s: %s", *tridentPVCName, err)
		return
	}

	if !pvExists {
		// Start ephemeral Trident
		if tridentEphemeralPod, err = createTridentEphemeralPod(launcher.kubeClient); err != nil {
			launcherErr = fmt.Errorf("Launcher failed to launch pod %s: %s",
				tridentEphemeralPodName, err)
			return
		}
		log.WithFields(log.Fields{
			"pod": tridentEphemeralPodName,
		}).Info("Launcher created the pod.")
		tridentEphemeralCreated = true

		// Wait for the pod to run
		if tridentEphemeralPod, err = launcher.kubeClient.GetRunningPod(
			tridentEphemeralPod, k8sTimeout,
			tridentEphemeralLabels); err != nil {
			launcherErr = err
			return
		}

		// Create Trident client
		if tridentEphemeralPod.Status.PodIP == "" {
			launcherErr = fmt.Errorf("Pod %s doesn't have an IP address!",
				tridentEphemeralPod.Name)
			return
		}
		log.WithFields(log.Fields{
			"pod":       tridentEphemeralPod.Name,
			"ipAddress": tridentEphemeralPod.Status.PodIP,
		}).Infof("Launcher detected the IP address for the pod.")
		launcher.tridentEphemeralClient.Configure(tridentEphemeralPod.Status.PodIP,
			tridentDefaultPort, *tridentTimeout)

		// Check the pod is functional
		if err = checkTridentAPI(launcher.tridentEphemeralClient, tridentEphemeralPod.Name); err != nil {
			launcherErr = fmt.Errorf(
				"Launcher failed to bring up a functional pod: %s "+
					"Try 'kubectl logs %s' to diagnose the problem.",
				err, tridentEphemeralPod.Name)
			return
		}

		// Provision the volume
		if err = provisionVolume(launcher.tridentEphemeralClient); err != nil {
			launcherErr = fmt.Errorf("Launcher failed in adding volume %s: %s",
				*tridentVolumeName, err.Error())
			return
		}
		volumeCreated = true
		log.WithFields(log.Fields{
			"volume":     *tridentVolumeName,
			"volumeSize": fmt.Sprintf("%d%s", *tridentVolumeSize, "GB"),
		}).Info("Launcher successfully created the volume.")
		deleteTridentEphemeral = true
	}

	if !pvcExists {
		// Create the PVC
		if pvc, err = createPVC(launcher.kubeClient, *tridentPVCName); err != nil {
			launcherErr = fmt.Errorf("Launcher failed in creating PVC %s: %s",
				*tridentPVCName, err)
			return
		}
		pvcCreated = true
		log.WithFields(log.Fields{
			"pvc": *tridentPVCName,
		}).Info("Launcher successfully created the PVC.")
	} else {
		// Retrieve the preexisting PVC
		var options metav1.GetOptions
		if pvc, err = launcher.kubeClient.GetPVC(*tridentPVCName, options); err != nil {
			launcherErr = fmt.Errorf("Launcher failed in getting PVC %s: %s",
				*tridentPVCName, err)
			return
		}
	}

	// At this point, we should have created the volume and the PVC for the
	// stateful Trident pod.
	if !pvExists {
		// Create the pre-bound PV
		// Get the volume information
		var volumeExternal *storage.VolumeExternal
		if volumeExternal, err = getVolume(launcher.tridentEphemeralClient, *tridentVolumeName); err != nil {
			launcherErr = err
			return
		}
		volConfig = volumeExternal.Config
		if pv, err = createPV(launcher.kubeClient, *tridentPVName, volumeExternal, pvc); err != nil {
			launcherErr = fmt.Errorf("Launcher failed in creating PV %s: %s. "+
				"Either use -pv_name in launcher-pod.yaml to create a volume "+
				"and a PV with a different name, or delete PV %s so that "+
				"launcher can create a new PV with the same name. "+
				"(The new PV will reuse the volume represented by the old "+
				"PV unless the volume is manually deleted from the storage "+
				"backend.)",
				*tridentPVName, err, *tridentPVName)
			return
		}
		pvCreated = true
		log.WithFields(log.Fields{
			"pv":     pv.Name,
			"volume": volConfig.InternalName,
		}).Info("Launcher successfully created the PV for the volume.")

		// Wait for the Trident PVC and PV to bind
		if pvc, err = launcher.kubeClient.GetBoundPVC(pvc,
			pv, k8sTimeout, tridentLabels); err != nil {
			launcherErr = err
			return
		}
	}

	// Start the stateful Trident
	if launcher.tridentDeployment, err = launcher.kubeClient.CreateDeployment(launcher.tridentDeployment); err != nil {
		launcherErr = fmt.Errorf("Launcher failed in creating deployment %s: %s",
			launcher.tridentDeployment.Name, err)
		return
	}
	deploymentCreated = true

	// Get the stateful Trident pod
	if tridentPod, err = launcher.kubeClient.GetPodByLabels(
		k8s_client.CreateListOptions(k8sTimeout,
			tridentLabels, launcher.tridentDeployment.ResourceVersion)); err != nil {
		launcherErr = err
		return
	}
	log.WithFields(log.Fields{
		"pod":        tridentPod.Name,
		"podPhase":   tridentPod.Status.Phase,
		"deployment": launcher.tridentDeployment.Name,
	}).Info("Launcher successfully retrieved information about the deployment.")

	// Wait for a running Pod
	if tridentPod.Status.Phase != v1.PodRunning {
		if tridentPod, err = launcher.kubeClient.GetRunningPod(
			tridentPod, k8sTimeout, tridentLabels); err != nil {
			launcherErr = fmt.Errorf("%s "+
				"Try 'kubectl describe pod %s' for more information.",
				err, tridentPod.Name)
			return
		} else {
			log.WithFields(log.Fields{
				"pod":        tridentPod.Name,
				"podPhase":   tridentPod.Status.Phase,
				"deployment": launcher.tridentDeployment.Name,
			}).Info("Launcher successfully retrieved information about the deployment.")
		}
	}

	// Create the client for the stateful pod
	if tridentPod.Status.PodIP == "" {
		launcherErr = fmt.Errorf("Pod %s doesn't have an IP address!",
			tridentPod.Name)
		return
	}
	log.WithFields(log.Fields{
		"pod":       tridentPod.Name,
		"ipAddress": tridentPod.Status.PodIP,
	}).Infof("Launcher detected the IP address for the pod.")
	launcher.tridentClient = launcher.tridentClient.Configure(tridentPod.Status.PodIP,
		tridentDefaultPort, *tridentTimeout)

	// Check the pod is functional
	if err = checkTridentAPI(launcher.tridentClient, tridentPod.Name); err != nil {
		log.Warnf("%s Perhaps Trident is bootstrapping a lot of state. "+
			"Try 'kubectl logs %s -c %s' to see if Trident has bootstrapped successfully.",
			err, tridentPod.Name, tridentContainerName)
	}
	return
}

func main() {
	var (
		config            *k8srest.Config
		err               error = nil
		kubeClient        k8s_client.Interface
		tridentDeployment *v1beta1.Deployment
	)

	// Process command line arguments
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
	if *apiServerIP != "" {
		config, err = clientcmd.BuildConfigFromFlags(*apiServerIP, "")
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Fatal("Launcher is unable to get the config for the API server client!")
		}
	} else {
		config, err = k8srest.InClusterConfig()
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Fatal("Launcher is unable to get the config for the API " +
				"server client through service account!")
		}
	}

	// Read Trident deployment definition
	if tridentDeployment, err = createTridentDeploymentFromFile(*deploymentFile); err != nil {
		log.WithFields(log.Fields{
			"error":          err,
			"deploymentFile": *deploymentFile,
		}).Fatal("Launcher failed in reading the deployment file!")
	}

	// Retrieve Trident pod label, image, and namespace
	tridentLabels = tridentDeployment.Spec.Template.Labels
	if len(tridentLabels) == 0 {
		log.WithFields(log.Fields{
			"deploymentFile": *deploymentFile,
		}).Fatal("Launcher requires the deployment definition to have labels for the Trident pod!")
	}
	for _, container := range tridentDeployment.Spec.Template.Spec.Containers {
		if container.Name == tridentContainerName {
			tridentImage = container.Image
			bytes, err := ioutil.ReadFile(tridentNamespaceFile)
			if err != nil {
				log.WithFields(log.Fields{
					"error":         err,
					"namespaceFile": tridentNamespaceFile,
				}).Fatal("Launcher failed to obtain the namespace for the launcher pod!")
			}
			tridentNamespace = string(bytes)
			break
		}
	}
	if tridentImage == "" {
		log.WithFields(log.Fields{
			"container":      tridentContainerName,
			"deploymentFile": *deploymentFile,
		}).Fatal("Launcher couldn't find the Trident container in the " +
			"deployment definition!")
	}
	if tridentNamespace == "" {
		log.WithFields(log.Fields{
			"deploymentFile": *deploymentFile,
		}).Fatal("Launcher requires a namespace in the deployment definition!")
	}

	// Set up the Kubernetes API server client
	if kubeClient, err = k8s_client.NewKubeClient(config, tridentNamespace); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Launcher was unable to create an API client!")
	}
	k8sVersion := kubeClient.Version()
	log.WithFields(log.Fields{
		"major":     k8sVersion.Major,
		"minor":     k8sVersion.Minor,
		"gitCommit": k8sVersion.GitCommit,
	}).Infof("Launcher successfully retrieved the version of Kubernetes.")

	// Set up clients for the ephemeral and stateful Trident pods
	tridentEphemeralClient := &tridentrest.TridentClient{}
	tridentClient := &tridentrest.TridentClient{}

	// Create the launcher object
	launcher := NewLauncher(kubeClient, tridentClient, tridentEphemeralClient,
		tridentDeployment)

	// Check whether this version of Kubernetes is supported by Trident
	if _, err := launcher.ValidateKubeVersion(k8sVersion); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Warn("Launcher encountered an error in checking the version of " +
			"the container orchestrator!")
		log.WithFields(log.Fields{
			"yourVersion":         k8sVersion.GitVersion,
			"minSupportedVersion": k8sfrontend.KubernetesVersionMin,
			"maxSupportedVersion": k8sfrontend.KubernetesVersionMax,
		}).Warn("Launcher may not support this version of the container " +
			"orchestrator; however, it will proceed.")
	}

	// Run the launcher
	if errors := launcher.Run(); len(errors) == 0 {
		log.Info("Trident deployment was successfully launched.")
	}
}
