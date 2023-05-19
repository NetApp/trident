// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import "context"

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

type NVMeDevice struct {
	Device        string `json:"Device"`
	Vserver       string `json:"Vserver"`
	NamespacePath string `json:"Namespace_Path"`
	NSID          int    `json:"NSID"`
	UUID          string `json:"UUID"`
	Size          string `json:"Size"`
	LBADataSize   int    `json:"LBA_Data_Size"`
	NamespaceSize int64  `json:"Namespace_Size"`
}

type Path struct {
	Name      string `json:"Name"`
	Transport string `json:"Transport"`
	Address   string `json:"Address"`
	State     string `json:"State"`
}

type NVMeSubsystem struct {
	Name  string `json:"Name"`
	NQN   string `json:"NQN"`
	Paths []Path `json:"Paths"`
}

type Subsystems struct {
	Subsystems []NVMeSubsystem `json:"Subsystems"`
}

type NVMeSubsystemInterface interface {
	GetConnectionStatus() NVMeSubsystemConnectionStatus
	Connect(ctx context.Context, nvmeTargetIps []string) error
	Disconnect(ctx context.Context) error
	GetNamespaceCount(ctx context.Context) (int, error)
}

type NVMeDeviceInterface interface {
	GetPath() string
	FlushDevice(ctx context.Context) error
}

// NVMeHandler implements the NVMeInterface. It acts as a layer to invoke all the NVMe related
// operations on the k8s node. This struct is currently empty as we don't need to store any
// node or NVMe related information yet.
type NVMeHandler struct{}

type NVMeInterface interface {
	NVMeActiveOnHost(ctx context.Context) (bool, error)
	GetHostNqn(ctx context.Context) (string, error)
	NewNVMeSubsystem(ctx context.Context, subsNqn string) NVMeSubsystemInterface
	NewNVMeDevice(ctx context.Context, nsPath string) (NVMeDeviceInterface, error)
}
