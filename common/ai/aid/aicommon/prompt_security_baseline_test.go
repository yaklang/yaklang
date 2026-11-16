package aicommon

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
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
