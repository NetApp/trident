/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller contains utilities for unit testing that help approximate COSI Controller
// behaviors.
package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	controller "github.com/netapp/trident/cosi/controller"
	cositest "github.com/netapp/trident/cosi/third_party/test"
)

// ReconcileBucketClaim reconciles the BucketClaim with the given namespaced name for unit tests.
// BucketClaim should exist (if desired) in the bootstrapped dependencies's client.
func ReconcileBucketClaim(
	t *testing.T,
	bootstrapped *cositest.Dependencies,
	nsName types.NamespacedName,
) (*cosiapi.BucketClaim, error) {
	r := controller.BucketClaimReconciler{
		Client: bootstrapped.Client,
		Scheme: bootstrapped.Client.Scheme(),
	}

	_, err := r.Reconcile(bootstrapped.ContextWithLogger, ctrl.Request{NamespacedName: nsName})
	if err != nil {
		if err.Error() != "waiting for Bucket to be provisioned" {
			// to make tests consuming this easier, only return an error if it's not the one we
			// (currently) expect after a successful reconcile
			return nil, err
		}
	}

	claim := &cosiapi.BucketClaim{}
	if err := bootstrapped.Client.Get(bootstrapped.ContextWithLogger, nsName, claim); err != nil {
		return nil, err
	}
	return claim, nil
}

// ReconcileBucketAccess reconciles the BucketAccess with the given namespaced name for unit tests.
// BucketAccess should exist (if desired) in the bootstrapped dependencies's client.
func ReconcileBucketAccess(
	t *testing.T,
	bootstrapped *cositest.Dependencies,
	nsName types.NamespacedName,
) (*cosiapi.BucketAccess, error) {
	r := controller.BucketAccessReconciler{
		Client: bootstrapped.Client,
		Scheme: bootstrapped.Client.Scheme(),
	}

	_, err := r.Reconcile(bootstrapped.ContextWithLogger, ctrl.Request{NamespacedName: nsName})
	if err != nil {
		return nil, err
	}

	access := &cosiapi.BucketAccess{}
	if err := bootstrapped.Client.Get(bootstrapped.ContextWithLogger, nsName, access); err != nil {
		return nil, err
	}
	return access, nil
}
