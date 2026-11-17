//go:build !linux

package patch

// implementation is the platform-specific patcher.
//
// The patch is a size optimisation: it drops relocations that reference libyak
// modules the program never registers, so lld --gc-sections can discard those
// sections. Linux implements it for ELF; on the other platforms the archives
// stay untouched and the linker keeps that dead weight.
type implementation struct{}

func newImplementation() (*implementation, error) {
	return &implementation{}, nil
}

func (p *implementation) patch(req Request) error {
	return nil
}