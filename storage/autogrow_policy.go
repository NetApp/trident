// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"sync"

	"github.com/brunoga/deep"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/cache/generic_cache"
)

// AutogrowPolicyState represents the operational state of an Autogrow policy
type AutogrowPolicyState string

const (
	AutogrowPolicyStateSuccess  AutogrowPolicyState = "Success"
	AutogrowPolicyStateFailed   AutogrowPolicyState = "Failed"
	AutogrowPolicyStateDeleting AutogrowPolicyState = "Deleting"
)

// AutogrowPolicyConfig is the configuration for an Autogrow policy.
// This is separate from the K8s CR spec to avoid importing K8s types in core
type AutogrowPolicyConfig struct {
	Name          string              `json:"name"`
	UsedThreshold string              `json:"usedThreshold"`
	GrowthAmount  string              `json:"growthAmount"`
	MaxSize       string              `json:"maxSize"`
	State         AutogrowPolicyState `json:"state"`
}

// AutogrowPolicy represents the internal state of an Autogrow policy in the orchestrator
type AutogrowPolicy struct {
	name          string
	usedThreshold string
	growthAmount  string
	maxSize       string
	state         AutogrowPolicyState
	volumes       *generic_cache.SingleValueCache[string, bool] // Key: volume name, Value: true
	mutex         *sync.RWMutex
}

// NewAutogrowPolicy creates a new Autogrow policy object
func NewAutogrowPolicy(name, usedThreshold, growthAmount, maxSize string, state AutogrowPolicyState) *AutogrowPolicy {
	if state == "" {
		state = AutogrowPolicyStateSuccess
	}
	return &AutogrowPolicy{
		name:          name,
		usedThreshold: usedThreshold,
		growthAmount:  growthAmount,
		maxSize:       maxSize,
		state:         state,
		volumes:       generic_cache.NewSingleValueCache[string, bool](),
		mutex:         &sync.RWMutex{},
	}
}

func NewAutogrowPolicyFromConfig(config *AutogrowPolicyConfig) *AutogrowPolicy {
	return NewAutogrowPolicy(config.Name, config.UsedThreshold, config.GrowthAmount, config.MaxSize, config.State)
}

// State check helpers
func (s AutogrowPolicyState) IsSuccess() bool  { return s == AutogrowPolicyStateSuccess }
func (s AutogrowPolicyState) IsFailed() bool   { return s == AutogrowPolicyStateFailed }
func (s AutogrowPolicyState) IsDeleting() bool { return s == AutogrowPolicyStateDeleting }

// Name returns the policy name
func (p *AutogrowPolicy) Name() string {
	return p.name
}

// UsedThreshold returns the used threshold
func (p *AutogrowPolicy) UsedThreshold() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.usedThreshold
}

// GrowthAmount returns the growth amount
func (p *AutogrowPolicy) GrowthAmount() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.growthAmount
}

// MaxSize returns the max size
func (p *AutogrowPolicy) MaxSize() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.maxSize
}

// State returns the policy state
func (p *AutogrowPolicy) State() AutogrowPolicyState {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.state
}

// GetConfig returns the config representation
func (p *AutogrowPolicy) GetConfig() *AutogrowPolicyConfig {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return &AutogrowPolicyConfig{
		Name:          p.name,
		UsedThreshold: p.usedThreshold,
		GrowthAmount:  p.growthAmount,
		MaxSize:       p.maxSize,
		State:         p.state,
	}
}

// UpdateFromConfig updates the policy from a config
func (p *AutogrowPolicy) UpdateFromConfig(config *AutogrowPolicyConfig) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.usedThreshold = config.UsedThreshold
	p.growthAmount = config.GrowthAmount
	p.maxSize = config.MaxSize
	// Update state if provided
	if config.State != "" {
		p.state = config.State
	}
}

// ============================================================================
// Volume Association Methods
// ============================================================================

// AddVolume associates a volume with this policy
func (p *AutogrowPolicy) AddVolume(volumeName string) {
	_ = p.volumes.Set(volumeName, true)
}

// RemoveVolume disassociates a volume from this policy
func (p *AutogrowPolicy) RemoveVolume(volumeName string) {
	_ = p.volumes.DeleteKey(volumeName)
}

