// Copyright 2022 NetApp, Inc. All Rights Reserved.

package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockapi "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestCombineAccessModes(t *testing.T) {
	accessModesTests := []struct {
		accessModes []config.AccessMode
		expected    config.AccessMode
	}{
		{[]config.AccessMode{config.ModeAny, config.ModeAny}, config.ModeAny},
		{[]config.AccessMode{config.ModeAny, config.ReadWriteOnce}, config.ReadWriteOnce},
		{[]config.AccessMode{config.ModeAny, config.ReadOnlyMany}, config.ReadOnlyMany},
		{[]config.AccessMode{config.ModeAny, config.ReadWriteMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteOnce, config.ModeAny}, config.ReadWriteOnce},
		{[]config.AccessMode{config.ReadWriteOnce, config.ReadWriteOnce}, config.ReadWriteOnce},
		{[]config.AccessMode{config.ReadWriteOnce, config.ReadOnlyMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteOnce, config.ReadWriteMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ModeAny}, config.ReadOnlyMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ReadWriteOnce}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ReadOnlyMany}, config.ReadOnlyMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ReadWriteMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ModeAny}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ReadWriteOnce}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ReadOnlyMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ReadWriteMany}, config.ReadWriteMany},
	}

	for _, tc := range accessModesTests {
		accessMode := CombineAccessModes(context.Background(), tc.accessModes)
		assert.Equal(t, tc.expected, accessMode, "Access Modes not combining as expected!")
	}
}

func TestGetStorageClass(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	o := mockapi.NewMockOrchestrator(ctrl)

	ctx := context.Background()
	backendAndPools1 := "backend1:pool1,pool2"
	backendAndPools2 := "backend2:pool3,pool4"
	options := map[string]string{
		sa.StoragePools:           backendAndPools1,
		sa.AdditionalStoragePools: backendAndPools2,
		"encryption":              "true",
	}

	// sa.StoragePools and sa.AdditionalStoragePools gets deleted from the "options" map
	o.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(nil, fmt.Errorf("error"))
	_, err := GetStorageClass(ctx, options, o)
	assert.Error(t, err, "GetStorageClass from orchestrator returned storage class")

	// GetStorageClass from orchestrator returns matching storageclass from cache
	sce := &storageclass.External{Config: &storageclass.Config{Name: "basicsc"}}
	o.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(sce, nil)
	sc, err := GetStorageClass(ctx, options, o)
	assert.NoError(t, err, "GetStorageClass from orchestrator returned error")
	assert.Equal(t, "basicsc", sc.Name, "Found different storage class in orchestrator")

	// AddStorageClass from orchestrator adds newly created storageclass in cache
	o.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(nil, nil)
	o.EXPECT().AddStorageClass(ctx, gomock.Any()).Return(sce, nil)
	sc, err = GetStorageClass(ctx, options, o)
	assert.NoError(t, err, "Failed to add storage class to orchestrator")
	assert.Equal(t, "basicsc", sc.Name, "Created storage class with different name")

	// AddStorageClass from orchestrator fails in creating/adding the new storageclass to cache
	o.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(nil, nil)
	o.EXPECT().AddStorageClass(ctx, gomock.Any()).Return(nil, fmt.Errorf("error"))
	_, err = GetStorageClass(ctx, options, o)
	assert.Error(t, err, "Added storage class to orchestrator")

	// error condition for CreateBackendStoragePoolsMapFromEncodedString for sa.StoragePools
	options[sa.StoragePools] = "backend1"
	_, err = GetStorageClass(ctx, options, o)
	assert.Error(t, err, "StoragePools storage attribute is correctly encoded")
	delete(options, sa.StoragePools)

	// error condition for CreateBackendStoragePoolsMapFromEncodedString for sa.AdditionalStoragePools
	options[sa.AdditionalStoragePools] = "backend1"
	_, err = GetStorageClass(ctx, options, o)
	assert.Error(t, err, "AdditionalStoragePools storage attribute is correctly encoded")
	delete(options, sa.AdditionalStoragePools)

	// error condition for CreateAttributeRequestFromAttributeValue (error is only logged in this case)
	options["dummy"] = "dummy"
	o.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(nil, fmt.Errorf("error"))
	_, err = GetStorageClass(ctx, options, o)
	assert.Error(t, err, "Correct attribute is added to storage attribute map")
}

func TestGetVolumeConfig(t *testing.T) {
	dummyString := "dummy"
	dummyMap := make([]map[string]string, 0)
	expected := &storage.VolumeConfig{
		Name:                dummyString,
		Size:                "100",
		StorageClass:        dummyString,
		Protocol:            config.Protocol(dummyString),
		AccessMode:          config.AccessMode(dummyString),
		VolumeMode:          config.VolumeMode(dummyString),
		RequisiteTopologies: dummyMap,
		PreferredTopologies: dummyMap,
	}

	vol, err := GetVolumeConfig(context.Background(), dummyString, dummyString, 100, map[string]string{},
		config.Protocol(dummyString), config.AccessMode(dummyString), config.VolumeMode(dummyString),
		dummyMap, dummyMap)

	assert.Nil(t, err, "Error while creating VolumeConfig object")
	assert.Equal(t, expected, vol, "VolumeConfig object does not match")
}
