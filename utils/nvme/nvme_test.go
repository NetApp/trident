// Copyright 2023 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

var (
	testSubsystem1 = NVMeSubsystem{NQN: "nqn1", Paths: []Path{{Address: "traddr=1.1.1.1,trsvcid=4420"}}}
	testSubsystem2 = NVMeSubsystem{NQN: "nqn2", Paths: []Path{{Address: "traddr=2.2.2.2,trsvcid=4420"}}}
)

func TestExtractIPFromNVMeAddress(t *testing.T) {
	ubuntuAddress := "traddr=10.193.108.74 trsvcid=4420"
	rhelAddress := "traddr=10.193.108.74,trsvcid=4420"

	lif1 := extractIPFromNVMeAddress(ubuntuAddress)
	lif2 := extractIPFromNVMeAddress(rhelAddress)

	assert.Equal(t, lif1, "10.193.108.74")
	assert.Equal(t, lif2, "10.193.108.74")
}

func TestNVMeSessions_AddNVMeSession(t *testing.T) {
	ns := NVMeSessions{}

	ns.AddNVMeSession(testSubsystem1, []string{})
	sd := ns.Info[testSubsystem1.NQN]

	assert.True(t, ns.CheckNVMeSessionExists(testSubsystem1.NQN), "Failed to add NVMe session.")
	assert.False(t, sd.IsTargetIPPresent("2.2.2.2"), "Target IP found.")

	// Adding the same subsystem with new targetIPs in the same session.
	ns.AddNVMeSession(testSubsystem1, []string{"2.2.2.2"})

	assert.True(t, ns.CheckNVMeSessionExists(testSubsystem1.NQN), "Failed to add NVMe session.")
	assert.True(t, sd.IsTargetIPPresent("2.2.2.2"), "Target IP not found.")
}

func TestNVMeSessions_RemoveNVMeSession(t *testing.T) {
	ns := NVMeSessions{}
	assert.False(t, ns.CheckNVMeSessionExists(testSubsystem1.NQN), "NVMe session found.")

	ns.AddNVMeSession(testSubsystem1, []string{})
	assert.True(t, ns.CheckNVMeSessionExists(testSubsystem1.NQN), "NVMe session not found.")

	ns.RemoveNVMeSession(testSubsystem1.NQN)
	assert.False(t, ns.CheckNVMeSessionExists(testSubsystem1.NQN), "NVMe session still exists.")
}

func TestNVMeSessions_ResetRemediationForAll(t *testing.T) {
	ns := NewNVMeSessions()

	ns.AddNVMeSession(testSubsystem1, []string{})
	sd := ns.Info[testSubsystem1.NQN]
	sd.SetRemediation(ConnectOp)

	ns.ResetRemediationForAll()

	assert.Equal(t, NoOp, sd.Remediation, "Failed to update remediation.")
}

func TestSortSubsystemsUsingSessions(t *testing.T) {
	oldSubs := []NVMeSubsystem{testSubsystem1, testSubsystem2}
	ns := NewNVMeSessions()
	newSubs := make([]NVMeSubsystem, len(oldSubs))
	copy(newSubs, oldSubs)

	SortSubsystemsUsingSessions(newSubs, ns)

	assert.Equal(t, oldSubs, newSubs, "Subsystems are not same.")

	// Adding the subsystems to the NVMe sessions and modifying their access times.
	for index, sub := range oldSubs {
		ns.AddNVMeSession(sub, []string{})
		ns.Info[sub.NQN].LastAccessTime = time.UnixMicro(int64(100 - index))
	}

	SortSubsystemsUsingSessions(newSubs, ns)

	assert.Equal(t, oldSubs[1], newSubs[0], "Subsystem mismatch.")
	assert.Equal(t, oldSubs[0], newSubs[1], "Subsystem mismatch.")
}

func TestNVMeHandler_AddPublishedNVMeSession(t *testing.T) {
	nh := NewNVMeHandler()
	volPubInfo := &models.VolumePublishInfo{}
	volPubInfo.NVMeSubsystemNQN = testSubsystem1.NQN

	// Uninitialized published sessions case.
	var pubSessions *NVMeSessions
	nh.AddPublishedNVMeSession(pubSessions, volPubInfo)

	assert.Nil(t, pubSessions, "Published sessions initialized.")

	// Initialized published sessions case.
	pubSessions = NewNVMeSessions()
	nh.AddPublishedNVMeSession(pubSessions, volPubInfo)

	assert.True(t, pubSessions.CheckNVMeSessionExists(testSubsystem1.NQN), "NVMe session not found.")
}