// HasVolume checks if a specific volume is associated with this policy
func (p *AutogrowPolicy) HasVolume(volumeName string) bool {
	return p.volumes.Has(volumeName)
}

// HasVolumes returns true if any volumes are using this policy
func (p *AutogrowPolicy) HasVolumes() bool {
	return p.volumes.Len() > 0
}

// GetVolumes returns a list of all volume names using this policy
func (p *AutogrowPolicy) GetVolumes() []string {
	volumeMap := p.volumes.List()
	volumes := make([]string, 0, len(volumeMap))
	for volumeName := range volumeMap {
		volumes = append(volumes, volumeName)
	}
	return volumes
}

// VolumeCount returns the number of volumes using this policy
func (p *AutogrowPolicy) VolumeCount() int {
	return p.volumes.Len()
}

// ClearVolumes removes all volume associations (for testing)
func (p *AutogrowPolicy) ClearVolumes() {
	p.volumes.Clear()
}

// SmartCopy returns the same AutogrowPolicy pointer.
// This is safe because AutogrowPolicy implements interior mutability with
// mutex-protected fields and a thread-safe generic_cache.SingleValueCache.
func (p *AutogrowPolicy) SmartCopy() interface{} {
	return deep.MustCopy(p)
}

// ============================================================================
// External and Persistent Representations
// ============================================================================

// AutogrowPolicyExternal is the external representation returned by API calls
type AutogrowPolicyExternal struct {
	Name          string              `json:"name"`
	UsedThreshold string              `json:"usedThreshold"`
	GrowthAmount  string              `json:"growthAmount"`
	MaxSize       string              `json:"maxSize"`
	State         AutogrowPolicyState `json:"state"`
	Volumes       []string            `json:"volumes"`
	VolumeCount   int                 `json:"volumeCount"`
}

// ConstructExternal creates an external representation of the policy
func (p *AutogrowPolicy) ConstructExternal(ctx context.Context) *AutogrowPolicyExternal {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	volumes := p.GetVolumes()

	Logc(ctx).WithFields(LogFields{
		"agPolicyName": p.name,
	}).Trace("Constructing external Autogrow policy.")

	return &AutogrowPolicyExternal{
		Name:          p.name,
		UsedThreshold: p.usedThreshold,
		GrowthAmount:  p.growthAmount,
		MaxSize:       p.maxSize,
		State:         p.state,
		Volumes:       volumes,
		VolumeCount:   len(volumes),
	}
}

// ConstructExternalWithVolumes creates an external representation with provided volume list.
// This is used by concurrent_core which queries volumes on-demand instead of caching them.
func (p *AutogrowPolicy) ConstructExternalWithVolumes(ctx context.Context, volumes []string) *AutogrowPolicyExternal {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	Logc(ctx).WithFields(LogFields{
		"agPolicyName": p.name,
	}).Trace("Constructing external autogrow policy with provided volumes.")

	return &AutogrowPolicyExternal{
		Name:          p.name,
		UsedThreshold: p.usedThreshold,
		GrowthAmount:  p.growthAmount,
		MaxSize:       p.maxSize,
		State:         p.state,
		Volumes:       volumes,
		VolumeCount:   len(volumes),
	}
}

// AutogrowPolicyPersistent is used to store policy in persistent store
type AutogrowPolicyPersistent struct {
	Name          string              `json:"name"`
	UsedThreshold string              `json:"usedThreshold"`
	GrowthAmount  string              `json:"growthAmount"`
	MaxSize       string              `json:"maxSize,omitempty"`
	State         AutogrowPolicyState `json:"state"`
}

// ConstructPersistent creates a persistent representation of the policy
func (p *AutogrowPolicy) ConstructPersistent() *AutogrowPolicyPersistent {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return &AutogrowPolicyPersistent{
		Name:          p.name,
		UsedThreshold: p.usedThreshold,
		GrowthAmount:  p.growthAmount,
		MaxSize:       p.maxSize,
		State:         p.state,
	}
}

// NewAutogrowPolicyFromPersistent reconstructs a policy from persistent storage
func NewAutogrowPolicyFromPersistent(persistent *AutogrowPolicyPersistent) *AutogrowPolicy {
	return NewAutogrowPolicy(
		persistent.Name,
		persistent.UsedThreshold,
		persistent.GrowthAmount,
		persistent.MaxSize,
		persistent.State,
	)
}
