// Copyright 2016 NetApp, Inc. All Rights Reserved.

package rest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type FakeTridentClient struct {
	volumes    map[string]storage.VolumeExternal
	failMatrix map[string]bool
}

func NewFakeTridentClient(failMatrix map[string]bool) *FakeTridentClient {
	return &FakeTridentClient{
		volumes:    make(map[string]storage.VolumeExternal, 0),
		failMatrix: failMatrix,
	}
}

func (client *FakeTridentClient) Configure(ip string, port, timeout int) Interface {
	return client
}

func (client *FakeTridentClient) Get(endpoint string) (*http.Response, error) {
	if fail, ok := client.failMatrix["Get"]; fail && ok {
		return nil, fmt.Errorf("Get failed")
	}
	return nil, nil
}

func (client *FakeTridentClient) Post(endpoint string, body io.Reader) (*http.Response, error) {
	if fail, ok := client.failMatrix["Post"]; fail && ok {
		return nil, fmt.Errorf("Post failed")
	}
	return nil, nil
}

func (client *FakeTridentClient) Delete(endpoint string) (*http.Response, error) {
	if fail, ok := client.failMatrix["Delete"]; fail && ok {
		return nil, fmt.Errorf("Delete failed")
	}
	return nil, nil
}

func (client *FakeTridentClient) GetBackend(backendID string) (*GetBackendResponse, error) {
	return nil, nil
}

func (client *FakeTridentClient) PostBackend(backendFile string) (*AddBackendResponse, error) {
	return nil, nil
}

func (client *FakeTridentClient) ListBackends() (*ListBackendsResponse, error) {
	return nil, nil
}

func (client *FakeTridentClient) AddStorageClass(storageClassConfig *storage_class.Config) (*AddStorageClassResponse, error) {
	return nil, nil
}

func (client *FakeTridentClient) GetVolume(volName string) (*GetVolumeResponse, error) {
	var (
		err               error
		ok                = false
		vol               storage.VolumeExternal
		getVolumeResponse GetVolumeResponse
	)
	if _, err = client.Get("volume/" + volName); err != nil {
		return nil, err
	}
	if vol, ok = client.volumes[volName]; !ok {
		getVolumeResponse.Volume = nil
		getVolumeResponse.Error = "Volume wasn't found"
		return &getVolumeResponse, nil
	}
	getVolumeResponse.Volume = &vol
	if fail, ok := client.failMatrix["GetVolume"]; fail && ok {
		getVolumeResponse.Error = "GetVolume failed"
		return nil, fmt.Errorf("GetVolume failed.")
	}
	return &getVolumeResponse, nil
}

func (client *FakeTridentClient) AddVolume(volConfig *storage.VolumeConfig) (*AddVolumeResponse, error) {
	var err error = nil
	if _, err = client.Post("", bytes.NewBuffer(make([]byte, 0))); err != nil {
		return nil, err
	}
	client.volumes[volConfig.Name] = storage.VolumeExternal{
		Config:  volConfig,
		Backend: "ontapnas_1.1.1.1",
		Pool:    "aggr1",
	}
	addVolumeResponse := &AddVolumeResponse{
		BackendID: "ontapnas_1.1.1.1",
	}
	if fail, ok := client.failMatrix["AddVolume"]; fail && ok {
		addVolumeResponse.Error = "AddVolume failed"
		return nil, fmt.Errorf("AddVolume failed.")
	}
	return addVolumeResponse, nil
}

func (client *FakeTridentClient) DeleteVolume(volName string) (*DeleteResponse, error) {
	var (
		err            error
		ok             = false
		deleteResponse DeleteResponse
	)
	if _, err = client.Delete("volume/" + volName); err != nil {
		return nil, err
	}
	if _, ok = client.volumes[volName]; !ok {
		deleteResponse.Error = "Volume wasn't found"
		return &deleteResponse, nil
	}
	delete(client.volumes, volName)
	if fail, ok := client.failMatrix["DeleteVolume"]; fail && ok {
		deleteResponse.Error = "DeleteVolume failed"
		return nil, fmt.Errorf("DeleteVolume failed.")
	}
	return &deleteResponse, nil
}
