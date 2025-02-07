// Copyright 2023 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"time"

	"github.com/spf13/afero"

	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
)

//go:generate mockgen -destination=../../mocks/mock_utils/nvme/mock_nvme_utils.go github.com/netapp/trident/utils/nvme NVMeInterface

// MaxSessionsPerSubsystem represents the total number of paths from host to ONTAP subsystem.
// NVMe subsystem can have maximum 2 dataLIFs, so it is capped to 2.
const MaxSessionsPerSubsystem = 2

// TransportAddressEqualTo is a part of Path.Address string. It is used to extract IP address.
const TransportAddressEqualTo = "traddr="

// NVMeListCmdTimeoutInSeconds is the default timeout supplied to NVMe cli command.
const NVMeListCmdTimeoutInSeconds = 10

// NVMeSubsystemConnectionStatus is a data structure which reflects NVMe subsystem connection status.
type NVMeSubsystemConnectionStatus int8

// NVMe subsystem connection states
const (
	NVMeSubsystemConnected NVMeSubsystemConnectionStatus = iota
	NVMeSubsystemDisconnected
	NVMeSubsystemPartiallyConnected
)

type NVMeDevices struct {
	Devices []NVMeDevice `json:"ONTAPdevices"`
}

// NVMeDevice represents the NVMe device structure present in the cli json output of `nvme netapp ontapdevices`
//
//	 "ONTAPdevices":[
//	  {
//	    "Device":"/dev/nvme0n1",
//	    "Vserver":"nvme_svm0",
//	    "Namespace_Path":"/vol/trident_pvc_402e841e_d6ca_424e_be0b_07d412bf7fdf/namespace0",
//	    "NSID":1,
//	    "UUID":"1055d699-7f4f-4195-a573-5a433b8eb4bc",
//	    "Size":"31.46MB",
//	    "LBA_Data_Size":4096,
//	    "Namespace_Size":7680
//	  }
//	]
type NVMeDevice struct {
	Device        string `json:"Device"`
	NamespacePath string `json:"Namespace_Path"`
	NSID          int    `json:"NSID"`
	UUID          string `json:"UUID"`
	Size          string `json:"Size"`
	NamespaceSize int64  `json:"Namespace_Size"`
	command       exec.Command
}

type Path struct {
	Name      string `json:"Name"`
	Transport string `json:"Transport"`
	Address   string `json:"Address"`
	State     string `json:"State"`
}

// NVMeSubsystem represents the NVMe subsystem structure present in the cli json output of `nvme list-subsys`.
// Paths represent all the sessions present for that subsystem.
// "Subsystems":[
//
//	  {
//	    "Name":"nvme-subsys0",
//	    "NQN":"nqn.1992-08.com.netapp:sn.d65e8c1addb211ed9257005056b32ae5:subsystem.scspa2826047001-02d067a5-a376-4cc1-bcd7-093ed58afe70",
//	    "Paths":[
//	      {
//	        "Name":"nvme0",
//	        "Transport":"tcp",
//	        "Address":"traddr=10.193.96.225,trsvcid=4420",
//	        "State":"live"
//	      },
//	      {
//	        "Name":"nvme1",
//	        "Transport":"tcp",
//	        "Address":"traddr=10.193.96.226,trsvcid=4420",
//	        "State":"live"
//	      }
//	    ]
//	  }
//	]
type NVMeSubsystem struct {
	Name    string `json:"Name"`
	NQN     string `json:"NQN"`
	Paths   []Path `json:"Paths"`
	osFs    afero.Fs
	command exec.Command
}

type Subsystems struct {
	Subsystems []NVMeSubsystem `json:"Subsystems"`
}

// NVMeOperation is a data structure for NVMe self-healing operations.
type NVMeOperation int8

const (
	NoOp NVMeOperation = iota
	ConnectOp
)

// NVMeSessionData contains all the information related to any NVMe session. It has the subsystem information, the
// corresponding backend target IPs (dataLIFs for ONTAP), last access time and remediation. Last access time is used
// in self-healing so that newer sessions will get prioritised.
// In the future, we can add DH-HMAC-CHAP to this structure once we start supporting CHAP for NVMe. Also, if we realise
// at any point that we have a namespace missing use case to handle, we need to store that too in this structure.
type NVMeSessionData struct {
	Subsystem      NVMeSubsystem
	Namespaces     map[string]bool
	NVMeTargetIPs  []string
	LastAccessTime time.Time
	Remediation    NVMeOperation
}

// NVMeSessions is a map of subsystem NQN and NVMeSessionData used for tracking self-healing information.
type NVMeSessions struct {
	Info map[string]*NVMeSessionData
}

type NVMeSubsystemInterface interface {
	GetConnectionStatus() NVMeSubsystemConnectionStatus
	Connect(ctx context.Context, nvmeTargetIps []string, connectOnly bool) error
	Disconnect(ctx context.Context) error
	GetNamespaceCount(ctx context.Context) (int, error)
	IsNetworkPathPresent(ip string) bool
	ConnectSubsystemToHost(ctx context.Context, IP string) error
	DisconnectSubsystemFromHost(ctx context.Context) error
	GetNamespaceCountForSubsDevice(ctx context.Context) (int, error)
	GetNVMeDevice(ctx context.Context, nsUUID string) (NVMeDeviceInterface, error)
	GetNVMeDeviceAt(ctx context.Context, nsUUID string) (NVMeDeviceInterface, error)
	GetNVMeDeviceCountAt(ctx context.Context, path string) (int, error)
}

type NVMeDeviceInterface interface {
	GetPath() string
	FlushDevice(ctx context.Context, ignoreErrors, force bool) error
	IsNil() bool
	FlushNVMeDevice(ctx context.Context) error
}

// NVMeHandler implements the NVMeInterface. It acts as a layer to invoke all the NVMe related
// operations on the k8s node. This struct is currently empty as we don't need to store any
// node or NVMe related information yet.
type NVMeHandler struct {
	command       exec.Command
	devicesClient devices.Devices
	mountClient   mount.Mount
	fsClient      filesystem.Filesystem
	osFs          afero.Fs
}

type NVMeInterface interface {
	NVMeActiveOnHost(ctx context.Context) (bool, error)
	GetHostNqn(ctx context.Context) (string, error)
	NewNVMeSubsystem(ctx context.Context, subsNqn string) NVMeSubsystemInterface
	AddPublishedNVMeSession(pubSessions *NVMeSessions, publishInfo *models.VolumePublishInfo)
	RemovePublishedNVMeSession(pubSessions *NVMeSessions, subNQN, nsUUID string) bool
	PopulateCurrentNVMeSessions(ctx context.Context, currSessions *NVMeSessions) error
	InspectNVMeSessions(ctx context.Context, pubSessions, currSessions *NVMeSessions) []NVMeSubsystem
	RectifyNVMeSession(ctx context.Context, subsystemToFix NVMeSubsystem, pubSessions *NVMeSessions)
	NVMeMountVolume(
		ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
	) error
	AttachNVMeVolume(
		ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
	) error
	AttachNVMeVolumeRetry(
		ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
		timeout time.Duration,
	) error
	GetNVMeDeviceList(ctx context.Context) (NVMeDevices, error)
	GetNVMeSubsystem(ctx context.Context, nqn string) (*NVMeSubsystem, error)
}
