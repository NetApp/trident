// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/testutils"
)

func TestGetError(t *testing.T) {
	e := GetError(nil, nil)

	testutils.AssertEqual(t, "Strings not equal",
		"failed", e.(ZapiError).Status())

	testutils.AssertEqual(t, "Strings not equal",
		azgo.EINTERNALERROR, e.(ZapiError).Code())

	testutils.AssertEqual(t, "Strings not equal",
		"unexpected nil ZAPI result", e.(ZapiError).Reason())
}
