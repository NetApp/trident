// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices/mock_luks"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

func TestAttemptLock_FastPathSucceeds(t *testing.T) {
	ctx := context.Background()
	lockID := "attemptLock-fast-" + t.Name()

	ok := attemptLock(ctx, "test", lockID, 5*time.Second)

	assert.True(t, ok)
	locks.Unlock(ctx, "test", lockID)
}

func TestAttemptLock_ReturnsFalseWhenQueuedTooLong(t *testing.T) {
	ctx := context.Background()
	lockID := "attemptLock-slow-" + t.Name()

	// Hold the lock ourselves first so attemptLock's call to locks.Lock blocks.
	locks.Lock(ctx, "holder", lockID)

	resultCh := make(chan bool, 1)
	go func() { resultCh <- attemptLock(ctx, "test", lockID, 1*time.Nanosecond) }()

	// Give the goroutine time to be queued waiting on the mutex, and to guarantee elapsed time
	// exceeds the 1ns timeout once it does acquire the lock.
	time.Sleep(50 * time.Millisecond)
	locks.Unlock(ctx, "holder", lockID)

	select {
	case result := <-resultCh:
		assert.False(t, result, "attemptLock should report it waited too long")
	case <-time.After(2 * time.Second):
		t.Fatal("attemptLock did not return")
	}
	// attemptLock still acquired the underlying mutex even though it reported timeout; release it
	// to avoid leaking global lock state into other tests.
	locks.Unlock(ctx, "test", lockID)
}

func TestAcquireVolumeLock_SucceedsWhenUncontended(t *testing.T) {
	core, _ := newTestCore(t)

	release, err := core.acquireVolumeLock(context.Background(), "vol1")
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

func TestAcquireVolumeLock_BlocksUntilVolumeLockReleased(t *testing.T) {
	core, _ := newTestCore(t)
	core.volumeLocks.Lock("vol1")

	done := make(chan struct{})
	go func() {
		release, err := core.acquireVolumeLock(context.Background(), "vol1")
		require.NoError(t, err)
		release()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("acquireVolumeLock proceeded before the volume lock was released")
	default:
	}

	core.volumeLocks.Unlock("vol1")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acquireVolumeLock did not proceed after the volume lock was released")
	}
}

func TestEnsureLUKSVolumePassphrase_EmptyPassphrase(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", map[string]string{}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LUKS passphrase cannot be empty")
}

func TestEnsureLUKSVolumePassphrase_EmptyPassphraseName(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{"luks-passphrase": "secret"}

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LUKS passphrase name cannot be empty")
}

func TestEnsureLUKSVolumePassphrase_CheckPassphraseErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{"luks-passphrase": "secret", "luks-passphrase-name": "name-a"}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, errors.New("device busy"))

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not verify passphrase name-a")
	assert.Contains(t, err.Error(), "device busy")
}

func TestEnsureLUKSVolumePassphrase_CurrentPassphraseAlreadyMatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{"luks-passphrase": "secret", "luks-passphrase-name": "name-a"}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(true, nil)

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	assert.NoError(t, err)
}

func TestEnsureLUKSVolumePassphrase_NoPreviousPassphraseProvided(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{"luks-passphrase": "secret", "luks-passphrase-name": "name-a"}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, nil)

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no working passphrase provided")
}

func TestEnsureLUKSVolumePassphrase_PreviousPassphraseNameEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{
		"luks-passphrase":          "secret",
		"luks-passphrase-name":     "name-a",
		"previous-luks-passphrase": "old-secret",
	}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, nil)

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "previous LUKS passphrase name cannot be empty")
}

func TestEnsureLUKSVolumePassphrase_PreviousPassphraseDoesNotCheckOut(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{
		"luks-passphrase":               "secret",
		"luks-passphrase-name":          "name-a",
		"previous-luks-passphrase":      "old-secret",
		"previous-luks-passphrase-name": "name-b",
	}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, nil)
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "old-secret").Return(false, nil)

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no working passphrase provided")
}

func TestEnsureLUKSVolumePassphrase_PreviousPassphraseCheckErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{
		"luks-passphrase":               "secret",
		"luks-passphrase-name":          "name-a",
		"previous-luks-passphrase":      "old-secret",
		"previous-luks-passphrase-name": "name-b",
	}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, nil)
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "old-secret").Return(false, errors.New("device error"))

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not verify passphrase name-b")
	assert.Contains(t, err.Error(), "device error")
}

func TestEnsureLUKSVolumePassphrase_SuccessfulRotation(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{
		"luks-passphrase":               "secret",
		"luks-passphrase-name":          "name-a",
		"previous-luks-passphrase":      "old-secret",
		"previous-luks-passphrase-name": "name-b",
	}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, nil)
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "old-secret").Return(true, nil)
	luksDevice.EXPECT().RotatePassphrase(gomock.Any(), "vol-1", "old-secret", "secret").Return(nil)

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	assert.NoError(t, err)
}

