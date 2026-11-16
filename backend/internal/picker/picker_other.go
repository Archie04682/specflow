//go:build !darwin && !linux && !windows

package picker

import "context"

func pick(context.Context) (string, error) {
	return "", ErrUnsupported
}
