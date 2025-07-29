// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"

	tridentconfig "github.com/netapp/trident/config"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockhelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var ctx = context.Background()

func generateController(
	mockOrchestrator *mockcore.MockOrchestrator, mockHelper *mockhelpers.MockControllerHelper,
) *Plugin {
	controllerServer := &Plugin{
		orchestrator:     mockOrchestrator,
		name:             Provisioner,
		nodeName:         "foo",
		version:          tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:         "bar",
		role:             CSIController,
		controllerHelper: mockHelper,
		opCache:          sync.Map{},
	}
	return controllerServer
}

func generateVolumePublicationFromCSIPublishRequest(req *csi.ControllerPublishVolumeRequest) *models.VolumePublication {
	vp := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(req.GetVolumeId(), req.GetNodeId()),
		VolumeName: req.GetVolumeId(),
		NodeName:   req.GetNodeId(),
		ReadOnly:   req.GetReadonly(),
	}
	if req.VolumeCapability != nil {
		if req.VolumeCapability.GetAccessMode() != nil {
			vp.AccessMode = int32(req.VolumeCapability.GetAccessMode().GetMode())
		}
	}
	return vp
}

func generateFakeNode(nodeID string) *models.Node {
	fakeNode := &models.Node{
		Name:    nodeID,
		Deleted: false,
	}
	return fakeNode
}

func generateFakeVolumeExternal(volumeID string) *storage.VolumeExternal {
	fakeVolumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:     volumeID,
			Protocol: "file",
		},
		Backend:     "backend",
		BackendUUID: "backendUUID",
		Pool:        "pool",
		Orphaned:    false,
		State:       storage.VolumeStateOnline,
	}
	return fakeVolumeExternal
}

var (
	// Define standard volume capabilities
	stdFilesystemVolCap = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			},
		},
	}

	stdRawBlockVolCap = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			},
		},
	}

	stdBlockVolCap = []*csi.VolumeCapability{
		{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			},
		},
	}

	stdMixedVolCap = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}

	stdCapacityRange = &csi.CapacityRange{
		RequiredBytes: 1024 * 1024 * 1024, // 1GB
	}

	stdParameters = map[string]string{
		"storageClass": "standard",
		"fsType":       "ext4",
	}
)

