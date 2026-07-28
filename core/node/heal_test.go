// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

// This file's tests must not use t.Parallel(); see core_test.go for why.

// resetISCSISessionGlobals resets the package-level published/current iSCSI session maps to
// fresh, empty instances, both immediately and via t.Cleanup, so tests that seed session state
// don't leak into sibling tests (in this file or others in the package).
func resetISCSISessionGlobals(t *testing.T) {
	t.Helper()
	publishedISCSISessions = models.NewISCSISessions()
	currentISCSISessions = models.NewISCSISessions()
	t.Cleanup(func() {
		publishedISCSISessions = models.NewISCSISessions()
		currentISCSISessions = models.NewISCSISessions()
	})
}

// resetNVMeSessionGlobals is the NVMe equivalent of resetISCSISessionGlobals.
func resetNVMeSessionGlobals(t *testing.T) {
	t.Helper()
	publishedNVMeSessions = nvme.NVMeSessions{}
	currentNVMeSessions = nvme.NVMeSessions{}
	t.Cleanup(func() {
		publishedNVMeSessions = nvme.NVMeSessions{}
		currentNVMeSessions = nvme.NVMeSessions{}
	})
}

// seedPublishedISCSISessionForPortal resets publishedISCSISessions to a fresh map containing a
// single portal/LUN/volume mapping, for tests that exercise selfHealingRectifySession.
func seedPublishedISCSISessionForPortal(t *testing.T, portal, iqn string, lun int32, volID string) {
	t.Helper()
	resetISCSISessionGlobals(t)
	require.NoError(t, publishedISCSISessions.AddPortal(portal, models.PortalInfo{ISCSITargetIQN: iqn}))
	require.NoError(t, publishedISCSISessions.AddLUNToPortal(portal, models.LUNData{LUN: lun, VolID: volID}))
}

func TestPopulatePublishedSessions_NoTrackingFiles(t *testing.T) {
	core, _ := newTestCore(t)
	resetISCSISessionGlobals(t)
	resetNVMeSessionGlobals(t)

	// GetAllVolumeIDs reads tridentDeviceInfoPath ("/var/lib/trident/tracking") from disk. That
	// directory does not exist in this sandbox, so no volume IDs are discovered and no
	// localStore/iscsi/nvme calls should occur; the "tracking info present" branches of
	// populatePublishedSessions are therefore not exercisable here (see final summary).
	assert.NotPanics(t, func() { core.populatePublishedSessions(context.Background()) })
}

func TestStartStopISCSISelfHealingThread_Disabled(t *testing.T) {
	core, _ := newTestCore(t, WithISCSISelfHealingInterval(0))

	core.startISCSISelfHealingThread(context.Background())
	assert.Nil(t, core.iSCSISelfHealingTicker)
	assert.Nil(t, core.iSCSISelfHealingChannel)

	assert.NotPanics(t, func() { core.stopISCSISelfHealingThread(context.Background()) })
}

func TestStartStopISCSISelfHealingThread_Enabled(t *testing.T) {
	core, _ := newTestCore(t, WithISCSISelfHealingInterval(1*time.Second))
	resetISCSISessionGlobals(t)

	core.startISCSISelfHealingThread(context.Background())
	require.NotNil(t, core.iSCSISelfHealingTicker)
	require.NotNil(t, core.iSCSISelfHealingChannel)

	// Stop immediately, well before the 1s tick fires, so no self-healing sweep (and thus no
	// mock calls) is triggered by the background goroutine.
	assert.NotPanics(t, func() { core.stopISCSISelfHealingThread(context.Background()) })
}

func TestStartStopNVMeSelfHealingThread_Disabled(t *testing.T) {
	core, _ := newTestCore(t, WithNVMeSelfHealingInterval(0))

	core.startNVMeSelfHealingThread(context.Background())
	assert.Nil(t, core.nvmeSelfHealingTicker)
	assert.Nil(t, core.nvmeSelfHealingChannel)

	assert.NotPanics(t, func() { core.stopNVMeSelfHealingThread(context.Background()) })
}

func TestStartStopNVMeSelfHealingThread_Enabled(t *testing.T) {
	core, _ := newTestCore(t, WithNVMeSelfHealingInterval(1*time.Second))
	resetNVMeSessionGlobals(t)

	core.startNVMeSelfHealingThread(context.Background())
	require.NotNil(t, core.nvmeSelfHealingTicker)
	require.NotNil(t, core.nvmeSelfHealingChannel)

	assert.NotPanics(t, func() { core.stopNVMeSelfHealingThread(context.Background()) })
}