func TestNVMeSessions_AddNamespaceToSession(t *testing.T) {
	var pubSessions *NVMeSessions
	// Uninitialized published session case.
	pubSessions.AddNamespaceToSession("testNQN", "testUUID")
	c := pubSessions.GetNamespaceCountForSession("testNQN")
	assert.Equal(t, c, 0, "unexpected namespaces surfaced")

	// subsystem not found case
	pubSessions = NewNVMeSessions()
	pubSessions.Info["testNQN"] = &NVMeSessionData{
		Subsystem:      NVMeSubsystem{},
		Namespaces:     nil,
		NVMeTargetIPs:  nil,
		LastAccessTime: time.Time{},
		Remediation:    0,
	}
	pubSessions.AddNamespaceToSession("testNQN-nonexistent", "testUUID")
	c = pubSessions.GetNamespaceCountForSession("testNQN-nonexistent")
	assert.Zero(t, c, "expected no namespaces.")

	// add namespace case
	pubSessions.AddNamespaceToSession("testNQN", "testUUID")
	c = pubSessions.GetNamespaceCountForSession("testNQN")
	assert.Equal(t, c, 1, "expected only one namespace")
}

func TestNVMeSessions_RemoveNamespaceFromSession(t *testing.T) {
	var pubSessions *NVMeSessions
	// Uninitialized published session case.
	pubSessions.RemoveNamespaceFromSession("testNQN", "testUUID")

	// subsystem not found case
	pubSessions = NewNVMeSessions()
	pubSessions.Info["testNQN"] = &NVMeSessionData{}
	pubSessions.RemoveNamespaceFromSession("testNQN-nonexistent", "testUUID")

	pubSessions.RemoveNamespaceFromSession("testNQN", "testUUID")
}

func TestNVMeHandler_RemovePublishedNVMeSession(t *testing.T) {
	nh := NewNVMeHandler()
	volPubInfo := &models.VolumePublishInfo{}
	volPubInfo.NVMeSubsystemNQN = testSubsystem1.NQN

	// Uninitialized published sessions case.
	var pubSessions *NVMeSessions
	disconnect := nh.RemovePublishedNVMeSession(pubSessions, testSubsystem1.NQN, "test")

	assert.Nil(t, pubSessions, "Published sessions initialized.")
	assert.False(t, disconnect, "Don't disconnect subsystem.")

	// Initialized published sessions case.
	pubSessions = NewNVMeSessions()
	nh.AddPublishedNVMeSession(pubSessions, volPubInfo)
	disconnect = nh.RemovePublishedNVMeSession(pubSessions, testSubsystem1.NQN, volPubInfo.NVMeNamespaceUUID)

	assert.False(t, pubSessions.CheckNVMeSessionExists(testSubsystem1.NQN), "NVMe session not deleted.")
	assert.True(t, disconnect, "Disconnect subsystem.")
}

func TestNVMeHandler_InspectNVMeSessions_EmptyPublishedSessions(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()

	subs := nh.InspectNVMeSessions(context.Background(), pubSessions, nil)

	assert.Equal(t, 0, len(subs), "Got few subsystems even though nothing is published.")
}

func TestNVMeHandler_InspectNVMeSessions_NilPublishedSessionData(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	pubSessions.Info[testSubsystem1.NQN] = nil

	subs := nh.InspectNVMeSessions(context.Background(), pubSessions, nil)

	assert.Equal(t, 0, len(subs), "Valid published session found.")
}

func TestNVMeHandler_InspectNVMeSessions_PublishedNotPresentInCurrent(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})

	subs := nh.InspectNVMeSessions(context.Background(), pubSessions, currSessions)

	assert.Equal(t, 0, len(subs), "Published session found in current session list.")
}

func TestNVMeHandler_InspectNVMeSessions_NilCurrentSessionData(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	currSessions.Info[testSubsystem1.NQN] = nil

	subs := nh.InspectNVMeSessions(context.Background(), pubSessions, currSessions)

	assert.Equal(t, 0, len(subs), "Published session found in current with a valid entry.")
}

func TestNVMeHandler_InspectNVMeSessions_DisconnectedSubsystem(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	currSessions.AddNVMeSession(NVMeSubsystem{NQN: testSubsystem1.NQN}, []string{})

	subs := nh.InspectNVMeSessions(context.Background(), pubSessions, currSessions)

	assert.Equal(t, 0, len(subs), "Disconnected subsystem present in subsystems to fix.")
}

func TestNVMeHandler_InspectNVMeSessions_PartiallyConnectedSubsystem(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	currSessions.AddNVMeSession(testSubsystem1, []string{})

	subs := nh.InspectNVMeSessions(context.Background(), pubSessions, currSessions)

	assert.Equal(t, 1, len(subs), "No subsystems found.")
	assert.Equal(t, testSubsystem1, subs[0], "No subsystems found which needs remediation.")
	assert.Equal(t, ConnectOp, pubSessions.Info[testSubsystem1.NQN].Remediation, "Remediation not set.")
}

