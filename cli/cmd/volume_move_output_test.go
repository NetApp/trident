// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils/models"

	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

func testTridentVolumeMove() *netappv1.TridentVolumeMove {
	return &netappv1.TridentVolumeMove{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netappv1.SchemeGroupVersion.String(),
			Kind:       "TridentVolumeMove",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv-1",
			Namespace: "trident",
		},
		Spec: netappv1.TridentVolumeMoveSpec{
			SourcePool: "pool-src",
			TargetPool: "pool-dst",
			SourceNode: "node-src",
			TargetNode: "node-dst",
		},
		Status: netappv1.TridentVolumeMoveStatus{
			State: models.VolumeMoveStatePending,
		},
	}
}

func TestWriteVolumeMoves(t *testing.T) {
	tvm := testTridentVolumeMove()

	testCases := []struct {
		name         string
		outputFormat string
		contains     []string
		notContains  []string
	}{
		{
			name:         "default table",
			outputFormat: "",
			contains:     []string{"NAME", "TARGET POOL", "TARGET NODE", "pv-1", "pool-dst", "node-dst"},
			notContains:  []string{"kind: TridentVolumeMove", "VOLUME"},
		},
		{
			name:         "yaml format",
			outputFormat: FormatYAML,
			contains:     []string{"kind: TridentVolumeMove", "name: pv-1"},
			notContains:  []string{"managedFields", "f:spec"},
		},
		{
			name:         "json format",
			outputFormat: FormatJSON,
			contains:     []string{`"kind": "TridentVolumeMove"`, `"name": "pv-1"`},
			notContains:  []string{"managedFields", "f:spec"},
		},
		{
			name:         "name format",
			outputFormat: FormatName,
			contains:     []string{"pv-1\n"},
			notContains:  []string{"pool-dst"},
		},
		{
			name:         "wide format",
			outputFormat: FormatWide,
			contains: []string{
				"NAME", "SOURCE POOL", "SOURCE NODE",
				"pv-1", "pool-src", "pool-dst", "node-src", "node-dst",
			},
			notContains: []string{"VOLUME"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origFormat := OutputFormat
			OutputFormat = tc.outputFormat
			defer func() { OutputFormat = origFormat }()

			_, finish := captureStdout(t)
			WriteVolumeMoves([]*netappv1.TridentVolumeMove{tvm})
			output := finish()

			for _, want := range tc.contains {
				assert.Contains(t, output, want)
			}
			for _, omit := range tc.notContains {
				assert.NotContains(t, output, omit)
			}
		})
	}
}

func TestVolumeMovesForDisplay_OmitsManagedFields(t *testing.T) {
	tvm := testTridentVolumeMove()
	tvm.ManagedFields = []metav1.ManagedFieldsEntry{
		{
			Manager:    "tridentctl",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:targetPool":{}}}`)},
		},
	}

	display := volumeMovesForDisplay([]*netappv1.TridentVolumeMove{tvm})
	assert.Len(t, display, 1)
	assert.Nil(t, display[0].ManagedFields)
	assert.NotSame(t, tvm, display[0])

	origFormat := OutputFormat
	OutputFormat = FormatYAML
	defer func() { OutputFormat = origFormat }()

	_, finish := captureStdout(t)
	WriteVolumeMoves([]*netappv1.TridentVolumeMove{tvm})
	output := finish()
	assert.NotContains(t, output, "managedFields")
	assert.NotContains(t, output, "f:spec")
}