func TestPerformISCSISelfHealing_EmptyPublishedSessions(t *testing.T) {
	core, _ := newTestCore(t)
	resetISCSISessionGlobals(t)

	// No mocks are set up: publishedISCSISessions.IsEmpty() must short-circuit before any
	// iscsi.PreChecks/PopulateCurrentSessions call, otherwise gomock's strict controller fails
	// the test for an unexpected call.
	core.performISCSISelfHealing(context.Background())
}

func TestPerformISCSISelfHealing_NonEmpty_CallsExpectedSequence(t *testing.T) {
	core, mocks := newTestCore(t)
	resetISCSISessionGlobals(t)
	require.NoError(t, publishedISCSISessions.AddPortal("192.0.2.20:3260", models.PortalInfo{ISCSITargetIQN: "iqn.test"}))

	mocks.ISCSI.EXPECT().PreChecks(gomock.Any()).Return(nil)
	mocks.ISCSI.EXPECT().PopulateCurrentSessions(gomock.Any(), gomock.Any()).Return(nil)
	mocks.ISCSI.EXPECT().InspectAllISCSISessions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]string{}, []string{})

	core.performISCSISelfHealing(context.Background())
}

func TestPerformISCSISelfHealing_PopulateCurrentSessionsError_ReturnsEarly(t *testing.T) {
	core, mocks := newTestCore(t)
	resetISCSISessionGlobals(t)
	require.NoError(t, publishedISCSISessions.AddPortal("192.0.2.21:3260", models.PortalInfo{ISCSITargetIQN: "iqn.test"}))

	mocks.ISCSI.EXPECT().PreChecks(gomock.Any()).Return(nil)
	mocks.ISCSI.EXPECT().PopulateCurrentSessions(gomock.Any(), gomock.Any()).Return(errors.New("populate boom"))
	// No InspectAllISCSISessions expectation: the strict mock fails the test if it is called.

	core.performISCSISelfHealing(context.Background())
}

func TestFixISCSISessions_NoPortals(t *testing.T) {
	core, _ := newTestCore(t)
	assert.NotPanics(t, func() {
		core.fixISCSISessions(context.Background(), nil, "stale", time.Now().Add(time.Second))
	})
}

func TestSelfHealingRectifySession_PortalNotFound(t *testing.T) {
	core, _ := newTestCore(t)
	resetISCSISessionGlobals(t)

	err := core.selfHealingRectifySession(context.Background(), "10.0.0.1:3260", models.Scan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get publish info for session on portal")
}

func TestSelfHealingRectifySession_LogoutLoginScan_NotAccessible(t *testing.T) {
	core, mocks := newTestCore(t)
	portal := "192.0.2.10:3260"
	seedPublishedISCSISessionForPortal(t, portal, "iqn.test", 0, "vol1")

	mocks.ISCSI.EXPECT().IsPortalAccessible(gomock.Any(), portal).Return(false, errors.New("unreachable"))
	// No Logout expectation: it must not be called when the portal is inaccessible.

	err := core.selfHealingRectifySession(context.Background(), portal, models.LogoutLoginScan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot safely log out")
}

func TestSelfHealingRectifySession_LogoutLoginScan_LogoutFails(t *testing.T) {
	core, mocks := newTestCore(t)
	portal := "192.0.2.14:3260"
	seedPublishedISCSISessionForPortal(t, portal, "iqn.test", 0, "vol1")

	mocks.ISCSI.EXPECT().IsPortalAccessible(gomock.Any(), portal).Return(true, nil)
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), "iqn.test", portal).Return(errors.New("logout boom"))

	err := core.selfHealingRectifySession(context.Background(), portal, models.LogoutLoginScan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error while logging out of target")
}

func TestSelfHealingRectifySession_LogoutLoginScan_AccessibleThenLoginFails(t *testing.T) {
	core, mocks := newTestCore(t)
	portal := "192.0.2.15:3260"
	seedPublishedISCSISessionForPortal(t, portal, "iqn.test", 0, "vol1")

	mocks.ISCSI.EXPECT().IsPortalAccessible(gomock.Any(), portal).Return(true, nil)
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), "iqn.test", portal).Return(nil)
	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("login boom"))

	err := core.selfHealingRectifySession(context.Background(), portal, models.LogoutLoginScan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to login to the target")
}

