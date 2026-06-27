package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// WritePhase1DiscoveryReport generates phase1_discovery_report.md summarizing Stage0–probe outputs.
func WritePhase1DiscoveryReport(rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	var b strings.Builder
	b.WriteString("# Phase1: API 发现与验证报告\n\n")
	b.WriteString(fmt.Sprintf("生成时间: %s\n\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Session: `%s`\n\n", rt.Session.UUID))

	profile, _ := loadProjectProfile(rt.WorkDir)
	if profile != nil {
		b.WriteString("## 项目画像 (Stage0)\n\n")
		b.WriteString(fmt.Sprintf("- 文件数: %d\n", len(profile.Files)))
		b.WriteString(fmt.Sprintf("- Context-path: `%s` (来源: %s)\n", profile.ContextPath, profile.ContextPathSrc))
		if len(profile.Frameworks) > 0 {
			b.WriteString("- 框架探测:\n")
			for _, f := range profile.Frameworks {
				b.WriteString(fmt.Sprintf("  - %s (%s): %s\n", f.Label, f.Confidence, f.Evidence))
			}
		}
		b.WriteString("\n")
	}

	stages, _ := loadAllCodeReadingStages(rt.WorkDir)
	b.WriteString(fmt.Sprintf("## 分阶段代码阅读 (%d stages)\n\n", len(stages)))
	totalAPIs := 0
	for _, st := range stages {
		totalAPIs += len(st.APIFragments)
		b.WriteString(fmt.Sprintf("- Stage %d: 读 %d 文件, %d API 片段, next=%d\n",
			st.Stage, len(st.ReadFilesCompleted), len(st.APIFragments), len(st.NextWorklist)))
	}
	b.WriteString("\n")

	if tech, err := loadTechArchitectureRecord(rt.WorkDir); err == nil && tech != nil {
		b.WriteString("## 技术架构\n\n")
		b.WriteString(fmt.Sprintf("- language: `%s`\n", tech.Language))
		b.WriteString(fmt.Sprintf("- deployment: `%s`\n", tech.DeploymentModel))
		b.WriteString(fmt.Sprintf("- summary: %s\n", tech.SystemSummary))
		if len(tech.ModuleLayout.Modules) > 0 {
			b.WriteString("- modules:\n")
			for _, m := range tech.ModuleLayout.Modules {
				b.WriteString(fmt.Sprintf("  - %s (%s)\n", m.Name, m.Role))
			}
		}
		b.WriteString("\n")
	}

	if inv, err := loadJavaBusinessScopeInventory(rt.WorkDir); err == nil && inv != nil {
		b.WriteString("## Java 业务 Scope Inventory\n\n")
		b.WriteString(fmt.Sprintf("- layout: `%s`\n", inv.Layout))
		b.WriteString(fmt.Sprintf("- java_package units: %d\n\n", inv.Stats.JavaPackageUnits))
	}

	if biz, err := loadBusinessFunctionMap(rt.WorkDir); err == nil && biz != nil {
		b.WriteString("## 业务功能域\n\n")
		b.WriteString(fmt.Sprintf("- strategy: %s\n", biz.ClassificationStrategy))
		b.WriteString(fmt.Sprintf("- functions: %d\n", len(biz.Functions)))
		b.WriteString(fmt.Sprintf("- coverage: %d/%d complete=%v\n\n", biz.Coverage.Covered, biz.Coverage.TotalRequired, biz.Coverage.Complete))
		for name, fn := range biz.Functions {
			b.WriteString(fmt.Sprintf("- **%s**: %s (scopes=%d)\n", name, utils.ShrinkString(fn.Description, 80), len(fn.ScopePaths)))
		}
		b.WriteString("\n")
	}

	catalog, _ := loadApiCatalog(rt.WorkDir)
	if catalog != nil {
		b.WriteString("## API Catalog\n\n")
		b.WriteString(fmt.Sprintf("- 条目: %d\n", len(catalog.Entries)))
		b.WriteString(fmt.Sprintf("- 装配依据: %s\n", catalog.AssemblyBasis))
		b.WriteString(fmt.Sprintf("- Context-path: `%s`\n\n", catalog.ContextPath))
		b.WriteString("| Method | Path | Full URL | 证据 |\n|--------|------|----------|------|\n")
		limit := len(catalog.Entries)
		if limit > 100 {
			limit = 100
		}
		for i := 0; i < limit; i++ {
			e := catalog.Entries[i]
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				e.Method, e.PathPattern, utils.ShrinkString(e.FullURL, 60), utils.ShrinkString(e.CodeEvidence, 40)))
		}
		if len(catalog.Entries) > limit {
			b.WriteString(fmt.Sprintf("\n… 另有 %d 条未列出\n", len(catalog.Entries)-limit))
		}
		b.WriteString("\n")
	}

	if rt.Repo != nil {
		vha, _ := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
		verified := 0
		for _, v := range vha {
			if v.Verified {
				verified++
			}
		}
		b.WriteString("## 动态验证\n\n")
		b.WriteString(fmt.Sprintf("- verified_http_apis: %d 总计, %d verified\n\n", len(vha), verified))
	}

	if authB, err := os.ReadFile(store.AuthStatePath(rt.WorkDir)); err == nil {
		var rec authStateRecord
		if json.Unmarshal(authB, &rec) == nil {
			b.WriteString(fmt.Sprintf("## 鉴权状态\n\n- state: `%s`\n- detail: %s\n\n", rec.State, rec.Detail))
		}
	}

	if ev, err := loadAuthEvidenceFromWorkDir(rt.WorkDir); err == nil && ev != nil {
		b.WriteString("## 鉴权分析 (code reading auth_evidence)\n\n")
		b.WriteString(fmt.Sprintf("- verified: %v\n", ev.Verified))
		if ev.SessionMechanism != "" {
			b.WriteString(fmt.Sprintf("- session: %s\n", ev.SessionMechanism))
		}
		if ev.VerificationDetail != "" {
			b.WriteString(fmt.Sprintf("- detail: %s\n", ev.VerificationDetail))
		}
		for i, ep := range ev.LoginEndpoints {
			b.WriteString(fmt.Sprintf("- endpoint[%d]: %s %s (%s)\n", i, ep.Method, ep.Path, ep.ContentType))
			if ep.PasswordTransform != "" {
				b.WriteString(fmt.Sprintf("  - password_transform: %s (%s)\n", ep.PasswordTransform, ep.PasswordTransformEvidence))
			}
			if len(ep.FormFields) > 0 {
				fields, _ := json.Marshal(ep.FormFields)
				b.WriteString(fmt.Sprintf("  - form_fields: %s\n", string(fields)))
			}
			if ep.ProbeAttempted {
				b.WriteString(fmt.Sprintf("  - probe: attempted=%v succeeded=%v\n", ep.ProbeAttempted, ep.ProbeSucceeded))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## 引用文件\n\n")
	for _, p := range []struct{ path, desc string }{
		{store.ProjectProfilePath(rt.WorkDir), "project_profile.json"},
		{store.TechArchitecturePath(rt.WorkDir), "tech_architecture.json"},
		{store.JavaBusinessScopeInventoryPath(rt.WorkDir), "java_business_scope_inventory.json"},
		{store.BusinessFunctionMapPath(rt.WorkDir), "business_function_map.json"},
		{store.ApiCatalogPath(rt.WorkDir), "api_catalog.json"},
		{store.CodeReadingPlanPath(rt.WorkDir), "code_reading_plan.json"},
		{store.AuthStatePath(rt.WorkDir), "auth_state.json"},
		{store.AuthEvidencePath(rt.WorkDir), "auth_evidence.json"},
	} {
		if _, err := os.Stat(p.path); err == nil {
			b.WriteString(fmt.Sprintf("- %s — %s\n", p.path, p.desc))
		}
	}

	path := store.Phase1DiscoveryReportPath(rt.WorkDir)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: phase1_discovery_report written %s", path)
	return nil
}
