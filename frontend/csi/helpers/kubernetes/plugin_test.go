package kubernetes

import (
	"testing"

	"github.com/golang/mock/gomock"
	storageattribute "github.com/netapp/trident/storage_attribute"
	k8sstoragev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi"
	mockcore "github.com/netapp/trident/mocks/mock_core"
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

	backendTypeAttr, _ := storageattribute.CreateAttributeRequestFromAttributeValue("backendType", "ontap-nas")

	expectedSCConfig := &storageclass.Config{
		Name: FakeStorageClass,
		Attributes: map[string]storageattribute.Request{
			"backendType": backendTypeAttr,
		},
	}

	mockCore.EXPECT().AddStorageClass(gomock.Any(), expectedSCConfig).Return(nil, nil).Times(1)

	plugin.addStorageClass(sc)
}
