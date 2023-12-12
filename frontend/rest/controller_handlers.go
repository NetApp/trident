// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/netapp/trident/acp"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/common"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	k8shelper "github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

type listResponse interface {
	setList([]string)
}

func httpStatusCodeForAdd(err error) int {
	if err == nil {
		return http.StatusCreated
	} else if errors.IsNotReadyError(err) {
		return http.StatusServiceUnavailable
	} else if errors.IsBootstrapError(err) {
		return http.StatusInternalServerError
	} else {
		return http.StatusBadRequest
	}
}

func httpStatusCodeForGetUpdateList(err error) int {
	if err == nil {
		return http.StatusOK
	} else if errors.IsNotReadyError(err) {
		return http.StatusServiceUnavailable
	} else if errors.IsBootstrapError(err) {
		return http.StatusInternalServerError
	} else if errors.IsNotFoundError(err) {
		return http.StatusNotFound
	} else {
		return http.StatusBadRequest
	}
}

func httpStatusCodeForDelete(err error) int {
	if err == nil {
		return http.StatusOK
	} else if errors.IsNotReadyError(err) {
		return http.StatusServiceUnavailable
	} else if errors.IsBootstrapError(err) {
		return http.StatusInternalServerError
	} else if errors.IsNotFoundError(err) {
		return http.StatusNotFound
	} else {
		return http.StatusBadRequest
	}
}

func writeHTTPResponse(ctx context.Context, w http.ResponseWriter, response interface{}, httpStatusCode int) {
	// we're already marshaling the json, so just use the resulting byte array later
	b, err := json.Marshal(response)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"response": response,
			"error":    err,
		}).Error("Failed to marshal HTTP response.")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(httpStatusCode)
	if _, err := w.Write(b); err != nil {
		Logc(ctx).WithFields(LogFields{
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

	body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxRESTRequestSize))
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
	updater func(http.ResponseWriter, *http.Request, httpResponse, map[string]string, []byte) int,
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
	body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxRESTRequestSize))
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
	httpStatusCode = updater(w, r, response, vars, body)
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
	Logc(ctx).WithFields(LogFields{
		"backend": r.BackendID,
		"handler": "AddBackend",
	}).Info("Added a new backend.")
}

func (r *AddBackendResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"backend": r.BackendID,
		"handler": "AddBackend",
	}).Error(r.Error)
}

type GetVersionResponse struct {
	Version    string `json:"version"`
	GoVersion  string `json:"goVersion"`
	Error      string `json:"error,omitempty"`
	ACPVersion string `json:"acpVersion,omitempty"`
}

