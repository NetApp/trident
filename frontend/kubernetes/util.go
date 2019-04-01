package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	commontypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"

	"github.com/netapp/trident/config"
	k8sutilversion "github.com/netapp/trident/utils"
)

type PanicKubeVersionError struct {
	Message string
}

func NewPanicKubeVersionError(msg string) *PanicKubeVersionError {
	return &PanicKubeVersionError{msg}
}

func (err *PanicKubeVersionError) Error() string {
	return err.Message
}

func IsPanicKubeVersion(err error) bool {
	_, ok := err.(*PanicKubeVersionError)
	return ok
}

type UnsupportedKubeVersionError struct {
	Message string
}

func NewUnsupportedKubeVersionError(msg string) *UnsupportedKubeVersionError {
	return &UnsupportedKubeVersionError{msg}
}

func (err *UnsupportedKubeVersionError) Error() string {
	return err.Message
}

func IsUnsupportedKubeVersion(err error) bool {
	_, ok := err.(*UnsupportedKubeVersionError)
	return ok
}

func ValidateKubeVersion(versionInfo *k8sversion.Info) (kubeVersion *k8sutilversion.Version, err error) {
	kubeVersion, err = nil, nil
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"panic": r,
			}).Errorf("Kubernetes frontend recovered from a panic!")
			err = NewPanicKubeVersionError(
				fmt.Sprintf("validating the Kubernetes version caused a panic: %v", r))
		}
	}()
	kubeVersion = k8sutilversion.MustParseSemantic(versionInfo.GitVersion)
	if !kubeVersion.AtLeast(k8sutilversion.MustParseSemantic(config.KubernetesVersionMin)) {
		err = NewUnsupportedKubeVersionError(
			fmt.Sprintf("minimum supported version is %v", config.KubernetesVersionMin))
	}
	return
}

func getUniqueClaimName(claim *v1.PersistentVolumeClaim) string {
	id := string(claim.UID)
	r := strings.NewReplacer("-", "", "_", "", " ", "", ",", "")
	id = r.Replace(id)
	if len(id) > 5 {
		id = id[:5]
	}
	return fmt.Sprintf("%s-%s-%s", claim.Namespace, claim.Name, id)
}

// getClaimProvisioner returns the provisioner for a claim
func getClaimProvisioner(claim *v1.PersistentVolumeClaim) string {
	if provisioner, found := claim.Annotations[AnnStorageProvisioner]; found {
		return provisioner
	}
	return ""
}

// PatchPV patches a PV after an update
func PatchPV(kubeClient kubernetes.Interface,
	pv *v1.PersistentVolume,
	pvUpdated *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	oldPVData, err := json.Marshal(pv)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling the PV %q: %v", pv.Name, err)
	}

	newPVData, err := json.Marshal(pvUpdated)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling the PV %q: %v", pvUpdated.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldPVData, newPVData, pvUpdated)
	if err != nil {
		return nil, fmt.Errorf("error in creating the two-way merge patch for PV %q: %v", pvUpdated.Name, err)
	}

	return kubeClient.CoreV1().PersistentVolumes().Patch(pvUpdated.Name,
		commontypes.StrategicMergePatchType, patchBytes)
}

// PatchPVC patches a PVC spec after an update
func PatchPVC(kubeClient kubernetes.Interface,
	pvc *v1.PersistentVolumeClaim,
	pvcUpdated *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	oldPVCData, err := json.Marshal(pvc)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling the PVC %q: %v", pvc.Name, err)
	}

	newPVCData, err := json.Marshal(pvcUpdated)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling the PVC %q: %v", pvcUpdated.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldPVCData, newPVCData, pvcUpdated)
	if err != nil {
		return nil,
			fmt.Errorf("error in creating the two-way merge patch for PV %q: %v",
				pvcUpdated.Name, err)
	}

	return kubeClient.CoreV1().PersistentVolumeClaims(pvcUpdated.Namespace).Patch(pvcUpdated.Name,
		commontypes.StrategicMergePatchType, patchBytes)
}

// PatchPVCStatus patches a PVC status after an update
func PatchPVCStatus(kubeClient kubernetes.Interface,
	pvc *v1.PersistentVolumeClaim,
	pvcUpdated *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	oldPVCData, err := json.Marshal(pvc)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling the PVC %q: %v", pvc.Name, err)
	}

	newPVCData, err := json.Marshal(pvcUpdated)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling the PVC %q: %v", pvcUpdated.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldPVCData, newPVCData, pvcUpdated)
	if err != nil {
		return nil,
			fmt.Errorf("error in creating the two-way merge patch for PV %q: %v",
				pvcUpdated.Name, err)
	}

	return kubeClient.CoreV1().PersistentVolumeClaims(pvcUpdated.Namespace).Patch(pvcUpdated.Name,
		commontypes.StrategicMergePatchType, patchBytes, "status")
}

func getPersistentVolumeClaimDataSource(claim *v1.PersistentVolumeClaim) string {
	if claim.Spec.DataSource == nil {
		return ""
	}
	if claim.Spec.DataSource.Kind != "VolumeSnapshot" {
		return ""
	}
	return claim.Spec.DataSource.Name
}
