// Copyright 2020 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend/csi/helpers"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
)

const (
	CSIController = "controller"
	CSINode       = "node"
	CSIAllInOne   = "allInOne"
)

type Plugin struct {
	orchestrator core.Orchestrator

	name     string
	nodeName string
	version  string
	endpoint string
	role     string

	unsafeDetach bool

	hostInfo *utils.HostSystem
	nodePrep *utils.NodePrep

	restClient *RestClient
	helper     helpers.HybridPlugin

	aesKey []byte

	grpc NonBlockingGRPCServer

	csCap []*csi.ControllerServiceCapability
	nsCap []*csi.NodeServiceCapability
	vCap  []*csi.VolumeCapability_AccessMode

	opCache sync.Map

	nodeIsRegistered bool
}

func NewControllerPlugin(
	nodeName, endpoint, aesKeyFile string, orchestrator core.Orchestrator, helper *helpers.HybridPlugin,
) (*Plugin, error) {

	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)

	p := &Plugin{
		orchestrator: orchestrator,
		name:         Provisioner,
		nodeName:     nodeName,
		version:      tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:     endpoint,
		role:         CSIController,
		helper:       *helper,
		opCache:      sync.Map{},
	}

	var err error
	p.aesKey, err = ReadAESKey(ctx, aesKeyFile)
	if err != nil {
		return nil, err
	}

	// Define controller capabilities
	p.addControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	})

	// Define volume capabilities
	p.addVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	return p, nil
}

func NewNodePlugin(
	nodeName, endpoint, caCert, clientCert, clientKey, aesKeyFile string, orchestrator core.Orchestrator,
	unsafeDetach, nodePrep bool,
) (*Plugin, error) {

	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)

	p := &Plugin{
		orchestrator: orchestrator,
		name:         Provisioner,
		nodeName:     nodeName,
		version:      tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:     endpoint,
		role:         CSINode,
		unsafeDetach: unsafeDetach,
		opCache:      sync.Map{},
		nodePrep:     &utils.NodePrep{Enabled: nodePrep},
	}

	// Initialize node prep statuses
	p.initNodePrep(ctx)

	p.addNodeServiceCapabilities(
		[]csi.NodeServiceCapability_RPC_Type{
			csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
			csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
			csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		},
	)

	port := os.Getenv("TRIDENT_CSI_SERVICE_PORT")
	if port == "" {
		port = "34571"
	}

	hostname := os.Getenv("TRIDENT_CSI_SERVICE_HOST")
	if hostname == "" {
		hostname = tridentconfig.ServerCertName
	}

	restURL := "https://" + hostname + ":" + port
	var err error
	p.restClient, err = CreateTLSRestClient(restURL, caCert, clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	p.aesKey, err = ReadAESKey(ctx, aesKeyFile)
	if err != nil {
		return nil, err
	}
	// Define volume capabilities
	p.addVolumeCapabilityAccessModes(
		[]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	)

	return p, nil
}

// The NewAllInOnePlugin is required to support the CSI Sanity test suite.
// CSI Sanity expects a single process to respond to controller, node, and
// identity interfaces.
func NewAllInOnePlugin(
	nodeName, endpoint, caCert, clientCert, clientKey, aesKeyFile string,
	orchestrator core.Orchestrator, helper *helpers.HybridPlugin,
	unsafeDetach, nodePrep bool,
) (*Plugin, error) {

	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)

	p := &Plugin{
		orchestrator: orchestrator,
		name:         Provisioner,
		nodeName:     nodeName,
		version:      tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:     endpoint,
		role:         CSIAllInOne,
		unsafeDetach: unsafeDetach,
		helper:       *helper,
		opCache:      sync.Map{},
		nodePrep:     &utils.NodePrep{Enabled: nodePrep},
	}

	// Initialize node prep statuses
	p.initNodePrep(ctx)

	// Define controller capabilities
	p.addControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	})

	p.addNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	})
	port := "34571"
	for _, envVar := range os.Environ() {
		values := strings.Split(envVar, "=")
		if values[0] == "TRIDENT_CSI_SERVICE_PORT" {
			port = values[1]
			break
		}
	}
	restURL := "https://" + tridentconfig.ServerCertName + ":" + port
	var err error
	p.restClient, err = CreateTLSRestClient(restURL, caCert, clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	p.aesKey, err = ReadAESKey(ctx, aesKeyFile)
	if err != nil {
		return nil, err
	}

	// Define volume capabilities
	p.addVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	return p, nil
}

