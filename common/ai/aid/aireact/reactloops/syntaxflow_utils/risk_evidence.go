package syntaxflow_utils

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RiskReviewMode controls whether disposal writes are allowed for SSA risk review flows.
type RiskReviewMode string

const (
	RiskReviewModeAnalyze        RiskReviewMode = "analyze"
	RiskReviewModeAnalyzeDispose RiskReviewMode = "analyze_dispose"
)

// ParseRiskReviewMode maps wire strings (attachments / action params) to RiskReviewMode.
func ParseRiskReviewMode(s string) RiskReviewMode {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "analyze", "read_only", "readonly":
		return RiskReviewModeAnalyze
	case "analyze_dispose", "dispose", "analyze+dispose", "analyze-dispose", "write":
		return RiskReviewModeAnalyzeDispose
	default:
		return RiskReviewModeAnalyze
	}
}

// RiskEvidence centralizes SSA-risk-related reads (source IR FS, dataflow graph, audit chain)
// and disposal writes so ReAct loops do not scatter gorm/yakit/ssaapi calls.
//
// Design: there is only one production implementation today (default SSA project DB +
// yak helpers). We use a concrete struct instead of an interface to avoid boilerplate;
// if multiple backends or mocks are needed later, extract an interface from these methods.
type RiskEvidence struct {
	db *gorm.DB
}

var (
	riskEvidenceOnce     sync.Once
	riskEvidenceSingleton *RiskEvidence
)

// NewRiskEvidence constructs a helper bound to the given DB (typically SSA project DB).
func NewRiskEvidence(db *gorm.DB) *RiskEvidence {
	return &RiskEvidence{db: db}
}

// DefaultRiskEvidence uses GetSSADB().
func DefaultRiskEvidence() *RiskEvidence {
	return NewRiskEvidence(GetSSADB())
}

// GetRiskEvidence returns a process-wide singleton using GetSSADB().
func GetRiskEvidence() *RiskEvidence {
	riskEvidenceOnce.Do(func() {
		riskEvidenceSingleton = DefaultRiskEvidence()
	})
	return riskEvidenceSingleton
}

func (e *RiskEvidence) dbOrErr() (*gorm.DB, error) {
	if e == nil || e.db == nil {
		return nil, utils.Error("SSA database not available")
	}
	return e.db, nil
}

// LoadRisk loads one SSA risk row by primary key.
func (e *RiskEvidence) LoadRisk(id int64) (*schema.SSARisk, error) {
	db, err := e.dbOrErr()
	if err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, utils.Error("risk id must be positive")
	}
	return yakit.GetSSARiskByID(db, id)
}

// ListDisposals lists disposal rows for a risk id.
func (e *RiskEvidence) ListDisposals(riskID int64, includeInherited bool) ([]schema.SSARiskDisposals, error) {
	db, err := e.dbOrErr()
	if err != nil {
		return nil, err
	}
	if includeInherited {
		return yakit.GetSSARiskDisposalsWithInheritance(db, riskID)
	}
	return yakit.GetSSARiskDisposalsOnly(db, riskID)
}

// WriteDisposal creates disposal rows via yakit; blocked unless mode is RiskReviewModeAnalyzeDispose.
func (e *RiskEvidence) WriteDisposal(ids []int64, status, comment string, mode RiskReviewMode) ([]schema.SSARiskDisposals, error) {
	if mode != RiskReviewModeAnalyzeDispose {
		return nil, utils.Error("disposal disabled in analyze-only mode")
	}
	db, err := e.dbOrErr()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, utils.Error("risk ids empty")
	}
	st := NormalizeDisposalStatus(status)
	req := &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: ids,
		Status:  st,
		Comment: strings.TrimSpace(comment),
	}
	return yakit.CreateSSARiskDisposals(db, req)
}

