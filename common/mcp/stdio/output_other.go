//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package stdio

import (
	"fmt"
	"io"
	"os"
)

func prepareOutput(output io.WriteCloser) (io.WriteCloser, func(), error) {
	if _, ok := output.(*os.File); ok {
		return nil, nil, fmt.Errorf("cancellable MCP file output is unsupported on this platform")
	}
	return output, func() { _ = output.Close() }, nil
}