func TestSelfHealingRectifySession_LoginScan_AttachFails(t *testing.T) {
	core, mocks := newTestCore(t)
	portal := "192.0.2.16:3260"
	seedPublishedISCSISessionForPortal(t, portal, "iqn.test", 0, "vol1")

	// No IsPortalAccessible/Logout expectations: the LoginScan action must not log out.
	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("login boom"))

	err := core.selfHealingRectifySession(context.Background(), portal, models.LoginScan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to login to the target")
}

func TestSelfHealingRectifySession_LoginScan_LoginSucceeds_ReachesScan(t *testing.T) {
	core, mocks := newTestCore(t)
	portal := "192.0.2.17:3260"
	seedPublishedISCSISessionForPortal(t, portal, "iqn.test", 0, "vol1")

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)

	// LoginScan falls through into the Scan case, which calls the package-level
	// iscsi.InitiateScanForLuns function directly (not through an interface Core can mock).
	// In this sandbox there is no real /sys/class/scsi_host, so the scan itself fails; we can
	// only assert that login succeeded and the Scan branch was reached (a returned error,
	// rather than a panic), not that scanning itself succeeds. See final summary.
	err := core.selfHealingRectifySession(context.Background(), portal, models.LoginScan)
	assert.Error(t, err)
}

func TestSelfHealingRectifySession_NoLUNForPortal(t *testing.T) {
	core, _ := newTestCore(t)
	portal := "192.0.2.18:3260"
	resetISCSISessionGlobals(t)
	require.NoError(t, publishedISCSISessions.AddPortal(portal, models.PortalInfo{ISCSITargetIQN: "iqn.test"}))

	err := core.selfHealingRectifySession(context.Background(), portal, models.LoginScan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get volume ID for lun ID")
}

func TestSelfHealingRectifySession_DefaultAction(t *testing.T) {
	core, _ := newTestCore(t)
	portal := "192.0.2.19:3260"
	seedPublishedISCSISessionForPortal(t, portal, "iqn.test", 0, "vol1")

	err := core.selfHealingRectifySession(context.Background(), portal, models.NoAction)
	assert.NoError(t, err)
}

func TestDeprecatedIgroupInUse_NoTrackingInfo(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)

	assert.False(t, core.deprecatedIgroupInUse(context.Background()))
}

func TestDeprecatedIgroupInUse_DeprecatedIgroupDetected(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.IscsiIgroup = "not-a-per-node-igroup"
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{"vol1": ti}, nil)

	assert.True(t, core.deprecatedIgroupInUse(context.Background()))
}

func TestUpdateCHAPInfoForSessions_NilPublishedSessions(t *testing.T) {
	core, _ := newTestCore(t)
	assert.NoError(t, core.updateCHAPInfoForSessions(context.Background(), nil, models.NewISCSISessions()))
}

func TestUpdateCHAPInfoForSessions_NilCurrentSessions(t *testing.T) {
	core, _ := newTestCore(t)
	assert.NoError(t, core.updateCHAPInfoForSessions(context.Background(), models.NewISCSISessions(), nil))
}

func TestUpdateCHAPInfoForSessions_NonCHAPSessionSkipped(t *testing.T) {
	core, _ := newTestCore(t)
	published := models.NewISCSISessions()
	require.NoError(t, published.AddPortal("192.0.2.40:3260", models.PortalInfo{ISCSITargetIQN: "iqn.test"}))
	current := models.NewISCSISessions()

	// No controller.GetChap expectation: a non-CHAP portal must be skipped entirely.
	err := core.updateCHAPInfoForSessions(context.Background(), published, current)
	assert.NoError(t, err)
}

