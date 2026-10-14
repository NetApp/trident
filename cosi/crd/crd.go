// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package crd carries the COSI CustomResourceDefinitions Trident installs.
//
// The YAML is embedded rather than hand-transcribed so it stays byte-identical to the pinned
// upstream commit tracked in cosi/upstream.manifest. This package is inert: no behaviour, no
// imports from COSI or controller-runtime.
package crd

import (
	_ "embed"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// The pinned COSI API. Trident owns this schema; see design-docs/cosi.
const (
	// Group is the API group these CRDs define.
	Group = "objectstorage.k8s.io"

	// Version is the single served and stored version. Trident pins it and does not follow
	// upstream churn.
	Version = "v1alpha2"
)

// Ownership marking. Trident stamps every COSI CRD it creates, so a later install can tell its
// own CRD from one another COSI driver installed.
const (
	// ManagedByLabel follows the Kubernetes recommended label.
	ManagedByLabel = "app.kubernetes.io/managed-by"

	// ManagedByValue identifies Trident as the installer.
	ManagedByValue = "trident.netapp.io"
)

// ProtectionFinalizer is the finalizer COSI puts on BucketClaims, BucketAccesses, Buckets, and
// the credentials Secrets it writes. Nothing removes it once the reconcilers stop, so a
// namespace containing a BucketClaim will sit in Terminating forever.
const ProtectionFinalizer = "objectstorage.k8s.io/protection"

//go:embed objectstorage.k8s.io_buckets.yaml
var bucketsYAML string

//go:embed objectstorage.k8s.io_bucketclaims.yaml
var bucketClaimsYAML string

//go:embed objectstorage.k8s.io_bucketclasses.yaml
var bucketClassesYAML string

//go:embed objectstorage.k8s.io_bucketaccesses.yaml
var bucketAccessesYAML string

//go:embed objectstorage.k8s.io_bucketaccessclasses.yaml
var bucketAccessClassesYAML string

// CRD names, as they appear in metadata.name.
const (
	BucketCRDName            = "buckets." + Group
	BucketClaimCRDName       = "bucketclaims." + Group
	BucketClassCRDName       = "bucketclasses." + Group
	BucketAccessCRDName      = "bucketaccesses." + Group
	BucketAccessClassCRDName = "bucketaccessclasses." + Group
)

// Names lists every COSI CRD Trident installs, in dependency-free order. Returns a fresh slice
// so a caller cannot mutate the canonical list for everyone else.
func Names() []string {
	return []string{
		BucketCRDName,
		BucketClaimCRDName,
		BucketClassCRDName,
		BucketAccessCRDName,
		BucketAccessClassCRDName,
	}
}

// YAMLByName returns each CRD's YAML keyed by its metadata.name.
func YAMLByName() map[string]string {
	return map[string]string{
		BucketCRDName:            bucketsYAML,
		BucketClaimCRDName:       bucketClaimsYAML,
		BucketClassCRDName:       bucketClassesYAML,
		BucketAccessCRDName:      bucketAccessesYAML,
		BucketAccessClassCRDName: bucketAccessClassesYAML,
	}
}

// GetCRDsYAML returns every COSI CRD as one multi-document YAML string, matching the shape
// k8sclient.GetCRDsYAML returns for Trident's own CRDs.
func GetCRDsYAML() string {
	docs := make([]string, 0, len(Names()))
	byName := YAMLByName()

	for _, name := range Names() {
		docs = append(docs, strings.TrimPrefix(byName[name], "---\n"))
	}

	return "---\n" + strings.Join(docs, "\n---\n")
}

// GetCRDYAML returns the YAML for one CRD by name, exactly as vendored from upstream.
func GetCRDYAML(name string) (string, error) {
	doc, ok := YAMLByName()[name]
	if !ok {
		return "", fmt.Errorf("no COSI CRD named %q", name)
	}
	return doc, nil
}

// GetOwnedCRDYAML returns the CRD YAML with Trident's managed-by label applied.
//
// The label is added here rather than in the vendored file, so the file stays byte-identical
// to upstream. The document is round-tripped through a generic map, not the typed
// CustomResourceDefinition, so no field is dropped along the way.
func GetOwnedCRDYAML(name string) (string, error) {
	doc, err := GetCRDYAML(name)
	if err != nil {
		return "", err
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
		return "", fmt.Errorf("parse %s CRD: %w", name, err)
	}

	metadata, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("%s CRD has no metadata", name)
	}

	labels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		labels = map[string]interface{}{}
	}
	labels[ManagedByLabel] = ManagedByValue
	metadata["labels"] = labels

	out, err := yaml.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("render %s CRD: %w", name, err)
	}

	return string(out), nil
}

// IsTridentManaged reports whether a CRD already in the cluster carries Trident's ownership
// label. A false answer means another COSI provider installed it, and Trident must not modify it.
func IsTridentManaged(labels map[string]string) bool {
	return labels[ManagedByLabel] == ManagedByValue
}
