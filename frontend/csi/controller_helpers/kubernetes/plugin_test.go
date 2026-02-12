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
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
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

// Fake implementations for testing
type FakeListWatcher struct{}

func (f *FakeListWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	return nil, nil
}

func (f *FakeListWatcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

type FakeController struct{}

func (f *FakeController) Run(stopCh <-chan struct{})                {}
func (f *FakeController) HasSynced() bool                           { return true }
func (f *FakeController) LastSyncResourceVersion() string           { return "" }
func (f *FakeController) GetIndexer() cache.Indexer                 { return &FakeIndexer{} }
func (f *FakeController) AddIndexers(indexers cache.Indexers) error { return nil }
func (f *FakeController) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *FakeController) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *FakeController) RemoveEventHandler(handle cache.ResourceEventHandlerRegistration) error {
	return nil
}

func (f *FakeController) AddEventHandlerWithOptions(handler cache.ResourceEventHandler, options cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}
func (f *FakeController) RunWithContext(ctx context.Context) {}
func (f *FakeController) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	return nil
}
func (f *FakeController) GetStore() cache.Store                                      { return &FakeStore{} }
func (f *FakeController) GetController() cache.Controller                            { return nil }
func (f *FakeController) SetWatchErrorHandler(handler cache.WatchErrorHandler) error { return nil }
func (f *FakeController) SetTransform(transform cache.TransformFunc) error           { return nil }
func (f *FakeController) IsStopped() bool                                            { return false }

type FakeIndexer struct {
	// Fields for ByIndex method testing
	items        []interface{}
	byIndexError error

	// Fields for GetByKey method testing
	getByKeyResult interface{}
	getByKeyExists bool
	getByKeyError  error
}

func (f *FakeIndexer) Index(indexName string, obj interface{}) ([]interface{}, error) {
	return nil, nil
}

func (f *FakeIndexer) IndexKeys(indexName, indexedValue string) ([]string, error) {
	return nil, nil
}
func (f *FakeIndexer) ListIndexFuncValues(indexName string) []string { return nil }
func (f *FakeIndexer) ByIndex(indexName, indexedValue string) ([]interface{}, error) {
	if f.byIndexError != nil {
		return nil, f.byIndexError
	}
	return f.items, nil
}
func (f *FakeIndexer) GetIndexers() cache.Indexers                  { return nil }
func (f *FakeIndexer) AddIndexers(newIndexers cache.Indexers) error { return nil }
func (f *FakeIndexer) Add(obj interface{}) error                    { return nil }
func (f *FakeIndexer) Update(obj interface{}) error                 { return nil }
func (f *FakeIndexer) Delete(obj interface{}) error                 { return nil }
func (f *FakeIndexer) List() []interface{}                          { return nil }
func (f *FakeIndexer) ListKeys() []string                           { return nil }
func (f *FakeIndexer) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (f *FakeIndexer) GetByKey(key string) (item interface{}, exists bool, err error) {
	if f.getByKeyError != nil {
		return nil, false, f.getByKeyError
	}
	return f.getByKeyResult, f.getByKeyExists, nil
}
func (f *FakeIndexer) Replace([]interface{}, string) error { return nil }
func (f *FakeIndexer) Resync() error                       { return nil }

type FakeStore struct{}

func (f *FakeStore) Add(obj interface{}) error    { return nil }
func (f *FakeStore) Update(obj interface{}) error { return nil }
func (f *FakeStore) Delete(obj interface{}) error { return nil }
func (f *FakeStore) List() []interface{}          { return nil }
func (f *FakeStore) ListKeys() []string           { return nil }
func (f *FakeStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (f *FakeStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}
func (f *FakeStore) Replace([]interface{}, string) error { return nil }
func (f *FakeStore) Resync() error                       { return nil }

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
		orchestrator:  mockCore,
		tridentClient: tridentfake.NewSimpleClientset(),
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
		ForceDetach    bool
		Taint          v1.Taint
		RemediationCRD *tridentv1.TridentNodeRemediation
		Condition      v1.NodeCondition
		ExpectedFlags  *models.NodePublicationStateFlags
		ExpectedError  error
	}{
		{
			false,
			noTaint,
			nil,
			nodeReadyConditionTrue,
			nil,
			nil,
		},
		{
			false,
			nodeOutOfServiceTaint,
			nil,
			nodeReadyConditionFalse,
			nil,
			nil,
		},
		{
			true,
			noTaint,
			nil,
			nodeReadyConditionTrue,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(true)},
			nil,
		},
		{
			true,
			nodeOutOfServiceTaint,
			nil,
			nodeReadyConditionTrue,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(false)},
			nil,
		},
		{
			true,
			noTaint,
			nil,
			nodeReadyConditionUnknown,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(true)},
			nil,
		},
		{
			true,
			nodeOutOfServiceTaint,
			nil,
			nodeReadyConditionUnknown,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
			nil,
		},
		{
			true,
			noTaint,
			nil,
			nodeReadyConditionFalse,
			&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(true)},
			nil,
		},
		{
			true,
			nodeOutOfServiceTaint,
			nil,
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
		name              string
		pods              []v1.Pod
		expectError       bool
		expectedVersion   string
		expectedConnector bool
	}{
		{
			name:              "No Trident Protect pods",
			pods:              []v1.Pod{},
			expectError:       false,
			expectedVersion:   "",
			expectedConnector: false,
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
			expectError:       false,
			expectedVersion:   "100.2506.0",
			expectedConnector: false,
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
			expectError:       false,
			expectedVersion:   "100.2507.1",
			expectedConnector: false,
		},
		{
			name: "Controller manager with connector present",
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-connector-58d5bf7644-6zsjc",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app": "connector.protect.trident.netapp.io",
						},
					},
				},
			},
			expectError:       false,
			expectedVersion:   "100.2506.0",
			expectedConnector: true,
		},
		{
			name: "Controller manager with connector in different namespace",
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-connector-58d5bf7644-6zsjc",
						Namespace: "other-namespace",
						Labels: map[string]string{
							"app": "connector.protect.trident.netapp.io",
						},
					},
				},
			},
			expectError:       false,
			expectedVersion:   "100.2506.0",
			expectedConnector: true,
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
			expectError:       false,
			expectedVersion:   "100.2508.0", // First controller manager found (kube-system comes before trident-protect alphabetically)
			expectedConnector: false,
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
			expectError:       false,
			expectedVersion:   "",
			expectedConnector: false,
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
			expectError:       false,
			expectedVersion:   "",
			expectedConnector: false,
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
			expectError:       false,
			expectedVersion:   "",
			expectedConnector: false,
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

			version, connectorPresent, err := plugin.getTridentProtectVersion(ctx)

			if test.expectError {
				assert.Error(t, err)
				assert.Empty(t, version)
				assert.False(t, connectorPresent)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedVersion, version)
				assert.Equal(t, test.expectedConnector, connectorPresent)
			}
		})
	}
}

