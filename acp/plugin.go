// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/version"
)

var (
	initialInterval = 1 * time.Second
	multiplier      = 1.414 // approx sqrt(2)
	maxInterval     = 120 * time.Second
	randomFactor    = 0.1
	maxElapsedTime  = 300 * time.Second
)

// Activate reaches out to ACP REST API and attempts to establish a connection. It will retry for 5 minutes.
// It should be called by each member of Trident that must consume ACP APIs to complete premium workflows.
// It is up to the caller to decide whether this executes as a synchronous or asynchronous routine.
func (c *Client) Activate() error {
	var err error
	ctx := GenerateRequestContext(
		context.Background(), "", ContextSourceInternal, WorkflowPluginActivate, LogLayerUtils,
	)

	var v *version.Version
	activate := func() error {
		v, err = c.GetVersion(ctx)
		return err
	}

	activateBackoff := backoff.NewExponentialBackOff()
	activateBackoff.InitialInterval = initialInterval
	activateBackoff.Multiplier = multiplier
	activateBackoff.MaxInterval = maxInterval
	activateBackoff.RandomizationFactor = randomFactor
	activateBackoff.MaxElapsedTime = maxElapsedTime

	activateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
		}).Debug("Could not get trident-acp version; retrying...")
	}

	err = backoff.RetryNotify(activate, activateBackoff, activateNotify)
	if err != nil {
		Logc(ctx).WithError(err).Error("Unable to communicate with trident-acp REST API.")
		return err
	}

	Logc(ctx).WithField("version", v.String()).Info("The trident-acp REST API is responsive.")
	return nil
}

func (c *Client) Deactivate() error {
	// Close any idle connections if they exist.
	c.httpClient.CloseIdleConnections()
	return nil
}

func (c *Client) GetName() string {
	return appName
}

// Version returns the API client version.
func (c *Client) Version() string {
	return apiVersion
}
