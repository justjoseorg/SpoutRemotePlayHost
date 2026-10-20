//go:build !windows && !linux

package picker

import (
	"context"
	"errors"
)

func openFile(context.Context, string) (string, error) {
	return "", errors.New("file dialog is not supported on this platform")
}
