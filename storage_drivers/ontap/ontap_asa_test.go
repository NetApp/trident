// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func getASAVolumeConfig() storage.VolumeConfig {
	return storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "true",
		FileSystem:   "xfs",
		Name:         "vol1",
		InternalName: "trident-pvc-1234",
	}
}

func newMockOntapASADriver(t *testing.T) (*mockapi.MockOntapAPI, *ASAStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapASADriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}

	return mockAPI, driver
}

func cloneTestOntapASADriver(driver *ASAStorageDriver) *ASAStorageDriver {
	clone := *driver
	return &clone
}

func newTestOntapASADriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, apiOverride api.OntapAPI,
) *ASAStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true
	// config.Labels = map[string]string{"app": "wordpress"}
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-asa-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = sp("test_")

	asaDriver := &ASAStorageDriver{}
	asaDriver.Config = *config

	var ontapAPI api.OntapAPI

	if apiOverride != nil {
		ontapAPI = apiOverride
	} else {
		// ZAPI is not supported in ASA driver. Return Rest Client all the time.
		ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
	}

	asaDriver.API = ontapAPI
	asaDriver.telemetry = &Telemetry{
		Plugin:        asaDriver.Name(),
		StoragePrefix: *asaDriver.Config.StoragePrefix,
		Driver:        asaDriver,
	}

	return asaDriver
}

func TestOntapASAStorageDriver_Resize_Success(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volConfig := getASAVolumeConfig()

	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(1073741824, nil)
	mockAPI.EXPECT().LunSetSize(ctx, "trident-pvc-1234", "2147483648").Return(uint64(214748364), nil)

	err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.NoError(t, err, "Volume resize failed")
}

func TestOntapASAStorageDriver_Resize_LesserSizeThanCurrent(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volConfig := getASAVolumeConfig()

	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(2147483648, nil) // 2GB

	err := driver.Resize(ctx, &volConfig, 1073741824) // 1GB

	assert.Error(t, err, "Expected error when resizing to lesser size than current")
}

func TestOntapASAStorageDriver_Resize_APIErrors(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volConfig := getASAVolumeConfig()

	// Case: Failure while checking if LUN exists
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(false, fmt.Errorf("error checking LUN existence"))

	err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when checking LUN existence")

	// Case: LUN does not exist
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(false, nil)

	err = driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when LUN does not exist")

	// Case: Failure while getting LUN size
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(0, fmt.Errorf("error getting LUN size"))

	err = driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when getting LUN size")

	// Case: Failure while resizing LUN
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(1073741824, nil)
	mockAPI.EXPECT().LunSetSize(ctx, "trident-pvc-1234", "2147483648").Return(uint64(0), fmt.Errorf("error resizing LUN"))

	err = driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when resizing LUN")
}

func TestOntapASAStorageDriver_Import_Managed_Success(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "2g",
		Name:    "lun1",
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}
	igroups := []string{"igroup1", "igroup2"}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
	mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
		"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(
		igroups, nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup1", volConfig.InternalName).Return(nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup2", volConfig.InternalName).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Expected no error in managed import, but got error")
	assert.Equal(t, "2g", volConfig.Size, "Expected volume config to be updated with actual LUN size")
}

func TestOntapASAStorageDriver_Import_UnManaged_Success(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Name:       originalVolumeName,
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "2g",
		Name:    "lun1",
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Expected no error in unmanaged import, but got error")
	assert.Equal(t, "2g", volConfig.Size, "Expected volume config to be updated with actual LUN size")
}

func TestOntapASAStorageDriver_Import_VolumeNotRW(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Name:       originalVolumeName,
		AccessType: "ro",
	}
	lun := api.Lun{
		Size:    "1g",
		Name:    "lun1",
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when volume is not RW, but got none")
}

func TestOntapASAStorageDriver_Import_LunNotOnline(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Name:       originalVolumeName,
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "1g",
		Name:    "lun1",
		State:   "offline",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when LUN is not online, but got none")
}

func TestOntapASAStorageDriver_NameTemplateLabel(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"

	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "2g",
		Name:    originalVolumeName,
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	tests := []struct {
		labelTestName string
		labelKey      string
		labelValue    string
		expectedLabel string
	}{
		{
			"label", "nameTemplate", "dev-test-cluster-1",
			"{\"provisioning\":{\"nameTemplate\":\"dev-test-cluster-1\"}}",
		},
		{
			"emptyLabelKey", "", "dev-test-cluster-1",
			"{\"provisioning\":{\"\":\"dev-test-cluster-1\"}}",
		},
		{
			"emptyLabelValue", "nameTemplate", "",
			"{\"provisioning\":{\"nameTemplate\":\"\"}}",
		},
		{
			"labelValueSpecialCharacter", "nameTemplate^%^^\\u0026\\u0026^%_________", "dev-test-cluster-1%^%",
			"{\"provisioning\":{\"nameTemplate^%^^\\\\u0026\\\\u0026^%_________\":\"dev-test-cluster-1%^%\"}}",
		},
	}

	for _, test := range tests {
		t.Run(test.labelTestName, func(t *testing.T) {
			driver.Config.Labels = map[string]string{
				test.labelKey: test.labelValue,
			}
			pool1.SetAttributes(map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
			})
			driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

			mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
			mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
			mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
				test.expectedLabel).Return(nil)
			mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(nil, nil)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			assert.NoError(t, err, "Volume import fail")
		})
	}
}

