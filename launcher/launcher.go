// Copyright 2016 NetApp, Inc. All Rights Reserved.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	k8srest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
)

var (
	apiServerIP = flag.String("apiserver", "", "Kubernetes API server IP "+
		"address.")
	backendFile = flag.String("backend", "/etc/config/backend.json",
		"Name of the configuration file for the backend that will host the "+
			"etcd volume.")
	deploymentFile = flag.String("deployment_file",
		"/etc/config/trident-deployment.yaml",
		"Deployment definition file for Trident")
	debug = flag.Bool("debug", false, "Enable debug output.")
)

const (
	scName  = "trident-basic"
	volName = "trident"

	volGB              int = 1
	maxTries               = 10
	defaultTridentPort     = 8000
)

func WaitOnline(ip string) error {
	for tries := 0; tries < maxTries; tries++ {
		log.Debug("Checking that ephemeral Trident is online")
		_, err := http.Get(fmt.Sprintf("http://%s:%d/trident/v1/backend",
			ip, defaultTridentPort))
		if err == nil {
			return nil
		}
		if _, ok := err.(net.Error); !ok {
			return err
		}
		// Assume that net errors are likely to be transient (e.g., connection
		// refused).  Even if they aren't, retrying here won't hurt.
		log.Debug("Connection to ephemeral Trident got network error; " +
			"retrying.")
		time.Sleep(time.Second)
	}
	return fmt.Errorf("Unable to connect to ephemeral Trident after %d "+
		"seconds.", maxTries)
}

func PostBackend(ip string) (backendName string, err error) {
	var backendResponse rest.AddBackendResponse

	jsonBytes, err := ioutil.ReadFile(*backendFile)
	if err != nil {
		return "", err
	}
	resp, err := http.Post(fmt.Sprintf("http://%s:%d/trident/v1/backend", ip,
		defaultTridentPort), "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(body, &backendResponse)
	if err != nil {
		return "", err
	}
	if backendResponse.Error != "" {
		return "", fmt.Errorf("%s", backendResponse.Error)
	}
	return backendResponse.BackendID, nil
}

func GetBackendStoragePools(ip, backendName string) ([]string, error) {
	// Unmarshaling the entire backend is difficult, so just get what we need.
	type partialBackend struct {
		Backend struct {
			Name    string                      `json:"name"`
			Storage map[string]*json.RawMessage `json:"storage"`
		} `json:"backend"`
		Error string `json:"error"`
	}
	var backendResponse partialBackend
	storagePoolNames := make([]string, 0)

	resp, err := http.Get(
		fmt.Sprintf("http://%s:%d/trident/v1/backend/%s", ip, defaultTridentPort,
			backendName))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &backendResponse)
	if err != nil {
		return nil, err
	}
	if backendResponse.Error != "" {
		return nil, fmt.Errorf("%s", backendResponse.Error)
	}
	for name, _ := range backendResponse.Backend.Storage {
		storagePoolNames = append(storagePoolNames, name)
	}
	return storagePoolNames, nil
}

// Do this against the Trident API, rather than Kubernetes, so that we don't
// have an extra storage class lying around Kubernetes
func PostStorageClass(ip, backendName string, storagePoolList []string) error {
	var scResponse rest.AddStorageClassResponse

	scConfig := sc.Config{
		Version:             "v1",
		Name:                scName,
		BackendStoragePools: map[string][]string{backendName: storagePoolList},
	}
	jsonBytes, err := json.Marshal(&scConfig)
	if err != nil {
		return err
	}
	resp, err := http.Post(fmt.Sprintf("http://%s:%d/trident/v1/storageclass",
		ip, defaultTridentPort), "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &scResponse)
	if err != nil {
		return err
	}
	if scResponse.Error != "" {
		return fmt.Errorf("%s", scResponse.Error)
	}
	return nil
}

func GetVolume(ip, name string) (*storage.VolumeConfig, error) {
	var volResponse rest.GetVolumeResponse

	log.Debug("Retrieving created volume.")
	resp, err := http.Get(
		fmt.Sprintf("http://%s:%d/trident/v1/volume/%s", ip,
			defaultTridentPort, name))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Debug("IO error.")
		return nil, err
	}

	err = json.Unmarshal(body, &volResponse)
	if err != nil {
		log.Debug("Failed to unmarshal.  JSON:  ", string(body))
		return nil, err
	}
	if volResponse.Error != "" {
		log.Debug("Backend error.")
		return nil, fmt.Errorf("%s", volResponse.Error)
	}

	return volResponse.Volume.Config, err
}

