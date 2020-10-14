// Copyright 2020 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNetInterfaceInfoTypeAddress(t *testing.T) {
	log.Debug("Running TestNetInterfaceInfoTypeAddress...")
	netInterfaceInfoType := NewNetInterfaceInfoType()
	result := netInterfaceInfoType.Address() // make sure this doesn't cause a panic on a nil pointer deref
	assert.Equal(t, "", result)
}