func TestUpdateCHAPInfoForSessions_CHAPSession_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	published := models.NewISCSISessions()
	portal := "192.0.2.41:3260"
	require.NoError(t, published.AddPortal(portal, models.PortalInfo{
		ISCSITargetIQN: "iqn.test",
		Credentials:    models.IscsiChapInfo{UseCHAP: true},
	}))
	require.NoError(t, published.AddLUNToPortal(portal, models.LUNData{LUN: 0, VolID: "vol1"}))
	current := models.NewISCSISessions()

	mocks.Controller.MockChapClient.EXPECT().GetChap(gomock.Any(), "vol1", "test-node").
		Return(&models.IscsiChapInfo{UseCHAP: true, IscsiUsername: "updated"}, nil)

	err := core.updateCHAPInfoForSessions(context.Background(), published, current)
	assert.NoError(t, err)

	portalInfo, ierr := published.PortalInfo(portal)
	require.NoError(t, ierr)
	assert.Equal(t, "updated", portalInfo.Credentials.IscsiUsername)
}

func TestUpdateCHAPInfoForSessions_GetChapFailure_AccumulatesButContinues(t *testing.T) {
	core, mocks := newTestCore(t)
	published := models.NewISCSISessions()

	portalBad := "192.0.2.42:3260"
	require.NoError(t, published.AddPortal(portalBad, models.PortalInfo{
		ISCSITargetIQN: "iqn.bad",
		Credentials:    models.IscsiChapInfo{UseCHAP: true},
	}))
	require.NoError(t, published.AddLUNToPortal(portalBad, models.LUNData{LUN: 0, VolID: "vol-bad"}))

	portalGood := "192.0.2.43:3260"
	require.NoError(t, published.AddPortal(portalGood, models.PortalInfo{
		ISCSITargetIQN: "iqn.good",
		Credentials:    models.IscsiChapInfo{UseCHAP: true},
	}))
	require.NoError(t, published.AddLUNToPortal(portalGood, models.LUNData{LUN: 0, VolID: "vol-good"}))

	current := models.NewISCSISessions()

	mocks.Controller.MockChapClient.EXPECT().GetChap(gomock.Any(), "vol-bad", "test-node").
		Return(nil, errors.New("chap lookup failed"))
	mocks.Controller.MockChapClient.EXPECT().GetChap(gomock.Any(), "vol-good", "test-node").
		Return(&models.IscsiChapInfo{UseCHAP: true, IscsiUsername: "updated"}, nil)

	err := core.updateCHAPInfoForSessions(context.Background(), published, current)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chap lookup failed")

	// The good portal must still have been processed despite the bad portal's failure.
	portalInfo, ierr := published.PortalInfo(portalGood)
	require.NoError(t, ierr)
	assert.Equal(t, "updated", portalInfo.Credentials.IscsiUsername)
}

func TestPerformNVMeSelfHealing_EmptyPublishedSessions(t *testing.T) {
	core, _ := newTestCore(t)
	resetNVMeSessionGlobals(t)

	core.performNVMeSelfHealing(context.Background())
}

func TestPerformNVMeSelfHealing_NonEmpty_CallsExpectedSequence(t *testing.T) {
	core, mocks := newTestCore(t)
	resetNVMeSessionGlobals(t)
	publishedNVMeSessions.AddNVMeSession(nvme.NVMeSubsystem{NQN: "nqn.test"}, []string{"192.0.2.30"})

	mocks.NVMe.EXPECT().PopulateCurrentNVMeSessions(gomock.Any(), gomock.Any()).Return(nil)
	mocks.NVMe.EXPECT().InspectNVMeSessions(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]nvme.NVMeSubsystem{})

	core.performNVMeSelfHealing(context.Background())
}

func TestFixNVMeSessions_NoSubsystems(t *testing.T) {
	core, _ := newTestCore(t)
	assert.NotPanics(t, func() {
		core.fixNVMeSessions(context.Background(), time.Now().Add(time.Second), nil)
	})
}

func TestPublicationReconciliation_Disabled_NoOp(t *testing.T) {
	core, _ := newTestCore(t)

	core.startPublicationReconciliation(context.Background())
	assert.Nil(t, core.nodePublicationTimer)
	assert.Nil(t, core.stopNodePublicationLoop)

	assert.NotPanics(t, func() { core.stopPublicationReconciliation(context.Background()) })
}

func TestPublicationReconciliation_Enabled_StartStop(t *testing.T) {
	core, _ := newTestCore(t, WithEnableForceDetach(true))

	core.startPublicationReconciliation(context.Background())
	require.NotNil(t, core.nodePublicationTimer)
	require.NotNil(t, core.stopNodePublicationLoop)

	// defaultNodeReconciliationPeriod is 1 minute, so stopping immediately avoids racing a real
	// tick (and thus avoids needing to mock reconcileNodePublicationState's dependencies here).
	assert.NotPanics(t, func() { core.stopPublicationReconciliation(context.Background()) })
}

