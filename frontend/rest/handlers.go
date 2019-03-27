// Copyright 2019 NetApp, Inc. All Rights Reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend/kubernetes"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type listResponse interface {
	setList([]string)
}

func httpStatusCodeForAdd(err error) int {
	if err == nil {
		return http.StatusCreated
	} else {
		switch err.(type) {
		case *core.NotReadyError:
			return http.StatusServiceUnavailable
		case *core.BootstrapError:
			return http.StatusInternalServerError
		default:
			return http.StatusBadRequest
		}
	}
}

func httpStatusCodeForGetUpdateList(err error) int {
	if err == nil {
		return http.StatusOK
	} else {
		switch err.(type) {
		case *core.NotReadyError:
			return http.StatusServiceUnavailable
		case *core.BootstrapError:
			return http.StatusInternalServerError
		case *core.NotFoundError:
			return http.StatusNotFound
		default:
			return http.StatusBadRequest
		}
	}
}

func httpStatusCodeForDelete(err error) int {
	if err == nil {
		return http.StatusOK
	} else {
		switch err.(type) {
		case *core.NotReadyError:
			return http.StatusServiceUnavailable
		case *core.BootstrapError:
			return http.StatusInternalServerError
		case *core.NotFoundError:
			return http.StatusNotFound
		default:
			return http.StatusBadRequest
		}
	}
}

func writeHTTPResponse(w http.ResponseWriter, response interface{}, httpStatusCode int) {

	if _, err := json.Marshal(response); err != nil {
		log.WithFields(log.Fields{
			"response": response,
			"error":    err,
		}).Error("Failed to marshal HTTP response.")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(httpStatusCode)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.WithFields(log.Fields{
			"response": response,
			"error":    err,
		}).Error("Failed to write HTTP response.")
	}
}

func ListGeneric(
	w http.ResponseWriter,
	r *http.Request,
	response listResponse,
	lister func() int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	httpStatusCode := lister()

	writeHTTPResponse(w, response, httpStatusCode)
}

func GetGeneric(w http.ResponseWriter,
	r *http.Request,
	varName string,
	response interface{},
	getter func(string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	target := vars[varName]
	httpStatusCode := getter(target)

	writeHTTPResponse(w, response, httpStatusCode)
}

func GetGenericNoArg(w http.ResponseWriter,
	r *http.Request,
	response interface{},
	getter func() int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	httpStatusCode := getter()

	writeHTTPResponse(w, response, httpStatusCode)
}

type httpResponse interface {
	setError(err error)
	isError() bool
	logSuccess()
	logFailure()
}

func AddGeneric(
	w http.ResponseWriter,
	r *http.Request,
	response httpResponse,
	adder func([]byte) int,
) {
	var err error
	var httpStatusCode int

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	defer func() {
		if response.isError() {
			response.logFailure()
		} else {
			response.logSuccess()
		}

		writeHTTPResponse(w, response, httpStatusCode)
	}()

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, config.MaxRESTRequestSize))
	if err != nil {
		response.setError(err)
		httpStatusCode = httpStatusCodeForAdd(err)
		return
	}
	if err := r.Body.Close(); err != nil {
		response.setError(err)
		httpStatusCode = httpStatusCodeForAdd(err)
		return
	}
	httpStatusCode = adder(body)
}

func UpdateGeneric(
	w http.ResponseWriter,
	r *http.Request,
	varName string,
	response httpResponse,
	updater func(string, []byte) int,
) {
	var err error
	var httpStatusCode int

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	defer func() {
		if response.isError() {
			response.logFailure()
		} else {
			response.logSuccess()
		}
		writeHTTPResponse(w, response, httpStatusCode)
	}()

	vars := mux.Vars(r)
	target := vars[varName]
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, config.MaxRESTRequestSize))
	if err != nil {
		response.setError(err)
		httpStatusCode = httpStatusCodeForGetUpdateList(err)
		return
	}
	if err := r.Body.Close(); err != nil {
		response.setError(err)
		httpStatusCode = httpStatusCodeForGetUpdateList(err)
		return
	}
	httpStatusCode = updater(target, body)
}

type DeleteResponse struct {
	Error string `json:"error,omitempty"`
}

type deleteFunc func(name string) error

func DeleteGeneric(
	w http.ResponseWriter,
	r *http.Request,
	deleter deleteFunc,
	varName string,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	response := DeleteResponse{}

	vars := mux.Vars(r)
	toDelete := vars[varName]

	err := deleter(toDelete)
	if err != nil {
		response.Error = err.Error()
	}
	httpStatusCode := httpStatusCodeForDelete(err)

	writeHTTPResponse(w, response, httpStatusCode)
}

