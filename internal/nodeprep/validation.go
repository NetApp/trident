package nodeprep

import (
	"fmt"
	"strings"

	"github.com/netapp/trident/utils/errors"
)

const iscsi = "iscsi"

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
		return errors.UnsupportedError(fmt.Sprintf("only one protocl (iSCSI) is supported at this time but node prep protocol set to '%s'", requestedProtocols))
	}
	if requestedProtocols[0] != iscsi {
		return errors.UnsupportedError(fmt.Sprintf("'%s' is not a valid node prep protocol", requestedProtocols))
	}

	return nil
}
