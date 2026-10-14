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

package bucketaccess

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
)

func TestManagedBySidecar(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// input parameters for target function.
		isHandedOffToSidecar                bool
		hasDeletionTimestamp                bool
		hasSidecarCleanupFinishedAnnotation bool
		// desired result
		want bool
	}{
		// expected real-world scenarios
		{name: "new BA",
			isHandedOffToSidecar:                false,
			hasDeletionTimestamp:                false,
			hasSidecarCleanupFinishedAnnotation: false,
			want:                                false,
		},
		{name: "BA handoff to sidecar",
			isHandedOffToSidecar:                true,
			hasDeletionTimestamp:                false,
			hasSidecarCleanupFinishedAnnotation: false,
			want:                                true,
		},
		{name: "sidecar-managed BA begins deleting",
			isHandedOffToSidecar:                true,
			hasDeletionTimestamp:                true,
			hasSidecarCleanupFinishedAnnotation: false,
			want:                                true,
		},
		{name: "controller hand-back after sidecar deletion cleanup",
			isHandedOffToSidecar:                true,
			hasDeletionTimestamp:                true,
			hasSidecarCleanupFinishedAnnotation: true,
			want:                                false,
		},
		{name: "BA deleted before sidecar handoff",
			isHandedOffToSidecar:                false,
			hasDeletionTimestamp:                true,
			hasSidecarCleanupFinishedAnnotation: false,
			want:                                false,
		},
		// degraded scenarios
		{name: "new BA, erroneous sidecar cleanup annotation",
			isHandedOffToSidecar:                false,
			hasDeletionTimestamp:                false,
			hasSidecarCleanupFinishedAnnotation: true, // erroneous
			want:                                false,
		},
		{name: "sidecar-managed BA, erroneous sidecar cleanup annotation",
			isHandedOffToSidecar:                true,
			hasDeletionTimestamp:                false,
			hasSidecarCleanupFinishedAnnotation: true, // erroneous
			want:                                true,
		},
		{name: "BA deleted before sidecar handoff, erroneous sidecar cleanup annotation",
			isHandedOffToSidecar:                false,
			hasDeletionTimestamp:                true,
			hasSidecarCleanupFinishedAnnotation: true, // erroneous
			want:                                false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &cosiapi.BucketAccess{
				ObjectMeta: meta.ObjectMeta{
					Name:      "my-access",
					Namespace: "tenant",
					Finalizers: []string{
						cosiapi.ProtectionFinalizer,
						"something-else",
					},
					Annotations: map[string]string{
						"user-annotation": "value",
						"key-only":        "",
					},
					CreationTimestamp: meta.NewTime(time.Now()),
					Generation:        2,
					UID:               types.UID("qwerty"),
				},
				Spec: cosiapi.BucketAccessSpec{
					BucketClaims: []cosiapi.BucketClaimAccess{
						{
							BucketClaimName:  "bc-1",
							AccessMode:       cosiapi.BucketAccessModeReadWrite,
							AccessSecretName: "bc-1-creds",
						},
					},
					BucketAccessClassName: "bac-standard",
					Protocol:              cosiapi.ObjectProtocolS3,
					ServiceAccountName:    "my-app",
				},
			}

			copy := base.DeepCopy()

			if tt.isHandedOffToSidecar {
				copy.Status.AccessedBuckets = []cosiapi.AccessedBucket{
					{
						BucketName:      "bc-asdfgh",
						BucketClaimName: "bc-1",
					},
				}
				copy.Status.DriverName = "some.driver.io"
				copy.Status.AuthenticationType = cosiapi.BucketAccessAuthenticationTypeKey
				copy.Status.Parameters = map[string]string{}
			}

			if tt.hasDeletionTimestamp {
				copy.DeletionTimestamp = &meta.Time{Time: time.Now()}
			}

			if tt.hasSidecarCleanupFinishedAnnotation {
				copy.Annotations[cosiapi.SidecarCleanupFinishedAnnotation] = ""
			}

			got := ManagedBySidecar(copy)
			assert.Equal(t, tt.want, got)

			// for all cases,applying the controller override annotation makes it controller-managed
			copy.Annotations[cosiapi.ControllerManagementOverrideAnnotation] = ""
			withOverride := ManagedBySidecar(copy)
			assert.False(t, withOverride)
		})
	}
}