func TestCreateVolume(t *testing.T) {
	volName := "test-volume-id"
	volSize := int64(1024 * 1024 * 1024) // 1GB
	testCases := []struct {
		name                   string
		req                    *csi.CreateVolumeRequest
		expectedVolumeResponse *csi.CreateVolumeResponse
		expErrCode             codes.Code

		supportsFeatureReturn bool

		getVolumeReturns      *storage.VolumeExternal
		getVolumeError        error
		getVolumeConfigReturn *storage.VolumeConfig
		getVolumeConfigErr    error
		addVolumeError        error
		cloneVolumeError      error
		importVolumeError     error
		addVolumeReturns      *storage.VolumeExternal
		cloneVolumeReturns    *storage.VolumeExternal
		importVolumeReturns   *storage.VolumeExternal
		ListBackendsReturns   []*storage.BackendExternal
		ListBackendsError     error
	}{
		{
			name: "Success - New  volume",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: volSize,
					VolumeId:      "test-volume-id",
					VolumeContext: map[string]string{
						"fsType":       "ext4",
						"storageClass": "standard",
					},
				},
			},
			expErrCode:       codes.OK,
			getVolumeReturns: nil,
			getVolumeError:   errors.NotFoundError("volume not found"),
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
			ListBackendsError:     nil,
			addVolumeReturns:      createMockVolume(volName, "test-volume-id", volSize),
			addVolumeError:        nil,
			getVolumeConfigReturn: createMockVolumeConfig(volName, volSize),
			getVolumeConfigErr:    nil,
		},
		{
			name: "Success - New block volume",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdBlockVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: volSize,
					VolumeId:      "test-volume-id",
					VolumeContext: map[string]string{
						"fsType":       "ext4",
						"storageClass": "standard",
					},
				},
			},
			expErrCode:       codes.OK,
			getVolumeReturns: nil,
			getVolumeError:   errors.NotFoundError("volume not found"),
			addVolumeReturns: createMockVolume(volName, "test-volume-id", volSize),
			addVolumeError:   nil,
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
			ListBackendsError:     nil,
			getVolumeConfigReturn: createMockVolumeConfig(volName, volSize),
			getVolumeConfigErr:    nil,
			supportsFeatureReturn: true,
		},
		{
			name: "Error - no available storage for access modes",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdBlockVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: volSize,
					VolumeId:      "test-volume-id",
					VolumeContext: map[string]string{
						"fsType":       "ext4",
						"storageClass": "standard",
					},
				},
			},
			expErrCode:       codes.InvalidArgument,
			getVolumeReturns: nil,
			getVolumeError:   errors.NotFoundError("volume not found"),
			addVolumeReturns: createMockVolume(volName, "test-volume-id", volSize),
			addVolumeError:   nil,
			ListBackendsReturns: []*storage.BackendExternal{{
				Protocol: tridentconfig.Block,
			}},
			ListBackendsError:     nil,
			getVolumeConfigReturn: createMockVolumeConfig(volName, volSize),
			getVolumeConfigErr:    nil,
			supportsFeatureReturn: true,
		},
		{
			name: "Success - Existing volume with matching capacity",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: volSize,
					VolumeId:      "existing-volume-id",
					VolumeContext: map[string]string{
						"fsType":       "ext4",
						"storageClass": "standard",
					},
				},
			},
			expErrCode:       codes.OK,
			getVolumeReturns: createMockVolumeWithSize("existing-volume-id", "existing-volume-id", volSize),
			getVolumeError:   nil,
		},
		{
			name: "failure - raw block volumes are not supported for this container orchestrator",
			req: &csi.CreateVolumeRequest{
				Name:               "test-volume-id",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdRawBlockVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.FailedPrecondition,
			supportsFeatureReturn:  false,
			getVolumeReturns:       nil,
		},
		{
			name: "Success - Clone from volume",
			req: &csi.CreateVolumeRequest{
				Name:               "cloned-volume-id",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "source-volume-id",
						},
					},
				},
			},
			expectedVolumeResponse: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: volSize,
					VolumeId:      "cloned-volume-id",
					VolumeContext: map[string]string{
						"fsType":       "ext4",
						"storageClass": "standard",
					},
					ContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Volume{
							Volume: &csi.VolumeContentSource_VolumeSource{
								VolumeId: "source-volume-id",
							},
						},
					},
				},
			},
			expErrCode:         codes.OK,
			getVolumeReturns:   nil,
			getVolumeError:     errors.NotFoundError("volume not found"),
			cloneVolumeReturns: createMockVolume(volName, "cloned-volume-id", volSize),
			cloneVolumeError:   nil,
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "iscsi-backend",
				BackendUUID: "uuid-iscsi",
				Protocol:    tridentconfig.Block,
			}, {
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
			getVolumeConfigReturn: createMockVolumeConfigWithClone(volName, "source-volume-id", ""),
			getVolumeConfigErr:    nil,
		},
		{
			name: "Success - Clone from snapshot",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "source-volume/snapshot-name",
						},
					},
				},
			},
			expectedVolumeResponse: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: volSize,
					VolumeId:      "cloned-volume-id",
					VolumeContext: map[string]string{
						"fsType":       "ext4",
						"storageClass": "standard",
					},
					ContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: "source-volume/snapshot-name",
							},
						},
					},
				},
			},
			expErrCode:         codes.OK,
			getVolumeReturns:   nil,
			getVolumeError:     errors.NotFoundError("volume not found"),
			cloneVolumeReturns: createMockVolume(volName, "cloned-volume-id", volSize),
			cloneVolumeError:   nil,
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "iscsi-backend",
				BackendUUID: "uuid-iscsi",
				Protocol:    tridentconfig.Block,
			}, {
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
			getVolumeConfigReturn: createMockVolumeConfigWithClone(volName, "source-volume", "snapshot-name"),
			getVolumeConfigErr:    nil,
		},
		{
			name: "Success - Existing volume and volume request snapshot id missing",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-vol",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
			getVolumeReturns:       createMockVolumeWithSize("existing-vol", "existing-vol", volSize), // 1GB,
			getVolumeError:         nil,
		},
		{
			name: "Error - Empty volume name",
			req: &csi.CreateVolumeRequest{
				Name:               "",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
		},
		{
			name: "Error - Missing volume capabilities",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: nil,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
		},
		{
			name: "Error - Mixed block and mount capabilities",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdMixedVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
		},
		{
			name: "Error - Existing volume with different size",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id",
				CapacityRange:      &csi.CapacityRange{RequiredBytes: 2 * 1024 * 1024 * 1024}, // 2GB
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.AlreadyExists,
			getVolumeReturns:       createMockVolumeWithSize("existing-volume-id", "existing-volume-id", volSize), // 1GB
			getVolumeError:         nil,
			getVolumeConfigReturn:  createMockVolumeConfig("existing-volume-id", 2*1024*1024*1024), // Expect 2GB in mock
			getVolumeConfigErr:     nil,
		},
		{
			name: "Error - Clone source volume ID missing",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
			getVolumeReturns:       nil,
			getVolumeError:         errors.NotFoundError("volume not found"),
		},
		{
			name: "Error - Clone source snapshot ID missing",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
			getVolumeReturns:       nil,
			getVolumeError:         errors.NotFoundError("volume not found"),
		},
		{
			name: "Error - Invalid snapshot ID format",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "invalid-snapshot-id",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.NotFound,
			getVolumeReturns:       nil,
			getVolumeError:         errors.NotFoundError("volume not found"),
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
		},
		{
			name: "Error - Invalid req snapshot ID format for existing volume",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id-2",
				CapacityRange:      &csi.CapacityRange{RequiredBytes: 1 * 1024 * 1024 * 1024},
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "invalid-snapshot-id",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
			getVolumeReturns:       createMockVolumeWithSize("existing-volume-id-2", "existing-volume-id-2", 1*1024*1024*1024),
			getVolumeError:         nil,
			getVolumeConfigReturn:  createMockVolumeConfig("existing-volume-id-2", 1*1024*1024*1024),
			getVolumeConfigErr:     nil,
		},
		{
			name: "Error - content source volume ID missing in request",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id-2",
				CapacityRange:      &csi.CapacityRange{RequiredBytes: 1 * 1024 * 1024 * 1024},
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
			getVolumeReturns:       createMockVolumeWithSize("existing-volume-id-2", "existing-volume-id-2", 1*1024*1024*1024),
			getVolumeError:         nil,
			getVolumeConfigReturn:  createMockVolumeConfig("existing-volume-id-2", 1*1024*1024*1024),
			getVolumeConfigErr:     nil,
		},
		{
			name: "Error - volume already exists with different volume source",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id-2",
				CapacityRange:      &csi.CapacityRange{RequiredBytes: 1 * 1024 * 1024 * 1024},
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "existing-volume-id-123",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.AlreadyExists,
			getVolumeReturns:       createMockVolumeWithcloneSource("existing-volume-id-2", "existing-volume-id-2", 1*1024*1024*1024),
			getVolumeError:         nil,
			getVolumeConfigReturn:  createMockVolumeConfig("existing-volume-id-2", 1*1024*1024*1024),
			getVolumeConfigErr:     nil,
		},
		{
			name: "Error - snapshot volume already exists with different snapshot source ",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id-2",
				CapacityRange:      &csi.CapacityRange{RequiredBytes: 1 * 1024 * 1024 * 1024},
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "source-volume/snapshot-name",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.AlreadyExists,
			getVolumeReturns:       createMockVolumeWithcloneSource("existing-volume-id-2", "existing-volume-id-2", 1*1024*1024*1024),
			getVolumeError:         nil,
			getVolumeConfigReturn:  createMockVolumeConfig("existing-volume-id-2", 1*1024*1024*1024),
			getVolumeConfigErr:     nil,
		},
		{
			name: "Error - snapshot volume already exists with different volume source ",
			req: &csi.CreateVolumeRequest{
				Name:               "existing-volume-id-2",
				CapacityRange:      &csi.CapacityRange{RequiredBytes: 1 * 1024 * 1024 * 1024},
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "existing-volume-id-2/snapshot-name",
						},
					},
				},
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.AlreadyExists,
			getVolumeReturns:       createMockVolumeWithcloneSource("existing-volume-id-2", "existing-volume-id-2", 1*1024*1024*1024),
			getVolumeError:         nil,
			getVolumeConfigReturn:  createMockVolumeConfig("existing-volume-id-2", 1*1024*1024*1024),
			getVolumeConfigErr:     nil,
		},
		{
			name: "Error - No backend for protocol",
			req: &csi.CreateVolumeRequest{
				Name:               volName,
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.InvalidArgument,
			getVolumeReturns:       nil,
			getVolumeError:         errors.NotFoundError("volume not found"),
			ListBackendsReturns:    []*storage.BackendExternal{},
		},
		{
			name: "Error - Orchestrator error during volume creation",
			req: &csi.CreateVolumeRequest{
				Name:               "new-vol-1",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.Unknown,
			getVolumeReturns:       nil,
			getVolumeError:         errors.NotFoundError("volume not found"),
			addVolumeReturns:       nil,
			addVolumeError:         errors.New("orchestrator error"),
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "iscsi-backend",
				BackendUUID: "uuid-iscsi",
				Protocol:    tridentconfig.Block,
			}, {
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
			getVolumeConfigReturn: createMockVolumeConfig("new-vol-1", volSize),
			getVolumeConfigErr:    nil,
		},
		{
			name: "Success - get CSIVolume from trident volume",
			req: &csi.CreateVolumeRequest{
				Name:               "new-vol-3",
				CapacityRange:      stdCapacityRange,
				VolumeCapabilities: stdFilesystemVolCap,
				Parameters:         stdParameters,
			},
			expectedVolumeResponse: nil,
			expErrCode:             codes.OK,
			getVolumeReturns:       nil,
			getVolumeError:         errors.NotFoundError("volume not found"),
			addVolumeReturns:       createMockVolume("new-vol-3", "new-vol-3", volSize),
			addVolumeError:         nil,
			ListBackendsReturns: []*storage.BackendExternal{{
				Name:        "iscsi-backend",
				BackendUUID: "uuid-iscsi",
				Protocol:    tridentconfig.Block,
			}, {
				Name:        "nfs-backend",
				BackendUUID: "uuid-nfs",
				Protocol:    tridentconfig.File,
			}},
			getVolumeConfigReturn: createMockVolumeConfig("new-vol-3", volSize),
			getVolumeConfigErr:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.Name).Return(tc.getVolumeReturns, tc.getVolumeError).AnyTimes()

			// Mock SupportsFeature if needed
			if len(tc.req.VolumeCapabilities) > 0 {
				for _, cap := range tc.req.VolumeCapabilities {
					if cap.GetBlock() != nil {
						// Simply use supportsFeatureReturn directly
						mockHelper.EXPECT().SupportsFeature(gomock.Any(), CSIBlockVolumes).Return(tc.supportsFeatureReturn).AnyTimes()
					}
				}
			}
			mockHelper.EXPECT().GetVolumeConfig(
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
			).Return(tc.getVolumeConfigReturn, tc.getVolumeConfigErr).AnyTimes()

			mockOrchestrator.EXPECT().ListBackends(gomock.Any()).Return(tc.ListBackendsReturns, nil).AnyTimes()

			// Mock volume operations
			mockOrchestrator.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(tc.addVolumeReturns, tc.addVolumeError).AnyTimes()

			mockOrchestrator.EXPECT().CloneVolume(gomock.Any(), gomock.Any()).Return(tc.cloneVolumeReturns, tc.cloneVolumeError).AnyTimes()

			mockOrchestrator.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(tc.importVolumeReturns, tc.importVolumeError).AnyTimes()

			mockHelper.EXPECT().RecordVolumeEvent(gomock.Any(), tc.req.Name, v1.EventTypeNormal, "ProvisioningSuccess", "provisioned a volume").AnyTimes()

			// Execute the test
			resp, err := controllerServer.CreateVolume(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}

			// Verify success case
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotNil(t, resp.Volume)

			// Verify volume response
			if tc.expectedVolumeResponse != nil {
				assert.Equal(t, tc.expectedVolumeResponse.Volume.CapacityBytes, resp.Volume.CapacityBytes)
				assert.Equal(t, tc.expectedVolumeResponse.Volume.VolumeId, resp.Volume.VolumeId)

			}
		})
	}
}