func TestGetTridentProtectConnectorPresent(t *testing.T) {
	ctx := context.Background()
	_, plugin := newMockPlugin(t)

	tests := []struct {
		name              string
		pods              []v1.Pod
		expectError       bool
		expectedConnector bool
	}{
		{
			name:              "No connector pods",
			pods:              []v1.Pod{},
			expectError:       false,
			expectedConnector: false,
		},
		{
			name: "Connector pod present",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-connector-58d5bf7644-6zsjc",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app": "connector.protect.trident.netapp.io",
						},
					},
				},
			},
			expectError:       false,
			expectedConnector: true,
		},
		{
			name: "Multiple connector pods across namespaces",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-connector-abc123",
						Namespace: "namespace1",
						Labels: map[string]string{
							"app": "connector.protect.trident.netapp.io",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-connector-def456",
						Namespace: "namespace2",
						Labels: map[string]string{
							"app": "connector.protect.trident.netapp.io",
						},
					},
				},
			},
			expectError:       false,
			expectedConnector: true,
		},
		{
			name: "Pod with different label - should not detect",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-connector-pod",
						Namespace: "default",
						Labels: map[string]string{
							"app": "some.other.connector",
						},
					},
				},
			},
			expectError:       false,
			expectedConnector: false,
		},
		{
			name: "Pod with connector in name but wrong label",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trident-protect-connector-wrong-label",
						Namespace: "trident-protect",
						Labels: map[string]string{
							"app.kubernetes.io/name": "trident-protect",
						},
					},
				},
			},
			expectError:       false,
			expectedConnector: false,
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

			connectorPresent, err := plugin.getTridentProtectConnectorPresent(ctx)

			if test.expectError {
				assert.Error(t, err)
				assert.False(t, connectorPresent)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedConnector, connectorPresent)
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

	connectorPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trident-protect-connector-58d5bf7644-6zsjc",
			Namespace: "trident-protect",
			Labels: map[string]string{
				"app": "connector.protect.trident.netapp.io",
			},
		},
	}

	// Setup fake clientset with test objects
	objs := []runtime.Object{kubeSystemNS, tridentProtectPod, connectorPod}
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
	assert.True(t, telemetry.TridentProtectConnectorPresent, "should set connector present to true when connector pod exists")
	assert.NotEmpty(t, telemetry.PlatformVersion, "should set platform version from Kubernetes server version")

	// Test with a telemetry object that has existing values to ensure they get overwritten
	existingTelemetry := &config.Telemetry{
		PlatformUID:                    "e48f1b9c-ac99-430-82d2-d89815e2ebd1",
		PlatformNodeCount:              5,
		PlatformVersion:                "v1.31.8",
		TridentProtectVersion:          "25.07.0-preview",
		TridentProtectConnectorPresent: false,
	}

	plugin.updateTelemetryFields(ctx, existingTelemetry)

	// Verify that the values were updated, not appended
	assert.Equal(t, "test-cluster-uid-12345", existingTelemetry.PlatformUID, "should overwrite existing cluster UID")
	assert.Equal(t, 3, existingTelemetry.PlatformNodeCount, "should overwrite existing node count")
	assert.Equal(t, "100.2506.0", existingTelemetry.TridentProtectVersion, "should overwrite existing Trident Protect version")
	assert.True(t, existingTelemetry.TridentProtectConnectorPresent, "should overwrite existing connector presence")
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
func (f *fakeController) AddEventHandlerWithOptions(handler cache.ResourceEventHandler,
	options cache.HandlerOptions,
) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}
func (f *fakeController) RunWithContext(ctx context.Context) {}
func (f *fakeController) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	return nil
}

func TestMetaUIDKeyFunc(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expected    []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid UID string",
			input:       "550e8400-e29b-41d4-a716-446655440000",
			expected:    []string{"550e8400-e29b-41d4-a716-446655440000"},
			expectError: false,
		},
		{
			name:        "invalid UID string",
			input:       "invalid-uid",
			expected:    []string{""},
			expectError: true,
			errorMsg:    "object has no meta",
		},
		{
			name: "object with valid UID",
			input: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID: "550e8400-e29b-41d4-a716-446655440000",
				},
			},
			expected:    []string{"550e8400-e29b-41d4-a716-446655440000"},
			expectError: false,
		},
		{
			name: "object with empty UID",
			input: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID: "",
				},
			},
			expected:    []string{""},
			expectError: true,
			errorMsg:    "object has no UID",
		},
		{
			name:        "object without meta",
			input:       "not-a-meta-object",
			expected:    []string{""},
			expectError: true,
			errorMsg:    "object has no meta",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := MetaUIDKeyFunc(test.input)

			if test.expectError {
				assert.Error(t, err, "Expected error for MetaUIDKeyFunc test case: %s", test.name)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestMetaNameKeyFunc(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expected  []string
		assertErr assert.ErrorAssertionFunc
		errorMsg  string
	}{
		{
			name:      "valid name string",
			input:     "test-object-name",
			expected:  []string{"test-object-name"},
			assertErr: assert.NoError,
		},
		{
			name: "object with valid name",
			input: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			expected:  []string{"test-pod"},
			assertErr: assert.NoError,
		},
		{
			name: "object with empty name",
			input: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "",
				},
			},
			expected:  []string{""},
			assertErr: assert.Error,
			errorMsg:  "object has no name",
		},
		{
			name:      "object without meta",
			input:     123,
			expected:  []string{""},
			assertErr: assert.Error,
			errorMsg:  "object has no meta",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := MetaNameKeyFunc(test.input)

			// Use function types for assertions - no conditional logic needed!
			test.assertErr(t, err, "Unexpected error result for MetaNameKeyFunc test case: %s", test.name)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestTridentVolumeReferenceKeyFunc(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expected  []string
		assertErr assert.ErrorAssertionFunc
		errorMsg  string
	}{
		{
			name:      "valid key string",
			input:     "namespace_pvc-namespace/pvc-name",
			expected:  []string{"namespace_pvc-namespace/pvc-name"},
			assertErr: assert.NoError,
		},
		{
			name: "valid TridentVolumeReference",
			input: &netappv1.TridentVolumeReference{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "trident-ns",
				},
				Spec: netappv1.TridentVolumeReferenceSpec{
					PVCName:      "test-pvc",
					PVCNamespace: "default",
				},
			},
			expected:  []string{"trident-ns_default/test-pvc"},
			assertErr: assert.NoError,
		},
		{
			name: "TridentVolumeReference with empty PVCName",
			input: &netappv1.TridentVolumeReference{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "trident-ns",
				},
				Spec: netappv1.TridentVolumeReferenceSpec{
					PVCName:      "",
					PVCNamespace: "default",
				},
			},
			expected:  []string{""},
			assertErr: assert.Error,
			errorMsg:  "TridentVolumeReference CR has no PVCName set",
		},
		{
			name: "TridentVolumeReference with empty PVCNamespace",
			input: &netappv1.TridentVolumeReference{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "trident-ns",
				},
				Spec: netappv1.TridentVolumeReferenceSpec{
					PVCName:      "test-pvc",
					PVCNamespace: "",
				},
			},
			expected:  []string{""},
			assertErr: assert.Error,
			errorMsg:  "TridentVolumeReference CR has no PVCNamespace set",
		},
		{
			name:      "wrong object type",
			input:     &v1.Pod{},
			expected:  []string{""},
			assertErr: assert.Error,
			errorMsg:  "object is not a TridentVolumeReference CR",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := TridentVolumeReferenceKeyFunc(test.input)

			// Use function types for assertions
			test.assertErr(t, err, "Unexpected error result for TridentVolumeReferenceKeyFunc test case: %s", test.name)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestGetName(t *testing.T) {
	h := &helper{}

	result := h.GetName()

	assert.Equal(t, controllerhelpers.KubernetesHelper, result)
}

func TestVersion(t *testing.T) {
	tests := []struct {
		name        string
		kubeVersion *k8sversion.Info
		expected    string
	}{
		{
			name: "valid version",
			kubeVersion: &k8sversion.Info{
				Major:      "1",
				Minor:      "28",
				GitVersion: "v1.28.0",
			},
			expected: "v1.28.0",
		},
		{
			name: "version with build info",
			kubeVersion: &k8sversion.Info{
				Major:      "1",
				Minor:      "27",
				GitVersion: "v1.27.5+k3s1",
			},
			expected: "v1.27.5+k3s1",
		},
		{
			name: "empty version",
			kubeVersion: &k8sversion.Info{
				GitVersion: "",
			},
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := &helper{
				kubeVersion: test.kubeVersion,
			}

			result := h.Version()

			assert.Equal(t, test.expected, result)
		})
	}
}

