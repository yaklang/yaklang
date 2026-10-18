package pcaputil

// CC0 unexecuted old public API semantic witness. No new production symbols.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestSparkplugTemplateParameterCommandsExistingAPI(t *testing.T) {
	raw := spTemplateParameterCommandSource(t, "baseline-controls.json")
	require.Equal(t, "54cc2cbcfdd341e8dcb09290b59080e1c70ece225fc7f875f2e90d341be77169", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 2)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 16, total)
}

func spTemplateParameterCommandSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_TEMPLATE_PARAMETER_COMMAND_SOURCE_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return raw
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-template-command-parameters/" + name)
	require.NoError(t, err)
	return raw
}

func TestSparkplugTemplateParameterCommandsStateExistingAPI(t *testing.T) {
	raw := spTemplateParameterCommandSource(t, "parameter-command-controls.json")
	require.Equal(t, "24a9258f653d4617185d2f9cffa93dc36200ff6e9b4ba0f82dd678a3da8eabec", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 6)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 52, total)
}

// Upgrade precisely two independently pinned native command projections. Old
// archive bytes, inputs, surrounding steps and partial-command case stay fixed.
func spTemplateParameterCommandLegacyGold(t *testing.T, raw []byte) []byte {
	t.Helper()
	require.Equal(t, "9cd36c44243dde6ef9aafbc2cba60f2fd941b14e20d0379ed8514c98183a57ca", fmt.Sprintf("%x", sha256.Sum256(raw)))
	sideRaw := spTemplateParameterCommandSource(t, "historical-parameter-command-upgrade.json")
	require.Equal(t, "63d593be2df9e5b0931c95d1ff9b2455b731d3a0d124b39eebba05d30a0eda5f", fmt.Sprintf("%x", sha256.Sum256(sideRaw)))
	var side, gold map[string]any
	spCommandResourceDraftDecode(t, sideRaw, &side)
	spCommandResourceDraftDecode(t, raw, &gold)
	archive := side["parent_archive"].(map[string]any)
	require.Equal(t, "validation-sparkplug-template-device-commands.zip", archive["filename"])
	require.Equal(t, "d922a372fa8fb9ae07cc03328599326e7833b2b1a2d7d20394d8861a462960b7", archive["sha256"])
	require.Equal(t, json.Number("802963"), archive["bytes"])
	require.Equal(t, "sparkplug-template-device/command-boundary-controls.json", archive["member"])
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), archive["member_sha256"])
	require.Equal(t, json.Number(fmt.Sprint(len(raw))), archive["member_bytes"])
	// Old compressed bytes are retained in the external freeze. The verified
	// storage source-index binds the identical original member contract; the
	// complete member/case/step hashes below still verify actual logical bytes.
	origin, err := trafficfixture.SourceArchive(archive["filename"].(string))
	require.NoError(t, err)
	require.Equal(t, int64(802963), origin.Bytes)
	require.Equal(t, archive["sha256"], origin.SHA256)
	require.Equal(t, archive["filename"], origin.File)
	cases := gold["cases"].([]any)
	require.Len(t, cases, 2)
	require.Equal(t, json.Number("0"), side["case_index"])
	c := cases[0].(map[string]any)
	require.Equal(t, side["case_name"], c["name"])
	spTemplatePartialCanonical(t, c, side["original_case_canonical_sha256"].(string))
	steps := c["steps"].([]any)
	require.Len(t, steps, 8)
	records := side["steps"].([]any)
	require.Len(t, records, 2)
	require.Equal(t, []any{"Session", "GetFields_Sparkplug", "Decode_session"}, side["allowed_changed_step_keys"])
	for i, item := range records {
		record := item.(map[string]any)
		require.Equal(t, json.Number(fmt.Sprint(i+4)), record["step_index"])
		st := steps[i+4].(map[string]any)
		require.Equal(t, record["old_step"], st)
		spTemplatePartialCanonical(t, st, record["original_step_canonical_sha256"].(string))
		replacement := record["new_step"].(map[string]any)
		for _, key := range []string{"Session", "GetFields_Sparkplug", "Decode_session"} {
			st[key] = replacement[key]
		}
		require.Equal(t, replacement, st)
		spTemplatePartialCanonical(t, st, record["new_step_canonical_sha256"].(string))
	}
	require.Equal(t, side["new_case"], c)
	spTemplatePartialCanonical(t, c, side["new_case_canonical_sha256"].(string))
	unchanged := side["unchanged_cases"].([]any)
	require.Len(t, unchanged, 1)
	pin := unchanged[0].(map[string]any)
	require.Equal(t, json.Number("1"), pin["index"])
	require.Equal(t, pin["name"], cases[1].(map[string]any)["name"])
	spTemplatePartialCanonical(t, cases[1], pin["canonical_sha256"].(string))
	out, err := json.Marshal(gold)
	require.NoError(t, err)
	return out
}

func TestSparkplugTemplateParameterCommandsCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spTemplateParameterCommandSource, "sparkplug-template-command-parameters", "1c34531dd7bfa00c494f7c7529cf3668a511a1bc3c9a9cfe80fb7ad67f9aa566", 6)
}

func TestSparkplugTemplateParameterCommandsZeroMemberExistingAPI(t *testing.T) {
	raw := spTemplateParameterCommandSource(t, "zero-member-controls.json")
	require.Equal(t, "84a9db9e0c7603d51479bba989e8997a35b0d7d2f69408383053687f611122ff", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 2)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 28, total)
}