func GetVersion(w http.ResponseWriter, r *http.Request) {
	response := &GetVersionResponse{}
	GetGeneric(w, r, response,
		func(map[string]string) int {
			response.GoVersion = runtime.Version()
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowCoreVersion, LogLayerRESTFrontend)

			version, err := orchestrator.GetVersion(ctx)
			if err != nil {
				response.Error = err.Error()
			}
			response.Version = version

			response.ACPVersion = GetACPVersion(ctx)
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func GetACPVersion(ctx context.Context) string {
	version, err := acp.API().GetVersion(ctx)
	if err != nil {
		if !errors.IsUnsupportedError(err) {
			Logc(ctx).WithError(err).Error("Could not get Trident-ACP version.")
		}
		return ""
	}

	if version == nil {
		Logc(ctx).WithError(err).Error("Trident-ACP version is empty.")
		return ""
	}

	return version.String()
}

func AddBackend(w http.ResponseWriter, r *http.Request) {
	response := &AddBackendResponse{}
	AddGeneric(w, r, response,
		func(body []byte) int {
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowBackendCreate, LogLayerRESTFrontend)

			backend, err := orchestrator.AddBackend(ctx, string(body), "")
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
	Logc(ctx).WithFields(LogFields{
		"backend": r.BackendID,
		"handler": "UpdateBackend",
	}).Info("Updated a backend.")
}

func (r *UpdateBackendResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"backend": r.BackendID,
		"handler": "UpdateBackend",
	}).Error(r.Error)
}

func UpdateBackend(w http.ResponseWriter, r *http.Request) {
	response := &UpdateBackendResponse{}
	UpdateGeneric(w, r, response,
		func(w http.ResponseWriter, r *http.Request, response httpResponse, vars map[string]string, body []byte) int {
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowBackendUpdate, LogLayerRESTFrontend)

			updateResponse, ok := response.(*UpdateBackendResponse)
			if !ok {
				response.setError(fmt.Errorf("response object must be of type UpdateBackendResponse"))
				return http.StatusInternalServerError
			}
			backend, err := orchestrator.UpdateBackend(ctx, vars["backend"], string(body), "")
			if err != nil {
				updateResponse.Error = err.Error()
			}
			if backend != nil {
				updateResponse.BackendID = backend.Name
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func UpdateBackendState(w http.ResponseWriter, r *http.Request) {
	response := &UpdateBackendResponse{}
	UpdateGeneric(w, r, response,
		func(w http.ResponseWriter, r *http.Request, response httpResponse, vars map[string]string, body []byte) int {
			updateResponse, ok := response.(*UpdateBackendResponse)
			if !ok {
				response.setError(fmt.Errorf("response object must be of type UpdateBackendResponse"))
				return http.StatusInternalServerError
			}
			request := new(storage.UpdateBackendStateRequest)
			err := json.Unmarshal(body, request)
			if err != nil {
				updateResponse.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForGetUpdateList(err)
			}
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowBackendUpdate, LogLayerRESTFrontend)

			backend, err := orchestrator.UpdateBackendState(ctx, vars["backend"], request.BackendState,
				request.UserBackendState)
			if err != nil {
				updateResponse.Error = err.Error()
			}
			if backend != nil {
				updateResponse.BackendID = backend.Name
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowBackendList, LogLayerRESTFrontend)

			backends, err := orchestrator.ListBackends(ctx)
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowBackendGet, LogLayerRESTFrontend)

			if IsValidUUID(backend) {
				result, err = orchestrator.GetBackendByBackendUUID(ctx, backend)
			} else {
				result, err = orchestrator.GetBackend(ctx, backend)
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

// DeleteBackend calls OfflineBackend in the orchestrator, as we currently do
// not allow for full deletion of backends due to the potential for race
// conditions and the additional bookkeeping that would be required.
func DeleteBackend(w http.ResponseWriter, r *http.Request) {
	DeleteGeneric(w, r, func(ctx context.Context, vars map[string]string) error {
		ctx = GenerateRequestContext(r.Context(), "", "", WorkflowBackendDelete, LogLayerRESTFrontend)

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
	Logc(ctx).WithFields(LogFields{
		"handler": "AddVolume",
		"backend": a.BackendID,
	}).Info("Added a new volume.")
}

func (a *AddVolumeResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowVolumeCreate, LogLayerRESTFrontend)

			volume, err := orchestrator.AddVolume(ctx, volumeConfig)
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowVolumeList, LogLayerRESTFrontend)

			if r.URL.Query().Has("subordinateOf") {
				volumes, err = orchestrator.ListSubordinateVolumes(ctx, r.URL.Query().Get("subordinateOf"))
			} else if r.URL.Query().Has("parentOfSubordinate") {
				volume, err = orchestrator.GetSubordinateSourceVolume(ctx, r.URL.Query().Get("parentOfSubordinate"))
				volumes = append(volumes, volume)
			} else {
				volumes, err = orchestrator.ListVolumes(ctx)
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowVolumeGet, LogLayerRESTFrontend)

			volume, err := orchestrator.GetVolume(ctx, vars["volume"])
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
		ctx = GenerateRequestContext(r.Context(), "", "", WorkflowVolumeDelete, LogLayerRESTFrontend)

		return orchestrator.DeleteVolume(ctx, vars["volume"])
	})
}

type UpdateVolumeResponse struct {
	Volume *storage.VolumeExternal `json:"volume"`
	Error  string                  `json:"error,omitempty"`
}

func (r *UpdateVolumeResponse) setError(err error) {
	r.Error = err.Error()
}

func (r *UpdateVolumeResponse) isError() bool {
	return r.Error != ""
}

func (r *UpdateVolumeResponse) logSuccess(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"handler": "UpdateVolume",
	}).Info("Updated a volume.")
}

func (r *UpdateVolumeResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"handler": "UpdateVolume",
	}).Error(r.Error)
}

