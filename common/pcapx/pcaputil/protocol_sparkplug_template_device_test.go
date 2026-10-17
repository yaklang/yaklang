package pcaputil

// Independent fixed-source device Template baseline using the public API.
import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestSparkplugDeviceTemplateExistingAPI(t *testing.T) {
	raw := spTemplateDeviceSource(t, "baseline-controls.json")
	require.Equal(t, "9c667833491e547427f3131a809b6ef08b2018c917999b432544db14eacdee2e", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&gold))
	require.Len(t, gold.Cases, 3)
	for _, c := range gold.Cases {
		t.Run(c.Name, func(t *testing.T) {
			// Existing full/deferred and whole/1/3/17 strict checker retains
			// full fields/typed Session/raw/metadata, actual IDs/business0,
			// private query/input/history poisoning and Close release checks.
			spDataSetStateTrace(t, c.Steps)
		})
	}
}

func spTemplateDeviceSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_TEMPLATE_DEVICE_SOURCE_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return raw
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-template-device/" + name)
	require.NoError(t, err)
	return raw
}
func TestSparkplugDeviceTemplateStateExistingAPI(t *testing.T) {
	raw := spTemplateDeviceSource(t, "device-controls.json")
	require.Equal(t, "eae487a016a8c690a60f16d24ef1dcfd535db0213729ff3bf5dd01d72bbddae2", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string          `json:"name"`
			Steps []spDataSetStep `json:"steps"`
		} `json:"cases"`
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 6)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 74, total)
}
func TestSparkplugDeviceTemplateGlobalBudgetExistingAPI(t *testing.T) {
	raw := spTemplateDeviceSource(t, "resource-controls.json")
	require.Equal(t, "0b91618ebfbd19cf1dd3ba22ffa76e4fd0b6524d3cea91795e9bfe6c2c287627", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct{ Cases []spCommandResourceDraftCase }
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 2)
	var identical []spCommandResourceDraftStep
	for i, c := range gold.Cases {
		var steps []spCommandResourceDraftStep
		spCommandResourceDraftDecode(t, c.Steps, &steps)
		require.Len(t, steps, 5)
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
		require.Equal(t, 64, c.Budget.MaxRecursionDepth)
		require.Contains(t, []int{47, 48}, c.Budget.MaxCollectionElements)
		t.Run(c.Name, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("whole-deferred%v", deferred), func(t *testing.T) { spCommandResourceDraftTrace(t, c, steps, deferred) })
			}
			if c.Budget.MaxCollectionElements == 48 {
				b := DefaultParserBudget()
				b.MaxRecursionDepth = 64
				b.MaxCollectionElements = 48
				var normal []spDataSetStep
				spCommandResourceDraftDecode(t, c.Steps, &normal)
				spDataSetStateTraceBudget(t, normal, b)
			} else {
				for _, deferred := range []bool{false, true} {
					for _, chunk := range []int{1, 3, 17} {
						t.Run(fmt.Sprintf("limited-chunk%d-deferred%v", chunk, deferred), func(t *testing.T) { spSparkplugAggregateLimitedChunks(t, c, chunk, deferred, 3) })
					}
				}
			}
		})
	}
	require.Equal(t, 48, gold.Cases[0].Budget.MaxCollectionElements)
	require.Equal(t, 47, gold.Cases[1].Budget.MaxCollectionElements)
}

func TestSparkplugDeviceTemplateCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spTemplateDeviceSource, "sparkplug-template-device", "7f1a242a5407da187db422c22ef8f5d648c02286caccebf802684e708a3b67c3", 9)
}

func TestSparkplugTemplateCommandsExistingAPI(t *testing.T) {
	var raw []byte
	if root := os.Getenv("SPARKPLUG_TEMPLATE_COMMAND_SOURCE_ROOT"); root != "" {
		var err error
		raw, err = os.ReadFile(filepath.Join(root, "command-controls.json"))
		require.NoError(t, err)
	} else {
		raw = spTemplateDeviceSource(t, "command-controls.json")
	}
	require.Equal(t, "185e4200125a16d53f1df65d7e6706a71176d6d291d6a1e5a3201fce4e165308", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 3)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 24, total)
}

func TestSparkplugTemplateCommandBoundariesExistingAPI(t *testing.T) {
	var raw []byte
	if root := os.Getenv("SPARKPLUG_TEMPLATE_COMMAND_SOURCE_ROOT"); root != "" {
		var err error
		raw, err = os.ReadFile(filepath.Join(root, "command-boundary-controls.json"))
		require.NoError(t, err)
	} else {
		raw = spTemplateDeviceSource(t, "command-boundary-controls.json")
	}
	require.Equal(t, "9cd36c44243dde6ef9aafbc2cba60f2fd941b14e20d0379ed8514c98183a57ca", fmt.Sprintf("%x", sha256.Sum256(raw)))
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

func TestSparkplugTemplateCommandTypesExistingAPI(t *testing.T) {
	var raw []byte
	if root := os.Getenv("SPARKPLUG_TEMPLATE_COMMAND_SOURCE_ROOT"); root != "" {
		var err error
		raw, err = os.ReadFile(filepath.Join(root, "command-types-controls.json"))
		require.NoError(t, err)
	} else {
		raw = spTemplateDeviceSource(t, "command-types-controls.json")
	}
	require.Equal(t, "bd5cf270970685133268b1f31d906f06914abac0af0a8d06435ac5e48ce690ac", fmt.Sprintf("%x", sha256.Sum256(raw)))
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
