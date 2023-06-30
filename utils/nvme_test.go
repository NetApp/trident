// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	volPubInfo := &VolumePublishInfo{}
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

func TestNVMeHandler_RemovePublishedNVMeSession(t *testing.T) {
	nh := NewNVMeHandler()
	volPubInfo := &VolumePublishInfo{}
	volPubInfo.NVMeSubsystemNQN = testSubsystem1.NQN

	// Uninitialized published sessions case.
	var pubSessions *NVMeSessions
	nh.RemovePublishedNVMeSession(pubSessions, testSubsystem1.NQN)

	assert.Nil(t, pubSessions, "Published sessions initialized.")

	// Initialized published sessions case.
	pubSessions = NewNVMeSessions()
	nh.AddPublishedNVMeSession(pubSessions, volPubInfo)
	nh.RemovePublishedNVMeSession(pubSessions, testSubsystem1.NQN)

	assert.False(t, pubSessions.CheckNVMeSessionExists(testSubsystem1.NQN), "NVMe session not deleted.")
}

func TestNVMeHandler_InspectNVMeSessions_EmptyPublishedSessions(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()

	subs := nh.InspectNVMeSessions(ctx(), pubSessions, nil)

	assert.Equal(t, 0, len(subs), "Got few subsystems even though nothing is published.")
}

func TestNVMeHandler_InspectNVMeSessions_NilPublishedSessionData(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	pubSessions.Info[testSubsystem1.NQN] = nil

	subs := nh.InspectNVMeSessions(ctx(), pubSessions, nil)

	assert.Equal(t, 0, len(subs), "Valid published session found.")
}

func TestNVMeHandler_InspectNVMeSessions_PublishedNotPresentInCurrent(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})

	subs := nh.InspectNVMeSessions(ctx(), pubSessions, currSessions)

	assert.Equal(t, 0, len(subs), "Published session found in current session list.")
}

func TestNVMeHandler_InspectNVMeSessions_NilCurrentSessionData(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	currSessions.Info[testSubsystem1.NQN] = nil

	subs := nh.InspectNVMeSessions(ctx(), pubSessions, currSessions)

	assert.Equal(t, 0, len(subs), "Published session found in current with a valid entry.")
}

func TestNVMeHandler_InspectNVMeSessions_DisconnectedSubsystem(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	currSessions.AddNVMeSession(NVMeSubsystem{NQN: testSubsystem1.NQN}, []string{})

	subs := nh.InspectNVMeSessions(ctx(), pubSessions, currSessions)

	assert.Equal(t, 0, len(subs), "Disconnected subsystem present in subsystems to fix.")
}

func TestNVMeHandler_InspectNVMeSessions_PartiallyConnectedSubsystem(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()
	currSessions := NewNVMeSessions()
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	currSessions.AddNVMeSession(testSubsystem1, []string{})

	subs := nh.InspectNVMeSessions(ctx(), pubSessions, currSessions)

	assert.Equal(t, 1, len(subs), "No subsystems found.")
	assert.Equal(t, testSubsystem1, subs[0], "No subsystems found which needs remediation.")
	assert.Equal(t, ConnectOp, pubSessions.Info[testSubsystem1.NQN].Remediation, "Remediation not set.")
}

func TestNVMeHandler_RectifyNVMeSession(t *testing.T) {
	nh := NewNVMeHandler()
	pubSessions := NewNVMeSessions()

	// Empty published sessions case.
	nh.RectifyNVMeSession(ctx(), testSubsystem1, pubSessions)

	// Nil published session data case.
	pubSessions.Info[testSubsystem1.NQN] = nil
	nh.RectifyNVMeSession(ctx(), testSubsystem1, pubSessions)

	// NoOp remediation case.
	pubSessions.RemoveNVMeSession(testSubsystem1.NQN)
	pubSessions.AddNVMeSession(testSubsystem1, []string{})
	nh.RectifyNVMeSession(ctx(), testSubsystem1, pubSessions)
}

func TestNVMeHandler_PopulateCurrentNVMeSessions_NilCurrentSessions(t *testing.T) {
	nh := NewNVMeHandler()

	err := nh.PopulateCurrentNVMeSessions(ctx(), nil)

	assert.ErrorContains(t, err, "current NVMeSessions not initialized",
		"Populated current sessions successfully.")
}