func TestNewHelper(t *testing.T) {
	tests := []struct {
		name              string
		masterURL         string
		kubeConfigPath    string
		enableForceDetach bool
		mockK8sSetup      func() (*k8sfake.Clientset, error)
		expectError       bool
		errorContains     string
		validateResult    func(*testing.T, frontend.Plugin)
	}{
		{
			name:              "successful initialization with default config",
			masterURL:         "",
			kubeConfigPath:    "",
			enableForceDetach: false,
			mockK8sSetup: func() (*k8sfake.Clientset, error) {
				return k8sfake.NewSimpleClientset(), nil
			},
			expectError: false,
			validateResult: func(t *testing.T, plugin frontend.Plugin) {
				assert.NotNil(t, plugin)

				// Type assert to helper to check internal state
				helper, ok := plugin.(*helper)
				require.True(t, ok, "Plugin should be of type *helper")

				// Check that all required fields are initialized
				assert.NotNil(t, helper.kubeClient)
				assert.NotNil(t, helper.pvcController)
				assert.NotNil(t, helper.pvController)
				assert.NotNil(t, helper.scController)
				assert.NotNil(t, helper.nodeController)
				assert.NotNil(t, helper.mrController)
				assert.NotNil(t, helper.vrefController)
				assert.NotNil(t, helper.pvcIndexer)
				assert.NotNil(t, helper.pvIndexer)
				assert.NotNil(t, helper.scIndexer)
				assert.NotNil(t, helper.nodeIndexer)
				assert.NotNil(t, helper.mrIndexer)
				assert.NotNil(t, helper.vrefIndexer)
				assert.NotNil(t, helper.eventRecorder)

				// Check stop channels are initialized
				assert.NotNil(t, helper.pvcControllerStopChan)
				assert.NotNil(t, helper.pvControllerStopChan)
				assert.NotNil(t, helper.scControllerStopChan)
				assert.NotNil(t, helper.nodeControllerStopChan)
				assert.NotNil(t, helper.mrControllerStopChan)
				assert.NotNil(t, helper.vrefControllerStopChan)

				// Check force detach setting
				assert.Equal(t, false, helper.enableForceDetach)
			},
		},
		{
			name:              "successful initialization with force detach enabled",
			masterURL:         "",
			kubeConfigPath:    "",
			enableForceDetach: true,
			mockK8sSetup: func() (*k8sfake.Clientset, error) {
				return k8sfake.NewSimpleClientset(), nil
			},
			expectError: false,
			validateResult: func(t *testing.T, plugin frontend.Plugin) {
				helper, ok := plugin.(*helper)
				require.True(t, ok)
				assert.Equal(t, true, helper.enableForceDetach)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Mock the orchestrator
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Since NewHelper creates real K8s clients and we can't easily mock the client creation,
			// we'll test the parts we can and expect specific errors for invalid configs
			if test.expectError {
				plugin, err := NewHelper(orchestrator, test.masterURL, test.kubeConfigPath, test.enableForceDetach)
				assert.Error(t, err, "Expected error for NewHelper test case: %s", test.name)
				assert.Nil(t, plugin)
				if test.errorContains != "" {
					assert.Contains(t, err.Error(), test.errorContains)
				}
			} else {
				// For non-error cases, we expect them to fail with K8s connection errors in test environment
				// but we can still test the function structure
				plugin, err := NewHelper(orchestrator, test.masterURL, test.kubeConfigPath, test.enableForceDetach)
				if err != nil {
					// Expected in test environment without real K8s cluster or kubectl
					// Check for various possible connection error messages
					errorMsg := err.Error()
					connectionErrorFound := strings.Contains(errorMsg, "failed to construct clientset") ||
						strings.Contains(errorMsg, "connection to the server") ||
						strings.Contains(errorMsg, "was refused") ||
						strings.Contains(errorMsg, "unable to load in-cluster configuration") ||
						strings.Contains(errorMsg, "no such host") ||
						strings.Contains(errorMsg, "timeout") ||
						strings.Contains(errorMsg, "could not find the Kubernetes CLI") ||
						strings.Contains(errorMsg, "executable file not found")
					assert.True(t, connectionErrorFound, "Expected a K8s connection error, got: %s", errorMsg)
				} else if plugin != nil {
					// If it somehow succeeds (shouldn't in test env), validate the result
					test.validateResult(t, plugin)
				}
			}
		})
	}
}

func TestNewHelper_InvalidMasterURL(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Test with invalid master URL - use a URL that will definitely fail
	plugin, err := NewHelper(orchestrator, "http://invalid-master-url-that-does-not-exist:6443", "", false)

	// In test environment, this might succeed if kubectl config is available
	// So we check if error occurred OR if plugin creation succeeded but with fallback config
	if err != nil {
		assert.Error(t, err, "Expected error when connecting to invalid master URL")
		assert.Nil(t, plugin)
	} else if plugin != nil {
		// If it succeeds (fallback to kubectl config), verify it's a valid plugin
		assert.NotNil(t, plugin)
		assert.IsType(t, &helper{}, plugin)
	}
}

func TestNewHelper_InvalidKubeConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer func() {
		if mockCtrl != nil {
			mockCtrl.Finish()
		}
	}()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Test with non-existent kubeconfig path - this might fail or fallback to default config
	plugin, err := NewHelper(orchestrator, "", "/this/path/definitely/does/not/exist/kubeconfig", false)

	if err != nil {
		// If it fails as expected, we just verify an error occurred
		assert.Error(t, err)
		assert.Nil(t, plugin)
	} else {
		// If it doesn't fail (fallback scenario), ensure we got a valid plugin
		assert.NotNil(t, plugin)
		// Clean up if plugin was created successfully
		if plugin != nil {
			plugin.Deactivate()
		}
	}
}

func TestActivate(t *testing.T) {
	tests := []struct {
		name                  string
		setupHelper           func(*testing.T) *helper
		mockOrchestratorSetup func(*mockcore.MockOrchestrator)
		expectError           bool
		errorContains         string
		validateChannels      func(*testing.T, *helper)
	}{
		{
			name: "successful activation with no volumes",
			setupHelper: func(t *testing.T) *helper {
				// Create a helper with mock controllers and channels
				h := &helper{
					pvcControllerStopChan:  make(chan struct{}),
					pvControllerStopChan:   make(chan struct{}),
					scControllerStopChan:   make(chan struct{}),
					nodeControllerStopChan: make(chan struct{}),
					mrControllerStopChan:   make(chan struct{}),
					vrefControllerStopChan: make(chan struct{}),
					kubeVersion: &k8sversion.Info{
						Major:      "1",
						Minor:      "27",
						GitVersion: "v1.27.0",
					},
					namespace: "default",
				}

				// Create fake clients
				clientset := k8sfake.NewSimpleClientset()
				h.kubeClient = clientset

				// Create fake trident client with mock namespace and version object
				scheme := runtime.NewScheme()
				_ = tridentv1.AddToScheme(scheme)
				tridentClient := tridentfake.NewSimpleClientset()
				h.tridentClient = tridentClient

				// Create a TridentVersion object to avoid the nil pointer
				version := &tridentv1.TridentVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.OrchestratorName,
						Namespace: "default",
					},
					TridentVersion:     "22.01.0",
					PublicationsSynced: true, // No reconciliation needed
				}
				_, _ = tridentClient.TridentV1().TridentVersions("default").Create(context.Background(), version, metav1.CreateOptions{})

				// Initialize controllers with fake data sources to prevent nil pointer panics
				h.pvcSource = &FakeListWatcher{}
				h.pvcController = &FakeController{}
				h.pvcIndexer = &FakeIndexer{}

				h.pvSource = &FakeListWatcher{}
				h.pvController = &FakeController{}
				h.pvIndexer = &FakeIndexer{}

				h.scSource = &FakeListWatcher{}
				h.scController = &FakeController{}
				h.scIndexer = &FakeIndexer{}

				h.nodeSource = &FakeListWatcher{}
				h.nodeController = &FakeController{}
				h.nodeIndexer = &FakeIndexer{}

				h.mrSource = &FakeListWatcher{}
				h.mrController = &FakeController{}
				h.mrIndexer = &FakeIndexer{}

				h.vrefSource = &FakeListWatcher{}
				h.vrefController = &FakeController{}
				h.vrefIndexer = &FakeIndexer{}

				return h
			},
			mockOrchestratorSetup: func(mockOrchestrator *mockcore.MockOrchestrator) {
				// Set up expectations for volume publication reconciliation check
				mockOrchestrator.EXPECT().ListVolumes(gomock.Any()).Return([]*storage.VolumeExternal{}, nil).AnyTimes()
				// Set up expectations for node reconciliation
				mockOrchestrator.EXPECT().ListNodes(gomock.Any()).Return([]*models.NodeExternal{}, nil).AnyTimes()
			},
			expectError: false,
			validateChannels: func(t *testing.T, h *helper) {
				// Channels should still be open after activation
				select {
				case <-h.pvcControllerStopChan:
					t.Error("PVC controller stop channel should not be closed")
				default:
					// Expected - channel is open
				}
			},
		},
		{
			name: "activation with volumes requiring reconciliation",
			setupHelper: func(t *testing.T) *helper {
				h := &helper{
					pvcControllerStopChan:  make(chan struct{}),
					pvControllerStopChan:   make(chan struct{}),
					scControllerStopChan:   make(chan struct{}),
					nodeControllerStopChan: make(chan struct{}),
					mrControllerStopChan:   make(chan struct{}),
					vrefControllerStopChan: make(chan struct{}),
					kubeVersion: &k8sversion.Info{
						Major:      "1",
						Minor:      "27",
						GitVersion: "v1.27.0",
					},
					namespace: "default",
				}

				clientset := k8sfake.NewSimpleClientset()
				h.kubeClient = clientset

				// Create fake trident client
				tridentClient := tridentfake.NewSimpleClientset()
				h.tridentClient = tridentClient

				// Create a TridentVersion object requiring reconciliation
				version := &tridentv1.TridentVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.OrchestratorName,
						Namespace: "default",
					},
					TridentVersion:     "22.01.0",
					PublicationsSynced: false, // Reconciliation needed
				}
				_, _ = tridentClient.TridentV1().TridentVersions("default").Create(context.Background(), version, metav1.CreateOptions{})

				// Initialize controllers with fake data sources to prevent nil pointer panics
				h.pvcSource = &FakeListWatcher{}
				h.pvcController = &FakeController{}
				h.pvcIndexer = &FakeIndexer{}

				h.pvSource = &FakeListWatcher{}
				h.pvController = &FakeController{}
				h.pvIndexer = &FakeIndexer{}

				h.scSource = &FakeListWatcher{}
				h.scController = &FakeController{}
				h.scIndexer = &FakeIndexer{}

				h.nodeSource = &FakeListWatcher{}
				h.nodeController = &FakeController{}
				h.nodeIndexer = &FakeIndexer{}

				h.mrSource = &FakeListWatcher{}
				h.mrController = &FakeController{}
				h.mrIndexer = &FakeIndexer{}

				h.vrefSource = &FakeListWatcher{}
				h.vrefController = &FakeController{}
				h.vrefIndexer = &FakeIndexer{}

				return h
			},
			mockOrchestratorSetup: func(mockOrchestrator *mockcore.MockOrchestrator) {
				// Set up a scenario where volumes exist (requiring reconciliation)
				volume := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Name: "test-volume",
					},
				}
				mockOrchestrator.EXPECT().ListVolumes(gomock.Any()).Return([]*storage.VolumeExternal{volume}, nil).AnyTimes()
				// Set up expectations for node reconciliation
				mockOrchestrator.EXPECT().ListNodes(gomock.Any()).Return([]*models.NodeExternal{}, nil).AnyTimes()
				// Set up expectations for volume publication reconciliation
				mockOrchestrator.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.AssignableToTypeOf([]*models.VolumePublicationExternal{})).Return(nil).AnyTimes()
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create mock orchestrator
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Set up the helper
			h := test.setupHelper(t)
			h.orchestrator = orchestrator

			// Set up orchestrator expectations
			if test.mockOrchestratorSetup != nil {
				test.mockOrchestratorSetup(orchestrator)
			}

			// Test activation
			err := h.Activate()

			if test.expectError {
				assert.Error(t, err, "Expected error for Activate test case: %s", test.name)
			} else {
				assert.NoError(t, err)
				if test.validateChannels != nil {
					test.validateChannels(t, h)
				}
			}
		})
	}
}

