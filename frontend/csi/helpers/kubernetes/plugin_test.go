// Copyright 2022 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
)

const (
	FakeStorageClass = "fakeStorageClass"
)

func newMockPlugin(t *testing.T) (*mockcore.MockOrchestrator, *Plugin) {
	mockCtrl := gomock.NewController(t)
	mockCore := mockcore.NewMockOrchestrator(mockCtrl)

	plugin := &Plugin{
		orchestrator: mockCore,
	}

	return mockCore, plugin
}

func TestAddStorageClass_WrongProvisioner(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: "fakeProvisioner",
		Parameters: map[string]string{
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	mockCore.EXPECT().AddStorageClass(gomock.Any(), gomock.Any()).Return(nil, nil).Times(0)

	plugin.addStorageClass(sc)
}

func TestAddStorageClass(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scv1Beta := &k8sstoragev1beta.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scDummy := "StorageClass"

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

	expectedSCConfig := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
		},
	}

	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(2)

	plugin.addStorageClass(sc)
	plugin.addStorageClass(scv1Beta)
	plugin.addStorageClass(scDummy)
}

func TestUpdateStorageClass(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scv1Beta := &k8sstoragev1beta.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scDummy := "StorageClass"

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")
	expectedSCConfig := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
		},
	}

	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(4)

	plugin.addStorageClass(sc)
	plugin.addStorageClass(scv1Beta)

	mockCore.EXPECT().GetStorageClass(gomock.Any(), sc.Name).Return(nil, nil).Times(1)
	plugin.updateStorageClass(nil, sc)
	mockCore.EXPECT().GetStorageClass(gomock.Any(), scv1Beta.Name).Return(nil, nil).Times(1)
	plugin.updateStorageClass(nil, scv1Beta)
	plugin.updateStorageClass(nil, scDummy)
}

func TestDeleteStorageClass(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scv1Beta := &k8sstoragev1beta.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scDummy := "StorageClass"

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

	expectedSCConfig := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
		},
	}

	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(2)
	plugin.addStorageClass(sc)
	plugin.addStorageClass(scv1Beta)
	plugin.addStorageClass(scDummy)
	mockCore.EXPECT().DeleteStorageClass(gomock.Any(), sc.Name).Return(nil).Times(2)
	plugin.deleteStorageClass(sc)
	plugin.deleteStorageClass(scv1Beta)
	plugin.deleteStorageClass(scDummy)
}

func TestProcessDeletedStorageClass_Failure(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)
	ctx := context.TODO()

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	mockCore.EXPECT().DeleteStorageClass(gomock.Any(), sc.Name).Return(nil).Times(1)
	plugin.processDeletedStorageClass(ctx, sc)
}

func TestProcessStorageClass(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)
	ctx := context.TODO()
	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

	expectedSCConfig := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
		},
	}

	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(2)
	plugin.addStorageClass(sc)
	mockCore.EXPECT().GetStorageClass(gomock.Any(), sc.Name).Return(nil, nil).Times(1)
	plugin.processStorageClass(ctx, sc, "update")
	mockCore.EXPECT().DeleteStorageClass(gomock.Any(), sc.Name).Return(nil).Times(1)
	plugin.processStorageClass(ctx, sc, "delete")
}

func TestProcessAddedStorageClass(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)
	ctx := context.TODO()

	tests := []struct {
		Key   string
		Value string
	}{
		// Valid attributes with prefix
		// {"trident.netapp.io/backendType", "ontap-nas"}, //Omitting as this is a mandatory attribute
		{"trident.netapp.io/media", "hdd"},
		{"trident.netapp.io/provisioningType", "thin"},
		{"trident.netapp.io/snapshots", "true"},
		{"trident.netapp.io/clones", "false"},
		{"trident.netapp.io/encryption", "true"},
		{"trident.netapp.io/IOPS", "2"},
		{"trident.netapp.io/storagePools", "backend1:pool1,pool2;backend2:pool1"},
		{"trident.netapp.io/additionalStoragePools", "backend1:pool1,pool2;backend2:pool1"},
		{"trident.netapp.io/excludeStoragePools", "backend1:pool1,pool2;backend2:pool1"},
		{"trident.netapp.io/nasType", "NFS"},
		{"trident.netapp.io/nasType", "Nfs"},
		{"trident.netapp.io/nasType", "nfS"},
		{"trident.netapp.io/nasType", "nfs"},
		// Valid attributes without prefix
		{"media", "hdd"},
		{"provisioningType", "thin"},
		{"snapshots", "true"},
		{"clones", "false"},
		{"encryption", "true"},
		{"IOPS", "2"},
		{"storagePools", "backend1:pool1,pool2;backend2:pool1"},
		{"additionalStoragePools", "backend1:pool1,pool2;backend2:pool1"},
		{"excludeStoragePools", "backend1:pool1,pool2;backend2:pool1"},
		{"nasType", "SMB"},
		{"nasType", "SMb"},
		{"nasType", "sMb"},
		{"nasType", "smb"},
	}

	for _, test := range tests {
		t.Run(test.Key, func(t *testing.T) {
			newKey := removeSCParameterPrefix(test.Key)

			sc := &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: FakeStorageClass,
				},
				Provisioner: csi.Provisioner,
				Parameters: map[string]string{
					"backendType": "ontap-nas",
					newKey:        test.Value,
				},
			}

			var mapAttr map[string][]string
			var valueAttr storageattribute.Request
			if newKey == "storagePools" || newKey == "additionalStoragePools" || newKey == "excludeStoragePools" {
				mapAttr, _ = storageattribute.CreateBackendStoragePoolsMapFromEncodedString(test.Value)
			} else {
				// This check is added as the expected value should always be in lowercase
				if newKey == "nasType" {
					test.Value = strings.ToLower(test.Value)
				}
				valueAttr, _ = storageattribute.CreateAttributeRequestFromAttributeValue(newKey, test.Value)
			}

			backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

			expectedSCConfig := &storageclass.Config{
				Name: FakeStorageClass,
				Attributes: map[string]storageattribute.Request{
					"backendType": backendTypeAttr,
				},
			}

			switch newKey {
			case "storagePools":
				expectedSCConfig.Pools = mapAttr
			case "additionalStoragePools":
				expectedSCConfig.AdditionalPools = mapAttr
			case "excludeStoragePools":
				expectedSCConfig.ExcludePools = mapAttr
			default:
				expectedSCConfig.Attributes[newKey] = valueAttr
			}

			mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(1)
			plugin.processAddedStorageClass(ctx, sc)
		})
	}
}