// Helper function for creating volume config with clone information
func createMockVolumeConfigWithClone(name, cloneSourceVolume, cloneSourceSnapshot string) *storage.VolumeConfig {
	config := createMockVolumeConfig(name, int64(1024*1024*1024))
	config.CloneSourceVolume = cloneSourceVolume
	config.CloneSourceSnapshot = cloneSourceSnapshot
	return config
}

// Helper functions to create mock objects
func createMockVolume(name, volumeID string, size int64) *storage.VolumeExternal {
	sizeStr := strconv.FormatInt(size, 10)
	return &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:       volumeID,
			Size:       sizeStr,
			InternalID: "test",
		},
		Backend:  "mock-backend",
		Pool:     "mock-pool",
		Orphaned: false,
		State:    storage.VolumeStateOnline,
	}
}

func createMockVolumeWithSize(name, volumeID string, size int64) *storage.VolumeExternal {
	vol := createMockVolume(name, volumeID, size)
	vol.Config.Size = strconv.FormatInt(size, 10)
	return vol
}

func createMockVolumeWithcloneSource(name, volumeID string, size int64) *storage.VolumeExternal {
	vol := createMockVolume(name, volumeID, size)
	vol.Config.Size = strconv.FormatInt(size, 10)
	vol.Config.CloneSourceVolume = name
	vol.Config.CloneSourceSnapshot = name
	return vol
}

func createMockVolumeConfig(name string, size int64) *storage.VolumeConfig {
	return &storage.VolumeConfig{
		Name:                name,
		Size:                fmt.Sprintf("%d", size), // Use the passed size parameter
		Protocol:            tridentconfig.File,
		SpaceReserve:        "none",
		SecurityStyle:       "unix",
		SnapshotPolicy:      "default",
		SnapshotReserve:     "5",
		SnapshotDir:         "false",
		UnixPermissions:     "0755",
		StorageClass:        "standard",
		AccessMode:          tridentconfig.ReadWriteOnce,
		BlockSize:           "",
		FileSystem:          "ext4",
		CloneSourceVolume:   "",
		CloneSourceSnapshot: "",
		ImportOriginalName:  "",
		ImportBackendUUID:   "",
		InternalID:          "test",
	}
}

func TestDeleteVolume(t *testing.T) {
	testCases := []struct {
		name              string
		req               *csi.DeleteVolumeRequest
		expectedResponse  *csi.DeleteVolumeResponse
		expErrCode        codes.Code
		deleteVolumeError error
	}{
		{
			name: "Success - Delete volume",
			req: &csi.DeleteVolumeRequest{
				VolumeId: "vol-id",
			},
			expectedResponse:  &csi.DeleteVolumeResponse{},
			expErrCode:        codes.OK,
			deleteVolumeError: nil,
		},
		{
			name: "Error - Delete volume with empty volumeId",
			req: &csi.DeleteVolumeRequest{
				VolumeId: "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - Deleting volume ",
			req: &csi.DeleteVolumeRequest{
				VolumeId: "vol-id",
			},
			expectedResponse:  nil,
			expErrCode:        codes.Unknown,
			deleteVolumeError: errors.New("some error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			mockOrchestrator.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(tc.deleteVolumeError).AnyTimes()

			resp, err := controllerServer.DeleteVolume(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestControllerPublishVolume(t *testing.T) {
	testCases := []struct {
		name                           string
		req                            *csi.ControllerPublishVolumeRequest
		expectedResponse               *csi.ControllerPublishVolumeResponse
		expErrCode                     codes.Code
		orchestratorGetVolumeError     error
		orchestratorGetNodeError       error
		orchestratorPublishVolumeError error
		publishInfo                    models.VolumePublishInfo
		protocol                       string
		sanType                        string
		filesystemType                 string
	}{
		{
			name: "Success - controller publish volume for smb filesystem type",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			expectedResponse: &csi.ControllerPublishVolumeResponse{PublishContext: map[string]string{"filesystemType": "smb", "formatOptions": "", "mountOptions": "", "smbPath": "", "smbServer": "", "protocol": "file"}},
			publishInfo: models.VolumePublishInfo{
				SANType:        sa.FCP,
				FilesystemType: "smb",
			},
			expErrCode:     codes.OK,
			filesystemType: "smb",
		},
		{
			name: "Success - controller publish volume",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			expectedResponse: &csi.ControllerPublishVolumeResponse{PublishContext: map[string]string{"filesystemType": "", "formatOptions": "", "mountOptions": "", "nfsPath": "", "nfsServerIp": "", "protocol": "file"}},
			expErrCode:       codes.OK,
		},
		{
			name: "Error - no volume id provided",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "",
				NodeId:   "Node-id",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - no node id provided",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - no volume capability provided",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId:         "vol-id",
				NodeId:           "Node-id",
				VolumeCapability: nil,
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Get volume failed with some error",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			expectedResponse:           nil,
			expErrCode:                 codes.Unknown,
			orchestratorGetVolumeError: errors.New("some error"),
		},
		{
			name: "Get node info failed with error not found",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			expectedResponse:         nil,
			expErrCode:               codes.NotFound,
			orchestratorGetNodeError: errors.New("Not found"),
		},
		{
			name: "Error - controller publish failed",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			expectedResponse:               nil,
			expErrCode:                     codes.Unknown,
			orchestratorPublishVolumeError: errors.New("some error"),
		},
		{
			name: "Success - controller publish for NVMe protocol",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"mnt-flag"},
						},
					},
				},
			},
			expectedResponse: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"filesystemType":    "",
					"LUKSEncryption":    "",
					"mountOptions":      "mnt-flag",
					"formatOptions":     "",
					"nvmeNamespaceUUID": "",
					"nvmeSubsystemNqn":  "",
					"protocol":          "block",
					"SANType":           sa.NVMe,
					"sharedTarget":      "false",
					"nvmeTargetIPs":     "",
				},
			},
			expErrCode: codes.OK,
			publishInfo: models.VolumePublishInfo{
				SANType: sa.NVMe,
			},
			protocol: "block",
			sanType:  "NVMe",
		},
		{
			name: "Success - controller publish for Iscsi protocol",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
				},
			},
			expectedResponse: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"filesystemType":         "",
					"iscsiIgroup":            "",
					"iscsiInterface":         "",
					"iscsiLunNumber":         "0",
					"iscsiLunSerial":         "",
					"iscsiTargetIqn":         "",
					"iscsiTargetPortalCount": "1",
					"LUKSEncryption":         "",
					"mountOptions":           "",
					"formatOptions":          "",
					"p1":                     "",
					"protocol":               "block",
					"SANType":                sa.ISCSI,
					"sharedTarget":           "false",
					"useCHAP":                "false",
				},
			},
			expErrCode: codes.OK,
			protocol:   "block",
			sanType:    "iscsi",
		},
		{
			name: "Success - controller publish for FCP protocol",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
				},
			},
			expectedResponse: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"LUKSEncryption": "",
					"SANType":        sa.FCP,
					"fcTargetWWNN":   "",
					"fcpIgroup":      "",
					"fcpLunNumber":   "0",
					"fcpLunSerial":   "",
					"filesystemType": "",
					"formatOptions":  "",
					"mountOptions":   "",
					"protocol":       "block",
					"sharedTarget":   "false",
					"useCHAP":        "false",
				},
			},
			expErrCode: codes.OK,
			publishInfo: models.VolumePublishInfo{
				SANType: sa.FCP,
			},
			protocol: "block",
			sanType:  "FCP",
		},
		{
			name: "Error in controller publish for Iscsi protocol using chap with nil encryption key",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
				},
			},
			expectedResponse: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"LUKSEncryption": "",
					"SANType":        sa.ISCSI,
					"fcTargetWWNN":   "",
					"fcpIgroup":      "",
					"fcpLunNumber":   "0",
					"fcpLunSerial":   "",
					"filesystemType": "",
					"formatOptions":  "",
					"mountOptions":   "",
					"protocol":       "block",
					"sharedTarget":   "false",
					"useCHAP":        "true",
				},
			},
			expErrCode: codes.Internal,
			publishInfo: models.VolumePublishInfo{
				SANType: sa.FCP,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiChapInfo: models.IscsiChapInfo{UseCHAP: true},
					},
				},
			},
			protocol: "block",
			sanType:  "Iscsi",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			getVolumeResp := generateFakeVolumeExternal(tc.req.VolumeId)

			if tc.protocol == "block" {
				getVolumeResp.Config.Protocol = tridentconfig.Block
			}

			mockOrchestrator.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(getVolumeResp, tc.orchestratorGetVolumeError).AnyTimes()

			mockOrchestrator.EXPECT().GetNode(gomock.Any(), gomock.Any()).Return(generateFakeNode(tc.req.NodeId).ConstructExternal(), tc.orchestratorGetNodeError).AnyTimes()

			mockOrchestrator.EXPECT().PublishVolume(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, tc.publishInfo).Return(tc.orchestratorPublishVolumeError).AnyTimes()

			resp, err := controllerServer.ControllerPublishVolume(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tc.expectedResponse.PublishContext, resp.PublishContext)
		})
	}
}

