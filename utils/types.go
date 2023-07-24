// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/copystructure"

	"github.com/netapp/trident/utils/errors"
)

//go:generate mockgen -destination=../mocks/mock_utils/mock_json_utils.go github.com/netapp/trident/utils JSONReaderWriter
//go:generate mockgen -destination=../mocks/mock_utils/mock_luks/mock_luks.go -package mock_luks github.com/netapp/trident/utils LUKSDeviceInterface

type VolumeAccessInfo struct {
	IscsiAccessInfo
	NVMeAccessInfo
	NfsAccessInfo
	SMBAccessInfo
	NfsBlockAccessInfo
	MountOptions       string `json:"mountOptions,omitempty"`
	PublishEnforcement bool   `json:"publishEnforcement,omitempty"`
	ReadOnly           bool   `json:"readOnly,omitempty"`
	// The access mode values are defined by CSI
	// See https://github.com/container-storage-interface/spec/blob/release-1.5/lib/go/csi/csi.pb.go#L135
	AccessMode int32 `json:"accessMode,omitempty"`
}

type IscsiChapInfo struct {
	UseCHAP              bool   `json:"useCHAP"`
	IscsiUsername        string `json:"iscsiUsername,omitempty"`
	IscsiInitiatorSecret string `json:"iscsiInitiatorSecret,omitempty"`
	IscsiTargetUsername  string `json:"iscsiTargetUsername,omitempty"`
	IscsiTargetSecret    string `json:"iscsiTargetSecret,omitempty"`
}

// String implements the stringer interface and ensures that sensitive fields are redacted before being logged/printed
func (i IscsiChapInfo) String() string {
	return ToStringRedacted(&i,
		[]string{"IscsiUsername", "IscsiInitiatorSecret", "IscsiTargetUsername", "IscsiTargetSecret"}, nil)
}

type IscsiAccessInfo struct {
	IscsiTargetPortal string   `json:"iscsiTargetPortal,omitempty"`
	IscsiPortals      []string `json:"iscsiPortals,omitempty"`
	IscsiTargetIQN    string   `json:"iscsiTargetIqn,omitempty"`
	IscsiLunNumber    int32    `json:"iscsiLunNumber,omitempty"`
	IscsiInterface    string   `json:"iscsiInterface,omitempty"`
	IscsiIgroup       string   `json:"iscsiIgroup,omitempty"`
	IscsiVAGs         []int64  `json:"iscsiVags,omitempty"`
	IscsiLunSerial    string   `json:"iscsiLunSerial,omitempty"`
	IscsiChapInfo
}

type NfsAccessInfo struct {
	NfsServerIP string `json:"nfsServerIp,omitempty"`
	NfsPath     string `json:"nfsPath,omitempty"`
	NfsUniqueID string `json:"nfsUniqueID,omitempty"`
}

type SMBAccessInfo struct {
	SMBServer string `json:"smbServer,omitempty"`
	SMBPath   string `json:"smbPath,omitempty"`
}

type NfsBlockAccessInfo struct {
	SubvolumeName         string `json:"subvolumeName,omitempty"`
	SubvolumeMountOptions string `json:"subvolumeMountOptions,omitempty"`
	NFSMountpoint         string `json:"nfsMountpoint,omitempty"`
}

type NVMeAccessInfo struct {
	NVMeTargetIPs     []string `json:"nvmeTargetIPs,omitempty"`
	NVMeSubsystemNQN  string   `json:"nvmeSubsystemNqn,omitempty"`
	NVMeSubsystemUUID string   `json:"nvmeSubsystemUUID,omitempty"`
	NVMeNamespaceUUID string   `json:"nvmeNamespaceUUID,omitempty"`
}

