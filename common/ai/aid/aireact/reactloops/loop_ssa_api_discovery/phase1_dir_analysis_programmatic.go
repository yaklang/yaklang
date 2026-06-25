package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

// RunDirectoryAnalysisProgrammatic builds the directory tree and runs heuristic BFS analysis
// without requiring an AI runtime. This is suitable for standalone testing and focus modes.
func RunDirectoryAnalysisProgrammatic(ctx context.Context, workDir string, codeRoot string) (*DirectoryTreeV1, error) {
	if strings.TrimSpace(codeRoot) == "" {
		return nil, fmt.Errorf("empty code root path")
	}

	javaRoot := FindJavaRoot(codeRoot)
	if javaRoot == "" {
		return nil, fmt.Errorf("cannot find Java source root under %s", codeRoot)
	}
	log.Infof("ssa_dir_analysis[programmatic]: Java root=%s", javaRoot)

	tree := BuildDirectoryTree(javaRoot)
	if tree == nil {
		return nil, fmt.Errorf("failed to build directory tree")
	}
	log.Infof("ssa_dir_analysis[programmatic]: tree built: %d dirs, %d files, %d KB total",
		tree.TotalDirs, tree.TotalFiles, tree.TotalSizeKB)

	projCtx, _ := loadProjectContextSummary(workDir)

	currentLevel := GetRootNodeIDs(tree)
	for levelIdx := 0; len(currentLevel) > 0; levelIdx++ {
		log.Infof("ssa_dir_analysis[programmatic]: analyzing level %d with %d dirs", levelIdx, len(currentLevel))
		if err := analyzeLevelProgrammatic(tree, currentLevel, projCtx, codeRoot); err != nil {
			log.Warnf("ssa_dir_analysis[programmatic]: level %d error: %v", levelIdx, err)
		}

		nextIDs := CollectNextLevelIDs(tree, currentLevel)
		if len(nextIDs) == 0 {
			log.Infof("ssa_dir_analysis[programmatic]: BFS finished at level %d", levelIdx)
			break
		}
		currentLevel = nextIDs
	}
	FinalizeDirectoryTreeAnalysis(tree, projCtx, codeRoot)

	rt := &Runtime{WorkDir: workDir, Session: &store.DiscoverySession{CodeRootPath: codeRoot}}
	if err := persistDirectoryTree(rt, tree); err != nil {
		return nil, fmt.Errorf("persist directory tree: %w", err)
	}
	log.Infof("ssa_dir_analysis[programmatic]: persisted to %s", store.DirectoryAnalysisPath(workDir))

	return tree, nil
}

func analyzeLevelProgrammatic(tree *DirectoryTreeV1, levelIDs []string, projCtx *ProjectContextSummaryV1, codeRoot string) error {
	var firstErr error
	for _, id := range levelIDs {
		if ShouldSkipDirAnalysis(tree, id) {
			continue
		}
		node := tree.GetNode(id)
		if node == nil {
			continue
		}
		if node.Analysis != nil && node.Analysis.BfsControl == BfsControlStop {
			continue
		}
		analysis, err := analyzeDirectoryProgrammatic(node, projCtx, codeRoot)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for i := range tree.Nodes {
			if tree.Nodes[i].ID == id {
				tree.Nodes[i].Analysis = analysis
				break
			}
		}
	}
	return firstErr
}

