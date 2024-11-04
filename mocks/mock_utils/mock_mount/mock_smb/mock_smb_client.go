// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/netapp/trident/utils/mount (interfaces: SmbClient)
//
// Generated by this command:
//
//	mockgen -destination=../../mocks/mock_utils/mock_mount/mock_smb/mock_smb_client.go -package=mock_smb github.com/netapp/trident/utils/mount SmbClient
//

// Package mock_smb is a generated GoMock package.
package mock_smb

import (
	context "context"
	reflect "reflect"

	v1 "github.com/kubernetes-csi/csi-proxy/client/api/smb/v1"
	gomock "go.uber.org/mock/gomock"
	grpc "google.golang.org/grpc"
)

// MockSmbClient is a mock of SmbClient interface.
type MockSmbClient struct {
	ctrl     *gomock.Controller
	recorder *MockSmbClientMockRecorder
}

// MockSmbClientMockRecorder is the mock recorder for MockSmbClient.
type MockSmbClientMockRecorder struct {
	mock *MockSmbClient
}

// NewMockSmbClient creates a new mock instance.
func NewMockSmbClient(ctrl *gomock.Controller) *MockSmbClient {
	mock := &MockSmbClient{ctrl: ctrl}
	mock.recorder = &MockSmbClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSmbClient) EXPECT() *MockSmbClientMockRecorder {
	return m.recorder
}

// NewSmbGlobalMapping mocks base method.
func (m *MockSmbClient) NewSmbGlobalMapping(arg0 context.Context, arg1 *v1.NewSmbGlobalMappingRequest, arg2 ...grpc.CallOption) (*v1.NewSmbGlobalMappingResponse, error) {
	m.ctrl.T.Helper()
	varargs := []any{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "NewSmbGlobalMapping", varargs...)
	ret0, _ := ret[0].(*v1.NewSmbGlobalMappingResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewSmbGlobalMapping indicates an expected call of NewSmbGlobalMapping.
func (mr *MockSmbClientMockRecorder) NewSmbGlobalMapping(arg0, arg1 any, arg2 ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewSmbGlobalMapping", reflect.TypeOf((*MockSmbClient)(nil).NewSmbGlobalMapping), varargs...)
}

// RemoveSmbGlobalMapping mocks base method.
func (m *MockSmbClient) RemoveSmbGlobalMapping(arg0 context.Context, arg1 *v1.RemoveSmbGlobalMappingRequest, arg2 ...grpc.CallOption) (*v1.RemoveSmbGlobalMappingResponse, error) {
	m.ctrl.T.Helper()
	varargs := []any{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RemoveSmbGlobalMapping", varargs...)
	ret0, _ := ret[0].(*v1.RemoveSmbGlobalMappingResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RemoveSmbGlobalMapping indicates an expected call of RemoveSmbGlobalMapping.
func (mr *MockSmbClientMockRecorder) RemoveSmbGlobalMapping(arg0, arg1 any, arg2 ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveSmbGlobalMapping", reflect.TypeOf((*MockSmbClient)(nil).RemoveSmbGlobalMapping), varargs...)
}