func TestControllerUnPublishVolume(t *testing.T) {
	testCases := []struct {
		name                             string
		req                              *csi.ControllerUnpublishVolumeRequest
		expectedResponse                 *csi.ControllerUnpublishVolumeResponse
		expErrCode                       codes.Code
		GetNodePublicationStateError     error
		orchestratorUpdateNodeError      error
		orchestratorUnpublishVolumeError error
	}{
		{
			name: "Success - controller unpublish volume",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
			},
			expectedResponse: &csi.ControllerUnpublishVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Error - no volume id provided",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "",
				NodeId:   "Node-id",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - no node id provided",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Get node publication failed with some error",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
			},
			expectedResponse:             nil,
			expErrCode:                   codes.Internal,
			GetNodePublicationStateError: errors.New("some error"),
		},
		{
			name: "Sucessful unpublish volume when get node publication failed with node not found error",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
			},
			expectedResponse:             nil,
			expErrCode:                   codes.OK,
			GetNodePublicationStateError: errors.NotFoundError("some error"),
		},
		{
			name: "Error - controller unpublish failed",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
			},
			expectedResponse:                 nil,
			expErrCode:                       codes.Internal,
			orchestratorUnpublishVolumeError: errors.New("some error"),
		},
		{
			name: "ControllerUnpublish, Could not update core with node status",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "vol-id",
				NodeId:   "Node-id",
			},
			expectedResponse:            nil,
			expErrCode:                  codes.Internal,
			orchestratorUpdateNodeError: errors.New("some error"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			mockHelper.EXPECT().GetNodePublicationState(gomock.Any(), gomock.Any()).Return(nil, tc.GetNodePublicationStateError).AnyTimes()

			mockOrchestrator.EXPECT().UpdateNode(gomock.Any(), gomock.Any(), nil).Return(tc.orchestratorUpdateNodeError).AnyTimes()

			mockOrchestrator.EXPECT().UnpublishVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.orchestratorUnpublishVolumeError).AnyTimes()

			resp, err := controllerServer.ControllerUnpublishVolume(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestControllerListSnapshots_CoreListsSnapshots(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshotConfig := &storage.SnapshotConfig{
		Name:       snapshotID,
		VolumeName: volumeID,
	}
	snapshot := storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	mockOrchestrator.EXPECT().ListSnapshots(
		gomock.Any(),
	).Return([]*storage.SnapshotExternal{snapshot.ConstructExternal()}, nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(volume.ConstructExternal(), nil)

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: ""}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp.Entries)
}

func TestControllerListSnapshots_CoreListsSnapshotsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshotConfig := &storage.SnapshotConfig{
		Name:       snapshotID,
		VolumeName: volumeID,
	}
	snapshot := storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	mockOrchestrator.EXPECT().ListSnapshotsForVolume(
		gomock.Any(), volumeID,
	).Return([]*storage.SnapshotExternal{snapshot.ConstructExternal()}, nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(volume.ConstructExternal(), nil)

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: volumeID}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp.Entries)
}

func TestControllerListSnapshots_CoreFailsToFindSnapshotsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"

	mockOrchestrator.EXPECT().ListSnapshotsForVolume(
		gomock.Any(), volumeID,
	).Return(nil, errors.NotFoundError("snapshots not found"))

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: volumeID}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.Entries)
	assert.Empty(t, resp.NextToken)
}

func TestControllerListSnapshots_CoreFailsToListsSnapshotsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"

	mockOrchestrator.EXPECT().ListSnapshotsForVolume(
		gomock.Any(), volumeID,
	).Return(nil, errors.New("core error"))

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: volumeID}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestControllerListSnapshots_SnapshotImport_FailsWithInvalidID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapshotID := "snap-content-01"

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s--%s", volumeID, snapshotID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)

	// CSI spec calls for empty return and error if snapshot is not found.
	assert.Nil(t, resp.Entries)
	assert.NoError(t, err)
}

func TestControllerListSnapshots_SnapshotImport_FailsWithInvalidIDComponents(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap_content-01!"

	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(false)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)

	// CSI spec calls for empty return and error if snapshot is not found.
	assert.Nil(t, resp.Entries)
	assert.NoError(t, err)
}

func TestControllerListSnapshots_SnapshotImport_FailsToGetSnapshots(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"

	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.New("core error"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_FailsToGetSnapshotConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"

	// If a snapshotID isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.NotFoundError("snapshotID not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.New("helper err"))

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_CoreFailsToImportSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapshotID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:       snapshotID,
		VolumeName: volumeID,
	}

	// If a snapshotID isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapshotID,
	).Return(nil, errors.NotFoundError("snapshotID not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapshotID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapshotID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(nil, errors.New("core error"))

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapshotID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_CoreFailsToFindParentVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:         snapContentID,
		VolumeName:   volumeID,
		InternalName: "snap.2023-05-23_175116",
	}
	snapshot := &storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// If a snapshot isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.NotFoundError("snapshot not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapContentID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(snapshot.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(nil, errors.New("core error"))

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Internal, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_CoreFailsToFindSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:         snapContentID,
		VolumeName:   volumeID,
		InternalName: "snap.2023-05-23_175116",
	}
	snapshot := &storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}
	notFoundErr := errors.NotFoundError("snapshot not found")

	// If a snapshot isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(gomock.Any(), volumeID, snapContentID).Return(nil, notFoundErr)
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapContentID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(snapshot.ConstructExternal(), notFoundErr)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)

	// CSI spec calls for empty return and error if snapshot is not found.
	assert.Nil(t, resp.Entries)
	assert.NoError(t, err)
}

func TestControllerListSnapshots_SnapshotImport_Succeeds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapContentID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:         snapContentID,
		VolumeName:   volumeID,
		InternalName: "snap.2023-05-23_175116",
	}
	snapshot := &storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// If a snapshot isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.NotFoundError("snapshot not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapContentID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(snapshot.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(volume.ConstructExternal(), nil)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp.Entries)

	csiSnapshot := resp.Entries[0]
	assert.NotNil(t, csiSnapshot)
	assert.Equal(t, snapshot.ID(), csiSnapshot.GetSnapshot().GetSnapshotId())
}

func TestControllerGetListSnapshots_FailsWhenRequestIsInvalid(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	var snapshots []*storage.SnapshotExternal

	fakeReq := &csi.ListSnapshotsRequest{MaxEntries: -1}
	snapshotResp, err := controllerServer.getListSnapshots(ctx, fakeReq, snapshots)
	assert.Error(t, err)

	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, status.Code())
	assert.Nil(t, snapshotResp)
}

func TestControllerGetListSnapshots_AddsSnapshotToCSISnapshotResponse(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshots := []*storage.SnapshotExternal{
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       snapshotID,
					VolumeName: volumeID,
				},
			},
		},
	}

	mockOrchestrator.EXPECT().GetVolume(
		gomock.Any(), volumeID,
	).Return(volume.ConstructExternal(), nil).Times(len(snapshots))

	fakeReq := &csi.ListSnapshotsRequest{MaxEntries: int32(len(snapshots) + 1), StartingToken: snapshots[0].ID()}
	listSnapshotResp, err := controllerServer.getListSnapshots(ctx, fakeReq, snapshots)
	assert.NoError(t, err)
	assert.NotNil(t, listSnapshotResp)

	entries := listSnapshotResp.GetEntries()
	assert.NotEmpty(t, entries)

	csiSnapshot := entries[0].GetSnapshot()
	assert.NotNil(t, csiSnapshot)
	assert.Equal(t, snapshots[0].ID(), csiSnapshot.GetSnapshotId())
}

