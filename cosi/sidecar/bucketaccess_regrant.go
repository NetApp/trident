// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file is NETAPP LOCAL, absent from cosi/upstream.manifest, so resync.sh ignores it.

package reconciler

import (
	corev1 "k8s.io/api/core/v1"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
)

// accessAlreadyGranted reports whether a BucketAccess holds a completed S3 key grant: it is
// ready, records its account, and every one of its Secrets already carries both S3
// credentials. Anything short of that returns false, so the reconciler falls through to the
// upstream grant and the driver repairs the access.
//
// status.Error is deliberately not consulted. Reconcile records every failure there,
// including transient lookups that run before this check, without clearing readyToUse, so
// treating it as a failed grant would rotate a working key after any API blip. A grant that
// failed part way shows up instead as readyToUse unset or a Secret missing a credential.
func accessAlreadyGranted(access *cosiapi.BucketAccess, secretsByName map[string]*corev1.Secret) bool {
	status := access.Status
	if status.ReadyToUse == nil || !*status.ReadyToUse || status.AccountID == "" {
		return false
	}
	if status.AuthenticationType != cosiapi.BucketAccessAuthenticationTypeKey ||
		access.Spec.Protocol != cosiapi.ObjectProtocolS3 {
		return false
	}
	if len(secretsByName) == 0 {
		return false
	}

	for _, secret := range secretsByName {
		for _, key := range []cosiapi.CredentialVar{
			cosiapi.CredentialVar_S3_AccessKeyId,
			cosiapi.CredentialVar_S3_AccessSecretKey,
		} {
			if !secretHasValue(secret, string(key)) {
				return false
			}
		}
	}
	return true
}

// secretHasValue checks Data, which the API server returns, and StringData, which a Secret
// written by this process and not yet re-read (or held by a fake client) still carries.
func secretHasValue(secret *corev1.Secret, key string) bool {
	return len(secret.Data[key]) > 0 || secret.StringData[key] != ""
}
