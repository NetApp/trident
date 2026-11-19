// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathState(t *testing.T) {
	cases := map[string]struct {
		state PathState
		live  bool
		str   string
	}{
		"live":                {PathStateLive, true, "live"},
		"connecting":          {PathStateConnecting, false, "connecting"},
		"deleting":            {PathStateDeleting, false, "deleting"},
		"unknown_empty":       {PathStateUnknown, false, ""},
		"resetting_unlisted":  {PathState("resetting"), false, "resetting"},
		"deleting_no_io":      {PathState("deleting (no IO)"), false, "deleting (no IO)"},
		"case_sensitive_live": {PathState("Live"), false, "Live"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.live, tc.state.IsLive())
			assert.Equal(t, tc.str, tc.state.String())
		})
	}
}

func TestPath(t *testing.T) {
	p := &Path{name: "nvme0", address: "10.0.0.1", port: "4420", transport: TransportTCP, state: PathStateLive}

	assert.Equal(t, "nvme0", p.Name())
	assert.Equal(t, "10.0.0.1", p.Address())
	assert.Equal(t, "4420", p.Port())
	assert.Equal(t, TransportTCP, p.Transport())
	assert.Equal(t, PathStateLive, p.State())
	assert.True(t, p.IsLive())

	p.state = PathStateConnecting
	assert.False(t, p.IsLive())
}

func TestParseAddress(t *testing.T) {
	cases := map[string]struct{ in, traddr, port string }{
		"rhel_comma":         {"traddr=10.193.108.74,trsvcid=4420", "10.193.108.74", "4420"},
		"ubuntu_space":       {"traddr=10.193.108.74 trsvcid=4420", "10.193.108.74", "4420"},
		"tab_separated":      {"traddr=10.0.0.1\ttrsvcid=4420", "10.0.0.1", "4420"},
		"trailing_src_addr":  {"traddr=10.193.156.158,trsvcid=4420,src_addr=10.211.55.6", "10.193.156.158", "4420"},
		"traddr_not_first":   {"trsvcid=4420,traddr=10.0.0.1", "10.0.0.1", "4420"},
		"host_traddr_ignore": {"host_traddr=10.9.9.9,traddr=10.0.0.1,trsvcid=4420", "10.0.0.1", "4420"},
		"host_iface_ignored": {"traddr=10.0.0.1,trsvcid=4420,host_iface=eth0", "10.0.0.1", "4420"},
		"ipv6": {
			"traddr=fd20:8b1e:b258:2014:9c83:2d91:44a:b618,trsvcid=4420",
			"fd20:8b1e:b258:2014:9c83:2d91:44a:b618", "4420",
		},
		"no_service_id_transport": {
			"traddr=nn-0x200000109b1c1204:pn-0x100000109b1c1204,host_traddr=nn-0x200000109b1c1205:pn-0x100000109b1c1205",
			"nn-0x200000109b1c1204:pn-0x100000109b1c1204", "",
		},
		"no_port":       {"traddr=10.0.0.1", "10.0.0.1", ""},
		"no_traddr":     {"trsvcid=4420", "", "4420"},
		"empty":         {"", "", ""},
		"garbage":       {"nonsense", "", ""},
		"empty_value":   {"traddr=,trsvcid=4420", "", "4420"},
		"extra_equals":  {"traddr=a=b,trsvcid=4420", "a=b", "4420"},
		"repeated_keys": {"traddr=10.0.0.1,traddr=10.0.0.2", "10.0.0.2", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			traddr, port := parseAddress(tc.in)
			assert.Equal(t, tc.traddr, traddr)
			assert.Equal(t, tc.port, port)
		})
	}
}
