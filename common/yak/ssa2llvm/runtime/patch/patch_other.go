//go:build !linux

package patch

import "fmt"

type implementation struct{}

func newImplementation() (*implementation, error) {
	return &implementation{}, nil
}

func (p *implementation) patch(req Request) error {
	return fmt.Errorf("patch: unsupported platform (only linux ELF implemented)")
}