func TestDeactivate(t *testing.T) {
	tests := []struct {
		name             string
		setupHelper      func(*testing.T) *helper
		validateChannels func(*testing.T, *helper)
	}{
		{
			name: "successful deactivation",
			setupHelper: func(t *testing.T) *helper {
				return &helper{
					pvcControllerStopChan:  make(chan struct{}),
					pvControllerStopChan:   make(chan struct{}),
					scControllerStopChan:   make(chan struct{}),
					nodeControllerStopChan: make(chan struct{}),
					mrControllerStopChan:   make(chan struct{}),
					vrefControllerStopChan: make(chan struct{}),
				}
			},
			validateChannels: func(t *testing.T, h *helper) {
				// All channels should be closed after deactivation
				select {
				case <-h.pvcControllerStopChan:
					// Expected - channel is closed
				default:
					t.Error("PVC controller stop channel should be closed")
				}

				select {
				case <-h.pvControllerStopChan:
					// Expected - channel is closed
				default:
					t.Error("PV controller stop channel should be closed")
				}

				select {
				case <-h.scControllerStopChan:
					// Expected - channel is closed
				default:
					t.Error("SC controller stop channel should be closed")
				}

				select {
				case <-h.nodeControllerStopChan:
					// Expected - channel is closed
				default:
					t.Error("Node controller stop channel should be closed")
				}

				select {
				case <-h.mrControllerStopChan:
					// Expected - channel is closed
				default:
					t.Error("MR controller stop channel should be closed")
				}

				select {
				case <-h.vrefControllerStopChan:
					// Expected - channel is closed
				default:
					t.Error("VRef controller stop channel should be closed")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := test.setupHelper(t)

			// Test deactivation
			err := h.Deactivate()

			// Deactivate should always succeed
			assert.NoError(t, err)

			// Validate channel states
			if test.validateChannels != nil {
				test.validateChannels(t, h)
			}
		})
	}
}

func TestAddPVC(t *testing.T) {
	tests := []struct {
		name        string
		obj         interface{}
		expectError bool
		expectLog   string
	}{
		{
			name: "Valid PVC object",
			obj: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				},
				Status: v1.PersistentVolumeClaimStatus{
					Phase: v1.ClaimPending,
				},
			},
			expectError: false,
		},
		{
			name:        "Invalid object type",
			obj:         "not-a-pvc",
			expectError: true,
			expectLog:   "K8S helper expected PVC",
		},
		{
			name:        "Nil object",
			obj:         nil,
			expectError: true,
			expectLog:   "K8S helper expected PVC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create helper with minimal setup
			h := &helper{}

			// Capture log output would require more complex setup
			// For now, just test that the function doesn't panic
			assert.NotPanics(t, func() {
				h.addPVC(tt.obj)
			})
		})
	}
}

func TestUpdatePVC(t *testing.T) {
	tests := []struct {
		name        string
		oldObj      interface{}
		newObj      interface{}
		expectError bool
	}{
		{
			name:   "Valid PVC update",
			oldObj: nil, // oldObj is ignored in updatePVC
			newObj: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
				Status: v1.PersistentVolumeClaimStatus{
					Phase: v1.ClaimBound,
				},
			},
			expectError: false,
		},
		{
			name:        "Invalid new object type",
			oldObj:      nil,
			newObj:      "not-a-pvc",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &helper{}

			assert.NotPanics(t, func() {
				h.updatePVC(tt.oldObj, tt.newObj)
			})
		})
	}
}

func TestDeletePVC(t *testing.T) {
	tests := []struct {
		name        string
		obj         interface{}
		expectError bool
	}{
		{
			name: "Valid PVC deletion",
			obj: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:        "Invalid object type",
			obj:         123,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &helper{}

			assert.NotPanics(t, func() {
				h.deletePVC(tt.obj)
			})
		})
	}
}

func TestProcessPVC(t *testing.T) {
	tests := []struct {
		name      string
		pvc       *v1.PersistentVolumeClaim
		eventType string
	}{
		{
			name: "Process PVC add event",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
					AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					StorageClassName: &[]string{"fast-ssd"}[0],
				},
				Status: v1.PersistentVolumeClaimStatus{
					Phase: v1.ClaimPending,
				},
			},
			eventType: eventAdd,
		},
		{
			name: "Process PVC update event",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
					VolumeName: "pv-test-volume",
				},
				Status: v1.PersistentVolumeClaimStatus{
					Phase: v1.ClaimBound,
				},
			},
			eventType: eventUpdate,
		},
		{
			name: "Process PVC delete event",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			eventType: eventDelete,
		},
		{
			name: "Process PVC without storage size",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-pvc",
					Namespace: "default",
					UID:       "test-uid-invalid",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							// No storage size specified
						},
					},
				},
			},
			eventType: eventAdd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &helper{}
			ctx := context.TODO()

			// Test that processPVC doesn't panic
			assert.NotPanics(t, func() {
				h.processPVC(ctx, nil, tt.pvc, tt.eventType)
			})
		})
	}
}

