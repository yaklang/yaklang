package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryAllLibraryNames(t *testing.T) {
	names, err := QueryAllLibraryNames()
	require.NoError(t, err)
	require.NotEmpty(t, names)
	require.Contains(t, names, "str")
}

func TestQueryLibraryDetails_KnownLib(t *testing.T) {
	details, err := QueryLibraryDetails([]string{"str"})
	require.NoError(t, err)
	require.Contains(t, details, "str")
	require.NotEmpty(t, details["str"].Functions)
}

func TestQueryLibraryDetails_UnknownLib(t *testing.T) {
	_, err := QueryLibraryDetails([]string{"not_a_real_yak_lib_xyz"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestQueryFunctionDetails_KnownFunction(t *testing.T) {
	results, err := QueryFunctionDetails("str", []string{"Split"})
	require.NoError(t, err)
	require.NotNil(t, results["Split"])
	text := FormatFunctionDetails(results)
	require.Contains(t, strings.ToLower(text), "split")
}

func TestQueryFunctionDetails_NotFound(t *testing.T) {
	_, err := QueryFunctionDetails("str", []string{"NotARealFunctionXYZ"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestFormatLibraryDetails_Truncation(t *testing.T) {
	names := make([]string, yakdocMaxNameListItems+5)
	for i := range names {
		names[i] = "fn" + strings.Repeat("x", 4) + string(rune('a'+i%26))
	}
	text := formatNameList("Functions", names)
	require.Contains(t, text, "more")
}
