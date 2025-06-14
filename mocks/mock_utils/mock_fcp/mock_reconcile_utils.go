// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/netapp/trident/utils/fcp (interfaces: FcpReconcileUtils)
//
// Generated by this command:
//
//	mockgen -destination=../../mocks/mock_utils/mock_fcp/mock_reconcile_utils.go github.com/netapp/trident/utils/fcp FcpReconcileUtils
//

// Package mock_fcp is a generated GoMock package.
package mock_fcp

import (
	context "context"
	reflect "reflect"

	models "github.com/netapp/trident/utils/models"
	gomock "go.uber.org/mock/gomock"
)

// MockFcpReconcileUtils is a mock of FcpReconcileUtils interface.
type MockFcpReconcileUtils struct {
	ctrl     *gomock.Controller
	recorder *MockFcpReconcileUtilsMockRecorder
	isgomock struct{}
}

// MockFcpReconcileUtilsMockRecorder is the mock recorder for MockFcpReconcileUtils.
type MockFcpReconcileUtilsMockRecorder struct {
	mock *MockFcpReconcileUtils
}

// NewMockFcpReconcileUtils creates a new mock instance.
func NewMockFcpReconcileUtils(ctrl *gomock.Controller) *MockFcpReconcileUtils {
	mock := &MockFcpReconcileUtils{ctrl: ctrl}
	mock.recorder = &MockFcpReconcileUtilsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFcpReconcileUtils) EXPECT() *MockFcpReconcileUtilsMockRecorder {
	return m.recorder
}

// CheckZoningExistsWithTarget mocks base method.
func (m *MockFcpReconcileUtils) CheckZoningExistsWithTarget(arg0 context.Context, arg1 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckZoningExistsWithTarget", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CheckZoningExistsWithTarget indicates an expected call of CheckZoningExistsWithTarget.
func (mr *MockFcpReconcileUtilsMockRecorder) CheckZoningExistsWithTarget(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckZoningExistsWithTarget", reflect.TypeOf((*MockFcpReconcileUtils)(nil).CheckZoningExistsWithTarget), arg0, arg1)
}

// GetDevicesForLUN mocks base method.
func (m *MockFcpReconcileUtils) GetDevicesForLUN(paths []string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDevicesForLUN", paths)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetDevicesForLUN indicates an expected call of GetDevicesForLUN.
func (mr *MockFcpReconcileUtilsMockRecorder) GetDevicesForLUN(paths any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDevicesForLUN", reflect.TypeOf((*MockFcpReconcileUtils)(nil).GetDevicesForLUN), paths)
}

// GetFCPHostSessionMapForTarget mocks base method.
func (m *MockFcpReconcileUtils) GetFCPHostSessionMapForTarget(arg0 context.Context, arg1 string) []map[string]int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetFCPHostSessionMapForTarget", arg0, arg1)
	ret0, _ := ret[0].([]map[string]int)
	return ret0
}

// GetFCPHostSessionMapForTarget indicates an expected call of GetFCPHostSessionMapForTarget.
func (mr *MockFcpReconcileUtilsMockRecorder) GetFCPHostSessionMapForTarget(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetFCPHostSessionMapForTarget", reflect.TypeOf((*MockFcpReconcileUtils)(nil).GetFCPHostSessionMapForTarget), arg0, arg1)
}

// GetSysfsBlockDirsForLUN mocks base method.
func (m *MockFcpReconcileUtils) GetSysfsBlockDirsForLUN(arg0 int, arg1 []map[string]int) []string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSysfsBlockDirsForLUN", arg0, arg1)
	ret0, _ := ret[0].([]string)
	return ret0
}

// GetSysfsBlockDirsForLUN indicates an expected call of GetSysfsBlockDirsForLUN.
func (mr *MockFcpReconcileUtilsMockRecorder) GetSysfsBlockDirsForLUN(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSysfsBlockDirsForLUN", reflect.TypeOf((*MockFcpReconcileUtils)(nil).GetSysfsBlockDirsForLUN), arg0, arg1)
}

// ReconcileFCPVolumeInfo mocks base method.
func (m *MockFcpReconcileUtils) ReconcileFCPVolumeInfo(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReconcileFCPVolumeInfo", ctx, trackingInfo)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReconcileFCPVolumeInfo indicates an expected call of ReconcileFCPVolumeInfo.
func (mr *MockFcpReconcileUtilsMockRecorder) ReconcileFCPVolumeInfo(ctx, trackingInfo any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReconcileFCPVolumeInfo", reflect.TypeOf((*MockFcpReconcileUtils)(nil).ReconcileFCPVolumeInfo), ctx, trackingInfo)
}
