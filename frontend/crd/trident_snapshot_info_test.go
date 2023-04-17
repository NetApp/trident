// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

func TestUpdateTSI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.crdInformerFactory, "crdController.crdInformerFactory is nil")

	statusCondition1 := &netappv1.TridentSnapshotInfoStatus{SnapshotHandle: "volume/foo"}
	statusCondition2 := &netappv1.TridentSnapshotInfoStatus{SnapshotHandle: "volume/bar"}
	snapshotInfo := &netappv1.TridentSnapshotInfo{}
	snapshotInfo.Status = *statusCondition1

	snapshotInfo, err = crdClient.TridentV1().TridentSnapshotInfos(tridentNamespace).Create(ctx(), snapshotInfo,
		metav1.CreateOptions{})
	assert.Nil(t, err)
	defer crdClient.TridentV1().TridentSnapshotInfos(tridentNamespace).Delete(ctx(), snapshotInfo.Name,
		metav1.DeleteOptions{})

	updatedSnapshotInfo, err := crdController.updateTSIStatus(ctx(), snapshotInfo, statusCondition2)
	assert.Nil(t, err)
	assert.EqualValues(t, *statusCondition2, updatedSnapshotInfo.Status)
}