type VolumePublishInfo struct {
	Localhost         bool     `json:"localhost,omitempty"`
	HostIQN           []string `json:"hostIQN,omitempty"`
	HostNQN           string   `json:"hostNQN,omitempty"`
	HostIP            []string `json:"hostIP,omitempty"`
	BackendUUID       string   `json:"backendUUID,omitempty"`
	Nodes             []*Node  `json:"nodes,omitempty"`
	HostName          string   `json:"hostName,omitempty"`
	FilesystemType    string   `json:"fstype,omitempty"`
	SharedTarget      bool     `json:"sharedTarget,omitempty"`
	DevicePath        string   `json:"devicePath,omitempty"`
	RawDevicePath     string   `json:"rawDevicePath,omitempty"` // NOTE: devicePath was renamed to this 23.01-23.04
	Unmanaged         bool     `json:"unmanaged,omitempty"`
	StagingMountpoint string   `json:"stagingMountpoint,omitempty"` // NOTE: Added in 22.04 release
	TridentUUID       string   `json:"tridentUUID,omitempty"`       // NOTE: Added in 22.07 release
	LUKSEncryption    string   `json:"LUKSEncryption,omitempty"`
	SANType           string   `json:"SANType,omitempty"`
	VolumeAccessInfo
}

type VolumeTrackingPublishInfo struct {
	StagingTargetPath string `json:"stagingTargetPath"`
}

type VolumeTrackingInfo struct {
	VolumePublishInfo
	VolumeTrackingInfoPath string
	StagingTargetPath      string              `json:"stagingTargetPath"`
	PublishedPaths         map[string]struct{} `json:"publishedTargetPaths"`
}

type VolumePublication struct {
	Name       string `json:"name"`
	NodeName   string `json:"node"`
	VolumeName string `json:"volume"`
	ReadOnly   bool   `json:"readOnly"`
	// The access mode values are defined by CSI
	// See https://github.com/container-storage-interface/spec/blob/release-1.5/lib/go/csi/csi.pb.go#L135
	AccessMode int32 `json:"accessMode"`
}

type VolumePublicationExternal struct {
	Name       string `json:"name"`
	NodeName   string `json:"node"`
	VolumeName string `json:"volume"`
	ReadOnly   bool   `json:"readOnly"`
	// The access mode values are defined by CSI
	// See https://github.com/container-storage-interface/spec/blob/release-1.5/lib/go/csi/csi.pb.go#L135
	AccessMode int32 `json:"accessMode"`
}

// Copy returns a new copy of the VolumePublication.
func (v *VolumePublication) Copy() *VolumePublication {
	return &VolumePublication{
		Name:       v.Name,
		NodeName:   v.NodeName,
		VolumeName: v.VolumeName,
		ReadOnly:   v.ReadOnly,
		AccessMode: v.AccessMode,
	}
}

// ConstructExternal returns an externally facing representation of the VolumePublication.
func (v *VolumePublication) ConstructExternal() *VolumePublicationExternal {
	return &VolumePublicationExternal{
		Name:       v.Name,
		NodeName:   v.NodeName,
		VolumeName: v.VolumeName,
		ReadOnly:   v.ReadOnly,
		AccessMode: v.AccessMode,
	}
}

type Node struct {
	Name             string               `json:"name"`
	IQN              string               `json:"iqn,omitempty"`
	NQN              string               `json:"nqn,omitempty"`
	IPs              []string             `json:"ips,omitempty"`
	TopologyLabels   map[string]string    `json:"topologyLabels,omitempty"`
	NodePrep         *NodePrep            `json:"nodePrep,omitempty"`
	HostInfo         *HostSystem          `json:"hostInfo,omitempty"`
	Deleted          bool                 `json:"deleted"`
	PublicationState NodePublicationState `json:"publicationState"`
}

type NodeExternal struct {
	Name             string               `json:"name"`
	IQN              string               `json:"iqn,omitempty"`
	NQN              string               `json:"nqn,omitempty"`
	IPs              []string             `json:"ips,omitempty"`
	TopologyLabels   map[string]string    `json:"topologyLabels,omitempty"`
	NodePrep         *NodePrep            `json:"nodePrep,omitempty"`
	HostInfo         *HostSystem          `json:"hostInfo,omitempty"`
	Deleted          *bool                `json:"deleted,omitempty"`
	PublicationState NodePublicationState `json:"publicationState,omitempty"`
}

func (n *Node) Copy() *Node {
	if clone, err := copystructure.Copy(*n); err != nil {
		return &Node{}
	} else {
		node := clone.(Node)
		return &node
	}
}

