// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestSubsystem() *Subsystem {
	return &Subsystem{
		nqn: "nqn.test",
		dir: "/sys/class/nvme-subsystem/nvme-subsys0",
		paths: []*Path{
			{name: "nvme0", address: "10.0.0.1", state: PathStateConnecting},
			{name: "nvme1", address: "10.0.0.2", state: PathStateLive},
		},
		namespaces: []*Namespace{{name: "nvme0n1", uuid: "uuid-1"}, {name: "nvme0n2", uuid: "uuid-2"}},
	}
}

func TestSubsystem_Accessors(t *testing.T) {
	sub := newTestSubsystem()

	assert.Equal(t, "nqn.test", sub.NQN())
	assert.Equal(t, "/sys/class/nvme-subsystem/nvme-subsys0", sub.Dir())
	assert.Len(t, sub.Paths(), 2)
	assert.Len(t, sub.Namespaces(), 2)
	assert.Equal(t, "nvme1", sub.Paths()[1].Name())
	assert.Equal(t, "uuid-2", sub.Namespaces()[1].UUID())
}

func TestSubsystem_SlicesAreCopies(t *testing.T) {
	sub := newTestSubsystem()

	paths := sub.Paths()
	paths[0] = nil
	_ = append(paths, &Path{name: "extra"})
	namespaces := sub.Namespaces()
	namespaces[0] = nil

	assert.NotNil(t, sub.Paths()[0])
	assert.Len(t, sub.Paths(), 2)
	assert.NotNil(t, sub.Namespaces()[0])
}

func TestSubsystem_EmptyViews(t *testing.T) {
	sub := &Subsystem{nqn: "nqn.test"}

	assert.Empty(t, sub.Paths())
	assert.Empty(t, sub.Namespaces())
	assert.NotNil(t, sub.Paths())
	assert.NotNil(t, sub.Namespaces())
	assert.False(t, sub.HasLivePath())
}

func TestSubsystem_HasLivePath(t *testing.T) {
	cases := map[string]struct {
		states []PathState
		want   bool
	}{
		"no_paths":            {nil, false},
		"one_live":            {[]PathState{PathStateLive}, true},
		"one_connecting":      {[]PathState{PathStateConnecting}, false},
		"live_among_others":   {[]PathState{PathStateConnecting, PathStateDeleting, PathStateLive}, true},
		"none_live":           {[]PathState{PathStateConnecting, PathStateDeleting, PathStateUnknown}, false},
		"unlisted_state":      {[]PathState{PathState("resetting"), PathState("dead")}, false},
		"all_live":            {[]PathState{PathStateLive, PathStateLive}, true},
		"unreadable_is_empty": {[]PathState{PathStateUnknown}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sub := &Subsystem{}
			for _, s := range tc.states {
				sub.paths = append(sub.paths, &Path{state: s})
			}
			assert.Equal(t, tc.want, sub.HasLivePath())
		})
	}
}
