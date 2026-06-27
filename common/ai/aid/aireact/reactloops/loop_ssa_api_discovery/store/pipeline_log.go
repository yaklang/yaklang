package store

import (
	"path/filepath"
)

// PipelineLogPath returns the path of the persistent pipeline log file.
func PipelineLogPath(workDir string) string {
	return filepath.Join(workDir, SubDirName(), "pipeline.log")
}
