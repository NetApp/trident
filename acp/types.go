// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

import (
	"context"

	"github.com/netapp/trident/utils/version"
)

// API represents a set of methods for talking with Trident-ACP REST APIs.
type API interface {
	Activate() error
	Deactivate() error
	GetName() string
	Version() string

	GetVersion(context.Context) (*version.Version, error)
	Entitled(context.Context, string) (bool, error)
}
