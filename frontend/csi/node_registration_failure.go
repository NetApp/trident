// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netapp/trident/logging"
)

// ErrNodeRegistration identifies node→controller registration failures in the error chain.
var ErrNodeRegistration = errors.New("node registration failed")

func wrapNodeRegistrationFailure(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrNodeRegistration, err)
}

func registrationFailureLogFields(
	ctx context.Context, nodeName string, err error, increment time.Duration,
) logging.LogFields {
	fields := logging.LogFields{
		"increment": increment,
		"error":     err,
	}
	if nodeName != "" {
		fields["node"] = nodeName
	}
	if ctx != nil {
		if reqID := ctx.Value(logging.ContextKeyRequestID); reqID != nil {
			if id := strings.TrimSpace(fmt.Sprint(reqID)); id != "" {
				fields["requestID"] = id
			}
		}
	}
	return fields
}