func volumeLUKSPassphraseNamesUpdater(
	_ http.ResponseWriter, r *http.Request, response httpResponse, vars map[string]string, body []byte,
) int {
	updateResponse, ok := response.(*UpdateVolumeResponse)
	if !ok {
		response.setError(fmt.Errorf("response object must be of type UpdateVolumeResponse"))
		return http.StatusInternalServerError
	}
	passphraseNames := new([]string)
	volume, err := orchestrator.GetVolume(r.Context(), vars["volume"])
	if err != nil {
		updateResponse.Error = err.Error()
		if errors.IsNotFoundError(err) {
			return http.StatusNotFound
		} else {
			return http.StatusInternalServerError
		}
	} else {
		updateResponse.Volume = volume
	}
	err = json.Unmarshal(body, passphraseNames)
	if err != nil {
		updateResponse.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
		return http.StatusBadRequest
	}

	err = orchestrator.UpdateVolumeLUKSPassphraseNames(r.Context(), vars["volume"], passphraseNames)
	if err != nil {
		response.setError(fmt.Errorf("failed to update LUKS passphrase names for volume %s: %s", vars["volume"],
			err.Error()))
		if errors.IsNotFoundError(err) {
			return http.StatusNotFound
		}
		return http.StatusInternalServerError
	}

	return http.StatusOK
}

func UpdateVolumeLUKSPassphraseNames(w http.ResponseWriter, r *http.Request) {
	response := &UpdateVolumeResponse{}
	UpdateGeneric(w, r, response, volumeLUKSPassphraseNamesUpdater)
}

func UpdateVolume(w http.ResponseWriter, r *http.Request) {
	response := &UpdateVolumeResponse{}
	UpdateGeneric(w, r, response, volumeUpdater)
}

func volumeUpdater(
	_ http.ResponseWriter, r *http.Request,
	response httpResponse, vars map[string]string, body []byte,
) int {
	ctx := r.Context()
	Logc(ctx).Debug(">>>> volumeUpdater")
	defer Logc(ctx).Debug("<<<< volumeUpdater")

	updateResponse, ok := response.(*UpdateVolumeResponse)
	if !ok {
		response.setError(fmt.Errorf("response object must be of type UpdateVolumeResponse"))
		return http.StatusInternalServerError
	}

	updateVolRequest := &utils.VolumeUpdateInfo{}
	err := json.Unmarshal(body, updateVolRequest)
	if err != nil {
		updateResponse.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
		return http.StatusBadRequest
	}

	volName := vars["volume"]

	// Update the volume
	Logc(ctx).Debugf("Updating volume %v with update info %v", volName, updateVolRequest)

	err = orchestrator.UpdateVolume(ctx, volName, updateVolRequest)
	if err != nil {
		updateResponse.Error = err.Error()
		if errors.IsInvalidInputError(err) {
			return http.StatusBadRequest
		} else if errors.IsNotFoundError(err) {
			return http.StatusNotFound
		} else if errors.IsUnsupportedError(err) {
			return http.StatusForbidden
		} else {
			return http.StatusInternalServerError
		}
	}

	// Get the updated volume and set back in response
	volumeExternal, err := orchestrator.GetVolume(ctx, volName)
	if err != nil {
		updateResponse.Error = err.Error()
		return http.StatusInternalServerError
	}

	updateResponse.Volume = volumeExternal

	return http.StatusOK
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
		Logc(ctx).WithFields(LogFields{
			"handler":     "ImportVolume",
			"backendUUID": i.Volume.BackendUUID,
			"volume":      i.Volume.Config.Name,
		}).Info("Imported an existing volume.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"handler": "ImportVolume",
		}).Info("Imported an existing volume.")
	}
}

func (i *ImportVolumeResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowVolumeImport, LogLayerRESTFrontend)

			k8sFrontend, err := orchestrator.GetFrontend(ctx, controllerhelpers.KubernetesHelper)
			if err != nil {
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			k8s, ok := k8sFrontend.(k8shelper.K8SControllerHelperPlugin)
			if !ok {
				err = fmt.Errorf("unable to obtain Kubernetes frontend")
				response.setError(err)
				return httpStatusCodeForAdd(err)
			}
			volume, err := k8s.ImportVolume(ctx, importVolumeRequest)
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

func (a *AddStorageClassResponse) logSuccess(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"handler":      "AddStorageClass",
		"storageClass": a.StorageClassID,
	}).Info("Added a new storage class.")
}

