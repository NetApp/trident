// Copyright 2025 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	k8sfakesnapshotter "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	FakeStorageClass = "fakeStorageClass"
)

var (
	noTaint               = v1.Taint{}
	nodeOutOfServiceTaint = v1.Taint{
		Key: "node.kubernetes.io/out-of-service",
	}
	nodeReadyConditionFalse = v1.NodeCondition{
		Type:   v1.NodeReady,
		Status: v1.ConditionFalse,
	}
	nodeReadyConditionUnknown = v1.NodeCondition{
		Type:   v1.NodeReady,
		Status: v1.ConditionUnknown,
	}
	nodeReadyConditionTrue = v1.NodeCondition{
		Type:   v1.NodeReady,
		Status: v1.ConditionTrue,
	}
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func newMockPlugin(t *testing.T) (*mockcore.MockOrchestrator, *helper) {
	mockCtrl := gomock.NewController(t)
	mockCore := mockcore.NewMockOrchestrator(mockCtrl)

	plugin := &helper{
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

	scOtherProvisioner := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other",
		},
		Provisioner: "other",
	}

	scDummy := "StorageClass"

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

	expectedSCConfig := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
		},
	}

	// Ensure add works
	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(1)

	plugin.addStorageClass(sc)

	// Ensure add handles failure
	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, errors.New("failed")).Times(1)

	plugin.addStorageClass(sc)

	// Ensure negative cases cause no call to core
	plugin.addStorageClass(scDummy)
	plugin.addStorageClass(scOtherProvisioner)
}

func TestUpdateStorageClass(t *testing.T) {
	ctx := GenerateRequestContextForLayer(context.TODO(), LogLayerCore)
	mockCore, plugin := newMockPlugin(t)

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            FakeStorageClass,
			ResourceVersion: "1",
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"selector":                  "cluster=Cluster1",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scUpdate := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            FakeStorageClass,
			ResourceVersion: "2",
			Annotations: map[string]string{
				AnnSelector: "cluster=Cluster2",
			},
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType":               "ontap-nas",
			"selector":                  "cluster=Cluster1",
			"csi.storage.k8s.io/fsType": "nfs",
		},
	}

	scOtherProvisioner := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other",
		},
		Provisioner: "other",
	}

	scDummy := "StorageClass"

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")
	selector1, _ := storageattribute.CreateAttributeRequestFromAttributeValue("selector", "cluster=Cluster1")
	selector2, _ := storageattribute.CreateAttributeRequestFromAttributeValue("selector", "cluster=Cluster2")

	expectedSCConfig1 := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
			"selector":    selector1,
		},
	}

	expectedSCConfig2 := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
			"selector":    selector2,
		},
	}

	// Ensure add works
	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig1).Return(nil, nil)

	plugin.addStorageClass(sc)

	// Ensure negative cases cause no call to core
	plugin.updateStorageClass(sc, nil)
	plugin.updateStorageClass(nil, sc)
	plugin.updateStorageClass(sc, sc)
	plugin.updateStorageClass(sc, scOtherProvisioner)
	plugin.updateStorageClass(nil, scDummy)

	// Ensure update works
	mockCore.EXPECT().GetStorageClass(gomock.Any(), FakeStorageClass).Return(
		storageclass.New(expectedSCConfig1).ConstructExternal(ctx), nil).Times(1)
	mockCore.EXPECT().UpdateStorageClass(gomock.Any(), expectedSCConfig2).Return(nil, nil)

	plugin.updateStorageClass(sc, scUpdate)

	// Ensure update handles failure
	mockCore.EXPECT().GetStorageClass(gomock.Any(), FakeStorageClass).Return(
		storageclass.New(expectedSCConfig1).ConstructExternal(ctx), nil).Times(1)
	mockCore.EXPECT().UpdateStorageClass(gomock.Any(), expectedSCConfig2).Return(nil, errors.New("failed"))

	plugin.updateStorageClass(sc, scUpdate)

	// Ensure update treated as add if not in core
	mockCore.EXPECT().GetStorageClass(gomock.Any(), FakeStorageClass).Return(
		nil, errors.NotFoundError("notFound")).Times(1)
	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig2).Return(nil, nil)

	plugin.updateStorageClass(sc, scUpdate)
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

	scOtherProvisioner := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other",
		},
		Provisioner: "other",
	}

	scDummy := "StorageClass"

	// Ensure delete works
	mockCore.EXPECT().DeleteStorageClass(gomock.Any(), sc.Name).Return(nil).Times(1)

	plugin.deleteStorageClass(sc)

	// Ensure delete handles failure
	mockCore.EXPECT().DeleteStorageClass(gomock.Any(), sc.Name).Return(errors.New("failed")).Times(1)

	plugin.deleteStorageClass(sc)

	// Ensure negative cases cause no call to core
	plugin.deleteStorageClass(scDummy)
	plugin.deleteStorageClass(scOtherProvisioner)
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
			plugin.processStorageClass(ctx, sc, false)
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
	mockCore, plugin := newMockPlugin(t)

	tests := []struct {
		key           string
		taint         v1.Taint
		condition     v1.NodeCondition
		expectedFlags *models.NodePublicationStateFlags
		error         error
	}{
		{
			key:           "nodeNotReady",
			taint:         nodeOutOfServiceTaint,
			condition:     nodeReadyConditionFalse,
			expectedFlags: &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
			error:         nil,
		},
		{
			key:           "nodeUnknown",
			taint:         nodeOutOfServiceTaint,
			condition:     nodeReadyConditionUnknown,
			expectedFlags: &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
			error:         errors.New("failed"),
		},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			plugin.enableForceDetach = true

			newNode := &v1.Node{}
			newNode.Name = "fakeNode"
			newNode.Spec.Taints = append(newNode.Spec.Taints, test.taint)
			newNode.Status.Conditions = append(newNode.Status.Conditions, test.condition)

			mockCore.EXPECT().UpdateNode(gomock.Any(), "fakeNode", test.expectedFlags).Return(test.error).Times(1)

			plugin.updateNode(&v1.Node{}, newNode)
		})
	}
}