// ConstructExternal returns an externally facing representation of the Node.
func (n *Node) ConstructExternal() *NodeExternal {
	node := n.Copy()

	if n.PublicationState == "" {
		node.PublicationState = NodeClean
	}

	return &NodeExternal{
		Name:             node.Name,
		IQN:              node.IQN,
		NQN:              node.NQN,
		IPs:              node.IPs,
		TopologyLabels:   node.TopologyLabels,
		NodePrep:         node.NodePrep,
		HostInfo:         node.HostInfo,
		Deleted:          Ptr(node.Deleted),
		PublicationState: node.PublicationState,
	}
}

type NodePublicationState string

const (
	NodeDirty     = NodePublicationState("dirty")
	NodeCleanable = NodePublicationState("cleanable")
	NodeClean     = NodePublicationState("clean")
)

type NodePublicationStateFlags struct {
	OrchestratorReady  *bool `json:"orchestratorReady,omitempty"`  // Is the node in a state such that volumes may be published?
	AdministratorReady *bool `json:"administratorReady,omitempty"` // Should new publications be allowed?
	ProvisionerReady   *bool `json:"provisionerReady,omitempty"`   // Is publishing volumes to this node safe for the backend?
}

func (f *NodePublicationStateFlags) IsNodeDirty() bool {
	if f.OrchestratorReady == nil || f.AdministratorReady == nil {
		return false
	}

	return !*f.OrchestratorReady && !*f.AdministratorReady
}

func (f *NodePublicationStateFlags) IsNodeCleanable() bool {
	if f.OrchestratorReady == nil || f.AdministratorReady == nil {
		return false
	}

	return *f.OrchestratorReady && *f.AdministratorReady
}

func (f *NodePublicationStateFlags) IsNodeCleaned() bool {
	if f.OrchestratorReady == nil || f.AdministratorReady == nil || f.ProvisionerReady == nil {
		return false
	}

	return *f.ProvisionerReady && *f.OrchestratorReady && *f.AdministratorReady
}

func (f *NodePublicationStateFlags) String() string {
	return fmt.Sprintf("OrchestratorReady: %s; AdministratorReady: %s; ProvisionerReady: %s",
		PtrToString(f.OrchestratorReady), PtrToString(f.AdministratorReady),
		PtrToString(f.ProvisionerReady))
}

// NodePrep struct is deprecated and only here for backwards compatibility
type NodePrep struct {
	Enabled            bool           `json:"enabled"`
	NFS                NodePrepStatus `json:"nfs,omitempty"`
	NFSStatusMessage   string         `json:"nfsStatusMessage,omitempty"`
	ISCSI              NodePrepStatus `json:"iscsi,omitempty"`
	ISCSIStatusMessage string         `json:"iscsiStatusMessage,omitempty"`
}

type NodePrepStatus string

type HostSystem struct {
	OS       SystemOS `json:"os"`
	Services []string `json:"services,omitempty"`
}

type SystemOS struct {
	Distro  string `json:"distro"` // ubuntu/centos/rhel/windows
	Version string `json:"version"`
	Release string `json:"release,omitempty"`
}

type NodePrepBreadcrumb struct {
	TridentVersion string `json:"tridentVersion"`
	NFS            string `json:"nfs,omitempty"`
	ISCSI          string `json:"iscsi,omitempty"`
}

type JSONReaderWriter interface {
	WriteJSONFile(ctx context.Context, fileContents interface{}, filepath, fileDescription string) error
	ReadJSONFile(ctx context.Context, fileContents interface{}, filepath, fileDescription string) error
}

type LUKSDeviceInterface interface {
	RawDevicePath() string
	MappedDevicePath() string
	MappedDeviceName() string

	IsLUKSFormatted(ctx context.Context) (bool, error)
	IsOpen(ctx context.Context) (bool, error)

	Open(ctx context.Context, luksPassphrase string) error
	LUKSFormat(ctx context.Context, luksPassphrase string) error
	EnsureFormattedAndOpen(ctx context.Context, luksPassphrase string) (bool, error)
	RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error
	CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error)
	Resize(ctx context.Context, luksPassphrase string) error
}

