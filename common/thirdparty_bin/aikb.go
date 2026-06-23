package thirdparty_bin

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// YaklangCodeAIKBZipName is the thirdparty_bin registry name for grep sample search.
	YaklangCodeAIKBZipName = "yaklang-aikb"
	// YaklangCodeAIKBRagName is the thirdparty_bin registry name for semantic sample search.
	YaklangCodeAIKBRagName = "yaklang-aikb-rag"
)

// IsYaklangCodeAIKBDownload reports whether a DownloadRAGs / online rag entry refers to
// the Yaklang code-generation AIKB bundle (yaklang-aikb.rag[.gz] + yaklang-aikb.zip).
func IsYaklangCodeAIKBDownload(ragName, filename string) bool {
	filename = strings.ToLower(filepath.Base(strings.TrimSpace(filename)))
	if strings.Contains(filename, "yaklang-aikb") {
		return true
	}
	ragName = strings.ToLower(strings.TrimSpace(ragName))
	return strings.Contains(ragName, "yaklang") && strings.Contains(ragName, "knowledge")
}

// installPathCandidates returns filesystem paths that should satisfy a registered bin_path.
// RAG exports are often distributed as .rag.gz while bin_cfg expects .rag.
func installPathCandidates(targetPath string) []string {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return nil
	}
	candidates := []string{targetPath}
	lower := strings.ToLower(targetPath)
	if strings.HasSuffix(lower, ".rag") {
		candidates = append(candidates, targetPath+".gz")
	}
	return candidates
}

// InstallYaklangCodeAIKBZip installs yaklang-aikb.zip for grep_yaklang_samples.
// Safe to call when the zip is already present (no-op unless Force is set).
func InstallYaklangCodeAIKBZip(options *InstallOptions) error {
	if err := EnsureInitialized(); err != nil {
		return err
	}
	log.Infof("installing companion yaklang code AIKB zip (%s)", YaklangCodeAIKBZipName)
	if err := Install(YaklangCodeAIKBZipName, options); err != nil {
		return utils.Wrap(err, "install yaklang-aikb zip")
	}
	return nil
}