func TestUpdateNode_OtherType(t *testing.T) {
	_, plugin := newMockPlugin(t)
	plugin.updateNode(nil, &v1.Pod{})
}

func TestDeleteNode(t *testing.T) {
	mockCore, plugin := newMockPlugin(t)
	newNode := &v1.Node{}
	newNode.ObjectMeta.Name = "FakeNode"
	plugin.addNode(newNode)

	tests := []struct {
		Node       *v1.Node
		CoreReturn error
	}{
		// Valid attributes with prefix
		{newNode, nil},
		{newNode, errors.New("failed")},
		{newNode, errors.NotFoundError("not found")},
	}

	for _, test := range tests {
		mockCore.EXPECT().DeleteNode(gomock.Any(), newNode.Name).Return(test.CoreReturn)

		plugin.deleteNode(test.Node)
	}
}

func TestDeleteNode_OtherType(t *testing.T) {
	_, plugin := newMockPlugin(t)
	plugin.deleteNode(&v1.Pod{})
}

func TestGetNodePublicationState(t *testing.T) {
	_, plugin := newMockPlugin(t)

	tests := []struct {
		ForceDetach   bool
		Taint         v1.Taint
		Condition     v1.NodeCondition
		ExpectedFlags *models.NodePublicationStateFlags
		ExpectedError error
	}{
		{
			false,
			noTaint,
			nodeReadyConditionTrue,
			nil,
			nil,
		},
		{
			false,
			nodeOutOfServiceTaint,
			nodeReadyConditionFalse,
			nil,
			nil,
		},
		{
			true,
			noTaint,
			nodeReadyConditionTrue,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(true)},
			nil,
		},
		{
			true,
			nodeOutOfServiceTaint,
			nodeReadyConditionTrue,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(false)},
			nil,
		},
		{
			true,
			noTaint,
			nodeReadyConditionUnknown,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(true)},
			nil,
		},
		{
			true,
			nodeOutOfServiceTaint,
			nodeReadyConditionUnknown,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
			nil,
		},
		{
			true,
			noTaint,
			nodeReadyConditionFalse,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(true)},
			nil,
		},
		{
			true,
			nodeOutOfServiceTaint,
			nodeReadyConditionFalse,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
			nil,
		},
	}

	for _, test := range tests {
		node := &v1.Node{}
		node.ObjectMeta.Name = "FakeNode"
		node.Spec.Taints = append(node.Spec.Taints, test.Taint)
		node.Status.Conditions = append(node.Status.Conditions, test.Condition)

		plugin.enableForceDetach = test.ForceDetach
		plugin.kubeClient = k8sfake.NewSimpleClientset(node)

		result, resultErr := plugin.GetNodePublicationState(context.TODO(), node.Name)

		assert.Equal(t, test.ExpectedFlags, result)
		assert.Equal(t, test.ExpectedError, resultErr)
	}
}

func TestGetNodePublicationState_NotFound(t *testing.T) {
	_, plugin := newMockPlugin(t)
	plugin.enableForceDetach = true
	plugin.kubeClient = k8sfake.NewSimpleClientset()

	result, resultErr := plugin.GetNodePublicationState(context.TODO(), "FakeNode")

	assert.Nil(t, result)
	assert.NotNil(t, resultErr)
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

func TestProcessStorageClass_CreateBackendSPMapFromEncodedString_Failure(t *testing.T) {
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
	plugin.processStorageClass(ctx, sc, false)
}

func TestProcessStorageClass_CreateAttributeRequestFromAttributeValue_Failure(t *testing.T) {
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
			plugin.processStorageClass(ctx, sc, false)
		})
	}
}

func TestListVolumeAttachments(t *testing.T) {
	ctx := context.Background()
	volume := "bar"
	attachmentList := &k8sstoragev1.VolumeAttachmentList{
		Items: []k8sstoragev1.VolumeAttachment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-attachment"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "foo",
					Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volume},
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-wrong-attacher"},
				Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: "no-trident"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-no-source-volume"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "foo",
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-no-node"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "",
					Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volume},
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
		},
	}
	tests := map[string]struct {
		setupMocks func(c *k8sfake.Clientset)
		shouldFail bool
	}{
		"with no error": {
			setupMocks: func(c *k8sfake.Clientset) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, attachmentList, nil
					},
				)
			},
			shouldFail: false,
		},
		"with attachments not found status": {
			setupMocks: func(c *k8sfake.Clientset) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						status := &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}
						return true, nil, status
					},
				)
			},
			shouldFail: true,
		},
		"with k8s clientset failure": {
			setupMocks: func(c *k8sfake.Clientset) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						status := &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusFailure}}
						return true, nil, status
					},
				)
			},
			shouldFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			clientSet := &k8sfake.Clientset{}

			test.setupMocks(clientSet)

			h := &helper{kubeClient: clientSet}
			attachments, err := h.listVolumeAttachments(ctx)

			if test.shouldFail {
				assert.Nil(t, attachments)
				assert.Error(t, err)
			} else {
				assert.NotNil(t, attachments)
				assert.NoError(t, err)
			}
		},
		)
	}
}

