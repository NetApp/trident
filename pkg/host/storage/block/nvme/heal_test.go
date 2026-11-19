// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/config"
	adapter "github.com/netapp/trident/pkg/host/adapter/nvme"
	"github.com/netapp/trident/pkg/host/service/nvme"
	"github.com/netapp/trident/pkg/host/storage"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type connectArgs struct {
	nqn string
	ips []string
}

// fakeNVMe replaces the two Service methods the heal logic uses. Any other method panics on the nil
// embedded interface, which flags an unexpected host access.
type fakeNVMe struct {
	nvme.Service

	subsystems map[string]*nvme.Subsystem
	readErrors map[string]error
	connectErr error
	connects   []connectArgs
	reads      int
}

func (f *fakeNVMe) Subsystem(_ context.Context, nqn string) (*nvme.Subsystem, error) {
	f.reads++
	if err, ok := f.readErrors[nqn]; ok {
		return nil, err
	}
	if sub, ok := f.subsystems[nqn]; ok {
		return sub, nil
	}
	return nil, errors.NotFoundError("no subsystem paths found for NVMe subsystem %s", nqn)
}

func (f *fakeNVMe) ConnectPaths(_ context.Context, nqn string, ips []string) error {
	f.connects = append(f.connects, connectArgs{nqn: nqn, ips: append([]string(nil), ips...)})
	return f.connectErr
}

func livePath(ip string) *nvme.Path {
	return adapter.NewPathForTest("nvme-"+ip, ip, adapter.PathStateLive)
}

func newTestDriver(f *fakeNVMe) *Driver {
	return &Driver{nvme: f, healingLock: storage.NewCountingRWMutex(), driverContext: config.ContextCSI}
}

func TestDriver_inspectSessions_EmptyPublishedSessions(t *testing.T) {
	f := &fakeNVMe{}
	d := newTestDriver(f)

	assert.Empty(t, d.inspectSessions(context.Background(), NewSessions()))
	assert.Empty(t, d.inspectSessions(context.Background(), nil))
	assert.Zero(t, f.reads, "nothing published, so the host must not be read")
}

func TestDriver_inspectSessions_NilPublishedSessionData(t *testing.T) {
	f := &fakeNVMe{}
	d := newTestDriver(f)
	published := NewSessions()
	published.Info[testNQN1] = nil

	assert.Empty(t, d.inspectSessions(context.Background(), published), "Valid published session found.")
	assert.Zero(t, f.reads)
}

func TestDriver_inspectSessions_PublishedNotPresentOnHost(t *testing.T) {
	d := newTestDriver(&fakeNVMe{})
	published := NewSessions()
	published.AddSession(testNQN1, []string{"1.1.1.1"})

	assert.Empty(t, d.inspectSessions(context.Background(), published))
	assert.Equal(t, NoOp, published.Info[testNQN1].Remediation)
}

func TestDriver_inspectSessions_ReadErrorSkipsOnlyThatSubsystem(t *testing.T) {
	f := &fakeNVMe{
		readErrors: map[string]error{testNQN1: fmt.Errorf("sysfs unreadable")},
		subsystems: map[string]*nvme.Subsystem{
			testNQN2: adapter.NewSubsystemForTest(testNQN2, livePath("3.3.3.3")),
		},
	}
	d := newTestDriver(f)
	published := NewSessions()
	published.AddSession(testNQN1, []string{"1.1.1.1", "2.2.2.2"})
	published.AddSession(testNQN2, []string{"3.3.3.3", "4.4.4.4"})

	nqns := d.inspectSessions(context.Background(), published)

	assert.Equal(t, []string{testNQN2}, nqns)
	assert.Equal(t, NoOp, published.Info[testNQN1].Remediation)
	assert.Equal(t, ConnectOp, published.Info[testNQN2].Remediation)
}

