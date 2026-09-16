// Copyright 2026 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"testing"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
)

func withUncachedK8SClients(t *testing.T) {
	t.Helper()
	prev := createK8SClients
	createK8SClients = k8sclient.BuildK8SClients
	t.Cleanup(func() { createK8SClients = prev })
}
