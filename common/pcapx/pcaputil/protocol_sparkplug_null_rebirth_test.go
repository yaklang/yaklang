package pcaputil

// Independent fixed command gold; complete public API and lifecycle assertions.
import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func spNullRebirthDraft(t *testing.T, name, sha string, count int) {
	t.Helper()
	b := spNullRebirthSource(t, name)
	require.Equal(t, sha, fmt.Sprintf("%x", sha256.Sum256(b)))
	var g struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	require.Len(t, g.Cases, count)
	for _, c := range g.Cases {
		t.Run(c.Name, func(t *testing.T) {
			// Existing strict helper preserves full/deferred/chunk whole/1/3/17,
			// completeSession/raw/metadata/0IDs/privatequeries/poison/history/Close.
			spDataSetStateTrace(t, c.Steps)
		})
	}
}

func TestSparkplugNullCommandExistingAPI(t *testing.T) {
	spNullRebirthDraft(t, "baseline-controls.json", "130b5364f3d2d34add3de64cb29ab084edf4ed88c5d0676c40b06443fabe89c7", 3)
}
func TestSparkplugNullMixedRebirthStateExistingAPI(t *testing.T) {
	spNullRebirthDraft(t, "null-rebirth-controls.json", "1db9abb9d85db7dca85c400ece12b690939d367b833746cc2c37257ef103e342", 10)
}

func spNullRebirthSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_NULL_REBIRTH_SOURCE_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, e)
		return b
	}
	b, e := trafficfixture.ReadFile("testdata/sparkplug-command-null-rebirth/" + name)
	require.NoError(t, e)
	return b
}
func TestSparkplugNullMixedRebirthCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spNullRebirthSource, "sparkplug-command-null-rebirth", "51e9b177aef5ec6e61a0931f829f16616e4a848bacefbc030f1d42f5afa43cbf", 10)
}

// Upgrade exactly one immutable historical capability projection. Input bytes,
// direction, subsequent Data, and every other original profile remain pinned.
func spNullCommandLegacyAnswers(t *testing.T, original []byte) []byte {
	t.Helper()
	require.Equal(t, "bd9160ae704294a8d805613ab6a64175abfe92c60ffcfacd130f77bee99f26f3", fmt.Sprintf("%x", sha256.Sum256(original)))
	side := spNullRebirthSource(t, "historical-null-command-upgrade.json")
	require.Equal(t, "22ef6e515ad94c0867e85aa4afbad8e93c0c7a175917f826ad6f12c3cad44f33", fmt.Sprintf("%x", sha256.Sum256(side)))
	var u struct {
		Archive struct {
			SHA string `json:"sha256"`
		}
		Member struct {
			SHA string `json:"sha256"`
		}
		Records []struct {
			Profile       string         `json:"profile"`
			Index         int            `json:"event_index"`
			OldProfileSHA string         `json:"old_profile_canonical_sha256"`
			Record        string         `json:"record"`
			RecordSHA     string         `json:"record_sha256"`
			RecordBytes   int            `json:"record_bytes"`
			RecordHex     string         `json:"record_hex"`
			Direction     int            `json:"direction"`
			CaptureMember string         `json:"capture_member"`
			CaptureSHA    string         `json:"capture_sha256"`
			CaptureBytes  int            `json:"capture_bytes"`
			OldSession    map[string]any `json:"old_session"`
			NewSession    map[string]any `json:"new_session"`
			NewProfile    map[string]any `json:"new_profile"`
		}
	}
	d := json.NewDecoder(bytes.NewReader(side))
	d.UseNumber()
	require.NoError(t, d.Decode(&u))
	require.Len(t, u.Records, 1)
	require.Equal(t, "7f4a275506ab8ef95e1ca8f56e7ff975d495913007849fdccbad1ac197914fd6", u.Archive.SHA)
	require.Equal(t, "bd9160ae704294a8d805613ab6a64175abfe92c60ffcfacd130f77bee99f26f3", u.Member.SHA)
	var gold map[string]any
	d = json.NewDecoder(bytes.NewReader(original))
	d.UseNumber()
	require.NoError(t, d.Decode(&gold))
	profiles := gold["profiles"].(map[string]any)
	require.Len(t, profiles, 19)
	r := u.Records[0]
	require.Equal(t, "command-null", r.Profile)
	require.Equal(t, 4, r.Index)
	p := profiles[r.Profile].(map[string]any)
	before, e := json.Marshal(p)
	require.NoError(t, e)
	require.Equal(t, r.OldProfileSHA, fmt.Sprintf("%x", sha256.Sum256(before)))
	events := p["events"].([]any)
	event := events[r.Index].(map[string]any)
	require.Len(t, event, 4)
	require.Equal(t, r.Record, event["record"])
	require.Equal(t, json.Number(fmt.Sprint(r.Direction)), event["direction"])
	require.Equal(t, r.OldSession, event["session"])
	const prefix = "common/pcapx/pcaputil/"
	require.True(t, strings.HasPrefix(r.Record, prefix))
	wire, e := trafficfixture.ReadFile(strings.TrimPrefix(r.Record, prefix))
	require.NoError(t, e)
	require.Equal(t, r.RecordBytes, len(wire))
	require.Equal(t, r.RecordHex, fmt.Sprintf("%x", wire))
	require.Equal(t, r.RecordSHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
	require.True(t, strings.HasPrefix(r.CaptureMember, "sparkplug-commands/"))
	cap, e := trafficfixture.ReadFile("testdata/" + r.CaptureMember)
	require.NoError(t, e)
	require.Equal(t, r.CaptureBytes, len(cap))
	require.Equal(t, r.CaptureSHA, fmt.Sprintf("%x", sha256.Sum256(cap)))
	event["session"] = r.NewSession
	require.Equal(t, r.NewProfile, p)
	out, e := json.Marshal(gold)
	require.NoError(t, e)
	return out
}

func TestSparkplugStructuredNullCommandExistingAPI(t *testing.T) {
	spNullRebirthDraft(t, "structured-null-controls.json", "0f2e2bf57f9adcf3bfd20331f5b0552bba44913cc113bcd4d555f6c5646550c2", 4)
}