type AddBackendResponse struct {
	BackendID string `json:"backend"`
	Error     string `json:"error,omitempty"`
}

func (r *AddBackendResponse) setError(err error) {
	r.Error = err.Error()
}

func (r *AddBackendResponse) isError() bool {
	return r.Error != ""
}

func (r *AddBackendResponse) logSuccess() {
	log.WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "AddBackend",
	}).Info("Added a new backend.")
}

func (r *AddBackendResponse) logFailure() {
	log.WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "AddBackend",
	}).Error(r.Error)
}

type GetVersionResponse struct {
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}

func GetVersion(w http.ResponseWriter, r *http.Request) {
	response := &GetVersionResponse{}
	GetGenericNoArg(w, r, response,
		func() int {
			version, err := orchestrator.GetVersion()
			if err != nil {
				response.Error = err.Error()
			}
			response.Version = version
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func AddBackend(w http.ResponseWriter, r *http.Request) {
	response := &AddBackendResponse{}
	AddGeneric(w, r, response,
		func(body []byte) int {
			backend, err := orchestrator.AddBackend(string(body))
			if err != nil {
				response.setError(err)
			}
			if backend != nil {
				response.BackendID = backend.Name
			}
			return httpStatusCodeForAdd(err)
		},
	)
}

type UpdateBackendResponse struct {
	BackendID string `json:"backend"`
	Error     string `json:"error,omitempty"`
}

func (r *UpdateBackendResponse) setError(err error) {
	r.Error = err.Error()
}

func (r *UpdateBackendResponse) isError() bool {
	return r.Error != ""
}

func (r *UpdateBackendResponse) logSuccess() {
	log.WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "UpdateBackend",
	}).Info("Updated a backend.")
}

func (r *UpdateBackendResponse) logFailure() {
	log.WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "UpdateBackend",
	}).Error(r.Error)
}

