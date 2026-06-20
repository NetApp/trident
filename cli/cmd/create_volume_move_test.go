// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	fakeTridentClient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
)

var createVolumeMoveTestMutex sync.RWMutex

type savedCreateVolumeMoveGlobals struct {
	volume                       string
	volumeMoveTargetPool         string
	volumeMoveTargetNode         string
	volumeMoveSrcNode            string
	volumeMoveSrcPool            string
	volumeMoveDeleteAfterSuccess string
}

func saveCreateVolumeMoveGlobals() savedCreateVolumeMoveGlobals {
	return savedCreateVolumeMoveGlobals{
		volume:                       volume,
		volumeMoveTargetPool:         volumeMoveTargetPool,
		volumeMoveTargetNode:         volumeMoveTargetNode,
		volumeMoveSrcNode:            volumeMoveSrcNode,
		volumeMoveSrcPool:            volumeMoveSrcPool,
		volumeMoveDeleteAfterSuccess: volumeMoveDeleteAfterSuccess,
	}
}

func restoreCreateVolumeMoveGlobals(s savedCreateVolumeMoveGlobals) {
	volume = s.volume
	volumeMoveTargetPool = s.volumeMoveTargetPool
	volumeMoveTargetNode = s.volumeMoveTargetNode
	volumeMoveSrcNode = s.volumeMoveSrcNode
	volumeMoveSrcPool = s.volumeMoveSrcPool
	volumeMoveDeleteAfterSuccess = s.volumeMoveDeleteAfterSuccess
}

func withCreateVolumeMoveTestMode(t *testing.T, operatingMode string, mockCommand func() *exec.Cmd, fn func()) {
	t.Helper()
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prevMove := saveCreateVolumeMoveGlobals()
	prevOperatingMode := OperatingMode
	prevTridentPodName := TridentPodName
	prevTridentPodNamespace := TridentPodNamespace
	prevExecKubernetesCLIRaw := execKubernetesCLIRaw

	defer func() {
		restoreCreateVolumeMoveGlobals(prevMove)
		OperatingMode = prevOperatingMode
		TridentPodName = prevTridentPodName
		TridentPodNamespace = prevTridentPodNamespace
		execKubernetesCLIRaw = prevExecKubernetesCLIRaw
	}()

	OperatingMode = operatingMode
	if operatingMode == ModeTunnel {
		TridentPodName = "trident-controller-test"
		TridentPodNamespace = "trident"
		if mockCommand != nil {
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return mockCommand()
			}
		} else {
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return exec.Command("echo", "Created TridentVolumeMove trident/tvm-1")
			}
		}
	}

	fn()
}

func TestBuildTridentVolumeMove(t *testing.T) {
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prev := saveCreateVolumeMoveGlobals()
	defer restoreCreateVolumeMoveGlobals(prev)

	volume = "pv-1"
	volumeMoveTargetPool = "pool-dst"
	volumeMoveSrcPool = "pool-src"
	volumeMoveTargetNode = "node-dst"
	volumeMoveSrcNode = "node-src"

	t.Run("cr_name_from_volume", func(t *testing.T) {
		tvm := buildTridentVolumeMove(nil)
		assert.Equal(t, "pv-1", tvm.Name)
		assert.Empty(t, tvm.GenerateName)
		assert.Equal(t, netappv1.SchemeGroupVersion.String(), tvm.APIVersion)
		assert.Equal(t, "TridentVolumeMove", tvm.Kind)
		assert.Equal(t, "pool-dst", tvm.Spec.TargetPool)
		assert.Equal(t, "pool-src", tvm.Spec.SourcePool)
		assert.Equal(t, "node-dst", tvm.Spec.TargetNode)
		assert.Equal(t, "node-src", tvm.Spec.SourceNode)
		assert.Nil(t, tvm.Spec.DeleteAfterSuccess)
	})

	t.Run("delete_after_success", func(t *testing.T) {
		d, err := parseVolumeMoveDeleteAfterSuccess("10m")
		require.NoError(t, err)
		tvm := buildTridentVolumeMove(d)
		require.NotNil(t, tvm.Spec.DeleteAfterSuccess)
		assert.Equal(t, 10*time.Minute, tvm.Spec.DeleteAfterSuccess.Duration)
	})
}