func TestAddNode(t *testing.T) {
	_, plugin := newMockPlugin(t)
	newNode := &v1.Node{}
	plugin.addNode(newNode)
	plugin.addNode(nil)
}

func TestUpdateNode(t *testing.T) {
	_, plugin := newMockPlugin(t)
	newNode := &v1.Node{}
	plugin.updateNode(nil, newNode)
	plugin.updateNode(nil, nil)
}

func TestDeleteNode(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)
	newNode := &v1.Node{}
	newNode.ObjectMeta.Name = "FakeNode"
	plugin.addNode(newNode)
	mockCore.EXPECT().DeleteNode(gomock.Any(), newNode.Name)
	plugin.deleteNode(newNode)
	plugin.deleteNode(nil)
}

func TestRemoveSCParameterPrefix(t *testing.T) {
	key := "trident.netapp.io/backendType"
	expectedKey := "backendType"
	newKey := removeSCParameterPrefix(key)

	tests := []struct {
		Param       string
		ExpectedVal string
	}{
		// Valid attributes with prefix
		{"trident.netapp.io/backendType", "backendType"},
		{"trident.netapp.io/media", "media"},
		{"trident.netapp.io/provisioningType", "provisioningType"},
		{"trident.netapp.io/snapshots", "snapshots"},
		{"trident.netapp.io/clones", "clones"},
		{"trident.netapp.io/encryption", "encryption"},
		{"trident.netapp.io/IOPS", "IOPS"},
		{"trident.netapp.io/storagePools", "storagePools"},
		{"trident.netapp.io/additionalStoragePools", "additionalStoragePools"},
		{"trident.netapp.io/excludeStoragePools", "excludeStoragePools"},
	}

	for _, test := range tests {
		t.Run(test.Param, func(t *testing.T) {
			newKey := removeSCParameterPrefix(test.Param)
			assert.Equal(t, test.ExpectedVal, newKey)
		})
	}
	assert.Equal(t, expectedKey, newKey)
}

func TestRemoveSCParameterPrefix_Failure(t *testing.T) {
	tests := []struct {
		Param string
	}{
		// Invalid names
		{"trident.netapp.iobackendType"},
		{"trident.netapp.io/"},
	}

	for _, test := range tests {
		t.Run(test.Param, func(t *testing.T) {
			expectedParam := test.Param
			newKey := removeSCParameterPrefix(test.Param)

			// If removeSCParameterPrefix fails , it returns the input key as it is
			// Checks if the expected key is same as the input key is
			assert.Equal(t, expectedParam, newKey, "expected behavior")
		})
	}
}

func TestProcessAddedStorageClass_CreateBackendSPMapFromEncodedString_Failure(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)
	ctx := context.TODO()
	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: FakeStorageClass,
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":            "ontap-nas",
			"additionalStoragePools": "backend1",
			"excludeStoragePools":    "backend2",
			"storagePools":           "backend3",
		},
	}

	aspAttr, aspErr := storageattribute.CreateBackendStoragePoolsMapFromEncodedString("backend1")
	espAttr, espErr := storageattribute.CreateBackendStoragePoolsMapFromEncodedString("backend2")
	spAttr, spErr := storageattribute.CreateBackendStoragePoolsMapFromEncodedString("backend3")
	btAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

	expectedSCConfig := &storageclass.Config{
		Name:            FakeStorageClass,
		AdditionalPools: aspAttr,
		ExcludePools:    espAttr,
		Pools:           spAttr,
		Attributes: map[string]storageattribute.Request{
			"backendType": btAttr,
		},
	}

	assert.NotNil(t, aspErr, espErr, spErr, "expected error")
	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(2)
	plugin.addStorageClass(sc)
	plugin.processAddedStorageClass(ctx, sc)
}

func TestProcessAddedStorageClass_CreateAttributeRequestFromAttributeValue_Failure(t *testing.T) {
	_, plugin := newMockPlugin(t)
	ctx := context.TODO()

	tests := []struct {
		Key   string
		Value string
	}{
		// Invalid attributes
		{"trident.netapp.io/mymedia", "hdd"},
		{"IOPS", "10.52"},
	}

	for _, test := range tests {
		t.Run(test.Key, func(t *testing.T) {
			sc := &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: FakeStorageClass,
				},
				Provisioner: csi.Provisioner,
				Parameters: map[string]string{
					"backendType": "ontap-nas",
					test.Key:      test.Value,
				},
			}

			valueAttr, err := storageattribute.CreateAttributeRequestFromAttributeValue(test.Key, test.Value)
			backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

			expectedSCConfig := &storageclass.Config{
				Name: FakeStorageClass,
				Attributes: map[string]storageattribute.Request{
					"backendType": backendTypeAttr,
				},
			}
			expectedSCConfig.Attributes[test.Key] = valueAttr

			assert.NotNil(t, err, "expected error")
			assert.Nil(t, valueAttr, "expected error")
			plugin.processAddedStorageClass(ctx, sc)
		})
	}
}
