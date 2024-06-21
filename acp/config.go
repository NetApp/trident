// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

const (
	// HTTP defaults.

	DefaultBaseURL = "http://127.0.0.1:8100"

	// Feature constants.

	FeatureSnapshotRestore      = "SnapshotRestore"
	FeatureSnapshotMirrorUpdate = "SnapshotMirrorUpdate"
	FeatureReadOnlyClone        = "ReadOnlyClone"
	FeatureInflightEncryption   = "InflightEncryption"
)
