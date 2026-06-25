package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestHarvestFrontendCallsFromFTL(t *testing.T) {
	dir := t.TempDir()
	codeRoot := filepath.Join(dir, "repo")
	templates := filepath.Join(codeRoot, "admin/src/main/resources/templates/cms")
	require.NoError(t, os.MkdirAll(templates, 0o755))
	ftl := `<form action="cmsCategory/save" method="post">
<input type="hidden" name="_csrf" value="tok"/>
<input name="pageIndex" value="1"/>
<script>
$.ajax({url:'cmsCategory/list', type:'POST', data:{pageIndex:1, _csrf:'tok'}});
</script>`
	require.NoError(t, os.WriteFile(filepath.Join(templates, "list.ftl"), []byte(ftl), 0o644))

	workDir := filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	servlet := `{"schema_version":1,"dispatchers":[{"id":"admin","url_prefix":"/admin","component_scan":"com.admin"}]}`
	require.NoError(t, writeJSONFile(store.ServletRoutingMapPath(workDir), []byte(servlet)))

	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{CodeRootPath: codeRoot, CodePathOK: true, Language: "java"},
	}
	calls := harvestFrontendCallsFromSource(rt, []byte(ftl), "admin/src/main/resources/templates/cms/list.ftl")
	require.NotEmpty(t, calls)
	var paths []string
	for _, c := range calls {
		paths = append(paths, c.PathResolved)
	}
	require.Contains(t, paths, "/admin/cmsCategory/list")
	require.Contains(t, paths, "/admin/cmsCategory/save")
}

func TestControllerStemFromEntryFile(t *testing.T) {
	require.Equal(t, "cmsCategory", controllerStemFromEntryFile("com/foo/CmsCategoryAdminController.java"))
}

func TestBuildFrontendAPIHintsBlock(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	inv := FrontendAPIInventory{
		Calls: []FrontendAPICall{{
			Method:       "POST",
			PathResolved: "/admin/cmsCategory/list",
			Params:       []FrontendAPIParam{{Name: "_csrf", Location: "post", Required: true}},
		}},
	}
	b, _ := json.MarshalIndent(inv, "", "  ")
	require.NoError(t, writeJSONFile(store.FrontendAPIInventoryPath(dir), b))
	rt := &Runtime{WorkDir: dir}
	job := FeatureWorkJob{EntryFile: "com/x/CmsCategoryAdminController.java"}
	block := buildFrontendAPIHintsBlock(rt, job)
	require.Contains(t, block, "frontend_api_hints")
	require.Contains(t, block, "cmsCategory/list")
}