func TestNVMeHandler_RectifyNVMeSession(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()

	// Empty published sessions case.
	nh.RectifyNVMeSession(context.Background(), testSubsystem1, pubSessions)

	// Nil published session data case.
	pubSessions.Info[testSubsystem1.NQN] = nil
	nh.RectifyNVMeSession(context.Background(), testSubsystem1, pubSessions)

	// NoOp remediation case.
	pubSessions.RemoveNVMeSession(testSubsystem1.NQN)
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	nh.RectifyNVMeSession(context.Background(), testSubsystem1, pubSessions)

	// Happy path connect remediation
	pubSessions.RemoveNVMeSession(testSubsystem1.NQN)
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	pubSessions.Info[testSubsystem1.NQN].Remediation = ConnectOp
	nh.RectifyNVMeSession(context.Background(), testSubsystem1, pubSessions)
}

func TestNVMeHandler_PopulateCurrentNVMeSessions_NilCurrentSessions(t *testing.T) {
	nh := NewNVMeHandler()

	err := nh.PopulateCurrentNVMeSessions(context.Background(), nil)

	assert.ErrorContains(t, err, "current NVMeSessions not initialized",
		"Populated current sessions successfully.")
}

func TestGetConnectionStatus(t *testing.T) {
	tests := map[string]struct {
		subsystem NVMeSubsystem
		expect    NVMeSubsystemConnectionStatus
	}{
		"Partially connected subsystem": {
			subsystem: NVMeSubsystem{NQN: "mock-nqn", Paths: []Path{{Address: "mock-address"}}},
			expect:    NVMeSubsystemPartiallyConnected,
		},
		"Connected subsystem": {
			subsystem: NVMeSubsystem{
				NQN:   "mock-nqn",
				Paths: []Path{{Address: "mock-address"}, {Address: "mock-address2"}},
			},
			expect: NVMeSubsystemConnected,
		},
		"Disconnected subsystem": {
			subsystem: NVMeSubsystem{NQN: "mock-nqn", Paths: nil},
			expect:    NVMeSubsystemDisconnected,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expect, params.subsystem.GetConnectionStatus(), "Unexpected connection status found.")
		})
	}
}

func TestIsNetworkPathPresent(t *testing.T) {
	tests := map[string]struct {
		ip     string
		path   []Path
		expect bool
	}{
		"Ubuntu format": {
			ip: "10.193.108.74",
			path: []Path{
				{Address: "traddr=10.193.108.74 trsvcid=4420"},
			},
			expect: true,
		},
		"RHEL format": {
			ip: "10.193.108.74",
			path: []Path{
				{Address: "traddr=10.193.108.74,trsvcid=4420"},
			},
			expect: true,
		},
		"Invalid format": {
			ip: "1.2.3.4",
			path: []Path{
				{Address: "invalid"},
			},
			expect: false,
		},
		"Not Found": {
			ip: "1.2.3.4",
			path: []Path{
				{Address: "traddr=10.193.108.74,trsvcid=4420"},
			},
			expect: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			subsystem := NewNVMeSubsystemDetailed("mock-nqn", "mock-name", params.path, nil, nil)
			isPresent := subsystem.IsNetworkPathPresent(params.ip)
			assert.Equal(t, params.expect, isPresent)
		})
	}
}

func toBytes(data interface{}) []byte {
	bytesBuffer := new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(data)
	return bytesBuffer.Bytes()
}

func getMockNNVMeDevices() NVMeDevices {
	mockDevices := NVMeDevices{
		Devices: []NVMeDevice{
			{
				Device: "mock-device",
				UUID:   "1234",
			},
		},
	}
	return mockDevices
}

func getMockNvmeListRHELOutput() []byte {
	return []byte(`[
  {
    "HostNQN":"nqn.2014-08.org.mock:uuid:a48f0944-3ab0-4e5b-8019-79192500a44e",
    "Subsystems":[
      {
        "Name":"nvme-subsys0",
        "NQN":"mock-nqn",
        "IOPolicy":"numa",
        "Paths":[
			{"Address": "mock-address-1"},
			{"Address": "mock-address-2"}
		]
      }
    ]
  }
]`)
}

func getMockNvmeListUnuntuOutput() []byte {
	return []byte(`{
    "HostNQN":"nqn.2014-08.org.mock:uuid:a48f0944-3ab0-4e5b-8019-79192500a44e",
    "Subsystems":[
      {
        "Name":"nvme-subsys0",
        "NQN":"mock-nqn",
        "IOPolicy":"numa",
        "Paths":[
			{"Address": "mock-address-1"},
			{"Address": "mock-address-2"}
		]
      }
    ]
  }`)
}

func TestIsNil(t *testing.T) {
	tests := map[string]struct {
		device *NVMeDevice
		expect bool
	}{
		"Nil device": {
			device: nil,
			expect: true,
		},
		"Valid device": {
			device: &NVMeDevice{Device: "mock-device"},
			expect: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expect, params.device.IsNil())
		})
	}
}

func TestGetPath(t *testing.T) {
	tests := map[string]struct {
		device *NVMeDevice
		expect string
	}{
		"Valid path": {
			device: &NVMeDevice{Device: "mock-device"},
			expect: "mock-device",
		},
		"Nil device": {
			device: nil,
			expect: "",
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expect, params.device.GetPath(), "Unexpected path found.")
		})
	}
}
