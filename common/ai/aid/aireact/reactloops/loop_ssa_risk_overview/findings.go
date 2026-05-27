package loop_ssa_risk_overview

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

var (
	riskIDRangeRE  = regexp.MustCompile(`\b(\d{2,})\s*[-–—]\s*(\d{2,})\b`)
	riskIDTokenRE  = regexp.MustCompile(`\b\d{2,}\b`)
)

const (
	overviewFindingsKey       = "ssa_overview_findings"
	overviewFindingsFieldName = "findings"
	overviewFindingsAITagName = "FINDINGS"
	overviewFindingsAINodeID  = "ssa-overview-findings"

	overviewFindingsGeneralSection = "## Overview Findings"
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
			title = overviewFindingsGeneralSection
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

// extractRiskIDsFromFindings pulls numeric risk ids from markdown findings (supports "9829-9832" and lists).
func extractRiskIDsFromFindings(text string) []string {
	seen := make(map[string]struct{})
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
	}
	for _, m := range riskIDRangeRE.FindAllStringSubmatch(text, -1) {
		lo, err1 := strconv.ParseInt(m[1], 10, 64)
		hi, err2 := strconv.ParseInt(m[2], 10, 64)
		if err1 == nil && err2 == nil && lo <= hi && hi-lo <= 32 {
			for v := lo; v <= hi; v++ {
				add(strconv.FormatInt(v, 10))
			}
			continue
		}
		add(m[1])
		add(m[2])
	}
	for _, tok := range riskIDTokenRE.FindAllString(text, -1) {
		add(tok)
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func existingCoversRiskCluster(existing string, ids []string) bool {
	if strings.TrimSpace(existing) == "" || len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !strings.Contains(existing, id) {
			return false
		}
	}
	return true
}

func appendOverviewFindings(loop *reactloops.ReActLoop, incoming string) (string, bool) {
	incoming = normalizeFindings(incoming)
	if incoming == "" {
		return loop.Get(overviewFindingsKey), false
	}

	existing := loop.Get(overviewFindingsKey)
	if ids := extractRiskIDsFromFindings(incoming); len(ids) > 0 && existingCoversRiskCluster(existing, ids) {
		return existing, false
	}
	merged := mergeFindingsDocuments(existing, incoming)
	if merged == normalizeFindings(existing) {
		return merged, false
	}
	loop.Set(overviewFindingsKey, merged)
	return merged, true
}

func emitOverviewFindingsMarkdown(loop *reactloops.ReActLoop, findings string) {
	findings = normalizeFindings(findings)
	if findings == "" {
		return
	}
	taskIndex := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(overviewFindingsAINodeID, strings.NewReader(findings), taskIndex, func() {}); err != nil {
			log.Warnf("ssa_risk_overview: emit findings markdown failed: %v", err)
		}
	}
}
