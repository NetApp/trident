// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

// NewNamespaceForTest builds a Namespace without reading the host, so that callers in other packages
// can fake services that return namespaces in their tests. It must not be used in production code.
func NewNamespaceForTest(name, uuid string) *Namespace {
	return &Namespace{name: name, uuid: uuid}
}
