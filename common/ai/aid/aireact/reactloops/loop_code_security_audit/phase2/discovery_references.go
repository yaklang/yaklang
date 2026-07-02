package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// ReferenceResource describes an on-disk artifact the discovery sub-agent may read on demand.
// Content is NOT inlined into the sub-loop prompt — only pointers and reading guidance.
type ReferenceResource struct {
	ID          string
	Path        string
	Kind        string // recon_report | recon_outline | category_spec
	Description string
	SuggestWhen string
}

// BuildDiscoveryReferenceCatalog lists optional materials for fast_context / deep discovery.
func BuildDiscoveryReferenceCatalog(state *model.AuditState, category model.VulnCategory) []ReferenceResource {
	if state == nil {
		return nil
	}
	var out []ReferenceResource

	if p := strings.TrimSpace(state.GetReconFilePath()); p != "" {
		out = append(out, ReferenceResource{
			ID:          "recon_report",
			Path:        p,
			Kind:        "recon_report",
			Description: "Phase1 项目背景报告（路由、模块目录、技术栈、数据流模式）",
			SuggestWhen: "首轮 grep 命中过少、或需要确认模块/路由/框架专属写法时再读；勿全文背诵，按需截取相关章节",
		})
	}
	if outline := strings.TrimSpace(state.GetReconOutline()); outline != "" {
		out = append(out, ReferenceResource{
			ID:          "recon_outline",
			Path:        "(in-memory outline, see parent reference_material)",
			Kind:        "recon_outline",
			Description: "侦察报告章节目录摘要（已在 reference_material 中给出短版本）",
			SuggestWhen: "判断是否需要 read_file 打开完整 recon_report",
		})
	}
	out = append(out, ReferenceResource{
		ID:          "category_spec",
		Path:        "(embedded in reference_material)",
		Kind:        "category_spec",
		Description: fmt.Sprintf("漏洞类别 %s (%s) 的 Source/Sink 语义与搜索策略", category.Name, category.ID),
		SuggestWhen: "推导 grep pattern 时必读（已内联 Sink/Source 提示）",
	})
	return out
}

// RenderDiscoveryReferenceCatalog formats the catalog for fast_context persistent context.
func RenderDiscoveryReferenceCatalog(catalog []ReferenceResource) string {
	if len(catalog) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### 可选参考材料（按需 read_file，不要假设已读过全文）\n\n")
	b.WriteString("通过 `require_tool` + `read_file` 读取下列路径；仅在需要更深上下文时读取，避免浪费迭代。\n\n")
	for i, r := range catalog {
		b.WriteString(fmt.Sprintf("%d. **%s** (`%s`)\n", i+1, r.ID, r.Kind))
		if r.Path != "" && !strings.HasPrefix(r.Path, "(") {
			b.WriteString(fmt.Sprintf("   - 路径: `%s`\n", r.Path))
		}
		b.WriteString(fmt.Sprintf("   - 说明: %s\n", r.Description))
		if r.SuggestWhen != "" {
			b.WriteString(fmt.Sprintf("   - 何时读: %s\n", r.SuggestWhen))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// BuildFastContextReferenceMaterial builds lean inline context plus an optional-read catalog.
func BuildFastContextReferenceMaterial(state *model.AuditState, category model.VulnCategory) string {
	var b strings.Builder
	b.WriteString("## 代码安全审计 Phase2 阶段A 搜索上下文\n\n")
	if state.ProjectPath != "" {
		b.WriteString("- 项目绝对路径: ")
		b.WriteString(state.ProjectPath)
		b.WriteByte('\n')
	}
	if state.TechStack != "" {
		b.WriteString("- 技术栈: ")
		b.WriteString(state.TechStack)
		b.WriteByte('\n')
	}
	if state.EntryPoints != "" {
		b.WriteString("- 入口点: ")
		b.WriteString(state.EntryPoints)
		b.WriteByte('\n')
	}
	if outline := strings.TrimSpace(state.GetReconOutline()); outline != "" {
		b.WriteString("\n### 背景报告大纲（短摘要；完整内容见可选参考 recon_report）\n")
		b.WriteString(utils.ShrinkString(outline, 400))
		b.WriteByte('\n')
	}
	if hints := strings.TrimSpace(category.RenderSinkHints()); hints != "" {
		b.WriteString("\n### 本类别 Sink 语义提示（据此推导并行 grep pattern）\n")
		b.WriteString(hints)
	}
	b.WriteString("\n### 搜索策略要求\n")
	b.WriteString("- 优先 `grep_files_batch` 并行 4-8 个 pattern（files_with_matches）\n")
	b.WriteString("- 每种 Sink/Source 语义至少一个 pattern；技术栈专属写法优先\n")
	b.WriteString("- 命中过少时：再 `read_file` 打开 recon_report 或抽查疑似文件，然后追加 grep 轮次\n")
	b.WriteString("- 返回候选路径即可；深度审计由父 loop Phase B 完成\n")

	catalog := BuildDiscoveryReferenceCatalog(state, category)
	if cat := RenderDiscoveryReferenceCatalog(catalog); cat != "" {
		b.WriteString("\n\n")
		b.WriteString(cat)
	}
	return strings.TrimSpace(b.String())
}
