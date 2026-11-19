// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"fmt"
	"regexp"
)

// Transport is an NVMe-oF transport, named as the kernel and nvme-cli name it (the `transport` sysfs
// attribute and `nvme connect -t`). The kernel module providing it is nvme_<transport>.
type Transport string

const (
	// TransportTCP is currently the only supported NVMe network protocol.
	TransportTCP Transport = "tcp"
)

func (t Transport) String() string { return string(t) }

// transportRegex limits a transport name to lowercase letters, which every kernel transport uses. It keeps a
// caller-supplied value from becoming a flag or a path component.
var transportRegex = regexp.MustCompile(`^[a-z]+$`)

func (t Transport) validate() error {
	if !transportRegex.MatchString(string(t)) {
		return fmt.Errorf("invalid NVMe transport %q", string(t))
	}
	return nil
}

// usesPort reports whether the transport addresses a target by an IP port (trsvcid). TCP does. A transport
// added later should be listed here if it does too.
func (t Transport) usesPort() bool { return t == TransportTCP }

// defaultPort is the IANA port for NVMe-oF over an IP transport.
const defaultPort = "4420"
