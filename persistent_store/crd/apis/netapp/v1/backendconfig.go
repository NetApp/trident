// Copyright 2021 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils"
)

const (
	BackendDeletionPolicyDelete string = "delete"
	BackendDeletionPolicyRetain string = "retain"
)

type TridentBackendConfigPhase string

const (
	// used for TridentBackendConfigs that failed to bind
	PhaseUnbound TridentBackendConfigPhase = ""
	// used for TridentBackendConfigs that are bound
	PhaseBound TridentBackendConfigPhase = "Bound"
	// used for TridentBackendConfigs that were deleted using deletionPolicy `delete`,
	// but are still bound to a backend that went into a deleting state after a deletion attempt via tbc
	PhaseDeleting TridentBackendConfigPhase = "Deleting"
	// used for TridentBackendConfigs that are unknown after bound e.g. due to tbe CRD deletion etc.
	PhaseUnknown TridentBackendConfigPhase = "Unknown"
	// used for TridentBackendConfigs that lost their underlying
	// TridentBackend. The config was bound to a TridentBackendConfigs
	// and this TridentBackend does not exist any longer
	PhaseLost TridentBackendConfigPhase = "Lost"
)

func (in *TridentBackendConfig) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentBackendConfig) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentBackendConfig) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentBackendConfig) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentBackendConfig) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}

func (s *TridentBackendConfigSpec) ToString() string {
	clone := s.DeepCopy()

	var backendConfigSpec map[string]interface{}
	err := json.Unmarshal(clone.Raw, &backendConfigSpec)
	if err != nil {
		log.Errorf("could not parse JSON configuration: %v", err)
		return ""
	}

	// Redact the credentials information
	backendConfigSpec["credentials"] = "<REDACTED>"

	return fmt.Sprintf("backendConfig: %+v", backendConfigSpec)
}

func (in *TridentBackendConfig) IsSpecValid() bool {
	return len(in.Spec.Raw) != 0
}

// Validate function validates the TridentBackendConfig
func (in *TridentBackendConfig) Validate() error {
	if !in.IsSpecValid() {
		return fmt.Errorf("empty Spec is not allowed")
	}

	if err := in.Spec.SetDefaultsAndValidate(in.Name); err != nil {
		return err
	}

	return nil
}

func (s *TridentBackendConfigSpec) SetDefaultsAndValidate(backendName string) error {

	var backendConfigSpec map[string]interface{}
	err := json.Unmarshal(s.Raw, &backendConfigSpec)
	if err != nil {
		log.Errorf("could not parse JSON configuration: %v", err)
		return fmt.Errorf("could not parse JSON configuration: %v", err)
	}

	// Set the backend name if not set, set it same as the CR name
	if _, ok := backendConfigSpec["backendName"]; !ok {
		backendConfigSpec["backendName"] = backendName
	}

	// Set the deletionPolicy if not set already
	var deletionPolicy string
	if deletionPolicyVal, ok := backendConfigSpec["deletionPolicy"]; !ok {
		backendConfigSpec["deletionPolicy"] = BackendDeletionPolicyDelete
		deletionPolicy = BackendDeletionPolicyDelete
	} else {
		deletionPolicy = fmt.Sprintf("%v", deletionPolicyVal)
	}

	if err = s.ValidateDeletionPolicy(deletionPolicy); err != nil {
		return err
	}

	if s.Raw, err = json.Marshal(backendConfigSpec); err != nil {
		log.Errorf("could not marshal JSON backend configuration: %v", err)
		return err
	}

	return nil
}

func (s *TridentBackendConfigSpec) GetDeletionPolicy() (string, error) {

	var backendConfigSpec map[string]interface{}
	err := json.Unmarshal(s.Raw, &backendConfigSpec)
	if err != nil {
		log.Errorf("could not parse JSON configuration: %v", err)
		return "", fmt.Errorf("could not parse JSON configuration: %v", err)
	}

	// Set the deletionPolicy if not set already
	var deletionPolicy string
	if deletionPolicyVal, ok := backendConfigSpec["deletionPolicy"]; !ok {
		deletionPolicy = BackendDeletionPolicyDelete
	} else {
		deletionPolicy = fmt.Sprintf("%v", deletionPolicyVal)
	}

	return deletionPolicy, nil
}

func (s *TridentBackendConfigSpec) GetSecretName() (string, error) {

	var backendConfigSpec map[string]interface{}
	err := json.Unmarshal(s.Raw, &backendConfigSpec)
	if err != nil {
		log.Errorf("could not parse JSON configuration: %v", err)
		return "", fmt.Errorf("could not parse JSON configuration: %v", err)
	}

	var secretName string

	if credentials, ok := backendConfigSpec["credentials"]; !ok {
		secretName = ""
	} else {
		credentialsMap := credentials.(map[string]interface{})
		if name, ok := credentialsMap["name"]; !ok {
			secretName = ""
		} else {
			secretName = name.(string)
		}
	}

	return secretName, nil
}

func (s *TridentBackendConfigSpec) ValidateDeletionPolicy(deletionPolicy string) error {
	allowedDeletionPolicies := []string{BackendDeletionPolicyDelete, BackendDeletionPolicyRetain}

	if !utils.SliceContainsStringCaseInsensitive(allowedDeletionPolicies, deletionPolicy) {
		return fmt.Errorf("invalid deletion policy applied: %v", deletionPolicy)
	}

	return nil
}
