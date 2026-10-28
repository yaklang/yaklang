package pcaputil

// Fixed independent vectors exercise existing public Feed, query and Close APIs.
// Public array values preserve typed cells and raw bytes preserve presence.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func spArrayDraftLoad(t *testing.T, name, pin string) []struct {
	Name  string          `json:"name"`
	Steps []spDataSetStep `json:"steps"`
} {
	t.Helper()
	raw := spArraySource(t, name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	return g.Cases
}

func TestSparkplugArrayCandidateExistingAPI(t *testing.T) {
	cases := spArrayDraftLoad(t, "baseline-controls.json", "21a4f3519c9df9a4fabdfb8c39ab17a4ecbbe0ac4f22e4921cf941605c2e7be8")
	require.Len(t, cases, 2)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			// Full Session, metadata, raw, queries, input/public/query graph
			// ownership and post-Close historical snapshots stay strict.
			spDataSetStateTrace(t, c.Steps)
			spArrayDraftTypedTrace(t, c.Steps)
		})
	}
}

func TestSparkplugArrayNodeDeviceLifecycleExistingAPI(t *testing.T) {
	cases := spArrayDraftLoad(t, "lifecycle-controls.json", "c915030ba38b9ce0230eff327e2fe5a1bbf118eaecd399d5c20db4cb7e4ada29")
	require.Len(t, cases, 2)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			spDataSetStateTrace(t, c.Steps)
			spArrayDraftTypedTrace(t, c.Steps)
		})
	}
}

func spArrayDraftTyped(t *testing.T, session map[string]any) {
	t.Helper()
	sp, present := session["Sparkplug"].(map[string]any)
	if !present {
		return
	}
	if p, present := sp["Payload"].(map[string]any); present {
		ms, _ := p["metrics"].([]map[string]any)
		for _, m := range ms {
			if b, present := m["bytes_value"]; present {
				_, typed := b.([]byte)
				require.True(t, typed, "raw bytes_value stays []byte")
			}
		}
	}
	ms, _ := sp["Metrics"].([]map[string]any)
	for _, m := range ms {
		typ, typed := m["Datatype"].(uint64)
		require.True(t, typed)
		if typ < 22 || typ > 34 || m["Value"] == nil {
			continue
		}
		a, typed := m["Value"].([]any)
		require.True(t, typed, "array public Value proposed []any")
		for _, v := range a {
			typed = false
			switch typ {
			case 22, 23, 24, 25:
				_, typed = v.(int64)
			case 26, 27, 28, 29, 34:
				_, typed = v.(uint64)
			case 30:
				_, typed = v.(float32)
			case 31:
				_, typed = v.(float64)
			case 32:
				_, typed = v.(bool)
			case 33:
				_, typed = v.(string)
			}
			require.True(t, typed, "datatype%d cell must preserve explicit numeric type", typ)
		}
	}
}

func spArrayDraftTypedTrace(t *testing.T, steps []spDataSetStep) {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
	require.NoError(t, err)
	defer s.Close("cleanup")
	for _, st := range steps {
		wire, err := hex.DecodeString(st.Input)
		require.NoError(t, err)
		require.Equal(t, st.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
		r := s.Feed(st.Direction, time.Unix(st.Time, 0), wire)
		require.Nil(t, r.Err)
		require.False(t, r.NeedMore)
		require.Equal(t, len(wire), r.Consumed)
		require.Len(t, r.Events, 1)
		e := r.Events[0]
		spAssertTree(t, st.Session, e.Session)
		spArrayDraftTyped(t, e.Session)
		f, err := e.GetFields()
		require.NoError(t, err)
		spArrayDraftTyped(t, map[string]any{"Sparkplug": f["Sparkplug"]})
		q, err := e.Decode()
		require.NoError(t, err)
		spArrayDraftTyped(t, q["session"].(map[string]any))
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
	}
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.Close("again"))
}

func TestSparkplugBytesEmptyPresenceExistingAPI(t *testing.T) {
	cases := spArrayDraftLoad(t, "bytes-presence-controls.json", "94bdc5c857a574e32ba07840a6d273cb81568c740d896090a9a35f3171063d6e")
	require.Len(t, cases, 3)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

func TestSparkplugArrayFullVectorsExistingAPI(t *testing.T) {
	cases := spArrayDraftLoad(t, "array-vector-full-controls.json", "dee62b8443e07301ea89660c17abc8b542f21fe683f1580b96983c263da51c8e")
	require.Len(t, cases, 27)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}
func TestSparkplugArrayNegativeStateExistingAPI(t *testing.T) {
	cases := spArrayDraftLoad(t, "negative-state-full-controls.json", "da28e3c26084b2c921128d99070f70964668aee2c8346001f1acba6c62e85889")
	require.Len(t, cases, 4)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

func spArraySource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_ARRAY_SOURCE_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return raw
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-arrays/" + name)
	require.NoError(t, err)
	return raw
}
func TestSparkplugArrayCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spArraySource, "sparkplug-arrays", "a5ff6129ab11c4c267d10e28bac6a0a2ac8d159098097f037da71d6be07f0628", 36)
}
