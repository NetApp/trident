/*
Copyright 2025 The Kubernetes Authors.

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

package reconciler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	controller "github.com/netapp/trident/cosi/controller"
)

func Test_validateAccessAgainstClass(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		class   *cosiapi.BucketAccessClassSpec
		access  *cosiapi.BucketAccessSpec
		wantErr bool
	}{
		{"key auth, disallow nothing",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeKey,
				MultiBucketAccess:  cosiapi.MultiBucketAccessMultipleBuckets,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "",
			},
			false,
		},
		{"key auth, default disallow multi-bucket",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeKey,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "",
			},
			true,
		},
		{"key auth, disallow multi-bucket",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeKey,
				MultiBucketAccess:  cosiapi.MultiBucketAccessSingleBucket,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "",
			},
			true,
		},
		{"key auth, disallow write modes",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeKey,
				MultiBucketAccess:  cosiapi.MultiBucketAccessMultipleBuckets,
				DisallowedBucketAccessModes: []cosiapi.BucketAccessMode{
					cosiapi.BucketAccessModeReadWrite,
					cosiapi.BucketAccessModeWriteOnly,
				},
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "",
			},
			true,
		},
		{"serviceaccount auth, sa given",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeServiceAccount,
				MultiBucketAccess:  cosiapi.MultiBucketAccessMultipleBuckets,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "my-sa",
			},
			false,
		},
		{"serviceaccount auth, no sa",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeServiceAccount,
				MultiBucketAccess:  cosiapi.MultiBucketAccessMultipleBuckets,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "",
			},
			true,
		},
		{"serviceaccount auth, default disallow multi-bucket",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeServiceAccount,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "my-sa",
			},
			true,
		},
		{"serviceaccount auth, disallow multi-bucket",
			&cosiapi.BucketAccessClassSpec{
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeServiceAccount,
				MultiBucketAccess:  cosiapi.MultiBucketAccessSingleBucket,
			},
			&cosiapi.BucketAccessSpec{
				BucketClaims: []cosiapi.BucketClaimAccess{
					{
						BucketClaimName:  "rw",
						AccessMode:       cosiapi.BucketAccessModeReadWrite,
						AccessSecretName: "rw",
					},
					{
						BucketClaimName:  "ro",
						AccessMode:       cosiapi.BucketAccessModeReadOnly,
						AccessSecretName: "ro",
					},
				},
				ServiceAccountName: "my-sa",
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := controller.ValidateAccessAgainstClass(tt.class, tt.access)
			if tt.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.NoError(t, gotErr)
			}
		})
	}
}