func TestControllerGetListSnapshots_DoesNotExceedMaxEntries(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshots := []*storage.SnapshotExternal{
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       snapshotID,
					VolumeName: volumeID,
				},
			},
		},
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       snapshotID,
					VolumeName: volumeID,
				},
			},
		},
	}

	mockOrchestrator.EXPECT().GetVolume(
		gomock.Any(), volumeID,
	).Return(volume.ConstructExternal(), nil).Times(len(snapshots))

	fakeReq := &csi.ListSnapshotsRequest{MaxEntries: 0, StartingToken: ""}
	listSnapshotResp, err := controllerServer.getListSnapshots(ctx, fakeReq, snapshots)
	assert.NoError(t, err)
	assert.NotNil(t, listSnapshotResp)

	entries := listSnapshotResp.GetEntries()
	assert.NotEmpty(t, entries)
	assert.NotEqual(t, math.MaxInt16, len(entries))
}

func TestCreateSnapshot(t *testing.T) {
	testCases := []struct {
		name                                 string
		req                                  *csi.CreateSnapshotRequest
		expectedResponse                     *csi.CreateSnapshotResponse
		expErrCode                           codes.Code
		orchestratorListSnapshotsByNameError error
		orchestratorGetSnapshotError         error
		orchestratorGetSnapshotReturns       *storage.SnapshotExternal
		getSnapshotConfigForCreateError      error
		orchestratorCreateSnapshotError      error
		orchestratorCreateSnapshotReturns    *storage.SnapshotExternal
		listSnapshotByNameReturns            []*storage.SnapshotExternal
	}{
		{
			name: "Success - Create snapshot",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse: &csi.CreateSnapshotResponse{},
			expErrCode:       codes.OK,
			orchestratorCreateSnapshotReturns: storage.NewSnapshot(&storage.SnapshotConfig{
				Version:             "1",
				Name:                "test-snapshot",
				InternalName:        "test-snapshot-internal",
				VolumeName:          "test-volume",
				VolumeInternalName:  "test-volume-internal",
				LUKSPassphraseNames: []string{"passphrase1", "passphrase2"},
			}, "2023-10-01T00:00:00Z", 1024, "online").ConstructExternal(),
		},
		{
			name: "Error - Create snapshot with empty source volumeId",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error -  Create snapshot with empty snapshot name",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Success -  snapshot already exists, return the same",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse: &csi.CreateSnapshotResponse{Snapshot: &csi.Snapshot{}},
			expErrCode:       codes.OK,
			orchestratorGetSnapshotReturns: storage.NewSnapshot(&storage.SnapshotConfig{
				Version:             "1",
				Name:                "test-snapshot",
				InternalName:        "test-snapshot-internal",
				VolumeName:          "test-volume",
				VolumeInternalName:  "test-volume-internal",
				LUKSPassphraseNames: []string{"passphrase1", "passphrase2"},
			}, "2023-10-01T00:00:00Z", 1024, "online").ConstructExternal(),
		},
		{
			name: "Error while checking if snapshot exists",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:             nil,
			expErrCode:                   codes.Unknown,
			orchestratorGetSnapshotError: errors.New("some error"),
		},
		{
			name: "Error while checking pre-existing snapshot with the same name on a different volume",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:                     nil,
			expErrCode:                           codes.Unknown,
			orchestratorListSnapshotsByNameError: errors.New("some error"),
		},
		{
			name: "Existing snapshot with the same name on a different volume found",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse: nil,
			expErrCode:       codes.AlreadyExists,
			listSnapshotByNameReturns: []*storage.SnapshotExternal{
				storage.NewSnapshot(&storage.SnapshotConfig{
					Version:             "1",
					Name:                "test-snapshot",
					InternalName:        "test-snapshot-internal",
					VolumeName:          "test-volume",
					VolumeInternalName:  "test-volume-internal",
					LUKSPassphraseNames: []string{"passphrase1", "passphrase2"},
				}, "2023-10-01T00:00:00Z", 1024, "online").ConstructExternal(),
			},
		},
		{
			name: "Error in creating trident snapshot config from request",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:                nil,
			expErrCode:                      codes.Unknown,
			getSnapshotConfigForCreateError: errors.New("some error"),
			orchestratorCreateSnapshotError: errors.New("some error"),
		},
		{
			name: "Error in creating snapshot",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:                nil,
			expErrCode:                      codes.Internal,
			orchestratorCreateSnapshotError: errors.New("some error"),
		},
		{
			name: "Error NotFoundError in create snapshot ",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:                nil,
			expErrCode:                      codes.NotFound,
			orchestratorCreateSnapshotError: errors.NotFoundError("some error"),
		},
		{
			name: "Error UnsupportedError in create snapshot",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:                nil,
			expErrCode:                      codes.FailedPrecondition,
			orchestratorCreateSnapshotError: errors.UnsupportedError(""),
		},
		{
			name: "Error MaxLimitReachedError in create snapshot",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "test-volume",
				Name:           "test-snapshot",
			},
			expectedResponse:                nil,
			expErrCode:                      codes.ResourceExhausted,
			orchestratorCreateSnapshotError: errors.MaxLimitReachedError(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			mockOrchestrator.EXPECT().ListSnapshotsByName(gomock.Any(), gomock.Any()).Return(tc.listSnapshotByNameReturns, tc.orchestratorListSnapshotsByNameError).AnyTimes()

			mockOrchestrator.EXPECT().GetSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.orchestratorGetSnapshotReturns, tc.orchestratorGetSnapshotError).AnyTimes()

			mockHelper.EXPECT().GetSnapshotConfigForCreate(gomock.Any(), gomock.Any()).Return(nil, tc.getSnapshotConfigForCreateError).AnyTimes()

			mockOrchestrator.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(createMockVolume("test-volume", "test-volume", 1*1024*1024*1024), nil).AnyTimes()

			mockHelper.EXPECT().RecordVolumeEvent(gomock.Any(), tc.req.Name, v1.EventTypeNormal, "ProvisioningFailed", "some error").AnyTimes()

			mockOrchestrator.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any()).Return(tc.orchestratorCreateSnapshotReturns, tc.orchestratorCreateSnapshotError).AnyTimes()

			resp, err := controllerServer.CreateSnapshot(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	testCases := []struct {
		name                string
		req                 *csi.DeleteSnapshotRequest
		expectedResponse    *csi.DeleteSnapshotResponse
		expErrCode          codes.Code
		deleteSnapshotError error
		// deleteSnapshotNotFoundError bool
	}{
		{
			name: "Success - Delete snapshot",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "volume-1/snapshot-1",
			},
			expectedResponse: &csi.DeleteSnapshotResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Error - No snapshot ID provided",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - Invalid snapshot ID",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "invalid-snapshot-id",
			},
			expectedResponse: &csi.DeleteSnapshotResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Error - Snapshot not found",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "volume-1/snapshot-1",
			},
			expectedResponse:    &csi.DeleteSnapshotResponse{},
			expErrCode:          codes.OK,
			deleteSnapshotError: errors.NotFoundError(""),
		},
		{
			name: "Error - Delete snapshot failed",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "volume-1/snapshot-1",
			},
			expectedResponse:    nil,
			expErrCode:          codes.Unknown,
			deleteSnapshotError: errors.New("some error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			mockOrchestrator.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.deleteSnapshotError).AnyTimes()

			resp, err := controllerServer.DeleteSnapshot(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestControllerExpandVolume(t *testing.T) {
	testCases := []struct {
		name                   string
		req                    *csi.ControllerExpandVolumeRequest
		expectedResponse       *csi.ControllerExpandVolumeResponse
		expErrCode             codes.Code
		getVolumeError         bool
		volumeSize             int64
		resizeVolumeError      bool
		getResizedVolumeError  bool
		invalidVolumeSizeError bool
	}{
		{
			name: "Success - Volume already at required size",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 1024 * 1024 * 1024, // 1GB
				},
			},
			expectedResponse: &csi.ControllerExpandVolumeResponse{
				CapacityBytes:         1024 * 1024 * 1024, // 1GB
				NodeExpansionRequired: false,
			},
			expErrCode: codes.OK,
			volumeSize: 1024 * 1024 * 1024, // 1GB
		},
		{
			name: "Success - Volume resize required and resized successfully",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024, // 2GB
				},
			},
			expectedResponse: &csi.ControllerExpandVolumeResponse{
				CapacityBytes:         2 * 1024 * 1024 * 1024, // 2GB
				NodeExpansionRequired: false,
			},
			expErrCode: codes.OK,
			volumeSize: 1 * 1024 * 1024 * 1024, // 1GB
		},
		{
			name: "Error - No volume ID provided",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 1024 * 1024 * 1024, // 1GB
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - No capacity range provided",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId:      "test-volume",
				CapacityRange: nil,
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - LimitBytes smaller than RequiredBytes",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024, // 2GB
					LimitBytes:    1 * 1024 * 1024 * 1024, // 1GB
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - Volume not found",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 1024 * 1024 * 1024, // 1GB
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.NotFound,
			getVolumeError:   true,
		},
		{
			name: "Error - Resize volume failed",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024, // 2GB
				},
			},
			expectedResponse:  nil,
			expErrCode:        codes.Unknown,
			volumeSize:        1 * 1024 * 1024 * 1024, // 1GB
			resizeVolumeError: true,
		},
		{
			name: "Error - Getting resized volume failed",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024, // 2GB
				},
			},
			expectedResponse:      nil,
			expErrCode:            codes.Unknown,
			volumeSize:            1 * 1024 * 1024 * 1024, // 1GB
			getResizedVolumeError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create an instance of Plugin for this test
			plugin := &Plugin{
				orchestrator: mockOrchestrator,
			}

			volumeConfig := &storage.VolumeConfig{
				Name:     tc.req.VolumeId,
				Size:     strconv.FormatInt(tc.volumeSize, 10),
				Protocol: tridentconfig.File,
			}

			switch tc.name {
			case "Success - Volume resize required and resized successfully":
				gomock.InOrder(
					// First GetVolume returns the old size
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).
						Return(&storage.VolumeExternal{Config: volumeConfig}, nil),
					// ResizeVolume succeeds
					mockOrchestrator.EXPECT().ResizeVolume(gomock.Any(), tc.req.VolumeId, gomock.Any()).
						Return(nil),
					// Second GetVolume returns the new size
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).
						Return(&storage.VolumeExternal{Config: &storage.VolumeConfig{
							Size: strconv.FormatInt(tc.req.CapacityRange.RequiredBytes, 10),
						}}, nil),
				)
			case "Error - Getting resized volume failed":
				gomock.InOrder(
					// First GetVolume returns the old size
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).
						Return(&storage.VolumeExternal{Config: volumeConfig}, nil),
					// ResizeVolume succeeds
					mockOrchestrator.EXPECT().ResizeVolume(gomock.Any(), tc.req.VolumeId, gomock.Any()).
						Return(nil),
					// Second GetVolume returns an error
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).
						Return(nil, errors.New("get resized volume error")),
				)
			default:
				if tc.req.VolumeId != "" && tc.req.CapacityRange != nil && !tc.getVolumeError {
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).Return(&storage.VolumeExternal{Config: volumeConfig}, nil).AnyTimes()
				} else if tc.getVolumeError {
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).Return(nil, errors.NotFoundError("volume not found")).AnyTimes()
				}

				if tc.req.VolumeId != "" && tc.req.CapacityRange != nil && !tc.getVolumeError && !tc.resizeVolumeError && !tc.getResizedVolumeError && !tc.invalidVolumeSizeError {
					mockOrchestrator.EXPECT().ResizeVolume(gomock.Any(), tc.req.VolumeId, gomock.Any()).Return(nil).AnyTimes()
					mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).Return(&storage.VolumeExternal{Config: &storage.VolumeConfig{Size: strconv.FormatInt(tc.req.CapacityRange.RequiredBytes, 10)}}, nil).AnyTimes()
				} else if tc.resizeVolumeError {
					mockOrchestrator.EXPECT().ResizeVolume(gomock.Any(), tc.req.VolumeId, gomock.Any()).Return(errors.New("resize volume error")).AnyTimes()
				}
			}

			resp, err := plugin.ControllerExpandVolume(ctx, tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tc.expectedResponse.CapacityBytes, resp.CapacityBytes)
			assert.Equal(t, tc.expectedResponse.NodeExpansionRequired, resp.NodeExpansionRequired)
		})
	}
}

