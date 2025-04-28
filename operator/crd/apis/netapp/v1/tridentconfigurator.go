// Copyright 2024 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"fmt"

	"github.com/netapp/trident/pkg/collection"
)

type (
	TConfStatus string
	TConfPhase  string
)

const (
	// TridentConfigurator Status Values

	Processing TConfStatus = "Processing"
	Success    TConfStatus = "Success"
	Failed     TConfStatus = "Failed"

	// TridentConfigurator Phase Values

	ValidatingConfig  TConfPhase = "Validating Config"
	ValidatedConfig   TConfPhase = "Validated Config"
	CreatingBackend   TConfPhase = "Creating Backend"
	CreatedBackend    TConfPhase = "Created Backend"
	CreatingSC        TConfPhase = "Creating Storage Class"
	CreatedSC         TConfPhase = "Created Storage Class"
	CreatingSnapClass TConfPhase = "Creating Snapshot Class"
	Done              TConfPhase = "Done"

	StorageDriverName = "storageDriverName"
	FsxnID            = "fsxnID"
)

func (tc *TridentConfigurator) GetStorageDriverName() (string, error) {
	var tConfSpec map[string]interface{}
	if err := json.Unmarshal(tc.Spec.Raw, &tConfSpec); err != nil {
		return "", err
	}

	if name, ok := tConfSpec[StorageDriverName]; ok {
		return name.(string), nil
	}

	return "", fmt.Errorf("storageDriverName not set")
}

func (tc *TridentConfigurator) IsSpecValid() bool {
	return len(tc.Spec.Raw) != 0
}

func (tc *TridentConfigurator) Validate() error {
	if !tc.IsSpecValid() {
		return fmt.Errorf("empty tconf spec is not allowed")
	}
	return nil
}

func (tc *TridentConfigurator) IsAWSFSxNTconf() (bool, error) {
	var tConfSpec map[string]interface{}
	if err := json.Unmarshal(tc.Spec.Raw, &tConfSpec); err != nil {
		return false, err
	}
	svms, _ := tConfSpec["svms"].([]interface{})
	for _, svm := range svms {
		svmMap, _ := svm.(map[string]interface{})
		fsxnId, ok := svmMap[FsxnID].(string)
		if ok && fsxnId != "" {
			return true, nil
		}
	}
	return false, nil
}

func (in *TridentConfigurator) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentConfigurator) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentConfigurator) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentConfigurator) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