// ResolveRiskIDs returns risk primary keys matching filter up to capLimit (page 1, newest first).
func (e *RiskEvidence) ResolveRiskIDs(filter *ypb.SSARisksFilter, capLimit int64) ([]int64, int64, error) {
	db, err := e.dbOrErr()
	if err != nil {
		return nil, 0, err
	}
	if filter == nil {
		filter = &ypb.SSARisksFilter{}
	}
	if capLimit <= 0 {
		capLimit = DefaultOverviewPageLimit
	}
	total, err := yakit.QuerySSARiskCount(db, filter)
	if err != nil {
		return nil, 0, err
	}
	paging := &ypb.Paging{Page: 1, Limit: capLimit, OrderBy: "id", Order: "desc"}
	_, risks, err := yakit.QuerySSARisk(db, filter, paging)
	if err != nil {
		return nil, int64(total), err
	}
	ids := make([]int64, 0, len(risks))
	for _, rk := range risks {
		if rk != nil {
			ids = append(ids, int64(rk.ID))
		}
	}
	return ids, int64(total), nil
}

// OverviewPage returns total count and the first page of risks (newest first), for overview preface/summary.
func (e *RiskEvidence) OverviewPage(filter *ypb.SSARisksFilter, listLimit int64) (total int, risks []*schema.SSARisk, err error) {
	db, err := e.dbOrErr()
	if err != nil {
		return 0, nil, err
	}
	if filter == nil {
		filter = &ypb.SSARisksFilter{}
	}
	if listLimit <= 0 {
		listLimit = DefaultOverviewPageLimit
	}
	total, err = yakit.QuerySSARiskCount(db, filter)
	if err != nil {
		return 0, nil, err
	}
	paging := &ypb.Paging{Page: 1, Limit: listLimit, OrderBy: "id", Order: "desc"}
	_, risks, err = yakit.QuerySSARisk(db, filter, paging)
	return total, risks, err
}

// --- Source slice (ssadb IR FS, same backing as ssadb:// yakurl) ---

// RiskSourceSlice is a window of source text from the IR-backed filesystem.
type RiskSourceSlice struct {
	ProgramName   string `json:"program_name"`
	FilePath      string `json:"file_path"`
	SourceType    string `json:"source_type"` // database | local_fs | none
	Content       string `json:"content"`
	StartLine     int    `json:"start_line"`
	LinesReturned int    `json:"lines_returned"`
	TotalLines    int    `json:"total_lines"`
	HasMore       bool   `json:"has_more"`
	Error           string `json:"error,omitempty"`
}

// LoadSource reads a source file slice from DB-backed IR FS or local compile path fallback.
func (e *RiskEvidence) LoadSource(programName, filePath string, startLine, lineCount int) (*RiskSourceSlice, error) {
	if strings.TrimSpace(programName) == "" {
		return nil, utils.Error("program_name is required")
	}
	filePath = strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	if filePath == "" {
		return nil, utils.Error("file_path is required")
	}
	if startLine < 1 {
		startLine = 1
	}
	if lineCount <= 0 {
		lineCount = 100
	}
	out := &RiskSourceSlice{
		ProgramName: programName,
		FilePath:    filePath,
		StartLine:   startLine,
		SourceType:  "none",
	}

	formatContent := func(sourceCode string) {
		allLines := strings.Split(sourceCode, "\n")
		totalLines := len(allLines)
		out.TotalLines = totalLines
		endLine := startLine + lineCount - 1
		if startLine > totalLines {
			out.Error = fmt.Sprintf("start line %d beyond total lines %d", startLine, totalLines)
			return
		}
		if endLine > totalLines {
			endLine = totalLines
		}
		selected := allLines[startLine-1 : endLine]
		out.LinesReturned = len(selected)
		out.HasMore = endLine < totalLines
		var sb strings.Builder
		for i, line := range selected {
			sb.WriteString(fmt.Sprintf("%6d | %s\n", startLine+i, line))
		}
		out.Content = sb.String()
	}

	irfs := ssadb.NewIrSourceFs()
	fullPath := "/" + programName + "/" + filePath
	content, err := irfs.ReadFile(fullPath)
	if err == nil {
		out.SourceType = "database"
		formatContent(string(content))
		return out, nil
	}

	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err == nil && irProg != nil && irProg.ConfigInput != "" {
		localPath, ok := tryGetLocalPathFromRiskEvidenceConfig(irProg.ConfigInput)
		if ok && localPath != "" {
			exists, _ := filesys.NewLocalFs().Exists(localPath)
			if exists {
				localFs := filesys.NewLocalFs()
				fullLocal := localFs.Join(localPath, filePath)
				b, err := localFs.ReadFile(fullLocal)
				if err == nil {
					out.SourceType = "local_fs"
					formatContent(string(b))
					return out, nil
				}
			}
		}
	}

	out.Error = fmt.Sprintf("file %q not readable in program %q", filePath, programName)
	return out, nil
}

