package reactloops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestParseWorkspaceAttachedContext_TargetOverridesDirectory(t *testing.T) {
	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyDirectoryPath, "/tmp/workspace"),
		aicommon.NewAttachedResource(AttachedResourceTypeFile, "skill_audit_target_path", "/tmp/skill/demo-skill"),
	}
	ctx := ParseWorkspaceAttachedContext(attached, "skill_audit_target_path")
	require.NotNil(t, ctx)
	require.Equal(t, "/tmp/workspace", ctx.DirectoryPath)
	require.Equal(t, "/tmp/skill/demo-skill", ctx.TargetPath)
	require.Equal(t, "/tmp/skill/demo-skill", ctx.ResolveTargetPath())
	require.Equal(t, "/tmp/skill/demo-skill", ctx.ResolveScanTarget())
	require.Equal(t, "/tmp/skill/demo-skill", ctx.ResolveAttachedScanDirectory())
}

func TestParseWorkspaceAttachedContext_FallbackToDirectory(t *testing.T) {
	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyDirectoryPath, "/tmp/skill/demo-skill"),
	}
	ctx := ParseWorkspaceAttachedContext(attached, "skill_audit_target_path")
	require.NotNil(t, ctx)
	require.Equal(t, "/tmp/skill/demo-skill", ctx.ResolveScanTarget())
	require.Equal(t, "", ctx.ResolveTargetPath())
}

func TestParseWorkspaceAttachedContext_FileAndSelectionDoNotInferScanPath(t *testing.T) {
	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyFilePath, "/tmp/skill/SKILL.md"),
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, `{"path":"/tmp/skill/SKILL.md","content":"hello","startLine":1,"endLine":1}`),
	}
	ctx := ParseWorkspaceAttachedContext(attached, "skill_audit_target_path")
	require.NotNil(t, ctx)
	require.Equal(t, "", ctx.ResolveAttachedScanDirectory())
	require.Equal(t, "/tmp/skill", ctx.ResolveScanTarget())
	require.Equal(t, "/tmp/skill/SKILL.md", ctx.FilePath)
	require.True(t, ctx.HasSelection())
}

func TestParseWorkspaceAttachedContext_FileAndSelection(t *testing.T) {
	sel := aicommon.AttachedCodeSelection{
		Path:      "/tmp/skill/demo-skill/scripts/run.py",
		Content:   "import socket",
		StartLine: 1,
		EndLine:   1,
		Language:  "python",
	}
	raw, err := json.Marshal(sel)
	require.NoError(t, err)

	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, string(raw)),
	}
	ctx := ParseWorkspaceAttachedContext(attached, "skill_audit_target_path")
	require.NotNil(t, ctx)
	require.True(t, ctx.HasSelection())
	require.Equal(t, "/tmp/skill/demo-skill/scripts/run.py", ctx.FilePath)
	require.Equal(t, "/tmp/skill/demo-skill/scripts", ctx.ResolveScanTarget())
}

func TestParseWorkspaceAttachedContext_LegacyTargetOnly(t *testing.T) {
	for _, targetKey := range []string{"code_audit_target_path", "skill_audit_target_path"} {
		attached := []*aicommon.AttachedResource{
			aicommon.NewAttachedResource(AttachedResourceTypeFile, targetKey, "/tmp/project"),
		}
		ctx := ParseWorkspaceAttachedContext(attached, targetKey)
		require.NotNil(t, ctx)
		require.Equal(t, "/tmp/project", ctx.ResolveTargetPath())
		require.Equal(t, "/tmp/project", ctx.ResolveScanTarget())
	}
}

func TestInitWorkspaceAttachedContext_SamePayloadForBothAudits(t *testing.T) {
	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyDirectoryPath, "/tmp/shared-root"),
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyFilePath, "/tmp/shared-root/main.go"),
	}
	for _, targetKey := range []string{"code_audit_target_path", "skill_audit_target_path"} {
		task := aicommon.NewStatefulTaskBase("test", "", context.Background(), nil, true)
		task.SetAttachedDatas(attached)
		ws := InitWorkspaceAttachedContext(nil, nil, task, targetKey)
		require.NotNil(t, ws)
		require.Equal(t, "/tmp/shared-root", ws.ResolveAttachedScanDirectory())
		require.Equal(t, "/tmp/shared-root/main.go", ws.FilePath)
	}
}

func TestFocusPromptVars(t *testing.T) {
	vars := FocusPromptVars("/tmp/a.py", &aicommon.AttachedCodeSelection{
		Path:    "/tmp/a.py",
		Content: "import os",
	})
	require.True(t, vars["HasSelectionFocus"].(bool))
	require.True(t, vars["HasFocus"].(bool))
	require.Contains(t, vars["Selection"].(string), "import os")

	vars = FocusPromptVars("/tmp/open.go", nil)
	require.True(t, vars["HasOpenFileFocus"].(bool))
	require.False(t, vars["HasSelectionFocus"].(bool))
}

func TestResolveFocusFilePath(t *testing.T) {
	require.Equal(t, "/tmp/file.py", ResolveFocusFilePath("/tmp/file.py", nil))
	require.Equal(t, "/tmp/from-selection.py", ResolveFocusFilePath("", &aicommon.AttachedCodeSelection{Path: "/tmp/from-selection.py"}))
}
