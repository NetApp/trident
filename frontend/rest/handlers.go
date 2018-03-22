// Copyright 2018 NetApp, Inc. All Rights Reserved.

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
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
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

func httpStatusCodeForGet(err error) int {
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

type addResponse interface {
	setError(err error)
	isError() bool
	logSuccess()
	logFailure()
}

func AddGeneric(
	w http.ResponseWriter,
	r *http.Request,
	response addResponse,
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

func (a *AddBackendResponse) setError(err error) {
	a.Error = err.Error()
}

func (a *AddBackendResponse) isError() bool {
	return a.Error != ""
}

func (a *AddBackendResponse) logSuccess() {
	log.WithFields(log.Fields{
		"backend": a.BackendID,
		"handler": "AddBackend",
	}).Info("Added a new backend.")
}

func (a *AddBackendResponse) logFailure() {
	log.WithFields(log.Fields{
		"backend": a.BackendID,
		"handler": "AddBackend",
	}).Error(a.Error)
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
			return httpStatusCodeForGet(err)
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
			return httpStatusCodeForGet(err)
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
			return httpStatusCodeForGet(err)
		},
	)
}

// DeleteBackend calls OfflineBackend in the orchestrator, as we currently do
// not allow for full deletion of backends due to the potential for race
// conditions and the additional bookkeeping that would be required.
func DeleteBackend(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.OfflineBackend, "backend")
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
			return httpStatusCodeForGet(err)
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
			return httpStatusCodeForGet(err)
		},
	)
}

func DeleteVolume(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteVolume, "volume")
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
			return httpStatusCodeForGet(err)
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
			return httpStatusCodeForGet(err)
		},
	)
}

func DeleteStorageClass(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteStorageClass, "storageClass")
}
