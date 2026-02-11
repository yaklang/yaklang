package aiskillloader

import (
	"io"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// NewZipSkillLoader creates a SkillLoader from a local zip file path.
// The zip file should contain skill directories at its root level.
func NewZipSkillLoader(zipPath string) (*FSSkillLoader, error) {
	zipFS, err := filesys.NewZipFSFromLocal(zipPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to open zip file: %s", zipPath)
	}
	return NewFSSkillLoader(zipFS)
}

// NewZipSkillLoaderFromReader creates a SkillLoader from a zip reader.
// The reader must implement io.ReaderAt and provide the total size.
func NewZipSkillLoaderFromReader(reader io.ReaderAt, size int64) (*FSSkillLoader, error) {
	zipFS, err := filesys.NewZipFSRaw(reader, size)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem from reader")
	}
	return NewFSSkillLoader(zipFS)
}
