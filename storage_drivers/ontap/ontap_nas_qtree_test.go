// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newNASQtreeStorageDriver(api api.OntapAPI) *NASQtreeStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	config.ManagementLIF = ONTAPTEST_LOCALHOST
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "ontap-nas-qtree-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas-economy"
	config.StoragePrefix = sp("test_")

	nasqtreeDriver := &NASQtreeStorageDriver{}
	nasqtreeDriver.Config = *config
	nasqtreeDriver.qtreesPerFlexvol = defaultQtreesPerFlexvol
	nasqtreeDriver.quotaResizeMap = make(map[string]bool)

	nasqtreeDriver.API = api

	return nasqtreeDriver
}

func newMockOntapNasQtreeDriver(t *testing.T) (*mockapi.MockOntapAPI, *NASQtreeStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	return mockAPI, driver
}

func TestOntapNasQtreeStorageDriverConfigString(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	qtreeDrivers := []NASQtreeStorageDriver{
		*newNASQtreeStorageDriver(mockAPI),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-nas-qtree-user",
		"password":        "password1!",
		"client username": "client_username",
		"client password": "client_password",
	}

	externalIncludeList := map[string]string{
		"<REDACTED>":                   "<REDACTED>",
		"username":                     "Username:<REDACTED>",
		"password":                     "Password:<REDACTED>",
		"api":                          "API:<REDACTED>",
		"chap username":                "ChapUsername:<REDACTED>",
		"chap initiator secret":        "ChapInitiatorSecret:<REDACTED>",
		"chap target username":         "ChapTargetUsername:<REDACTED>",
		"chap target initiator secret": "ChapTargetInitiatorSecret:<REDACTED>",
		"client private key":           "ClientPrivateKey:<REDACTED>",
	}

	for _, qtreeDriver := range qtreeDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, qtreeDriver.String(), val, "ontap-nas-economy driver does not contain %v", key)
			assert.Contains(t, qtreeDriver.GoString(), val, "ontap-nas-economy driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, qtreeDriver.String(), val, "ontap-nas-economy driver contains %v", key)
			assert.NotContains(t, qtreeDriver.GoString(), val, "ontap-nas-economy driver contains %v", key)
		}
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_NoRulesSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"
	rules := []string{"0.0.0.0/0", "::/0"}

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return an empty set of rules when asked for them
	ruleListCall := mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(make(map[string]int), nil)
	// Ensure that the default rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[0],
		gomock.Any()).After(ruleListCall).Return(nil)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[1],
		gomock.Any()).After(ruleListCall).Return(nil)

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(context.Background()); err != nil {
		t.Error(err)
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_RulesExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"
	fakeRules := map[string]int{
		"foo": 0,
		"bar": 1,
		"baz": 2,
	}

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return the fake rules when asked
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(fakeRules, nil)

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(context.Background()); err != nil {
		t.Error(err)
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_ErrorGettingRules(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return an error when asked for export rules
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(nil, fmt.Errorf("foobar"))

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(context.Background()); err == nil {
		t.Error("Error was not propagated")
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_ErrorCreatingRules(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"
	rules := []string{"0.0.0.0/0", "::/0"}

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return an empty set of rules when asked for them
	ruleListCall := mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(make(map[string]int), nil)
	// Ensure that the default rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[0], gomock.Any()).After(ruleListCall).Return(
		fmt.Errorf("foobar"),
	)

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(context.Background()); err == nil {
		t.Error("Error was not propagated")
	}
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_1Gi(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	quotaEntry := &api.QuotaEntry{
		Target:         "",
		DiskLimitBytes: 1024 ^ 3,
	}
	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := quotaEntry.DiskLimitBytes

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(quotaEntry, nil)

	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(context.Background(), qtreeName, flexvolName)
	assert.Nil(t, err, fmt.Sprintf("Unexpected err, %v", err))
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_1Ki(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	quotaEntry := &api.QuotaEntry{
		Target:         "",
		DiskLimitBytes: 1024,
	}
	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := quotaEntry.DiskLimitBytes

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(quotaEntry, nil)

	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(context.Background(), qtreeName, flexvolName)
	assert.Nil(t, err, fmt.Sprintf("Unexpected err, %v", err))
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_Error(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := int64(0)

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(nil, fmt.Errorf("error"))

	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(context.Background(), qtreeName, flexvolName)
	assert.NotNil(t, err, "Unexpected success")
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestCreateQtreeInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	svm := "fakeSVM"
	flexvol := "fakeFlexVol"
	qtree := "fakeQtree"
	testString := fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", svm, flexvol, qtree)
	str := driver.CreateQtreeInternalID(svm, flexvol, qtree)
	assert.Equal(t, testString, str)
}

func TestParseInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	testString := "/svm/fakeSVM/flexvol/fakeFlexvol/qtree/fakeQtree"
	svm, flexvol, qtree, err := driver.ParseQtreeInternalID(testString)
	assert.NoError(t, err, "unexpected error found while parsing InternalId")
	assert.Equal(t, svm, "fakeSVM")
	assert.Equal(t, flexvol, "fakeFlexvol")
	assert.Equal(t, qtree, "fakeQtree")
}

func TestParseInternalIdWithMissingSVM(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	testString := "/flexvol/fakeFlexvol/qtree/fakeQtree"
	_, _, _, err := driver.ParseQtreeInternalID(testString)
	assert.Error(t, err, "expected an error when SVM Name is missing")
}

func TestSetVolumePatternWithInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := "/svm/fakeSVM/flexvol/fakeFlexvol/qtree/fakeQtree"
	internalName := "fakeQtree"
	volumePrefix := "fakePrefix"
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(context.Background(), internalID, internalName,
		volumePrefix)
	assert.NoError(t, err, "unexpected error found while setting setting volume pattern")
	assert.Equal(t, volumePattern, "fakeFlexvol")
	assert.Equal(t, name, "fakeQtree")
}

func TestSetVolumePatternWithMisformedInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := "/flexvol/fakeFlexvol/qtree/fakeQtree"
	internalName := "fakeQtree"
	volumePrefix := "fakePrefix"
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(context.Background(), internalID, internalName,
		volumePrefix)
	assert.Error(t, err, "expected an error when InternalID is misformed")
	assert.Equal(t, volumePattern, "")
	assert.Equal(t, name, "")
}

func TestSetVolumePatternWithoutInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := ""
	internalName := "fakeQtree"
	volumePrefix := "fakePrefix"
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(context.Background(), internalID, internalName,
		volumePrefix)
	assert.NoError(t, err, "unexpected error found while setting setting volume pattern")
	assert.Equal(t, volumePattern, "fakePrefix*")
	assert.Equal(t, name, "fakeQtree")
}

func TestSetVolumePatternWithNoInternalIDAndNoPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := ""
	internalName := "fakeQtree"
	volumePrefix := ""
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(context.Background(), internalID, internalName,
		volumePrefix)
	assert.NoError(t, err, "unexpected error found while setting setting volume pattern")
	assert.Equal(t, volumePattern, "*")
	assert.Equal(t, name, "fakeQtree")
}

func TestNASQtreeStorageDriver_VolumeCreate(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         "qtree1",
		InternalName: "qtree1",
	}
	flexvolName := "flexvol1"
	flexvol := &api.Volume{Name: flexvolName}
	flexvols := []*api.Volume{flexvol}
	sizeBytes := 1073741824
	sizeBytesStr := strconv.FormatUint(uint64(sizeBytes), 10)
	sizeKB := 1048576
	sizeKBStr := strconv.FormatUint(uint64(sizeKB), 10)

	sb := &storage.StorageBackend{}
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		SnapshotDir:       "true",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().QtreeExists(ctx, "qtree1", "*").Return(false, "", nil)
	mockAPI.EXPECT().TieringPolicyValue(ctx).Return("snapshot-only")
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(flexvols, nil)
	mockAPI.EXPECT().QtreeCount(ctx, "flexvol1").Return(0, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "flexvol1").Return(flexvol, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, "flexvol1").Return(api.QuotaEntries{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, flexvolName, sizeBytesStr).Return(nil)
	mockAPI.EXPECT().QtreeCreate(ctx, "qtree1", "flexvol1", "0755", "trident-"+BackendUUID,
		"mixed", "fake-qos-policy").Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, "qtree1", "flexvol1", "tree", sizeKBStr).Return(nil)

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
	assert.Equal(t, "/svm/SVM1/flexvol/flexvol1/qtree/qtree1", volConfig.InternalID)
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "trident-"+BackendUUID, volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
}