func TestEnsureLUKSVolumePassphrase_RotatePassphraseErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	luksDevice := mock_luks.NewMockDevice(ctrl)
	secrets := map[string]string{
		"luks-passphrase":               "secret",
		"luks-passphrase-name":          "name-a",
		"previous-luks-passphrase":      "old-secret",
		"previous-luks-passphrase-name": "name-b",
	}
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "secret").Return(false, nil)
	luksDevice.EXPECT().CheckPassphrase(gomock.Any(), "old-secret").Return(true, nil)
	luksDevice.EXPECT().RotatePassphrase(gomock.Any(), "vol-1", "old-secret", "secret").Return(errors.New("cryptsetup failed"))

	err := ensureLUKSVolumePassphrase(context.Background(), luksDevice, "vol-1", secrets, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to rotate LUKS passphrase")
	assert.Contains(t, err.Error(), "cryptsetup failed")
}

func TestGetVolumeProtocolFromPublishInfo(t *testing.T) {
	tests := []struct {
		name         string
		publishInfo  *models.VolumePublishInfo
		expectedProt config.Protocol
		expectErr    bool
	}{
		{
			name:         "SMB alone is File",
			publishInfo:  samplePublishInfo(SMB),
			expectedProt: config.File,
		},
		{
			name:         "NFS alone is File",
			publishInfo:  samplePublishInfo(NFS),
			expectedProt: config.File,
		},
		{
			name:         "iSCSI alone is Block",
			publishInfo:  samplePublishInfo(ISCSI),
			expectedProt: config.Block,
		},
		{
			name:         "NVMe alone is Block",
			publishInfo:  samplePublishInfo(NVMe),
			expectedProt: config.Block,
		},
		{
			name:         "FCP alone is Block",
			publishInfo:  samplePublishInfo(FCP),
			expectedProt: config.Block,
		},
		{
			name: "ambiguous NFS + iSCSI is an error",
			publishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					NfsAccessInfo:   models.NfsAccessInfo{NfsServerIP: "192.0.2.1"},
					IscsiAccessInfo: models.IscsiAccessInfo{IscsiTargetIQN: "iqn.test"},
				},
			},
			expectErr: true,
		},
		// Regression coverage: isNfs/isSmb previously did not exclude nqnSet/fcpSet, so these
		// combinations silently misclassified as File instead of erroring on ambiguous input.
		{
			name: "ambiguous NFS + NVMe is an error",
			publishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					NfsAccessInfo:  models.NfsAccessInfo{NfsServerIP: "192.0.2.1"},
					NVMeAccessInfo: models.NVMeAccessInfo{NVMeSubsystemNQN: "nqn.test"},
				},
			},
			expectErr: true,
		},
		{
			name: "ambiguous NFS + FCP is an error",
			publishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					NfsAccessInfo: models.NfsAccessInfo{NfsServerIP: "192.0.2.1"},
					FCPAccessInfo: models.FCPAccessInfo{
						FibreChannelAccessInfo: models.FibreChannelAccessInfo{FCTargetWWNN: "wwnn.test"},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "ambiguous SMB + NVMe is an error",
			publishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					SMBAccessInfo:  models.SMBAccessInfo{SMBPath: "\\\\server\\share"},
					NVMeAccessInfo: models.NVMeAccessInfo{NVMeSubsystemNQN: "nqn.test"},
				},
			},
			expectErr: true,
		},
		{
			name: "ambiguous SMB + FCP is an error",
			publishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					SMBAccessInfo: models.SMBAccessInfo{SMBPath: "\\\\server\\share"},
					FCPAccessInfo: models.FCPAccessInfo{
						FibreChannelAccessInfo: models.FibreChannelAccessInfo{FCTargetWWNN: "wwnn.test"},
					},
				},
			},
			expectErr: true,
		},
		{
			name:        "all empty is an error",
			publishInfo: &models.VolumePublishInfo{},
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, err := getVolumeProtocolFromPublishInfo(tt.publishInfo)
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to infer volume protocol")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedProt, protocol)
		})
	}
}

func TestReadAllTrackingFiles_NoTrackingDirReturnsEmpty(t *testing.T) {
	core, _ := newTestCore(t)

	// GetAllVolumeIDs does a real os.ReadDir against tridentDeviceInfoPath, which won't exist in
	// this sandbox; it handles that gracefully by returning nil, so no NodeHelper calls should
	// ever be made (no expectations are set on mocks.NodeHelper).
	publishInfos := core.readAllTrackingFiles(context.Background())

	assert.Empty(t, publishInfos)
}

