package loop_yaklangcode

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// YaklangCodePreviewOnlyLoopKey marks loops without an editor target file (no file_path attachment).
// Generated code is kept in memory during the loop and written once to yakit-projects/code on completion.
const YaklangCodePreviewOnlyLoopKey = "yaklang_code_preview_only"

func setYaklangCodePreviewOnly(loop *reactloops.ReActLoop, enabled bool) {
	if loop == nil {
		return
	}
	loop.Set(YaklangCodePreviewOnlyLoopKey, enabled)
}

func isYaklangCodePreviewOnly(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return true
	}
	switch v := loop.GetVariable(YaklangCodePreviewOnlyLoopKey).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return strings.TrimSpace(loop.Get("editor_file_path")) == ""
	}
}

func yaklangPreviewCodeDir() string {
	return filepath.Join(consts.GetDefaultYakitBaseDir(), "code")
}

// newYaklangPreviewCodePath reserves a gen_code path under yakit-projects/code (no content written yet).
func newYaklangPreviewCodePath() (string, error) {
	dir := yaklangPreviewCodeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", utils.Errorf("create preview code dir %s: %w", dir, err)
	}
	name := "gen_code_" + utils.DatetimePretty2() + ".yak"
	return filepath.Join(dir, name), nil
}

// persistYaklangPreviewCode writes final preview code to yakit-projects/code once the loop finishes.
func persistYaklangPreviewCode(loop *reactloops.ReActLoop, content string) (string, error) {
	path := strings.TrimSpace(loop.Get("filename"))
	if path == "" {
		reserved, err := newYaklangPreviewCodePath()
		if err != nil {
			return "", err
		}
		path = reserved
		loop.Set("filename", path)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", utils.Errorf("write preview code to %s: %w", path, err)
	}
	log.Infof("code preview mode: wrote %d bytes to %s", len(content), path)
	return path, nil
}
