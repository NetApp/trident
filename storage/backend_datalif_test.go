// Copyright 2026 NetApp, Inc. All Rights Reserved.

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	"github.com/netapp/trident/storage"
)

type reportingDriver struct {
	*mockstorage.MockDriver
	lifs []string
}

func (d *reportingDriver) DataLIFs() []string {
	return d.lifs
}

func TestStorageBackend_DataLIFs(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDriver := mockstorage.NewMockDriver(ctrl)

	plain := &storage.StorageBackend{}
	plain.SetDriver(mockDriver)
	assert.False(t, plain.CanRefreshDataAccess())
	assert.Nil(t, plain.DataLIFs())

	lifs := []string{"192.0.2.10", "192.0.2.11"}
	reporting := &storage.StorageBackend{}
	reporting.SetDriver(&reportingDriver{MockDriver: mockDriver, lifs: lifs})
	assert.True(t, reporting.CanRefreshDataAccess())
	assert.Equal(t, lifs, reporting.DataLIFs())

	unknown := &storage.StorageBackend{}
	unknown.SetDriver(&reportingDriver{MockDriver: mockDriver})
	assert.True(t, unknown.CanRefreshDataAccess())
	assert.Nil(t, unknown.DataLIFs())
}

func TestStorageBackend_ConstructPersistent_DataLIFs(t *testing.T) {
	lifs := []string{"192.0.2.10", "192.0.2.11"}

	tests := []struct {
		name     string
		enabled  bool
		driver   func(*mockstorage.MockDriver) storage.Driver
		expected *[]string
	}{
		{
			name:    "snapshot published when refresh is enabled",
			enabled: true,
			driver: func(m *mockstorage.MockDriver) storage.Driver {
				return &reportingDriver{MockDriver: m, lifs: lifs}
			},
			expected: &lifs,
		},
		{
			name:    "empty snapshot published as empty",
			enabled: true,
			driver: func(m *mockstorage.MockDriver) storage.Driver {
				return &reportingDriver{MockDriver: m, lifs: []string{}}
			},
			expected: &[]string{},
		},
		{
			name:    "unknown snapshot not published",
			enabled: true,
			driver: func(m *mockstorage.MockDriver) storage.Driver {
				return &reportingDriver{MockDriver: m}
			},
		},
		{
			name:    "non-reporting driver not published",
			enabled: true,
			driver:  func(m *mockstorage.MockDriver) storage.Driver { return m },
		},
		{
			name:    "nothing published when refresh is disabled",
			enabled: false,
			driver: func(m *mockstorage.MockDriver) storage.Driver {
				return &reportingDriver{MockDriver: m, lifs: lifs}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			previous := tridentconfig.EnableDataLIFRefresh
			tridentconfig.EnableDataLIFRefresh = tc.enabled
			t.Cleanup(func() { tridentconfig.EnableDataLIFRefresh = previous })

			mockDriver := mockstorage.NewMockDriver(gomock.NewController(t))
			mockDriver.EXPECT().StoreConfig(gomock.Any(), gomock.Any()).AnyTimes()
			backend := storage.NewTestStorageBackend()
			backend.SetDriver(tc.driver(mockDriver))

			persistent := backend.ConstructPersistent(context.Background())

			assert.Equal(t, tc.expected, persistent.DataLIFs)
		})
	}
}
