package ssaapi

import (
	"fmt"
	"strings"
)

// SourceStatistics describes saved sources, not risk attachments or a repository.
// Physical lines follow MemEditor: LF count + 1, including empty final lines.
type SourceStatistics struct {
	SchemaVersion     string `json:"schema_version"`
	Scope             string `json:"scope"`
	LineCountKind     string `json:"line_count_kind"`
	AnalyzedFileCount int64  `json:"analyzed_file_count"`
	AnalyzedLineCount int64  `json:"analyzed_line_count"`
}

// GetSourceStatistics fails closed when any visible source cannot be read.
// It never uses the cumulatively updated Program.LineCount.
func (p *Program) GetSourceStatistics() (*SourceStatistics, error) {
	if p == nil || p.Program == nil || p.Program.FileList == nil {
		return nil, fmt.Errorf("source statistics: missing program")
	}
	if overlay := p.GetOverlay(); overlay != nil && overlay.IsTopLayerProgram(p) {
		return overlay.GetSourceStatistics()
	}
	if p.IsIncrementalCompile() && !p.IsBaseProgram() {
		return nil, fmt.Errorf("source statistics: incremental view is unavailable")
	}
	lines := make(map[string]int64)
	for _, files := range []map[string]string{p.Program.FileList, p.Program.ExtraFile} {
		for path, hash := range files {
			pathKey := normalizeOverlayFilePath(path, p.GetProgramName())
			if pathKey == "" {
				return nil, fmt.Errorf("source statistics: invalid source path")
			}
			if _, exists := lines[pathKey]; exists {
				continue
			}
			editor, err := p.getEditor(path, hash)
			if err != nil || editor == nil {
				return nil, fmt.Errorf("source statistics: incomplete saved sources")
			}
			lines[pathKey] = int64(editor.GetLineCount())
		}
	}
	return sourceStatisticsFromLines(lines), nil
}

// GetSourceStatistics follows the same effective file ownership as aggregateFileSystems,
// but a failed read is an error instead of silently reducing the count.
func (p *ProgramOverLay) GetSourceStatistics() (*SourceStatistics, error) {
	if p == nil || p.Base == nil || p.Base.Program == nil || p.Base.Program.FileList == nil || len(p.Diff) == 0 {
		return nil, fmt.Errorf("source statistics: incomplete overlay")
	}
	lines := make(map[string]int64)
	for _, layer := range p.Diff {
		if layer == nil || layer.Program == nil || layer.Program.Program == nil {
			return nil, fmt.Errorf("source statistics: missing overlay layer")
		}
		for _, filePath := range layer.File {
			path := ensureOverlayPathSlash(filePath)
			content, ok := readStatisticsSource(layer.Program, path)
			if path == "" || !ok {
				return nil, fmt.Errorf("source statistics: incomplete overlay sources")
			}
			lines[path] = int64(strings.Count(content, "\n")) + 1
		}
	}
	excluded := overlayPathSet(p.ExcludeFile)
	for _, files := range []map[string]string{p.Base.Program.FileList, p.Base.Program.ExtraFile} {
		for filePath, hash := range files {
			path := normalizeOverlayFilePath(filePath, p.Base.GetProgramName())
			if path == "" {
				return nil, fmt.Errorf("source statistics: invalid source path")
			}
			if _, skip := excluded[path]; skip {
				continue
			}
			if _, owned := lines[path]; owned {
				continue
			}
			editor, err := p.Base.getEditor(filePath, hash)
			if err != nil || editor == nil {
				return nil, fmt.Errorf("source statistics: incomplete base sources")
			}
			lines[path] = int64(editor.GetLineCount())
		}
	}
	return sourceStatisticsFromLines(lines), nil
}

func sourceStatisticsFromLines(lines map[string]int64) *SourceStatistics {
	stats := &SourceStatistics{SchemaVersion: "ssa-source-statistics.v1", Scope: "compiled_sources", LineCountKind: "physical", AnalyzedFileCount: int64(len(lines))}
	for _, count := range lines {
		stats.AnalyzedLineCount += count
	}
	return stats
}

// ExtraFile keeps source-mode sidecars such as YAML/properties. They belong to
// the same saved-source scope as FileList and must survive overlay counting.
func readStatisticsSource(p *Program, path string) (string, bool) {
	if content, ok := readProgramFileContent(p, path); ok {
		return content, true
	}
	for filePath, hash := range p.Program.ExtraFile {
		if normalizeOverlayFilePath(filePath, p.GetProgramName()) != path {
			continue
		}
		editor, err := p.getEditor(filePath, hash)
		if err == nil && editor != nil {
			return editor.GetSourceCode(), true
		}
	}
	return "", false
}
