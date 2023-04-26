// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

type unixPermissionsTest struct {
	value          string
	expectedResult string
}

func TestConvertUnixPermissions(t *testing.T) {
	testValues := []unixPermissionsTest{
		{"---rwxrwxrwx", "777"},
		{"---rwxr-xr-x", "755"},
		{"---rw-rw-rw-", "666"},
		{"---r-xr-xr-x", "555"},
		{"---r--r--r--", "444"},
		{"----w--w--w-", "222"},
		{"------------", "000"},
		{"rwxrwxrwx", "777"},
	}

	for _, test := range testValues {
		assert.Equal(t,
			test.expectedResult,
			convertUnixPermissions(test.value),
			"Unexpected value",
		)
	}
}

func TestHasNextLink(t *testing.T) {
	////////////////////////////////////////////
	// negative tests
	assert.False(t,
		HasNextLink(nil),
		"Should NOT have a next link")

	assert.False(t,
		HasNextLink(APIResponse{}),
		"Should NOT have a next link")

	assert.False(t,
		HasNextLink(&models.VolumeResponse{
			Links: &models.VolumeResponseInlineLinks{},
		}),
		"Should NOT have a next link")

	////////////////////////////////////////////
	// positive tests
	assert.True(t,
		HasNextLink(
			&models.VolumeResponse{
				Links: &models.VolumeResponseInlineLinks{
					Next: &models.Href{
						Href: utils.Ptr("/api/storage/volumes?start.uuid=00c881eb-f36c-11e8-996b-00a0986e75a0&fields=%2A%2A&max_records=1&name=%2A&return_records=true&svm.name=SVM"),
					},
				},
			}),
		"SHOULD have a next link")

	assert.True(t,
		HasNextLink(
			&models.SnapshotResponse{
				Links: &models.SnapshotResponseInlineLinks{
					Next: &models.Href{
						Href: utils.Ptr("/api/storage/snapshot?start.uuid=00c881eb-f36c-11e8-996b-00a0986e75a0&fields=%2A%2A&max_records=1&name=%2A&return_records=true&svm.name=SVM"),
					},
				},
			}),
		"SHOULD have a next link")
}

func TestIsSnapshotBusyError(t *testing.T) {
	snapshotName := "testSnapshot"
	sourceVolume := "sourceVolume"
	err := SnapshotBusyError(fmt.Sprintf("snapshot %s backing volume %s is busy", snapshotName, sourceVolume))
	assert.True(t,
		IsSnapshotBusyError(err),
		"Should be a SnapshotBusyError")
}
