// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

func TestCheckMountOptions(t *testing.T) {
	type parameters struct {
		mountOptions string
		assertError  assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"valid option m1": {
			mountOptions: "m1",
			assertError:  assert.NoError,
		},
		"valid option sm2": {
			mountOptions: "sm2",
			assertError:  assert.NoError,
		},
		"no mount option": {
			mountOptions: "",
			assertError:  assert.NoError,
		},
		"invalid option m9": {
			mountOptions: "m9",
			assertError:  assert.Error,
		},
		"invalid option sm9": {
			mountOptions: "sm9",
			assertError:  assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mountInfo := models.MountInfo{
				MountId:      0,
				ParentId:     0,
				DeviceId:     "",
				Root:         "",
				MountPoint:   "",
				MountOptions: []string{"m1", "m2"},
				FsType:       "",
				MountSource:  "",
				SuperOptions: []string{"sm1", "sm2"},
			}

			res := checkMountOptions(context.Background(), mountInfo, params.mountOptions)
			params.assertError(t, res)
		})
	}
}
