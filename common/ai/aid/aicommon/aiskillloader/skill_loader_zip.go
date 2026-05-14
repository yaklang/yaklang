package aiskillloader

import (
	"io"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// NewArchiveSkillLoader creates a SkillLoader from a local archive path.
// Supported archive formats are zip, tar, tar.gz and tgz.
func NewArchiveSkillLoader(archivePath string) (*FSSkillLoader, error) {
	archiveFS, err := filesys.NewArchiveFSFromLocal(archivePath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to open archive file: %s", archivePath)
	}
	return NewFSSkillLoader(archiveFS)
}

// NewZipSkillLoader creates a SkillLoader from a local zip file path.
// The archive should contain skill directories at its root level.
func NewZipSkillLoader(zipPath string) (*FSSkillLoader, error) {
	return NewArchiveSkillLoader(zipPath)
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