// RiskFileEntry is one file under a program in IR FS.
type RiskFileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// RiskFileList is a page of source files for grep/list actions.
type RiskFileList struct {
	ProgramName string          `json:"program_name"`
	SourceType  string          `json:"source_type"`
	Files       []RiskFileEntry `json:"files"`
	TotalCount  int             `json:"total_count"`
	Returned    int             `json:"returned_count"`
	HasMore     bool            `json:"has_more"`
	Error       string          `json:"error,omitempty"`
}

// ListFiles lists source files under programName with optional path prefix.
func (e *RiskEvidence) ListFiles(programName, pathPrefix string, offset, limit int) (*RiskFileList, error) {
	if strings.TrimSpace(programName) == "" {
		return nil, utils.Error("program_name is required")
	}
	pathPrefix = strings.TrimPrefix(strings.TrimSpace(pathPrefix), "/")
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	out := &RiskFileList{ProgramName: programName, SourceType: "none"}

	collect := func(fsys filesys_interface.FileSystem, basePath, stripPrefix string) []RiskFileEntry {
		all := []RiskFileEntry{}
		filesys.Recursive(
			basePath,
			filesys.WithFileSystem(fsys),
			filesys.WithFileStat(func(filepath string, info fs.FileInfo) error {
				if info.IsDir() {
					return nil
				}
				rel := filepath
				if stripPrefix != "" {
					rel = strings.TrimPrefix(filepath, stripPrefix)
					rel = strings.TrimPrefix(rel, "/")
				}
				if pathPrefix != "" && !strings.HasPrefix(rel, pathPrefix) {
					return nil
				}
				all = append(all, RiskFileEntry{Path: rel, Size: info.Size()})
				return nil
			}),
		)
		return all
	}

	irfs := ssadb.NewIrSourceFs()
	progPath := "/" + programName
	if _, err := irfs.ReadDir(progPath); err == nil {
		all := collect(irfs, progPath, progPath)
		out.SourceType = "database"
		out.TotalCount = len(all)
		end := offset + limit
		if offset >= len(all) {
			out.Files = nil
		} else {
			if end > len(all) {
				end = len(all)
			}
			out.Files = all[offset:end]
			out.Returned = len(out.Files)
		}
		out.HasMore = offset+out.Returned < len(all)
		return out, nil
	}

	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err == nil && irProg != nil && irProg.ConfigInput != "" {
		localPath, ok := tryGetLocalPathFromRiskEvidenceConfig(irProg.ConfigInput)
		if ok && localPath != "" {
			if exists, _ := filesys.NewLocalFs().Exists(localPath); exists {
				relFs := filesys.NewRelLocalFs(localPath)
				all := collect(relFs, ".", "")
				out.SourceType = "local_fs"
				out.TotalCount = len(all)
				end := offset + limit
				if offset >= len(all) {
					out.Files = nil
				} else {
					if end > len(all) {
						end = len(all)
					}
					out.Files = all[offset:end]
					out.Returned = len(out.Files)
				}
				out.HasMore = offset+out.Returned < len(all)
				return out, nil
			}
		}
	}

	out.Error = fmt.Sprintf("program %q source not accessible", programName)
	return out, nil
}

// RiskGrepMatch is one grep hit in program source.
type RiskGrepMatch struct {
	File        string `json:"file"`
	LineNumber  int    `json:"line_number"`
	LineContent string `json:"line_content"`
	Context     string `json:"context"`
}

// RiskGrepResult aggregates grep results over IR FS or local fallback.
type RiskGrepResult struct {
	ProgramName string           `json:"program_name"`
	Pattern     string           `json:"pattern"`
	SourceType  string           `json:"source_type"`
	Matches     []RiskGrepMatch  `json:"matches"`
	Truncated   bool             `json:"truncated"`
	Error       string           `json:"error,omitempty"`
}