func analyzeDirectoryProgrammatic(node *DirectoryNode, projCtx *ProjectContextSummaryV1, codeRoot string) (*DirAnalysis, error) {
	if node == nil {
		return nil, fmt.Errorf("nil node")
	}

	if prog, ok := tryProgrammaticDirAnalysis(node, projCtx, codeRoot); ok {
		return sanitizeDirAnalysis(node, projCtx, codeRoot, prog), nil
	}

	rel := node.RelPath
	name := filepath.Base(rel)
	hasController := false
	hasService := false
	hasRepository := false
	hasEntity := false
	hasConfig := false
	hasFilter := false
	hasSecurity := false
	var httpEntryFiles []string
	var bizDomains []string
	var dbFeatures []string
	javaFileCount := 0

	for _, fn := range node.FileNames {
		if !strings.HasSuffix(fn, ".java") {
			continue
		}
		javaFileCount++

		lower := strings.ToLower(fn)
		if strings.Contains(lower, "controller") || strings.Contains(lower, "rest") {
			hasController = true
			httpEntryFiles = append(httpEntryFiles, fn)
		}
		if strings.Contains(lower, "service") {
			hasService = true
		}
		if strings.Contains(lower, "repository") || strings.Contains(lower, "dao") || strings.Contains(lower, "mapper") {
			hasRepository = true
		}
		if strings.Contains(lower, "entity") || strings.Contains(lower, "model") || strings.Contains(lower, "pojo") {
			hasEntity = true
		}
		if strings.Contains(lower, "config") {
			hasConfig = true
		}
		if strings.Contains(lower, "filter") || strings.Contains(lower, "interceptor") {
			hasFilter = true
		}
		if strings.Contains(lower, "security") {
			hasSecurity = true
		}
	}

	var techLayers []string
	if hasController {
		techLayers = append(techLayers, "tech:controller")
	}
	if hasService {
		techLayers = append(techLayers, "tech:service")
	}
	if hasRepository {
		techLayers = append(techLayers, "tech:dao")
	}
	if hasEntity {
		techLayers = append(techLayers, "tech:entity")
	}
	if hasConfig {
		techLayers = append(techLayers, "tech:config")
	}
	if hasFilter {
		techLayers = append(techLayers, "tech:filter")
	}
	if hasSecurity {
		techLayers = append(techLayers, "tech:security")
	}
	if len(techLayers) == 0 {
		techLayers = []string{"tech:other"}
	}

	pkg := strings.ToLower(node.PackageHint)
	relLower := strings.ToLower(rel)
	bizDomainHints := map[string]bool{
		"user":    strings.Contains(relLower, "user") || strings.Contains(pkg, "user"),
		"trade":   strings.Contains(relLower, "order") || strings.Contains(relLower, "trade") || strings.Contains(pkg, "order"),
		"oauth":   strings.Contains(relLower, "auth") || strings.Contains(relLower, "login") || strings.Contains(pkg, "auth"),
		"file":    strings.Contains(relLower, "file") || strings.Contains(relLower, "upload") || strings.Contains(pkg, "file"),
		"sys":     strings.Contains(relLower, "sys") || strings.Contains(relLower, "config") || strings.Contains(pkg, "sys"),
		"log":     strings.Contains(relLower, "log") || strings.Contains(relLower, "audit") || strings.Contains(pkg, "log"),
		"api":     hasController,
		"web":     hasController && strings.Contains(relLower, "web"),
		"cms":     strings.Contains(relLower, "content") || strings.Contains(relLower, "cms"),
	}
	for k, ok := range bizDomainHints {
		if ok {
			bizDomains = append(bizDomains, "biz:"+k)
		}
	}
	if len(bizDomains) == 0 {
		bizDomains = []string{}
	}

	if hasRepository || hasEntity {
		dbFeatures = append(dbFeatures, "db:none")
	}
	if strings.Contains(relLower, "jpa") || strings.Contains(pkg, "jpa") {
		dbFeatures = append(dbFeatures, "db:jpa")
	}
	if strings.Contains(relLower, "mybatis") || strings.Contains(pkg, "mybatis") || strings.Contains(relLower, "mapper") {
		dbFeatures = append(dbFeatures, "db:mybatis")
	}
	if len(dbFeatures) == 0 {
		dbFeatures = []string{}
	}

	isBusiness := javaFileCount > 0
	isHTTPEntry := hasController && len(httpEntryFiles) > 0
	hasDB := len(dbFeatures) > 0

	desc := businessDescription(name, techLayers, bizDomains)

	return &DirAnalysis{
		FunctionDesc:   desc,
		TechLayers:     techLayers,
		BizDomains:     bizDomains,
		DbFeatures:     dbFeatures,
		BfsControl:     BfsControlContinue,
		IsBusiness:     isBusiness,
		IsHttpEntry:    isHTTPEntry,
		HasDB:          hasDB,
		HttpEntryFiles: httpEntryFiles,
	}, nil
}

func businessDescription(name string, techLayers, bizDomains []string) string {
	segments := []string{}
	for _, t := range techLayers {
		switch t {
		case "tech:controller":
			segments = append(segments, "HTTP接口层")
		case "tech:service":
			segments = append(segments, "业务服务层")
		case "tech:dao":
			segments = append(segments, "数据访问层")
		case "tech:entity":
			segments = append(segments, "数据实体层")
		case "tech:config":
			segments = append(segments, "配置与初始化")
		case "tech:filter":
			segments = append(segments, "请求拦截器")
		case "tech:security":
			segments = append(segments, "安全与鉴权")
		}
	}
	if len(segments) == 0 {
		return fmt.Sprintf("业务模块: %s", name)
	}
	sort.Strings(segments)
	return fmt.Sprintf("%s (%s)", name, strings.Join(segments, " + "))
}
