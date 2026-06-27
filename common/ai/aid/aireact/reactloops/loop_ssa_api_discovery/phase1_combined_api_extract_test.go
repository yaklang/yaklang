package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const publicCMSBenchmarkRoot = "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/real-cms/PublicCMS/publiccms-parent"

func TestCombinedProgrammaticAPIExtraction_PublicCMS(t *testing.T) {
	if _, err := os.Stat(publicCMSBenchmarkRoot); err != nil {
		t.Skip("PublicCMS benchmark not available: ", publicCMSBenchmarkRoot)
	}

	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))

	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{
			CodeRootPath: publicCMSBenchmarkRoot,
			CodePathOK:   true,
			Language:     "java",
		},
	}

	catalog, err := RunCombinedProgrammaticAPIExtraction(rt)
	require.NoError(t, err)
	require.NotNil(t, catalog)
	require.Greater(t, catalog.Stats.Total, 50, "expected many backend endpoints from PublicCMS")
	t.Logf("combined catalog: total=%d merged=%d backend_only=%d frontend_only=%d csrf=%d session=%d",
		catalog.Stats.Total, catalog.Stats.MergedBoth, catalog.Stats.BackendOnly,
		catalog.Stats.FrontendOnly, catalog.Stats.WithCsrf, catalog.Stats.WithSessionAuth)

	save := findCombinedAPIRecordExact(catalog, "POST", "/admin/cmsCategory/save")
	require.NotNil(t, save, "expected cmsCategory/save in catalog")
	t.Logf("cmsCategory/save: method=%s path=%s auth=%+v params=%d sources=%v",
		save.Method, save.Path, save.Auth, len(save.Params), save.Sources)

	require.Equal(t, "POST", save.Method, "form save mutations should stay POST")
	require.True(t, strings.HasPrefix(save.Path, "/admin/"), "servlet prefix should be applied")
	require.Contains(t, save.Auth.Mechanisms, "csrf_token")
	require.Equal(t, AuthRealmAdmin, save.Auth.Realm)
	require.True(t, paramNamesContain(save.Params, "_csrf"), "expected _csrf param")

	listRec := findCombinedAPIRecordExact(catalog, "GET", "/admin/cmsContent/list")
	if listRec != nil {
		require.False(t, listRec.RequiresCsrf(), "read list endpoints must not require _csrf")
		t.Logf("cmsContent/list: method=%s csrf=%v mechanisms=%v", listRec.Method, listRec.RequiresCsrf(), listRec.Auth.Mechanisms)
	}

	deleteRec := findCombinedAPIRecordExact(catalog, "GET", "/admin/sysSite/delete")
	if deleteRec != nil {
		require.True(t, deleteRec.RequiresCsrf(), "sysSite/delete should be @Csrf")
		require.Equal(t, "GET", deleteRec.Method, "PublicCMS ajaxTodo delete stays GET")
		t.Logf("sysSite/delete: method=%s csrf=%v", deleteRec.Method, deleteRec.RequiresCsrf())
	}

	listRec = findCombinedAPIRecord(catalog, "", "cmsCategory/list")
	if listRec != nil {
		t.Logf("cmsCategory/list: method=%s path=%s sources=%v frontend_files=%v",
			listRec.Method, listRec.Path, listRec.Sources, listRec.FrontendFiles)
	}

	b, _ := json.MarshalIndent(catalog.Stats, "", "  ")
	t.Logf("stats: %s", string(b))
}

func TestCombinedProgrammaticAPIExtraction_MiniFixture(t *testing.T) {
	dir := t.TempDir()
	codeRoot := filepath.Join(dir, "repo")
	javaDir := filepath.Join(codeRoot, "module/src/main/java/com/publiccms/controller/admin/cms")
	tplDir := filepath.Join(codeRoot, "module/src/main/resources/templates/admin/cmsCategory")
	require.NoError(t, os.MkdirAll(javaDir, 0o755))
	require.NoError(t, os.MkdirAll(tplDir, 0o755))

	javaSrc := `package com.publiccms.controller.admin.cms;
import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.SessionAttribute;
import com.publiccms.common.annotation.Csrf;
@Controller
@RequestMapping("cmsCategory")
public class CmsCategoryAdminController {
    @RequestMapping("save")
    @Csrf
    public String save(@SessionAttribute SysUser admin, CmsCategory entity) { return "done"; }
    @RequestMapping("list")
    public String list() { return "list"; }
}`
	require.NoError(t, os.WriteFile(filepath.Join(javaDir, "CmsCategoryAdminController.java"), []byte(javaSrc), 0o644))

	ftl := `<form action="cmsCategory/save" method="post">
<input type="hidden" name="_csrf" value="x"/>
<input name="pageIndex" value="1"/>
<script>$.post('cmsCategory/list', {pageIndex:1, _csrf:'x'});</script>`
	require.NoError(t, os.WriteFile(filepath.Join(tplDir, "list.html"), []byte(ftl), 0o644))

	workDir := filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	servlet := `{"schema_version":1,"dispatchers":[{"id":"admin","url_prefix":"/admin","component_scan":"com.publiccms.controller.admin"}]}`
	require.NoError(t, writeJSONFile(store.ServletRoutingMapPath(workDir), []byte(servlet)))

	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{CodeRootPath: codeRoot, CodePathOK: true, Language: "java"},
	}

	catalog, err := RunCombinedProgrammaticAPIExtraction(rt)
	require.NoError(t, err)
	require.GreaterOrEqual(t, catalog.Stats.MergedBoth, 1)

	save := findCombinedAPIRecord(catalog, "POST", "cmsCategory/save")
	require.NotNil(t, save)
	require.Equal(t, "/admin/cmsCategory/save", save.Path)
	require.Contains(t, save.Sources, "backend")
	require.Contains(t, save.Sources, "frontend")
	require.Contains(t, save.Auth.Mechanisms, "csrf_token")
	require.True(t, paramNamesContain(save.Params, "_csrf"))
	require.True(t, paramNamesContain(save.Params, "pageIndex"))
}

func paramNamesContain(params []CombinedAPIParam, name string) bool {
	for _, p := range params {
		if p.Name == name {
			return true
		}
	}
	return false
}
