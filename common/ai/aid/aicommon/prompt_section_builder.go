package aicommon

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

const (
	PromptSectionHighStatic   = "high-static"
	PromptSectionSemiDynamic  = "semi-dynamic"
	PromptSectionSemiDynamic1 = "semi-dynamic-1"
	PromptSectionSemiDynamic2 = "semi-dynamic-2"
	PromptSectionTimelineOpen = "timeline-open"
	PromptSectionDynamic      = "dynamic"

	promptMessageSectionTagName = "PROMPT_SECTION"
	aiCacheSystemSectionTagName = "AI_CACHE_SYSTEM"
)

// SharedPlanAndExecHighStaticTemplate 是 plan/execution 系统级共享 high-static。
// 它必须完全无模板变量，避免污染 AI_CACHE_SYSTEM。
//
//go:embed prompts/shared/plan_and_exec/high_static_section.txt
var SharedPlanAndExecHighStaticTemplate string

//go:embed prompts/shared/plan_and_exec/semi_dynamic_1_section.txt
var SharedSemiDynamic1Template string

// SharedTaskInstructionSchemaExampleTemplate 复用 aireact 的
// TaskInstruction -> Schema -> OutputExample 半动态槽位顺序。
//
//go:embed prompts/shared/plan_and_exec/semi_dynamic_2_section.txt
var SharedTaskInstructionSchemaExampleTemplate string

// SharedFrozenBlockTemplate 复用 aireact 的 frozen-block 段模板。
//
//go:embed prompts/shared/plan_and_exec/frozen_block_section.txt
var SharedFrozenBlockTemplate string

// SharedTimelineOpenTemplate 复用 aireact 的 timeline-open 段模板。
//
//go:embed prompts/shared/plan_and_exec/timeline_open_section.txt
var SharedTimelineOpenTemplate string

type PromptPrefixBuilder struct {
	HighStaticTemplateName string
	HighStaticTemplate     string

	FrozenBlockTemplateName string
	FrozenBlockTemplate     string

	SemiDynamicTemplateName string
	SemiDynamicTemplate     string
	SemiDynamicSectionName  string
	ForceSemiDynamicSection bool

	SemiDynamic2TemplateName string
	SemiDynamic2Template     string
	SemiDynamic2SectionName  string

	TimelineOpenTemplateName string
	TimelineOpenTemplate     string
}

// NewDefaultPromptPrefixBuilder 返回本仓库统一使用的中性 prefix builder。
// 任何遵守同一套 five-section prefix + PROMPT_SECTION + AI_CACHE_* 契约的调用方
// 都应复用它，不要再包产品/场景专用的等价工厂。
func NewDefaultPromptPrefixBuilder() *PromptPrefixBuilder {
	return &PromptPrefixBuilder{
		HighStaticTemplateName:   "shared-high-static",
		HighStaticTemplate:       SharedPlanAndExecHighStaticTemplate,
		FrozenBlockTemplateName:  "shared-frozen-block",
		FrozenBlockTemplate:      SharedFrozenBlockTemplate,
		SemiDynamicTemplateName:  "shared-semi-dynamic-1",
		SemiDynamicTemplate:      SharedSemiDynamic1Template,
		SemiDynamicSectionName:   PromptSectionSemiDynamic1,
		ForceSemiDynamicSection:  true,
		SemiDynamic2TemplateName: "shared-semi-dynamic-2",
		SemiDynamic2Template:     SharedTaskInstructionSchemaExampleTemplate,
		SemiDynamic2SectionName:  PromptSectionSemiDynamic2,
		TimelineOpenTemplateName: "shared-timeline-open",
		TimelineOpenTemplate:     SharedTimelineOpenTemplate,
	}
}

type PromptPrefixAssemblyResult struct {
	Prompt       string
	HighStatic   string
	FrozenBlock  string
	SemiDynamic  string
	SemiDynamic2 string
	TimelineOpen string
}

