package aicommon

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Fixed inputs capture the security agent output before regrouping (41b6a24fee),
// including cache boundaries. Only the process-random projection nonce is normalized.
func TestSecurityPromptBaseline(t *testing.T) {
	for _, native := range []bool{false, true} {
		m := &PromptMaterials{FunctionCallMode: native, Workspace: true, OSArch: "linux/amd64", WorkingDir: "/project", AIArtifactsDir: "/artifacts", CurrentTime: "2026-11-16 12:00", TaskInstruction: "instruction", Schema: "schema", OutputExample: "example"}
		p, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(m, "baseline", "{{ . }}", "user input", "baseline")
		require.NoError(t, err)
		require.Equal(t, map[bool]string{false: "fd8944e1243651493b216166c401d0d0bb3ecf06350c1ceac9ae2e3630efb8c9", true: "3b9c4a765659dd25c97c94152a54dc44754374adb8e48bba3925af7cb5657ac3"}[native], fmt.Sprintf("%x", sha256.Sum256([]byte(aiprojection.RedactNonce(p)))))
	}
	for path, want := range map[string]string{
		"agents/security/default_loop/instruction.txt":    "bbc4c0942d5e9ac1398cc0cbebd117d325a6e562de3baf8059d2ccea4defdcbc",
		"agents/security/default_loop/output_example.txt": "6bb3d24f459cf0e1ee41cbfa8c5d914fb3f05c857a4f01a951981772513c0fe1",
		"agents/security/verification/instruction.txt":    "dd69f004199f2c20bd053e2b82959593ed16bbe46bef6944b92e6e5496b2c178",
		"agents/security/verification/output_example.txt": "0c7731d846f2a0f21df1cde281771363578753d78a042c3b6fd2918b63c963be",
	} {
		require.Equal(t, want, fmt.Sprintf("%x", sha256.Sum256([]byte(promptloader.MustLoad(path)))), path)
	}
}

// Capture the complete security review requests before separating Coding review
// content. Normalize only envelope nonces, the current time and host OS/architecture.
func TestSecurityPlanningReviewPromptBaseline(t *testing.T) {
	expected := map[string]string{
		"plan/zh": "a8638185a75ab87b9453fe94814f04856e406714ccf0318e72054e47693f00c3",
		"plan/en": "e23bb00e9cf9d492e6a4a2d2d746d91d91e7d9f5cc1c5bc98b1673d5f71fa6d7",
		"task/zh": "f56fa147a0bc51efbee7d88b5bf6ce6b52847227575fe3a9ed2117c0c023207b",
		"task/en": "fe25ed8a980087561df076e85c502944db97defa2e4f48c609d260766d9d2c9d",
	}
	materials := aitool.InvokeParams{
		"plan_summary":  "inspect -> implement -> verify",
		"short_summary": "implementation complete",
		"long_summary":  "updated the parser and ran its tests",
		"progress":      "1-1 complete",
		"pending_tasks": "1-2 verify integration",
	}
	for _, mode := range []string{"", ypb.AgentModeSecurity} {
		for _, language := range []string{"zh", "en"} {
			for name, build := range map[string]func(*Config, aitool.InvokeParams) (string, error){
				"plan": generatePlanReviewPrompt,
				"task": generateTaskReviewPrompt,
			} {
				t.Run(mode+"/"+name+"/"+language, func(t *testing.T) {
					cfg := &Config{AgentMode: mode, Workdir: "/review-artifacts", Language: language}
					prompt, err := build(cfg, materials)
					require.NoError(t, err)
					prompt = aiprojection.RedactNonce(prompt)
					prompt = regexp.MustCompile(`(PROMPT_SECTION_dynamic_(?:END_)?)[a-zA-Z0-9]{4}(_REDACTED_PROJECTION_NONCE)`).ReplaceAllString(prompt, "${1}FIXED${2}")
					prompt = regexp.MustCompile(`(# Current Time\n)\d{4}-\d{2}-\d{2} \d{2}:\d{2}`).ReplaceAllString(prompt, "${1}2026-11-19 12:00")
					prompt = strings.ReplaceAll(prompt, runtime.GOOS+"/"+runtime.GOARCH, "test-os/test-arch")
					require.Equal(t, expected[name+"/"+language], fmt.Sprintf("%x", sha256.Sum256([]byte(prompt))))
				})
			}
		}
	}
}
