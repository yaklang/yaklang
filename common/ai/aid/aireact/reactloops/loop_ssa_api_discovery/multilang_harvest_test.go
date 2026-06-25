package loop_ssa_api_discovery

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHarvestGoHTTPMappings(t *testing.T) {
	root := filepath.Join("testdata", "minimal_go_http")
	eps, err := HarvestGoHTTPMappings(root)
	require.NoError(t, err)
	paths := map[string]string{}
	for _, e := range eps {
		paths[e.PathPattern] = e.Method
	}
	require.Equal(t, "GET", paths["/api/ping"])
	require.Equal(t, "POST", paths["/api/items"])
	require.Equal(t, "GET", paths["/health"])
}

func TestHarvestJavaScriptHTTPMappings(t *testing.T) {
	root := filepath.Join("testdata", "minimal_express")
	eps, err := HarvestJavaScriptHTTPMappings(root)
	require.NoError(t, err)
	paths := map[string]string{}
	for _, e := range eps {
		paths[e.PathPattern] = e.Method
	}
	require.Equal(t, "GET", paths["/users"])
	require.Equal(t, "POST", paths["/items"])
}

func TestHarvestTypeScriptHTTPMappings_Nest(t *testing.T) {
	root := filepath.Join("testdata", "minimal_nest_ts")
	eps, err := HarvestTypeScriptHTTPMappings(root)
	require.NoError(t, err)
	var sawProfile bool
	for _, e := range eps {
		if normURLPath(e.PathPattern) == "/profile" && e.Method == "GET" {
			sawProfile = true
		}
	}
	require.True(t, sawProfile, "expected Nest @Get('profile')")
}

func TestHarvestPythonHTTPMappings(t *testing.T) {
	root := filepath.Join("testdata", "minimal_python_http")
	eps, err := HarvestPythonHTTPMappings(root)
	require.NoError(t, err)
	paths := map[string]string{}
	for _, e := range eps {
		k := e.Method + " " + e.PathPattern
		paths[k] = e.Provenance
	}
	require.Contains(t, paths, "GET /hello")
	require.Contains(t, paths, "POST /legacy")
	require.Contains(t, paths, "GET /items")
}

func TestHarvestPHPHTTPMappings(t *testing.T) {
	root := filepath.Join("testdata", "minimal_laravel_routes")
	eps, err := HarvestPHPHTTPMappings(root)
	require.NoError(t, err)
	paths := map[string]string{}
	for _, e := range eps {
		paths[e.PathPattern] = e.Method
	}
	require.Equal(t, "GET", paths["/dashboard"])
	require.Equal(t, "POST", paths["api/store"])
}

func TestLanguageHasStaticHarvester(t *testing.T) {
	require.True(t, LanguageHasStaticHarvester("java"))
	require.True(t, LanguageHasStaticHarvester("Java"))
	require.True(t, LanguageHasStaticHarvester("golang"))
	require.True(t, LanguageHasStaticHarvester("javascript"))
	require.True(t, LanguageHasStaticHarvester("js"))
	require.True(t, LanguageHasStaticHarvester("ts"))
	require.True(t, LanguageHasStaticHarvester("python"))
	require.True(t, LanguageHasStaticHarvester("php"))
	require.False(t, LanguageHasStaticHarvester("c"))
	require.False(t, LanguageHasStaticHarvester("yak"))
	require.False(t, LanguageHasStaticHarvester("general"))
}