type LUKSDevice struct {
	rawDevicePath string
	mappingName   string
}

// Data structure and related interfaces to help iSCSI self-healing

// PortalInvalid is a data structure for iSCSI self-healing capturing Portal's invalid non-recoverable state
type PortalInvalid int8

const (
	NotInvalid PortalInvalid = iota
	MissingTargetIQN
	MissingMpathDevice
	DuplicatePortals
)

func (a PortalInvalid) String() string {
	switch a {
	case NotInvalid:
		return "not invalid"
	case MissingTargetIQN:
		return "missing target IQN"
	case MissingMpathDevice:
		return "missing multipath device"
	case DuplicatePortals:
		return "duplicate portals found"
	}

	return "unknown"
}

// ISCSIAction is a data structure for iSCSI self-healing actions
type ISCSIAction int8

const (
	NoAction ISCSIAction = iota
	Scan
	LoginScan
	LogoutLoginScan
)

func (a ISCSIAction) String() string {
	switch a {
	case NoAction:
		return "no action"
	case Scan:
		return "LUN scanning"
	case LoginScan:
		return "login and LUN scanning"
	case LogoutLoginScan:
		return "re-login and LUN scanning"
	}

	return "unknown"
}

/*
	Sessions {
		Info (map)
			Key: Portal (string)
			Value: <PortalInfo, LUNs, Remediation (ISCSIAction)>

	}

	PortalInfo {
		ISCSITargetIQN (string)
		Credentials (IscsiChapInfo) {
			UseCHAP	(bool)
			IscsiUsername
			IscsiInitiatorSecret
			IscsiTargetUsername
			IscsiTargetSecret (string)
		}
		SessionNumber (string)
		ReasonInvalid (PortalInvalid)
		LastAccessTime (time.Time)
		FirstIdentifiedStaleAt (time.Time)
		Source (string)
	}

	LUNs {
		Info (map)
			key: LUN (int32)
			value: VolumeID (string)
	}

e.g.
(Published Sessions)
[10.193.156.185]:
        PortalInfo: {ISCSITargetIQN:iqn.1992-08.com.netapp:sn.something1:vs.17 SessionNumber: Credentials:UseCHAP:true IscsiUsername:<REDACTED> IscsiInitiatorSecret:<REDACTED> IscsiTargetUsername:<REDACTED> IscsiTargetSecret:<REDACTED>  ReasonInvalid:not invalid LastAccessTime:2023-01-04 08:40:58.620794241 -0500 EST m=+1080.812296147 FirstIdentifiedStaleAt:0001-01-01 00:00:00 +0000 UTC Source:nodeStage}
        LUNInfo: {Info:map[0:pvc-be27afff-0054-4672-b796-0329fa870462]}

(Current Sessions)
[Portal: 10.193.156.185]:
         PortalInfo: {ISCSITargetIQN:iqn.1992-08.com.netapp:sn.something1:vs.17 SessionNumber:40 Credentials:UseCHAP:true IscsiUsername:<REDACTED> IscsiInitiatorSecret:<REDACTED> IscsiTargetUsername:<REDACTED> IscsiTargetSecret:<REDACTED>  ReasonInvalid:not invalid LastAccessTime:0001-01-01 00:00:00 +0000 UTC FirstIdentifiedStaleAt:0001-01-01 00:00:00 +0000 UTC Source:currentStatus}
        LUNInfo: {Info:map[0:]}
*/

type LUNData struct {
	LUN   int32
	VolID string
}

type LUNs struct {
	Info map[int32]string
}

// AddLUN adds a LUN to LUNs map
func (l *LUNs) AddLUN(m LUNData) {
	if l.Info == nil {
		l.Info = make(map[int32]string)
	}

	l.Info[m.LUN] = m.VolID
}

// RemoveLUN removes a given LUN from LUNs map
func (l *LUNs) RemoveLUN(x int32) {
	delete(l.Info, x)
}

// IsEmpty verifies whether there are any LUN entries in the map
func (l *LUNs) IsEmpty() bool {
	if l == nil || len(l.Info) == 0 {
		return true
	}

	return false
}