func TestReconcileNodePublicationState_GetNodeErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	core.nodePublicationTimer = time.NewTimer(time.Hour)
	t.Cleanup(func() { core.nodePublicationTimer.Stop() })

	mocks.Controller.MockClient.EXPECT().GetNodeCleanupStatus(gomock.Any(), "test-node").
		Return(models.NodePublicationState(""), errors.New("get node boom"))

	err := core.reconcileNodePublicationState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get node boom")
}

func TestReconcileNodePublicationState_NodeClean_NoCleanupCalled(t *testing.T) {
	core, mocks := newTestCore(t)
	core.nodePublicationTimer = time.NewTimer(time.Hour)
	t.Cleanup(func() { core.nodePublicationTimer.Stop() })

	mocks.Controller.MockClient.EXPECT().GetNodeCleanupStatus(gomock.Any(), "test-node").
		Return(models.NodeClean, nil)
	// No GetDesiredPublications/ListVolumeTrackingInfo expectations: performNodeCleanup
	// must not run when the node is already clean.

	err := core.reconcileNodePublicationState(context.Background())
	assert.NoError(t, err)
}

func TestReconcileNodePublicationState_Cleanable_CallsCleanupAndUpdate(t *testing.T) {
	core, mocks := newTestCore(t)
	core.nodePublicationTimer = time.NewTimer(time.Hour)
	t.Cleanup(func() { core.nodePublicationTimer.Stop() })

	mocks.Controller.MockClient.EXPECT().GetNodeCleanupStatus(gomock.Any(), "test-node").
		Return(models.NodeCleanable, nil)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)
	mocks.Controller.MockClient.EXPECT().MarkNodeCleanupComplete(gomock.Any(), "test-node").Return(nil)

	err := core.reconcileNodePublicationState(context.Background())
	assert.NoError(t, err)
}

// discoverStalePublications

func TestDiscoverDesiredPublicationState_ControllerError(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(nil, errors.New("controller boom"))

	_, err := core.discoverDesiredPublicationState(context.Background())
	require.Error(t, err)
}

func TestDiscoverDesiredPublicationState_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(map[string]*models.VolumePublicationExternal{
			"vol1": {VolumeName: "vol1"},
			"vol2": {VolumeName: "vol2"},
		}, nil)

	state, err := core.discoverDesiredPublicationState(context.Background())
	require.NoError(t, err)
	assert.Len(t, state, 2)
	assert.Contains(t, state, "vol1")
	assert.Contains(t, state, "vol2")
}

func TestDiscoverActualPublicationState_NonNotFoundErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, errors.New("disk boom"))

	_, err := core.discoverActualPublicationState(context.Background())
	require.Error(t, err)
}

func TestDiscoverActualPublicationState_NotFoundTolerated(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(nil, errors.NotFoundError("no tracking files"))

	state, err := core.discoverActualPublicationState(context.Background())
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestDiscoverStalePublications_DeltaComputation(t *testing.T) {
	core, _ := newTestCore(t)

	actual := map[string]*models.VolumeTrackingInfo{
		"vol-published": sampleTrackingInfo(NFS),
		"vol-stale":     sampleTrackingInfo(NFS),
	}
	desired := map[string]*models.VolumePublicationExternal{
		"vol-published": {VolumeName: "vol-published"},
	}

	stale := core.discoverStalePublications(context.Background(), actual, desired)
	assert.Len(t, stale, 1)
	assert.Contains(t, stale, "vol-stale")
	assert.NotContains(t, stale, "vol-published")
}

func TestPerformNodeCleanup_DesiredStateErrorWrapped(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(nil, errors.New("controller boom"))

	err := core.performNodeCleanup(context.Background())
	require.Error(t, err)
	assert.True(t, errors.IsReconcileFailedError(err))
}

func TestPerformNodeCleanup_ActualStateErrorWrapped(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, errors.New("disk boom"))

	err := core.performNodeCleanup(context.Background())
	require.Error(t, err)
	assert.True(t, errors.IsReconcileFailedError(err))
}

func TestPerformNodeCleanup_NoStalePublications_NoCleanupCalled(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)

	err := core.performNodeCleanup(context.Background())
	assert.NoError(t, err)
}

