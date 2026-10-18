package pcaputil

// Independent fixed-source Template controls exercise the existing public API.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSparkplugTemplateDefinitionInstanceExistingAPI(t *testing.T) {
	b := spTemplateSource(t, "baseline-controls-v2.json")
	require.Equal(t, "c00c99ba292bf34a49d837adf3adbbc2383981b97597fb5c7f60aacc13cb456d", fmt.Sprintf("%x", sha256.Sum256(b)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&gold))
	require.Len(t, gold.Cases, 2)
	for _, c := range gold.Cases {
		t.Run(c.Name, func(t *testing.T) {
			// Existing strict checker retains complete metadata, all Session,
			// real event IDs/zero business IDs, input/public/private ownership,
			// later event history and Close; whole/1/3/17 and full/deferred.
			spDataSetStateTrace(t, c.Steps)
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
			require.NoError(t, err)
			defer s.Close("cleanup")
			for _, st := range c.Steps {
				wire, err := hex.DecodeString(st.Input)
				require.NoError(t, err)
				r := s.Feed(st.Direction, time.Unix(st.Time, 0), wire)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				spAssertTree(t, st.Session, e.Session)
				spTemplateTypes(t, e.Session)
				fields, err := e.GetFields()
				require.NoError(t, err)
				spTemplateTypes(t, map[string]any{"Sparkplug": fields["Sparkplug"]})
				q, err := e.Decode()
				require.NoError(t, err)
				spTemplateTypes(t, q["session"].(map[string]any))
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
			}
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("again"))
		})
	}
}

func spTemplateTypes(t *testing.T, session map[string]any) {
	t.Helper()
	sp, ok := session["Sparkplug"].(map[string]any)
	if !ok {
		return
	}
	if p, ok := sp["Payload"].(map[string]any); ok {
		ms, _ := p["metrics"].([]map[string]any)
		for _, m := range ms {
			if v, present := m["template_value"]; present {
				_, ok := v.([]byte)
				require.True(t, ok)
			}
		}
	}
	ms, _ := sp["Metrics"].([]map[string]any)
	for index, m := range ms {
		if m["Datatype"] != uint64(19) {
			continue
		}
		if spTemplateKnownWholeNullTypeGuard(t, sp, ms, index) {
			continue
		}
		v, ok := m["Value"].(map[string]any)
		require.True(t, ok)
		if version, present := v["version"]; present {
			_, ok = version.(string)
			require.True(t, ok)
		}
		definition, ok := v["is_definition"].(bool)
		require.True(t, ok)
		if definition {
			require.NotContains(t, v, "template_ref")
		} else {
			_, ok = v["template_ref"].(string)
			require.True(t, ok)
		}
		children, ok := v["metrics"].([]map[string]any)
		require.True(t, ok)
		for _, child := range children {
			require.Len(t, child, 3)
			require.Contains(t, child, "value")
			_, ok = child["name"].(string)
			require.True(t, ok)
			typ, ok := child["datatype"].(uint64)
			require.True(t, ok)
			// A Metric child may explicitly observe is_null; exact source gold
			// still requires nil and the retained wire carries the null flag.
			// Parameters below have no null flag and retain strict Basic types.
			if child["value"] != nil {
				spTemplateBasicType(t, typ, child["value"])
			}
		}
		params, ok := v["parameters"].([]map[string]any)
		require.True(t, ok)
		for _, p := range params {
			require.Len(t, p, 3)
			_, ok = p["name"].(string)
			require.True(t, ok)
			typ, typed := p["type"].(uint64)
			require.True(t, typed)
			spTemplateBasicType(t, typ, p["value"])
		}
	}
}

func TestSparkplugTemplateCompleteContextExistingAPI(t *testing.T) {
	raw := spTemplateSource(t, "full-controls.json")
	require.Equal(t, "9b54c5bc25649a8b0e57bc3fd46366f2845658005a7d4a1dea5a76568ab07e6d", fmt.Sprintf("%x", sha256.Sum256(raw)))
	raw = spTemplatePartialLegacyGold(t, raw)
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 20)
	for _, c := range gold.Cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

func spTemplateSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_TEMPLATE_SOURCE_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return raw
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-template/" + name)
	require.NoError(t, err)
	return raw
}

func TestSparkplugTemplateGlobalBudgetExistingAPI(t *testing.T) {
	raw := spTemplateSource(t, "resource-controls.json")
	require.Equal(t, "06e490871a0989afa0d236f112741f0c64f20023a43d138785fff58976b03d44", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct{ Cases []spCommandResourceDraftCase }
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 4)
	var identical []spCommandResourceDraftStep
	for i, c := range gold.Cases {
		var steps []spCommandResourceDraftStep
		spCommandResourceDraftDecode(t, c.Steps, &steps)
		require.Len(t, steps, 4)
		if i == 0 {
			identical = steps
		} else {
			for j := range steps {
				require.Equal(t, identical[j].Input, steps[j].Input)
				require.Equal(t, identical[j].SHA, steps[j].SHA)
				require.Equal(t, identical[j].Direction, steps[j].Direction)
				require.Equal(t, identical[j].Time, steps[j].Time)
			}
		}
		t.Run(c.Name, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("whole-deferred%v", deferred), func(t *testing.T) { spCommandResourceDraftTrace(t, c, steps, deferred) })
			}
			if i == 0 || i == 2 {
				b := DefaultParserBudget()
				b.MaxCollectionElements = c.Budget.MaxCollectionElements
				b.MaxRecursionDepth = c.Budget.MaxRecursionDepth
				var normal []spDataSetStep
				spCommandResourceDraftDecode(t, c.Steps, &normal)
				spDataSetStateTraceBudget(t, normal, b)
			} else {
				for _, deferred := range []bool{false, true} {
					for _, chunk := range []int{1, 3, 17} {
						t.Run(fmt.Sprintf("limited-chunk%d-deferred%v", chunk, deferred), func(t *testing.T) { spSparkplugAggregateLimitedChunks(t, c, chunk, deferred, 2) })
					}
				}
			}
		})
	}
}

func TestSparkplugTemplateCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spTemplateSource, "sparkplug-template", "732fcdabb628142e6f6e946c36f753e70a6819d6603f5d077559bcee93430bad", 20)
}

func spTemplateBasicType(t *testing.T, typ uint64, v any) {
	t.Helper()
	ok := false
	switch typ {
	case 1, 2, 3, 4:
		_, ok = v.(int64)
	case 5, 6, 7, 8, 13:
		_, ok = v.(uint64)
	case 9:
		_, ok = v.(float32)
	case 10:
		_, ok = v.(float64)
	case 11:
		_, ok = v.(bool)
	case 12, 14:
		_, ok = v.(string)
	default:
		t.Fatalf("unselected Basic datatype %d", typ)
	}
	require.True(t, ok, "Template Basic%d value has exact Go type, got %T", typ, v)
}
func TestSparkplugTemplateParameterTypesExistingAPI(t *testing.T) {
	raw := spTemplateSource(t, "parameter-controls.json")
	require.Equal(t, "ab738d0ddda3f4592b79075afd602f82ea48594e4f5f0da468538fa67e42b58a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 8)
	for _, c := range gold.Cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}
