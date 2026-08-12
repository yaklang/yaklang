//go:build linux

package patch

import "fmt"

// implementation is the platform-specific patcher.
type implementation struct{}

func newImplementation() (*implementation, error) {
	return &implementation{}, nil
}

func (p *implementation) patch(req Request) error {
	removed, err := patchArchive(req.ArchivePath, req.UsedModules)
	if err != nil {
		return err
	}
	if removed > 0 {
		fmt.Printf("patch: removed %d relocations referencing unused modules\n", removed)
	}
	return nil
}