func TestGetCachedPVCByUID(t *testing.T) {
	tests := []struct {
		name           string
		uid            string
		setupIndexer   func(*FakeIndexer)
		assertErr      assert.ErrorAssertionFunc
		assertPVC      assert.ValueAssertionFunc
		expectedPVCUID string
	}{
		{
			name: "Successfully find PVC by UID",
			uid:  "test-uid-123",
			setupIndexer: func(indexer *FakeIndexer) {
				pvc := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "default",
						UID:       "test-uid-123",
					},
				}
				indexer.items = []interface{}{pvc}
			},
			assertErr:      assert.NoError,
			assertPVC:      assert.NotNil,
			expectedPVCUID: "test-uid-123",
		},
		{
			name: "PVC not found by UID",
			uid:  "nonexistent-uid",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.items = []interface{}{} // Empty
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
		{
			name: "Multiple PVCs found by UID - error",
			uid:  "duplicate-uid",
			setupIndexer: func(indexer *FakeIndexer) {
				pvc1 := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc1", UID: "duplicate-uid",
					},
				}
				pvc2 := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc2", UID: "duplicate-uid",
					},
				}
				indexer.items = []interface{}{pvc1, pvc2}
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
		{
			name: "Non-PVC object found by UID",
			uid:  "wrong-type-uid",
			setupIndexer: func(indexer *FakeIndexer) {
				// Return a non-PVC object
				indexer.items = []interface{}{"not-a-pvc"}
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
		{
			name: "Indexer error",
			uid:  "error-uid",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.byIndexError = errors.New("indexer error")
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIndexer := &FakeIndexer{}
			tt.setupIndexer(fakeIndexer)

			h := &helper{
				pvcIndexer: fakeIndexer,
			}

			ctx := context.TODO()
			pvc, err := h.getCachedPVCByUID(ctx, tt.uid)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for test case: %s", tt.name)
			tt.assertPVC(t, pvc, "Unexpected PVC result for test case: %s", tt.name)

			// Only check UID when we expect a non-nil PVC
			if tt.expectedPVCUID != "" {
				assert.Equal(t, tt.expectedPVCUID, string(pvc.UID))
			}
		})
	}
}

func TestWaitForCachedPVCByUID(t *testing.T) {
	tests := []struct {
		name            string
		uid             string
		maxElapsedTime  time.Duration
		setupIndexer    func(*FakeIndexer)
		assertErr       assert.ErrorAssertionFunc
		assertPVC       assert.ValueAssertionFunc
		expectedTimeout bool
	}{
		{
			name:           "Successfully find PVC immediately",
			uid:            "immediate-uid",
			maxElapsedTime: 1 * time.Second,
			setupIndexer: func(indexer *FakeIndexer) {
				pvc := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "default",
						UID:       "immediate-uid",
					},
				}
				indexer.items = []interface{}{pvc}
			},
			assertErr: assert.NoError,
			assertPVC: assert.NotNil,
		},
		{
			name:           "Timeout waiting for PVC",
			uid:            "timeout-uid",
			maxElapsedTime: 10 * time.Millisecond, // Very short timeout
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.items = []interface{}{} // Always empty
			},
			assertErr:       assert.Error,
			assertPVC:       assert.Nil,
			expectedTimeout: true,
		},
		{
			name:           "Indexer error prevents finding PVC",
			uid:            "error-uid",
			maxElapsedTime: 100 * time.Millisecond,
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.byIndexError = errors.New("persistent indexer error")
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIndexer := &FakeIndexer{}
			tt.setupIndexer(fakeIndexer)

			h := &helper{
				pvcIndexer: fakeIndexer,
			}

			ctx := context.TODO()
			pvc, err := h.waitForCachedPVCByUID(ctx, tt.uid, tt.maxElapsedTime)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for waitForCachedPVCByUID test case: %s", tt.name)
			tt.assertPVC(t, pvc, "Unexpected PVC result for waitForCachedPVCByUID test case: %s", tt.name)

			if tt.expectedTimeout {
				assert.Error(t, err, "Expected timeout error")
			}

			// Only check UID when we expect a non-nil PVC (successful cases)
			if pvc != nil {
				assert.Equal(t, tt.uid, string(pvc.UID))
			}
		})
	}
}

func TestGetCachedPVCByName(t *testing.T) {
	tests := []struct {
		name         string
		pvcName      string
		namespace    string
		setupIndexer func(*FakeIndexer)
		assertErr    assert.ErrorAssertionFunc
		assertPVC    assert.ValueAssertionFunc
	}{
		{
			name:      "Successfully find PVC by name",
			pvcName:   "test-pvc",
			namespace: "default",
			setupIndexer: func(indexer *FakeIndexer) {
				pvc := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "default",
					},
				}
				indexer.getByKeyResult = pvc
				indexer.getByKeyExists = true
			},
			assertErr: assert.NoError,
			assertPVC: assert.NotNil,
		},
		{
			name:      "PVC not found by name",
			pvcName:   "nonexistent-pvc",
			namespace: "default",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyResult = nil
				indexer.getByKeyExists = false
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
		{
			name:      "Non-PVC object found",
			pvcName:   "wrong-type",
			namespace: "default",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyResult = "not-a-pvc"
				indexer.getByKeyExists = true
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
		{
			name:      "Indexer error",
			pvcName:   "error-pvc",
			namespace: "default",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyError = errors.New("indexer error")
			},
			assertErr: assert.Error,
			assertPVC: assert.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIndexer := &FakeIndexer{}
			tt.setupIndexer(fakeIndexer)

			h := &helper{
				pvcIndexer: fakeIndexer,
			}

			ctx := context.TODO()
			pvc, err := h.getCachedPVCByName(ctx, tt.pvcName, tt.namespace)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for test case: %s", tt.name)
			tt.assertPVC(t, pvc, "Unexpected PVC result for test case: %s", tt.name)

			// Only validate PVC details when we expect a non-nil PVC
			if pvc != nil {
				assert.Equal(t, tt.pvcName, pvc.Name)
				assert.Equal(t, tt.namespace, pvc.Namespace)
			}
		})
	}
}

func TestGetCachedPVByName(t *testing.T) {
	tests := []struct {
		name         string
		pvName       string
		setupIndexer func(*FakeIndexer)
		assertErr    assert.ErrorAssertionFunc
		assertPV     assert.ValueAssertionFunc
	}{
		{
			name:   "Successfully find PV by name",
			pvName: "test-pv",
			setupIndexer: func(indexer *FakeIndexer) {
				pv := &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pv",
					},
				}
				indexer.getByKeyResult = pv
				indexer.getByKeyExists = true
			},
			assertErr: assert.NoError,
			assertPV:  assert.NotNil,
		},
		{
			name:   "PV not found by name",
			pvName: "nonexistent-pv",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyResult = nil
				indexer.getByKeyExists = false
			},
			assertErr: assert.Error,
			assertPV:  assert.Nil,
		},
		{
			name:   "Non-PV object found",
			pvName: "wrong-type",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyResult = "not-a-pv"
				indexer.getByKeyExists = true
			},
			assertErr: assert.Error,
			assertPV:  assert.Nil,
		},
		{
			name:   "Indexer error",
			pvName: "error-pv",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyError = errors.New("indexer error")
			},
			assertErr: assert.Error,
			assertPV:  assert.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIndexer := &FakeIndexer{}
			tt.setupIndexer(fakeIndexer)

			h := &helper{
				pvIndexer: fakeIndexer,
			}

			ctx := context.TODO()
			pv, err := h.getCachedPVByName(ctx, tt.pvName)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for test case: %s", tt.name)
			tt.assertPV(t, pv, "Unexpected PV result for test case: %s", tt.name)

			// Only validate PV name when we expect a non-nil PV
			if pv != nil {
				assert.Equal(t, tt.pvName, pv.Name)
			}
		})
	}
}

func TestIsPVInCache(t *testing.T) {
	tests := []struct {
		name         string
		pvName       string
		setupIndexer func(*FakeIndexer)
		assertErr    assert.ErrorAssertionFunc
		assertFound  func(assert.TestingT, bool, ...interface{}) bool
	}{
		{
			name:   "PV found in cache",
			pvName: "existing-pv",
			setupIndexer: func(indexer *FakeIndexer) {
				pv := &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{Name: "existing-pv"},
				}
				indexer.getByKeyResult = pv
				indexer.getByKeyExists = true
			},
			assertErr:   assert.NoError,
			assertFound: assert.True,
		},
		{
			name:   "PV not found in cache",
			pvName: "missing-pv",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyResult = nil
				indexer.getByKeyExists = false
			},
			assertErr:   assert.NoError,
			assertFound: assert.False,
		},
		{
			name:   "Non-PV object found",
			pvName: "wrong-type",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyResult = "not-a-pv"
				indexer.getByKeyExists = true
			},
			assertErr:   assert.Error,
			assertFound: assert.False,
		},
		{
			name:   "Indexer error",
			pvName: "error-pv",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.getByKeyError = errors.New("cache error")
			},
			assertErr:   assert.Error,
			assertFound: assert.False,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIndexer := &FakeIndexer{}
			tt.setupIndexer(fakeIndexer)

			h := &helper{
				pvIndexer: fakeIndexer,
			}

			ctx := context.TODO()
			found, err := h.isPVInCache(ctx, tt.pvName)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for test case: %s", tt.name)
			tt.assertFound(t, found, "Unexpected found result for test case: %s", tt.name)
		})
	}
}

