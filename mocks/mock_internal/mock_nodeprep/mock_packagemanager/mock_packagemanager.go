// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/netapp/trident/internal/nodeprep/packagemanager (interfaces: PackageManager)
//
// Generated by this command:
//
//	mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_packagemanager/mock_packagemanager.go github.com/netapp/trident/internal/nodeprep/packagemanager PackageManager
//

// Package mock_packagemanager is a generated GoMock package.
package mock_packagemanager

import (
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockPackageManager is a mock of PackageManager interface.
type MockPackageManager struct {
	ctrl     *gomock.Controller
	recorder *MockPackageManagerMockRecorder
}

// MockPackageManagerMockRecorder is the mock recorder for MockPackageManager.
type MockPackageManagerMockRecorder struct {
	mock *MockPackageManager
}

// NewMockPackageManager creates a new mock instance.
func NewMockPackageManager(ctrl *gomock.Controller) *MockPackageManager {
	mock := &MockPackageManager{ctrl: ctrl}
	mock.recorder = &MockPackageManagerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPackageManager) EXPECT() *MockPackageManagerMockRecorder {
	return m.recorder
}

// InstallIscsiRequirements mocks base method.
func (m *MockPackageManager) InstallIscsiRequirements(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InstallIscsiRequirements", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// InstallIscsiRequirements indicates an expected call of InstallIscsiRequirements.
func (mr *MockPackageManagerMockRecorder) InstallIscsiRequirements(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InstallIscsiRequirements", reflect.TypeOf((*MockPackageManager)(nil).InstallIscsiRequirements), arg0)
}

// MultipathToolsInstalled mocks base method.
func (m *MockPackageManager) MultipathToolsInstalled(arg0 context.Context) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MultipathToolsInstalled", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// MultipathToolsInstalled indicates an expected call of MultipathToolsInstalled.
func (mr *MockPackageManagerMockRecorder) MultipathToolsInstalled(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MultipathToolsInstalled", reflect.TypeOf((*MockPackageManager)(nil).MultipathToolsInstalled), arg0)
}