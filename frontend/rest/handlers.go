// Copyright 2016 NetApp, Inc. All Rights Reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type listResponse interface {
	setList([]string)
}

func ListGeneric(
	w http.ResponseWriter,
	r *http.Request,
	response listResponse,
	lister func() []string,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	payload := lister()
	response.setList(payload)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		panic(err)
	}
}

func GetGeneric(w http.ResponseWriter,
	r *http.Request,
	varName string,
	response interface{},
	get func(string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	defer func() {
		if err := json.NewEncoder(w).Encode(response); err != nil {
			panic(err)
		}
	}()
	vars := mux.Vars(r)
	target := vars[varName]
	status := get(target)
	w.WriteHeader(status)
}

func GetGenericNoArg(w http.ResponseWriter,
	r *http.Request,
	response interface{},
	get func() int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	defer func() {
		if err := json.NewEncoder(w).Encode(response); err != nil {
			panic(err)
		}
	}()
	status := get()
	w.WriteHeader(status)
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
	add func([]byte),
) {
	var err error = nil
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	defer func() {
		if response.isError() {
			response.logFailure()
			w.WriteHeader(http.StatusBadRequest)
		} else {
			response.logSuccess()
			w.WriteHeader(http.StatusCreated)
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			panic(err)
		}
	}()

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, config.MaxRESTRequestSize))
	if err != nil {
		response.setError(err)
		return
	}
	if err := r.Body.Close(); err != nil {
		response.setError(err)
		return
	}
	add(body)
}

type DeleteResponse struct {
	Error string `json:"error,omitempty"`
}

type deleteFunc func(name string) (bool, error)

func DeleteGeneric(
	w http.ResponseWriter, r *http.Request, d deleteFunc, varName string,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	toDelete := vars[varName]

	response := DeleteResponse{
		Error: "",
	}

	found, err := d(toDelete)
	headerCode := http.StatusOK
	if err != nil {
		if !found {
			headerCode = http.StatusNotFound
		} else {
			headerCode = http.StatusInternalServerError
		}
		response.Error = err.Error()
	}
	w.WriteHeader(headerCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		panic(err)
	}
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
			version := orchestrator.GetVersion()
			if version == "" {
				response.Error = fmt.Sprintf("Couldn't retrieve %v's version!",
					config.OrchestratorName)
				return http.StatusNotFound
			}
			response.Version = version
			return http.StatusOK
		},
	)
}

func AddBackend(w http.ResponseWriter, r *http.Request) {
	response := &AddBackendResponse{
		BackendID: "",
		Error:     "",
	}
	AddGeneric(w, r, response,
		func(body []byte) {
			if backend, err := orchestrator.AddStorageBackend(string(body)); err != nil {
				response.Error = err.Error()
			} else if backend != nil {
				response.BackendID = backend.Name
			}
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
	ListGeneric(w, r,
		&ListBackendsResponse{},
		func() []string {
			backends := orchestrator.ListBackends()
			backendNames := make([]string, 0, len(backends))
			for _, b := range backends {
				backendNames = append(backendNames, b.Name)
			}
			return backendNames
		},
	)
}

type GetBackendResponse struct {
	Backend *storage.StorageBackendExternal `json:"backend"`
	Error   string                          `json:"error,omitempty"`
}

func GetBackend(w http.ResponseWriter, r *http.Request) {
	response := &GetBackendResponse{}
	GetGeneric(w, r, "backend", response,
		func(backendName string) int {
			backend := orchestrator.GetBackend(backendName)
			if backend == nil {
				response.Error = fmt.Sprintf("Backend %v was not found!",
					backendName)
				return http.StatusNotFound
			}
			response.Backend = backend
			return http.StatusOK
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
	response := &AddVolumeResponse{
		BackendID: "",
		Error:     "",
	}
	AddGeneric(w, r, response,
		func(body []byte) {
			volumeConfig := new(storage.VolumeConfig)
			err := json.Unmarshal(body, volumeConfig)
			if err != nil {
				response.Error = "Invalid JSON: " + err.Error()
				return
			}
			if err = volumeConfig.Validate(); err != nil {
				response.setError(err)
				return
			}
			volume, err := orchestrator.AddVolume(volumeConfig)
			if err != nil {
				response.setError(err)
			}
			if volume != nil {
				response.BackendID = volume.Backend
			}
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
	ListGeneric(w, r,
		&ListVolumesResponse{},
		func() []string {
			volumes := orchestrator.ListVolumes()
			volumeNames := make([]string, 0, len(volumes))
			for _, v := range volumes {
				volumeNames = append(volumeNames, v.Config.Name)
			}
			return volumeNames
		},
	)
}

type GetVolumeResponse struct {
	Volume *storage.VolumeExternal `json:"volume"`
	Error  string                  `json:"error,omitempty"`
}

func GetVolume(w http.ResponseWriter, r *http.Request) {
	response := &GetVolumeResponse{
		Volume: nil,
		Error:  "",
	}
	GetGeneric(w, r, "volume", response,
		func(volName string) int {
			volume := orchestrator.GetVolume(volName)
			if volume == nil {
				response.Error = fmt.Sprintf("Volume %v was not found!",
					volName)
				return http.StatusNotFound
			}
			response.Volume = volume
			return http.StatusOK
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
		func(body []byte) {
			scConfig := new(storage_class.Config)
			err := json.Unmarshal(body, scConfig)
			if err != nil {
				response.Error = "Invalid JSON: " + err.Error()
				return
			}
			sc, err := orchestrator.AddStorageClass(scConfig)
			if err != nil {
				response.setError(err)
			}
			if sc != nil {
				response.StorageClassID = sc.GetName()
			}
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
	ListGeneric(w, r,
		&ListStorageClassesResponse{},
		func() []string {
			storageClasses := orchestrator.ListStorageClasses()
			storageClassNames := make([]string, 0, len(storageClasses))
			for _, sc := range storageClasses {
				storageClassNames = append(storageClassNames, sc.GetName())
			}
			return storageClassNames
		},
	)
}

type GetStorageClassResponse struct {
	StorageClass *storage_class.StorageClassExternal `json:"storageClass"`
	Error        string                              `json:"error,omitempty"`
}

func GetStorageClass(w http.ResponseWriter, r *http.Request) {
	response := &GetStorageClassResponse{}
	GetGeneric(w, r, "storageClass", response,
		func(scName string) int {
			storageClass := orchestrator.GetStorageClass(scName)
			if storageClass == nil {
				response.Error = fmt.Sprintf("StorageClass %s was not found!",
					scName)
				return http.StatusNotFound
			}
			response.StorageClass = storageClass
			return http.StatusOK
		},
	)
}

func DeleteStorageClass(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, orchestrator.DeleteStorageClass, "storageClass")
}
