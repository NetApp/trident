// Copyright 2016 NetApp, Inc. All Rights Reserved.

package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

const (
	contentType = "application/json"
)

type RESTInterface interface {
	Get(endpoint string) (*http.Response, error)
	Post(endpoint string, body io.Reader) (*http.Response, error)
	Delete(endpoint string) (*http.Response, error)
}

type Interface interface {
	RESTInterface
	Configure(ip string, port, timeout int) Interface
	GetBackend(backendID string) (*GetBackendResponse, error)
	PostBackend(backendFile string) (*AddBackendResponse, error)
	ListBackends() (*ListBackendsResponse, error)
	AddStorageClass(storageClassConfig *storage_class.Config) (*AddStorageClassResponse, error)
	GetVolume(volName string) (*GetVolumeResponse, error)
	AddVolume(volConfig *storage.VolumeConfig) (*AddVolumeResponse, error)
	DeleteVolume(volName string) (*DeleteResponse, error)
}

type TridentClient struct {
	ip     string
	port   int
	client *http.Client
}

func NewTridentClient(ip string, port, timeout int) *TridentClient {
	return &TridentClient{
		ip:   ip,
		port: port,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (client *TridentClient) Configure(ip string, port, timeout int) Interface {
	client.ip = ip
	client.port = port
	client.client = &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}
	return client
}

func (client *TridentClient) Get(endpoint string) (*http.Response, error) {
	return client.client.Get(fmt.Sprintf("http://%s:%d/trident/v%s/%s",
		client.ip, client.port, config.OrchestratorAPIVersion, endpoint))
}

func (client *TridentClient) Post(endpoint string, body io.Reader) (*http.Response, error) {
	return client.client.Post(fmt.Sprintf("http://%s:%d/trident/v%s/%s",
		client.ip, client.port, config.OrchestratorAPIVersion, endpoint),
		contentType, body)
}

func (client *TridentClient) Delete(endpoint string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("http://%s:%d/trident/v%s/%s",
			client.ip, client.port, config.OrchestratorAPIVersion, endpoint),
		nil)
	if err != nil {
		return &http.Response{}, err
	}
	return client.client.Do(req)
}

func (client *TridentClient) GetBackend(backendID string) (*GetBackendResponse, error) {
	var (
		resp               *http.Response
		err                error
		bytes              []byte
		getBackendResponse GetBackendResponse
	)
	if resp, err = client.Get("backend/" + backendID); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if bytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(bytes, &getBackendResponse); err != nil {
		return nil, err
	}
	return &getBackendResponse, nil
}

func (client *TridentClient) PostBackend(backendFile string) (*AddBackendResponse, error) {
	var (
		resp               *http.Response
		err                error
		jsonBytes          []byte
		addBackendResponse AddBackendResponse
	)
	body, err := ioutil.ReadFile(backendFile)
	if err != nil {
		return nil, err
	}
	if resp, err = client.Post("backend", bytes.NewBuffer(body)); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if jsonBytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	//TODO: Fix the unmarshaling problem with  StorageBackendExternal.Storage.Attributes
	if err = json.Unmarshal(jsonBytes, &addBackendResponse); err != nil {
		return nil, err
	}
	return &addBackendResponse, nil
}

func (client *TridentClient) ListBackends() (*ListBackendsResponse, error) {
	var (
		resp                 *http.Response
		err                  error
		bytes                []byte
		listBackendsResponse ListBackendsResponse
	)
	if resp, err = client.Get("backend"); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if bytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(bytes, &listBackendsResponse); err != nil {
		return nil, err
	}
	return &listBackendsResponse, nil
}

func (client *TridentClient) AddStorageClass(storageClassConfig *storage_class.Config) (*AddStorageClassResponse, error) {
	var (
		resp                    *http.Response
		err                     error
		jsonBytes               []byte
		addStorageClassResponse AddStorageClassResponse
	)
	jsonBytes, err = json.Marshal(storageClassConfig)
	if err != nil {
		return nil, err
	}
	if resp, err = client.Post("storageclass", bytes.NewBuffer(jsonBytes)); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if jsonBytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(jsonBytes, &addStorageClassResponse); err != nil {
		return nil, err
	}
	return &addStorageClassResponse, nil
}

func (client *TridentClient) GetVolume(volName string) (*GetVolumeResponse, error) {
	var (
		resp           *http.Response
		err            error
		bytes          []byte
		getVolResponse GetVolumeResponse
	)
	if resp, err = client.Get("volume/" + volName); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if bytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(bytes, &getVolResponse); err != nil {
		return nil, err
	}
	return &getVolResponse, nil
}

func (client *TridentClient) AddVolume(volConfig *storage.VolumeConfig) (*AddVolumeResponse, error) {
	var (
		resp           *http.Response
		err            error
		jsonBytes      []byte
		addVolResponse AddVolumeResponse
	)
	jsonBytes, err = json.Marshal(volConfig)
	if err != nil {
		return nil, err
	}
	if resp, err = client.Post("volume", bytes.NewBuffer(jsonBytes)); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if jsonBytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(jsonBytes, &addVolResponse); err != nil {
		return nil, err
	}
	return &addVolResponse, nil
}

func (client *TridentClient) DeleteVolume(volName string) (*DeleteResponse, error) {
	var (
		resp        *http.Response
		err         error
		jsonBytes   []byte
		delResponse DeleteResponse
	)
	if resp, err = client.Delete("volume/" + volName); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if jsonBytes, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(jsonBytes, &delResponse); err != nil {
		return nil, err
	}
	return &delResponse, nil
}
