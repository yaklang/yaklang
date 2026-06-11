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

func TestSearchYakDocument_ByKeyword(t *testing.T) {
	hits, err := SearchYakDocument("Split", 10, "str")
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	found := false
	for _, hit := range hits {
		if hit.Name == "Split" {
			found = true
			break
		}
	}
	require.True(t, found, "expected str.Split in search results")
}

func TestSearchYakDocument_EmptyQuery(t *testing.T) {
	_, err := SearchYakDocument("", 10, "")
	require.Error(t, err)
}

func TestEnrichExternFieldError_KnownCase(t *testing.T) {
	msg := `ExternLib [poc] don't has [appendHeade], maybe you meant appendHeader ?`
	enriched := EnrichExternFieldError(msg)
	require.Contains(t, enriched, "已自动附加 YakDocument")
	require.Contains(t, enriched, "appendHeader")
	require.Contains(t, enriched, "相近函数")
}

func TestEnrichExternFieldError_NonMatching(t *testing.T) {
	require.Empty(t, EnrichExternFieldError("syntax error at line 1"))
}

func TestFormatLibraryDetails_Truncation(t *testing.T) {
	names := make([]string, yakdocMaxNameListItems+5)
	for i := range names {
		names[i] = "fn" + strings.Repeat("x", 4) + string(rune('a'+i%26))
	}
	text := formatNameList("Functions", names)
	require.Contains(t, text, "more")
}