func RenderPromptTemplate(name, templateContent string, data any) (string, error) {
	if strings.TrimSpace(templateContent) == "" {
		return "", nil
	}
	tmpl, err := template.New(name).Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("error parsing %s template: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing %s template: %w", name, err)
	}
	return buf.String(), nil
}

func (b *PromptPrefixBuilder) AssemblePromptPrefix(materials *PromptMaterials) (*PromptPrefixAssemblyResult, error) {
	if b == nil {
		return nil, fmt.Errorf("prompt prefix builder is nil")
	}
	if materials == nil {
		materials = &PromptMaterials{}
	}

	highStatic, err := RenderPromptTemplate(b.HighStaticTemplateName, b.HighStaticTemplate, materials.HighStaticData())
	if err != nil {
		return nil, err
	}
	frozenBlock, err := RenderPromptTemplate(b.FrozenBlockTemplateName, b.FrozenBlockTemplate, materials.FrozenBlockData())
	if err != nil {
		return nil, err
	}
	semiDynamic, err := RenderPromptTemplate(b.SemiDynamicTemplateName, b.SemiDynamicTemplate, materials.SemiDynamicData())
	if err != nil {
		return nil, err
	}
	semiDynamic2, err := RenderPromptTemplate(b.SemiDynamic2TemplateName, b.SemiDynamic2Template, materials.SemiDynamic2Data())
	if err != nil {
		return nil, err
	}
	timelineOpen, err := RenderPromptTemplate(b.TimelineOpenTemplateName, b.TimelineOpenTemplate, materials.TimelineOpenData())
	if err != nil {
		return nil, err
	}

	return &PromptPrefixAssemblyResult{
		Prompt:       JoinPromptSections(highStatic, frozenBlock, semiDynamic, semiDynamic2, timelineOpen),
		HighStatic:   highStatic,
		FrozenBlock:  frozenBlock,
		SemiDynamic:  semiDynamic,
		SemiDynamic2: semiDynamic2,
		TimelineOpen: timelineOpen,
	}, nil
}

func (b *PromptPrefixBuilder) AssemblePromptWithDynamicSection(
	materials *PromptMaterials,
	dynamicTemplateName string,
	dynamicTemplate string,
	dynamicData any,
	dynamicNonce string,
) (string, error) {
	prefix, err := b.AssemblePromptPrefix(materials)
	if err != nil {
		return "", err
	}
	dynamicSection, err := RenderPromptTemplate(dynamicTemplateName, dynamicTemplate, dynamicData)
	if err != nil {
		return "", err
	}
	return b.buildTaggedPromptSections(
		prefix.HighStatic,
		prefix.FrozenBlock,
		prefix.SemiDynamic,
		prefix.SemiDynamic2,
		prefix.TimelineOpen,
		dynamicSection,
		dynamicNonce,
	)
}

func (b *PromptPrefixBuilder) buildTaggedPromptSections(highStatic string, frozenBlock string, semiDynamic string, semiDynamic2 string, timelineOpen string, dynamic string, dynamicNonce string) (string, error) {
	return JoinPromptSections(
		wrapPromptMessageSectionWithForce(PromptSectionHighStatic, highStatic, "", false),
		WrapAICacheFrozen(frozenBlock),
		WrapAICacheSemi(wrapPromptMessageSectionWithForce(semiDynamicSectionName(b.SemiDynamicSectionName), semiDynamic, "", b.ForceSemiDynamicSection)),
		WrapAICacheSemi2(wrapPromptMessageSectionWithForce(semiDynamic2SectionName(b.SemiDynamic2SectionName), semiDynamic2, "", false)),
		wrapPromptMessageSectionWithForce(PromptSectionTimelineOpen, timelineOpen, "", false),
		wrapPromptMessageSectionWithForce(PromptSectionDynamic, dynamic, dynamicNonce, false),
	), nil
}

func BuildTaggedPromptSections(highStatic string, frozenBlock string, semiDynamic string, semiDynamic2 string, timelineOpen string, dynamic string, dynamicNonce string) string {
	return BuildTaggedPromptSectionsWithSectionNames(
		highStatic,
		frozenBlock,
		semiDynamic,
		PromptSectionSemiDynamic1,
		semiDynamic2,
		PromptSectionSemiDynamic2,
		timelineOpen,
		dynamic,
		dynamicNonce,
	)
}

