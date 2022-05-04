// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
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

	nasqtreeDriver.API = api

	return nasqtreeDriver
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
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[0]).After(ruleListCall).Return(nil)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[1]).After(ruleListCall).Return(nil)

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
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[0]).After(ruleListCall).Return(
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
