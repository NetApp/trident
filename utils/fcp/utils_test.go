package fcp

import (
	"testing"
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