// GrepOption configures RiskEvidence.Grep.
type GrepOption struct {
	PatternMode string // substr | isubstr | regexp
	FilePattern string
	ContextLines int
	MaxResults   int
}

// Grep searches source text under a program (same backing as ssa-grep tool).
func (e *RiskEvidence) Grep(programName, pattern string, opt GrepOption) (*RiskGrepResult, error) {
	if strings.TrimSpace(programName) == "" || strings.TrimSpace(pattern) == "" {
		return nil, utils.Error("program_name and pattern are required")
	}
	if opt.ContextLines < 0 {
		opt.ContextLines = 0
	}
	if opt.MaxResults <= 0 {
		opt.MaxResults = 20
	}
	mode := strings.ToLower(strings.TrimSpace(opt.PatternMode))
	if mode == "" {
		mode = "substr"
	}
	var patternRe *regexp.Regexp
	var err error
	switch mode {
	case "regexp":
		patternRe, err = regexp.Compile(pattern)
		if err != nil {
			return &RiskGrepResult{Error: fmt.Sprintf("invalid regexp: %v", err)}, nil
		}
	case "isubstr":
		patternRe = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
	default:
		patternRe = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	matchFile := func(name string) bool {
		fp := opt.FilePattern
		if fp == "" {
			return true
		}
		if strings.HasPrefix(fp, "*.") {
			return strings.HasSuffix(name, fp[1:])
		}
		return strings.Contains(name, fp)
	}

	out := &RiskGrepResult{
		ProgramName: programName,
		Pattern:     pattern,
		SourceType:  "none",
	}

	searchIn := func(relPath string, lines []string) {
		for lineIdx, line := range lines {
			if len(out.Matches) >= opt.MaxResults {
				return
			}
			idxs := patternRe.FindAllStringIndex(line, -1)
			if len(idxs) == 0 {
				continue
			}
			startCtx := lineIdx - opt.ContextLines
			if startCtx < 0 {
				startCtx = 0
			}
			endCtx := lineIdx + opt.ContextLines + 1
			if endCtx > len(lines) {
				endCtx = len(lines)
			}
			var ctx strings.Builder
			for i := startCtx; i < endCtx; i++ {
				prefix := "  "
				if i == lineIdx {
					prefix = "> "
				}
				ctx.WriteString(fmt.Sprintf("%s%4d | %s\n", prefix, i+1, lines[i]))
			}
			out.Matches = append(out.Matches, RiskGrepMatch{
				File:        relPath,
				LineNumber:  lineIdx + 1,
				LineContent: line,
				Context:     ctx.String(),
			})
		}
	}

	irfs := ssadb.NewIrSourceFs()
	progPath := "/" + programName
	if _, err := irfs.ReadDir(progPath); err == nil {
		out.SourceType = "database"
		filesys.Recursive(
			progPath,
			filesys.WithFileSystem(irfs),
			filesys.WithFileStat(func(filepath string, info fs.FileInfo) error {
				if info.IsDir() || len(out.Matches) >= opt.MaxResults {
					return nil
				}
				content, err := irfs.ReadFile(filepath)
				if err != nil {
					return nil
				}
				rel := strings.TrimPrefix(filepath, progPath)
				rel = strings.TrimPrefix(rel, "/")
				if !matchFile(rel) {
					return nil
				}
				searchIn(rel, strings.Split(string(content), "\n"))
				return nil
			}),
		)
		out.Truncated = len(out.Matches) >= opt.MaxResults
		return out, nil
	}

	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err == nil && irProg != nil && irProg.ConfigInput != "" {
		localPath, ok := tryGetLocalPathFromRiskEvidenceConfig(irProg.ConfigInput)
		if ok && localPath != "" {
			if exists, _ := filesys.NewLocalFs().Exists(localPath); exists {
				out.SourceType = "local_fs"
				localFs := filesys.NewLocalFs()
				relFs := filesys.NewRelLocalFs(localPath)
				filesys.Recursive(
					".",
					filesys.WithFileSystem(relFs),
					filesys.WithFileStat(func(filepath string, info fs.FileInfo) error {
						if info.IsDir() || len(out.Matches) >= opt.MaxResults {
							return nil
						}
						if !matchFile(filepath) {
							return nil
						}
						full := localFs.Join(localPath, filepath)
						content, err := localFs.ReadFile(full)
						if err != nil {
							return nil
						}
						searchIn(filepath, strings.Split(string(content), "\n"))
						return nil
					}),
				)
				out.Truncated = len(out.Matches) >= opt.MaxResults
				return out, nil
			}
		}
	}

	out.Error = fmt.Sprintf("program %q source not grep-able", programName)
	return out, nil
}

func tryGetLocalPathFromRiskEvidenceConfig(configInput string) (string, bool) {
	var configInfo map[string]any
	if err := json.Unmarshal([]byte(configInput), &configInfo); err != nil {
		return "", false
	}
	codeSource, ok := configInfo["CodeSource"].(map[string]any)
	if !ok {
		return "", false
	}
	kind, _ := codeSource["kind"].(string)
	localFile, _ := codeSource["local_file"].(string)
	if kind == "local" && localFile != "" {
		return localFile, true
	}
	return "", false
}

// LoadDataflowGraph builds the same graph summary as syntaxflow yakurl Value2Response (dot + node infos).
func (e *RiskEvidence) LoadDataflowGraph(risk *schema.SSARisk) (*ssaapi.GraphInfo, error) {
	if risk == nil {
		return nil, utils.Error("nil risk")
	}
	v, err := yakurl.GetValueByRiskHash(risk.ProgramName, risk)
	if err != nil {
		return nil, utils.Wrap(err, "resolve SSA value for dataflow graph")
	}
	if v == nil {
		return nil, utils.Error("nil SSA value for dataflow graph")
	}
	return v.GetGraphInfo(), nil
}

// AuditChainStep is one hop in an audit-node walk from the risk root value.
type AuditChainStep struct {
	Depth    int    `json:"depth"`
	Relation string `json:"relation"`
	UUID     string `json:"uuid"`
	IR       string `json:"ir"`
}

// AuditChainReport is a bounded walk over audit edges from the risk anchor node.
type AuditChainReport struct {
	ProgramName string            `json:"program_name"`
	RiskID      int64             `json:"risk_id"`
	Steps       []AuditChainStep  `json:"steps"`
	Truncated   bool              `json:"truncated"`
	Error       string            `json:"error,omitempty"`
}

// LoadAuditChain walks predecessors / depend-on / effect-on relations up to maxNodes steps.
func (e *RiskEvidence) LoadAuditChain(risk *schema.SSARisk, maxNodes int) (*AuditChainReport, error) {
	if risk == nil {
		return nil, utils.Error("nil risk")
	}
	if maxNodes <= 0 {
		maxNodes = 24
	}
	rep := &AuditChainReport{
		ProgramName: risk.ProgramName,
		RiskID:      int64(risk.ID),
	}
	v, err := yakurl.GetValueByRiskHash(risk.ProgramName, risk)
	if err != nil || v == nil {
		rep.Error = fmt.Sprintf("resolve anchor value: %v", err)
		return rep, nil
	}

	type qItem struct {
		v    *ssaapi.Value
		dep  int
		rel  string
	}
	queue := []qItem{{v: v, dep: 0, rel: "root"}}
	seen := map[string]bool{}

	for len(queue) > 0 && len(rep.Steps) < maxNodes {
		it := queue[0]
		queue = queue[1:]
		if it.v == nil {
			continue
		}
		id := it.v.GetUUID()
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ir := strings.TrimSpace(it.v.String())
		if len(ir) > 500 {
			ir = ir[:500] + "..."
		}
		rep.Steps = append(rep.Steps, AuditChainStep{
			Depth:    it.dep,
			Relation: it.rel,
			UUID:     id,
			IR:       ir,
		})
		if len(rep.Steps) >= maxNodes {
			rep.Truncated = true
			break
		}
		for _, pv := range it.v.GetPredecessors() {
			if pv != nil && pv.Node != nil {
				queue = append(queue, qItem{pv.Node, it.dep + 1, "predecessor"})
			}
		}
		for _, d := range it.v.GetDependOn() {
			queue = append(queue, qItem{d, it.dep + 1, "depend_on"})
		}
		for _, ef := range it.v.GetEffectOn() {
			queue = append(queue, qItem{ef, it.dep + 1, "effect_on"})
		}
	}
	return rep, nil
}
