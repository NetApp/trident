package utils

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mock "github.com/netapp/trident/mocks/mock_utils"
)

func newMockCSIProxy(t *testing.T) *mock.MockCSIProxyUtils {
	mockCtrl := gomock.NewController(t)
	newMockCSIProxyUtils := mock.NewMockCSIProxyUtils(mockCtrl)
	return newMockCSIProxyUtils
}

func TestNormalizeWindowsPath(t *testing.T) {
	path := "\\test\\volume\\path"

	result := normalizeWindowsPath(path)
	assert.Equal(t, result, "c:\\test\\volume\\path", "path mismatch")
}
