// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/logging"
)

func TestWrapNodeRegistrationFailure(t *testing.T) {
	t.Parallel()

	assert.NoError(t, wrapNodeRegistrationFailure(nil))

	cause := fmt.Errorf("connection reset")
	wrapped := wrapNodeRegistrationFailure(cause)
	require.Error(t, wrapped)
	assert.ErrorIs(t, wrapped, ErrNodeRegistration)
	assert.ErrorIs(t, wrapped, cause)
}

func TestRegistrationFailureLogFields_IncludesCorrelationFields(t *testing.T) {
	t.Parallel()

	ctx := logging.GenerateRequestContext(
		context.Background(), "csi-reg-attempt-42", logging.ContextSourceCSI,
		logging.WorkflowNodeCreate, logging.LogLayerCSIFrontend,
	)
	err := wrapNodeRegistrationFailure(fmt.Errorf("registration failed"))

	fields := registrationFailureLogFields(ctx, "worker-1", err, 15*time.Second)

	assert.Equal(t, "worker-1", fields["node"])
	assert.Equal(t, "csi-reg-attempt-42", fields["requestID"])
	assert.Equal(t, 15*time.Second, fields["increment"])
	assert.Equal(t, err, fields["error"])
}