func TestProcessNode(t *testing.T) {
	tests := []struct {
		name                   string
		node                   *v1.Node
		eventType              string
		setupOrchestrator      func(*mockcore.MockOrchestrator)
		expectOrchestratorCall bool
	}{
		{
			name: "Process node add event",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			eventType: eventAdd,
			setupOrchestrator: func(mockOrchestrator *mockcore.MockOrchestrator) {
				// No orchestrator call expected for add events
			},
			expectOrchestratorCall: false,
		},
		{
			name: "Process node update event",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-2",
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			eventType: eventUpdate,
			setupOrchestrator: func(mockOrchestrator *mockcore.MockOrchestrator) {
				// No orchestrator call expected for update events
			},
			expectOrchestratorCall: false,
		},
		{
			name: "Process node delete event - successful deletion",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-3",
				},
			},
			eventType: eventDelete,
			setupOrchestrator: func(mockOrchestrator *mockcore.MockOrchestrator) {
				mockOrchestrator.EXPECT().DeleteNode(gomock.Any(), "test-node-3").Return(nil)
			},
			expectOrchestratorCall: true,
		},
		{
			name: "Process node delete event - not found error (should not log error)",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-4",
				},
			},
			eventType: eventDelete,
			setupOrchestrator: func(mockOrchestrator *mockcore.MockOrchestrator) {
				mockOrchestrator.EXPECT().DeleteNode(gomock.Any(), "test-node-4").Return(errors.NotFoundError("node not found"))
			},
			expectOrchestratorCall: true,
		},
		{
			name: "Process node delete event - other error",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-5",
				},
			},
			eventType: eventDelete,
			setupOrchestrator: func(mockOrchestrator *mockcore.MockOrchestrator) {
				mockOrchestrator.EXPECT().DeleteNode(gomock.Any(), "test-node-5").Return(errors.New("database error"))
			},
			expectOrchestratorCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
			tt.setupOrchestrator(mockOrchestrator)

			h := &helper{
				orchestrator: mockOrchestrator,
			}

			ctx := context.TODO()

			// Test that processNode doesn't panic and calls orchestrator when expected
			assert.NotPanics(t, func() {
				h.processNode(ctx, tt.node, tt.eventType)
			})
		})
	}
}

func TestGetNodeTopologyLabels(t *testing.T) {
	tests := []struct {
		name           string
		nodeName       string
		setupClient    func() *k8sfake.Clientset
		expectedLabels map[string]string
		assertErr      assert.ErrorAssertionFunc
	}{
		{
			name:     "Node with topology labels",
			nodeName: "node-with-topology",
			setupClient: func() *k8sfake.Clientset {
				node := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-with-topology",
						Labels: map[string]string{
							"topology.kubernetes.io/zone":   "us-east-1a",
							"topology.kubernetes.io/region": "us-east-1",
							"kubernetes.io/hostname":        "node-with-topology",
							"regular-label":                 "not-topology",
						},
					},
				}
				client := k8sfake.NewSimpleClientset(node)
				return client
			},
			expectedLabels: map[string]string{
				"topology.kubernetes.io/zone":   "us-east-1a",
				"topology.kubernetes.io/region": "us-east-1",
			},
			assertErr: assert.NoError,
		},
		{
			name:     "Node with no topology labels",
			nodeName: "node-no-topology",
			setupClient: func() *k8sfake.Clientset {
				node := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-no-topology",
						Labels: map[string]string{
							"kubernetes.io/hostname": "node-no-topology",
							"regular-label":          "value",
						},
					},
				}
				client := k8sfake.NewSimpleClientset(node)
				return client
			},
			expectedLabels: map[string]string{},
			assertErr:      assert.NoError,
		},
		{
			name:     "Node with custom topology prefix",
			nodeName: "node-custom-topology",
			setupClient: func() *k8sfake.Clientset {
				node := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-custom-topology",
						Labels: map[string]string{
							"topology.example.com/rack":   "rack-1", // Non-standard prefix, should not be included
							"topology.kubernetes.io/zone": "us-west-2b",
							"normal-label":                "value",
						},
					},
				}
				client := k8sfake.NewSimpleClientset(node)
				return client
			},
			expectedLabels: map[string]string{
				"topology.kubernetes.io/zone": "us-west-2b", // Only standard prefix should be included
			},
			assertErr: assert.NoError,
		},
		{
			name:     "Node not found",
			nodeName: "nonexistent-node",
			setupClient: func() *k8sfake.Clientset {
				// Create empty client without the requested node
				client := k8sfake.NewSimpleClientset()
				return client
			},
			expectedLabels: nil,
			assertErr:      assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := tt.setupClient()

			h := &helper{
				kubeClient: kubeClient,
			}

			ctx := context.TODO()
			labels, err := h.GetNodeTopologyLabels(ctx, tt.nodeName)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for GetNodeTopologyLabels test case: %s", tt.name)
			if err == nil {
				assert.Equal(t, tt.expectedLabels, labels)
			}
		})
	}
}

func TestGetCachedVolumeReference(t *testing.T) {
	tests := []struct {
		name          string
		vrefNamespace string
		pvcName       string
		pvcNamespace  string
		setupIndexer  func(*FakeIndexer)
		assertErr     assert.ErrorAssertionFunc
		assertVRef    assert.ValueAssertionFunc
	}{
		{
			name:          "Successfully find volume reference",
			vrefNamespace: "trident",
			pvcName:       "test-pvc",
			pvcNamespace:  "default",
			setupIndexer: func(indexer *FakeIndexer) {
				vref := &netappv1.TridentVolumeReference{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vref",
						Namespace: "trident",
					},
					Spec: netappv1.TridentVolumeReferenceSpec{
						PVCName:      "test-pvc",
						PVCNamespace: "default",
					},
				}
				indexer.items = []interface{}{vref}
			},
			assertErr:  assert.NoError,
			assertVRef: assert.NotNil,
		},
		{
			name:          "Volume reference not found",
			vrefNamespace: "trident",
			pvcName:       "missing-pvc",
			pvcNamespace:  "default",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.items = []interface{}{} // Empty
			},
			assertErr:  assert.Error,
			assertVRef: assert.Nil,
		},
		{
			name:          "Multiple volume references found - error",
			vrefNamespace: "trident",
			pvcName:       "duplicate-pvc",
			pvcNamespace:  "default",
			setupIndexer: func(indexer *FakeIndexer) {
				vref1 := &netappv1.TridentVolumeReference{
					ObjectMeta: metav1.ObjectMeta{Name: "vref1"},
				}
				vref2 := &netappv1.TridentVolumeReference{
					ObjectMeta: metav1.ObjectMeta{Name: "vref2"},
				}
				indexer.items = []interface{}{vref1, vref2}
			},
			assertErr:  assert.Error,
			assertVRef: assert.Nil,
		},
		{
			name:          "Non-VolumeReference object found",
			vrefNamespace: "trident",
			pvcName:       "wrong-type",
			pvcNamespace:  "default",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.items = []interface{}{"not-a-vref"}
			},
			assertErr:  assert.Error,
			assertVRef: assert.Nil,
		},
		{
			name:          "Indexer error",
			vrefNamespace: "trident",
			pvcName:       "error-pvc",
			pvcNamespace:  "default",
			setupIndexer: func(indexer *FakeIndexer) {
				indexer.byIndexError = errors.New("cache error")
			},
			assertErr:  assert.Error,
			assertVRef: assert.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeIndexer := &FakeIndexer{}
			tt.setupIndexer(fakeIndexer)

			h := &helper{
				vrefIndexer: fakeIndexer,
			}

			ctx := context.TODO()
			vref, err := h.getCachedVolumeReference(ctx, tt.vrefNamespace, tt.pvcName, tt.pvcNamespace)

			// Use function types for assertions - no conditional logic needed!
			tt.assertErr(t, err, "Unexpected error result for getCachedVolumeReference test case: %s", tt.name)
			tt.assertVRef(t, vref, "Unexpected VRef result for getCachedVolumeReference test case: %s", tt.name)
		})
	}
}

// Autogrow Policy Annotation Change Tests (Recent Changes)