func (p *Plugin) Activate() error {
	go func() {
		ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)
		p.grpc = NewNonBlockingGRPCServer()

		Logc(ctx).Info("Activating CSI frontend.")
		if p.role == CSINode || p.role == CSIAllInOne {
			p.nodeRegisterWithController(ctx, 0) // Retry indefinitely
		}
		p.grpc.Start(p.endpoint, p, p, p)
	}()
	return nil
}

func (p *Plugin) Deactivate() error {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)

	Logc(ctx).Info("Deactivating CSI frontend.")
	p.grpc.GracefulStop()
	return nil
}

func (p *Plugin) GetName() string {
	return string(tridentconfig.ContextCSI)
}

func (p *Plugin) Version() string {
	return tridentconfig.OrchestratorVersion.String()
}

func (p *Plugin) addControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {

	var csCap []*csi.ControllerServiceCapability

	for _, c := range cl {
		log.WithField("capability", c.String()).Info("Enabling controller service capability.")
		csCap = append(csCap, NewControllerServiceCapability(c))
	}

	p.csCap = csCap
}

func (p *Plugin) addNodeServiceCapabilities(cl []csi.NodeServiceCapability_RPC_Type) {

	var nsCap []*csi.NodeServiceCapability

	for _, c := range cl {
		log.WithField("capability", c.String()).Info("Enabling node service capability.")
		nsCap = append(nsCap, NewNodeServiceCapability(c))
	}

	p.nsCap = nsCap
}

func (p *Plugin) addVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) {

	var vCap []*csi.VolumeCapability_AccessMode

	for _, c := range vc {
		log.WithField("mode", c.String()).Info("Enabling volume access mode.")
		vCap = append(vCap, NewVolumeCapabilityAccessMode(c))
	}

	p.vCap = vCap
}

func (p *Plugin) getCSIErrorForOrchestratorError(err error) error {
	if utils.IsNotReadyError(err) {
		return status.Error(codes.Unavailable, err.Error())
	} else if utils.IsBootstrapError(err) {
		return status.Error(codes.FailedPrecondition, err.Error())
	} else if utils.IsNotFoundError(err) {
		return status.Error(codes.NotFound, err.Error())
	} else if ok, errPtr := utils.HasUnsupportedCapacityRangeError(err); ok && errPtr != nil {
		return status.Error(codes.OutOfRange, errPtr.Error())
	} else {
		return status.Error(codes.Unknown, err.Error())
	}
}

func (p *Plugin) initNodePrep(ctx context.Context) {
	if p.nodePrep.Enabled {

		p.nodePrep.NFS = utils.PrepPending
		p.nodePrep.ISCSI = utils.PrepPending

		// Check if the protocols have previously been successfully set up
		breadcrumb, err := p.readNodePrepBreadcrumbFile(ctx)
		if err != nil && !utils.IsNotFoundError(err) {
			Logc(ctx).Warn("Node prep status file was found, but could not be read; ignoring old statuses.")
		} else if err == nil {
			if breadcrumb.NFS != "" {
				if breadcrumb.TridentVersion == tridentconfig.OrchestratorVersion.String() {
					p.nodePrep.NFS = utils.PrepCompleted
				} else {
					p.nodePrep.NFS = utils.PrepOutdated
				}
				p.nodePrep.NFSStatusMessage = breadcrumb.NFS
			}
			if breadcrumb.ISCSI != "" {
				if breadcrumb.TridentVersion == tridentconfig.OrchestratorVersion.String() {
					p.nodePrep.ISCSI = utils.PrepCompleted
				} else {
					p.nodePrep.ISCSI = utils.PrepOutdated
				}
				p.nodePrep.ISCSIStatusMessage = breadcrumb.ISCSI
			}
		}
	}
}

func ReadAESKey(ctx context.Context, aesKeyFile string) ([]byte, error) {
	var aesKey []byte
	var err error

	if "" != aesKeyFile {
		aesKey, err = ioutil.ReadFile(aesKeyFile)
		if err != nil {
			return nil, err
		}
	} else {
		Logc(ctx).Warn("AES encryption key not provided!")
	}
	return aesKey, nil
}

func (p *Plugin) IsReady() bool {
	return p.nodeIsRegistered
}
