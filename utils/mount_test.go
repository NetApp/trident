// Copyright 2022 Netapp Inc. All Rights Reserved.

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseProcSelfMountinfo(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expected    []MountInfo
		expectedErr error
	}{
		{
			name: "10-13 fields",
			content: `40 35 0:34 / /sys/fs/cgroup/cpu,cpuacct rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,cpu,cpuacct
46 35 0:40 / /sys/fs/cgroup/blkio rw,nosuid,nodev,noexec,relatime shared:21 - cgroup cgroup rw,blkio
47 35 0:41 / /sys/fs/cgroup/rdma rw,nosuid,nodev,noexec,relatime shared:22 master:1 - cgroup cgroup rw,rdma
48 35 0:42 / /sys/fs/cgroup/devices rw,nosuid,nodev,noexec,relatime shared:23 shared:74 master:2 - cgroup cgroup rw,devices`,
			expected: []MountInfo{
				{
					MountId:      40,
					ParentId:     35,
					DeviceId:     "0:34",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/cpu,cpuacct",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "cpu", "cpuacct"},
				},
				{
					MountId:      46,
					ParentId:     35,
					DeviceId:     "0:40",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/blkio",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "blkio"},
				},
				{
					MountId:      47,
					ParentId:     35,
					DeviceId:     "0:41",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/rdma",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "rdma"},
				},
				{
					MountId:      48,
					ParentId:     35,
					DeviceId:     "0:42",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/devices",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "devices"},
				},
			},
		},
		{
			name: "one valid one invalid",
			content: `36 35 0:30 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:10 - cgroup2 cgroup2 rw
47 35 0:41 / rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,rdma`,
			expectedErr: errors.New("wrong number of fields (expected at least 10, got 9): 47 35 0:41 / rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,rdma"),
		},
		{
			name:        "too few fields",
			content:     `36 35 0:30 / /sys/fs/cgroup/unified - cgroup2 cgroup2 rw`,
			expectedErr: errors.New("wrong number of fields (expected at least 10, got 9): 36 35 0:30 / /sys/fs/cgroup/unified - cgroup2 cgroup2 rw"),
		},
		{
			name:        "separator in 5th position",
			content:     `49 35 0:43 / /sys/fs/cgroup/pids rw,nosuid,nodev,noexec,relatime - shared:24 cgroup cgroup rw,pids`,
			expectedErr: errors.New("malformed mountinfo (could not find separator): 49 35 0:43 / /sys/fs/cgroup/pids rw,nosuid,nodev,noexec,relatime - shared:24 cgroup cgroup rw,pids"),
		},
		{
			name:        "no separator",
			content:     `52 26 0:12 / /sys/kernel/tracing rw,nosuid,nodev,noexec,relatime shared:27 tracefs tracefs rw`,
			expectedErr: errors.New("malformed mountinfo (could not find separator): 52 26 0:12 / /sys/kernel/tracing rw,nosuid,nodev,noexec,relatime shared:27 tracefs tracefs rw"),
		},
	}
	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			m, err := parseProcSelfMountinfo([]byte(tests[i].content))
			assert.Equal(t, tests[i].expectedErr, err)
			assert.Equal(t, tests[i].expected, m)
		})
	}
}