func TestPerformNodeCleanup_StalePublicationCleaned(t *testing.T) {
	core, mocks := newTestCore(t)

	trackingInfo := sampleTrackingInfo(NFS)
	mocks.Controller.MockClient.EXPECT().GetDesiredPublications(gomock.Any(), "test-node").
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{"vol1": trackingInfo}, nil)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol1").Return(nil)

	err := core.performNodeCleanup(context.Background())
	assert.NoError(t, err)
}

//
// TestCleanStalePublications_ErrorFromOneVolumeIsNotSwallowed below reproduces (and, after the
// fix in heal.go, verifies the correction of) a real bug: cleanStalePublications's forceDetach
// closure set the OUTER "err" variable on failure but then unconditionally "return nil" itself.
// Because "err = multierr.Combine(err, forceDetach())" evaluates its first argument ("err")
// BEFORE calling forceDetach() (Go evaluates call arguments left-to-right), the freshly-set
// error was always immediately clobbered by that same assignment, so cleanStalePublications
// always returned nil even when a stale volume failed to force-detach.

func TestCleanStalePublications_HappyPath_NoPublishedPaths(t *testing.T) {
	core, mocks := newTestCore(t)

	stale := map[string]*models.VolumeTrackingInfo{"vol1": sampleTrackingInfo(NFS)}

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol1").Return(nil)

	err := core.cleanStalePublications(context.Background(), stale)
	assert.NoError(t, err)
}

func TestCleanStalePublications_MultipleStaleVolumes_AllSucceed(t *testing.T) {
	core, mocks := newTestCore(t)

	stale := map[string]*models.VolumeTrackingInfo{
		"vol-a": sampleTrackingInfo(NFS),
		"vol-b": sampleTrackingInfo(NFS),
	}

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol-a").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol-a").Return(nil)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol-b").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol-b").Return(nil)

	err := core.cleanStalePublications(context.Background(), stale)
	assert.NoError(t, err)
}

func TestCleanStalePublications_ErrorFromOneVolumeIsNotSwallowed(t *testing.T) {
	core, mocks := newTestCore(t)

	stale := map[string]*models.VolumeTrackingInfo{
		"vol-good": sampleTrackingInfo(NFS),
		"vol-bad":  sampleTrackingInfo(NFS),
	}

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol-good").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol-good").Return(nil)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol-bad").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol-bad").
		Return(errors.New("delete failed for vol-bad"))

	err := core.cleanStalePublications(context.Background(), stale)
	require.Error(t, err, "cleanStalePublications must surface a force-detach failure for at least one stale volume")
	assert.Contains(t, err.Error(), "delete failed for vol-bad")
}

func TestCleanStalePublications_PublishedPathsAreUnpublishedFirst(t *testing.T) {
	core, mocks := newTestCore(t)

	const targetPath = "/var/lib/kubelet/pods/pod1/volumes/vol1"
	ti := sampleTrackingInfo(NFS)
	ti.PublishedPaths = map[string]struct{}{targetPath: {}}
	stale := map[string]*models.VolumeTrackingInfo{"vol1": ti}

	mocks.OsUtils.EXPECT().IsLikelyDir(targetPath).Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), targetPath).Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), targetPath).Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", targetPath).Return(nil)

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(sampleTrackingInfo(NFS), nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "vol1").Return(nil)

	err := core.cleanStalePublications(context.Background(), stale)
	assert.NoError(t, err)
}

func TestUpdateNodePublicationState_NotCleanable_NoOp(t *testing.T) {
	core, _ := newTestCore(t)

	assert.NoError(t, core.updateNodePublicationState(context.Background(), models.NodeDirty))
}

func TestUpdateNodePublicationState_Cleanable_CallsUpdateNode(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.Controller.MockClient.EXPECT().MarkNodeCleanupComplete(gomock.Any(), "test-node").Return(nil)

	assert.NoError(t, core.updateNodePublicationState(context.Background(), models.NodeCleanable))
}

func TestUpdateNodePublicationState_Cleanable_ErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.Controller.MockClient.EXPECT().MarkNodeCleanupComplete(gomock.Any(), "test-node").
		Return(errors.New("update boom"))

	err := core.updateNodePublicationState(context.Background(), models.NodeCleanable)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update boom")
}