// AllLUNs returns a list of LUN Numbers.
func (l *LUNs) AllLUNs() []int32 {
	luns := make([]int32, 0, len(l.Info))
	for k := range l.Info {
		luns = append(luns, k)
	}

	return luns
}

// CheckLUNExists verifies whether the given LUN exists in LUNs map
func (l *LUNs) CheckLUNExists(x int32) bool {
	if _, ok := l.Info[x]; ok {
		return true
	}
	return false
}

// IdentifyMissingLUNs returns the missing LUNs i.e., existing in m but not in l.
func (l *LUNs) IdentifyMissingLUNs(m LUNs) []int32 {
	if l == nil || len(l.Info) == 0 {
		return m.AllLUNs()
	}

	if len(m.Info) == 0 {
		return []int32{}
	}

	var luns []int32
	for k := range m.Info {
		if !l.CheckLUNExists(k) {
			luns = append(luns, k)
		}
	}

	return luns
}

// VolumeID returns volume ID associated with LUN
func (l *LUNs) VolumeID(x int32) (string, error) {
	if volId, ok := l.Info[x]; ok {
		return volId, nil
	}

	return "", errors.NotFoundError("LUN not found")
}

func (l *LUNs) String() string {
	return fmt.Sprintf("LUNs: %v", l.AllLUNs())
}

type PortalInfo struct {
	ISCSITargetIQN         string
	SessionNumber          string // Should only be set when capturing current state of the Portal
	Credentials            IscsiChapInfo
	ReasonInvalid          PortalInvalid // A portal may be in an invalid state, if so then why?
	LastAccessTime         time.Time     // Time at which there was an attempt to self-heal session
	FirstIdentifiedStaleAt time.Time     // Time at which session was first identified to be stale
	Source                 string        // Source of the portal Info
}

// IsValid verifies if PortalInfo is valid or not
func (p *PortalInfo) IsValid() bool {
	if p.ReasonInvalid != NotInvalid {
		return false
	}
	return true
}

// HasTargetIQN verifies if target IQN is set or not
func (p *PortalInfo) HasTargetIQN() bool {
	if p.ISCSITargetIQN != "" {
		return true
	}
	return false
}

// CHAPInUse identifies whether CHAP is set or not
func (p *PortalInfo) CHAPInUse() bool {
	return p.Credentials.UseCHAP
}

// UpdateCHAPCredentials updates Portal's CHAP credentials
func (p *PortalInfo) UpdateCHAPCredentials(credentials IscsiChapInfo) {
	p.Credentials = credentials
}

// IsFirstIdentifiedStaleAtSet identifies if FirstIdentifiedStaleAt time is set or not
func (p *PortalInfo) IsFirstIdentifiedStaleAtSet() bool {
	return p.FirstIdentifiedStaleAt != time.Time{}
}

// ResetFirstIdentifiedStaleAt reset FirstIdentifiedStaleAt time to its zero value
func (p *PortalInfo) ResetFirstIdentifiedStaleAt() {
	p.FirstIdentifiedStaleAt = time.Time{}
}

// RecordChanges compares a given portal info with another portal info and record changes
func (p *PortalInfo) RecordChanges(m PortalInfo) string {
	var sb strings.Builder

	if p.ISCSITargetIQN != m.ISCSITargetIQN {
		sb.WriteString(fmt.Sprintf("TargetIQN changed from '%v' to '%v'.\n", p.ISCSITargetIQN, m.ISCSITargetIQN))
	}

	if p.SessionNumber != m.SessionNumber {
		sb.WriteString(fmt.Sprintf("Session number changed from '%v' to '%v'.\n", p.SessionNumber, m.SessionNumber))
	}

	if p.ReasonInvalid != m.ReasonInvalid {
		sb.WriteString(fmt.Sprintf("Reason invalid changed from '%v' to '%v'.\n", p.ReasonInvalid, m.ReasonInvalid))
	}

	if p.Credentials.UseCHAP != m.Credentials.UseCHAP {
		sb.WriteString(fmt.Sprintf("CHAP changed from '%v' to '%v'.\n", p.Credentials.UseCHAP, m.Credentials.
			UseCHAP))
	}

	if p.Credentials.IscsiUsername != m.Credentials.IscsiUsername ||
		p.Credentials.IscsiTargetUsername != m.Credentials.IscsiTargetUsername ||
		p.Credentials.IscsiInitiatorSecret != m.Credentials.IscsiInitiatorSecret ||
		p.Credentials.IscsiTargetSecret != m.Credentials.IscsiTargetSecret {
		sb.WriteString("CHAP credentials have changed.\n")
	}

	return sb.String()
}

