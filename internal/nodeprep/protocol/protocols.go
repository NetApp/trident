// Copyright 2024 NetApp, Inc. All Rights Reserved.

package protocol

import (
	"fmt"
	"strings"

	utilserrors "github.com/netapp/trident/utils/errors"
)

type Protocol = string

const ISCSI Protocol = "iscsi"

func FormatProtocols(requestedProtocols []string) (formattedProtocols []string) {
	for _, protocol := range requestedProtocols {
		formattedProtocols = append(formattedProtocols, strings.TrimSpace(strings.ToLower(protocol)))
	}
	return
}

func ValidateProtocols(requestedProtocols []string) error {
	if len(requestedProtocols) == 0 {
		return nil
	}
	if len(requestedProtocols) > 1 {
		return utilserrors.UnsupportedError(fmt.Sprintf("only one protocol ("+
			"iSCSI) is supported at this time but node prep protocol set to '%s'", requestedProtocols))
	}
	if requestedProtocols[0] != string(ISCSI) {
		return utilserrors.UnsupportedError(fmt.Sprintf("'%s' is not a valid node prep protocol", requestedProtocols))
	}

	return nil
}
