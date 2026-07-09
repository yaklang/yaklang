package util

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const scanObsFilePrefix = "scan_obs_"

// CategoryObservationFilename returns per-category observation JSON filename.
func CategoryObservationFilename(categoryID string) string {
	return scanObsFilePrefix + categoryID + ".json"
}

// ListScanObservationFiles returns scan_obs_{category}.json files under auditDir.
func ListScanObservationFiles(auditDir string) []string {
	if auditDir == "" {
		return nil
	}
	entries, err := os.ReadDir(auditDir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, scanObsFilePrefix) && strings.HasSuffix(name, ".json") {
			paths = append(paths, filepath.Join(auditDir, name))
		}
	}
	sort.Strings(paths)
	return paths
}
