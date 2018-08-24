// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage_class"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func StorageClassFromPersistentStorageClass(persistent *storageclass.Persistent) (*StorageClass, error) {
	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return nil, err
	}

	return &StorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "StorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: persistent.Config.Name,
		},
		Spec: runtime.RawExtension{
			Raw: config,
		},
	}, nil
}

func PersistentStorageClassFromStorageClass(volume *StorageClass) (*storageclass.Persistent, error) {
	persistent := &storageclass.Persistent{}

	return persistent, json.Unmarshal(volume.Spec.Raw, persistent.Config)
}