func TestStashIscsiTargetPortals(t *testing.T) {
	testCases := []struct {
		name                string
		publishInfo         map[string]string
		volumePublishInfo   *models.VolumePublishInfo
		expectedPublishInfo map[string]string
	}{
		{
			name:        "Single iSCSI target portal",
			publishInfo: make(map[string]string),
			volumePublishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "10.0.0.1:3260",
						IscsiPortals:      []string{},
					},
				},
			},
			expectedPublishInfo: map[string]string{
				"iscsiTargetPortalCount": "1",
				"p1":                     "10.0.0.1:3260",
			},
		},
		{
			name:        "Multiple iSCSI target portals",
			publishInfo: make(map[string]string),
			volumePublishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "10.0.0.1:3260",
						IscsiPortals:      []string{"10.0.0.2:3260", "10.0.0.3:3260"},
					},
				},
			},
			expectedPublishInfo: map[string]string{
				"iscsiTargetPortalCount": "3",
				"p1":                     "10.0.0.1:3260",
				"p2":                     "10.0.0.2:3260",
				"p3":                     "10.0.0.3:3260",
			},
		},
		{
			name:        "No iSCSI target portals",
			publishInfo: make(map[string]string),
			volumePublishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "",
						IscsiPortals:      []string{},
					},
				},
			},
			expectedPublishInfo: map[string]string{
				"iscsiTargetPortalCount": "1",
				"p1":                     "",
			},
		},
		{
			name:        "Nil volumePublishInfo",
			publishInfo: make(map[string]string),
			volumePublishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "",
						IscsiPortals:      nil,
					},
				},
			},
			expectedPublishInfo: map[string]string{
				"iscsiTargetPortalCount": "1",
				"p1":                     "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stashIscsiTargetPortals(tc.publishInfo, tc.volumePublishInfo)

			assert.Equal(t, tc.expectedPublishInfo, tc.publishInfo)
		})
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	testCases := []struct {
		name             string
		req              *csi.ValidateVolumeCapabilitiesRequest
		expectedResponse *csi.ValidateVolumeCapabilitiesResponse
		expErrCode       codes.Code
		getVolumeError   bool
		volumeConfig     *storage.VolumeConfig
	}{
		{
			name: "Success - Valid volume capabilities",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
					},
				},
			},
			expectedResponse: &csi.ValidateVolumeCapabilitiesResponse{
				Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
							},
							AccessType: &csi.VolumeCapability_Block{
								Block: &csi.VolumeCapability_BlockVolume{},
							},
						},
					},
				},
			},
			expErrCode: codes.OK,
			volumeConfig: &storage.VolumeConfig{
				AccessMode: tridentconfig.ReadWriteOnce,
				Protocol:   tridentconfig.Block,
			},
		},
		{
			name: "Error - No volume ID provided",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
					},
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - No volume capabilities provided",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "test-volume",
				VolumeCapabilities: nil,
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Error - Volume not found",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
					},
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.NotFound,
			getVolumeError:   true,
		},
		{
			name: "Error - Access mode mismatch",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
						},
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
					},
				},
			},
			expectedResponse: &csi.ValidateVolumeCapabilitiesResponse{
				Message: "Could not satisfy one or more access modes.",
			},
			expErrCode: codes.OK,
			volumeConfig: &storage.VolumeConfig{
				AccessMode: tridentconfig.ReadWriteOnce,
				Protocol:   tridentconfig.Block,
			},
		},
		{
			name: "Error - Protocol mismatch (block)",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
					},
				},
			},
			expectedResponse: &csi.ValidateVolumeCapabilitiesResponse{
				Message: "Could not satisfy block protocol.",
			},
			expErrCode: codes.OK,
			volumeConfig: &storage.VolumeConfig{
				AccessMode: tridentconfig.ReadWriteOnce,
				Protocol:   tridentconfig.File,
			},
		},
		{
			name: "Error - Protocol mismatch (file)",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			expectedResponse: &csi.ValidateVolumeCapabilitiesResponse{
				Message: "Could not satisfy file protocol.",
			},
			expErrCode: codes.OK,
			volumeConfig: &storage.VolumeConfig{
				AccessMode: tridentconfig.ReadWriteOnce,
				Protocol:   tridentconfig.Block,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create an instance of Plugin for this test
			plugin := &Plugin{
				orchestrator: mockOrchestrator,
			}

			if tc.getVolumeError {
				mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).Return(nil, errors.New("volume not found")).AnyTimes()
			} else {
				volume := &storage.VolumeExternal{Config: tc.volumeConfig}
				mockOrchestrator.EXPECT().GetVolume(gomock.Any(), tc.req.VolumeId).Return(volume, nil).AnyTimes()
			}

			resp, err := plugin.ValidateVolumeCapabilities(context.Background(), tc.req)

			// Verify error expectations
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tc.expectedResponse, resp)
		})
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	// Create a mock plugin
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create an instance of Plugin for this test
	plugin := &Plugin{
		orchestrator: mockOrchestrator,
	}
	plugin.csCap = []*csi.ControllerServiceCapability{
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				},
			},
		},
	}

	// Create a context
	ctx := context.Background()

	// Call the ControllerGetCapabilities method
	resp, err := plugin.ControllerGetCapabilities(ctx, &csi.ControllerGetCapabilitiesRequest{})

	// Assert the response and error
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, plugin.csCap, resp.Capabilities)
}

