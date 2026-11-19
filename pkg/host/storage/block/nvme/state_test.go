// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	testNQN1 = "nqn1"
	testNQN2 = "nqn2"
)

type countingInjector struct{ calls int }

func (c *countingInjector) Inject() error {
	c.calls++
	return nil
}

func TestSessions_AddSession(t *testing.T) {
	ns := Sessions{}

	ns.AddSession(testNQN1, []string{})
	sd := ns.Info[testNQN1]

	assert.True(t, ns.SessionExists(testNQN1), "Failed to add NVMe session.")
	assert.Equal(t, testNQN1, sd.NQN)
	assert.False(t, sd.IsTargetIPPresent("2.2.2.2"), "Target IP found.")

	// Adding the same subsystem with new targetIPs in the same session.
	ns.AddSession(testNQN1, []string{"2.2.2.2"})

	assert.True(t, ns.SessionExists(testNQN1), "Failed to add NVMe session.")
	assert.True(t, sd.IsTargetIPPresent("2.2.2.2"), "Target IP not found.")

	// A target IP already in the session is not added twice.
	ns.AddSession(testNQN1, []string{"2.2.2.2"})
	assert.Equal(t, []string{"2.2.2.2"}, sd.NVMeTargetIPs)
}

func TestSessionData_RemoveTargetIP(t *testing.T) {
	sessionData := &SessionData{NVMeTargetIPs: []string{"1.1.1.1", "2.2.2.2", "1.1.1.1"}}

	sessionData.RemoveTargetIP("1.1.1.1")

	assert.Equal(t, []string{"2.2.2.2"}, sessionData.NVMeTargetIPs)
}

// RemoveTargetIP must build a new slice rather than compact in place, because any other holder of
// the session's target IP slice would otherwise observe shifted entries.
func TestSessionData_RemoveTargetIPLeavesExistingReferenceIntact(t *testing.T) {
	sessionData := NewSessionData(testNQN1, []string{"1.1.1.1", "2.2.2.2"})
	existingReference := sessionData.NVMeTargetIPs

	sessionData.RemoveTargetIP("1.1.1.1")

	assert.Equal(t, []string{"2.2.2.2"}, sessionData.NVMeTargetIPs)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, existingReference,
		"compacting in place corrupted a pre-existing reference to the target IP slice")
}

// NewSessionData must copy the caller's slice. Retaining it lets a later AddTargetIP append
// into the caller's spare capacity and overwrite memory the caller still owns.
func TestSessionData_NewSessionDataCopiesTargetIPs(t *testing.T) {
	callerIPs := make([]string, 1, 4)
	callerIPs[0] = "1.1.1.1"

	sessionData := NewSessionData(testNQN1, callerIPs)
	sessionData.AddTargetIP("2.2.2.2")

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, sessionData.NVMeTargetIPs)
	assert.Equal(t, "", callerIPs[:cap(callerIPs)][1],
		"AddTargetIP wrote into the caller's spare capacity, so the constructor retained its slice")
	assert.Equal(t, NoOp, sessionData.Remediation)
	assert.False(t, sessionData.LastAccessTime.IsZero())
}

func TestSessions_RemoveSession(t *testing.T) {
	ns := Sessions{}
	assert.False(t, ns.SessionExists(testNQN1), "NVMe session found.")

	ns.AddSession(testNQN1, []string{})
	assert.True(t, ns.SessionExists(testNQN1), "NVMe session not found.")

	ns.RemoveSession(testNQN1)
	assert.False(t, ns.SessionExists(testNQN1), "NVMe session still exists.")
}

func TestSessions_IsEmpty(t *testing.T) {
	var nilSessions *Sessions
	assert.True(t, nilSessions.IsEmpty())
	assert.True(t, (&Sessions{}).IsEmpty())
	assert.True(t, NewSessions().IsEmpty())

	ns := NewSessions()
	ns.AddSession(testNQN1, nil)
	assert.False(t, ns.IsEmpty())
}

func TestSessions_ResetRemediationForAll(t *testing.T) {
	ns := NewSessions()

	ns.AddSession(testNQN1, []string{})
	sd := ns.Info[testNQN1]
	sd.SetRemediation(ConnectOp)
	ns.Info[testNQN2] = nil

	ns.ResetRemediationForAll()

	assert.Equal(t, NoOp, sd.Remediation, "Failed to update remediation.")

	var nilSessions *Sessions
	nilSessions.ResetRemediationForAll()
}

func TestSessions_SortByLastAccess(t *testing.T) {
	oldNQNs := []string{testNQN1, testNQN2}
	ns := NewSessions()
	newNQNs := make([]string, len(oldNQNs))
	copy(newNQNs, oldNQNs)

	ns.SortByLastAccess(newNQNs)

	assert.Equal(t, oldNQNs, newNQNs, "NQNs are not same.")

	// Adding the sessions and modifying their access times.
	for index, nqn := range oldNQNs {
		ns.AddSession(nqn, []string{})
		ns.Info[nqn].LastAccessTime = time.UnixMicro(int64(100 - index))
	}

	ns.SortByLastAccess(newNQNs)

	assert.Equal(t, oldNQNs[1], newNQNs[0], "NQN mismatch.")
	assert.Equal(t, oldNQNs[0], newNQNs[1], "NQN mismatch.")

	// Nil sessions and nil session data leave the order alone or sort first.
	var nilSessions *Sessions
	nilSessions.SortByLastAccess(newNQNs)
	ns.Info[testNQN2] = nil
	ns.SortByLastAccess(newNQNs)
	assert.Equal(t, []string{testNQN2, testNQN1}, newNQNs)
}