func TestDriver_inspectSessions_PartiallyConnectedSubsystem(t *testing.T) {
	f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{
		testNQN1: adapter.NewSubsystemForTest(testNQN1, livePath("1.1.1.1")),
	}}
	d := newTestDriver(f)
	published := NewSessions()
	published.AddSession(testNQN1, []string{"1.1.1.1", "2.2.2.2"})

	nqns := d.inspectSessions(context.Background(), published)

	assert.Equal(t, []string{testNQN1}, nqns)
	assert.Equal(t, ConnectOp, published.Info[testNQN1].Remediation, "Remediation not set.")
}

func TestDriver_inspectSessions_AllDesiredPathsPresent(t *testing.T) {
	f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{
		testNQN1: adapter.NewSubsystemForTest(testNQN1, livePath("1.1.1.1")),
	}}
	d := newTestDriver(f)
	published := NewSessions()
	published.AddSession(testNQN1, []string{"1.1.1.1"})
	stale := time.Now().Add(-time.Hour)
	published.Info[testNQN1].LastAccessTime = stale

	nqns := d.inspectSessions(context.Background(), published)

	assert.Empty(t, nqns, "A subsystem with all desired paths should not need remediation.")
	assert.Equal(t, NoOp, published.Info[testNQN1].Remediation)
	assert.True(t, published.Info[testNQN1].LastAccessTime.After(stale), "LastAccessTime of a healthy session not updated.")
}

// A path in any state counts as present, because the kernel keeps trying to recover it.
func TestDriver_inspectSessions_NonLivePathCountsAsPresent(t *testing.T) {
	f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{
		testNQN1: adapter.NewSubsystemForTest(testNQN1,
			livePath("1.1.1.1"), adapter.NewPathForTest("nvme1", "2.2.2.2", adapter.PathStateConnecting)),
	}}
	d := newTestDriver(f)
	published := NewSessions()
	published.AddSession(testNQN1, []string{"1.1.1.1", "2.2.2.2"})

	assert.Empty(t, d.inspectSessions(context.Background(), published))
	assert.Equal(t, NoOp, published.Info[testNQN1].Remediation)
}

func TestDriver_inspectSessions_UnrelatedPathPresent(t *testing.T) {
	f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{
		testNQN1: adapter.NewSubsystemForTest(testNQN1, livePath("3.3.3.3")),
	}}
	d := newTestDriver(f)
	published := NewSessions()
	published.AddSession(testNQN1, []string{"2.2.2.2"})

	nqns := d.inspectSessions(context.Background(), published)

	assert.Equal(t, []string{testNQN1}, nqns, "An established subsystem should heal missing desired paths.")
	assert.Equal(t, ConnectOp, published.Info[testNQN1].Remediation)
}

// A subsystem the service returns without paths is treated as disconnected.
func TestDriver_inspectSessions_SubsystemWithoutPaths(t *testing.T) {
	f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{testNQN1: adapter.NewSubsystemForTest(testNQN1)}}
	d := newTestDriver(f)
	published := NewSessions()
	published.AddSession(testNQN1, []string{"1.1.1.1"})

	assert.Empty(t, d.inspectSessions(context.Background(), published),
		"Disconnected subsystem present in subsystems to fix.")
	assert.Equal(t, NoOp, published.Info[testNQN1].Remediation)
}

func TestDriver_inspectSessions_SortedByLastAccessTime(t *testing.T) {
	f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{}}
	published := NewSessions()
	base := time.Now()
	for i, nqn := range []string{"a", "b", "c", "d"} {
		f.subsystems[nqn] = adapter.NewSubsystemForTest(nqn, livePath("1.1.1.1"))
		published.AddSession(nqn, []string{"1.1.1.1", "2.2.2.2"})
		// Oldest last in map-insertion order: d, c, b, a ascending.
		published.Info[nqn].LastAccessTime = base.Add(time.Duration(-i) * time.Minute)
	}

	nqns := newTestDriver(f).inspectSessions(context.Background(), published)

	assert.Equal(t, []string{"d", "c", "b", "a"}, nqns)
}

