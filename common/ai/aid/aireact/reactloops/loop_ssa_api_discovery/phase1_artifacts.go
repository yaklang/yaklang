package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// buildPhase1ArtifactRefs returns markdown list of Phase1 workdir artifacts for report loops.
func buildPhase1ArtifactRefs(rt *Runtime, snapPath string) string {
	if rt == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("### 引用文件\n")
	if snapPath != "" {
		b.WriteString(fmt.Sprintf("- %s\n", snapPath))
	}
	for _, p := range []struct {
		path string
		desc string
	}{
		{store.Phase1PrepBundlePath(rt.WorkDir), "Phase1A 预分析汇总"},
		{store.ProjectProfilePath(rt.WorkDir), "Stage0 项目画像"},
		{store.TechArchitecturePath(rt.WorkDir), "技术架构分析"},
		{store.JavaBusinessScopeInventoryPath(rt.WorkDir), "Java 业务 scope inventory"},
		{store.BusinessFunctionMapPath(rt.WorkDir), "业务功能域 map"},
		{store.StaticRouteHintsPath(rt.WorkDir), "Phase1A 静态路由 hint（非权威）"},
		{store.CodeReadingPlanPath(rt.WorkDir), "Phase1B 代码阅读计划"},
		{store.ApiCatalogPath(rt.WorkDir), "API Catalog 装配"},
		{store.Phase1DiscoveryReportPath(rt.WorkDir), "Phase1 发现报告"},
		{store.RouteCandidatesPath(rt.WorkDir), "路由候选"},
		{store.ForwardingProfilePath(rt.WorkDir), "转发与 base 配置"},
		{store.AuthSurfacePath(rt.WorkDir), "鉴权面扫描"},
	} {
		if _, err := os.Stat(p.path); err == nil {
			b.WriteString(fmt.Sprintf("- %s — %s\n", p.path, p.desc))
		}
	}
	return b.String()
}
