// Copyright 2021 NetApp, Inc. All Rights Reserved.

package crd

import (
	"testing"

	"github.com/netapp/trident/core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateTSI(t *testing.T) {
	testingCache := NewTestingCache()
	orchestrator := core.NewMockOrchestrator()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	addCrdTestReactors(crdClient, testingCache)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assertNotNil(t, "kubeClient", kubeClient)
	assertNotNil(t, "crdClient", crdClient)
	assertNotNil(t, "crdController", crdController)
	assertNotNil(t, "crdController.crdInformerFactory", crdController.crdInformerFactory)

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
