package yakdiff

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// DiffZIPFile compares two ZIP files and returns diff string or invokes the handler for each change
// This is a high-level wrapper around FileSystemDiff for ZIP files
func DiffZIPFile(zipFile1, zipFile2 string, handler ...DiffHandler) (string, error) {
	// Check if files exist
	if ok, _ := utils.PathExists(zipFile1); !ok {
		return "", errors.Errorf("zip file %s not existed", zipFile1)
	}
	if ok, _ := utils.PathExists(zipFile2); !ok {
		return "", errors.Errorf("zip file %s not existed", zipFile2)
	}

	// Create ZIP file systems
	fs1, err := filesys.NewZipFSFromLocal(zipFile1)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create zip fs from %s", zipFile1)
	}

	fs2, err := filesys.NewZipFSFromLocal(zipFile2)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create zip fs from %s", zipFile2)
	}

	// Perform filesystem diff
	return FileSystemDiff(fs1, fs2, handler...)
}

var Exports = map[string]any{
	"Diff":               Diff,
	"DiffFromFileSystem": FileSystemDiff,
	"DiffDir": func(i string, j string) (string, error) {
		if ok, _ := utils.PathExists(i); !ok {
			return "", errors.Errorf("path %s not existed", i)
		}
		if ok, _ := utils.PathExists(j); !ok {
			return "", errors.Errorf("path %s not existed", j)
		}
		return FileSystemDiff(filesys.NewRelLocalFs(i), filesys.NewRelLocalFs(j))
	},
	"DiffZIPFile": DiffZIPFile,
}
