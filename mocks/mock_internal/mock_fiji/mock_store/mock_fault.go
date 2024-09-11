// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/netapp/trident/internal/fiji/store (interfaces: Fault)

// Package mock_store is a generated GoMock package.
package mock_store

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockFault is a mock of Fault interface.
type MockFault struct {
	ctrl     *gomock.Controller
	recorder *MockFaultMockRecorder
}

// MockFaultMockRecorder is the mock recorder for MockFault.
type MockFaultMockRecorder struct {
	mock *MockFault
}

// NewMockFault creates a new mock instance.
func NewMockFault(ctrl *gomock.Controller) *MockFault {
	mock := &MockFault{ctrl: ctrl}
	mock.recorder = &MockFaultMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFault) EXPECT() *MockFaultMockRecorder {
	return m.recorder
}

// Inject mocks base method.
func (m *MockFault) Inject() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Inject")
	ret0, _ := ret[0].(error)
	return ret0
}

// Inject indicates an expected call of Inject.
func (mr *MockFaultMockRecorder) Inject() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Inject", reflect.TypeOf((*MockFault)(nil).Inject))
}

// IsHandlerSet mocks base method.
func (m *MockFault) IsHandlerSet() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsHandlerSet")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsHandlerSet indicates an expected call of IsHandlerSet.
func (mr *MockFaultMockRecorder) IsHandlerSet() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsHandlerSet", reflect.TypeOf((*MockFault)(nil).IsHandlerSet))
}

// Reset mocks base method.
func (m *MockFault) Reset() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Reset")
}

// Reset indicates an expected call of Reset.
func (mr *MockFaultMockRecorder) Reset() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Reset", reflect.TypeOf((*MockFault)(nil).Reset))
}

// SetHandler mocks base method.
func (m *MockFault) SetHandler(arg0 []byte) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetHandler", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetHandler indicates an expected call of SetHandler.
func (mr *MockFaultMockRecorder) SetHandler(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetHandler", reflect.TypeOf((*MockFault)(nil).SetHandler), arg0)
}