func TestReconcileVolumePublications(t *testing.T) {
	ctx := context.Background()
	volume := "bar"
	validVolume := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:       volume,
			AccessMode: "ReadWriteOnce",
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly: false,
			},
		},
	}
	attachmentList := &k8sstoragev1.VolumeAttachmentList{
		Items: []k8sstoragev1.VolumeAttachment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-attachment"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "foo",
					Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volume},
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-wrong-attacher"},
				Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: "no-trident"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-no-source-volume"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "foo",
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-no-node"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "",
					Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volume},
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
		},
	}
	tests := map[string]struct {
		setupMocks func(c *k8sfake.Clientset, o *mockcore.MockOrchestrator)
		shouldFail bool
	}{
		"with no error": {
			setupMocks: func(c *k8sfake.Clientset, o *mockcore.MockOrchestrator) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, attachmentList, nil
					},
				)
				o.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(validVolume, nil)
				o.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.Any()).Return(nil)
			},
			shouldFail: false,
		},
		"with orchestrator error": {
			setupMocks: func(c *k8sfake.Clientset, o *mockcore.MockOrchestrator) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						runtimeObject := &k8sstoragev1.VolumeAttachmentList{Items: nil}
						return true, runtimeObject, nil
					},
				)
				o.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.Any()).Return(errors.New("core error"))
			},
			shouldFail: true,
		},
		"with client set error": {
			setupMocks: func(c *k8sfake.Clientset, o *mockcore.MockOrchestrator) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						runtimeObject := &k8sstoragev1.VolumeAttachmentList{Items: nil}
						return true, runtimeObject, errors.New("client set error")
					},
				)
				// No need to mock the orchestrator here; failing in the client set will cause reconcile to exit.
			},
			shouldFail: true,
		},
		"with nil attachment list": {
			setupMocks: func(c *k8sfake.Clientset, o *mockcore.MockOrchestrator) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, nil
					},
				)
				o.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.Any()).Return(nil)
			},
			shouldFail: false,
		},
		"with not found error": {
			setupMocks: func(c *k8sfake.Clientset, o *mockcore.MockOrchestrator) {
				c.Fake.PrependReactor(
					"list" /* use '*' for all operations */, "*", /* use '*' all object types */
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						status := &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}
						return true, nil, status
					},
				)
				o.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.Any()).Return(nil)
			},
			shouldFail: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			clientSet := &k8sfake.Clientset{}

			test.setupMocks(clientSet, orchestrator)

			h := &helper{kubeClient: clientSet, orchestrator: orchestrator}
			err := h.reconcileVolumePublications(ctx)

			if test.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListAttachmentsAsPublications(t *testing.T) {
	ctx := context.Background()
	volumeID := "bar"
	node := "foo"
	attachments := []k8sstoragev1.VolumeAttachment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "valid-attachment"},
			Spec: k8sstoragev1.VolumeAttachmentSpec{
				Attacher: csi.Provisioner,
				NodeName: node,
				Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volumeID},
			},
			Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-wrong-attacher"},
			Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: "not-trident"},
		},
	}
	volume := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:       volumeID,
			AccessMode: "ReadWriteOnce",
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly: false,
			},
		},
	}
	publications := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName(volumeID, node),
			VolumeName: volumeID,
			NodeName:   node,
			AccessMode: 1, // ReadWriteOnce / SINGLE_NODE_WRITER
			ReadOnly:   false,
		},
	}

	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockOrchestrator.EXPECT().GetVolume(ctx, gomock.Any()).Return(volume, nil)
	h := &helper{orchestrator: mockOrchestrator}
	actualPubs, err := h.listAttachmentsAsPublications(ctx, attachments)
	assert.NoError(t, err)
	assert.Equal(t, publications, actualPubs)
}

func TestListAttachmentsAsPublications_FailsToGetVolume(t *testing.T) {
	ctx := context.Background()
	volumeID := "bar"
	node := "foo"
	attachments := []k8sstoragev1.VolumeAttachment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "valid-attachment"},
			Spec: k8sstoragev1.VolumeAttachmentSpec{
				Attacher: csi.Provisioner,
				NodeName: node,
				Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volumeID},
			},
			Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-wrong-attacher"},
			Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: "not-trident"},
		},
	}
	volume := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:       volumeID,
			AccessMode: "ReadWriteOnce",
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly: false,
			},
		},
	}
	publications := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName(volumeID, node),
			VolumeName: volumeID,
			NodeName:   node,
			AccessMode: 1, // ReadWriteOnce / SINGLE_NODE_WRITER
			ReadOnly:   false,
		},
	}

	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockOrchestrator.EXPECT().GetVolume(ctx, gomock.Any()).Return(volume, errors.New("core error"))
	h := &helper{orchestrator: mockOrchestrator}
	actualPubs, err := h.listAttachmentsAsPublications(ctx, attachments)
	assert.Error(t, err)
	assert.NotEqual(t, publications, actualPubs)
}

