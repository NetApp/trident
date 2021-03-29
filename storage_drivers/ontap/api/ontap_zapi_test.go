// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

func TestGetError(t *testing.T) {
	e := GetError(context.Background(), nil, nil)

	assert.Equal(t, "failed", e.(ZapiError).Status(), "Strings not equal")

	assert.Equal(t, azgo.EINTERNALERROR, e.(ZapiError).Code(), "Strings not equal")

	assert.Equal(t, "unexpected nil ZAPI result", e.(ZapiError).Reason(), "Strings not equal")
}
