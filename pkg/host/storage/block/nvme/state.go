// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"sort"
	"time"

	"github.com/netapp/trident/internal/fiji"
)

// duringAddPublishedNVMeSession is the fault point of Sessions.AddPublished. It is typed as the interface so
// that tests can replace it in both fiji and non-fiji builds.
var duringAddPublishedNVMeSession fiji.Injector = fiji.Register("duringAddPublishedNVMeSession", "nvme")

// NVMeOperation is a data structure for NVMe self-healing operations.
type NVMeOperation int8

const (
	NoOp NVMeOperation = iota
	ConnectOp
)

// SessionData contains all the information related to one published NVMe session: the subsystem NQN, the
// backend target IPs (data LIFs for ONTAP), last access time and remediation. Last access time is used in
// self-healing so that newer sessions get prioritised. It holds no host state; the driver reads that from the
// NVMe service when it inspects the session.
type SessionData struct {
	NQN        string
	Namespaces map[string]bool
	// namespaceTargetIPs records the target IPs each published namespace was published with.
	// NVMeTargetIPs is a subsystem-wide set, so removing a namespace may only drop the IPs that
	// no remaining namespace still claims.
	namespaceTargetIPs map[string][]string
	NVMeTargetIPs      []string
	LastAccessTime     time.Time
	Remediation        NVMeOperation
}

// Sessions is a map of subsystem NQN and SessionData used for tracking self-healing information. The caller
// owns it and any locking around it.
type Sessions struct {
	Info map[string]*SessionData
}

// NewSessionData returns a SessionData for the subsystem and target IPs with default values.
func NewSessionData(nqn string, targetIPs []string) *SessionData {
	return &SessionData{
		NQN:            nqn,
		NVMeTargetIPs:  append([]string(nil), targetIPs...),
		LastAccessTime: time.Now(),
		Remediation:    NoOp,
	}
}

// SetRemediation updates the remediation value.
func (sd *SessionData) SetRemediation(op NVMeOperation) {
	sd.Remediation = op
}

// IsTargetIPPresent reports whether the target IP is in the session.
func (sd *SessionData) IsTargetIPPresent(ip string) bool {
	for _, existingIP := range sd.NVMeTargetIPs {
		if existingIP == ip {
			return true
		}
	}
	return false
}

// AddTargetIP appends a target IP to the session.
func (sd *SessionData) AddTargetIP(ip string) {
	sd.NVMeTargetIPs = append(sd.NVMeTargetIPs, ip)
}

// RemoveTargetIP removes all occurrences of an NVMe target IP.
func (sd *SessionData) RemoveTargetIP(ip string) {
	targetIPs := make([]string, 0, len(sd.NVMeTargetIPs))
	for _, existingIP := range sd.NVMeTargetIPs {
		if existingIP != ip {
			targetIPs = append(targetIPs, existingIP)
		}
	}
	sd.NVMeTargetIPs = targetIPs
}

// claimTargetIPs records the target IPs a namespace was published with.
func (sd *SessionData) claimTargetIPs(nsUUID string, targetIPs []string) {
	if sd.namespaceTargetIPs == nil {
		sd.namespaceTargetIPs = make(map[string][]string)
	}
	previousClaim := sd.namespaceTargetIPs[nsUUID]
	sd.namespaceTargetIPs[nsUUID] = append([]string(nil), targetIPs...)

	// Drop any previously claimed IP this namespace no longer declares and no other namespace needs.
	for _, ip := range previousClaim {
		if sd.isTargetIPClaimed(ip) {
			continue
		}
		sd.RemoveTargetIP(ip)
	}
}

// releaseTargetIPs drops a namespace's target IP claims and removes every released IP that no
// remaining namespace still claims.
func (sd *SessionData) releaseTargetIPs(nsUUID string) {
	released, ok := sd.namespaceTargetIPs[nsUUID]
	if !ok {
		return
	}
	delete(sd.namespaceTargetIPs, nsUUID)

	for _, ip := range released {
		if sd.isTargetIPClaimed(ip) {
			continue
		}
		sd.RemoveTargetIP(ip)
	}
}

// isTargetIPClaimed reports whether any published namespace still needs the given target IP.
func (sd *SessionData) isTargetIPClaimed(ip string) bool {
	for _, claimedIPs := range sd.namespaceTargetIPs {
		for _, claimedIP := range claimedIPs {
			if claimedIP == ip {
				return true
			}
		}
	}
	return false
}

// NewSessions initializes and returns an empty Sessions.
func NewSessions() *Sessions {
	return &Sessions{Info: make(map[string]*SessionData)}
}

// AddSession adds a new session to the session map. If the session exists, target IPs it lacks are added.
func (s *Sessions) AddSession(nqn string, targetIPs []string) {
	if s.Info == nil {
		s.Info = make(map[string]*SessionData)
	}

	sd, ok := s.Info[nqn]
	if !ok || sd == nil {
		s.Info[nqn] = NewSessionData(nqn, targetIPs)
		return
	}

	// Session already present, unlike the first publish of a subsystem. Check if NVMeTargetIPs present in the
	// existing session data match with the targetIPs sent in this function and update any missing IP.
	for _, ip := range targetIPs {
		if !sd.IsTargetIPPresent(ip) {
			sd.AddTargetIP(ip)
		}
	}
}