func TestListVolumes(t *testing.T) {
	volumeConfig := &storage.VolumeConfig{
		Name:       "vol1",
		AccessMode: tridentconfig.ReadWriteOnce,
		Protocol:   tridentconfig.Block,
	}
	volumeConfig2 := &storage.VolumeConfig{
		Name:       "vol2",
		AccessMode: tridentconfig.ReadWriteOnce,
		Protocol:   tridentconfig.Block,
	}

	testCases := []struct {
		name                                   string
		req                                    *csi.ListVolumesRequest
		expectedResponse                       *csi.ListVolumesResponse
		expErrCode                             codes.Code
		getVolumeError                         error
		listVolumeError                        error
		listVolumePublicationsForVolumeError   error
		getVolumeReturns                       *storage.VolumeExternal
		listVolumeReturns                      []*storage.VolumeExternal
		listVolumePublicationsForVolumeReturns []*models.VolumePublicationExternal
		getCSIVolumeFromTridentVolumeError     error
	}{
		{
			name: "volume named same as starting-token exists",
			req: &csi.ListVolumesRequest{
				MaxEntries:    1,
				StartingToken: "vol1",
			},
			getVolumeError:                         nil,
			getVolumeReturns:                       &storage.VolumeExternal{Config: volumeConfig},
			expErrCode:                             codes.OK,
			listVolumeError:                        nil,
			listVolumeReturns:                      []*storage.VolumeExternal{{Config: volumeConfig}},
			listVolumePublicationsForVolumeReturns: []*models.VolumePublicationExternal{},
			expectedResponse: &csi.ListVolumesResponse{
				Entries: []*csi.ListVolumesResponse_Entry{
					{
						Volume: &csi.Volume{
							VolumeId: "vol1",
							VolumeContext: map[string]string{
								"backendUUID":  "",
								"internalName": "",
								"name":         "vol1",
								"protocol":     "block",
							},
							AccessibleTopology: []*csi.Topology{},
						},
						Status: &csi.ListVolumesResponse_VolumeStatus{
							PublishedNodeIds: []string{},
						},
					},
				},
				NextToken: "",
			},
		},
		{
			name: "Error while checking volume named same as starting-token exists or not",
			req: &csi.ListVolumesRequest{
				MaxEntries:    1,
				StartingToken: "start-token",
			},
			getVolumeError: errors.New("some error"),
			expErrCode:     codes.Unknown,
		},
		{
			name: "volume named same as starting-token doesn't exists",
			req: &csi.ListVolumesRequest{
				MaxEntries:    1,
				StartingToken: "start-token",
			},
			getVolumeError:   nil,
			getVolumeReturns: nil,
			expErrCode:       codes.Aborted,
		},
		{
			name: "Negative MaxEntries returns InvalidArgument",
			req: &csi.ListVolumesRequest{
				MaxEntries: -1,
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Zero MaxEntries returns all volumes",
			req: &csi.ListVolumesRequest{
				MaxEntries: 0,
			},
			getVolumeError:                         nil,
			getVolumeReturns:                       nil,
			expErrCode:                             codes.OK,
			listVolumeError:                        nil,
			listVolumeReturns:                      []*storage.VolumeExternal{{Config: volumeConfig}},
			listVolumePublicationsForVolumeReturns: []*models.VolumePublicationExternal{},
			expectedResponse: &csi.ListVolumesResponse{
				Entries: []*csi.ListVolumesResponse_Entry{
					{
						Volume: &csi.Volume{
							VolumeId: "vol1",
							VolumeContext: map[string]string{
								"backendUUID":  "",
								"internalName": "",
								"name":         "vol1",
								"protocol":     "block",
							},
							AccessibleTopology: []*csi.Topology{},
						},
						Status: &csi.ListVolumesResponse_VolumeStatus{
							PublishedNodeIds: []string{},
						},
					},
				},
				NextToken: "",
			},
		},
		{
			name: "Pagination: more volumes than MaxEntries",
			req: &csi.ListVolumesRequest{
				MaxEntries: 1,
			},
			getVolumeError:   nil,
			getVolumeReturns: nil,
			expErrCode:       codes.OK,
			listVolumeError:  nil,
			listVolumeReturns: []*storage.VolumeExternal{
				{Config: volumeConfig},
				{Config: volumeConfig2},
			},
			listVolumePublicationsForVolumeReturns: []*models.VolumePublicationExternal{},
			expectedResponse: &csi.ListVolumesResponse{
				Entries: []*csi.ListVolumesResponse_Entry{
					{
						Volume: &csi.Volume{
							VolumeId: "vol1",
							VolumeContext: map[string]string{
								"backendUUID":  "",
								"internalName": "",
								"name":         "vol1",
								"protocol":     "block",
							},
							AccessibleTopology: []*csi.Topology{},
						},
						Status: &csi.ListVolumesResponse_VolumeStatus{
							PublishedNodeIds: []string{},
						},
					},
				},
				NextToken: "vol-2",
			},
		},
		{
			name: "Error from ListVolumes",
			req: &csi.ListVolumesRequest{
				MaxEntries: 1,
			},
			getVolumeError:   nil,
			getVolumeReturns: nil,
			listVolumeError:  errors.New("list error"),
			expErrCode:       codes.Unknown,
		},
		{
			name: "Error from ListVolumePublicationsForVolume",
			req: &csi.ListVolumesRequest{
				MaxEntries: 1,
			},
			getVolumeError:                       nil,
			getVolumeReturns:                     nil,
			listVolumeError:                      nil,
			listVolumeReturns:                    []*storage.VolumeExternal{{Config: volumeConfig}},
			listVolumePublicationsForVolumeError: errors.New("pub error"),
			expErrCode:                           codes.Internal,
		},
		{
			name: "Volume with publications",
			req: &csi.ListVolumesRequest{
				MaxEntries: 1,
			},
			getVolumeError:    nil,
			getVolumeReturns:  nil,
			listVolumeError:   nil,
			listVolumeReturns: []*storage.VolumeExternal{{Config: volumeConfig}},
			listVolumePublicationsForVolumeReturns: []*models.VolumePublicationExternal{
				{NodeName: "node1"},
				{NodeName: "node2"},
			},
			expectedResponse: &csi.ListVolumesResponse{
				Entries: []*csi.ListVolumesResponse_Entry{
					{
						Volume: &csi.Volume{
							VolumeId: "vol1",
							VolumeContext: map[string]string{
								"backendUUID":  "",
								"internalName": "",
								"name":         "vol1",
								"protocol":     "block",
							},
							AccessibleTopology: []*csi.Topology{},
						},
						Status: &csi.ListVolumesResponse_VolumeStatus{
							PublishedNodeIds: []string{"node1", "node2"},
						},
					},
				},
				NextToken: "",
			},
			expErrCode: codes.OK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			plugin := &Plugin{
				orchestrator: mockOrchestrator,
			}

			// Mock GetVolume
			mockOrchestrator.EXPECT().
				GetVolume(gomock.Any(), gomock.Any()).
				Return(tc.getVolumeReturns, tc.getVolumeError).
				AnyTimes()

			// Mock ListVolumes
			mockOrchestrator.EXPECT().
				ListVolumes(gomock.Any()).
				Return(tc.listVolumeReturns, tc.listVolumeError).
				AnyTimes()

			// Mock ListVolumePublicationsForVolume
			mockOrchestrator.EXPECT().
				ListVolumePublicationsForVolume(gomock.Any(), gomock.Any()).
				Return(tc.listVolumePublicationsForVolumeReturns, tc.listVolumePublicationsForVolumeError).
				AnyTimes()

			resp, err := plugin.ListVolumes(context.Background(), tc.req)

			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				assert.Nil(t, resp)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tc.expectedResponse.Entries, resp.Entries)
		})
	}
}

