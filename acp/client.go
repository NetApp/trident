// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/netapp/trident/acp/rest"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/version"
)

var (
	initialInterval = 1 * time.Second
	multiplier      = 1.414 // approx sqrt(2)
	maxInterval     = 24 * time.Second
	randomFactor    = 0.1
	maxElapsedTime  = 60 * time.Second
)

// tridentACP is a global API for interacting with Trident-ACP. It is a singleton.
var tridentACP TridentACP

// Initialize creates a singleton Trident-ACP API instance.
// This should only be called once during startup.
func Initialize(url string, enabled bool, timeout time.Duration) {
	tridentACP = newClient(rest.NewClient(url, timeout), enabled)
}

// API returns a singleton Trident-ACP API client.
func API() TridentACP {
	return tridentACP
}

// SetAPI should only be used for testing.
// It enables testing by replacing the package-level Trident-ACP API with a mock ACP API.
// This should not be used anywhere outside unit tests.
func SetAPI(acp TridentACP) {
	tridentACP = acp
}

// client is a mediator between Trident and the direct Trident-ACP REST API.
type client struct {
	restClient REST
	acpEnabled bool
}

// newClient builds a Trident-ACP client using package-level configuration state.
func newClient(restAPI REST, acpEnabled bool) TridentACP {
	return &client{restAPI, acpEnabled}
}

func (c *client) Enabled() bool {
	return c.acpEnabled
}

func (c *client) GetVersion(ctx context.Context) (*version.Version, error) {
	Logc(ctx).Debug("Getting Trident-ACP version.")

	if !c.Enabled() {
		Logc(ctx).Debug("ACP is not enabled.")
		return nil, errors.UnsupportedError("ACP is not enabled")
	}

	acpVersion, err := c.restClient.GetVersion(ctx)
	if err != nil {
		Logc(ctx).Error("Could not get Trident-ACP version.")
		return nil, err
	}

	if acpVersion == nil {
		Logc(ctx).Error("No version in response from Trident-ACP REST API.")
		return nil, fmt.Errorf("no version in response from Trident-ACP REST API")
	}

	Logc(ctx).WithField("version", acpVersion.String()).Debug("Received Trident-ACP version.")
	return acpVersion, nil
}

func (c *client) GetVersionWithBackoff(ctx context.Context) (*version.Version, error) {
	Logc(ctx).Debug("Checking if Trident-ACP REST API is available.")
	var v *version.Version
	var err error

	if !c.Enabled() {
		Logc(ctx).Debug("ACP is not enabled.")
		return nil, errors.UnsupportedError("ACP is not enabled")
	}

	getVersion := func() error {
		v, err = c.restClient.GetVersion(ctx)
		return err
	}

	getVersionBackoff := backoff.NewExponentialBackOff()
	getVersionBackoff.InitialInterval = initialInterval
	getVersionBackoff.Multiplier = multiplier
	getVersionBackoff.MaxInterval = maxInterval
	getVersionBackoff.RandomizationFactor = randomFactor
	getVersionBackoff.MaxElapsedTime = maxElapsedTime

	getVersionNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
		}).Debug("Could not get Trident-ACP version; retrying...")
	}

	if err = backoff.RetryNotify(getVersion, getVersionBackoff, getVersionNotify); err != nil {
		Logc(ctx).WithError(err).Error("Unable to communicate with Trident-ACP REST API.")
		return nil, err
	} else if v == nil {
		Logc(ctx).Error("No version in response from Trident-ACP REST API.")
		return nil, fmt.Errorf("no version in response from Trident-ACP REST API")
	}

	Logc(ctx).WithField("version", v.String()).Debug("The Trident-ACP REST API is responsive.")
	return v, nil
}

func (c *client) IsFeatureEnabled(ctx context.Context, feature string) error {
	fields := LogFields{"feature": feature}
	Logc(ctx).WithFields(fields).Debug("Checking if feature is enabled.")

	if !c.Enabled() {
		Logc(ctx).WithFields(fields).Warning("Feature requires Trident-ACP to be enabled.")
		return errors.UnsupportedConfigError("acp is not enabled")
	}

	// Entitled will return different errors based on the response from the API call. Return the exact error
	// so consumers of this client may act on certain error conditions.
	if err := c.restClient.Entitled(ctx, feature); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Feature enablement failed.")
		return err
	}

	Logc(ctx).WithFields(fields).Debug("Feature is enabled.")
	return nil
}