func TestIsAttachmentValid(t *testing.T) {
	ctx := context.Background()
	volume := "bar"
	node := "foo"
	tests := map[string]struct {
		shouldBeValid bool
		attachment    k8sstoragev1.VolumeAttachment
	}{
		"with wrong attacher": {
			false,
			k8sstoragev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-wrong-attacher"},
				Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: "no-trident"},
			},
		},
		"with attachment not attached": {
			false,
			k8sstoragev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-not-attached"},
				Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: csi.Provisioner},
				Status:     k8sstoragev1.VolumeAttachmentStatus{Attached: false},
			},
		},
		"with no source volume on attachment": {
			false,
			k8sstoragev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-no-source-volume"},
				Spec:       k8sstoragev1.VolumeAttachmentSpec{Attacher: csi.Provisioner},
				Status:     k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
		},
		"with no node on attachment": {
			false,
			k8sstoragev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-no-node"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: "",
					Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volume},
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
		},
		"with valid attachment": {
			true,
			k8sstoragev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-attachment"},
				Spec: k8sstoragev1.VolumeAttachmentSpec{
					Attacher: csi.Provisioner,
					NodeName: node,
					Source:   k8sstoragev1.VolumeAttachmentSource{PersistentVolumeName: &volume},
				},
				Status: k8sstoragev1.VolumeAttachmentStatus{Attached: true},
			},
		},
	}

	h := &helper{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			isValid := h.isAttachmentValid(ctx, test.attachment)
			assert.Equal(t, test.shouldBeValid, isValid)
		})
	}
}

func TestIsValidResourceName(t *testing.T) {
	tests := map[string]struct {
		name     string
		expected bool
	}{
		"name contains all legal characters": {
			"snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj",
			true,
		},
		"name is greater than 253 characters": {
			fmt.Sprintf("resource-name%v", strings.Join(make([]string, 50), "-test")),
			false,
		},
		"name is empty": {
			// "" is not valid for Kubernetes names.
			"",
			false,
		},
		"name contains illegal character at beginning": {
			// "-" is not valid for end of Kubernetes names.
			"-snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj",
			false,
		},
		"name contains illegal character within": {
			// "_" is not valid for Kubernetes names.
			"snap_2eff1a7e-679d-4fc6-892f-1nridmry3dj",
			false,
		},
		"name contains illegal character at end": {
			// "-" is not valid for end of Kubernetes names.
			"snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj-",
			false,
		},
		"name contains uppercase illegal character at beginning": {
			// Uppercase letters are not valid for Kubernetes names.
			"Snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj",
			false,
		},
	}

	h := &helper{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			isValid := h.IsValidResourceName(test.name)
			assert.Equal(t, test.expected, isValid)
		})
	}
}

func TestGetSnapshotCreateConfig(t *testing.T) {
	snapName := "snap-import"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	snapshotConfig := &storage.SnapshotConfig{
		Version:    config.OrchestratorAPIVersion,
		Name:       snapName,
		VolumeName: volumeName,
	}

	h := &helper{}
	config, err := h.GetSnapshotConfigForCreate(volumeName, snapName)
	assert.NoError(t, err)
	assert.Equal(t, config, snapshotConfig)
}

func TestGetSnapshotConfigForImport(t *testing.T) {
	type input struct {
		volumeName, snapName string
	}

	type event struct {
		error error
		vsc   *vsv1.VolumeSnapshotContent
	}

	type expectation struct {
		config *storage.SnapshotConfig
		error  bool
	}

	type test struct {
		input  input
		event  event
		expect expectation
	}

	snapName := "snap-import"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	internalSnapName := "snap.2023-05-23_175116"

	tests := map[string]test{
		"fails when no snapshot name is supplied": {
			input: input{
				volumeName: volumeName,
				snapName:   "",
			},
			event: event{
				error: nil,
				vsc:   &vsv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: snapName}},
			},
			expect: expectation{
				config: nil,
				error:  true,
			},
		},
		"fails when no volume name is supplied": {
			input: input{
				volumeName: "",
				snapName:   snapName,
			},
			event: event{
				error: nil,
				vsc:   &vsv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: snapName}},
			},
			expect: expectation{
				config: nil,
				error:  true,
			},
		},
		"fails when volume snapshot content is not found": {
			input: input{
				volumeName: volumeName,
				snapName:   snapName,
			},
			event: event{
				error: &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}},
				vsc:   nil,
			},
			expect: expectation{
				config: nil,
				error:  true,
			},
		},
		"fails when volume snapshot internal name is not found": {
			input: input{
				volumeName: volumeName,
				snapName:   snapName,
			},
			event: event{
				error: &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}},
				vsc: &vsv1.VolumeSnapshotContent{
					ObjectMeta: metav1.ObjectMeta{
						Name:        snapName,
						Annotations: map[string]string{AnnInternalSnapshotName: ""},
					},
				},
			},
			expect: expectation{
				config: nil,
				error:  true,
			},
		},
		"succeeds when volume snapshot content and internal name is found": {
			input: input{
				volumeName: volumeName,
				snapName:   snapName,
			},
			event: event{
				error: nil,
				vsc: &vsv1.VolumeSnapshotContent{
					ObjectMeta: metav1.ObjectMeta{
						Name:        snapName,
						Annotations: map[string]string{AnnInternalSnapshotName: internalSnapName},
					},
				},
			},
			expect: expectation{
				config: &storage.SnapshotConfig{
					Version:      config.OrchestratorAPIVersion,
					Name:         snapName,
					InternalName: internalSnapName,
					VolumeName:   volumeName,
				},
				error: false,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Initialize the fake snapshot Client
			fakeClient := k8sfakesnapshotter.NewSimpleClientset()

			fakeClient.Fake.PrependReactor(
				"get" /* use '*' for all operations */, "*", /* use '*' all object types */
				func(actionCopy k8stesting.Action) (bool, runtime.Object, error) {
					switch actionCopy.(type) {
					case k8stesting.GetActionImpl:
						return true, test.event.vsc, test.event.error
					default:
						// Use this to find if any unanticipated actions occurred.
						Log().Errorf("~~~ unhandled type: %T\n", actionCopy)
						return false, nil, nil
					}
				},
			)

			// Inject the fakeClient into a helper.
			h := &helper{snapClient: fakeClient}
			config, err := h.GetSnapshotConfigForImport(ctx, test.input.volumeName, test.input.snapName)
			assert.Equal(t, test.expect.error, err != nil)
			assert.Equal(t, test.expect.config, config)
		})
	}
}