func TestSessions_AddPublished(t *testing.T) {
	// Uninitialized published sessions case.
	var pubSessions *Sessions
	pubSessions.AddPublished(testNQN1, "", nil)

	assert.Nil(t, pubSessions, "Published sessions initialized.")

	// Initialized published sessions case.
	pubSessions = NewSessions()
	pubSessions.AddPublished(testNQN1, "", nil)

	assert.True(t, pubSessions.SessionExists(testNQN1), "NVMe session not found.")
}

func TestSessions_AddPublishedInjectsFault(t *testing.T) {
	saved := duringAddPublishedNVMeSession
	defer func() { duringAddPublishedNVMeSession = saved }()
	injector := &countingInjector{}
	duringAddPublishedNVMeSession = injector

	NewSessions().AddPublished(testNQN1, "ns", []string{"1.1.1.1"})
	assert.Equal(t, 1, injector.calls)

	// A nil session map returns before the fault point.
	var pubSessions *Sessions
	pubSessions.AddPublished(testNQN1, "ns", []string{"1.1.1.1"})
	assert.Equal(t, 1, injector.calls)
}

func TestSessions_AddNamespaceToSession(t *testing.T) {
	var pubSessions *Sessions
	// Uninitialized published session case.
	assert.False(t, pubSessions.AddNamespaceToSession("testNQN", "testUUID", nil))
	c := pubSessions.GetNamespaceCountForSession("testNQN")
	assert.Equal(t, c, 0, "unexpected namespaces surfaced")

	// subsystem not found case
	pubSessions = NewSessions()
	pubSessions.Info["testNQN"] = &SessionData{
		Namespaces:     nil,
		NVMeTargetIPs:  nil,
		LastAccessTime: time.Time{},
		Remediation:    0,
	}
	assert.False(t, pubSessions.AddNamespaceToSession("testNQN-nonexistent", "testUUID", nil))
	c = pubSessions.GetNamespaceCountForSession("testNQN-nonexistent")
	assert.Zero(t, c, "expected no namespaces.")

	// add namespace case
	assert.True(t, pubSessions.AddNamespaceToSession("testNQN", "testUUID", nil))
	c = pubSessions.GetNamespaceCountForSession("testNQN")
	assert.Equal(t, c, 1, "expected only one namespace")

	// nil session data case
	pubSessions.Info["nilNQN"] = nil
	assert.False(t, pubSessions.AddNamespaceToSession("nilNQN", "testUUID", nil))
	assert.Zero(t, pubSessions.GetNamespaceCountForSession("nilNQN"))
}

func TestSessions_RemoveNamespaceFromSession(t *testing.T) {
	var pubSessions *Sessions
	// Uninitialized published session case.
	pubSessions.RemoveNamespaceFromSession("testNQN", "testUUID")

	// subsystem not found case
	pubSessions = NewSessions()
	pubSessions.Info["testNQN"] = &SessionData{}
	pubSessions.Info["nilNQN"] = nil
	pubSessions.RemoveNamespaceFromSession("testNQN-nonexistent", "testUUID")
	pubSessions.RemoveNamespaceFromSession("nilNQN", "testUUID")

	pubSessions.RemoveNamespaceFromSession("testNQN", "testUUID")
}

func TestSessions_RemoveTargetIPsFromSession(t *testing.T) {
	var pubSessions *Sessions
	// Uninitialized published session case.
	pubSessions.RemoveTargetIPsFromSession("testNQN", []string{"1.1.1.1"})

	// Subsystem not found case.
	pubSessions = NewSessions()
	pubSessions.AddSession(testNQN1, []string{"1.1.1.1", "2.2.2.2"})
	pubSessions.RemoveTargetIPsFromSession("testNQN-nonexistent", []string{"1.1.1.1"})

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, pubSessions.Info[testNQN1].NVMeTargetIPs,
		"Removed a target IP from the wrong subsystem.")

	// An IP that was never recorded is a no-op rather than an error.
	pubSessions.RemoveTargetIPsFromSession(testNQN1, []string{"9.9.9.9"})

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, pubSessions.Info[testNQN1].NVMeTargetIPs,
		"Removing an absent target IP changed the list.")

	// Removing leaves the surviving IPs, in order.
	pubSessions.AddSession(testNQN1, []string{"3.3.3.3"})
	pubSessions.RemoveTargetIPsFromSession(testNQN1, []string{"1.1.1.1", "3.3.3.3"})

	assert.Equal(t, []string{"2.2.2.2"}, pubSessions.Info[testNQN1].NVMeTargetIPs,
		"Unexpected target IPs remain.")
}

