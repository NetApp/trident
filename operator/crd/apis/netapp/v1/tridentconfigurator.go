// Copyright 2024 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"fmt"
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