func (a *AddStorageClassResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowStorageClassCreate, LogLayerRESTFrontend)

			sc, err := orchestrator.AddStorageClass(ctx, scConfig)
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowStorageClassList, LogLayerRESTFrontend)

			storageClasses, err := orchestrator.ListStorageClasses(ctx)
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
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowStorageClassList, LogLayerRESTFrontend)

			storageClass, err := orchestrator.GetStorageClass(ctx, vars["storageClass"])
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
		ctx = GenerateRequestContext(r.Context(), "", "", WorkflowStorageClassDelete, LogLayerRESTFrontend)

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
	Logc(ctx).WithFields(LogFields{
		"handler": "AddOrUpdateNode",
		"node":    a.Name,
	}).Info("Added a new node.")
}

func (a *AddNodeResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
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

	const auditMsg = "AddNode endpoint called."

	UpdateGeneric(w, r, response,
		func(w http.ResponseWriter, r *http.Request, response httpResponse, _ map[string]string, body []byte) int {
			updateResponse, ok := response.(*AddNodeResponse)
			if !ok {
				response.setError(fmt.Errorf("response object must be of type AddNodeResponse"))
				return http.StatusInternalServerError
			}
			node := new(utils.Node)
			err := json.Unmarshal(body, node)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}

			var csiFrontend frontend.Plugin
			ctx := GenerateRequestContext(r.Context(), "", "", WorkflowNodeCreate, LogLayerRESTFrontend)

			csiFrontend, err = orchestrator.GetFrontend(ctx, controllerhelpers.KubernetesHelper)
			if err != nil {
				csiFrontend, err = orchestrator.GetFrontend(ctx, controllerhelpers.PlainCSIHelper)
			}
			if err != nil {
				err = fmt.Errorf("could not get CSI helper frontend")
				updateResponse.setError(err)
				return httpStatusCodeForAdd(err)
			}

			helper, ok := csiFrontend.(controllerhelpers.ControllerHelper)
			if !ok {
				err = fmt.Errorf("could not get CSI hybrid frontend")
				updateResponse.setError(err)
				return httpStatusCodeForAdd(err)
			}
			topologyLabels, err := helper.GetNodeTopologyLabels(ctx, node.Name)
			if err != nil {
				updateResponse.setError(err)
				return httpStatusCodeForAdd(err)
			}
			node.TopologyLabels = topologyLabels
			updateResponse.setTopologyLabels(topologyLabels)
			Logc(r.Context()).WithField("node", node.Name).Info("Determined topology labels for node: ",
				topologyLabels)

			nodeEventCallback := func(eventType, reason, message string) {
				helper.RecordNodeEvent(ctx, node.Name, eventType, reason, message)
			}
			err = orchestrator.AddNode(ctx, node, nodeEventCallback)
			if err != nil {
				response.setError(err)
			}
			updateResponse.Name = node.Name
			return httpStatusCodeForAdd(err)
		},
	)
}

type UpdateNodeResponse struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

func (u *UpdateNodeResponse) setError(err error) {
	u.Error = err.Error()
}

func (u *UpdateNodeResponse) isError() bool {
	return u.Error != ""
}

func (u *UpdateNodeResponse) logSuccess(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"handler": "UpdateNode",
		"node":    u.Name,
	}).Info("Updated a node.")
}

func (u *UpdateNodeResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"handler": "UpdateNode",
		"node":    u.Name,
	}).Error(u.Error)
}