func BuildTaggedPromptSectionsWithSectionNames(highStatic string, frozenBlock string, semiDynamic string, semiDynamicSection string, semiDynamic2 string, semiDynamic2Section string, timelineOpen string, dynamic string, dynamicNonce string) string {
	return BuildTaggedPromptSectionsWithSectionNamesAndForce(
		highStatic,
		frozenBlock,
		semiDynamic,
		semiDynamicSection,
		false,
		semiDynamic2,
		semiDynamic2Section,
		timelineOpen,
		dynamic,
		dynamicNonce,
	)
}

func BuildTaggedPromptSectionsWithSectionNamesAndForce(highStatic string, frozenBlock string, semiDynamic string, semiDynamicSection string, forceSemiDynamic bool, semiDynamic2 string, semiDynamic2Section string, timelineOpen string, dynamic string, dynamicNonce string) string {
	return JoinPromptSections(
		wrapPromptMessageSectionWithForce(PromptSectionHighStatic, highStatic, "", false),
		WrapAICacheFrozen(frozenBlock),
		WrapAICacheSemi(wrapPromptMessageSectionWithForce(semiDynamicSectionName(semiDynamicSection), semiDynamic, "", forceSemiDynamic)),
		WrapAICacheSemi2(wrapPromptMessageSectionWithForce(semiDynamic2SectionName(semiDynamic2Section), semiDynamic2, "", false)),
		wrapPromptMessageSectionWithForce(PromptSectionTimelineOpen, timelineOpen, "", false),
		wrapPromptMessageSectionWithForce(PromptSectionDynamic, dynamic, dynamicNonce, false),
	)
}

func semiDynamicSectionName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return PromptSectionSemiDynamic
	}
	return name
}

func semiDynamic2SectionName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return PromptSectionSemiDynamic2
	}
	return name
}

func WrapAICacheFrozen(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return fmt.Sprintf(
		"<|%s_%s|>\n%s\n<|%s_END_%s|>",
		TimelineFrozenBoundaryTagName,
		TimelineFrozenBoundaryNonce,
		content,
		TimelineFrozenBoundaryTagName,
		TimelineFrozenBoundaryNonce,
	)
}

func WrapAICacheSemi(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return fmt.Sprintf(
		"<|%s_%s|>\n%s\n<|%s_END_%s|>",
		SemiDynamicCacheBoundaryTagName,
		SemiDynamicCacheBoundaryNonce,
		content,
		SemiDynamicCacheBoundaryTagName,
		SemiDynamicCacheBoundaryNonce,
	)
}

func WrapAICacheSemi2(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return fmt.Sprintf(
		"<|%s_%s|>\n%s\n<|%s_END_%s|>",
		SemiDynamicPart2CacheBoundaryTagName,
		SemiDynamicPart2CacheBoundaryNonce,
		content,
		SemiDynamicPart2CacheBoundaryTagName,
		SemiDynamicPart2CacheBoundaryNonce,
	)
}

func WrapPromptMessageSection(sectionName string, content string, nonce string) string {
	return wrapPromptMessageSectionWithForce(sectionName, content, nonce, false)
}

func wrapPromptMessageSectionWithForce(sectionName string, content string, nonce string, force bool) string {
	content = strings.TrimSpace(content)
	if content == "" && !force {
		return ""
	}
	if sectionName == PromptSectionDynamic && nonce != "" {
		tagName := fmt.Sprintf("%s_%s", promptMessageSectionTagName, sectionName)
		return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tagName, nonce, content, tagName, nonce)
	}
	if sectionName == PromptSectionHighStatic {
		return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", aiCacheSystemSectionTagName, sectionName, content, aiCacheSystemSectionTagName, sectionName)
	}
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", promptMessageSectionTagName, sectionName, content, promptMessageSectionTagName, sectionName)
}

func JoinPromptSections(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "\n\n")
}
