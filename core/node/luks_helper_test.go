// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockTridentController "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_tridentcontroller"
)

func TestLUKSHelper_UsesControllerSetAfterNewCore(t *testing.T) {
	c := NewCore()
	assert.NotNil(t, c.nvmeDriver)

	client := mockTridentController.NewMockClient(gomock.NewController(t))
	client.EXPECT().UpdateVolume(gomock.Any(), "pvc-1", luksNamesUpdate("name-a")).Return(nil)
	c.SetController(&fakeController{MockClient: client})

	h := &luksHelper{core: c}
	assert.NoError(t, h.UpdateVolumeLUKSPassphraseNames(context.Background(), "pvc-1", []string{"name-a"}))
}

func TestLUKSHelper_NoController(t *testing.T) {
	h := &luksHelper{core: NewCore()}
	assert.Error(t, h.UpdateVolumeLUKSPassphraseNames(context.Background(), "pvc-1", []string{"name-a"}))
}