// RemoveTargetIPsFromSession drops the given target IPs from a subsystem's session. AddSession
// only ever merges IPs in, so without this a target IP stays in the session for the life of the
// process and self-healing keeps trying to connect to a data LIF that no longer serves the volume.
func (s *Sessions) RemoveTargetIPsFromSession(nqn string, ips []string) {
	if s == nil || s.IsEmpty() {
		return
	}

	session, ok := s.Info[nqn]
	if !ok || session == nil {
		// No session found for given subsystem.
		return
	}

	for _, ip := range ips {
		session.RemoveTargetIP(ip)
	}
}

// AddNamespaceToSession adds the Namespace UUID to the list of Namespaces for session and records the target IPs it
// was published with, so RemoveNamespaceFromSession can release them.
func (s *Sessions) AddNamespaceToSession(nqn, nsUUID string, targetIPs []string) bool {
	if s == nil || s.IsEmpty() {
		return false
	}

	session, ok := s.Info[nqn]
	if !ok || session == nil {
		// No session found for given subsystem.
		return false
	}
	// add namespace to the session
	if session.Namespaces == nil {
		session.Namespaces = make(map[string]bool)
	}
	session.Namespaces[nsUUID] = true
	session.claimTargetIPs(nsUUID, targetIPs)
	return true
}

// RemoveNamespaceFromSession removes the Namespace UUID from the list of Namespaces for session.
func (s *Sessions) RemoveNamespaceFromSession(nqn, nsUUID string) {
	if s == nil || s.IsEmpty() {
		return
	}

	session, ok := s.Info[nqn]
	if !ok || session == nil {
		// No session found for given subsystem.
		return
	}
	// Remove namespace from the list to the session
	delete(session.Namespaces, nsUUID)
	session.releaseTargetIPs(nsUUID)
}

// GetNamespaceCountForSession gets the number of Namespaces associated with the given session.
func (s *Sessions) GetNamespaceCountForSession(nqn string) int {
	if s == nil || s.IsEmpty() {
		return 0
	}

	session, ok := s.Info[nqn]
	if !ok || session == nil {
		// No session found for given subsystem.
		return 0
	}

	return len(session.Namespaces)
}

// RemoveSession deletes the session of a subsystem NQN.
func (s *Sessions) RemoveSession(nqn string) {
	if !s.IsEmpty() {
		delete(s.Info, nqn)
	}
}

// SessionExists queries if a session is present using the subsystem NQN.
func (s *Sessions) SessionExists(nqn string) bool {
	if s.IsEmpty() {
		return false
	}

	_, ok := s.Info[nqn]
	return ok
}

// IsEmpty checks if the session map is empty.
func (s *Sessions) IsEmpty() bool {
	return s == nil || len(s.Info) == 0
}

// ResetRemediationForAll updates the remediation for all the sessions to NoOp.
func (s *Sessions) ResetRemediationForAll() {
	if s == nil {
		return
	}
	for _, sData := range s.Info {
		if sData != nil {
			sData.Remediation = NoOp
		}
	}
}

// SortByLastAccess sorts the subsystem NQNs in accordance to the LastAccessTime in the session data, oldest
// first, so the session that was healed least recently goes first. NQNs without session data sort first.
func (s *Sessions) SortByLastAccess(nqns []string) {
	if s.IsEmpty() || len(nqns) < 2 {
		return
	}

	lastAccess := func(nqn string) time.Time {
		if sd := s.Info[nqn]; sd != nil {
			return sd.LastAccessTime
		}
		return time.Time{}
	}
	sort.SliceStable(nqns, func(i, j int) bool {
		return lastAccess(nqns[i]).Before(lastAccess(nqns[j]))
	})
}

// AddPublished adds the published namespace to the session map, creating the subsystem's session if needed,
// and records the target IPs the namespace was published with.
func (s *Sessions) AddPublished(nqn, nsUUID string, targetIPs []string) {
	if s == nil {
		return
	}
	// This fiji point is only for testing exits, so no error check needed
	_ = duringAddPublishedNVMeSession.Inject()

	s.AddSession(nqn, targetIPs)
	s.AddNamespaceToSession(nqn, nsUUID, targetIPs)
}

// RemovePublished deletes the namespace from the subsystem's session. If the number of namespaces associated
// with the session comes down to 0, the session is deleted and last is true: the caller should disconnect the
// subsystem.
func (s *Sessions) RemovePublished(nqn, nsUUID string) (last bool) {
	if s == nil {
		return false
	}

	s.RemoveNamespaceFromSession(nqn, nsUUID)
	if s.GetNamespaceCountForSession(nqn) < 1 {
		last = true
		s.RemoveSession(nqn)
	}

	return last
}
