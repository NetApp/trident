// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/netapp/trident/internal/fiji/models (interfaces: FaultHandler)

// Package mock_models is a generated GoMock package.
package mock_models

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockFaultHandler is a mock of FaultHandler interface.
type MockFaultHandler struct {
	ctrl     *gomock.Controller
	recorder *MockFaultHandlerMockRecorder
}

// MockFaultHandlerMockRecorder is the mock recorder for MockFaultHandler.
type MockFaultHandlerMockRecorder struct {
	mock *MockFaultHandler
}

// NewMockFaultHandler creates a new mock instance.
func NewMockFaultHandler(ctrl *gomock.Controller) *MockFaultHandler {
	mock := &MockFaultHandler{ctrl: ctrl}
	mock.recorder = &MockFaultHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFaultHandler) EXPECT() *MockFaultHandlerMockRecorder {
	return m.recorder
}

// Handle mocks base method.
func (m *MockFaultHandler) Handle() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Handle")
	ret0, _ := ret[0].(error)
	return ret0
}

// Handle indicates an expected call of Handle.
func (mr *MockFaultHandlerMockRecorder) Handle() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Handle", reflect.TypeOf((*MockFaultHandler)(nil).Handle))
}