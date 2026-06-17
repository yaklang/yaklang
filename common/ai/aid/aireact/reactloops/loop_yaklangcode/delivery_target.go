package loop_yaklangcode

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

// hasYaklangEditorDeliveryTarget is true when the loop should deliver code to an open editor file (replace).
func hasYaklangEditorDeliveryTarget(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	return strings.TrimSpace(loop.Get("editor_file_path")) != ""
}

func isYaklangGenCodePath(path string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	return strings.HasPrefix(base, "gen_code_") && strings.HasSuffix(base, ".yak")
}

func isYaklangAspaceArtifactPath(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	return strings.Contains(path, string(filepath.Separator)+"aispace"+string(filepath.Separator))
}

func yaklangGenCodeDir() string {
	return filepath.Join(consts.GetDefaultYakitBaseDir(), "code")
}

func newYaklangGenCodePath() (string, error) {
	dir := yaklangGenCodeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", utils.Errorf("create gen_code dir %s: %w", dir, err)
	}
	name := "gen_code_" + utils.DatetimePretty2() + ".yak"
	return filepath.Join(dir, name), nil
}

// resolveYaklangDeliveryTarget picks the single frontend delivery path and yaklang_code_change op.
// Editor file always wins (replace). Without editor: gen_code_* => create; other real paths => replace;
// aispace staging or empty => allocate yakit-projects/code/gen_code_*.yak (create).
func resolveYaklangDeliveryTarget(loop *reactloops.ReActLoop) (path string, eventOp string, err error) {
	if loop == nil {
		return "", "", utils.Error("loop is nil")
	}

	editorFile := strings.TrimSpace(loop.Get("editor_file_path"))
	if editorFile != "" {
		return filepath.Clean(editorFile), loopinfra.LoopYaklangCodeEventOpReplace, nil
	}

	filename := strings.TrimSpace(loop.Get("filename"))
	if filename != "" {
		if isYaklangGenCodePath(filename) {
			return filepath.Clean(filename), loopinfra.LoopYaklangCodeEventOpCreate, nil
		}
		if !isYaklangAspaceArtifactPath(filename) {
			return filepath.Clean(filename), loopinfra.LoopYaklangCodeEventOpReplace, nil
		}
	}

	genPath, err := newYaklangGenCodePath()
	if err != nil {
		return "", "", err
	}
	return genPath, loopinfra.LoopYaklangCodeEventOpCreate, nil
}
