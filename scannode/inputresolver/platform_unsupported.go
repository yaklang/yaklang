//go:build !linux

package inputresolver

import (
	"fmt"
	"os"
)

func Supported() bool { return false }

func openBeneath(string, string, int, os.FileMode) (*os.File, error) {
	return nil, fmt.Errorf("managed input isolation is unsupported on this platform")
}

func lockFile(*os.File, bool) error {
	return fmt.Errorf("managed input leases are unsupported on this platform")
}

func availableBytes(string) (uint64, error) {
	return 0, fmt.Errorf("managed input disk accounting is unsupported on this platform")
}
