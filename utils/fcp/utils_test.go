package fcp

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchWorldWideNames(t *testing.T) {
	tests := []struct {
		name            string
		wwn1            string
		wwn2            string
		identicalSearch bool
		expected        bool
	}{
		{
			name:            "Identical WWNs with identicalSearch true",
			wwn1:            "0x5005076801401b3f",
			wwn2:            "0x5005076801401b3f",
			identicalSearch: true,
			expected:        true,
		},
		{
			name:            "Different WWNs with identicalSearch true",
			wwn1:            "0x5005076801401b3f",
			wwn2:            "0x5005076801401b40",
			identicalSearch: true,
			expected:        false,
		},
		{
			name:            "Identical WWNs with identicalSearch false",
			wwn1:            "0x5005076801401b3f",
			wwn2:            "5005076801401b3f",
			identicalSearch: false,
			expected:        true,
		},
		{
			name:            "Different WWNs with identicalSearch false",
			wwn1:            "0x5005076801401b3f",
			wwn2:            "5005076801401b40",
			identicalSearch: false,
			expected:        false,
		},
		{
			name:            "WWNs with colons and identicalSearch false",
			wwn1:            "0x50:05:07:68:01:40:1b:3f",
			wwn2:            "5005076801401b3f",
			identicalSearch: false,
			expected:        true,
		},
		{
			name:            "WWNs with colons and identicalSearch false",
			wwn1:            "0x5005076801401b3f",
			wwn2:            "50:05:07:68:01:40:1b:3f",
			identicalSearch: false,
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchWorldWideNames(tt.wwn1, tt.wwn2, tt.identicalSearch)
			if result != tt.expected {
				t.Errorf("MatchWorldWideNames(%s, %s, %v) = %v; want %v", tt.wwn1, tt.wwn2, tt.identicalSearch, result, tt.expected)
			}
		})
	}
}

func TestGetFCPRPortsDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	path := "/tmp/sys/class/fc_remote_ports"

	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports")

	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports", 0o755)
	assert.NoError(t, err)

	f, err := os.Create("/tmp/sys/class/fc_remote_ports/rport_test")
	assert.NoError(t, err)

	_, err = f.WriteString("content")
	assert.NoError(t, err)

	_, getFCPErr := getFCPRPortsDirectories(context.TODO(), path)
	assert.Nil(t, getFCPErr, "getFCPRPortsDirectories returns error")
}

func TestGetFCPRPortsDirectoriesErrorCase(t *testing.T) {
	_, err := getFCPRPortsDirectories(context.TODO(), "")
	assert.NotNil(t, err, "getFCPRPortsDirectories returns error")
}

func TestConvertStrToWWNFormat(t *testing.T) {
	wwnStr := "1231231235"
	expected := "12:31:23:12:35"

	result := ConvertStrToWWNFormat(wwnStr)
	assert.Equal(t, result, expected, "ConvertStrToWWNFormat does not convert as expected")
}