func TestGetSnapshotContentByName(t *testing.T) {
	type event struct {
		error error
		vsc   *vsv1.VolumeSnapshotContent
	}

	type expectation struct {
		vsc   *vsv1.VolumeSnapshotContent
		error bool
	}

	type test struct {
		name   string
		event  event
		expect expectation
	}

	tests := map[string]test{
		"fails when snapshot client api call fails": {
			name: "snapshot-content",
			event: event{
				error: &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusFailure}},
				vsc:   nil,
			},
			expect: expectation{
				error: true,
				vsc:   nil,
			},
		},
		"fails when volume snapshot content is not found": {
			name: "snapshot-content",
			event: event{
				error: &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}},
				vsc:   nil,
			},
			expect: expectation{
				error: true,
				vsc:   nil,
			},
		},
		"succeeds when snapshot client finds volume snapshot content": {
			name: "snapshot-content",
			event: event{
				error: nil,
				vsc:   &vsv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: "snapshot-content"}},
			},
			expect: expectation{
				error: false,
				vsc:   &vsv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: "snapshot-content"}},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Initialize the fake snapshot Client
			fakeClient := k8sfakesnapshotter.NewSimpleClientset()

			// Prepend a reactor for each anticipated event.
			event := test.event
			fakeClient.Fake.PrependReactor(
				"get" /* use '*' for all operations */, "*", /* use '*' all object types */
				func(actionCopy k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					switch actionCopy.(type) {
					case k8stesting.GetActionImpl:
						return true, event.vsc, event.error
					default:
						// Use this to find if any unanticipated actions occurred.
						Log().Errorf("~~~ unhandled type: %T\n", actionCopy)
						return false, nil, nil
					}
				},
			)

			// Inject the fakeClient into a helper.
			h := &helper{snapClient: fakeClient}
			vsc, err := h.getSnapshotContentByName(ctx, test.name)
			assert.Equal(t, test.expect.vsc, vsc)
			assert.Equal(t, test.expect.error, err != nil)
		})
	}
}

func TestGetSnapshotInternalName(t *testing.T) {
	tests := map[string]struct {
		vsc    *vsv1.VolumeSnapshotContent
		errors bool
	}{
		"fails with nil vsc": {
			vsc:    nil,
			errors: true,
		},
		"fails with nil vsc annotations": {
			vsc:    &vsv1.VolumeSnapshotContent{},
			errors: true,
		},
		"fails with empty internalName annotation": {
			vsc: &vsv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{AnnInternalSnapshotName: ""},
				},
			},
			errors: true,
		},
		"succeeds with internalName specified in annotations": {
			vsc: &vsv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{AnnInternalSnapshotName: "snap.2023-05-23_175116"},
				},
			},
			errors: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			h := &helper{}
			internalName, err := h.getSnapshotInternalNameFromAnnotation(context.Background(), test.vsc)
			if test.errors {
				assert.Empty(t, internalName)
				assert.Error(t, err)
			} else {
				assert.NotEmpty(t, internalName)
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStorageClassParameters(t *testing.T) {
	tt := map[string]struct {
		keys   []string
		params map[string]string
		pass   bool
	}{
		"fails when a single key is missing from parameters": {
			keys:   []string{"a", "b"},
			params: map[string]string{"a": ""},
		},
		"fails when all keys are missing from parameters": {
			keys:   []string{"a", "b"},
			params: map[string]string{"c": "", "d": ""},
		},
		"fails when keys are expected but no parameters exist": {
			keys:   []string{"a", "b"},
			params: map[string]string{},
		},
		"succeeds when no keys are missing from parameters": {
			keys:   []string{"a", "b"},
			params: map[string]string{"a": "d", "b": "e", "c": "f"},
			pass:   true,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			p := helper{}
			sc := &k8sstoragev1.StorageClass{Parameters: test.params}
			err := p.validateStorageClassParameters(sc, test.keys...)
			assert.Equal(t, test.pass, err == nil)
		})
	}
}

