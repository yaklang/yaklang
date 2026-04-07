package policy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMinimalPolicy(t *testing.T) {
	data := []byte(`{"obfuscators":[{"name":"callret"}]}`)
	p, err := Parse(data)
	require.NoError(t, err)
	require.Len(t, p.Obfuscators, 1)
	require.Equal(t, "callret", p.Obfuscators[0].Name)
}

func TestParseFullPolicy(t *testing.T) {
	data := []byte(`{
		"seed": 42,
		"obfuscators": [
			{
				"name": "virtualize",
				"category": "body-replace",
				"selector": {
					"include": ["helper*"],
					"exclude": ["helperInternal"],
					"ratio": 0.5,
					"allow_entry": false,
					"min_blocks": 2,
					"min_insts": 5
				}
			},
			{
				"name": "callret",
				"category": "callflow"
			},
			{
				"name": "mba",
				"category": "llvm-local"
			}
		]
	}`)
	p, err := Parse(data)
	require.NoError(t, err)
	require.Equal(t, int64(42), p.Seed)
	require.Len(t, p.Obfuscators, 3)

	virt := p.Obfuscators[0]
	require.Equal(t, "virtualize", virt.Name)
	require.Equal(t, CategoryBodyReplace, virt.Category)
	require.Equal(t, []string{"helper*"}, virt.Selector.Include)
	require.Equal(t, []string{"helperInternal"}, virt.Selector.Exclude)
	require.NotNil(t, virt.Selector.Ratio)
	require.InDelta(t, 0.5, *virt.Selector.Ratio, 0.001)
	require.Equal(t, 2, virt.Selector.MinBlocks)
	require.Equal(t, 5, virt.Selector.MinInsts)
}

func TestParseEmptyNameFails(t *testing.T) {
	data := []byte(`{"obfuscators":[{"name":""}]}`)
	_, err := Parse(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty name")
}

func TestParseRatioAndCountMutuallyExclusive(t *testing.T) {
	data := []byte(`{"obfuscators":[{"name":"test","selector":{"ratio":0.5,"count":3}}]}`)
	_, err := Parse(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestParseRatioOutOfRange(t *testing.T) {
	data := []byte(`{"obfuscators":[{"name":"test","selector":{"ratio":1.5}}]}`)
	_, err := Parse(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ratio must be between")
}

func TestParseNegativeCountFails(t *testing.T) {
	data := []byte(`{"obfuscators":[{"name":"test","selector":{"count":-1}}]}`)
	_, err := Parse(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-negative")
}

func TestObfuscatorNames(t *testing.T) {
	p := &Policy{
		Obfuscators: []ObfEntry{
			{Name: "callret"},
			{Name: "virtualize"},
			{Name: "mba"},
		},
	}
	require.Equal(t, []string{"callret", "virtualize", "mba"}, p.ObfuscatorNames())
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse([]byte(`{invalid`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse obf policy")
}

func TestLoadFileEmptyPath(t *testing.T) {
	_, err := LoadFile("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty policy file path")
}

func TestLoadFileNonExistent(t *testing.T) {
	_, err := LoadFile("/nonexistent/path.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read policy file")
}
