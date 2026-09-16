package loop_syntaxflow_rule

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// seedSyntaxFlowFromAttached seeds loop state from IRify UI attachments:
//   - Type=selected Key=syntaxflow_rule → existing rule draft in「规则编写」
//   - Type=selected Key=content → vulnerability sample (selection JSON or plain text)
func seedSyntaxFlowFromAttached(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	attached []*aicommon.AttachedResource,
) (hasRuleDraft bool, hasCodeSample bool) {
	attached = aicommon.NonEmptyAttachedResources(attached)
	if len(attached) == 0 {
		return false, false
	}

	reactloops.RunAttachedExtraResourcesInit(r, loop, attached)

	for _, data := range attached {
		if data == nil || !data.HasType(aicommon.AttachedResourceTypeSelected) {
			continue
		}
		value := strings.TrimSpace(data.Value)
		if value == "" {
			continue
		}

		if data.HasKey(aicommon.AttachedResourceKeySyntaxFlowRule) {
			loop.Set("full_sf_code", value)
			hasRuleDraft = true
			r.AddToTimeline("syntaxflow_rule_draft", fmt.Sprintf(
				"【规则编写草稿】已附带现有 SyntaxFlow 规则 (%s)\n%s",
				utils.ByteSize(uint64(len(value))),
				utils.ShrinkTextBlock(value, 800),
			))
			log.Infof("seeded full_sf_code from attached syntaxflow_rule (%d bytes)", len(value))
			continue
		}

		if !data.HasKey(aicommon.AttachedResourceKeyContent) {
			continue
		}

		sampleCode, sampleLang, sampleFilename := extractSyntaxFlowSampleFromAttached(value)
		if sampleCode == "" {
			continue
		}
		hasCodeSample = true
		applySyntaxFlowSampleToLoop(r, loop, sampleCode, sampleLang, sampleFilename)
	}
	return hasRuleDraft, hasCodeSample
}

func extractSyntaxFlowSampleFromAttached(raw string) (code, language, filename string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	if sel, ok := aicommon.ParseAttachedCodeSelection(&aicommon.AttachedResource{Value: raw}); ok && sel != nil {
		code = strings.TrimSpace(sel.Content)
		language = strings.TrimSpace(sel.Language)
		if sel.Path != "" {
			filename = sel.Path
			if i := strings.LastIndexAny(filename, `/\`); i >= 0 {
				filename = filename[i+1:]
			}
		}
		if language == "" {
			language = guessSampleLanguage(code, filename)
		}
		if filename == "" || !strings.Contains(filename, ".") {
			filename = defaultSampleFilename(language)
		}
		return code, language, filename
	}
	// Plain text selection (non-JSON)
	if looksLikeSourceCodeSample(raw) {
		language = guessSampleLanguage(raw, "")
		return raw, language, defaultSampleFilename(language)
	}
	return "", "", ""
}

func looksLikeSourceCodeSample(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 20 {
		return false
	}
	markers := []string{
		"package ", "func ", "import ", "public class", "def ", "<?php",
		"#include", "function ", "console.log", "fn ",
	}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func guessSampleLanguage(code, filename string) string {
	lowerName := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lowerName, ".go"):
		return "golang"
	case strings.HasSuffix(lowerName, ".java"):
		return "java"
	case strings.HasSuffix(lowerName, ".php"):
		return "php"
	case strings.HasSuffix(lowerName, ".py"):
		return "python"
	case strings.HasSuffix(lowerName, ".js"), strings.HasSuffix(lowerName, ".ts"):
		return "javascript"
	case strings.HasSuffix(lowerName, ".c"), strings.HasSuffix(lowerName, ".h"):
		return "c"
	case strings.HasSuffix(lowerName, ".yak"):
		return "yak"
	}
	switch {
	case strings.Contains(code, "package ") && strings.Contains(code, "func "):
		return "golang"
	case strings.Contains(code, "public class") || strings.Contains(code, "import java."):
		return "java"
	case strings.Contains(code, "<?php"):
		return "php"
	case strings.Contains(code, "def ") && strings.Contains(code, "import "):
		return "python"
	default:
		return "golang"
	}
}

func defaultSampleFilename(language string) string {
	lang, _ := ssaconfig.ValidateLanguage(language)
	ext := lang.GetFileExt()
	if ext == "" {
		ext = ".go"
	}
	return "sample" + ext
}

func applySyntaxFlowSampleToLoop(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	sampleCode, sampleLanguage, sampleFilename string,
) {
	lang, _ := ssaconfig.ValidateLanguage(sampleLanguage)
	ext := lang.GetFileExt()
	if ext == "" {
		ext = ".go"
	}
	if sampleFilename == "" || !strings.Contains(sampleFilename, ".") {
		sampleFilename = "sample" + ext
	}
	samplePath := r.EmitFileArtifactWithExt("gen_sample", ext, sampleCode)
	loop.Set("sf_sample_filepath", samplePath)
	loop.Set("sf_sample_code", sampleCode)
	loop.Set("sf_sample_language", sampleLanguage)
	loop.Set("sf_sample_filename", sampleFilename)
	loop.Set("sf_has_code_sample", true)
	samplePreview := utils.ShrinkTextBlock(sampleCode, 500)
	r.AddToTimeline("vulnerability_sample", fmt.Sprintf(
		"【漏洞样例】\n保存路径: %s\n虚拟文件名: %s\n语言: %s\n大小: %s\n\n预览:\n%s",
		samplePath, sampleFilename, sampleLanguage,
		utils.ByteSize(uint64(len(sampleCode))),
		samplePreview,
	))
	log.Infof("seeded vulnerability sample from attached content to %s (lang=%s)", samplePath, sampleLanguage)
}
