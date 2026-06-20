// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentclientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils/errors"
)

const createVolumeMoveTimeout = 30 * time.Second

// createVolumeMoveCreateK8SClients creates Kubernetes clients for volume-move; tests may replace it.
var createVolumeMoveCreateK8SClients = k8sclient.CreateK8SClients

// createVolumeMoveTridentClientOverride, if set, is used for TridentVolumeMove Create instead of clients.TridentClient (tests).
var createVolumeMoveTridentClientOverride tridentclientset.Interface

var (
	volume                       string
	volumeMoveTargetPool         string
	volumeMoveTargetNode         string
	volumeMoveSrcNode            string
	volumeMoveSrcPool            string
	volumeMoveDeleteAfterSuccess string
)

func init() {
	createCmd.AddCommand(createVolumeMoveCmd)
	createVolumeMoveCmd.Flags().StringVar(&volume, "volume", "",
		"Name of the volume to move (also used as the TridentVolumeMove CR name)")
	createVolumeMoveCmd.Flags().StringVar(&volumeMoveTargetPool, "target-pool", "",
		"Name of the destination storage pool for the volume")
	createVolumeMoveCmd.Flags().StringVar(&volumeMoveSrcPool, "source-pool", "",
		"Name of the source storage pool for the volume")
	createVolumeMoveCmd.Flags().StringVar(&volumeMoveTargetNode, "target-node", "",
		"Name of the destination ONTAP node")
	createVolumeMoveCmd.Flags().StringVar(&volumeMoveSrcNode, "source-node", "",
		"Name of the source ONTAP node")
	createVolumeMoveCmd.Flags().StringVar(&volumeMoveDeleteAfterSuccess, "delete-after-success", "",
		"When set, delete this CR after the given duration once the move succeeds (for example 10m or 30s); "+
			"0 deletes immediately; omit to retain the CR (failed moves are not deleted)")

	_ = createVolumeMoveCmd.MarkFlagRequired("volume")
	_ = createVolumeMoveCmd.MarkFlagRequired("target-pool")
	_ = createVolumeMoveCmd.MarkFlagRequired("source-pool")
	_ = createVolumeMoveCmd.MarkFlagRequired("target-node")
	_ = createVolumeMoveCmd.MarkFlagRequired("source-node")
}

var createVolumeMoveCmd = &cobra.Command{
	Use:     "volume-move",
	Aliases: []string{"tvm"},
	Short:   "Start a volume move by creating a Kubernetes TridentVolumeMove CR",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse the duration to ensure it's valid
		deleteAfterSuccessDuration, err := parseVolumeMoveDeleteAfterSuccess(volumeMoveDeleteAfterSuccess)
		if err != nil {
			return err
		}
		if OperatingMode == ModeTunnel {
			tunnelArgs := []string{
				"create",
				"volume-move",
				"--volume", volume,
				"--target-pool", volumeMoveTargetPool,
				"--target-node", volumeMoveTargetNode,
				"--source-node", volumeMoveSrcNode,
				"--source-pool", volumeMoveSrcPool,
			}
			if volumeMoveDeleteAfterSuccess != "" {
				tunnelArgs = append(tunnelArgs, "--delete-after-success", volumeMoveDeleteAfterSuccess)
			}
			out, err := TunnelCommand(tunnelArgs)
			printOutput(cmd, out, err)
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), createVolumeMoveTimeout)
		defer cancel()
		return createTridentVolumeMove(ctx, deleteAfterSuccessDuration)
	},
}

// parseVolumeMoveDeleteAfterSuccess parses a Go duration string for deleteAfterSuccess.
// An empty string means the field is omitted on the CR.
func parseVolumeMoveDeleteAfterSuccess(durationStr string) (*metav1.Duration, error) {
	if durationStr == "" {
		return nil, nil
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid --delete-after-success duration %q: %w", durationStr, err)
	}
	if duration < 0 {
		return nil, fmt.Errorf("invalid --delete-after-success duration %q: must not be negative", durationStr)
	}
	return &metav1.Duration{Duration: duration}, nil
}

func buildTridentVolumeMove(deleteAfterSuccessDuration *metav1.Duration) *netappv1.TridentVolumeMove {
	spec := netappv1.TridentVolumeMoveSpec{
		TargetPool: volumeMoveTargetPool,
		SourcePool: volumeMoveSrcPool,
		TargetNode: volumeMoveTargetNode,
		SourceNode: volumeMoveSrcNode,
	}
	if deleteAfterSuccessDuration != nil {
		spec.DeleteAfterSuccess = deleteAfterSuccessDuration
	}

	return &netappv1.TridentVolumeMove{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netappv1.SchemeGroupVersion.String(),
			Kind:       "TridentVolumeMove",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: volume,
		},
		Spec: spec,
	}
}

func createTridentVolumeMove(ctx context.Context, deleteAfterSuccessDuration *metav1.Duration) error {
	tvm := buildTridentVolumeMove(deleteAfterSuccessDuration)

	clients, err := createVolumeMoveCreateK8SClients("", KubeConfigPath, TridentPodNamespace)
	if err != nil {
		return err
	}

	clients.K8SClient.SetTimeout(k8sTimeout)

	namespace := clients.Namespace
	if namespace == "" {
		return errors.New("namespace is required (use tridentctl -n or a kube context with a default namespace)")
	}

	trident := createVolumeMoveTridentClientOverride
	if trident == nil {
		trident = clients.TridentClient
	}
	created, err := trident.TridentV1().TridentVolumeMoves(namespace).Create(
		ctx, tvm, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("TridentVolumeMove %q already exists in namespace %s", tvm.Name, namespace)
		}
		return err
	}

	WriteVolumeMove(created)
	return nil
}