func TestSessions_RemovePublished(t *testing.T) {
	// Uninitialized published sessions case.
	var pubSessions *Sessions
	last := pubSessions.RemovePublished(testNQN1, "test")

	assert.Nil(t, pubSessions, "Published sessions initialized.")
	assert.False(t, last, "Don't disconnect subsystem.")

	// Initialized published sessions case.
	pubSessions = NewSessions()
	pubSessions.AddPublished(testNQN1, "", nil)
	last = pubSessions.RemovePublished(testNQN1, "")

	assert.False(t, pubSessions.SessionExists(testNQN1), "NVMe session not deleted.")
	assert.True(t, last, "Disconnect subsystem.")
}

// A published namespace's target IPs must outlive another namespace's removal, since
// NVMeTargetIPs is a subsystem-wide set that several namespaces can claim independently.
func TestSessions_RemovePublished_TargetIPOwnership(t *testing.T) {
	tests := map[string]struct {
		remainingTargetIPs []string
		expectedTargetIPs  []string
	}{
		"target IP claimed by a remaining namespace is kept": {
			remainingTargetIPs: []string{"1.1.1.1", "2.2.2.2"},
			expectedTargetIPs:  []string{"1.1.1.1", "2.2.2.2"},
		},
		"target IP claimed only by the departing namespace is dropped": {
			remainingTargetIPs: []string{"1.1.1.1"},
			expectedTargetIPs:  []string{"1.1.1.1"},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			pubSessions := NewSessions()

			pubSessions.AddPublished(testNQN1, "remaining-namespace", params.remainingTargetIPs)
			pubSessions.AddPublished(testNQN1, "departing-namespace", []string{"1.1.1.1", "2.2.2.2"})

			last := pubSessions.RemovePublished(testNQN1, "departing-namespace")

			assert.False(t, last, "A remaining namespace must prevent disconnect.")
			assert.Equal(t, params.expectedTargetIPs, pubSessions.Info[testNQN1].NVMeTargetIPs)
		})
	}
}

// Removing the last namespace drops the whole session, target IP claims included.
func TestSessions_RemovePublished_LastNamespaceRemovesSession(t *testing.T) {
	pubSessions := NewSessions()
	pubSessions.AddPublished(testNQN1, "only-namespace", []string{"1.1.1.1", "2.2.2.2"})

	last := pubSessions.RemovePublished(testNQN1, "only-namespace")

	assert.True(t, last, "Removing the last namespace must signal disconnect.")
	assert.False(t, pubSessions.SessionExists(testNQN1))
}

func TestSessions_RemovePublished_DoesNotMutateCallerSlice(t *testing.T) {
	pubSessions := NewSessions()
	targetIPs := []string{"1.1.1.1", "2.2.2.2"}

	pubSessions.AddPublished(testNQN1, "departing-namespace", targetIPs)
	pubSessions.AddPublished(testNQN1, "remaining-namespace", []string{"2.2.2.2"})

	pubSessions.RemovePublished(testNQN1, "departing-namespace")

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, targetIPs)
	assert.Equal(t, []string{"2.2.2.2"}, pubSessions.Info[testNQN1].NVMeTargetIPs)
}

func TestSessions_AddPublished_ShrinkingClaimDropsUnusedTargetIP(t *testing.T) {
	pubSessions := NewSessions()
	pubSessions.AddPublished(testNQN1, "namespace", []string{"1.1.1.1", "2.2.2.2"})

	pubSessions.AddPublished(testNQN1, "namespace", []string{"1.1.1.1"})

	assert.Equal(t, []string{"1.1.1.1"}, pubSessions.Info[testNQN1].NVMeTargetIPs)
}

func TestSessions_AddPublished_ShrinkingClaimKeepsSharedTargetIP(t *testing.T) {
	pubSessions := NewSessions()
	pubSessions.AddPublished(testNQN1, "first-namespace", []string{"1.1.1.1", "2.2.2.2"})
	pubSessions.AddPublished(testNQN1, "second-namespace", []string{"1.1.1.1", "2.2.2.2"})

	pubSessions.AddPublished(testNQN1, "first-namespace", []string{"1.1.1.1"})

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, pubSessions.Info[testNQN1].NVMeTargetIPs)
}

// A re-published namespace replaces its own claim, so a target IP it no longer declares is
// dropped once no other namespace claims it.
func TestSessions_AddPublished_ReplacesNamespaceTargetIPClaim(t *testing.T) {
	pubSessions := NewSessions()
	pubSessions.AddPublished(testNQN1, "namespace", []string{"1.1.1.1"})

	pubSessions.AddPublished(testNQN1, "namespace", []string{"1.1.1.1", "2.2.2.2"})

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, pubSessions.Info[testNQN1].NVMeTargetIPs)

	pubSessions.RemoveNamespaceFromSession(testNQN1, "namespace")

	assert.Empty(t, pubSessions.Info[testNQN1].NVMeTargetIPs)
}
