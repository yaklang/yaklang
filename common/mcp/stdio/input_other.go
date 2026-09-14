//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package stdio

import "io"

func prepareInput(input io.ReadCloser) (io.Reader, func(), error) {
	return input, func() {}, nil
}
