// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package node_helpers provides CSI node-side helpers for different container orchestrators.
//
// # Node-side responsibilities
//
// Trident CSI node flows split across two cooperating surfaces:
//
//   - node_helpers.NodeHelper — node-local tracking (VolumePublishManager), published paths,
//     volume stats, and tridentcontroller.ClientFactory for node→controller transport (CRD or REST).
package nodehelpers
