// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"

	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

// WriteVolumeMove displays a single TridentVolumeMove using the global output format.
func WriteVolumeMove(tvm *netappv1.TridentVolumeMove) {
	WriteVolumeMoves([]*netappv1.TridentVolumeMove{tvm})
}

// WriteVolumeMoves displays one or more TridentVolumeMove resources.
func WriteVolumeMoves(moves []*netappv1.TridentVolumeMove) {
	if len(moves) == 0 {
		return
	}

	displayMoves := volumeMovesForDisplay(moves)

	switch OutputFormat {
	case FormatJSON:
		// If only one, write it directly. Otherwise, write it as a list.
		if len(displayMoves) == 1 {
			WriteJSON(displayMoves[0])
		} else {
			WriteJSON(displayMoves)
		}
	case FormatYAML:
		if len(displayMoves) == 1 {
			WriteYAML(displayMoves[0])
		} else {
			for i, tvm := range displayMoves {
				if i > 0 {
					fmt.Println("---")
				}
				WriteYAML(tvm)
			}
		}
	case FormatName:
		writeVolumeMoveNames(moves)
	case FormatWide:
		writeWideVolumeMoveTable(moves)
	default:
		writeVolumeMoveTable(moves)
	}
}

func writeVolumeMoveNames(moves []*netappv1.TridentVolumeMove) {
	for _, tvm := range moves {
		if tvm == nil {
			continue
		}
		fmt.Println(tvm.Name)
	}
}

func writeVolumeMoveTable(moves []*netappv1.TridentVolumeMove) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Name", "Target Pool", "Target Node"})

	for _, tvm := range moves {
		if tvm == nil {
			continue
		}
		_ = table.Append([]string{
			tvm.Name,
			tvm.Spec.TargetPool,
			tvm.Spec.TargetNode,
		})
	}

	_ = table.Render()
}

func writeWideVolumeMoveTable(moves []*netappv1.TridentVolumeMove) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{
		"Name",
		"Source Pool",
		"Source Node",
		"Target Pool",
		"Target Node",
	})

	for _, tvm := range moves {
		if tvm == nil {
			continue
		}
		_ = table.Append([]string{
			tvm.Name,
			tvm.Spec.SourcePool,
			tvm.Spec.SourceNode,
			tvm.Spec.TargetPool,
			tvm.Spec.TargetNode,
		})
	}

	_ = table.Render()
}

// volumeMovesForDisplay returns copies of TridentVolumeMove objects suitable for CLI
// yaml/json output. Server-populated metadata such as managedFields is omitted.
func volumeMovesForDisplay(moves []*netappv1.TridentVolumeMove) []*netappv1.TridentVolumeMove {
	display := make([]*netappv1.TridentVolumeMove, 0, len(moves))
	for _, tvm := range moves {
		if tvm == nil {
			continue
		}
		copy := tvm.DeepCopy()
		copy.ManagedFields = nil
		display = append(display, copy)
	}
	return display
}