// nodeUpdater updates a Trident Node's persistent state asynchronously.
func nodeUpdater(
	_ http.ResponseWriter, r *http.Request, response httpResponse, vars map[string]string, body []byte,
) int {
	updateNodeResponse, ok := response.(*UpdateNodeResponse)
	if !ok {
		response.setError(fmt.Errorf("response object must be of type UpdateNodeResponse"))
		return http.StatusInternalServerError
	}

	nodePublicationState := new(utils.NodePublicationStateFlags)
	err := json.Unmarshal(body, nodePublicationState)
	if err != nil {
		response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
		return httpStatusCodeForAdd(err)
	}

	nodeName := vars["node"]
	if nodeName == "" {
		updateNodeResponse.setError(fmt.Errorf("node name not specified"))
		return http.StatusBadRequest
	}
	updateNodeResponse.Name = nodeName

	// Spawn a routine to do this asynchronously.
	go func() {
		ctx := context.Background()
		logEntry := Logc(ctx).WithFields(LogFields{"handler": "UpdateNode", "nodeName": nodeName})

		// Get the just-in-time node state from Kubernetes
		k8sFrontend, err := orchestrator.GetFrontend(ctx, controllerhelpers.KubernetesHelper)
		if err != nil {
			logEntry.Error(err)
			return
		}
		k8s, ok := k8sFrontend.(k8shelper.K8SControllerHelperPlugin)
		if !ok {
			logEntry.Error(fmt.Errorf("unable to obtain Kubernetes frontend"))
			return
		}

		k8sNodePublicationState, err := k8s.GetNodePublicationState(ctx, nodeName)
		if err != nil {
			logEntry.Error(err)
			return
		}

		// If the nodePublicationState came from another actor (like tridentctl, or trident node pod)
		// Then we should use those values. If those values are set, these values will be used. Otherwise,
		// we should use the k8sNodePublication state.
		if k8sNodePublicationState != nil {
			if nodePublicationState.OrchestratorReady == nil && k8sNodePublicationState.OrchestratorReady != nil {
				nodePublicationState.OrchestratorReady = k8sNodePublicationState.OrchestratorReady
			}
			if nodePublicationState.AdministratorReady == nil && k8sNodePublicationState.AdministratorReady != nil {
				nodePublicationState.AdministratorReady = k8sNodePublicationState.AdministratorReady
			}
		}

		err = orchestrator.UpdateNode(ctx, nodeName, nodePublicationState)
		if err == nil {
			logEntry.Infof("Updated a node: %s state", nodeName)
		} else {
			logEntry.Error(err)
		}
	}()
	return http.StatusAccepted
}

func UpdateNode(w http.ResponseWriter, r *http.Request) {
	response := &UpdateNodeResponse{}
	UpdateGeneric(w, r, response, nodeUpdater)
}

type GetNodeResponse struct {
	Node  *utils.NodeExternal `json:"node"`
	Error string              `json:"error,omitempty"`
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

type VolumePublicationResponse struct {
	VolumePublication *utils.VolumePublicationExternal `json:"volumePublication"`
	Error             string                           `json:"error,omitempty"`
}

func GetVolumePublication(w http.ResponseWriter, r *http.Request) {
	response := &VolumePublicationResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			pub, err := orchestrator.GetVolumePublication(r.Context(), vars["volume"], vars["node"])
			if err != nil {
				response.Error = err.Error()
			} else {
				response.VolumePublication = pub.ConstructExternal()
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
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
	Logc(ctx).WithFields(LogFields{
		"handler": "ListVolumePublications",
	}).Info("Listed volume publications.")
}

func (r *VolumePublicationsResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
		"handler": "ListVolumePublications",
	}).Error(r.Error)
}