func TestGetK8sClusterUID(t *testing.T) {
	ctx := context.TODO()
	_, plugin := newMockPlugin(t)

	expectedUID := "test-cluster-uid-123"
	kubeSystemNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "test-cluster-uid-123",
		},
	}

	tests := map[string]struct {
		setupClient func() *k8sfake.Clientset
		expectedUID string
		expectError bool
	}{
		"successfully gets cluster UID": {
			setupClient: func() *k8sfake.Clientset {
				return k8sfake.NewSimpleClientset(kubeSystemNamespace)
			},
			expectedUID: expectedUID,
			expectError: false,
		},
		"fails when kube-system namespace not found": {
			setupClient: func() *k8sfake.Clientset {
				clientSet := k8sfake.NewSimpleClientset()
				clientSet.Fake.PrependReactor(
					"get", "namespaces",
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, &k8serrors.StatusError{
							ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound},
						}
					},
				)
				return clientSet
			},
			expectedUID: "",
			expectError: true,
		},
		"fails when API call fails": {
			setupClient: func() *k8sfake.Clientset {
				clientSet := k8sfake.NewSimpleClientset()
				clientSet.Fake.PrependReactor(
					"get", "namespaces",
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, &k8serrors.StatusError{
							ErrStatus: metav1.Status{Reason: metav1.StatusFailure},
						}
					},
				)
				return clientSet
			},
			expectedUID: "",
			expectError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plugin.kubeClient = test.setupClient()
			uid, err := plugin.getK8sClusterUID(ctx)
			if test.expectError {
				assert.Error(t, err, "should return error when kube-system namespace cannot be retrieved")
				assert.Empty(t, uid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedUID, uid)
			}
		})
	}
}

func TestGetK8sNodeCount(t *testing.T) {
	ctx := context.TODO()
	_, plugin := newMockPlugin(t)

	tests := map[string]struct {
		setupClient   func() *k8sfake.Clientset
		expectedCount int
		expectError   bool
	}{
		"successfully gets node count - single node": {
			setupClient: func() *k8sfake.Clientset {
				node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
				return k8sfake.NewSimpleClientset(node)
			},
			expectedCount: 1,
			expectError:   false,
		},
		"successfully gets node count - multiple nodes": {
			setupClient: func() *k8sfake.Clientset {
				node1 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
				node2 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}}
				node3 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node3"}}
				return k8sfake.NewSimpleClientset(node1, node2, node3)
			},
			expectedCount: 3,
			expectError:   false,
		},
		"successfully gets node count - zero nodes": {
			setupClient: func() *k8sfake.Clientset {
				return k8sfake.NewSimpleClientset()
			},
			expectedCount: 0,
			expectError:   false,
		},
		"fails when API call fails": {
			setupClient: func() *k8sfake.Clientset {
				clientSet := k8sfake.NewSimpleClientset()
				clientSet.Fake.PrependReactor(
					"list", "nodes",
					func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, &k8serrors.StatusError{
							ErrStatus: metav1.Status{Reason: metav1.StatusFailure},
						}
					},
				)
				return clientSet
			},
			expectedCount: 0,
			expectError:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plugin.kubeClient = test.setupClient()

			// Initialize the nodeIndexer and nodeController for testing
			fakeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			plugin.nodeIndexer = fakeIndexer

			if test.expectError {
				// For error case, make the controller not synced
				plugin.nodeController = &fakeController{synced: false}
			} else {
				// For success case, make the controller synced and add nodes to indexer
				plugin.nodeController = &fakeController{synced: true}
				nodeList, _ := plugin.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
				for _, node := range nodeList.Items {
					err := plugin.nodeIndexer.Add(&node)
					assert.NoError(t, err)
				}
			}

			count, err := plugin.getK8sNodeCount(ctx)
			if test.expectError {
				assert.Error(t, err, "should return error when node cache not synchronized")
				assert.Equal(t, 0, count)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedCount, count)
			}
		})
	}
}

func TestIsTopologyInUse(t *testing.T) {
	ctx := context.TODO()
	_, plugin := newMockPlugin(t)

	tt := map[string]struct {
		labels      map[string]string
		injectError bool
		expected    bool
	}{
		"node with nil labels": {
			labels:   nil,
			expected: false,
		},
		"node with empty labels": {
			labels:   map[string]string{},
			expected: false,
		},
		"node with labels, but no topology labels": {
			labels:   map[string]string{"hostname.kubernetes.io/name": "host1"},
			expected: false,
		},
		"node with non-region topology label": {
			labels:   map[string]string{"topology.kubernetes.io/zone": "zone1"},
			expected: false,
		},
		"node with multiple topology labels": {
			labels:   map[string]string{"topology.kubernetes.io/region": "region1", "topology.kubernetes.io/zone": "zone1"},
			expected: true,
		},
		"error while listing the nodes": {
			labels:      map[string]string{"topology.kubernetes.io/region": "region1", "topology.kubernetes.io/zone": "zone1"},
			injectError: true,
			expected:    false,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			// create fake nodes and add to a fake k8s client
			fakeNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "fakeNode", Labels: test.labels}}
			clientSet := k8sfake.NewSimpleClientset(fakeNode)

			// add reactor to either return the list or return error if required
			clientSet.Fake.PrependReactor(
				"list" /* use '*' for all operations */, "*", /* use '*' all object types */
				func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					if test.injectError {
						status := &k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusFailure}}
						return true, nil, status
					} else {
						return true, &v1.NodeList{Items: []v1.Node{*fakeNode}}, nil
					}
				},
			)

			// add the fake client to the plugin
			plugin.kubeClient = clientSet

			// check if the topology is in use
			result := plugin.IsTopologyInUse(ctx)

			assert.Equal(t, test.expected, result, fmt.Sprintf("topology usage not as expected; expected %v, got %v", test.expected, result))
		})
	}
}

