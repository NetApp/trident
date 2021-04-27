// Copyright 2020 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi/helpers"
	k8shelper "github.com/netapp/trident/frontend/csi/helpers/kubernetes"
	"github.com/netapp/trident/frontend/kubernetes"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type listResponse interface {
	setList([]string)
}

func httpStatusCodeForAdd(err error) int {
	if err == nil {
		return http.StatusCreated
	} else if utils.IsNotReadyError(err) {
		return http.StatusServiceUnavailable
	} else if utils.IsBootstrapError(err) {
		return http.StatusInternalServerError
	} else {
		return http.StatusBadRequest
	}
}

func httpStatusCodeForGetUpdateList(err error) int {
	if err == nil {
		return http.StatusOK
	} else if utils.IsNotReadyError(err) {
		return http.StatusServiceUnavailable
	} else if utils.IsBootstrapError(err) {
		return http.StatusInternalServerError
	} else if utils.IsNotFoundError(err) {
		return http.StatusNotFound
	} else {
		return http.StatusBadRequest
	}
}

func httpStatusCodeForDelete(err error) int {
	if err == nil {
		return http.StatusOK
	} else if utils.IsNotReadyError(err) {
		return http.StatusServiceUnavailable
	} else if utils.IsBootstrapError(err) {
		return http.StatusInternalServerError
	} else if utils.IsNotFoundError(err) {
		return http.StatusNotFound
	} else {
		return http.StatusBadRequest
	}
}

