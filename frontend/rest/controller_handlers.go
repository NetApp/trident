// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi/helpers"
	k8shelper "github.com/netapp/trident/frontend/csi/helpers/kubernetes"
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
	// we're already marshaling the json, so just use the resulting byte array later
	b, err := json.Marshal(response)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"response": response,
			"error":    err,
		}).Error("Failed to marshal HTTP response.")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(httpStatusCode)
	if _, err := w.Write(b); err != nil {
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
	lister func(map[string]string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	httpStatusCode := lister(vars)

	writeHTTPResponse(r.Context(), w, response, httpStatusCode)
}

func GetGeneric(
	w http.ResponseWriter,
	r *http.Request,
	response interface{},
	getter func(map[string]string) int,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	httpStatusCode := getter(vars)

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
	response httpResponse,
	updater func(map[string]string, []byte) int,
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
	httpStatusCode = updater(vars, body)
}

type DeleteResponse struct {
	Error string `json:"error,omitempty"`
}

func DeleteGeneric(
	w http.ResponseWriter,
	r *http.Request,
	deleter func(ctx context.Context, vars map[string]string) error,
) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	response := DeleteResponse{}

	err := deleter(r.Context(), mux.Vars(r))
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
	GetGeneric(w, r, response,
		func(map[string]string) int {
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
	UpdateGeneric(w, r, response,
		func(vars map[string]string, body []byte) int {
			backend, err := orchestrator.UpdateBackend(r.Context(), vars["backend"], string(body), "")
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
	UpdateGeneric(w, r, response,
		func(vars map[string]string, body []byte) int {
			request := new(storage.UpdateBackendStateRequest)
			err := json.Unmarshal(body, request)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForGetUpdateList(err)
			}
			backend, err := orchestrator.UpdateBackendState(r.Context(), vars["backend"], request.State)
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
		func(_ map[string]string) int {
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
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			backend := vars["backend"]
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
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			backend, err := orchestrator.GetBackendByBackendUUID(r.Context(), vars["backendUUID"])
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
	DeleteGeneric(w, r, func(ctx context.Context, vars map[string]string) error {
		return orchestrator.DeleteBackend(ctx, vars["backend"])
	})
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
		func(map[string]string) int {
			var (
				volumes []*storage.VolumeExternal
				volume  *storage.VolumeExternal
				err     error
			)

			volumeNames := make([]string, 0)

			if r.URL.Query().Has("subordinateOf") {
				volumes, err = orchestrator.ListSubordinateVolumes(r.Context(), r.URL.Query().Get("subordinateOf"))
			} else if r.URL.Query().Has("parentOfSubordinate") {
				volume, err = orchestrator.GetSubordinateSourceVolume(r.Context(),
					r.URL.Query().Get("parentOfSubordinate"))
				volumes = append(volumes, volume)
			} else {
				volumes, err = orchestrator.ListVolumes(r.Context())
			}

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
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			volume, err := orchestrator.GetVolume(r.Context(), vars["volume"])
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
	DeleteGeneric(w, r, func(ctx context.Context, vars map[string]string) error {
		return orchestrator.DeleteVolume(ctx, vars["volume"])
	})
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
			k8sFrontend, err := orchestrator.GetFrontend(r.Context(), helpers.KubernetesHelper)
			if err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			k8s, ok := k8sFrontend.(k8shelper.K8SHelperPlugin)
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
	UpdateGeneric(w, r, response,
		func(_ map[string]string, body []byte) int {
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
		func(_ map[string]string) int {
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
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			storageClass, err := orchestrator.GetStorageClass(r.Context(), vars["storageClass"])
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
	DeleteGeneric(w, r, func(ctx context.Context, vars map[string]string) error {
		return orchestrator.DeleteStorageClass(ctx, vars["storageClass"])
	})
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
	UpdateGeneric(w, r, response,
		func(_ map[string]string, body []byte) int {
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
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			node, err := orchestrator.GetNode(r.Context(), vars["node"])
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
		func(_ map[string]string) int {
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
	DeleteGeneric(w, r, func(ctx context.Context, vars map[string]string) error {
		return orchestrator.DeleteNode(ctx, vars["node"])
	})
}

type VolumePublicationsResponse struct {
	VolumePublications []*utils.VolumePublicationExternal `json:"volumePublications"`
	Error              string                             `json:"error,omitempty"`
}

func (r *VolumePublicationsResponse) setError(err error) {
	r.Error = err.Error()
}

func (r *VolumePublicationsResponse) isError() bool {
	return r.Error != ""
}

func (r *VolumePublicationsResponse) logSuccess(ctx context.Context) {
	Logc(ctx).WithFields(log.Fields{
		"handler": "UpdateVolumePublication",
	}).Info("Updated a volume publication.")
}

func (r *VolumePublicationsResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(log.Fields{
		"handler": "UpdateVolumePublication",
	}).Error(r.Error)
}

func ListVolumePublicationsForNode(w http.ResponseWriter, r *http.Request) {
	response := &VolumePublicationsResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			pubs, err := orchestrator.ListVolumePublicationsForNode(r.Context(), vars["node"])
			response.VolumePublications = pubs
			response.Error = err.Error()
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func UpdateVolumePublication(w http.ResponseWriter, r *http.Request) {
	response := &VolumePublicationsResponse{}
	UpdateGeneric(w, r, response,
		func(vars map[string]string, _ []byte) int {
			var notSafeToAttach *bool
			if notSafeToAttachVar, ok := vars["notSafeToAttach"]; ok {
				notSafe, err := strconv.ParseBool(notSafeToAttachVar)
				if err != nil {
					return httpStatusCodeForGetUpdateList(err)
				}
				notSafeToAttach = &notSafe
			}
			err := orchestrator.UpdateVolumePublication(r.Context(), vars["volume"], vars["node"], notSafeToAttach)
			if err != nil {
				response.Error = err.Error()
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type GetSnapshotResponse struct {
	Snapshot *storage.SnapshotExternal `json:"snapshot"`
	Error    string                    `json:"error,omitempty"`
}

func GetSnapshot(w http.ResponseWriter, r *http.Request) {
	response := &GetSnapshotResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			snapshot, err := orchestrator.GetSnapshot(r.Context(), vars["volume"], vars["snapshot"])
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
		func(_ map[string]string) int {
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
	ListGeneric(w, r, response,
		func(vars map[string]string) int {
			snapshotIDs := make([]string, 0)
			snapshots, err := orchestrator.ListSnapshotsForVolume(r.Context(), vars["volume"])
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
	DeleteGeneric(w, r, func(ctx context.Context, vars map[string]string) error {
		return orchestrator.DeleteSnapshot(r.Context(), vars["volume"], vars["snapshot"])
	})
}

type GetCHAPResponse struct {
	CHAP  *utils.IscsiChapInfo `json:"chap"`
	Error string               `json:"error,omitempty"`
}

func GetCHAP(w http.ResponseWriter, r *http.Request) {
	response := &GetCHAPResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			chapInfo, err := orchestrator.GetCHAP(r.Context(), vars["volume"], vars["node"])
			if err != nil {
				response.Error = err.Error()
			}
			response.CHAP = chapInfo
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}