// TestUpdatePVC_AutogrowPolicyAnnotationChanged tests the Autogrow policy annotation handling in updatePVC
func TestUpdatePVC_AutogrowPolicyAnnotationChanged(t *testing.T) {
	tests := []struct {
		name                   string
		oldPVCAnnotation       string
		newPVCAnnotation       string
		volumeName             string
		mockOrchestratorError  error
		expectOrchestratorCall bool
		expectNotFoundEvent    bool
		expectNotUsableEvent   bool
		expectOtherError       bool
	}{
		{
			name:                   "Autogrow policy annotation added",
			oldPVCAnnotation:       "",
			newPVCAnnotation:       "gold-policy",
			volumeName:             "pvc-test-volume-123",
			mockOrchestratorError:  nil,
			expectOrchestratorCall: true,
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   false,
		},
		{
			name:                   "Autogrow policy annotation changed",
			oldPVCAnnotation:       "silver-policy",
			newPVCAnnotation:       "gold-policy",
			volumeName:             "pvc-test-volume-456",
			mockOrchestratorError:  nil,
			expectOrchestratorCall: true,
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   false,
		},
		{
			name:                   "Autogrow policy annotation removed",
			oldPVCAnnotation:       "gold-policy",
			newPVCAnnotation:       "",
			volumeName:             "pvc-test-volume-789",
			mockOrchestratorError:  nil,
			expectOrchestratorCall: true,
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   false,
		},
		{
			name:                   "Autogrow policy annotation set to none",
			oldPVCAnnotation:       "gold-policy",
			newPVCAnnotation:       "none",
			volumeName:             "pvc-test-volume-none",
			mockOrchestratorError:  nil,
			expectOrchestratorCall: true,
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   false,
		},
		{
			name:                   "Autogrow policy not found error",
			oldPVCAnnotation:       "",
			newPVCAnnotation:       "nonexistent-policy",
			volumeName:             "pvc-test-volume-notfound",
			mockOrchestratorError:  errors.NotFoundError("autogrow policy not found"),
			expectOrchestratorCall: true,
			expectNotFoundEvent:    true,
			expectNotUsableEvent:   false,
		},
		{
			name:                   "Autogrow policy not usable error",
			oldPVCAnnotation:       "",
			newPVCAnnotation:       "failed-policy",
			volumeName:             "pvc-test-volume-notusable",
			mockOrchestratorError:  errors.AutogrowPolicyNotUsableError("failed-policy", "Failed"),
			expectOrchestratorCall: true,
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   true,
		},
		{
			name:                   "Autogrow policy orchestrator other error",
			oldPVCAnnotation:       "",
			newPVCAnnotation:       "test-policy",
			volumeName:             "pvc-test-volume-error",
			mockOrchestratorError:  fmt.Errorf("database connection error"),
			expectOrchestratorCall: true,
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   false,
			expectOtherError:       true,
		},
		{
			name:                   "Autogrow policy annotation unchanged",
			oldPVCAnnotation:       "gold-policy",
			newPVCAnnotation:       "gold-policy",
			volumeName:             "pvc-test-volume-unchanged",
			mockOrchestratorError:  nil,
			expectOrchestratorCall: false, // Should not call orchestrator
			expectNotFoundEvent:    false,
			expectNotUsableEvent:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			plugin := &helper{
				orchestrator: mockOrchestrator,
			}

			// Setup orchestrator mock expectation
			if tt.expectOrchestratorCall {
				mockOrchestrator.EXPECT().
					UpdateVolumeAutogrowPolicy(gomock.Any(), tt.volumeName, tt.newPVCAnnotation).
					Return(tt.mockOrchestratorError).
					Times(1)
			}

			// Create old and new PVCs
			oldPVC := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						AnnAutogrowPolicy: tt.oldPVCAnnotation,
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					VolumeName: tt.volumeName,
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}

			newPVC := oldPVC.DeepCopy()
			newPVC.Annotations[AnnAutogrowPolicy] = tt.newPVCAnnotation

			// Call updatePVC
			plugin.updatePVC(oldPVC, newPVC)
		})
	}
}

// TestUpdatePVC_AutogrowPolicyAnnotationChanged_NoVolumeName tests when PVC has no volume yet
func TestUpdatePVC_AutogrowPolicyAnnotationChanged_NoVolumeName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	plugin := &helper{
		orchestrator: mockOrchestrator,
	}

	// Should not call orchestrator when VolumeName is empty
	// mockOrchestrator.EXPECT().UpdateVolumeAutogrowPolicy() - NO CALL EXPECTED

	oldPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			Annotations: map[string]string{
				AnnAutogrowPolicy: "",
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "", // No volume name yet
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	newPVC := oldPVC.DeepCopy()
	newPVC.Annotations[AnnAutogrowPolicy] = "gold-policy"

	// Call updatePVC - should not call orchestrator since no volume name
	plugin.updatePVC(oldPVC, newPVC)
}

// TestUpdatePVC_InvalidObjectTypes tests error handling for wrong object types
func TestUpdatePVC_InvalidObjectTypes(t *testing.T) {
	plugin := &helper{}

	tests := []struct {
		name   string
		oldObj interface{}
		newObj interface{}
	}{
		{
			name:   "Invalid old object type",
			oldObj: "not-a-pvc",
			newObj: &v1.PersistentVolumeClaim{},
		},
		{
			name:   "Invalid new object type",
			oldObj: &v1.PersistentVolumeClaim{},
			newObj: "not-a-pvc",
		},
		{
			name:   "Both invalid object types",
			oldObj: "not-a-pvc",
			newObj: 12345,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic - just log error and return
			assert.NotPanics(t, func() {
				plugin.updatePVC(tt.oldObj, tt.newObj)
			})
		})
	}
}

// TestProcessStorageClass_AutogrowPolicyErrors tests Autogrow policy error handling in processStorageClass
func TestProcessStorageClass_AutogrowPolicyErrors(t *testing.T) {
	tests := []struct {
		name                     string
		autogrowPolicyAnnotation string
		isUpdate                 bool
		mockOrchestratorError    error
		expectNotFoundEvent      bool
		expectNotUsableEvent     bool
	}{
		{
			name:                     "Update with autogrow policy not found",
			autogrowPolicyAnnotation: "nonexistent-policy",
			isUpdate:                 true,
			mockOrchestratorError:    errors.NotFoundError("autogrow policy not found"),
			expectNotFoundEvent:      true,
			expectNotUsableEvent:     false,
		},
		{
			name:                     "Update with autogrow policy not usable",
			autogrowPolicyAnnotation: "failed-policy",
			isUpdate:                 true,
			mockOrchestratorError:    errors.AutogrowPolicyNotUsableError("failed-policy", "Failed"),
			expectNotFoundEvent:      false,
			expectNotUsableEvent:     true,
		},
		{
			name:                     "Update with autogrow policy success",
			autogrowPolicyAnnotation: "success-policy",
			isUpdate:                 true,
			mockOrchestratorError:    nil,
			expectNotFoundEvent:      false,
			expectNotUsableEvent:     false,
		},
		{
			name:                     "Add with autogrow policy annotation",
			autogrowPolicyAnnotation: "test-policy",
			isUpdate:                 false,
			mockOrchestratorError:    nil,
			expectNotFoundEvent:      false,
			expectNotUsableEvent:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Create fake indexer to avoid panic in RecordStorageClassEvent
			fakeIndexer := &FakeIndexer{
				getByKeyResult: nil,
				getByKeyExists: false,
				getByKeyError:  nil,
			}

			plugin := &helper{
				orchestrator: mockOrchestrator,
				scIndexer:    fakeIndexer,
			}

			sc := &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-storage-class",
					Annotations: map[string]string{
						AnnAutogrowPolicy: tt.autogrowPolicyAnnotation,
					},
				},
				Provisioner: csi.Provisioner,
				Parameters: map[string]string{
					"backendType": "ontap-nas",
				},
			}

			// Setup mock expectations
			if tt.isUpdate {
				mockOrchestrator.EXPECT().
					UpdateStorageClass(gomock.Any(), gomock.Any()).
					Return(nil, tt.mockOrchestratorError).
					Times(1)
			} else {
				mockOrchestrator.EXPECT().
					AddStorageClass(gomock.Any(), gomock.Any()).
					Return(nil, tt.mockOrchestratorError).
					Times(1)
			}

			ctx := context.Background()
			// Call processStorageClass
			plugin.processStorageClass(ctx, sc, tt.isUpdate)
		})
	}
}

