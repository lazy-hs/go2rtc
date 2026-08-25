package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"1.9.14", true},
		{"0.0.0", true},
		{"10.0.3", true},
		{"1.10.0", false},
		{"1.2", false},
		{"v1.2.3", false},
		{"01.2.3", false},
		{"1.02.3", false},
		{"1.2.-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			_, err := parseVersion(tt.value)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestBump(t *testing.T) {
	tests := []struct {
		name     string
		current  version
		kind     releaseType
		expected version
	}{
		{"regular", version{1, 2, 3}, releaseRegular, version{1, 3, 0}},
		{"regular rollover", version{1, 9, 5}, releaseRegular, version{2, 0, 0}},
		{"special", version{1, 3, 0}, releaseSpecial, version{1, 3, 1}},
		{"special sequence", version{1, 3, 2}, releaseSpecial, version{1, 3, 3}},
		{"major", version{1, 7, 8}, releaseMajor, version{2, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.current.bump(tt.kind))
		})
	}
}

func TestCheckTransition(t *testing.T) {
	require.NoError(t, checkTransition(version{1, 9, 5}, version{2, 0, 0}, releaseRegular))
	require.Error(t, checkTransition(version{1, 9, 5}, version{1, 10, 0}, releaseRegular))
	require.Error(t, checkTransition(version{1, 3, 0}, version{1, 4, 0}, releaseSpecial))
}