func UpdateBackend(w http.ResponseWriter, r *http.Request) {
	response := &UpdateBackendResponse{}
	UpdateGeneric(w, r, "backend", response,
		func(backendName string, body []byte) int {
			backend, err := orchestrator.UpdateBackend(backendName, string(body))
			if err != nil {
				response.Error = err.Error()
			}
			if backend != nil {
				response.BackendID = backend.Name
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func UpdateBackendState(w http.ResponseWriter, r *http.Request) {
	response := &UpdateBackendResponse{}
	UpdateGeneric(w, r, "backend", response,
		func(backendName string, body []byte) int {
			request := new(storage.UpdateBackendStateRequest)
			err := json.Unmarshal(body, request)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForGetUpdateList(err)
			}
			backend, err := orchestrator.UpdateBackendState(backendName, request.State)
			if err != nil {
				response.Error = err.Error()
			}
			if backend != nil {
				response.BackendID = backend.Name
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type ListBackendsResponse struct {
	Backends []string `json:"backends"`
	Error    string   `json:"error,omitempty"`
}

func (l *ListBackendsResponse) setList(payload []string) {
	l.Backends = payload
}

func ListBackends(w http.ResponseWriter, r *http.Request) {
	response := &ListBackendsResponse{}
	ListGeneric(w, r, response,
		func() int {
			backends, err := orchestrator.ListBackends()
			backendNames := make([]string, 0, len(backends))
			if err != nil {
				log.Errorf("ListBackends: %v", err)
				response.Error = err.Error()
			} else if backends != nil {
				for _, backend := range backends {
					backendNames = append(backendNames, backend.Name)
				}
			}
			response.setList(backendNames)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type GetBackendResponse struct {
	Backend *storage.BackendExternal `json:"backend"`
	Error   string                   `json:"error,omitempty"`
}

func GetBackend(w http.ResponseWriter, r *http.Request) {
	response := &GetBackendResponse{}
	GetGeneric(w, r, "backend", response,
		func(backendName string) int {
			backend, err := orchestrator.GetBackend(backendName)
			if err != nil {
				response.Error = err.Error()
			} else {
				response.Backend = backend
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

// DeleteBackend calls OfflineBackend in the orchestrator, as we currently do
// not allow for full deletion of backends due to the potential for race
// conditions and the additional bookkeeping that would be required.
func DeleteBackend(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteBackend, "backend")
}

type AddVolumeResponse struct {
	BackendID string `json:"backend"`
	Error     string `json:"error,omitempty"`
}

func (a *AddVolumeResponse) setError(err error) {
	a.Error = err.Error()
}

func (a *AddVolumeResponse) isError() bool {
	return a.Error != ""
}

func (a *AddVolumeResponse) logSuccess() {
	log.WithFields(log.Fields{
		"handler": "AddVolume",
		"backend": a.BackendID,
	}).Info("Added a new volume.")
}
func (a *AddVolumeResponse) logFailure() {
	log.WithFields(log.Fields{
		"handler": "AddVolume",
	}).Error(a.Error)
}

func AddVolume(w http.ResponseWriter, r *http.Request) {
	response := &AddVolumeResponse{}
	AddGeneric(w, r, response,
		func(body []byte) int {
			volumeConfig := new(storage.VolumeConfig)
			err := json.Unmarshal(body, volumeConfig)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			if err = volumeConfig.Validate(); err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			volume, err := orchestrator.AddVolume(volumeConfig)
			if err != nil {
				response.setError(err)
			}
			if volume != nil {
				response.BackendID = volume.Backend
			}
			return httpStatusCodeForAdd(err)
		},
	)
}

type ListVolumesResponse struct {
	Volumes []string `json:"volumes"`
	Error   string   `json:"error,omitempty"`
}

func (l *ListVolumesResponse) setList(payload []string) {
	l.Volumes = payload
}

func ListVolumes(w http.ResponseWriter, r *http.Request) {
	response := &ListVolumesResponse{}
	ListGeneric(w, r, response,
		func() int {
			volumes, err := orchestrator.ListVolumes()
			volumeNames := make([]string, 0, len(volumes))
			if err != nil {
				response.Error = err.Error()
			} else if volumes != nil {
				for _, volume := range volumes {
					volumeNames = append(volumeNames, volume.Config.Name)
				}
			}
			response.setList(volumeNames)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type GetVolumeResponse struct {
	Volume *storage.VolumeExternal `json:"volume"`
	Error  string                  `json:"error,omitempty"`
}

func GetVolume(w http.ResponseWriter, r *http.Request) {
	response := &GetVolumeResponse{}
	GetGeneric(w, r, "volume", response,
		func(volName string) int {
			volume, err := orchestrator.GetVolume(volName)
			if err != nil {
				response.Error = err.Error()
			} else {
				response.Volume = volume
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func DeleteVolume(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteVolume, "volume")
}

type ImportVolumeResponse struct {
	Volume *storage.VolumeExternal `json:"volume"`
	Error  string                  `json:"error,omitempty"`
}

func (i *ImportVolumeResponse) setError(err error) {
	i.Error = err.Error()
}

func (i *ImportVolumeResponse) isError() bool {
	return i.Error != ""
}

func (i *ImportVolumeResponse) logSuccess() {
	log.WithFields(log.Fields{
		"handler": "ImportVolume",
		"backend": i.Volume.Backend,
		"volume":  i.Volume.Config.Name,
	}).Info("Imported an existing volume.")
}
func (i *ImportVolumeResponse) logFailure() {
	log.WithFields(log.Fields{
		"handler": "ImportVolume",
	}).Error(i.Error)
}

func ImportVolume(w http.ResponseWriter, r *http.Request) {
	response := &ImportVolumeResponse{}
	AddGeneric(w, r, response,
		func(body []byte) int {
			importVolumeRequest := new(storage.ImportVolumeRequest)
			err := json.Unmarshal(body, importVolumeRequest)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			if err = importVolumeRequest.Validate(); err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			k8sFrontend, err := orchestrator.GetFrontend(string(config.ContextKubernetes))
			if err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			k8s, ok := k8sFrontend.(kubernetes.KubernetesPlugin)
			if !ok {
				err = fmt.Errorf("unable to obtain Kubernetes frontend")
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			volume, err := k8s.ImportVolume(importVolumeRequest)

			if err != nil {
				response.setError(err)
			}
			if volume != nil {
				response.Volume = volume
			}
			return httpStatusCodeForAdd(err)
		},
	)
}

type AddStorageClassResponse struct {
	StorageClassID string `json:"storageClass"`
	Error          string `json:"error,omitempty"`
}

func (a *AddStorageClassResponse) setError(err error) {
	a.Error = err.Error()
}

func (a *AddStorageClassResponse) isError() bool {
	return a.Error != ""
}

func (a *AddStorageClassResponse) logSuccess() {
	log.WithFields(log.Fields{
		"handler":      "AddStorageClass",
		"storageClass": a.StorageClassID,
	}).Info("Added a new storage class.")
}
func (a *AddStorageClassResponse) logFailure() {
	log.WithFields(log.Fields{
		"handler":      "AddStorageClass",
		"storageClass": a.StorageClassID,
	}).Error(a.Error)
}

func AddStorageClass(w http.ResponseWriter, r *http.Request) {
	response := &AddStorageClassResponse{
		StorageClassID: "",
		Error:          "",
	}
	AddGeneric(w, r, response,
		func(body []byte) int {
			scConfig := new(storageclass.Config)
			err := json.Unmarshal(body, scConfig)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			sc, err := orchestrator.AddStorageClass(scConfig)
			if err != nil {
				response.setError(err)
			}
			if sc != nil {
				response.StorageClassID = sc.GetName()
			}
			return httpStatusCodeForAdd(err)
		},
	)
}

type ListStorageClassesResponse struct {
	StorageClasses []string `json:"storageClasses"`
	Error          string   `json:"error,omitempty"`
}

func (l *ListStorageClassesResponse) setList(payload []string) {
	l.StorageClasses = payload
}

func ListStorageClasses(w http.ResponseWriter, r *http.Request) {
	response := &ListStorageClassesResponse{}
	ListGeneric(w, r, response,
		func() int {
			storageClasses, err := orchestrator.ListStorageClasses()
			storageClassNames := make([]string, 0, len(storageClasses))
			if err != nil {
				response.Error = err.Error()
			} else if storageClasses != nil {
				for _, sc := range storageClasses {
					storageClassNames = append(storageClassNames, sc.GetName())
				}
			}
			response.setList(storageClassNames)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type GetStorageClassResponse struct {
	StorageClass *storageclass.External `json:"storageClass"`
	Error        string                 `json:"error,omitempty"`
}

func GetStorageClass(w http.ResponseWriter, r *http.Request) {
	response := &GetStorageClassResponse{}
	GetGeneric(w, r, "storageClass", response,
		func(scName string) int {
			storageClass, err := orchestrator.GetStorageClass(scName)
			if err != nil {
				response.Error = err.Error()
			} else {
				response.StorageClass = storageClass
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func DeleteStorageClass(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteStorageClass, "storageClass")
}

type AddNodeResponse struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

func (a *AddNodeResponse) setError(err error) {
	a.Error = err.Error()
}

func (a *AddNodeResponse) isError() bool {
	return a.Error != ""
}

func (a *AddNodeResponse) logSuccess() {
	log.WithFields(log.Fields{
		"handler": "AddOrUpdateNode",
		"node":    a.Name,
	}).Info("Added a new node.")
}

func (a *AddNodeResponse) logFailure() {
	log.WithFields(log.Fields{
		"handler": "AddOrUpdateNode",
		"node":    a.Name,
	}).Error(a.Error)
}

func AddNode(w http.ResponseWriter, r *http.Request) {
	response := &AddNodeResponse{
		Name:  "",
		Error: "",
	}
	UpdateGeneric(w, r, "node", response,
		func(name string, body []byte) int {
			node := new(utils.Node)
			err := json.Unmarshal(body, node)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			err = orchestrator.AddNode(node)
			if err != nil {
				response.setError(err)
			}
			response.Name = node.Name
			return httpStatusCodeForAdd(err)
		},
	)
}

type GetNodeResponse struct {
	Node  *utils.Node `json:"node"`
	Error string      `json:"error,omitempty"`
}

func GetNode(w http.ResponseWriter, r *http.Request) {
	response := &GetNodeResponse{}
	GetGeneric(w, r, "node", response,
		func(nName string) int {
			node, err := orchestrator.GetNode(nName)
			if err != nil {
				response.Error = err.Error()
			} else {
				response.Node = node
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type ListNodesResponse struct {
	Nodes []string `json:"nodes"`
	Error string   `json:"error,omitempty"`
}

func (l *ListNodesResponse) setList(payload []string) {
	l.Nodes = payload
}

func ListNodes(w http.ResponseWriter, r *http.Request) {
	response := &ListNodesResponse{}
	ListGeneric(w, r, response,
		func() int {
			nodes, err := orchestrator.ListNodes()
			nodeNames := make([]string, 0, len(nodes))
			if err != nil {
				response.Error = err.Error()
			} else if nodes != nil {
				for _, node := range nodes {
					nodeNames = append(nodeNames, node.Name)
				}
			}
			response.setList(nodeNames)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func DeleteNode(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteNode, "node")
}