func TestDriver_rectifySession(t *testing.T) {
	ctx := context.Background()

	t.Run("empty and nil sessions", func(t *testing.T) {
		f := &fakeNVMe{}
		d := newTestDriver(f)
		d.rectifySession(ctx, testNQN1, NewSessions())
		d.rectifySession(ctx, testNQN1, nil)
		assert.Empty(t, f.connects)
	})

	t.Run("nil session data", func(t *testing.T) {
		f := &fakeNVMe{}
		published := NewSessions()
		published.Info[testNQN1] = nil
		newTestDriver(f).rectifySession(ctx, testNQN1, published)
		assert.Empty(t, f.connects)
	})

	t.Run("unknown nqn", func(t *testing.T) {
		f := &fakeNVMe{}
		published := NewSessions()
		published.AddSession(testNQN2, []string{"1.1.1.1"})
		published.Info[testNQN2].Remediation = ConnectOp
		newTestDriver(f).rectifySession(ctx, testNQN1, published)
		assert.Empty(t, f.connects)
	})

	t.Run("no-op remediation updates access time only", func(t *testing.T) {
		f := &fakeNVMe{}
		published := NewSessions()
		published.AddSession(testNQN1, []string{"1.1.1.1"})
		stale := time.Now().Add(-time.Hour)
		published.Info[testNQN1].LastAccessTime = stale

		newTestDriver(f).rectifySession(ctx, testNQN1, published)

		assert.Empty(t, f.connects)
		assert.True(t, published.Info[testNQN1].LastAccessTime.After(stale))
	})

	t.Run("connect remediation connects the published target IPs", func(t *testing.T) {
		f := &fakeNVMe{}
		published := NewSessions()
		published.AddSession(testNQN1, []string{"1.1.1.1", "2.2.2.2"})
		published.Info[testNQN1].Remediation = ConnectOp
		stale := time.Now().Add(-time.Hour)
		published.Info[testNQN1].LastAccessTime = stale

		newTestDriver(f).rectifySession(ctx, testNQN1, published)

		assert.Equal(t, []connectArgs{{nqn: testNQN1, ips: []string{"1.1.1.1", "2.2.2.2"}}}, f.connects)
		assert.True(t, published.Info[testNQN1].LastAccessTime.After(stale))
	})

	t.Run("connect failure is logged and not returned", func(t *testing.T) {
		f := &fakeNVMe{connectErr: fmt.Errorf("connect failed")}
		published := NewSessions()
		published.AddSession(testNQN1, []string{"1.1.1.1"})
		published.Info[testNQN1].Remediation = ConnectOp

		newTestDriver(f).rectifySession(ctx, testNQN1, published)

		assert.Len(t, f.connects, 1)
	})
}

func TestDriver_heal(t *testing.T) {
	t.Run("connects the missing paths of published subsystems", func(t *testing.T) {
		f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{
			testNQN1: adapter.NewSubsystemForTest(testNQN1, livePath("1.1.1.1")),
			testNQN2: adapter.NewSubsystemForTest(testNQN2, livePath("3.3.3.3")),
		}}
		d := newTestDriver(f)
		d.sessionStore.AddPublished(testNQN1, "ns1", []string{"1.1.1.1", "2.2.2.2"})
		d.sessionStore.AddPublished(testNQN2, "ns2", []string{"3.3.3.3"})

		d.heal(context.Background())

		assert.Equal(t, []connectArgs{{nqn: testNQN1, ips: []string{"1.1.1.1", "2.2.2.2"}}}, f.connects)
	})

	t.Run("reads nothing when no session is published", func(t *testing.T) {
		f := &fakeNVMe{}

		newTestDriver(f).heal(context.Background())

		assert.Zero(t, f.reads)
	})
}

