package vulnreport

import (
	"context"
	"sort"
	"strings"
	"time"
)

func BuildSnapshotFromEntities[T VulnerabilityEntity](
	ctx context.Context,
	entities []T,
	meta *BuildSnapshotMeta,
) (*VulnerabilityReportSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if meta == nil {
		meta = &BuildSnapshotMeta{}
	}

	snapshotMeta := buildSnapshotMeta(entities, meta)
	snapshot := &VulnerabilityReportSnapshot{
		SchemaVersion: defaultSchemaVersion(meta.SchemaVersion),
		SourceKind:    strings.TrimSpace(meta.SourceKind),
		TemplateID:    strings.TrimSpace(meta.TemplateID),
		Meta:          snapshotMeta,
		Findings:      make([]VulnerabilityFinding, 0, len(entities)),
	}

	riskTypeCounter := make(map[string]int)
	ruleCounter := make(map[string]int)
	languageCounter := make(map[string]int)
	cweCounter := make(map[string]int)
	disposalCounter := make(map[string]int)

	for _, entity := range entities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		finding := buildFinding(entity, snapshot.SourceKind)
		snapshot.Findings = append(snapshot.Findings, finding)
		snapshot.Summary.Total++

		switch finding.Severity {
		case "critical":
			snapshot.Summary.Critical++
		case "high":
			snapshot.Summary.High++
		case "medium":
			snapshot.Summary.Medium++
		case "low":
			snapshot.Summary.Low++
		}

		if finding.IsPotential {
			snapshot.Summary.PotentialCount++
		}

		addMetric(riskTypeCounter, finding.RiskType, "unknown")
		addMetric(ruleCounter, finding.FromRule, "unknown")
		addMetric(languageCounter, finding.Language, "unknown")
		addMetric(disposalCounter, finding.LatestDisposalStatus, "not_set")
		for _, cwe := range finding.CWE {
			addMetric(cweCounter, cwe, "")
		}
	}

	snapshot.Summary.RiskTypes = sortedMetrics(riskTypeCounter)
	snapshot.Summary.Rules = sortedMetrics(ruleCounter)
	snapshot.Summary.Languages = sortedMetrics(languageCounter)
	snapshot.Summary.CWEs = sortedMetrics(cweCounter)
	snapshot.Summary.DisposalStatuses = sortedMetrics(disposalCounter)
	return snapshot, nil
}

func buildSnapshotMeta[T VulnerabilityEntity](entities []T, meta *BuildSnapshotMeta) VulnerabilityReportMeta {
	reportName := strings.TrimSpace(meta.ReportName)
	projectName := strings.TrimSpace(meta.ProjectName)
	programName := strings.TrimSpace(meta.ProgramName)
	taskID := strings.TrimSpace(meta.TaskID)

	taskIDs := make(map[string]struct{})
	for _, entity := range entities {
		if projectName == "" {
			projectName = strings.TrimSpace(entity.GetProgramName())
		}
		if programName == "" {
			programName = strings.TrimSpace(entity.GetProgramName())
		}
		addUniqueTask(taskIDs, entity.GetTaskID())
		addUniqueTask(taskIDs, entity.GetRuntimeID())
		if taskID == "" {
			taskID = firstNonEmpty(entity.GetTaskID(), entity.GetRuntimeID())
		}
	}

	taskCount := meta.TaskCount
	if taskCount <= 0 {
		taskCount = int64(len(taskIDs))
	}
	if taskCount <= 0 && taskID != "" {
		taskCount = 1
	}

	generatedAt := meta.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}

	scopeName := strings.TrimSpace(meta.ScopeName)
	if scopeName == "" {
		scopeName = firstNonEmpty(projectName, programName, reportName, "漏洞报告")
	}

	scopeType := strings.TrimSpace(meta.ScopeType)
	if scopeType == "" {
		switch {
		case taskCount > 1:
			scopeType = "task-set"
		case taskID != "":
			scopeType = "task"
		case projectName != "" || programName != "":
			scopeType = "project"
		default:
			scopeType = "manual"
		}
	}

	if reportName == "" {
		reportName = firstNonEmpty(projectName, programName, "漏洞报告")
	}

	return VulnerabilityReportMeta{
		ReportName:       reportName,
		ScopeType:        scopeType,
		ScopeName:        scopeName,
		ProjectName:      projectName,
		ProgramName:      programName,
		TaskID:           taskID,
		TaskCount:        taskCount,
		ScanBatch:        meta.ScanBatch,
		GeneratedAt:      generatedAt,
		SourceFinishedAt: meta.SourceFinishedAt,
		Owner:            strings.TrimSpace(meta.Owner),
		Filters:          append([]string(nil), meta.Filters...),
	}
}

