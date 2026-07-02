package loop_ai_skill_audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestWorkspaceAttachedContext_TargetOverridesDirectory(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("test", "", context.Background(), nil, true)
	task.SetAttachedDatas([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(reactloops.AttachedResourceTypeFile, reactloops.AttachedResourceKeyDirectoryPath, "/tmp/new-workspace"),
		aicommon.NewAttachedResource(reactloops.AttachedResourceTypeFile, AttachedResourceKeySkillAuditTargetPath, "/tmp/legacy-target"),
	})
	ws := reactloops.InitWorkspaceAttachedContext(nil, nil, task, AttachedResourceKeySkillAuditTargetPath)
	require.NotNil(t, ws)
	require.Equal(t, "/tmp/legacy-target", ws.ResolveAttachedScanDirectory())
}

func TestWorkspaceAttachedContext_LegacyTargetFallback(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("test", "", context.Background(), nil, true)
	task.SetAttachedDatas([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(reactloops.AttachedResourceTypeFile, AttachedResourceKeySkillAuditTargetPath, "/tmp/skill/demo"),
	})
	ws := reactloops.InitWorkspaceAttachedContext(nil, nil, task, AttachedResourceKeySkillAuditTargetPath)
	require.NotNil(t, ws)
	require.Equal(t, "/tmp/skill/demo", ws.ResolveAttachedScanDirectory())
}