func TestDriver_StartStop(t *testing.T) {
	ctx := context.Background()

	t.Run("does not start when the interval is not positive", func(t *testing.T) {
		d := newTestDriver(&fakeNVMe{})

		assert.NoError(t, d.Start(ctx, &storage.HealConfig{}))
		assert.Nil(t, d.healingTicker)
		d.Stop()
	})

	t.Run("does not start outside CSI", func(t *testing.T) {
		d := newTestDriver(&fakeNVMe{})
		d.driverContext = config.ContextDocker

		assert.NoError(t, d.Start(ctx, &storage.HealConfig{Interval: time.Millisecond}))
		assert.Nil(t, d.healingTicker)
		d.Stop()
	})

	t.Run("heals until stopped", func(t *testing.T) {
		f := &fakeNVMe{subsystems: map[string]*nvme.Subsystem{
			testNQN1: adapter.NewSubsystemForTest(testNQN1, livePath("1.1.1.1")),
		}}
		d := newTestDriver(f)
		d.Register(ctx, "vol1", trackingInfo(testNQN1, "ns1", "1.1.1.1", "2.2.2.2"))

		require.NoError(t, d.Start(ctx, &storage.HealConfig{Interval: time.Millisecond, Delay: time.Millisecond}))
		assert.Eventually(t, func() bool {
			d.healingLock.RLock()
			defer d.healingLock.RUnlock()
			return len(f.connects) > 0
		}, time.Second, time.Millisecond)
		d.Stop()
	})
}

func TestDriver_Register(t *testing.T) {
	d := newTestDriver(&fakeNVMe{})

	d.Register(context.Background(), "vol1", trackingInfo(testNQN1, "ns1", "1.1.1.1"))

	assert.Equal(t, 1, d.sessionStore.GetNamespaceCountForSession(testNQN1))
	assert.Equal(t, []string{"1.1.1.1"}, d.sessionStore.Info[testNQN1].NVMeTargetIPs)
}

func trackingInfo(nqn, nsUUID string, targetIPs ...string) *models.VolumeTrackingInfo {
	info := &models.VolumeTrackingInfo{}
	info.NVMeSubsystemNQN = nqn
	info.NVMeNamespaceUUID = nsUUID
	info.NVMeTargetIPs = targetIPs
	return info
}

// attachNVMe fails the attach at the connect step.
type attachNVMe struct {
	detachNVMe
	connectErr error
}

func (*attachNVMe) PreChecks(context.Context) error { return nil }

func (n *attachNVMe) EnsureConnections(context.Context, string, []string) error { return n.connectErr }

func TestAttach_PublishedSessions(t *testing.T) {
	ctx := context.Background()
	spec := func() *models.VolumeAttachSpec {
		spec := &models.VolumeAttachSpec{}
		spec.NVMeSubsystemNQN = "nqn.sub"
		spec.NVMeNamespaceUUID = "ns-1"
		spec.NVMeTargetIPs = []string{"1.1.1.1"}
		return spec
	}
	connectErr := stderrors.New("connect failed")

	t.Run("a failed attach unpublishes and disconnects the subsystem", func(t *testing.T) {
		n := &attachNVMe{connectErr: connectErr}
		d := New(WithNVMe(n))

		_, err := d.Attach(ctx, "vol1", spec())

		require.ErrorIs(t, err, connectErr)
		assert.False(t, d.sessionStore.SessionExists("nqn.sub"))
		assert.Equal(t, []disconnectCall{{"nqn.sub", "ns-1", true}}, n.disconnects)
	})

	t.Run("a failed attach keeps the subsystem of another published namespace", func(t *testing.T) {
		n := &attachNVMe{connectErr: connectErr}
		d := New(WithNVMe(n))
		d.Register(ctx, "vol2", trackingInfo("nqn.sub", "ns-2", "1.1.1.1"))

		_, err := d.Attach(ctx, "vol1", spec())

		require.ErrorIs(t, err, connectErr)
		assert.Equal(t, 1, d.sessionStore.GetNamespaceCountForSession("nqn.sub"))
		assert.Empty(t, n.disconnects)
	})

	t.Run("a failed attach keeps a namespace it did not publish", func(t *testing.T) {
		n := &attachNVMe{connectErr: connectErr}
		d := New(WithNVMe(n))
		d.Register(ctx, "vol1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1"))

		_, err := d.Attach(ctx, "vol1", spec())

		require.ErrorIs(t, err, connectErr)
		assert.Equal(t, 1, d.sessionStore.GetNamespaceCountForSession("nqn.sub"))
		assert.Empty(t, n.disconnects)
	})
}
