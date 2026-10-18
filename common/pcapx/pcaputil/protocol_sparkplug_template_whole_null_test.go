package pcaputil

import (
	"crypto/sha256"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
)

func spTemplateWholeNullSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_TEMPLATE_WHOLE_NULL_SOURCE_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return raw
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-template-whole-null/" + name)
	require.NoError(t, err)
	return raw
}
func TestSparkplugTemplateWholeNullExistingAPI(t *testing.T) {
	raw := spTemplateWholeNullSource(t, "baseline-controls.json")
	require.Equal(t, "2bd5765f163179e7ea4486fe10ab3b92bfdf2a260caa78fb02928eeb1b5e06bc", fmt.Sprintf("%x", sha256.Sum256(raw)))
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
	require.Equal(t, 18, total)
}

func spTemplateKnownWholeNullTypeGuard(t *testing.T, sp map[string]any, ms []map[string]any, index int) bool {
	t.Helper()
	m := ms[index]
	if m["Value"] != nil {
		return false
	}
	require.Contains(t, m, "Value")
	require.Equal(t, uint64(19), m["Datatype"])
	require.Len(t, m, 3)
	if sp["Context Status"] != "matched" {
		require.Equal(t, "rebirth-requested", sp["Context Status"])
		require.Equal(t, "NCMD", sp["Message Type"])
		require.NotContains(t, sp, "Operation")
	}
	require.Contains(t, []string{"NDATA", "DDATA", "NCMD", "DCMD"}, sp["Message Type"])
	payload, ok := sp["Payload"].(map[string]any)
	require.True(t, ok)
	literal, ok := payload["metrics"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, literal, len(ms))
	require.Equal(t, true, literal[index]["is_null"])
	if typ, present := literal[index]["datatype"]; present {
		require.Equal(t, uint64(19), typ)
	}
	for _, k := range []string{"int_value", "long_value", "float_value", "double_value", "boolean_value", "string_value", "bytes_value", "dataset_value", "template_value", "extension_value"} {
		require.NotContains(t, literal[index], k)
	}
	return true
}

func TestSparkplugTemplateWholeNullStateExistingAPI(t *testing.T) {
	raw := spTemplateWholeNullSource(t, "whole-null-controls.json")
	require.Equal(t, "76a20dfd56240821c29151631212e90bf8a9c068c6414124fff7dc920cbdf853", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 4)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 42, total)
}

func TestSparkplugTemplateWholeNullPresenceExistingAPI(t *testing.T) {
	raw := spTemplateWholeNullSource(t, "presence-controls.json")
	require.Equal(t, "f613720c2f177f2472cbb6fb2bd3656e5ecc8ea0d8cfc3281b7c65bfc0c63201", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 1)
	total := 0
	for _, c := range gold.Cases {
		total += len(c.Steps)
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
	require.Equal(t, 26, total)
}

func TestSparkplugTemplateWholeNullCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spTemplateWholeNullSource, "sparkplug-template-whole-null", "42efe978e45d2f71fb2b2e6ce051a34260af719384e85af941baa9b34ccc574e", 4)
}

func TestSparkplugTemplateWholeNullMixedRebirthExistingAPI(t *testing.T) {
	raw := spTemplateWholeNullSource(t, "mixed-rebirth-controls.json")
	require.Equal(t, "0579a252ec21c029672a93bf759b1c04e229c0b764014a9369ff13a8f440d6ef", fmt.Sprintf("%x", sha256.Sum256(raw)))
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
	require.Equal(t, 18, total)
}

func TestSparkplugTemplateWholeNullBoundariesExistingAPI(t *testing.T) {
	raw := spTemplateWholeNullSource(t, "boundary-controls.json")
	require.Equal(t, "bbd3ff43eaa2e66650488f63cce01a79d35010f55e22f457174baa07b4b28f3c", fmt.Sprintf("%x", sha256.Sum256(raw)))
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
	require.Equal(t, 32, total)
}
