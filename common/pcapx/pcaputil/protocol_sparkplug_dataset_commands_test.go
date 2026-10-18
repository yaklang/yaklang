package pcaputil

import (
	"bytes"
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

func spDataSetCommandSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_DATASET_COMMAND_SOURCE_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("testdata/sparkplug-dataset-commands/" + name)
	require.NoError(t, err)
	return b
}

func TestSparkplugDataSetCommandObservationExistingAPI(t *testing.T) {
	raw := spDataSetCommandSource(t, "command-controls.json")
	require.Equal(t, "c1a53e50013f3581f71236a7ca95251b477455b0acac87bd050ec235fb6246ef", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
		NormalNeighbors []struct {
			Name  string
			Steps []spDataSetStep
		} `json:"normal_neighbors"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	require.Len(t, g.Cases, 8)
	require.Len(t, g.NormalNeighbors, 1)
	for _, c := range g.Cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	for _, c := range g.NormalNeighbors {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

// Preserve the immutable parent gold and upgrade only two exact command
// projections supported by the separately pinned normative command contract.
// Every wire byte, metadata field and later producer observation stays fixed.
func spDataSetCommandLegacyCases(t *testing.T) {
	t.Helper()
	original := spDataSetSource(t, "command-boundary-controls.json")
	require.Equal(t, "ef875a8fe1e36580ced720f4bb94652d79731459ce0ae043e0c7af9a49c4baf4", fmt.Sprintf("%x", sha256.Sum256(original)))
	side := spDataSetCommandSource(t, "historical-command-upgrade-corrected-parent.json")
	require.Equal(t, "757e2446e76b2d51519d748bbd384fde390065a806850e3a27718c5bc35e76bb", fmt.Sprintf("%x", sha256.Sum256(side)))
	var g struct {
		Cases []struct {
			Name  string
			Steps []map[string]any
		}
	}
	dec := json.NewDecoder(bytes.NewReader(original))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&g))
	require.Len(t, g.Cases, 2)
	var u struct {
		ArchiveSHA string `json:"archive_sha256"`
		MemberSHA  string `json:"source_member_sha256"`
		Records    []struct {
			Case int            `json:"case_index"`
			Name string         `json:"case_name"`
			Step int            `json:"step_index"`
			Old  map[string]any `json:"old_step"`
			New  map[string]any `json:"new_step"`
		}
	}
	dec = json.NewDecoder(bytes.NewReader(side))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&u))
	require.Equal(t, "e8c985944be1db088db408d37dcf5aa0b0231333c838903266b43d4f27878e79", u.ArchiveSHA)
	require.Equal(t, "ef875a8fe1e36580ced720f4bb94652d79731459ce0ae043e0c7af9a49c4baf4", u.MemberSHA)
	require.Len(t, u.Records, 2)
	for i, r := range u.Records {
		require.Equal(t, i, r.Case)
		require.Equal(t, g.Cases[r.Case].Name, r.Name)
		require.Equal(t, r.Old, g.Cases[r.Case].Steps[r.Step])
		require.Len(t, r.New, len(r.Old))
		for k, v := range r.Old {
			if k != "Session" && k != "GetFields_Sparkplug" && k != "Decode_session" {
				require.Equal(t, v, r.New[k], k)
			}
		}
		g.Cases[r.Case].Steps[r.Step] = r.New
	}
	for _, c := range g.Cases {
		raw, err := json.Marshal(c.Steps)
		require.NoError(t, err)
		var steps []spDataSetStep
		dec = json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		require.NoError(t, dec.Decode(&steps))
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, steps) })
	}
}

func TestSparkplugDataSetCommandCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spDataSetCommandSource, "sparkplug-dataset-commands", "75c91be7509eddfc84b27555e956e02e11a77c528b680cbacb9de09a726a419f", 11)
}

// This is a versioned capability projection over two exact immutable captures,
// not a replacement oracle for their original bytes or other observations.
func spDataSetCommandLegacyCapturedAnswers(t *testing.T, raw []byte) []byte {
	t.Helper()
	side := spDataSetCommandSource(t, "historical-command-upgrade-corrected-parent.json")
	require.Equal(t, "757e2446e76b2d51519d748bbd384fde390065a806850e3a27718c5bc35e76bb", fmt.Sprintf("%x", sha256.Sum256(side)))
	var u struct {
		Records []struct {
			Case int            `json:"case_index"`
			Name string         `json:"case_name"`
			Step int            `json:"step_index"`
			SHA  string         `json:"input_sha256"`
			Old  map[string]any `json:"old_step"`
			New  map[string]any `json:"new_step"`
		}
	}
	d := json.NewDecoder(bytes.NewReader(side))
	d.UseNumber()
	require.NoError(t, d.Decode(&u))
	require.Len(t, u.Records, 2)
	var gold map[string]any
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&gold))
	profiles := gold["Profiles"].(map[string]any)
	require.Len(t, profiles, 25)
	for i, r := range u.Records {
		require.Equal(t, i, r.Case)
		name := "command-" + r.Name
		c, ok := profiles[name].(map[string]any)
		require.True(t, ok, name)
		events := c["Events"].([]any)
		require.True(t, r.Step < len(events))
		event := events[r.Step].(map[string]any)
		require.Len(t, event, 3)
		require.Equal(t, r.Old["Session"], event["Session"])
		require.Equal(t, r.Old["direction"], event["Direction"])
		record := event["Record"].(string)
		const prefix = "common/pcapx/pcaputil/"
		require.True(t, strings.HasPrefix(record, prefix))
		wire, err := trafficfixture.ReadFile(strings.TrimPrefix(record, prefix))
		require.NoError(t, err)
		require.Equal(t, r.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
		require.Equal(t, r.Old["input_sha256"], r.SHA)
		require.Equal(t, r.Old["input_hex"], fmt.Sprintf("%x", wire))
		// No direction, raw record, offset, later event, capture or hash changes.
		event["Session"] = r.New["Session"]
	}
	out, err := json.Marshal(gold)
	require.NoError(t, err)
	return out
}