func TestVerifyVolumePublicationIsNew(t *testing.T) {
	ctx := context.Background()
	volName := "vol1"
	nodeName := "node1"
	pub := &models.VolumePublication{
		VolumeName: volName,
		NodeName:   nodeName,
	}

	notFoundErr := errors.NotFoundError("not found")
	genericErr := errors.New("some error")
	foundErrMsg := "this volume is already published to this node with different options"

	testCases := []struct {
		name              string
		getPubReturn      *models.VolumePublication
		getPubErr         error
		inputPub          *models.VolumePublication
		expectErr         bool
		expectErrType     error
		expectErrContains string
	}{
		{
			name:          "Not found error from orchestrator",
			getPubReturn:  nil,
			getPubErr:     notFoundErr,
			inputPub:      pub,
			expectErr:     true,
			expectErrType: notFoundErr,
		},
		{
			name:         "Existing publication matches exactly",
			getPubReturn: pub,
			getPubErr:    nil,
			inputPub:     pub,
			expectErr:    false,
		},
		{
			name: "Existing publication differs",
			getPubReturn: &models.VolumePublication{
				VolumeName: volName,
				NodeName:   nodeName,
				ReadOnly:   true,
			},
			getPubErr:         nil,
			inputPub:          pub,
			expectErr:         true,
			expectErrContains: foundErrMsg,
		},
		{
			name:          "Generic error from orchestrator",
			getPubReturn:  nil,
			getPubErr:     genericErr,
			inputPub:      pub,
			expectErr:     true,
			expectErrType: genericErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			plugin := &Plugin{
				orchestrator: mockOrchestrator,
			}

			mockOrchestrator.EXPECT().
				GetVolumePublication(ctx, tc.inputPub.VolumeName, tc.inputPub.NodeName).
				Return(tc.getPubReturn, tc.getPubErr).
				Times(1)

			err := plugin.verifyVolumePublicationIsNew(ctx, tc.inputPub)

			if tc.expectErr {
				assert.Error(t, err)
				if tc.expectErrType != nil {
					assert.Equal(t, tc.expectErrType, err)
				}
				if tc.expectErrContains != "" {
					assert.Contains(t, err.Error(), tc.expectErrContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetAccessForCSIAccessMode(t *testing.T) {
	plugin := &Plugin{}

	testCases := []struct {
		name     string
		input    csi.VolumeCapability_AccessMode_Mode
		expected tridentconfig.AccessMode
	}{
		{
			name:     "SINGLE_NODE_SINGLE_WRITER",
			input:    csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			expected: tridentconfig.ReadWriteOncePod,
		},
		{
			name:     "SINGLE_NODE_MULTI_WRITER",
			input:    csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			expected: tridentconfig.ReadWriteOnce,
		},
		{
			name:     "SINGLE_NODE_WRITER",
			input:    csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			expected: tridentconfig.ReadWriteOnce,
		},
		{
			name:     "SINGLE_NODE_READER_ONLY",
			input:    csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
			expected: tridentconfig.ReadWriteOnce,
		},
		{
			name:     "MULTI_NODE_READER_ONLY",
			input:    csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			expected: tridentconfig.ReadOnlyMany,
		},
		{
			name:     "MULTI_NODE_SINGLE_WRITER",
			input:    csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			expected: tridentconfig.ReadWriteMany,
		},
		{
			name:     "MULTI_NODE_MULTI_WRITER",
			input:    csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			expected: tridentconfig.ReadWriteMany,
		},
		{
			name:     "Unknown/Default",
			input:    csi.VolumeCapability_AccessMode_UNKNOWN,
			expected: tridentconfig.ModeAny,
		},
		{
			name:     "Unrecognized value",
			input:    csi.VolumeCapability_AccessMode_Mode(999),
			expected: tridentconfig.ModeAny,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := plugin.getAccessForCSIAccessMode(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestControllerUnimplementedMethods(t *testing.T) {
	plugin := &Plugin{}

	testCases := []struct {
		name     string
		callFunc func() (interface{}, error)
	}{
		{
			name: "ControllerGetVolume",
			callFunc: func() (interface{}, error) {
				return plugin.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{})
			},
		},
		{
			name: "ControllerModifyVolume",
			callFunc: func() (interface{}, error) {
				return plugin.ControllerModifyVolume(context.Background(), &csi.ControllerModifyVolumeRequest{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.callFunc()
			assert.Nil(t, resp)
			assert.Error(t, err)
			st, ok := status.FromError(err)
			assert.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestHasBackendForProtocol(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name     string
		protocol tridentconfig.Protocol
		backends []*storage.BackendExternal
		listErr  error
		expected bool
	}{
		{
			name:     "No backends",
			protocol: tridentconfig.Block,
			backends: []*storage.BackendExternal{},
			expected: false,
		},
		{
			name:     "ListBackends returns error",
			protocol: tridentconfig.Block,
			backends: nil,
			listErr:  errors.New("error"),
			expected: false,
		},
		{
			name:     "ProtocolAny always returns true",
			protocol: tridentconfig.ProtocolAny,
			backends: []*storage.BackendExternal{
				{Protocol: tridentconfig.Block},
			},
			expected: true,
		},
		{
			name:     "Backend matches protocol",
			protocol: tridentconfig.Block,
			backends: []*storage.BackendExternal{
				{Protocol: tridentconfig.Block},
			},
			expected: true,
		},
		{
			name:     "Backend has ProtocolAny",
			protocol: tridentconfig.File,
			backends: []*storage.BackendExternal{
				{Protocol: tridentconfig.ProtocolAny},
			},
			expected: true,
		},
		{
			name:     "Backend does not match protocol",
			protocol: tridentconfig.Block,
			backends: []*storage.BackendExternal{
				{Protocol: tridentconfig.File},
			},
			expected: false,
		},
		{
			name:     "Nil backends",
			protocol: tridentconfig.Block,
			backends: nil,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			plugin := &Plugin{orchestrator: mockOrchestrator}

			mockOrchestrator.EXPECT().
				ListBackends(gomock.Any()).
				Return(tc.backends, tc.listErr).
				Times(1)

			result := plugin.hasBackendForProtocol(ctx, tc.protocol)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestContainsMultiNodeAccessMode(t *testing.T) {
	plugin := &Plugin{}

	multiNodeModes := []tridentconfig.AccessMode{
		tridentconfig.ReadWriteMany,
		tridentconfig.ReadOnlyMany,
	}

	testCases := []struct {
		name        string
		accessModes []tridentconfig.AccessMode
		expected    bool
	}{
		{
			name:        "Empty slice",
			accessModes: []tridentconfig.AccessMode{},
			expected:    false,
		},
		{
			name:        "No multi-node access mode",
			accessModes: []tridentconfig.AccessMode{tridentconfig.ReadWriteOnce},
			expected:    false,
		},
		{
			name:        "Contains ReadWriteMany",
			accessModes: []tridentconfig.AccessMode{tridentconfig.ReadWriteOnce, tridentconfig.ReadWriteMany},
			expected:    true,
		},
		{
			name:        "Contains ReadOnlyMany",
			accessModes: []tridentconfig.AccessMode{tridentconfig.ReadOnlyMany},
			expected:    true,
		},
		{
			name:        "Contains both multi-node modes",
			accessModes: multiNodeModes,
			expected:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := plugin.containsMultiNodeAccessMode(tc.accessModes)
			assert.Equal(t, tc.expected, result)
		})
	}
}