func (p *PortalInfo) String() string {
	return fmt.Sprintf("iSCSITargetIQN: %v, useCHAP: %v, lastAccessTime: %v, firstIdentifiedStaleAt: %v, "+
		"sessionNumber: %v, reasonInvalid: %v, source: %v", p.ISCSITargetIQN, p.Credentials.UseCHAP, p.LastAccessTime,
		p.FirstIdentifiedStaleAt, p.SessionNumber, p.ReasonInvalid, p.Source)
}

type ISCSISessionData struct {
	PortalInfo  PortalInfo
	LUNs        LUNs
	Remediation ISCSIAction
}

type ISCSISessions struct {
	Info map[string]*ISCSISessionData
}

// IsEmpty identifies whether portal to LUN mapping is empty or not.
func (p *ISCSISessions) IsEmpty() bool {
	return p == nil || len(p.Info) == 0
}

// ISCSISessionData returns the PortalInfo and LUNInfo associated to a portal
func (p *ISCSISessions) ISCSISessionData(portal string) (*ISCSISessionData, error) {
	if p.IsEmpty() {
		return nil, fmt.Errorf("no iSCSI sessions exist")
	}

	if strings.TrimSpace(portal) == "" {
		return nil, fmt.Errorf("portal value cannot be empty")
	}

	if iSCSISessionData, found := p.Info[parseHostportIP(portal)]; found {
		return iSCSISessionData, nil
	}

	return nil, errors.NotFoundError("portal not found")
}

// AddPortal creates a portal entry along with PortalInfo but without any LUNInfo
func (p *ISCSISessions) AddPortal(portal string, portalInfo PortalInfo) error {
	if p == nil {
		return fmt.Errorf("ISCSISession has not been initialized")
	}

	if strings.TrimSpace(portal) == "" {
		return fmt.Errorf("portal value cannot be empty")
	}

	if !portalInfo.HasTargetIQN() {
		return fmt.Errorf("portal info is missing target IQN")
	}

	if _, err := p.ISCSISessionData(portal); err == nil {
		return fmt.Errorf("unable to add new portal '%v' to the mapping; portal already exists", portal)
	}

	if p.Info == nil {
		p.Info = make(map[string]*ISCSISessionData)
	}

	var newLUNInfo LUNs
	newLUNInfo.Info = make(map[int32]string)

	iSCSISessionData := ISCSISessionData{
		PortalInfo: portalInfo,
		LUNs:       newLUNInfo,
	}
	p.Info[parseHostportIP(portal)] = &iSCSISessionData

	return nil
}

// UpdateAndRecordPortalInfoChanges updates the portal information associated with the portal
// and identifies changes to the portal info (if any)
func (p *ISCSISessions) UpdateAndRecordPortalInfoChanges(portal string, portalInfo PortalInfo) (string, error) {
	var portalInfoChanges string

	if !portalInfo.HasTargetIQN() {
		return portalInfoChanges, fmt.Errorf("new portal info is missing target IQN")
	}

	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return portalInfoChanges, fmt.Errorf("unable to update portal '%v' info in the mapping; %w", portal, err)
	} else if iSCSISessionData == nil {
		// Initialize current Portal LUN data to an empty value so that it can be updated.
		iSCSISessionData = &ISCSISessionData{}
	}

	currentPortalInfo := iSCSISessionData.PortalInfo

	// Identify portal information that may have changed
	portalInfoChanges = currentPortalInfo.RecordChanges(portalInfo)

	if portalInfoChanges != "" {
		portalInfo.LastAccessTime = currentPortalInfo.LastAccessTime
		portalInfo.FirstIdentifiedStaleAt = currentPortalInfo.FirstIdentifiedStaleAt
		iSCSISessionData.PortalInfo = portalInfo
	}

	return portalInfoChanges, nil
}

