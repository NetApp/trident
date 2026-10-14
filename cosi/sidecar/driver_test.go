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

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"
)

func Test_validateDriverName(t *testing.T) {
	tests := []struct {
		name    string
		isValid bool
	}{
		{"", false}, // empty string
		{"a", true},
		{"z", true},
		{"A", false}, // upper case char
		{"Z", false},
		{"/", false},               // backslash char
		{string([]byte{0}), false}, // nul char
		{".", false},
		{"a.", false},
		{".a", false},
		{"a.a", true},
		{"a..a", false},
		{" ", false},
		{"a ", false},
		{" a", false},
		{"1", false},
		{"1a", false},
		{"a1", true},
		{"-", false},
		{"-a", false},
		{"a-", false},
		{"a-a", true},
		{"a-1", true},
		{"long.driver.name.is.exactly.sixty-three.chars.and.does.not.fail", true},
		{"long.driver.name.is.exactly.sixty-four.chars.and.fails.as.wanted", false},
		{"kitchen.sink-0test.with-12.num3bers.and4-5.st-6-uff", true},
	}
	for _, tt := range tests {
		t.Run(tt.name /* input doubles as test name */, func(t *testing.T) {
			gotErr := validateDriverName(tt.name)
			if !tt.isValid {
				assert.Error(t, gotErr)
			} else {
				assert.NoError(t, gotErr)
			}
		})
	}
}

func Test_validateAndParseProtocols(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		objProtocols []*cosiproto.ObjectProtocol
		want         []cosiproto.ObjectProtocol_Type
		wantErr      bool
	}{
		{"no protos", []*cosiproto.ObjectProtocol{}, []cosiproto.ObjectProtocol_Type{}, true},
		{
			"S3",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
			},
			[]cosiproto.ObjectProtocol_Type{
				cosiproto.ObjectProtocol_S3,
			},
			false,
		},
		{
			"AZURE",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_AZURE},
			},
			[]cosiproto.ObjectProtocol_Type{
				cosiproto.ObjectProtocol_AZURE,
			},
			false,
		},
		{
			"GCS",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_GCS},
			},
			[]cosiproto.ObjectProtocol_Type{
				cosiproto.ObjectProtocol_GCS,
			},
			false,
		},
		{
			"S3, AZURE, GCS",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
				{Type: cosiproto.ObjectProtocol_AZURE},
				{Type: cosiproto.ObjectProtocol_GCS},
			},
			[]cosiproto.ObjectProtocol_Type{
				cosiproto.ObjectProtocol_S3,
				cosiproto.ObjectProtocol_AZURE,
				cosiproto.ObjectProtocol_GCS,
			},
			false,
		},
		{
			"AZURE, S3, GCS - ordering is preserved",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_AZURE},
				{Type: cosiproto.ObjectProtocol_S3},
				{Type: cosiproto.ObjectProtocol_GCS},
			},
			[]cosiproto.ObjectProtocol_Type{
				cosiproto.ObjectProtocol_AZURE,
				cosiproto.ObjectProtocol_S3,
				cosiproto.ObjectProtocol_GCS,
			},
			false,
		},
		{
			"UNKNOWN",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_UNKNOWN},
			},
			[]cosiproto.ObjectProtocol_Type{},
			true,
		},
		{
			"S3, UNKNOWN, GCS",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
				{Type: cosiproto.ObjectProtocol_UNKNOWN},
				{Type: cosiproto.ObjectProtocol_GCS},
			},
			[]cosiproto.ObjectProtocol_Type{},
			true,
		},
		{
			"S3, GCS, S3 - do not accept duplicate",
			[]*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
				{Type: cosiproto.ObjectProtocol_GCS},
				{Type: cosiproto.ObjectProtocol_S3},
			},
			[]cosiproto.ObjectProtocol_Type{},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := validateAndParseProtocols(tt.objProtocols)
			if tt.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.NoError(t, gotErr)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateAndSetDriverConnectionInfo(t *testing.T) {
	t.Run("invalid name", func(t *testing.T) {
		conn := &grpc.ClientConn{}
		response := &cosiproto.DriverGetInfoResponse{
			Name: "7.of.9",
			SupportedProtocols: []*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
			},
		}
		driverInfo, err := ValidateAndSetDriverConnectionInfo(response, conn)
		assert.ErrorContains(t, err, "driver name is invalid")
		assert.Nil(t, driverInfo)
	})

	t.Run("invalid supported protos", func(t *testing.T) {
		conn := &grpc.ClientConn{}
		response := &cosiproto.DriverGetInfoResponse{
			Name: "seven.of.nine",
			SupportedProtocols: []*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
				{Type: cosiproto.ObjectProtocol_S3},
			},
		}
		driverInfo, err := ValidateAndSetDriverConnectionInfo(response, conn)
		assert.ErrorContains(t, err, "supported protocols list is invalid")
		assert.Nil(t, driverInfo)
	})

	t.Run("valid response", func(t *testing.T) {
		conn := &grpc.ClientConn{}
		response := &cosiproto.DriverGetInfoResponse{
			Name: "seven.of.nine",
			SupportedProtocols: []*cosiproto.ObjectProtocol{
				{Type: cosiproto.ObjectProtocol_S3},
			},
		}
		driverInfo, err := ValidateAndSetDriverConnectionInfo(response, conn)
		assert.NoError(t, err)
		assert.Equal(t, "seven.of.nine", driverInfo.Name)
		assert.Equal(t, []cosiproto.ObjectProtocol_Type{cosiproto.ObjectProtocol_S3}, driverInfo.SupportedProtocols)
	})
}