func TestOntapASAStorageDriver_NameTemplateLabelLengthExceeding(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	longLabel := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
		"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ" +
		"bQBuHvTRNX6pHRKUXaIrjEpygM4SpaqHYdZ8O1k2meeugg7eXu4dPhqetI3Sip3W4v9QuFkh1YBaI" +
		"9sHE9w5eRxpmTv0POpCB5xAqzmN6XCkxuXKc4yfNS9PRwcTSpvkA3PcKCF3TD1TJU3NYzcChsFQgm" +
		"bAsR32cbJRdsOwx6BkHNfRCji0xSnBFUFUu1sGHfYCmzzd3OmChADIP6RwRtpnqNzvt0CU6uumBnl" +
		"Lc5U7mBI1Ndmqhn0BBSh588thKOQcpD4bvnSBYU788tBeVxQtE8KkdUgKl8574eWldqWDiALwoiCS" +
		"Ae2GuZzwG4ACw2uHdIkjb6FEwapSKCEogr4yWFAVCYPp2pA37Mj88QWN82BEpyoTV6BRAOsubNPfT" +
		"N94X0qCcVaQp4L5bA4SPTQu0ag20a2k9LmVsocy5y11U3ewpzVGtENJmxyuyyAbxOFOkDxKLRMhgs" +
		"uJMhhplD894tkEcPoiFhdsYZbBZ4MOBF6KkuBF5aqMrQbOCFt2vvTN843nRhomVMpY01SNuUeb5mh" +
		"UN53wsqqHSGoYb1eUBDlTUDLFcCcNacxfsILqmthnrD1B5u85jRm1SfkFfuIDOgaaTM9UhxNQ1U6M" +
		"mBaRYBkuGtTScoVTXyF4lij2sj1WWrKb7qWlaUUjxHiaxgLovPWErldCXXkNFsHgc7UYLQLF4j6lO" +
		"I1QdTAyrtCcSxRwdkjBxj8mQy1HblHnaaBwP7Nax9FvIvxpeqyD6s3X1vfFNGAMuRsc9DKmPDfxjh" +
		"qGzRQawFEbbURWij9xleKsUr0yCjukyKsxuaOlwbXnoFh4V3wtidrwrNXieFD608EANwvCp7u2S8Q" +
		"px99T4O87AdQGa5cAX8Ccojd9tENOmQRmOAwVEuFtuogos96TFlq0YHyfESDTB2TWayIuGJvgTIpX" +
		"lthQFQfHVgPpUZdzZMjXry"

	pool1 := storage.NewStoragePool(nil, "pool1")

	driver.Config.Labels = map[string]string{
		"app":     "my-db-app",
		longLabel: "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "1g",
		Name:    originalVolumeName,
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when label length is too long during import, but got none")
}

func TestOntapASAStorageDriver_APIErrors(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "LunInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(
					nil, fmt.Errorf("error while fetching LUN"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error while fetching LUN, got nil.",
		},
		{
			name: "LunInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN info to be nil, got non-nil.",
		},
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil,
					fmt.Errorf("error while fetching volume"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error while fetching volume, got nil.",
		},
		{
			name: "VolumeInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Expected Volume info to be nil, got non-nil.",
		},
		{
			name: "LunRename_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(
					fmt.Errorf("error while renaming LUN"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN rename to fail, but it succeeded",
		},
		{
			name: "LunSetComment_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
					"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(
					fmt.Errorf("error while setting LUN comment"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN set comment to fail, but it succeeded",
		},
		{
			name: "LunListIgroup_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
					"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(
					nil, fmt.Errorf("error while listing igroups of LUN"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN list igroup to fail, but it succeeded",
		},
		{
			name: "LunUnmap_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				igroups := []string{"igroup1", "igroup2"}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
					"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(igroups, nil)
				mockAPI.EXPECT().LunUnmap(ctx, gomock.Any(), volConfig.InternalName).Return(
					fmt.Errorf("error while unmaping igroup of LUN")).AnyTimes()
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN unmap igroup to fail, but it succeeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}
