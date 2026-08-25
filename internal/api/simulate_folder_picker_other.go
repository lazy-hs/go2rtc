//go:build !windows

package api

import (
	"context"
	"errors"
)

func simulateNativeFolderPickerAvailable() bool {
	return false
}

func simulatePickNativeFolder(context.Context, string) (string, error) {
	return "", errors.New("native folder picker is not supported on this platform")
}
