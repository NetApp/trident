// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testOtherTransport stands in for any transport other than TCP. It is a real kernel transport name, so the
// module name it maps to (nvme_loop) is realistic.
const testOtherTransport Transport = "loop"

func TestTransport_Validate(t *testing.T) {
	valid := []Transport{TransportTCP, testOtherTransport, "fc", "rdma", "pcie"}
	for _, tr := range valid {
		t.Run("valid_"+string(tr), func(t *testing.T) {
			assert.NoError(t, tr.validate())
		})
	}

	invalid := map[string]Transport{
		"empty":        "",
		"upper_case":   "TCP",
		"digit":        "tcp1",
		"trailing":     "tcp ",
		"inner_space":  "tcp -x",
		"flag":         "-t",
		"path":         "../x",
		"module_name":  "nvme_tcp",
		"underscore":   "t_cp",
		"newline":      "tcp\n",
		"nul":          "tcp\x00",
		"unicode":      "tcр", // Cyrillic er
		"module_dir_x": "tcp/../fc",
	}
	for name, tr := range invalid {
		t.Run("invalid_"+name, func(t *testing.T) {
			assert.Error(t, tr.validate())
		})
	}
}

func TestTransport_UsesPort(t *testing.T) {
	assert.True(t, TransportTCP.usesPort())
	assert.False(t, testOtherTransport.usesPort())
	assert.False(t, Transport("").usesPort())
}

func TestTransport_String(t *testing.T) {
	assert.Equal(t, "tcp", TransportTCP.String())
	assert.Equal(t, "loop", testOtherTransport.String())
}

func TestTransport_Constants(t *testing.T) {
	assert.Equal(t, "4420", defaultPort)
	assert.Equal(t, Transport("tcp"), TransportTCP)
}
