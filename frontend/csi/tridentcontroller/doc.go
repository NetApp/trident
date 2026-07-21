// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package tridentcontroller defines the node↔controller protocol for Trident node registration,
// publication desired state, and node cleanup.
//
//   - Client — outbound API used by the CSI node process (REST or CRD transport in subpackages).
//   - Server — transport-agnostic inbound registration API implemented by the CSI controller helper.
//
// Low-level REST HTTP to the controller backchannel remains in frontend/csi/controller_api.
package tridentcontroller