// UpdateCHAPForPortal updates the CHAP information associated with the portal
func (p *ISCSISessions) UpdateCHAPForPortal(portal string, newCHAP IscsiChapInfo) error {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return fmt.Errorf("failed to update CHAP; unable to get portal info for '%v'; %v", portal, err)
	} else if iSCSISessionData == nil {
		return fmt.Errorf("failed to update CHAP; portal info not set for portal '%v'", portal)
	}

	// Ensure current portal information has target IQN
	if !iSCSISessionData.PortalInfo.HasTargetIQN() {
		return fmt.Errorf("failed to update CHAP; portal '%v' is missing Target IQN", portal)
	}

	// Update portal's CHAP information
	iSCSISessionData.PortalInfo.UpdateCHAPCredentials(newCHAP)

	return nil
}

// AddLUNToPortal adds a LUNInfo to a pre-existing portal entry in the map
func (p *ISCSISessions) AddLUNToPortal(portal string, lData LUNData) error {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return fmt.Errorf("unable to add LUN '%v' to the portal '%v' in mapping; %v", lData, portal, err)
	} else if iSCSISessionData == nil {
		return fmt.Errorf("failed to add LUN; portal info not set for portal '%v'", portal)
	}

	// Ensure current portal information has target IQN
	if !iSCSISessionData.PortalInfo.HasTargetIQN() {
		return fmt.Errorf("failed to add LUN; portal '%v' is missing Target IQN", portal)
	}

	iSCSISessionData.LUNs.AddLUN(lData)

	return nil
}

// RemoveLUNFromPortal removes a LUNInfo from the given portal mapping
// If it is the last LUNInfo corresponding to a portal, then portal is
// removed from the map as well
func (p *ISCSISessions) RemoveLUNFromPortal(portal string, l int32) {
	if iSCSISessionData, err := p.ISCSISessionData(portal); err == nil && iSCSISessionData != nil {
		iSCSISessionData.LUNs.RemoveLUN(l)

		// If this is the last LUN, remove the portal entry
		if iSCSISessionData.LUNs.IsEmpty() {
			p.RemovePortal(portal)
		}
	}
}

// RemovePortal removes portal (along with its PortalInfo and LUNInfo) from the map
func (p *ISCSISessions) RemovePortal(portal string) {
	if p != nil {
		delete(p.Info, parseHostportIP(portal))
	}
}

// CheckPortalExists checks whether the portal is already in the map
func (p *ISCSISessions) CheckPortalExists(portal string) bool {
	if _, err := p.ISCSISessionData(portal); err != nil {
		return false
	}

	return true
}

// PortalInfo returns portal info for a given portal.
func (p *ISCSISessions) PortalInfo(portal string) (*PortalInfo, error) {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return nil, fmt.Errorf("unable to get portal info for portal '%v' in mapping; %v", portal, err)
	} else if iSCSISessionData == nil {
		return nil, fmt.Errorf("failed to get portal info; portal info not set for portal '%v'", portal)
	}

	return &iSCSISessionData.PortalInfo, nil
}

// LUNInfo returns LUNInfo for a given portal.
func (p *ISCSISessions) LUNInfo(portal string) (*LUNs, error) {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return nil, fmt.Errorf("unable to get LUN info for portal '%v' in mapping; %v", portal, err)
	} else if iSCSISessionData == nil {
		return nil, fmt.Errorf("failed to get LUN info; portal & LUN info not set for portal '%v'", portal)
	}

	return &iSCSISessionData.LUNs, nil
}

// LUNsForPortal returns the list of LUNs associated with this portal
func (p *ISCSISessions) LUNsForPortal(portal string) ([]int32, error) {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return nil, fmt.Errorf("unable to get LUNs for portal '%v' in mapping; %v", portal, err)
	} else if iSCSISessionData == nil {
		return nil, fmt.Errorf("failed to get LUNs; portal & LUN info not set for portal '%v'", portal)
	}

	return iSCSISessionData.LUNs.AllLUNs(), nil
}

