// Copyright 2021 NetApp, Inc. All Rights Reserved.

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

func TestInitFunctions(t *testing.T) {

	// the "github.com/netapp/trident/storage_drivers/ontap/api/rest/models" uses a lot of init()
	// the way they are generated within go-swagger, they can fail and cause our other unit tests
	// to fail as a side effect. this test exists as a quick sanity test to make sure those init
	// functions can be executed without generating panics.

	// this could be any class from the models package, it will still cause all of the init
	// functions within the package to be executed.  choosing this one as it has been a root
	// cause of failures.
	assert.NotNil(t, new(models.StorageBridge))
}
