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

type AzureProtocolGetter struct{}

func (AzureProtocolGetter) ApiProtocol() cosiapi.ObjectProtocol {
	return cosiapi.ObjectProtocolAzure
}

// AzureBucketInfoTranslator implements RpcApiTranslator for Azure bucket info.
type AzureBucketInfoTranslator struct {
	AzureProtocolGetter
}

var _ RpcApiTranslator[*cosiproto.AzureBucketInfo, cosiapi.BucketInfoVar] = AzureBucketInfoTranslator{}

// AzureCredentialTranslator implements RpcApiTranslator for Azure credentials.
type AzureCredentialTranslator struct {
	AzureProtocolGetter
}

var _ RpcApiTranslator[*cosiproto.AzureCredentialInfo, cosiapi.CredentialVar] = AzureCredentialTranslator{}

func (AzureBucketInfoTranslator) RpcToApi(b *cosiproto.AzureBucketInfo) map[cosiapi.BucketInfoVar]string {
	if b == nil {
		return nil
	}

	out := map[cosiapi.BucketInfoVar]string{
		cosiapi.BucketInfoVar_Azure_StorageAccount: b.StorageAccount,
	}

	return out
}

func (AzureBucketInfoTranslator) ApiToRpc(vars map[cosiapi.BucketInfoVar]string) *cosiproto.AzureBucketInfo {
	if len(vars) == 0 {
		return nil
	}

	out := &cosiproto.AzureBucketInfo{}

	out.StorageAccount = vars[cosiapi.BucketInfoVar_Azure_StorageAccount]

	return out
}

func (AzureBucketInfoTranslator) Validate(
	vars map[cosiapi.BucketInfoVar]string, _ cosiapi.BucketAccessAuthenticationType,
) error {
	errs := []error{}

	storageAccount := vars[cosiapi.BucketInfoVar_Azure_StorageAccount]
	if storageAccount == "" {
		errs = append(errs, fmt.Errorf("azure storage account cannot be unset"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("azure bucket info is invalid: %w", errors.Join(errs...))
	}
	return nil
}

func (AzureCredentialTranslator) RpcToApi(c *cosiproto.AzureCredentialInfo) map[cosiapi.CredentialVar]string {
	if c == nil {
		return nil
	}

	out := map[cosiapi.CredentialVar]string{
		cosiapi.CredentialVar_Azure_AccessToken:     c.AccessToken,
		cosiapi.CredentialVar_Azure_ExpiryTimestamp: c.ExpiryTimestamp,
	}

	return out
}

func (AzureCredentialTranslator) ApiToRpc(vars map[cosiapi.CredentialVar]string) *cosiproto.AzureCredentialInfo {
	if len(vars) == 0 {
		return nil
	}

	out := &cosiproto.AzureCredentialInfo{}

	out.AccessToken = vars[cosiapi.CredentialVar_Azure_AccessToken]
	out.ExpiryTimestamp = vars[cosiapi.CredentialVar_Azure_ExpiryTimestamp]

	return out
}

func (AzureCredentialTranslator) Validate(
	vars map[cosiapi.CredentialVar]string, authType cosiapi.BucketAccessAuthenticationType,
) error {
	// credentials are only required when authentication type is "Key"
	if authType != cosiapi.BucketAccessAuthenticationTypeKey {
		return nil
	}

	errs := []error{}

	accessToken := vars[cosiapi.CredentialVar_Azure_AccessToken]
	if accessToken == "" {
		errs = append(errs, fmt.Errorf("azure access token cannot be unset"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("azure credential info is invalid: %w", errors.Join(errs...))
	}
	return nil
}