func TestParseVolumeMoveDeleteAfterSuccess(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		d, err := parseVolumeMoveDeleteAfterSuccess("")
		assert.NoError(t, err)
		assert.Nil(t, d)
	})

	t.Run("valid", func(t *testing.T) {
		d, err := parseVolumeMoveDeleteAfterSuccess("30s")
		assert.NoError(t, err)
		require.NotNil(t, d)
		assert.Equal(t, 30*time.Second, d.Duration)
	})

	t.Run("zero", func(t *testing.T) {
		d, err := parseVolumeMoveDeleteAfterSuccess("0")
		assert.NoError(t, err)
		require.NotNil(t, d)
		assert.Equal(t, time.Duration(0), d.Duration)
	})

	t.Run("invalid_syntax", func(t *testing.T) {
		_, err := parseVolumeMoveDeleteAfterSuccess("not-a-duration")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --delete-after-success")
	})

	t.Run("negative", func(t *testing.T) {
		_, err := parseVolumeMoveDeleteAfterSuccess("-1m")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must not be negative")
	})
}

func TestCreateVolumeMoveCmd_InvalidDeleteAfterSuccess(t *testing.T) {
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prev := saveCreateVolumeMoveGlobals()
	defer restoreCreateVolumeMoveGlobals(prev)

	volume = "pv1"
	volumeMoveTargetPool = "tp"
	volumeMoveSrcPool = "sp"
	volumeMoveTargetNode = "tn"
	volumeMoveSrcNode = "sn"
	volumeMoveDeleteAfterSuccess = "bogus"

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := createVolumeMoveCmd.RunE(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --delete-after-success")
}

func TestCreateVolumeMoveCmd_ValidateArgs(t *testing.T) {
	assert.NoError(t, createVolumeMoveCmd.ValidateArgs(nil))
	assert.Error(t, createVolumeMoveCmd.ValidateArgs([]string{"my-volume"}))
}

func TestCreateVolumeMoveCmd_Aliases(t *testing.T) {
	assert.Contains(t, createVolumeMoveCmd.Aliases, "tvm")
}

func TestCreateVolumeMoveCmd_RunE(t *testing.T) {
	volumeName := "pv1"
	testCases := []struct {
		name          string
		operatingMode string
		mockCommand   func() *exec.Cmd
		wantErr       bool
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			wantErr:       false,
		},
		{
			name:          "tunnel mode error",
			operatingMode: ModeTunnel,
			wantErr:       true,
			mockCommand: func() *exec.Cmd {
				return exec.Command("false")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withCreateVolumeMoveTestMode(t, tc.operatingMode, tc.mockCommand, func() {
				innerPrev := saveCreateVolumeMoveGlobals()
				defer restoreCreateVolumeMoveGlobals(innerPrev)

				volume = volumeName
				volumeMoveTargetPool = "tp"
				volumeMoveSrcPool = "sp"
				volumeMoveTargetNode = "tn"
				volumeMoveSrcNode = "sn"
				cmd := &cobra.Command{}
				err := createVolumeMoveCmd.RunE(cmd, nil)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestCreateTridentVolumeMove_K8SClientError(t *testing.T) {
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prev := saveCreateVolumeMoveGlobals()
	prevHook := createVolumeMoveCreateK8SClients
	defer func() {
		restoreCreateVolumeMoveGlobals(prev)
		createVolumeMoveCreateK8SClients = prevHook
	}()

	volume = "pv1"
	volumeMoveTargetPool = "tp"
	volumeMoveSrcPool = "sp"
	volumeMoveTargetNode = "tn"
	volumeMoveSrcNode = "sn"
	createVolumeMoveCreateK8SClients = func(_, _, _ string) (*k8sclient.Clients, error) {
		return nil, assert.AnError
	}

	err := createTridentVolumeMove(context.Background(), nil)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestCreateTridentVolumeMove_NamespaceRequired(t *testing.T) {
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prev := saveCreateVolumeMoveGlobals()
	prevHook := createVolumeMoveCreateK8SClients
	defer func() {
		restoreCreateVolumeMoveGlobals(prev)
		createVolumeMoveCreateK8SClients = prevHook
	}()

	ctrl := gomock.NewController(t)
	mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
	mockK8s.EXPECT().SetTimeout(gomock.Any()).Times(1)

	createVolumeMoveCreateK8SClients = func(_, _, _ string) (*k8sclient.Clients, error) {
		return &k8sclient.Clients{
			Namespace: "",
			K8SClient: mockK8s,
		}, nil
	}

	err := createTridentVolumeMove(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}

func TestCreateTridentVolumeMove_Success(t *testing.T) {
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prev := saveCreateVolumeMoveGlobals()
	prevHook := createVolumeMoveCreateK8SClients
	prevTridentOverride := createVolumeMoveTridentClientOverride
	defer func() {
		restoreCreateVolumeMoveGlobals(prev)
		createVolumeMoveCreateK8SClients = prevHook
		createVolumeMoveTridentClientOverride = prevTridentOverride
	}()

	volume = "pv1"
	volumeMoveTargetPool = "tp"
	volumeMoveSrcPool = "sp"
	volumeMoveTargetNode = "tn"
	volumeMoveSrcNode = "sn"
	ctrl := gomock.NewController(t)
	mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
	mockK8s.EXPECT().SetTimeout(gomock.Any()).Times(1)

	fakeTrident := fakeTridentClient.NewSimpleClientset()
	createVolumeMoveTridentClientOverride = fakeTrident

	createVolumeMoveCreateK8SClients = func(_, _, _ string) (*k8sclient.Clients, error) {
		return &k8sclient.Clients{
			Namespace: "trident",
			K8SClient: mockK8s,
		}, nil
	}

	_, finish := captureStdout(t)
	err := createTridentVolumeMove(context.Background(), nil)
	output := finish()

	assert.NoError(t, err)
	assert.Contains(t, output, "pv1")
	assert.Contains(t, output, "tp")
	assert.Contains(t, output, "tn")
}

func TestCreateTridentVolumeMove_AlreadyExists(t *testing.T) {
	createVolumeMoveTestMutex.Lock()
	defer createVolumeMoveTestMutex.Unlock()

	prev := saveCreateVolumeMoveGlobals()
	prevHook := createVolumeMoveCreateK8SClients
	prevTridentOverride := createVolumeMoveTridentClientOverride
	defer func() {
		restoreCreateVolumeMoveGlobals(prev)
		createVolumeMoveCreateK8SClients = prevHook
		createVolumeMoveTridentClientOverride = prevTridentOverride
	}()

	ns := "trident"
	existing := &netappv1.TridentVolumeMove{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netappv1.SchemeGroupVersion.String(),
			Kind:       "TridentVolumeMove",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dup-move",
			Namespace: ns,
		},
		Spec: netappv1.TridentVolumeMoveSpec{},
	}

	volume = "dup-move"
	volumeMoveTargetPool = "tp"
	volumeMoveSrcPool = "sp"
	volumeMoveTargetNode = "tn"
	volumeMoveSrcNode = "sn"

	ctrl := gomock.NewController(t)
	mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
	mockK8s.EXPECT().SetTimeout(gomock.Any()).Times(1)

	fakeTrident := fakeTridentClient.NewSimpleClientset(existing)
	createVolumeMoveTridentClientOverride = fakeTrident

	createVolumeMoveCreateK8SClients = func(_, _, _ string) (*k8sclient.Clients, error) {
		return &k8sclient.Clients{
			Namespace: ns,
			K8SClient: mockK8s,
		}, nil
	}

	err := createTridentVolumeMove(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `TridentVolumeMove "dup-move" already exists`)
}

func TestCreateVolumeMoveCmd_InvalidDeleteAfterSuccess_TunnelMode(t *testing.T) {
	var execCalled bool
	withCreateVolumeMoveTestMode(t, ModeTunnel, func() *exec.Cmd {
		execCalled = true
		return exec.Command("false")
	}, func() {
		volume = "pv1"
		volumeMoveTargetPool = "tp"
		volumeMoveSrcPool = "sp"
		volumeMoveTargetNode = "tn"
		volumeMoveSrcNode = "sn"
		volumeMoveDeleteAfterSuccess = "bogus"

		err := createVolumeMoveCmd.RunE(&cobra.Command{}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --delete-after-success")
		assert.False(t, execCalled, "TunnelCommand should not run for invalid duration")
	})
}