// VolumeIDForPortal to get any valid volume ID behind a given portal.
func (p *ISCSISessions) VolumeIDForPortal(portal string) (string, error) {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return "", fmt.Errorf("unable to get volume ID for portal '%v' in mapping; %v", portal, err)
	} else if iSCSISessionData == nil {
		return "", fmt.Errorf("failed to get volume ID; portal & LUN info not set for portal '%v'", portal)
	}

	for _, volID := range iSCSISessionData.LUNs.Info {
		if volID != "" {
			return volID, nil
		}
	}

	return "", fmt.Errorf("no volume ID found for the portal: %v", portal)
}

// ResetPortalRemediationValue resets remediation information associated with the portal
func (p *ISCSISessions) ResetPortalRemediationValue(portal string) error {
	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return fmt.Errorf("failed to update remediation value; unable to get portal info for '%v'; %v", portal, err)
	} else if iSCSISessionData == nil {
		return fmt.Errorf("failed to update remediation value; portal info not set for portal '%v'", portal)
	}

	iSCSISessionData.Remediation = NoAction

	return nil
}

// ResetAllRemediationValues resets remediation information associated with all the portals
func (p *ISCSISessions) ResetAllRemediationValues() error {
	if p.IsEmpty() {
		return nil
	}

	var sb strings.Builder
	for portal := range p.Info {
		if err := p.ResetPortalRemediationValue(portal); err != nil {
			sb.WriteString(fmt.Sprintf("\n%v", err))
		}
	}

	errVal := sb.String()
	if errVal != "" {
		return fmt.Errorf(errVal)
	}

	return nil
}

func (p *ISCSISessions) GeneratePublishInfo(portal string) (VolumePublishInfo, error) {
	var publishInfo VolumePublishInfo

	iSCSISessionData, err := p.ISCSISessionData(portal)
	if err != nil {
		return publishInfo, fmt.Errorf("unable to generate publish info for portal '%v' in mapping; %v", portal, err)
	} else if iSCSISessionData == nil {
		return publishInfo, fmt.Errorf("failed to generate publish info; portal & LUN info not set for portal '%v'",
			portal)
	}

	// Ensure current portal information has target IQN
	if !iSCSISessionData.PortalInfo.HasTargetIQN() {
		return publishInfo, fmt.Errorf("cannot generate publish info; portal '%v' is missing target IQN", portal)
	}

	publishInfo = VolumePublishInfo{
		VolumeAccessInfo: VolumeAccessInfo{
			IscsiAccessInfo: IscsiAccessInfo{
				IscsiTargetIQN:    iSCSISessionData.PortalInfo.ISCSITargetIQN,
				IscsiChapInfo:     iSCSISessionData.PortalInfo.Credentials,
				IscsiTargetPortal: portal,
			},
		},
	}

	return publishInfo, nil
}

// String prints values of the map
func (p ISCSISessions) String() string {
	if p.IsEmpty() {
		return "empty portal to LUN mapping"
	}

	var sb strings.Builder
	for portal, iSCSISessionData := range p.Info {
		if strings.TrimSpace(portal) == "" {
			sb.WriteString("\n[Portal: portal value missing]:")

			if iSCSISessionData != nil {
				sb.WriteString(fmt.Sprintf("\n\tPortalInfo: %+v \n\tLUNInfo: %+v",
					iSCSISessionData.PortalInfo, iSCSISessionData.LUNs))
			}
		} else if iSCSISessionData == nil {
			sb.WriteString(fmt.Sprintf("\n[Portal: %v]: session information is missing", portal))
		} else {
			sb.WriteString(fmt.Sprintf("\n[Portal: %v]: \n\tPortalInfo: %+v \n\tLUNInfo: %+v",
				portal, iSCSISessionData.PortalInfo, iSCSISessionData.LUNs))
		}
	}

	return sb.String()
}

// GoString prints Go-syntax representation of the map
func (p ISCSISessions) GoString() string {
	return p.String()
}
