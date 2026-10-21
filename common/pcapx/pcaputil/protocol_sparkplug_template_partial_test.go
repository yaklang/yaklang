package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func spTemplatePartialSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_TEMPLATE_PARTIAL_SOURCE_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return raw
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-template-partial/" + name)
	require.NoError(t, err)
	return raw
}
func TestSparkplugTemplatePartialCandidateExistingAPI(t *testing.T) {
	raw := spTemplatePartialSource(t, "baseline-controls.json")
	require.Equal(t, "781110e9fdc751a41f4f2b29cb0671402d1daadd1644539638516021c2235a5a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 3)
	for _, c := range gold.Cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

// Only two independently pinned capability projections change. Historical
// source bytes, answers, captures, metadata and every other case stay fixed.
func spTemplatePartialUpgrade(t *testing.T) map[string]any {
	t.Helper()
	raw := spTemplatePartialSource(t, "historical-partial-Data-upgrade.json")
	require.Equal(t, "edc99648173d4b4fcaee09cd1a5b8d8494d9ba59450f373cf21051cf6a6c0774", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var side map[string]any
	spCommandResourceDraftDecode(t, raw, &side)
	arc := side["archive"].(map[string]any)
	require.Equal(t, "babc9c2d311f5884481edd995192a16be8868d4d6f8a7124119f52572858cff2", arc["sha256"])
	require.Equal(t, json.Number("724107"), arc["bytes"])
	capture := side["capture"].(map[string]any)
	raw, err := trafficfixture.ReadFile("testdata/" + capture["member"].(string))
	require.NoError(t, err)
	require.Equal(t, json.Number(fmt.Sprint(len(raw))), capture["bytes"])
	require.Equal(t, capture["sha256"], fmt.Sprintf("%x", sha256.Sum256(raw)))
	records := side["records"].([]any)
	require.Len(t, records, 2)
	for i, record := range records {
		r := record.(map[string]any)
		require.Equal(t, json.Number(fmt.Sprint(i+3)), r["source_step_index"])
		require.Equal(t, r["source_step_index"], r["captured_event_index"])
		const prefix = "common/pcapx/pcaputil/"
		name := r["Record"].(string)
		require.True(t, strings.HasPrefix(name, prefix))
		wire, err := trafficfixture.ReadFile(strings.TrimPrefix(name, prefix))
		require.NoError(t, err)
		require.Equal(t, r["input_hex"], fmt.Sprintf("%x", wire))
		require.Equal(t, r["input_sha256"], fmt.Sprintf("%x", sha256.Sum256(wire)))
	}
	return side
}
func spTemplatePartialCanonical(t *testing.T, v any, want string) {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	require.Equal(t, want, fmt.Sprintf("%x", sha256.Sum256(b)))
}
func spTemplatePartialLegacyGold(t *testing.T, raw []byte) []byte {
	t.Helper()
	require.Equal(t, "9b54c5bc25649a8b0e57bc3fd46366f2845658005a7d4a1dea5a76568ab07e6d", fmt.Sprintf("%x", sha256.Sum256(raw)))
	side := spTemplatePartialUpgrade(t)
	pin := side["source_member"].(map[string]any)
	var gold map[string]any
	spCommandResourceDraftDecode(t, raw, &gold)
	cases := gold["cases"].([]any)
	require.Len(t, cases, 20)
	c := cases[15].(map[string]any)
	require.Equal(t, pin["case_name"], c["name"])
	spTemplatePartialCanonical(t, c, pin["old_case_canonical_sha256"].(string))
	steps := c["steps"].([]any)
	require.Len(t, steps, 5)
	for i, r := range side["records"].([]any) {
		record := r.(map[string]any)
		st := steps[i+3].(map[string]any)
		require.Equal(t, record["input_hex"], st["input_hex"])
		require.Equal(t, record["input_sha256"], st["input_sha256"])
		require.Equal(t, record["direction"], st["direction"])
		require.Equal(t, record["old_session"], st["Session"])
		st["Session"] = record["new_session"]
		st["Decode_session"] = record["new_session"]
		st["GetFields_Sparkplug"] = record["new_session"].(map[string]any)["Sparkplug"]
	}
	require.Equal(t, side["new_source_case"], c)
	spTemplatePartialCanonical(t, c, pin["new_case_canonical_sha256"].(string))
	out, err := json.Marshal(gold)
	require.NoError(t, err)
	return out
}
func spTemplatePartialLegacyAnswers(t *testing.T, raw []byte) []byte {
	t.Helper()
	require.Equal(t, "50482308934a0b7e3d897fc71c9566ef7981a069c657dfe67c3ee7d3947e020f", fmt.Sprintf("%x", sha256.Sum256(raw)))
	side := spTemplatePartialUpgrade(t)
	pin := side["answers_member"].(map[string]any)
	var gold map[string]any
	spCommandResourceDraftDecode(t, raw, &gold)
	profiles := gold["Profiles"].(map[string]any)
	require.Len(t, profiles, 20)
	p := profiles[pin["profile_key"].(string)].(map[string]any)
	spTemplatePartialCanonical(t, p, pin["old_profile_canonical_sha256"].(string))
	events := p["Events"].([]any)
	require.Len(t, events, 5)
	for i, r := range side["records"].([]any) {
		record := r.(map[string]any)
		e := events[i+3].(map[string]any)
		require.Len(t, e, 3)
		require.Equal(t, record["Record"], e["Record"])
		require.Equal(t, record["direction"], e["Direction"])
		require.Equal(t, record["old_session"], e["Session"])
		e["Session"] = record["new_session"]
	}
	require.Equal(t, side["new_captured_profile"], p)
	spTemplatePartialCanonical(t, p, pin["new_profile_canonical_sha256"].(string))
	out, err := json.Marshal(gold)
	require.NoError(t, err)
	return out
}
func TestSparkplugTemplatePartialStateExistingAPI(t *testing.T) {
	raw := spTemplatePartialSource(t, "partial-controls.json")
	require.Equal(t, "d054523c849d59d30949e53cb8728ea57e24e4fe95dfe3055344a17d9f5dea8f", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 6)
	for _, c := range gold.Cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

func TestSparkplugTemplatePartialCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spTemplatePartialSource, "sparkplug-template-partial", "9ce1aa012e3e9b34cba3580da3bba731019654791b79e4578e27c3aeb999c1db", 6)
}

func TestSparkplugTemplateNullMemberExistingAPI(t *testing.T) {
	raw := spTemplatePartialSource(t, "null-member-controls.json")
	require.Equal(t, "85ad68fd60778fcf8522c5faa976e004f65b025fba0602045e282ea1c548fa20", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 2)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 13, total)
}