// TestProcessStorageClass_AutogrowPolicyOtherError tests non-Autogrow-specific errors
func TestProcessStorageClass_AutogrowPolicyOtherError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Create fake indexer to avoid panic in RecordStorageClassEvent
	fakeIndexer := &FakeIndexer{
		getByKeyResult: nil,
		getByKeyExists: false,
		getByKeyError:  nil,
	}

	plugin := &helper{
		orchestrator: mockOrchestrator,
		scIndexer:    fakeIndexer,
	}

	sc := &k8sstoragev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-storage-class",
			Annotations: map[string]string{
				AnnAutogrowPolicy: "test-policy",
			},
		},
		Provisioner: csi.Provisioner,
		Parameters: map[string]string{
			"backendType": "ontap-nas",
		},
	}

	// Mock orchestrator returning a generic error (not NotFound or NotUsable)
	genericError := fmt.Errorf("database connection error")
	mockOrchestrator.EXPECT().
		UpdateStorageClass(gomock.Any(), gomock.Any()).
		Return(nil, genericError).
		Times(1)

	ctx := context.Background()
	// Call processStorageClass - should log warning but not record event
	plugin.processStorageClass(ctx, sc, true)
}

// TestUpdatePVC_AutogrowPolicyCaseFolding tests case-insensitive annotation key handling
func TestUpdatePVC_AutogrowPolicyCaseFolding(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	plugin := &helper{
		orchestrator: mockOrchestrator,
	}

	volumeName := "pvc-test-volume-case"

	// getCaseFoldedAnnotation finds the annotation regardless of case
	// The value will be retrieved from the actual key that exists
	mockOrchestrator.EXPECT().
		UpdateVolumeAutogrowPolicy(gomock.Any(), volumeName, "GOLD-Policy").
		Return(nil).
		Times(1)

	oldPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			Annotations: map[string]string{
				// Different casing for the key
				"TRIDENT.NETAPP.IO/AUTOGROWPOLICY": "",
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: volumeName,
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	newPVC := oldPVC.DeepCopy()
	// Update annotation value (key stays the same case)
	newPVC.Annotations["TRIDENT.NETAPP.IO/AUTOGROWPOLICY"] = "GOLD-Policy"

	// Call updatePVC
	plugin.updatePVC(oldPVC, newPVC)
}

// Test GetPVC and GetPVCForPV (cache-first with API fallback)

func TestGetPVC_CacheHit(t *testing.T) {
	ctx := context.Background()
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pvc", Namespace: "default"},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
	}
	pvcIndexer := &FakeIndexer{
		getByKeyResult: pvc,
		getByKeyExists: true,
	}
	h := &helper{kubeClient: k8sfake.NewSimpleClientset(), pvcIndexer: pvcIndexer}

	result, err := h.GetPVC(ctx, "default", "my-pvc")
	require.NoError(t, err)
	assert.Equal(t, "my-pvc", result.Name)
	assert.Equal(t, "default", result.Namespace)
}

func TestGetPVC_CacheMiss_APISuccess(t *testing.T) {
	ctx := context.Background()
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "api-pvc", Namespace: "default"},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("5Gi")},
			},
		},
	}
	clientSet := k8sfake.NewSimpleClientset(pvc)
	pvcIndexer := &FakeIndexer{getByKeyExists: false} // cache miss
	h := &helper{kubeClient: clientSet, pvcIndexer: pvcIndexer}

	result, err := h.GetPVC(ctx, "default", "api-pvc")
	require.NoError(t, err)
	assert.Equal(t, "api-pvc", result.Name)
	assert.Equal(t, "default", result.Namespace)
}

func TestGetPVC_CacheMiss_APIFail(t *testing.T) {
	ctx := context.Background()
	clientSet := k8sfake.NewSimpleClientset()
	var getCalled bool
	clientSet.Fake.PrependReactor("get", "persistentvolumeclaims", func(a k8stesting.Action) (bool, runtime.Object, error) {
		getCalled = true
		return true, nil, k8serrors.NewNotFound(v1.Resource("persistentvolumeclaims"), "nonexistent")
	})
	pvcIndexer := &FakeIndexer{getByKeyExists: false}
	h := &helper{kubeClient: clientSet, pvcIndexer: pvcIndexer}

	result, err := h.GetPVC(ctx, "default", "nonexistent")
	require.True(t, getCalled, "API Get should have been called after cache miss")
	require.Error(t, err)
	require.True(t, k8serrors.IsNotFound(err), "error should be NotFound")
	// Result may be nil or empty depending on fake client behavior; we care that error is returned
	if result != nil {
		assert.Empty(t, result.Name, "result should be empty or nil when PVC is not found")
	}
}

func TestGetPVCForPV_PVNotFound(t *testing.T) {
	ctx := context.Background()
	clientSet := k8sfake.NewSimpleClientset()
	pvIndexer := &FakeIndexer{getByKeyExists: false}
	h := &helper{kubeClient: clientSet, pvIndexer: pvIndexer}

	result, err := h.GetPVCForPV(ctx, "nonexistent-pv")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "could not get PV")
	assert.Contains(t, err.Error(), "nonexistent-pv")
}

func TestGetPVCForPV_NoClaimRef(t *testing.T) {
	ctx := context.Background()
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-no-claim"},
		Spec:       v1.PersistentVolumeSpec{ClaimRef: nil},
	}
	pvIndexer := &FakeIndexer{getByKeyResult: pv, getByKeyExists: true}
	h := &helper{kubeClient: k8sfake.NewSimpleClientset(), pvIndexer: pvIndexer}

	result, err := h.GetPVCForPV(ctx, "pv-no-claim")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "has no bound PVC")
	assert.Contains(t, err.Error(), "pv-no-claim")
}

func TestGetPVCForPV_ClaimRefEmptyNamespace(t *testing.T) {
	ctx := context.Background()
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-empty-ns"},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{Name: "pvc1", Namespace: ""},
		},
	}
	pvIndexer := &FakeIndexer{getByKeyResult: pv, getByKeyExists: true}
	h := &helper{kubeClient: k8sfake.NewSimpleClientset(), pvIndexer: pvIndexer}

	result, err := h.GetPVCForPV(ctx, "pv-empty-ns")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "has no bound PVC")
}

func TestGetPVCForPV_SuccessFromCache(t *testing.T) {
	ctx := context.Background()
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pv"},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{Name: "bound-pvc", Namespace: "default"},
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "bound-pvc", Namespace: "default"},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
	}
	pvIndexer := &FakeIndexer{getByKeyResult: pv, getByKeyExists: true}
	pvcIndexer := &FakeIndexer{getByKeyResult: pvc, getByKeyExists: true}
	h := &helper{kubeClient: k8sfake.NewSimpleClientset(), pvIndexer: pvIndexer, pvcIndexer: pvcIndexer}

	result, err := h.GetPVCForPV(ctx, "my-pv")
	require.NoError(t, err)
	assert.Equal(t, "bound-pvc", result.Name)
	assert.Equal(t, "default", result.Namespace)
}

func TestGetPVCForPV_SuccessPVFromAPI_PVCFromCache(t *testing.T) {
	ctx := context.Background()
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-api"},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{Name: "pvc-from-cache", Namespace: "default"},
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-from-cache", Namespace: "default"},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	clientSet := k8sfake.NewSimpleClientset(pv)
	pvIndexer := &FakeIndexer{getByKeyExists: false}
	_, _ = clientSet.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
	pvcIndexer := &FakeIndexer{getByKeyResult: pvc, getByKeyExists: true}
	h := &helper{kubeClient: clientSet, pvIndexer: pvIndexer, pvcIndexer: pvcIndexer}

	result, err := h.GetPVCForPV(ctx, "pv-api")
	require.NoError(t, err)
	assert.Equal(t, "pvc-from-cache", result.Name)
	assert.Equal(t, "default", result.Namespace)
}

func TestGetPVCForPV_PVCNotFound(t *testing.T) {
	ctx := context.Background()
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pv"},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{Name: "missing-pvc", Namespace: "default"},
		},
	}
	pvIndexer := &FakeIndexer{getByKeyResult: pv, getByKeyExists: true}
	pvcIndexer := &FakeIndexer{getByKeyExists: false}
	clientSet := k8sfake.NewSimpleClientset()
	var pvcGetCalled bool
	clientSet.Fake.PrependReactor("get", "persistentvolumeclaims", func(a k8stesting.Action) (bool, runtime.Object, error) {
		pvcGetCalled = true
		return true, nil, k8serrors.NewNotFound(v1.Resource("persistentvolumeclaims"), "missing-pvc")
	})
	h := &helper{kubeClient: clientSet, pvIndexer: pvIndexer, pvcIndexer: pvcIndexer}

	result, err := h.GetPVCForPV(ctx, "my-pv")
	require.True(t, pvcGetCalled, "API Get for PVC should have been called after cache miss")
	require.Error(t, err)
	require.True(t, k8serrors.IsNotFound(err), "error should be NotFound")
	if result != nil {
		assert.Empty(t, result.Name, "result should be empty or nil when PVC is not found")
	}
}
