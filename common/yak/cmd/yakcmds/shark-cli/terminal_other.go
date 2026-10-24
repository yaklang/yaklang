//go:build !windows

package sharkcli

import "os"

func prepareTerminal(in, out *os.File) (func(), error) { return func() {}, nil }
