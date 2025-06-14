// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/netapp/trident/utils/iscsi (interfaces: OS)
//
// Generated by this command:
//
//	mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_os_client.go github.com/netapp/trident/utils/iscsi OS
//

// Package mock_iscsi is a generated GoMock package.
package mock_iscsi

import (
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockOS is a mock of OS interface.
type MockOS struct {
	ctrl     *gomock.Controller
	recorder *MockOSMockRecorder
	isgomock struct{}
}

// MockOSMockRecorder is the mock recorder for MockOS.
type MockOSMockRecorder struct {
	mock *MockOS
}

// NewMockOS creates a new mock instance.
func NewMockOS(ctrl *gomock.Controller) *MockOS {
	mock := &MockOS{ctrl: ctrl}
	mock.recorder = &MockOSMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOS) EXPECT() *MockOSMockRecorder {
	return m.recorder
}

// PathExists mocks base method.
func (m *MockOS) PathExists(path string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PathExists", path)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PathExists indicates an expected call of PathExists.
func (mr *MockOSMockRecorder) PathExists(path any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PathExists", reflect.TypeOf((*MockOS)(nil).PathExists), path)
}
