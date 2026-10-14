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

package protocol

import (
	"errors"
	"fmt"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"
)

type GcsProtocolGetter struct{}

func (GcsProtocolGetter) ApiProtocol() cosiapi.ObjectProtocol {
	return cosiapi.ObjectProtocolGcs
}

// GcsBucketInfoTranslator implements RpcApiTranslator for GCS bucket info.
type GcsBucketInfoTranslator struct {
	GcsProtocolGetter
}

var _ RpcApiTranslator[*cosiproto.GcsBucketInfo, cosiapi.BucketInfoVar] = GcsBucketInfoTranslator{}

// GcsCredentialTranslator implements RpcApiTranslator for GCS credentials.
type GcsCredentialTranslator struct {
	GcsProtocolGetter
}

var _ RpcApiTranslator[*cosiproto.GcsCredentialInfo, cosiapi.CredentialVar] = GcsCredentialTranslator{}

func (GcsBucketInfoTranslator) RpcToApi(b *cosiproto.GcsBucketInfo) map[cosiapi.BucketInfoVar]string {
	if b == nil {
		return nil
	}

	out := map[cosiapi.BucketInfoVar]string{
		cosiapi.BucketInfoVar_GCS_BucketName: b.BucketName,
		cosiapi.BucketInfoVar_GCS_ProjectId:  b.ProjectId,
	}

	return out
}

func (GcsBucketInfoTranslator) ApiToRpc(vars map[cosiapi.BucketInfoVar]string) *cosiproto.GcsBucketInfo {
	if len(vars) == 0 {
		return nil
	}

	out := &cosiproto.GcsBucketInfo{}

	out.BucketName = vars[cosiapi.BucketInfoVar_GCS_BucketName]
	out.ProjectId = vars[cosiapi.BucketInfoVar_GCS_ProjectId]

	return out
}

func (GcsBucketInfoTranslator) Validate(
	vars map[cosiapi.BucketInfoVar]string, _ cosiapi.BucketAccessAuthenticationType,
) error {
	errs := []error{}

	bucketName := vars[cosiapi.BucketInfoVar_GCS_BucketName]
	if bucketName == "" {
		errs = append(errs, fmt.Errorf("GCS bucket name cannot be unset"))
	}

	projectId := vars[cosiapi.BucketInfoVar_GCS_ProjectId]
	if projectId == "" {
		errs = append(errs, fmt.Errorf("GCS project ID cannot be unset"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("GCS bucket info is invalid: %w", errors.Join(errs...))
	}
	return nil
}

func (GcsCredentialTranslator) RpcToApi(c *cosiproto.GcsCredentialInfo) map[cosiapi.CredentialVar]string {
	if c == nil {
		return nil
	}

	out := map[cosiapi.CredentialVar]string{
		cosiapi.CredentialVar_GCS_AccessId:       c.AccessId,
		cosiapi.CredentialVar_GCS_AccessSecret:   c.AccessSecret,
		cosiapi.CredentialVar_GCS_PrivateKeyName: c.PrivateKeyName,
		cosiapi.CredentialVar_GCS_ServiceAccount: c.ServiceAccount,
	}

	return out
}

func (GcsCredentialTranslator) ApiToRpc(vars map[cosiapi.CredentialVar]string) *cosiproto.GcsCredentialInfo {
	if len(vars) == 0 {
		return nil
	}

	out := &cosiproto.GcsCredentialInfo{}

	out.AccessId = vars[cosiapi.CredentialVar_GCS_AccessId]
	out.AccessSecret = vars[cosiapi.CredentialVar_GCS_AccessSecret]
	out.PrivateKeyName = vars[cosiapi.CredentialVar_GCS_PrivateKeyName]
	out.ServiceAccount = vars[cosiapi.CredentialVar_GCS_ServiceAccount]

	return out
}

func (GcsCredentialTranslator) Validate(
	vars map[cosiapi.CredentialVar]string, authType cosiapi.BucketAccessAuthenticationType,
) error {
	errs := []error{}

	switch authType {
	case cosiapi.BucketAccessAuthenticationTypeKey:
		accessId := vars[cosiapi.CredentialVar_GCS_AccessId]
		if accessId == "" {
			errs = append(errs, fmt.Errorf("GCS access ID cannot be unset"))
		}

		accessSecret := vars[cosiapi.CredentialVar_GCS_AccessSecret]
		if accessSecret == "" {
			errs = append(errs, fmt.Errorf("GCS access secret cannot be unset"))
		}

	case cosiapi.BucketAccessAuthenticationTypeServiceAccount:
		privateKeyName := vars[cosiapi.CredentialVar_GCS_PrivateKeyName]
		if privateKeyName == "" {
			errs = append(errs, fmt.Errorf("GCS private key name cannot be unset"))
		}

		serviceAccount := vars[cosiapi.CredentialVar_GCS_ServiceAccount]
		if serviceAccount == "" {
			errs = append(errs, fmt.Errorf("GCS service account cannot be unset"))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("GCS credential info is invalid: %w", errors.Join(errs...))
	}
	return nil
}
