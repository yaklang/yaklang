package loop_http_flow_analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	httpFlowEvidenceKey        = "http_flow_analysis_evidence"
	httpFlowEvidenceActionName = "record_http_flow_evidence"
	httpFlowEvidenceFieldName  = "http_flow_evidence"
	httpFlowEvidenceAITagName  = "HTTP_FLOW_EVIDENCE"
	httpFlowEvidenceAINodeID   = "http-flow-analysis-evidence"

	httpFlowEvidenceGeneralSection = "## HTTP Flow Analysis Evidence"
)

type httpFlowEvidenceSection struct {
	Title string
	Lines []string
	seen  map[string]struct{}
}

func normalizeHTTPFlowEvidence(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}

func parseHTTPFlowEvidenceSections(content string) []*httpFlowEvidenceSection {
	content = normalizeHTTPFlowEvidence(content)
	if content == "" {
		return nil
	}

	sections := make([]*httpFlowEvidenceSection, 0)
	sectionMap := make(map[string]*httpFlowEvidenceSection)
	currentTitle := ""

	getSection := func(title string) *httpFlowEvidenceSection {
		if title == "" {
			title = httpFlowEvidenceGeneralSection
		}
		if sec, ok := sectionMap[title]; ok {
			return sec
		}
		sec := &httpFlowEvidenceSection{
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

	filtered := make([]*httpFlowEvidenceSection, 0, len(sections))
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		filtered = append(filtered, sec)
	}
	return filtered
}

func mergeHTTPFlowEvidenceDocuments(existing, incoming string) string {
	sections := parseHTTPFlowEvidenceSections(existing)
	sectionMap := make(map[string]*httpFlowEvidenceSection, len(sections))
	for _, sec := range sections {
		sectionMap[sec.Title] = sec
	}

	for _, inc := range parseHTTPFlowEvidenceSections(incoming) {
		target, ok := sectionMap[inc.Title]
		if !ok {
			target = &httpFlowEvidenceSection{
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

func appendHTTPFlowEvidence(loop *reactloops.ReActLoop, incoming string) (string, bool) {
	incoming = normalizeHTTPFlowEvidence(incoming)
	if incoming == "" {
		return loop.Get(httpFlowEvidenceKey), false
	}

	existing := loop.Get(httpFlowEvidenceKey)
	merged := mergeHTTPFlowEvidenceDocuments(existing, incoming)
	if merged == normalizeHTTPFlowEvidence(existing) {
		return merged, false
	}
	loop.Set(httpFlowEvidenceKey, merged)
	return merged, true
}

func emitHTTPFlowEvidenceMarkdown(loop *reactloops.ReActLoop, evidence string) {
	evidence = normalizeHTTPFlowEvidence(evidence)
	if evidence == "" {
		return
	}

	taskIndex := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(httpFlowEvidenceAINodeID, strings.NewReader(evidence), taskIndex, func() {}); err != nil {
			log.Warnf("http_flow_analyze: emit HTTP flow evidence markdown failed: %v", err)
		}
	}
}
