// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func VolumeFromVolumeExternal(persistent *storage.VolumeExternal) (*Volume, error) {
	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return nil, err
	}

	return &Volume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Volume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: persistent.Config.Name,
		},
		Spec: runtime.RawExtension{
			Raw: config,
		},
		Status: VolumeStatus{
			Backend:  persistent.Backend,
			Orphaned: persistent.Orphaned,
			Pool:     persistent.Pool,
		},
	}, nil
}

func VolumeExternalFromVolume(volume *Volume) (*storage.VolumeExternal, error) {
	persistent := &storage.VolumeExternal{
		Backend:  volume.Status.Backend,
		Orphaned: volume.Status.Orphaned,
		Pool:     volume.Status.Pool,
	}

	return persistent, json.Unmarshal(volume.Spec.Raw, persistent.Config)
}
