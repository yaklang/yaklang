package loop_plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils"
)

func TestGuidanceDocumentTemplate_WithFactsAndEvidence(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "darwin/arm64",
		"UserInput":   "analyze the target",
		"Facts":       "- Target is running on port 8080\n- Framework: Spring Boot",
		"Evidence":    "Nmap scan results show open ports 80, 443, 8080",
		"Context":     "## Scan results\nDetailed scan output here",
		"Timeline":    "- [12:00] Task started",
		"WorkingDir":  "/home/user/project",
		"Nonce":       "ABCD",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	assert.Contains(t, rendered, "Current Time: 2026-04-14 12:00:00")
	assert.Contains(t, rendered, "OS/Arch: darwin/arm64")
	assert.Contains(t, rendered, "Working Dir: /home/user/project")
	assert.Contains(t, rendered, "Timeline Memory")
	assert.Contains(t, rendered, "Task started")
	assert.Contains(t, rendered, "analyze the target")
	assert.Contains(t, rendered, "Target is running on port 8080")
	assert.Contains(t, rendered, "Nmap scan results")
	assert.Contains(t, rendered, "Detailed scan output here")
}
