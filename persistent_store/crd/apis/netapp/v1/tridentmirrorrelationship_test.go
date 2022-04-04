// Copyright 2021 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"
)

func TestIsValid(t *testing.T) {

	// Test valid cases
	validMR := &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{{LocalPVCName: "test"}},
		},
	}
	actual, _ := validMR.IsValid()
	if !actual {
		t.Fatalf("TridentMirrorRelationship should be valid, %v", validMR)
	}

	validMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{
				{
					RemoteVolumeHandle: "foo",
					LocalPVCName:       "bar",
				},
			},
			MirrorState: MirrorStateEstablished,
		},
	}
	actual, _ = validMR.IsValid()
	if !actual {
		t.Fatalf("TridentMirrorRelationship should be valid, %v", validMR)
	}

	validMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{
				{
					RemoteVolumeHandle: "foo",
					LocalPVCName:       "bar",
				},
			},
			MirrorState: MirrorStateReestablished,
		},
	}
	actual, _ = validMR.IsValid()
	if !actual {
		t.Fatalf("TridentMirrorRelationship should be valid, %v", validMR)
	}

	validMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{
				{
					LocalPVCName:           "test",
					PromotedSnapshotHandle: "test",
				},
			},
			MirrorState: MirrorStatePromoted,
		},
	}
	actual, _ = validMR.IsValid()
	if !actual {
		t.Fatalf("TridentMirrorRelationship should be valid, %v", validMR)
	}

	// Test invalid cases
	// Test nil volume mappings
	invalidMR := &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be invalid, %v", invalidMR)
	}

	// Test 0 volume mappings
	invalidMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{},
		},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be invalid, %v", invalidMR)
	}

	// Test volume mapping is missing local pvc name
	invalidMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{{}},
		},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be invalid, %v", invalidMR)
	}

	// Test unsupported mirror state
	invalidMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{{LocalPVCName: "test"}},
			MirrorState:    "notamirrorstate",
		},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be invalid, %v", invalidMR)
	}

	// Test providing a promotedSnapshotHandle when not promoting a relationship
	invalidMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{
				{
					LocalPVCName:           "test",
					PromotedSnapshotHandle: "test",
				},
			},
			MirrorState: MirrorStateEstablished,
		},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be invalid, %v", invalidMR)
	}

	// Test setting state to established but not providing a remoteVolumeHandle
	invalidMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{{LocalPVCName: "test"}},
			MirrorState:    MirrorStateEstablished,
		},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be valid, %v", invalidMR)
	}

	// Test setting state to reestablished but not providing a remoteVolumeHandle
	invalidMR = &TridentMirrorRelationship{
		Spec: TridentMirrorRelationshipSpec{
			VolumeMappings: []*TridentMirrorRelationshipVolumeMapping{{LocalPVCName: "test"}},
			MirrorState:    MirrorStateReestablished,
		},
	}
	actual, _ = invalidMR.IsValid()
	if actual {
		t.Fatalf("TridentMirrorRelationship should be valid, %v", invalidMR)
	}
}