func writeHTTPResponse(ctx context.Context, w http.ResponseWriter, response interface{}, httpStatusCode int) {

	if _, err := json.Marshal(response); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"response": response,
			"error":    err,
		}).Error("Failed to marshal HTTP response.")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(httpStatusCode)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		Logc(ctx).WithFields(log.Fields{
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

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

func ListGenericOneArg(
	w http.ResponseWriter,
	r *http.Request,
	varName string,
	response listResponse,
	lister func(string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	target := vars[varName]
	httpStatusCode := lister(target)

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

func GetGeneric(
	w http.ResponseWriter,
	r *http.Request,
	varName string,
	response interface{},
	getter func(string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	target := vars[varName]
	httpStatusCode := getter(target)

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

func GetGenericTwoArg(
	w http.ResponseWriter,
	r *http.Request,
	varName1, varName2 string,
	response interface{},
	getter func(string, string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	target1 := vars[varName1]
	target2 := vars[varName2]
	httpStatusCode := getter(target1, target2)

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

func GetGenericNoArg(
	w http.ResponseWriter,
	r *http.Request,
	response interface{},
	getter func() int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	httpStatusCode := getter()

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

type httpResponse interface {
	setError(err error)
	isError() bool
	logSuccess(context.Context)
	logFailure(context.Context)
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
			response.logFailure(r.Context())
		} else {
			response.logSuccess(r.Context())
		}

		writeHTTPResponse(r.Context(), w, response, httpStatusCode)
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
			response.logFailure(r.Context())
		} else {
			response.logSuccess(r.Context())
		}
		writeHTTPResponse(r.Context(), w, response, httpStatusCode)
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

type deleteFunc func(ctx context.Context, name string) error

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

	err := deleter(r.Context(), toDelete)
	if err != nil {
		response.Error = err.Error()
	}
	httpStatusCode := httpStatusCodeForDelete(err)

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

type deleteFuncTwoArg func(ctx context.Context, arg1, arg2 string) error

func DeleteGenericTwoArg(
	w http.ResponseWriter,
	r *http.Request,
	deleter deleteFuncTwoArg,
	varName1, varName2 string,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	response := DeleteResponse{}

	vars := mux.Vars(r)
	toDelete1 := vars[varName1]
	toDelete2 := vars[varName2]

	err := deleter(r.Context(), toDelete1, toDelete2)
	if err != nil {
		response.Error = err.Error()
	}
	httpStatusCode := httpStatusCodeForDelete(err)

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
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

func (r *AddBackendResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "AddBackend",
	}).Info("Added a new backend.")
}

func (r *AddBackendResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "AddBackend",
	}).Error(r.Error)
}

type GetVersionResponse struct {
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
	Error     string `json:"error,omitempty"`
}

func GetVersion(w http.ResponseWriter, r *http.Request) {
	response := &GetVersionResponse{}
	GetGenericNoArg(w, r, response,
		func() int {
			response.GoVersion = runtime.Version()
			version, err := orchestrator.GetVersion(r.Context())
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
			backend, err := orchestrator.AddBackend(r.Context(), string(body), "")
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

func (r *UpdateBackendResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "UpdateBackend",
	}).Info("Updated a backend.")
}

func (r *UpdateBackendResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"backend": r.BackendID,
		"handler": "UpdateBackend",
	}).Error(r.Error)
}

func UpdateBackend(w http.ResponseWriter, r *http.Request) {
	response := &UpdateBackendResponse{}
	UpdateGeneric(w, r, "backend", response,
		func(backendName string, body []byte) int {
			backend, err := orchestrator.UpdateBackend(r.Context(), backendName, string(body), "")
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
			backend, err := orchestrator.UpdateBackendState(r.Context(), backendName, request.State)
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
			backendNames := make([]string, 0)
			backends, err := orchestrator.ListBackends(r.Context())
			if err != nil {
				Logc(r.Context()).Errorf("ListBackends: %v", err)
				response.Error = err.Error()
			} else if len(backends) > 0 {
				backendNames = make([]string, 0, len(backends))
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

// IsValidUUID returns true if the supplied string 's' is a UUID, otherwise false
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func GetBackend(w http.ResponseWriter, r *http.Request) {
	response := &GetBackendResponse{}
	GetGeneric(w, r, "backend", response,
		func(backend string) int {
			var result *storage.BackendExternal
			var err error
			if IsValidUUID(backend) {
				result, err = orchestrator.GetBackendByBackendUUID(r.Context(), backend)
			} else {
				result, err = orchestrator.GetBackend(r.Context(), backend)
			}

			if err != nil {
				response.Error = err.Error()
			} else {
				response.Backend = result
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func GetBackendByBackendUUID(w http.ResponseWriter, r *http.Request) {
	response := &GetBackendResponse{}
	GetGeneric(w, r, "backendUUID", response,
		func(backendUUID string) int {
			backend, err := orchestrator.GetBackendByBackendUUID(r.Context(), backendUUID)
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

func (a *AddVolumeResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"handler": "AddVolume",
		"backend": a.BackendID,
	}).Info("Added a new volume.")
}
func (a *AddVolumeResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
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
			volume, err := orchestrator.AddVolume(r.Context(), volumeConfig)
			if err != nil {
				response.setError(err)
			}
			if volume != nil {
				response.BackendID = volume.BackendUUID
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
			volumeNames := make([]string, 0)
			volumes, err := orchestrator.ListVolumes(r.Context())
			if err != nil {
				response.Error = err.Error()
			} else if len(volumes) > 0 {
				volumeNames = make([]string, 0, len(volumes))
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
			volume, err := orchestrator.GetVolume(r.Context(), volName)
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

func (i *ImportVolumeResponse) logSuccess(ctx context.Context) {

	if i.Volume != nil {
		Logc(ctx).WithFields(log.Fields{
			"handler":     "ImportVolume",
			"backendUUID": i.Volume.BackendUUID,
			"volume":      i.Volume.Config.Name,
		}).Info("Imported an existing volume.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"handler": "ImportVolume",
		}).Info("Imported an existing volume.")
	}
}
func (i *ImportVolumeResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
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
			k8sFrontend, err := orchestrator.GetFrontend(r.Context(), string(config.ContextKubernetes))
			if err != nil {
				k8sFrontend, err = orchestrator.GetFrontend(r.Context(), helpers.KubernetesHelper)
			}
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
			volume, err := k8s.ImportVolume(r.Context(), importVolumeRequest)
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

type UpgradeVolumeResponse struct {
	Volume *storage.VolumeExternal `json:"volume"`
	Error  string                  `json:"error,omitempty"`
}

func (i *UpgradeVolumeResponse) setError(err error) {
	i.Error = err.Error()
}

func (i *UpgradeVolumeResponse) isError() bool {
	return i.Error != ""
}

func (i *UpgradeVolumeResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"handler": "UpgradeVolume",
		"volume":  i.Volume.Config.Name,
	}).Info("Upgraded an existing volume.")
}
func (i *UpgradeVolumeResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"handler": "UpgradeVolume",
	}).Error(i.Error)
}

func UpgradeVolume(w http.ResponseWriter, r *http.Request) {
	response := &UpgradeVolumeResponse{}
	UpdateGeneric(w, r, "volume", response,
		func(volumeName string, body []byte) int {
			upgradeVolumeRequest := new(storage.UpgradeVolumeRequest)
			err := json.Unmarshal(body, upgradeVolumeRequest)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForGetUpdateList(err)
			}
			if err = upgradeVolumeRequest.Validate(); err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			k8sHelperFrontend, err := orchestrator.GetFrontend(r.Context(), helpers.KubernetesHelper)
			if err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			k8sHelper, ok := k8sHelperFrontend.(k8shelper.K8SHelperPlugin)
			if !ok {
				err = fmt.Errorf("unable to obtain K8S helper frontend")
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}

			volume, err := k8sHelper.UpgradeVolume(r.Context(), upgradeVolumeRequest)
			if err != nil {
				response.Error = err.Error()
			}
			if volume != nil {
				response.Volume = volume
			}
			return httpStatusCodeForGetUpdateList(err)
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

func (a *AddStorageClassResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"handler":      "AddStorageClass",
		"storageClass": a.StorageClassID,
	}).Info("Added a new storage class.")
}
func (a *AddStorageClassResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
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
			sc, err := orchestrator.AddStorageClass(r.Context(), scConfig)
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
			storageClassNames := make([]string, 0)
			storageClasses, err := orchestrator.ListStorageClasses(r.Context())
			if err != nil {
				response.Error = err.Error()
			} else if len(storageClasses) > 0 {
				storageClassNames = make([]string, 0, len(storageClasses))
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
			storageClass, err := orchestrator.GetStorageClass(r.Context(), scName)
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
	Name           string            `json:"name"`
	TopologyLabels map[string]string `json:"topologyLabels,omitempty"`
	Error          string            `json:"error,omitempty"`
}

func (a *AddNodeResponse) setError(err error) {
	a.Error = err.Error()
}

func (a *AddNodeResponse) isError() bool {
	return a.Error != ""
}
func (a *AddNodeResponse) setTopologyLabels(payload map[string]string) {
	a.TopologyLabels = payload
}

func (a *AddNodeResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"handler": "AddOrUpdateNode",
		"node":    a.Name,
	}).Info("Added a new node.")
}

func (a *AddNodeResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"handler": "AddOrUpdateNode",
		"node":    a.Name,
	}).Error(a.Error)
}

func AddNode(w http.ResponseWriter, r *http.Request) {
	response := &AddNodeResponse{
		Name:           "",
		TopologyLabels: make(map[string]string),
		Error:          "",
	}
	UpdateGeneric(w, r, "node", response,
		func(name string, body []byte) int {
			node := new(utils.Node)
			err := json.Unmarshal(body, node)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}

			var csiFrontend frontend.Plugin
			csiFrontend, err = orchestrator.GetFrontend(r.Context(), helpers.KubernetesHelper)
			if err != nil {
				csiFrontend, err = orchestrator.GetFrontend(r.Context(), helpers.PlainCSIHelper)
			}
			if err != nil {
				err = fmt.Errorf("could not get CSI helper frontend")
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}

			helper, ok := csiFrontend.(helpers.HybridPlugin)
			if !ok {
				err = fmt.Errorf("could not get CSI hybrid frontend")
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			topologyLabels, err := helper.GetNodeTopologyLabels(r.Context(), node.Name)
			if err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			node.TopologyLabels = topologyLabels
			node.TopologyLabels = topologyLabels
			response.setTopologyLabels(topologyLabels)
			Logc(r.Context()).WithField("node", node.Name).Info("Determined topology labels for node: ",
				topologyLabels)

			nodeEventCallback := func(eventType, reason, message string) {
				helper.RecordNodeEvent(r.Context(), node.Name, eventType, reason, message)
			}
			err = orchestrator.AddNode(r.Context(), node, nodeEventCallback)
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
			node, err := orchestrator.GetNode(r.Context(), nName)
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
			nodeNames := make([]string, 0)
			nodes, err := orchestrator.ListNodes(r.Context())
			if err != nil {
				response.Error = err.Error()
			} else if len(nodes) > 0 {
				nodeNames = make([]string, 0, len(nodes))
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

type GetSnapshotResponse struct {
	Snapshot *storage.SnapshotExternal `json:"snapshot"`
	Error    string                    `json:"error,omitempty"`
}

func GetSnapshot(w http.ResponseWriter, r *http.Request) {
	response := &GetSnapshotResponse{}
	GetGenericTwoArg(w, r, "volume", "snapshot", response,
		func(volumeName, snapshotName string) int {
			snapshot, err := orchestrator.GetSnapshot(r.Context(), volumeName, snapshotName)
			if err != nil {
				response.Error = err.Error()
			} else {
				response.Snapshot = snapshot
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type ListSnapshotsResponse struct {
	Snapshots []string `json:"snapshots"`
	Error     string   `json:"error,omitempty"`
}

func (l *ListSnapshotsResponse) setList(payload []string) {
	l.Snapshots = payload
}

func ListSnapshots(w http.ResponseWriter, r *http.Request) {
	response := &ListSnapshotsResponse{}
	ListGeneric(w, r, response,
		func() int {
			snapshotIDs := make([]string, 0)
			snapshots, err := orchestrator.ListSnapshots(r.Context())
			if err != nil {
				response.Error = err.Error()
			} else if len(snapshots) > 0 {
				snapshotIDs = make([]string, 0, len(snapshots))
				for _, snapshot := range snapshots {
					snapshotIDs = append(snapshotIDs, snapshot.ID())
				}
			}
			response.setList(snapshotIDs)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func ListSnapshotsForVolume(w http.ResponseWriter, r *http.Request) {
	response := &ListSnapshotsResponse{}
	ListGenericOneArg(w, r, "volume", response,
		func(volumeName string) int {
			snapshotIDs := make([]string, 0)
			snapshots, err := orchestrator.ListSnapshotsForVolume(r.Context(), volumeName)
			if err != nil {
				response.Error = err.Error()
			} else if len(snapshots) > 0 {
				snapshotIDs = make([]string, 0, len(snapshots))
				for _, snapshot := range snapshots {
					snapshotIDs = append(snapshotIDs, snapshot.ID())
				}
			}
			response.setList(snapshotIDs)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type AddSnapshotResponse struct {
	SnapshotID string `json:"snapshotID"`
	Error      string `json:"error,omitempty"`
}

func (r *AddSnapshotResponse) setError(err error) {
	r.Error = err.Error()
}

func (r *AddSnapshotResponse) isError() bool {
	return r.Error != ""
}

func (r *AddSnapshotResponse) logSuccess(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"snapshot": r.SnapshotID,
		"handler":  "AddSnapshot",
	}).Info("Added a new volume snapshot.")
}

func (r *AddSnapshotResponse) logFailure(ctx context.Context) {

	Logc(ctx).WithFields(log.Fields{
		"snapshot": r.SnapshotID,
		"handler":  "AddSnapshot",
	}).Error(r.Error)
}

func AddSnapshot(w http.ResponseWriter, r *http.Request) {
	response := &AddSnapshotResponse{}
	AddGeneric(w, r, response,
		func(body []byte) int {
			snapshotConfig := new(storage.SnapshotConfig)
			if err := json.Unmarshal(body, snapshotConfig); err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			if err := snapshotConfig.Validate(); err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			snapshot, err := orchestrator.CreateSnapshot(r.Context(), snapshotConfig)
			if err != nil {
				response.setError(err)
			}
			if snapshot != nil {
				response.SnapshotID = snapshot.ID()
			}
			return httpStatusCodeForAdd(err)
		},
	)
}

func DeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	DeleteGenericTwoArg(w, r, orchestrator.DeleteSnapshot, "volume", "snapshot")
}