func PostVolume(ip string) (*storage.VolumeConfig, error) {
	var volResponse rest.AddVolumeResponse
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		Size:         fmt.Sprintf("%dGB", volGB),
		StorageClass: scName,
	}
	jsonBytes, err := json.Marshal(&volConfig)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(fmt.Sprintf("http://%s:%d/trident/v1/volume",
		ip, defaultTridentPort), "application/json",
		bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &volResponse)
	if err != nil {
		return nil, err
	}
	if volResponse.Error != "" {
		return nil, fmt.Errorf("%s", volResponse.Error)
	}
	return GetVolume(ip, volName)
}

func ProvisionVolume(kubeClient *KubeClient) {
	tridentIP, err := kubeClient.StartInMemoryTrident()
	if err != nil {
		log.Fatalf("Unable to start Trident:  %v\nCleanup of the Trident pod "+
			"may be necessary.", err)
	}
	log.Debugf("Started Trident at %s", tridentIP)
	err = WaitOnline(tridentIP)
	if err != nil {
		log.Fatalf("%v", err)
	}
	backendName, err := PostBackend(tridentIP)
	if err != nil {
		log.Fatalf("Unable to post backend:  %v\nCleanup of the Trident pod is"+
			" necessary", err)
	}
	log.Debugf("Added backend %s", backendName)
	storagePoolNames, err := GetBackendStoragePools(tridentIP, backendName)
	if err != nil {
		log.Fatalf("Unable to retrieve storagePool names:  %v\nCleanup of "+
			"the Trident pod is necessary.", err)
	}
	log.Debug("StoragePool names:  ", strings.Join(storagePoolNames, ", "))
	err = PostStorageClass(tridentIP, backendName, storagePoolNames)
	if err != nil {
		log.Fatalf("Unable to post storage class:  %v\nCleanup of the Trident "+
			"pod is necessary", err)
	}
	log.Debug("Added storage class.")
	volConfig, err := PostVolume(tridentIP)
	if err != nil {
		log.Fatalf("Unable to post volume:  %v\nCleanup of the Trident pod "+
			"is necessary", err)
	}
	log.Debug("Provisioned volume with configuration:  ", volConfig)
	bound, err := kubeClient.CreateKubeVolume(volConfig)
	if err != nil {
		log.Fatalf("Unable to create PVC and PV:  %v\nCleanup of Trident pod "+
			"and/or PVC may be necessary", err)
	}
	if bound {
		log.Infof("Provisioned volume %s for Trident", volConfig.Name)
	} else {
		log.Warnf("PVC bound to pre-existing volume; clean up volume %s.",
			volConfig.Name)
	}
	log.Debug("Provisioned volume.")
	err = kubeClient.RemoveInMemoryTrident()
	if err != nil {
		log.Fatalf("Unable to delete ephememral Trident:  %v\nRemove it "+
			"manually before starting the pod.", err)
	}
}

func main() {
	var (
		config *k8srest.Config
		err    error
	)

	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	if *apiServerIP != "" {
		config, err = clientcmd.BuildConfigFromFlags(*apiServerIP, "")
		if err != nil {
			log.Fatal("Unable to get client config:  ", err)
		}
	} else {
		config, err = k8srest.InClusterConfig()
		if err != nil {
			log.Fatal("Unable to get client config through service account: ",
				err)
		}
	}
	kubeClient, err := NewKubeClient(config, *deploymentFile)
	if err != nil {
		log.Fatal("Unable to construct client:  ", err)
	}

	running, err := kubeClient.CheckTridentRunning()
	if err != nil {
		log.Fatal("Unable to query whether Trident is running:  ", err)
	}
	if running {
		log.Info("Trident already running.")
		return
	}

	exists, err := kubeClient.PVCExists()
	if err != nil {
		log.Fatalf("Unable to query %s PVC existence:  %v", pvcName, err)
	}
	if exists {
		// At some point, we may want to ensure that the PVC is bound and not
		// errored.  For now, though, assume that the user will delete the PVC
		// if it is errored.
		log.Info("Trident PVC already exists; starting the pod.")
	} else {
		ProvisionVolume(kubeClient)
	}
	log.Infof("Provisioned PVC %s; starting Trident.", pvcName)
	err = kubeClient.StartFullTrident()
	if err != nil {
		log.Fatal("Unable to start full Trident deployment:  ", err)
	}
	log.Info("Trident is now running.")
}