func ListVolumePublications(w http.ResponseWriter, r *http.Request) {
	response := &VolumePublicationsResponse{}
	GetGeneric(w, r, response,
		func(_ map[string]string) int {
			pubs, err := orchestrator.ListVolumePublications(r.Context())
			if err != nil {
				response.Error = err.Error()
			} else {
				response.VolumePublications = pubs
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func ListVolumePublicationsForVolume(w http.ResponseWriter, r *http.Request) {
	response := &VolumePublicationsResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			pubs, err := orchestrator.ListVolumePublicationsForVolume(r.Context(), vars["volume"])
			if err != nil {
				response.Error = err.Error()
			} else {
				response.VolumePublications = pubs
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func ListVolumePublicationsForNode(w http.ResponseWriter, r *http.Request) {
	response := &VolumePublicationsResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			pubs, err := orchestrator.ListVolumePublicationsForNode(r.Context(), vars["node"])
			if err != nil {
				response.Error = err.Error()
			} else {
				response.VolumePublications = pubs
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
	Logc(ctx).WithFields(LogFields{
		"snapshot": r.SnapshotID,
		"handler":  "AddSnapshot",
	}).Info("Added a new volume snapshot.")
}

func (r *AddSnapshotResponse) logFailure(ctx context.Context) {
	Logc(ctx).WithFields(LogFields{
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

func GetCurrentLogLevel(w http.ResponseWriter, r *http.Request) {
	response := &common.GetLogLevelResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			logLevel, err := orchestrator.GetLogLevel(r.Context())
			if err != nil {
				response.Error = err.Error()
			}
			response.LogLevel = logLevel
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type ConfigureLoggingResponse struct {
	Error string `json:"error"`
}

func (r *ConfigureLoggingResponse) setError(err error) {
	r.Error = err.Error()
}

func (r *ConfigureLoggingResponse) isError() bool {
	return r.Error != ""
}

func (r *ConfigureLoggingResponse) logSuccess(ctx context.Context) {
	Logc(ctx).Info("Successfully updated logging configuration.")
}

func (r *ConfigureLoggingResponse) logFailure(ctx context.Context) {
	Logc(ctx).Info("Successfully updated logging configuration.")
}

func SetLogLevel(w http.ResponseWriter, r *http.Request) {
	response := &ConfigureLoggingResponse{}
	UpdateGeneric(w, r, response,
		func(w http.ResponseWriter, r *http.Request, _ httpResponse, vars map[string]string, _ []byte) int {
			err := orchestrator.SetLogLevel(r.Context(), vars["level"])
			if err != nil {
				response.Error = err.Error()
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func GetLoggingWorkflows(w http.ResponseWriter, r *http.Request) {
	response := &common.GetLoggingWorkflowsResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			logWorkflows, err := orchestrator.GetSelectedLoggingWorkflows(r.Context())
			if err != nil {
				response.Error = err.Error()
			}
			response.LogWorkflows = logWorkflows
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type ListLoggingWorkflowsResponse struct {
	AvailableLoggingWorkflows []string `json:"availableLoggingWorkflows"`
	Error                     string   `json:"error,omitempty"`
}

func ListLoggingWorkflows(w http.ResponseWriter, r *http.Request) {
	response := &ListLoggingWorkflowsResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			logWorkflows, err := orchestrator.ListLoggingWorkflows(r.Context())
			if err != nil {
				response.Error = err.Error()
			}
			response.AvailableLoggingWorkflows = logWorkflows
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type SetLoggingWorkflowsRequest struct {
	LoggingWorkflows string `json:"logWorkflows"`
}

func SetLoggingWorkflows(w http.ResponseWriter, r *http.Request) {
	response := &ConfigureLoggingResponse{}
	UpdateGeneric(w, r, response,
		func(w http.ResponseWriter, r *http.Request, _ httpResponse, _ map[string]string, body []byte) int {
			flows := &SetLoggingWorkflowsRequest{}
			err := json.Unmarshal(body, flows)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			err = orchestrator.SetLoggingWorkflows(r.Context(), flows.LoggingWorkflows)
			if err != nil {
				response.Error = err.Error()
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

func GetLoggingLayers(w http.ResponseWriter, r *http.Request) {
	response := &common.GetLoggingLayersResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			logLayers, err := orchestrator.GetSelectedLogLayers(r.Context())
			if err != nil {
				response.Error = err.Error()
			}
			response.LogLayers = logLayers
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type ListLoggingLayersResponse struct {
	AvailableLogLayers []string `json:"availableLogLayers"`
	Error              string   `json:"error,omitempty"`
}

func ListLoggingLayers(w http.ResponseWriter, r *http.Request) {
	response := &ListLoggingLayersResponse{}
	GetGeneric(w, r, response,
		func(vars map[string]string) int {
			logLayers, err := orchestrator.ListLogLayers(r.Context())
			if err != nil {
				response.Error = err.Error()
			}
			response.AvailableLogLayers = logLayers
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}

type SetLoggingLayersRequest struct {
	LogLayers string `json:"logLayers"`
}

func SetLoggingLayers(w http.ResponseWriter, r *http.Request) {
	response := &ConfigureLoggingResponse{}
	UpdateGeneric(w, r, response,
		func(w http.ResponseWriter, r *http.Request, _ httpResponse, _ map[string]string, body []byte) int {
			layers := &SetLoggingLayersRequest{}
			err := json.Unmarshal(body, layers)
			if err != nil {
				response.setError(fmt.Errorf("invalid JSON: %s", err.Error()))
				return httpStatusCodeForAdd(err)
			}
			err = orchestrator.SetLogLayers(r.Context(), layers.LogLayers)
			if err != nil {
				response.Error = err.Error()
			}
			return httpStatusCodeForGetUpdateList(err)
		},
	)
}
