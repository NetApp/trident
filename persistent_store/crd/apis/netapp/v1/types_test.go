// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MustEncode(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}

	return b
}

// TestFinalizers validates the interface behavior for TridentCRD objects
func TestFinalizers(t *testing.T) {
	objectMetaWithFinalizers := metav1.ObjectMeta{Finalizers: GetTridentFinalizers()}
	crdsWithFinalizers := []TridentCRD{
		&TridentBackend{ObjectMeta: objectMetaWithFinalizers},
		&TridentNode{ObjectMeta: objectMetaWithFinalizers},
		&TridentStorageClass{ObjectMeta: objectMetaWithFinalizers},
		&TridentTransaction{ObjectMeta: objectMetaWithFinalizers},
		&TridentVersion{ObjectMeta: objectMetaWithFinalizers},
		&TridentVolume{ObjectMeta: objectMetaWithFinalizers},
	}
	for _, crd := range crdsWithFinalizers {
		// make sure the finalizers are on the instance
		if len(crd.GetFinalizers()) == 0 {
			t.Fatalf("expected CRD interface object instance to have finalizers")
		}
		if !crd.HasTridentFinalizers() {
			t.Fatalf("expected CRD interface object instance to have finalizers")
		}

		// remove the finalizers
		crd.RemoveTridentFinalizers()

		// make sure they are gone
		if crd.HasTridentFinalizers() {
			t.Fatalf("expected CRD interface object instance to have its finalizers removed")
		}
		if len(crd.GetFinalizers()) != 0 {
			t.Fatalf("expected CRD interface object instance to have its finalizers removed")
		}
	}
}