func TestGetTridentProtectVersion(t *testing.T) {
	ctx := context.Background()
	_, plugin := newMockPlugin(t)

	tests := []struct {
		name        string
		pods        []v1.Pod
		expectError bool
		expected    string
	}{
		{
			name:        "No Trident Protect pods",
			pods:        []v1.Pod{},
			expectError: false,
			expected:    "",
		},
		{
			name: "Trident Protect controller manager pod with version in trident-protect namespace",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-controller-manager-abc123",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app.kubernetes.io/name":    "trident-protect",
							"app.kubernetes.io/version": "100.2506.0",
						},
					},
				},
			},
			expectError: false,
			expected:    "100.2506.0",
		},
		{
			name: "Trident Protect controller manager pod with version in different namespace",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-controller-manager-xyz789",
						Namespace: "my-custom-namespace",
						Labels: map[string]string{
							"app.kubernetes.io/name":    "trident-protect",
							"app.kubernetes.io/version": "100.2507.1",
						},
					},
				},
			},
			expectError: false,
			expected:    "100.2507.1",
		},
		{
			name: "Multiple Trident Protect pods across namespaces - returns first controller manager found",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-other-pod-abc123",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app.kubernetes.io/name": "trident-protect",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-controller-manager-def456",
						Namespace: "kube-system",
						Labels: map[string]string{
							"app.kubernetes.io/name":    "trident-protect",
							"app.kubernetes.io/version": "100.2508.0",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-controller-manager-ghi789",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app.kubernetes.io/name":    "trident-protect",
							"app.kubernetes.io/version": "100.2509.0",
						},
					},
				},
			},
			expectError: false,
			expected:    "100.2508.0", // First controller manager found (kube-system comes before trident-protect alphabetically)
		},
		{
			name: "Trident Protect pod without controller manager",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-other-pod-abc123",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app.kubernetes.io/name": "trident-protect",
						},
					},
				},
			},
			expectError: false,
			expected:    "",
		},
		{
			name: "Controller manager without version label",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-controller-manager-abc123",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app.kubernetes.io/name": "trident-protect",
						},
					},
				},
			},
			expectError: false,
			expected:    "",
		},
		{
			name: "Pods without proper app.kubernetes.io/name label are ignored",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-controller-manager-abc123",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/version": "100.2510.0",
						},
					},
				},
			},
			expectError: false,
			expected:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a fake clientset with the test pods
			objs := make([]runtime.Object, len(test.pods))
			for i := range test.pods {
				objs[i] = &test.pods[i]
			}
			fakeClientSet := k8sfake.NewSimpleClientset(objs...)

			plugin.kubeClient = fakeClientSet

			version, err := plugin.getTridentProtectVersion(ctx)

			if test.expectError {
				assert.Error(t, err)
				assert.Empty(t, version)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, version)
			}
		})
	}
}

func TestGetK8sPlatformVersion(t *testing.T) {
	ctx := context.Background()
	_, plugin := newMockPlugin(t)

	tests := []struct {
		name        string
		injectError bool
		expectError bool
		expected    string
	}{
		{
			name:        "Success getting platform version",
			injectError: false,
			expectError: false,
			expected:    "v1.21.0",
		},
		{
			name:        "Error getting platform version",
			injectError: true,
			expectError: true,
			expected:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClientSet := k8sfake.NewSimpleClientset()

			if test.injectError {
				fakeClientSet.Fake.PrependReactor("get", "version", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, fmt.Errorf("simulated error")
				})
			}

			plugin.kubeClient = fakeClientSet

			version, err := plugin.getK8sPlatformVersion(ctx)

			if test.expectError {
				assert.Error(t, err)
				assert.Empty(t, version)
			} else {
				assert.NoError(t, err)
				// The fake client returns a default version
				assert.NotEmpty(t, version)
			}
		})
	}
}