func buildFinding(entity VulnerabilityEntity, sourceKind string) VulnerabilityFinding {
	severity := NormalizeSeverity(entity.GetSeverity())
	title := strings.TrimSpace(entity.GetTitle())
	titleVerbose := strings.TrimSpace(entity.GetTitleVerbose())
	return VulnerabilityFinding{
		SourceKind:           sourceKind,
		SourceID:             strings.TrimSpace(entity.GetSourceID()),
		Hash:                 strings.TrimSpace(entity.GetHash()),
		RiskFeatureHash:      strings.TrimSpace(entity.GetRiskFeatureHash()),
		Title:                title,
		TitleVerbose:         titleVerbose,
		DisplayTitle:         firstNonEmpty(titleVerbose, title, entity.GetHash()),
		Severity:             severity,
		SeverityLabel:        SeverityLabel(severity),
		RiskType:             firstNonEmpty(entity.GetRiskType(), "unknown"),
		Description:          strings.TrimSpace(entity.GetDescription()),
		Solution:             strings.TrimSpace(entity.GetSolution()),
		ProgramName:          strings.TrimSpace(entity.GetProgramName()),
		TaskID:               strings.TrimSpace(entity.GetTaskID()),
		RuntimeID:            strings.TrimSpace(entity.GetRuntimeID()),
		Language:             strings.TrimSpace(entity.GetLanguage()),
		FromRule:             strings.TrimSpace(entity.GetFromRule()),
		CWE:                  compactValues(entity.GetCWEList()),
		Tags:                 compactValues(entity.GetTags()),
		CodeSourceURL:        strings.TrimSpace(entity.GetCodeSourceURL()),
		CodeRange:            strings.TrimSpace(entity.GetCodeRange()),
		CodeFragment:         strings.TrimSpace(entity.GetCodeFragment()),
		FunctionName:         strings.TrimSpace(entity.GetFunctionName()),
		Line:                 entity.GetLine(),
		LatestDisposalStatus: firstNonEmpty(entity.GetLatestDisposalStatus(), "not_set"),
		CreatedAt:            cloneTimePtr(entity.GetCreatedAt()),
		UpdatedAt:            cloneTimePtr(entity.GetUpdatedAt()),
		IsPotential:          entity.IsPotentialRisk(),
	}
}

func NormalizeSeverity(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "critical", "panic", "fatal", "严重":
		return "critical"
	case "high", "高危":
		return "high"
	case "middle", "medium", "warning", "warn", "中危":
		return "medium"
	case "low", "低危":
		return "low"
	default:
		return value
	}
}

func SeverityLabel(value string) string {
	switch NormalizeSeverity(value) {
	case "critical":
		return "严重"
	case "high":
		return "高危"
	case "medium":
		return "中危"
	case "low":
		return "低危"
	default:
		return "未知"
	}
}

func defaultSchemaVersion(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return DefaultSchemaVersion
}

func addMetric(counter map[string]int, value string, fallback string) {
	label := strings.TrimSpace(value)
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	if label == "" {
		return
	}
	counter[label]++
}

func sortedMetrics(counter map[string]int) []VulnerabilityMetric {
	metrics := make([]VulnerabilityMetric, 0, len(counter))
	for label, value := range counter {
		metrics = append(metrics, VulnerabilityMetric{
			Label: label,
			Value: value,
		})
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Value == metrics[j].Value {
			return metrics[i].Label < metrics[j].Label
		}
		return metrics[i].Value > metrics[j].Value
	})
	return metrics
}

func addUniqueTask(target map[string]struct{}, value string) {
	key := strings.TrimSpace(value)
	if key == "" {
		return
	}
	target[key] = struct{}{}
}

func compactValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