// fakeNVMeSubsystem is a minimal hand-rolled fake for nvme.NVMeSubsystemInterface: no gomock
// mock exists for this interface under mocks/mock_utils/nvme, only for NVMeInterface. Any method
// besides Disconnect is intentionally left unimplemented (nil-embedded) since
// disconnectNVMeSubsystemIfNeeded never calls them; using any of them would panic, which is the
// desired failure mode if the code under test changes to call something unexpected.
type fakeNVMeSubsystem struct {
	nvme.NVMeSubsystemInterface
	disconnectErr   error
	disconnectCalls int
}

func (f *fakeNVMeSubsystem) Disconnect(_ context.Context) error {
	f.disconnectCalls++
	return f.disconnectErr
}

// withCleanPublishedNVMeSessions snapshots and restores the package-level publishedNVMeSessions
// global so this test's seeding doesn't bleed into other tests in the package.
func withCleanPublishedNVMeSessions(t *testing.T) {
	original := publishedNVMeSessions
	publishedNVMeSessions = nvme.NVMeSessions{}
	t.Cleanup(func() { publishedNVMeSessions = original })
}

func TestDisconnectNVMeSubsystemIfNeeded_NoNamespaces_DisconnectsRegardlessOfFlag(t *testing.T) {
	withCleanPublishedNVMeSessions(t)
	core, _ := newTestCore(t)
	pi := samplePublishInfo(NVMe)
	fakeSubsys := &fakeNVMeSubsystem{}

	err := core.disconnectNVMeSubsystemIfNeeded(context.Background(), fakeSubsys, pi, false)

	require.NoError(t, err)
	assert.Equal(t, 1, fakeSubsys.disconnectCalls)
}

func TestDisconnectNVMeSubsystemIfNeeded_NamespacesPresent_DisconnectFlagFalse_NoDisconnect(t *testing.T) {
	withCleanPublishedNVMeSessions(t)
	core, _ := newTestCore(t, WithNVMeSelfHealingInterval(5*time.Second))
	pi := samplePublishInfo(NVMe)
	publishedNVMeSessions.AddNVMeSession(nvme.NVMeSubsystem{NQN: pi.NVMeSubsystemNQN}, nil)
	publishedNVMeSessions.AddNamespaceToSession(pi.NVMeSubsystemNQN, "ns-1")
	fakeSubsys := &fakeNVMeSubsystem{}

	err := core.disconnectNVMeSubsystemIfNeeded(context.Background(), fakeSubsys, pi, false)

	require.NoError(t, err)
	assert.Equal(t, 0, fakeSubsys.disconnectCalls)
}

func TestDisconnectNVMeSubsystemIfNeeded_NamespacesPresent_SelfHealingDisabled_NoDisconnect(t *testing.T) {
	withCleanPublishedNVMeSessions(t)
	// nvmeSelfHealingInterval defaults to zero (disabled) when not set via WithNVMeSelfHealingInterval.
	core, _ := newTestCore(t)
	pi := samplePublishInfo(NVMe)
	publishedNVMeSessions.AddNVMeSession(nvme.NVMeSubsystem{NQN: pi.NVMeSubsystemNQN}, nil)
	publishedNVMeSessions.AddNamespaceToSession(pi.NVMeSubsystemNQN, "ns-1")
	fakeSubsys := &fakeNVMeSubsystem{}

	err := core.disconnectNVMeSubsystemIfNeeded(context.Background(), fakeSubsys, pi, true)

	require.NoError(t, err)
	assert.Equal(t, 0, fakeSubsys.disconnectCalls, "self-healing disabled must gate the disconnect hint even if disconnect=true")
}

func TestDisconnectNVMeSubsystemIfNeeded_NamespacesPresent_SelfHealingEnabledAndDisconnect_Disconnects(t *testing.T) {
	withCleanPublishedNVMeSessions(t)
	core, _ := newTestCore(t, WithNVMeSelfHealingInterval(5*time.Second))
	pi := samplePublishInfo(NVMe)
	publishedNVMeSessions.AddNVMeSession(nvme.NVMeSubsystem{NQN: pi.NVMeSubsystemNQN}, nil)
	publishedNVMeSessions.AddNamespaceToSession(pi.NVMeSubsystemNQN, "ns-1")
	fakeSubsys := &fakeNVMeSubsystem{}

	err := core.disconnectNVMeSubsystemIfNeeded(context.Background(), fakeSubsys, pi, true)

	require.NoError(t, err)
	assert.Equal(t, 1, fakeSubsys.disconnectCalls)
}

func TestDisconnectNVMeSubsystemIfNeeded_DisconnectErrorPropagates(t *testing.T) {
	withCleanPublishedNVMeSessions(t)
	core, _ := newTestCore(t)
	pi := samplePublishInfo(NVMe)
	fakeSubsys := &fakeNVMeSubsystem{disconnectErr: errors.New("disconnect failed")}

	err := core.disconnectNVMeSubsystemIfNeeded(context.Background(), fakeSubsys, pi, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disconnect failed")
}