func TestUpdateTelemetryFields(t *testing.T) {
	ctx := context.Background()
	_, plugin := newMockPlugin(t)

	// Create fake Kubernetes objects
	kubeSystemNS := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "test-cluster-uid-12345",
		},
	}

	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
	}

	tridentProtectPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trident-protect-controller-manager-abc123",
			Namespace: "trident-protect",
			Labels: map[string]string{
				"app.kubernetes.io/name":    "trident-protect",
				"app.kubernetes.io/version": "100.2506.0",
			},
		},
	}

	// Setup fake clientset with test objects
	objs := []runtime.Object{kubeSystemNS, tridentProtectPod}
	for i := range nodes {
		objs = append(objs, &nodes[i])
	}
	fakeClientSet := k8sfake.NewSimpleClientset(objs...)

	plugin.kubeClient = fakeClientSet

	// Initialize the nodeIndexer with a fake indexer that we can use for testing
	fakeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	plugin.nodeIndexer = fakeIndexer

	// Setup the node controller cache with nodes for testing
	for _, node := range nodes {
		err := plugin.nodeIndexer.Add(&node)
		assert.NoError(t, err)
	}

	// Mark node controller as synced
	plugin.nodeController = &fakeController{synced: true}

	// Create a test telemetry object
	telemetry := &config.Telemetry{}

	// Call the updateTelemetryFields function
	plugin.updateTelemetryFields(ctx, telemetry)

	// Verify that all telemetry fields were updated correctly
	assert.Equal(t, "test-cluster-uid-12345", telemetry.PlatformUID, "should set cluster UID from kube-system namespace")
	assert.Equal(t, 3, telemetry.PlatformNodeCount, "should set node count from cached nodes")
	assert.Equal(t, "100.2506.0", telemetry.TridentProtectVersion, "should set Trident Protect version from controller manager pod label")
	assert.NotEmpty(t, telemetry.PlatformVersion, "should set platform version from Kubernetes server version")

	// Test with a telemetry object that has existing values to ensure they get overwritten
	existingTelemetry := &config.Telemetry{
		PlatformUID:           "old-uid",
		PlatformNodeCount:     999,
		PlatformVersion:       "old-version",
		TridentProtectVersion: "old-protect-version",
	}

	plugin.updateTelemetryFields(ctx, existingTelemetry)

	// Verify that the values were updated, not appended
	assert.Equal(t, "test-cluster-uid-12345", existingTelemetry.PlatformUID, "should overwrite existing cluster UID")
	assert.Equal(t, 3, existingTelemetry.PlatformNodeCount, "should overwrite existing node count")
	assert.Equal(t, "100.2506.0", existingTelemetry.TridentProtectVersion, "should overwrite existing Trident Protect version")
	assert.NotEmpty(t, existingTelemetry.PlatformVersion, "should overwrite existing platform version")
	assert.NotEqual(t, "old-version", existingTelemetry.PlatformVersion, "platform version should be updated, not kept as old value")
}

func TestCreateDynamicTelemetryUpdater(t *testing.T) {
	_, plugin := newMockPlugin(t)

	ctx := context.Background()

	// Create fake Kubernetes objects
	kubeSystemNS := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "test-cluster-uid-12345",
		},
	}

	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
	}

	tridentProtectPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trident-protect-controller-manager-abc123",
			Namespace: "trident-protect",
			Labels: map[string]string{
				"app.kubernetes.io/name":    "trident-protect",
				"app.kubernetes.io/version": "100.2506.0",
			},
		},
	}

	// Setup fake clientset with test objects
	objs := []runtime.Object{kubeSystemNS, tridentProtectPod}
	for i := range nodes {
		objs = append(objs, &nodes[i])
	}
	fakeClientSet := k8sfake.NewSimpleClientset(objs...)

	plugin.kubeClient = fakeClientSet

	// Initialize the nodeIndexer with a fake indexer that we can use for testing
	fakeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	plugin.nodeIndexer = fakeIndexer

	// Setup the node controller cache with nodes for testing
	for _, node := range nodes {
		err := plugin.nodeIndexer.Add(&node)
		assert.NoError(t, err)
	}

	// Mark node controller as synced
	plugin.nodeController = &fakeController{synced: true}

	// Create the telemetry updater
	updater := plugin.createDynamicTelemetryUpdater()
	assert.NotNil(t, updater, "should create a non-nil telemetry updater function")

	// Create a test telemetry object
	telemetry := &config.Telemetry{}

	// Call the updater function
	updater(ctx, telemetry)

	// Verify that the telemetry was updated
	assert.Equal(t, "test-cluster-uid-12345", telemetry.PlatformUID, "updater should set cluster UID from kube-system namespace")
	assert.Equal(t, 3, telemetry.PlatformNodeCount, "updater should set node count from cached nodes")
	assert.Equal(t, "100.2506.0", telemetry.TridentProtectVersion, "updater should set Trident Protect version from controller manager pod label")
	assert.NotEmpty(t, telemetry.PlatformVersion, "updater should set platform version from Kubernetes server version")
}

// fakeController implements cache.SharedIndexInformer interface for testing
type fakeController struct {
	synced bool
}

func (f *fakeController) HasSynced() bool {
	return f.synced
}

// Implement other required methods as no-ops for testing
func (f *fakeController) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeController) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeController) RemoveEventHandler(handle cache.ResourceEventHandlerRegistration) error {
	return nil
}
func (f *fakeController) GetStore() cache.Store                                      { return nil }
func (f *fakeController) GetController() cache.Controller                            { return nil }
func (f *fakeController) Run(stopCh <-chan struct{})                                 {}
func (f *fakeController) HasStarted() bool                                           { return true }
func (f *fakeController) LastSyncResourceVersion() string                            { return "" }
func (f *fakeController) SetWatchErrorHandler(handler cache.WatchErrorHandler) error { return nil }
func (f *fakeController) SetTransform(handler cache.TransformFunc) error             { return nil }
func (f *fakeController) IsStopped() bool                                            { return false }
func (f *fakeController) GetIndexer() cache.Indexer                                  { return nil }
func (f *fakeController) AddIndexers(indexers cache.Indexers) error                  { return nil }
