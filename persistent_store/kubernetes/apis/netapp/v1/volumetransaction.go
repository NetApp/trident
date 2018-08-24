// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func VolumeTransactionFromPersistentVolumeTransaction(op string, volumeConfig *storage.VolumeConfig) (*VolumeTransaction, error) {
	config, err := json.Marshal(volumeConfig)
	if err != nil {
		return nil, err
	}

	return &VolumeTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "VolumeTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeConfig.Name,
		},
		Transaction: VolumeTransactionSpec{
			Operation: op,
			Config: runtime.RawExtension{
				Raw: config,
			},
		},
	}, nil
}

func PersistentVolumeTransactionFromVolumeTransaction(tx *VolumeTransaction) (string, *storage.VolumeConfig, error) {
	config := &storage.VolumeConfig{}

	return tx.Transaction.Operation, config, json.Unmarshal(tx.Transaction.Config.Raw, config)
}
