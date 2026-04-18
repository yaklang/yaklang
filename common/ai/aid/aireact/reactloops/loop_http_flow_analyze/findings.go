package loop_http_flow_analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	findingsKey       = "flow_analyze_findings"
	findingsFieldName = "findings"
	findingsAITagName = "FINDINGS"
	findingsAINodeID  = "flow-findings"

	findingsGeneralSection = "## General Findings"
)

type findingsSection struct {
	Title string
	Lines []string
	seen  map[string]struct{}
}

func normalizeFindings(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}

func parseFindingsSections(content string) []*findingsSection {
	content = normalizeFindings(content)
	if content == "" {
		return nil
	}

	sections := make([]*findingsSection, 0)
	sectionMap := make(map[string]*findingsSection)
	currentTitle := ""

	getSection := func(title string) *findingsSection {
		if title == "" {
			title = findingsGeneralSection
		}
		if sec, ok := sectionMap[title]; ok {
			return sec
		}
		sec := &findingsSection{
			Title: title,
			Lines: make([]string, 0),
			seen:  make(map[string]struct{}),
		}
		sectionMap[title] = sec
		sections = append(sections, sec)
		return sec
	}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			currentTitle = line
			getSection(currentTitle)
			continue
		}
		sec := getSection(currentTitle)
		if _, exists := sec.seen[line]; exists {
			continue
		}
		sec.seen[line] = struct{}{}
		sec.Lines = append(sec.Lines, line)
	}

	filtered := make([]*findingsSection, 0, len(sections))
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		filtered = append(filtered, sec)
	}
	return filtered
}

func mergeFindingsDocuments(existing, incoming string) string {
	sections := parseFindingsSections(existing)
	sectionMap := make(map[string]*findingsSection, len(sections))
	for _, sec := range sections {
		sectionMap[sec.Title] = sec
	}

	for _, inc := range parseFindingsSections(incoming) {
		target, ok := sectionMap[inc.Title]
		if !ok {
			target = &findingsSection{
				Title: inc.Title,
				Lines: make([]string, 0, len(inc.Lines)),
				seen:  make(map[string]struct{}, len(inc.Lines)),
			}
			sections = append(sections, target)
			sectionMap[inc.Title] = target
		}
		for _, line := range inc.Lines {
			if _, exists := target.seen[line]; exists {
				continue
			}
			target.seen[line] = struct{}{}
			target.Lines = append(target.Lines, line)
		}
	}

	var blocks []string
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		blocks = append(blocks, sec.Title+"\n\n"+strings.Join(sec.Lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

func appendFindings(loop *reactloops.ReActLoop, incoming string) (string, bool) {
	incoming = normalizeFindings(incoming)
	if incoming == "" {
		return loop.Get(findingsKey), false
	}

	existing := loop.Get(findingsKey)
	merged := mergeFindingsDocuments(existing, incoming)
	if merged == normalizeFindings(existing) {
		return merged, false
	}
	loop.Set(findingsKey, merged)
	return merged, true
}

func emitFindingsMarkdown(loop *reactloops.ReActLoop, findings string) {
	findings = normalizeFindings(findings)
	if findings == "" {
		return
	}

	taskIndex := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(findingsAINodeID, strings.NewReader(findings), taskIndex, func() {}); err != nil {
			log.Warnf("http_flow_analyze: emit findings markdown failed: %v", err)
		}
	}
}
